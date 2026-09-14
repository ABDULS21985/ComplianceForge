//go:build integration

package repository_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

type failingFeatureFlagOutbox struct{}

func (failingFeatureFlagOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced feature flag outbox failure")
}

func TestFeatureFlagsWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	actorA, actorB := uuid.NewString(), uuid.NewString()
	planID := uuid.NewString()
	roleName := "grc_feature_flag_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Feature Flag A',$3,'active','starter'),($2,'Feature Flag B',$4,'active','starter')`,
		orgA, orgB, "feature-a-"+orgA, "feature-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Admin','active'),($4,$5,$6,'Bob','Admin','active')`,
		actorA, orgA, actorA+"@example.test", actorB, orgB, actorB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription_plans
		(id,name,slug,tier,max_users,max_frameworks,max_risks,max_vendors,max_storage_gb,features)
		VALUES ($1,'Feature Test Professional',$2,'professional',5,4,20,3,2,
		'{"basic_reporting":true,"advanced_reporting":true,"incident_management":true,"custom_branding":false}')`,
		planID, "feature-plan-"+planID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_subscriptions_v2
		(organization_id,plan_id,status,billing_cycle) VALUES ($1,$2,'active','monthly')`, orgA, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role: %v", err)
	}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM subscription_plans WHERE id=$1`, planID)
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}
	defer cleanup()
	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT ON organizations,users,subscription_plans,organization_subscriptions,organization_subscriptions_v2," +
		"organization_frameworks,risks,vendors,control_evidence,product_capabilities TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON tenant_feature_flag_overrides,feature_flag_change_events TO " + quotedRole +
		"; GRANT SELECT,INSERT ON queue_outbox TO " + quotedRole
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
	setTenant := func(connection *pgxpool.Conn, orgID string) context.Context {
		t.Helper()
		if _, err := connection.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgID); err != nil {
			t.Fatal(err)
		}
		return database.WithQuerier(ctx, connection)
	}
	tenantA := setTenant(conn, orgA)

	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewFeatureFlagRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	featureService := service.NewFeatureFlagService(repo, zerolog.Nop())

	evaluations, err := featureService.ListEvaluations(tenantA, orgA)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluations) < 20 {
		t.Fatalf("capability count=%d", len(evaluations))
	}
	byKey := make(map[string]models.FeatureFlagEvaluation, len(evaluations))
	for _, evaluation := range evaluations {
		byKey[evaluation.Capability.Key] = evaluation
	}
	if !byKey["advanced_reporting"].Enabled || byKey["custom_branding"].Enabled || byKey["custom_branding"].Reason != "subscription_denied" {
		t.Fatalf("plan evaluation advanced=%+v branding=%+v", byKey["advanced_reporting"], byKey["custom_branding"])
	}
	entitlements, err := featureService.GetEntitlements(tenantA, orgA)
	if err != nil {
		t.Fatal(err)
	}
	if entitlements.Source != "subscription_plan" || entitlements.Tier != "professional" || entitlements.Usage["users"] != 1 || entitlements.Limits["users"] != 5 {
		t.Fatalf("entitlements=%+v", entitlements)
	}
	capacity, err := featureService.CheckLimit(tenantA, orgA, "users", 5)
	if err != nil || capacity.Allowed || capacity.Remaining != 4 {
		t.Fatalf("capacity=%+v err=%v", capacity, err)
	}
	quotaTransaction, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureEntitlementCapacity(ctx, quotaTransaction, orgA, "users", 5); !errors.Is(err, repository.ErrEntitlementLimitExceeded) {
		_ = quotaTransaction.Rollback(ctx)
		t.Fatalf("atomic quota error=%v", err)
	}
	if err := quotaTransaction.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	failingRepo, err := repository.NewFeatureFlagRepository(pool, failingFeatureFlagOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.NewFeatureFlagService(failingRepo, zerolog.Nop()).UpsertOverride(
		tenantA, orgA, "advanced_reporting", actorA, "request-failed",
		models.FeatureFlagOverrideInput{Enabled: false, Reason: "Must roll back atomically"},
	)
	if err == nil || !strings.Contains(err.Error(), "forced feature flag outbox failure") {
		t.Fatalf("forced outbox error=%v", err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM tenant_feature_flag_overrides WHERE capability_key='advanced_reporting'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed mutation survived count=%d err=%v", count, err)
	}

	rollout := 2500
	override, err := featureService.UpsertOverride(tenantA, orgA, "advanced_reporting", actorA, "request-create", models.FeatureFlagOverrideInput{
		Enabled: true, RolloutBasisPoints: &rollout, Variant: map[string]any{"dashboard": "v2"}, Reason: "Controlled tenant rollout",
	})
	if err != nil {
		t.Fatal(err)
	}
	if override.Version != 1 || override.RolloutBasisPoints == nil || *override.RolloutBasisPoints != rollout {
		t.Fatalf("created override=%+v", override)
	}
	if _, err := featureService.UpsertOverride(tenantA, orgA, "advanced_reporting", actorA, "request-blind", models.FeatureFlagOverrideInput{
		Enabled: false, Reason: "Blind overwrite must fail",
	}); !errors.Is(err, service.ErrFeatureFlagConflict) {
		t.Fatalf("blind overwrite error=%v", err)
	}
	versionOne := int64(1)
	override, err = featureService.UpsertOverride(tenantA, orgA, "advanced_reporting", actorA, "request-update", models.FeatureFlagOverrideInput{
		Enabled: false, Reason: "Disable after staged review", ExpectedVersion: &versionOne,
	})
	if err != nil || override.Version != 2 {
		t.Fatalf("updated override=%+v err=%v", override, err)
	}
	if _, err := featureService.UpsertOverride(tenantA, orgA, "advanced_reporting", actorA, "request-stale", models.FeatureFlagOverrideInput{
		Enabled: true, Reason: "Stale update must fail", ExpectedVersion: &versionOne,
	}); !errors.Is(err, service.ErrFeatureFlagConflict) {
		t.Fatalf("stale update error=%v", err)
	}

	// Tenant B cannot observe or mutate tenant A's override, even through direct SQL.
	tenantB := setTenant(conn, orgB)
	overridesB, err := repo.ListOverrides(tenantB, orgB)
	if err != nil || len(overridesB) != 0 {
		t.Fatalf("tenant B overrides=%+v err=%v", overridesB, err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO tenant_feature_flag_overrides
		(organization_id,capability_key,enabled,reason,created_by,updated_by)
		VALUES ($1,'basic_reporting',false,'Cross tenant write',$2,$2)`, orgA, actorA); err == nil {
		t.Fatal("database accepted a cross-tenant feature flag override")
	}
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM tenant_feature_flag_overrides WHERE organization_id=$1`, orgA).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cross-tenant visibility count=%d err=%v", count, err)
	}

	tenantA = setTenant(conn, orgA)
	if err := featureService.ResetOverride(tenantA, orgA, "advanced_reporting", actorA, "request-reset-stale", models.FeatureFlagResetInput{
		ExpectedVersion: 1, Reason: "Stale reset must fail",
	}); !errors.Is(err, service.ErrFeatureFlagConflict) {
		t.Fatalf("stale reset error=%v", err)
	}
	if err := featureService.ResetOverride(tenantA, orgA, "advanced_reporting", actorA, "request-reset", models.FeatureFlagResetInput{
		ExpectedVersion: 2, Reason: "Return to plan default",
	}); err != nil {
		t.Fatal(err)
	}
	events, total, err := featureService.ListEvents(tenantA, orgA, "advanced_reporting", models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || total != 3 || len(events) != 3 || events[0].EventType != "reset" {
		t.Fatalf("events=%+v total=%d err=%v", events, total, err)
	}
	mutation, mutationErr := conn.Exec(ctx, `UPDATE feature_flag_change_events SET reason='tampered'
		WHERE organization_id=$1 AND capability_key='advanced_reporting'`, orgA)
	if mutationErr == nil && mutation.RowsAffected() != 0 {
		t.Fatal("tenant role changed feature flag audit history")
	}
	if _, err := pool.Exec(ctx, `UPDATE feature_flag_change_events SET reason='tampered'
		WHERE organization_id=$1 AND capability_key='advanced_reporting'`, orgA); err == nil {
		t.Fatal("append-only trigger allowed a privileged feature flag audit history mutation")
	}
	var outboxCount int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM queue_outbox
		WHERE tenant_id=$1 AND envelope->'payload'->>'type' LIKE 'settings.feature_flag.%'`, orgA).Scan(&outboxCount); err != nil || outboxCount != 3 {
		t.Fatalf("outbox count=%d err=%v", outboxCount, err)
	}

	// Two writers using the same optimistic version must produce one winner.
	concurrent, err := featureService.UpsertOverride(tenantA, orgA, "incident_management", actorA, "request-race-create", models.FeatureFlagOverrideInput{
		Enabled: true, Reason: "Prepare concurrency test",
	})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			worker, acquireErr := pool.Acquire(ctx)
			if acquireErr != nil {
				results <- acquireErr
				return
			}
			defer worker.Release()
			if _, roleErr := worker.Exec(ctx, "SET ROLE "+quotedRole); roleErr != nil {
				results <- roleErr
				return
			}
			defer func() { _, _ = worker.Exec(context.Background(), "RESET ROLE") }()
			workerCtx := setTenant(worker, orgA)
			expected := concurrent.Version
			_, updateErr := featureService.UpsertOverride(workerCtx, orgA, "incident_management", actorA,
				"request-race", models.FeatureFlagOverrideInput{
					Enabled: index == 0, Reason: "Concurrent guarded update", ExpectedVersion: &expected,
				})
			results <- updateErr
		}(index)
	}
	wait.Wait()
	close(results)
	winners, conflicts := 0, 0
	for updateErr := range results {
		switch {
		case updateErr == nil:
			winners++
		case errors.Is(updateErr, service.ErrFeatureFlagConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error=%v", updateErr)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("concurrent outcomes winners=%d conflicts=%d", winners, conflicts)
	}
}
