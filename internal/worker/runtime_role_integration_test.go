//go:build integration

package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/pkg/coordination"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/service"
)

// TestSeparatedRuntimeRolesLive is the executable contract for the reviewed
// runtime role manifests. It must run against a disposable, fully migrated
// PostgreSQL database after runtime-roles.sql and runtime-grants.sql have been
// applied. Runtime connections authenticate as their distinct logins with
// random disposable-only passwords; production secrets are never required.
func TestSeparatedRuntimeRolesLive(t *testing.T) {
	testDatabaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	apiRole := strings.TrimSpace(os.Getenv("API_POSTURE_TEST_ROLE"))
	workerRole := strings.TrimSpace(os.Getenv("WORKER_POSTURE_TEST_ROLE"))
	if testDatabaseURL == "" || apiRole == "" || workerRole == "" {
		t.Skip("TEST_DATABASE_URL, API_POSTURE_TEST_ROLE, and WORKER_POSTURE_TEST_ROLE are required")
	}
	if os.Getenv("RUNTIME_ROLE_SMOKE_CONFIRM_DISPOSABLE") != "yes" {
		t.Skip("actual-login smoke requires RUNTIME_ROLE_SMOKE_CONFIRM_DISPOSABLE=yes")
	}
	if !strings.HasPrefix(apiRole, "cf_api_smoke_") || !strings.HasPrefix(workerRole, "cf_worker_smoke_") {
		t.Fatal("actual-login smoke requires script-created temporary runtime roles; existing credential passwords are never changed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, testDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)
	apiPool := openRolePool(t, ctx, adminPool, testDatabaseURL, apiRole)
	workerPool := openRolePool(t, ctx, adminPool, testDatabaseURL, workerRole)
	t.Run("authenticated principal binding rejects SET ROLE masquerading", func(t *testing.T) {
		for _, pool := range []*pgxpool.Pool{apiPool, workerPool} {
			if err := database.ValidateRuntimeDatabaseLoginIdentity(ctx, pool, pool.Config().ConnConfig.User); err != nil {
				t.Fatalf("dedicated authenticated runtime identity: %v", err)
			}
		}
		config, err := pgxpool.ParseConfig(testDatabaseURL)
		if err != nil {
			t.Fatal(err)
		}
		config.MaxConns = 1
		config.AfterConnect = func(connectCtx context.Context, connection *pgx.Conn) error {
			_, err := connection.Exec(connectCtx, "SET ROLE "+pgx.Identifier{apiRole}.Sanitize())
			return err
		}
		masqueradingPool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		defer masqueradingPool.Close()
		if err := masqueradingPool.Ping(ctx); err != nil {
			t.Fatal(err)
		}
		if err := database.ValidateRuntimeDatabaseLoginIdentity(ctx, masqueradingPool, config.ConnConfig.User); err == nil || !strings.Contains(err.Error(), "role switching") {
			t.Fatalf("privileged credential hidden by SET ROLE was not rejected: %v", err)
		}
	})

	orgA := uuid.NewString()
	orgB := uuid.NewString()
	inactiveOrg := uuid.NewString()
	testSuffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	for _, organization := range []struct {
		id, name, slug, status string
	}{
		{orgA, "Runtime Role A", "runtime-a-" + testSuffix, "active"},
		{orgB, "Runtime Role B", "runtime-b-" + testSuffix, "active"},
		{inactiveOrg, "Runtime Role Inactive", "runtime-i-" + testSuffix, "deactivated"},
	} {
		if _, err := adminPool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status)
			VALUES ($1::uuid,$2,$3,$4::org_status)`, organization.id, organization.name, organization.slug, organization.status); err != nil {
			t.Fatalf("create test organization: %v", err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, _ = adminPool.Exec(cleanupCtx, `DELETE FROM queue_inbox WHERE tenant_id=ANY($1::uuid[]) OR tenant_id IS NULL`, []string{orgA, orgB, inactiveOrg})
		_, _ = adminPool.Exec(cleanupCtx, `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[]) OR tenant_id IS NULL`, []string{orgA, orgB, inactiveOrg})
		_, _ = adminPool.Exec(cleanupCtx, `DELETE FROM scheduler_leases WHERE task_name LIKE $1`, "runtime-role-"+testSuffix+"%")
		_, _ = adminPool.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB, inactiveOrg})
	})

	if err := database.ValidateAPIDatabasePosture(ctx, apiPool); err != nil {
		t.Fatalf("API database posture: %v", err)
	}
	if err := database.ValidateWorkerDatabasePosture(ctx, workerPool, true); err != nil {
		t.Fatalf("worker database posture: %v", err)
	}

	t.Run("API tenant CRUD and queue isolation", func(t *testing.T) {
		testAPIRuntimeRole(t, ctx, apiPool, workerPool, orgA, orgB)
	})
	t.Run("worker durable delivery and lease", func(t *testing.T) {
		testWorkerDurabilityRole(t, ctx, apiPool, workerPool, orgA, orgB, testSuffix)
	})
	t.Run("active tenant stream and scheduler SQL", func(t *testing.T) {
		testWorkerSchedulerRole(t, ctx, workerPool, orgA, orgB, inactiveOrg)
	})
	t.Run("populated tenant scheduler contracts", func(t *testing.T) {
		testPopulatedSchedulerRoles(t, ctx, adminPool, workerPool, orgA, orgB)
	})
}

func openRolePool(t *testing.T, ctx context.Context, adminPool *pgxpool.Pool, databaseURL, roleName string) *pgxpool.Pool {
	t.Helper()
	password := uuid.NewString()
	var passwordStatement string
	if err := adminPool.QueryRow(ctx, `SELECT format('ALTER ROLE %I PASSWORD %L', $1::TEXT, $2::TEXT)`, roleName, password).Scan(&passwordStatement); err != nil {
		t.Fatal("prepare disposable runtime login password")
	}
	if _, err := adminPool.Exec(ctx, passwordStatement); err != nil {
		t.Fatal("provision disposable runtime login password")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 4
	config.ConnConfig.User = roleName
	config.ConnConfig.Password = password
	delete(config.ConnConfig.RuntimeParams, "role")
	delete(config.ConnConfig.RuntimeParams, "session_authorization")
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("connect as role %q: %v", roleName, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testAPIRuntimeRole(
	t *testing.T,
	ctx context.Context,
	apiPool, workerPool *pgxpool.Pool,
	orgA, orgB string,
) {
	t.Helper()
	riskID := uuid.NewString()
	err := database.WithTenantConnection(ctx, apiPool, orgA, func(tenantCtx context.Context) error {
		querier := database.QuerierFromContext(tenantCtx, apiPool)
		if _, err := querier.Exec(tenantCtx, `INSERT INTO risks
			(id,organization_id,title,risk_ref) VALUES ($1::uuid,$2::uuid,'Runtime risk','RUNTIME-SMOKE')`, riskID, orgA); err != nil {
			return fmt.Errorf("insert tenant risk: %w", err)
		}
		var visible int
		if err := querier.QueryRow(tenantCtx, `SELECT count(*) FROM risks
			WHERE id=$1::uuid AND organization_id=$2::uuid`, riskID, orgA).Scan(&visible); err != nil || visible != 1 {
			return fmt.Errorf("read tenant risk count=%d: %w", visible, err)
		}
		if err := querier.QueryRow(tenantCtx, `SELECT count(*) FROM organizations WHERE id=$1::uuid`, orgB).Scan(&visible); err != nil || visible != 0 {
			return fmt.Errorf("cross-tenant organization visibility count=%d: %w", visible, err)
		}
		if err := querier.QueryRow(tenantCtx, `SELECT count(*) FROM effective_user_roles WHERE organization_id=$1::uuid`, orgA).Scan(&visible); err != nil {
			return fmt.Errorf("effective role invoker dependencies: %w", err)
		}
		if _, err := querier.Exec(tenantCtx, `UPDATE risks SET description='updated'
			WHERE id=$1::uuid AND organization_id=$2::uuid`, riskID, orgA); err != nil {
			return fmt.Errorf("update tenant risk: %w", err)
		}
		if _, err := querier.Exec(tenantCtx, `UPDATE risks SET deleted_at=NOW()
			WHERE id=$1::uuid AND organization_id=$2::uuid`, riskID, orgA); err != nil {
			return fmt.Errorf("soft-delete tenant risk: %w", err)
		}
		if _, err := querier.Exec(tenantCtx, `DELETE FROM risks WHERE id=$1::uuid`, riskID); err == nil {
			return errors.New("API runtime unexpectedly has physical risk DELETE")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	apiOutbox, err := queuepkg.NewPostgresOutbox(apiPool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	ownEnvelope, err := queuepkg.NewEnvelope("runtime.role.api", orgA, map[string]string{"scope": "own"})
	if err != nil {
		t.Fatal(err)
	}
	otherEnvelope, err := queuepkg.NewEnvelope("runtime.role.api", orgB, map[string]string{"scope": "other"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		tenantID string
		envelope queuepkg.Envelope
	}{
		{orgA, ownEnvelope},
		{orgB, otherEnvelope},
	} {
		if err := database.WithTenantConnection(ctx, apiPool, item.tenantID, func(tenantCtx context.Context) error {
			querier := database.QuerierFromContext(tenantCtx, apiPool)
			if err := apiOutbox.Enqueue(tenantCtx, querier, "runtime.roles", item.envelope); err != nil {
				return err
			}
			// Exercise the idempotent conflict-read path, not only INSERT.
			return apiOutbox.Enqueue(tenantCtx, querier, "runtime.roles", item.envelope)
		}); err != nil {
			t.Fatalf("enqueue API tenant message: %v", err)
		}
	}

	systemEnvelope, err := queuepkg.NewEnvelope("runtime.role.system", "", map[string]bool{"system": true})
	if err != nil {
		t.Fatal(err)
	}
	workerOutbox, err := queuepkg.NewPostgresOutbox(workerPool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := workerOutbox.Enqueue(ctx, workerPool, "runtime.roles", systemEnvelope); err != nil {
		t.Fatalf("enqueue worker system message: %v", err)
	}

	if err := database.WithTenantConnection(ctx, apiPool, orgA, func(tenantCtx context.Context) error {
		querier := database.QuerierFromContext(tenantCtx, apiPool)
		var crossTenant, system int
		if err := querier.QueryRow(tenantCtx, `SELECT count(*) FROM queue_outbox WHERE message_id=$1::uuid`, otherEnvelope.ID).Scan(&crossTenant); err != nil {
			return err
		}
		if err := querier.QueryRow(tenantCtx, `SELECT count(*) FROM queue_outbox WHERE message_id=$1::uuid`, systemEnvelope.ID).Scan(&system); err != nil {
			return err
		}
		if crossTenant != 0 || system != 0 {
			return fmt.Errorf("API queue visibility leaked cross-tenant=%d system=%d", crossTenant, system)
		}
		crossInsert, err := queuepkg.NewEnvelope("runtime.role.cross", orgB, map[string]bool{"cross": true})
		if err != nil {
			return err
		}
		if err := apiOutbox.Enqueue(tenantCtx, querier, "runtime.roles", crossInsert); err == nil {
			return errors.New("API inserted a cross-tenant outbox message")
		}
		apiSystem, err := queuepkg.NewEnvelope("runtime.role.system", "", map[string]bool{"system": true})
		if err != nil {
			return err
		}
		if err := apiOutbox.Enqueue(tenantCtx, querier, "runtime.roles", apiSystem); err == nil {
			return errors.New("API inserted a system outbox message")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func testWorkerDurabilityRole(
	t *testing.T,
	ctx context.Context,
	apiPool, workerPool *pgxpool.Pool,
	orgA, orgB, suffix string,
) {
	t.Helper()
	outbox, err := queuepkg.NewPostgresOutbox(workerPool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	messages, err := outbox.ClaimBatch(ctx)
	if err != nil {
		t.Fatalf("claim outbox batch: %v", err)
	}
	if len(messages) < 3 {
		t.Fatalf("worker claimed %d messages, want tenant A, tenant B, and system messages", len(messages))
	}
	for _, message := range messages {
		if err := outbox.MarkPublished(ctx, message); err != nil {
			t.Fatalf("mark outbox message published: %v", err)
		}
	}

	deduplicator, err := queuepkg.NewPostgresDeduplicator(workerPool, queuepkg.PostgresDeduplicatorConfig{
		ConsumerName: "runtime-role-" + suffix,
		OwnerID:      uuid.NewString(),
		Lease:        time.Minute,
		Retention:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	var inboxEnvelopes []queuepkg.Envelope
	for _, tenantID := range []string{orgA, orgB, ""} {
		envelope, err := queuepkg.NewEnvelope("runtime.role.inbox", tenantID, map[string]bool{"ok": true})
		if err != nil {
			t.Fatal(err)
		}
		if status, err := deduplicator.Begin(ctx, envelope); err != nil || status != queuepkg.DeduplicationNew {
			t.Fatalf("begin inbox status=%v error=%v", status, err)
		}
		if err := deduplicator.Complete(ctx, envelope); err != nil {
			t.Fatalf("complete inbox: %v", err)
		}
		inboxEnvelopes = append(inboxEnvelopes, envelope)
	}
	if err := database.WithTenantConnection(ctx, apiPool, orgA, func(tenantCtx context.Context) error {
		querier := database.QuerierFromContext(tenantCtx, apiPool)
		var own, cross, system int
		for index, destination := range []*int{&own, &cross, &system} {
			if err := querier.QueryRow(tenantCtx, `SELECT count(*) FROM queue_inbox WHERE message_id=$1::uuid`,
				inboxEnvelopes[index].ID).Scan(destination); err != nil {
				return err
			}
		}
		if own != 1 || cross != 0 || system != 0 {
			return fmt.Errorf("API inbox visibility own=%d cross-tenant=%d system=%d", own, cross, system)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	leaseStore, err := coordination.NewPostgresLeaseStore(workerPool, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	lease, acquired, err := leaseStore.TryAcquire(ctx, "runtime-role-"+suffix, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire scheduler lease acquired=%t error=%v", acquired, err)
	}
	lease, err = leaseStore.Renew(ctx, lease, time.Minute)
	if err != nil {
		t.Fatalf("renew scheduler lease: %v", err)
	}
	if err := leaseStore.Release(ctx, lease, nil); err != nil {
		t.Fatalf("release scheduler lease: %v", err)
	}
}

func testWorkerSchedulerRole(
	t *testing.T,
	ctx context.Context,
	workerPool *pgxpool.Pool,
	orgA, orgB, inactiveOrg string,
) {
	t.Helper()
	visited := make(map[string]bool)
	if err := runForScheduledTenantsPageSize(ctx, workerPool, "role smoke", func(tenantCtx context.Context, tenantID string) error {
		var effectiveTenant string
		if err := database.QuerierFromContext(tenantCtx, workerPool).QueryRow(
			tenantCtx, `SELECT get_current_tenant()::text`,
		).Scan(&effectiveTenant); err != nil {
			return err
		}
		if effectiveTenant != tenantID {
			return fmt.Errorf("effective tenant %s, want %s", effectiveTenant, tenantID)
		}
		visited[tenantID] = true
		return nil
	}, 1); err != nil {
		t.Fatalf("stream active scheduler tenants: %v", err)
	}
	if !visited[orgA] || !visited[orgB] || visited[inactiveOrg] {
		t.Fatalf("active scheduler tenant stream = %v", visited)
	}

	bus := service.NewEventBus()
	t.Cleanup(bus.Close)
	analytics := NewAnalyticsScheduler(workerPool)
	calendar := NewCalendarWorker(workerPool)
	dsr := NewDSRScheduler(workerPool)
	evidence := NewEvidenceScheduler(workerPool, bus)
	exceptions := NewExceptionScheduler(workerPool, bus)
	regulatory := NewRegulatoryScheduler(workerPool, bus)
	reports := NewReportScheduler(workerPool)
	search := NewSearchIndexer(workerPool)
	workflows := NewWorkflowScheduler(workerPool)

	err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
		checks := []struct {
			name string
			run  func(context.Context) error
		}{
			{"analytics daily snapshot", func(c context.Context) error { return analytics.takeOrgSnapshot(c, orgA, dailyAnalyticsSnapshot) }},
			{"analytics weekly snapshot", func(c context.Context) error { return analytics.takeOrgSnapshot(c, orgA, weeklyAnalyticsSnapshot) }},
			{"analytics monthly snapshot", func(c context.Context) error { return analytics.takeOrgSnapshot(c, orgA, monthlyAnalyticsSnapshot) }},
			{"analytics trends", func(c context.Context) error { return analytics.calculateTrendsForTenant(c, orgA) }},
			{"calendar reminders", func(c context.Context) error { return calendar.remindersForTenant(c, orgA) }},
			{"calendar escalations", func(c context.Context) error { return calendar.escalationsForTenant(c, orgA) }},
			{"calendar status", func(c context.Context) error { return calendar.updateStatusForTenant(c, orgA) }},
			{"DSR lifecycle", func(c context.Context) error { return dsr.runForTenant(c, orgA) }},
			{"evidence upcoming", func(c context.Context) error { return evidence.checkUpcomingCollectionsForTenant(c, orgA) }},
			{"evidence expiry capability", func(c context.Context) error { return evidence.expireStaleEvidenceForTenant(c, orgA) }},
			{"evidence overdue", func(c context.Context) error { return evidence.checkOverdueCollectionsForTenant(c, orgA) }},
			{"evidence missing", func(c context.Context) error { return evidence.checkControlsMissingEvidenceForTenant(c, orgA) }},
			{"exception reminders", func(c context.Context) error { return exceptions.checkExpiringExceptionsForTenant(c, orgA) }},
			{"exception expiry", func(c context.Context) error { return exceptions.autoExpireExceptionsForTenant(c, orgA) }},
			{"exception reviews", func(c context.Context) error { return exceptions.checkOverdueReviewsForTenant(c, orgA) }},
			{"GDPR deadlines", func(c context.Context) error { return regulatory.checkGDPRBreachDeadlinesForTenant(c, orgA) }},
			{"NIS2 deadlines", func(c context.Context) error { return regulatory.checkNIS2DeadlinesForTenant(c, orgA) }},
			{"policy reviews", func(c context.Context) error { return regulatory.checkPolicyReviewsForTenant(c, orgA) }},
			{"finding remediation", func(c context.Context) error { return regulatory.checkFindingRemediationsForTenant(c, orgA) }},
			{"vendor assessments", func(c context.Context) error { return regulatory.checkVendorAssessmentsForTenant(c, orgA) }},
			{"risk reviews", func(c context.Context) error { return regulatory.checkRiskReviewsForTenant(c, orgA) }},
			{"regulatory DSR", func(c context.Context) error { return regulatory.checkDSRDeadlinesForTenant(c, orgA) }},
			{"reports", func(c context.Context) error { return reports.runForTenant(c, orgA) }},
			{"workflow", func(c context.Context) error { return workflows.runForTenant(c, orgA) }},
			{"search health", func(c context.Context) error { return search.healthCheckForTenant(c, orgA) }},
			{"search full", func(c context.Context) error { _, err := search.indexAllEntities(c, orgA); return err }},
		}
		for _, check := range checks {
			if err := check.run(tenantCtx); err != nil {
				return fmt.Errorf("%s: %w", check.name, err)
			}
		}

		// Parse every dynamic incremental source query and require the expected
		// no-row result rather than silently accepting a schema error.
		for _, entity := range indexableEntities {
			err := search.incrementalIndexForTenant(tenantCtx, entity.TypeName, uuid.NewString(), orgA, "upsert")
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("incremental %s source query: got %v, want no rows", entity.TypeName, err)
			}
		}

		querier := database.QuerierFromContext(tenantCtx, workerPool)
		indexedID := uuid.NewString()
		if _, err := querier.Exec(tenantCtx, searchIndexUpsertSQL,
			"runtime", indexedID, orgA, "Runtime index", "first body"); err != nil {
			return fmt.Errorf("insert search index: %w", err)
		}
		if _, err := querier.Exec(tenantCtx, searchIndexUpsertSQL,
			"runtime", indexedID, orgA, "Runtime index updated", "second body"); err != nil {
			return fmt.Errorf("update search index conflict: %w", err)
		}
		return search.removeFromIndex(tenantCtx, querier, "runtime", indexedID, orgA)
	})
	if err != nil {
		t.Fatal(err)
	}
}
