//go:build integration

package repository_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/repository"
)

type failingEvidenceLifecycleOutbox struct{}

func (failingEvidenceLifecycleOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced evidence lifecycle outbox failure")
}

func TestEvidenceExpiryAndOutboxCommitAtomically(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	organizationID, userID := uuid.NewString(), uuid.NewString()
	frameworkID, controlID := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier)
		VALUES ($1,'Evidence lifecycle integration',$2,'active','enterprise')`,
		organizationID, "evidence-lifecycle-"+organizationID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		connection, acquireErr := pool.Acquire(cleanupCtx)
		if acquireErr != nil {
			t.Errorf("acquire evidence expiry cleanup connection: %v", acquireErr)
			return
		}
		defer connection.Release()
		if _, cleanupErr := connection.Exec(cleanupCtx, `RESET ROLE`); cleanupErr != nil {
			t.Errorf("reset role before evidence expiry cleanup: %v", cleanupErr)
			return
		}
		if _, cleanupErr := connection.Exec(cleanupCtx, `SELECT set_config('app.current_tenant',$1,false)`, organizationID); cleanupErr != nil {
			t.Errorf("set evidence expiry cleanup tenant: %v", cleanupErr)
			return
		}
		if _, cleanupErr := connection.Exec(cleanupCtx, `DELETE FROM queue_outbox WHERE tenant_id=$1::uuid`, organizationID); cleanupErr != nil {
			t.Errorf("delete evidence expiry outbox fixtures: %v", cleanupErr)
			return
		}
		deleteTag, cleanupErr := connection.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=$1::uuid`, organizationID)
		if cleanupErr != nil {
			t.Errorf("delete evidence expiry fixture organization: %v", cleanupErr)
			return
		}
		if deleteTag.RowsAffected() != 1 {
			t.Errorf("deleted evidence expiry fixture organizations=%d, want 1", deleteTag.RowsAffected())
		}
		if _, cleanupErr = connection.Exec(cleanupCtx, `SELECT set_config('app.current_tenant','',false)`); cleanupErr != nil {
			t.Errorf("clear evidence expiry cleanup tenant: %v", cleanupErr)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status)
		VALUES ($1,$2,$3,'Evidence','Owner','active')`, userID, organizationID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO compliance_frameworks
		(id,organization_id,code,name,version,category)
		VALUES ($1,$2,'EXPIRY_TEST','Expiry Test','1','security')`, frameworkID, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO framework_controls(id,framework_id,code,title)
		VALUES ($1,$2,'EXP.1','Expiring evidence control')`, controlID, frameworkID); err != nil {
		t.Fatal(err)
	}

	frameworkRepository := repository.NewFrameworkRepository(pool)
	controlRepository := repository.NewControlRepository(pool)
	var evidenceID string
	err = database.WithTenantConnection(ctx, pool, organizationID, func(tenantCtx context.Context) error {
		if _, err := frameworkRepository.Adopt(tenantCtx, organizationID, userID, frameworkID); err != nil {
			return err
		}
		yesterday := time.Now().UTC().AddDate(0, 0, -2)
		evidence, err := controlRepository.AttachEvidence(tenantCtx, organizationID, userID, controlID, models.AttachControlEvidenceInput{
			Title: "Evidence due for expiry", EvidenceType: "report", ValidUntil: &yesterday,
		})
		if err != nil {
			return err
		}
		evidenceID = evidence.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	failingRepository, err := repository.NewEvidenceLifecycleRepository(
		pool,
		repository.WithEvidenceLifecycleOutbox(failingEvidenceLifecycleOutbox{}, "complianceforge.worker"),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithTenantConnection(ctx, pool, organizationID, func(tenantCtx context.Context) error {
		_, err := failingRepository.ExpireDueEvidence(tenantCtx, organizationID, 10)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "forced evidence lifecycle outbox failure") {
		t.Fatalf("forced outbox error=%v", err)
	}
	assertEvidenceExpiryState(t, ctx, pool, organizationID, evidenceID, "active", 0, 0)

	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	reliableRepository, err := repository.NewEvidenceLifecycleRepository(
		pool,
		repository.WithEvidenceLifecycleOutbox(outbox, "complianceforge.worker"),
	)
	if err != nil {
		t.Fatal(err)
	}
	discoveredTenants, err := reliableRepository.DueEvidenceTenants(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	foundTenant := false
	for _, discoveredTenant := range discoveredTenants {
		if discoveredTenant == organizationID {
			foundTenant = true
			break
		}
	}
	if !foundTenant {
		t.Fatalf("keyset tenant discovery omitted %s from %v", organizationID, discoveredTenants)
	}
	var notices []models.ExpiredEvidenceNotice
	err = database.WithTenantConnection(ctx, pool, organizationID, func(tenantCtx context.Context) error {
		var expireErr error
		notices, expireErr = reliableRepository.ExpireDueEvidence(tenantCtx, organizationID, 10)
		return expireErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || notices[0].EvidenceID != evidenceID || !notices[0].NotificationQueued {
		t.Fatalf("expiry notices=%+v", notices)
	}
	assertEvidenceExpiryState(t, ctx, pool, organizationID, evidenceID, "expired", 1, 1)

	err = database.WithTenantConnection(ctx, pool, organizationID, func(tenantCtx context.Context) error {
		replayed, replayErr := reliableRepository.ExpireDueEvidence(tenantCtx, organizationID, 10)
		if replayErr == nil && len(replayed) != 0 {
			t.Fatalf("second expiry returned notices=%+v", replayed)
		}
		return replayErr
	})
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceExpiryState(t, ctx, pool, organizationID, evidenceID, "expired", 1, 1)
}

func assertEvidenceExpiryState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	organizationID, evidenceID, wantStatus string,
	wantExpiredEvents, wantOutbox int,
) {
	t.Helper()
	var status string
	var current bool
	var expiredEvents, outboxMessages int
	if err := pool.QueryRow(ctx, `SELECT lifecycle_status,is_current FROM control_evidence
		WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, evidenceID).
		Scan(&status, &current); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM evidence_custody_events
		WHERE organization_id=$1::uuid AND evidence_id=$2::uuid AND event_type='expired'`,
		organizationID, evidenceID).Scan(&expiredEvents); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_outbox
		WHERE tenant_id=$1::uuid AND message_type='notification.event'
		  AND envelope->'payload'->>'entity_id'=$2`, organizationID, evidenceID).Scan(&outboxMessages); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || current != (wantStatus == "active") ||
		expiredEvents != wantExpiredEvents || outboxMessages != wantOutbox {
		t.Fatalf("state status=%s current=%t expired_events=%d outbox=%d; want status=%s events=%d outbox=%d",
			status, current, expiredEvents, outboxMessages, wantStatus, wantExpiredEvents, wantOutbox)
	}
}
