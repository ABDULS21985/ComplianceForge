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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/repository"
)

func TestDiagnosticsRepositoryWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB := uuid.NewString(), uuid.NewString()
	integrationA, integrationB := uuid.NewString(), uuid.NewString()
	messageA, messageB := uuid.NewString(), uuid.NewString()
	roleName := "grc_diagnostics_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()

	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Diagnostics A',$3,'active','enterprise'),($2,'Diagnostics B',$4,'active','enterprise')`,
		orgA, orgB, "diagnostics-a-"+orgA, "diagnostics-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}()
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Operator','active'),($4,$5,$6,'Bob','Operator','active')`,
		userA, orgA, userA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO integrations
		(id,organization_id,integration_type,name,status,configuration_encrypted,health_status,created_by)
		VALUES ($1,$2,'custom_api','Diagnostics A','active','encrypted','unhealthy',$3),
		       ($4,$5,'custom_api','Diagnostics B','active','encrypted','healthy',$6)`,
		integrationA, orgA, userA, integrationB, orgB, userB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO integration_sync_logs
		(organization_id,integration_id,sync_type,status,created_at)
		VALUES ($1,$2,'incremental','failed',NOW()),($3,$4,'incremental','failed',NOW())`,
		orgA, integrationA, orgB, integrationB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notifications
		(organization_id,event_type,event_payload,recipient_user_id,channel_type,status,
		 delivery_key,body_text,scheduled_for,created_at)
		VALUES ($1,'diagnostics.test','{}',$2,'in_app','pending',encode(digest($3::text,'sha256'),'hex'),'test',NOW()-INTERVAL '20 minutes',NOW()-INTERVAL '20 minutes'),
		       ($4,'diagnostics.test','{}',$5,'in_app','pending',encode(digest($6::text,'sha256'),'hex'),'test',NOW()-INTERVAL '2 minutes',NOW()-INTERVAL '2 minutes')`,
		orgA, userA, uuid.NewString(), orgB, userB, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO queue_outbox
		(message_id,queue_name,tenant_id,message_type,schema_version,correlation_id,envelope,available_at)
		VALUES
			($1::uuid,'diagnostics.test',$2::uuid,'diagnostics.test',1,'diagnostics-a',
			 jsonb_build_object('id',($1::uuid)::text,'type','diagnostics.test','schema_version',1,'tenant_id',($2::uuid)::text,'correlation_id','diagnostics-a','attempt',1),
			 NOW()-INTERVAL '10 minutes'),
			($3::uuid,'diagnostics.test',$4::uuid,'diagnostics.test',1,'diagnostics-b',
			 jsonb_build_object('id',($3::uuid)::text,'type','diagnostics.test','schema_version',1,'tenant_id',($4::uuid)::text,'correlation_id','diagnostics-b','attempt',1),
		 NOW()-INTERVAL '1 minute')`, messageA, orgA, messageB, orgB); err != nil {
		t.Fatal(err)
	}
	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT ON schema_migrations,queue_outbox,queue_inbox,notifications,integrations,integration_sync_logs TO " + quotedRole
	if _, err := pool.Exec(ctx, grant); err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "RESET ROLE") }()

	repo, err := repository.NewDiagnosticsRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	load := func(tenantID, requestedID string) (*repositoryDiagnostics, error) {
		t.Helper()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, tenantID); err != nil {
			t.Fatal(err)
		}
		result, err := repo.LoadOperationalDiagnostics(database.WithQuerier(ctx, conn), requestedID)
		if err != nil {
			return nil, err
		}
		return &repositoryDiagnostics{
			queuePending: result.Queue.Pending, notificationDue: result.Notifications.Due,
			connectorTotal: result.Connectors.Total, unhealthy: result.Connectors.Unhealthy,
			healthy: result.Connectors.Healthy, failedSyncs: result.Connectors.RecentFailedSyncs,
			migrationVersion: result.Migration.CurrentVersion,
		}, nil
	}

	a, err := load(orgA, orgA)
	if err != nil {
		t.Fatal(err)
	}
	if a.queuePending != 1 || a.notificationDue != 1 || a.connectorTotal != 1 || a.unhealthy != 1 || a.healthy != 0 || a.failedSyncs != 1 || a.migrationVersion == 0 {
		t.Fatalf("tenant A diagnostics = %+v", a)
	}
	b, err := load(orgB, orgB)
	if err != nil {
		t.Fatal(err)
	}
	if b.queuePending != 1 || b.notificationDue != 1 || b.connectorTotal != 1 || b.unhealthy != 0 || b.healthy != 1 || b.failedSyncs != 1 {
		t.Fatalf("tenant B diagnostics = %+v", b)
	}
	if _, err := load(orgB, orgA); !errors.Is(err, repository.ErrDiagnosticsTenantContext) {
		t.Fatalf("cross-tenant diagnostics error = %v", err)
	}
}

type repositoryDiagnostics struct {
	queuePending     int64
	notificationDue  int64
	connectorTotal   int64
	unhealthy        int64
	healthy          int64
	failedSyncs      int64
	migrationVersion int64
}
