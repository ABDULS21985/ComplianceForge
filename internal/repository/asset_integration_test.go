package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
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

type failingAssetOutbox struct{}

func (failingAssetOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced asset outbox failure")
}

// TestAssetRepositoryWithNonSuperuserTenants proves both explicit tenant
// predicates and PostgreSQL FORCE RLS. It also exercises atomic references,
// transaction rollback, lifecycle history, concurrency control, filtering,
// statistics, and the deferred incident-to-asset tenant foreign key.
func TestAssetRepositoryWithNonSuperuserTenants(t *testing.T) {
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
	userA, ownerA, userB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleName := "grc_asset_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Asset RLS A',$3,'active','starter'),($2,'Asset RLS B',$4,'active','starter')`,
		orgA, orgB, "asset-a-"+orgA, "asset-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Administrator','active'),
		($4,$2,$5,'Olivia','Owner','active'),
		($6,$7,$8,'Bob','Administrator','active')`,
		userA, orgA, userA+"@example.test", ownerA, ownerA+"@example.test",
		userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role: %v", err)
	}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}
	defer cleanup()
	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT ON organizations,users TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON asset_reference_sequences,assets TO " + quotedRole +
		"; GRANT SELECT,INSERT ON asset_events,queue_outbox TO " + quotedRole
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
	setTenant := func(orgID string) context.Context {
		t.Helper()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgID); err != nil {
			t.Fatal(err)
		}
		return database.WithQuerier(ctx, conn)
	}

	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewAssetRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	assetService := service.NewAssetService(repo, zerolog.Nop())
	tenantA := setTenant(orgA)

	// Lifecycle history is part of the same transaction as the asset. Removing
	// event permission must therefore roll back both the record and its sequence.
	if _, err := pool.Exec(ctx, "REVOKE INSERT ON asset_events FROM "+quotedRole); err != nil {
		t.Fatal(err)
	}
	_, err = assetService.Create(tenantA, orgA, userA, models.AssetCreateInput{
		Name: "Must roll back", AssetType: models.AssetTypeData,
	})
	if err == nil {
		t.Fatal("expected lifecycle persistence failure")
	}
	var rolledBack int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM assets WHERE organization_id=$1`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("asset mutation survived event failure count=%d err=%v", rolledBack, err)
	}
	if _, err := pool.Exec(ctx, "GRANT INSERT ON asset_events TO "+quotedRole); err != nil {
		t.Fatal(err)
	}
	failingRepo, err := repository.NewAssetRepository(pool, failingAssetOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.NewAssetService(failingRepo, zerolog.Nop()).Create(tenantA, orgA, userA, models.AssetCreateInput{
		Name: "Outbox must roll back", AssetType: models.AssetTypeData,
	})
	if err == nil || !strings.Contains(err.Error(), "forced asset outbox failure") {
		t.Fatalf("forced outbox error=%v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM assets WHERE organization_id=$1`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("asset mutation survived outbox failure count=%d err=%v", rolledBack, err)
	}

	ip := "10.42.0.8"
	assetA, err := assetService.Create(tenantA, orgA, userA, models.AssetCreateInput{
		Name: "Customer identity store", AssetType: models.AssetTypeData,
		Criticality: models.AssetCriticalityCritical, Classification: models.AssetClassificationRestricted,
		OwnerUserID: &ownerA, IPAddress: &ip, ProcessesPersonalData: true,
		Tags: []string{"PII", "Crown-Jewel", "pii"},
	})
	if err != nil {
		t.Fatalf("creating first asset: %v", err)
	}
	if assetA.AssetRef != "AST-000001" || assetA.Version != 1 || assetA.Owner == nil || len(assetA.Tags) != 2 {
		t.Fatalf("first asset=%#v", assetA)
	}

	// The service and database must both reject a cross-tenant owner.
	_, err = assetService.Create(tenantA, orgA, userA, models.AssetCreateInput{
		Name: "Invalid owner", AssetType: models.AssetTypeSoftware, OwnerUserID: &userB,
	})
	if !errors.Is(err, service.ErrAssetOwnerNotFound) {
		t.Fatalf("cross-tenant owner error=%v", err)
	}

	const concurrentCreates = 8
	referenceResults := make(chan string, concurrentCreates)
	createErrors := make(chan error, concurrentCreates)
	var createGroup sync.WaitGroup
	for index := 0; index < concurrentCreates; index++ {
		index := index
		createGroup.Add(1)
		go func() {
			defer createGroup.Done()
			workerConn, acquireErr := pool.Acquire(ctx)
			if acquireErr != nil {
				createErrors <- acquireErr
				return
			}
			defer workerConn.Release()
			if _, roleErr := workerConn.Exec(ctx, "SET ROLE "+quotedRole); roleErr != nil {
				createErrors <- roleErr
				return
			}
			defer func() {
				_, _ = workerConn.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`)
				_, _ = workerConn.Exec(context.Background(), "RESET ROLE")
			}()
			if _, tenantErr := workerConn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); tenantErr != nil {
				createErrors <- tenantErr
				return
			}
			created, createErr := assetService.Create(database.WithQuerier(ctx, workerConn), orgA, userA, models.AssetCreateInput{
				Name: fmt.Sprintf("Concurrent service %d", index), AssetType: models.AssetTypeService,
			})
			if createErr != nil {
				createErrors <- createErr
				return
			}
			referenceResults <- created.AssetRef
		}()
	}
	createGroup.Wait()
	close(referenceResults)
	close(createErrors)
	for createErr := range createErrors {
		t.Fatalf("concurrent asset create: %v", createErr)
	}
	references := make([]string, 0, concurrentCreates)
	for reference := range referenceResults {
		references = append(references, reference)
	}
	sort.Strings(references)
	if len(references) != concurrentCreates || references[0] != "AST-000002" || references[len(references)-1] != "AST-000009" {
		t.Fatalf("concurrent references=%v", references)
	}

	status := models.AssetStatusInactive
	updated, err := assetService.Update(tenantA, orgA, assetA.ID, userA, models.AssetPatch{
		Status: &status, ExpectedVersion: &assetA.Version,
	})
	if err != nil || updated.Version != 2 || updated.Status != status {
		t.Fatalf("updated asset=%#v err=%v", updated, err)
	}
	if _, err := assetService.Update(tenantA, orgA, assetA.ID, userA, models.AssetPatch{
		Status: &status, ExpectedVersion: &assetA.Version,
	}); !errors.Is(err, service.ErrAssetConflict) {
		t.Fatalf("stale update error=%v", err)
	}
	events, eventTotal, err := assetService.ListEvents(tenantA, orgA, assetA.ID, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || eventTotal != 2 || len(events) != 2 || events[0].EventType != "status_changed" {
		t.Fatalf("events=%#v total=%d err=%v", events, eventTotal, err)
	}

	listed, total, err := assetService.List(tenantA, orgA, models.AssetListFilter{
		PaginationRequest: models.PaginationRequest{Page: 1, PageSize: 20},
		AssetType:         string(models.AssetTypeData), Tag: "PII", Search: "identity",
	})
	if err != nil || total != 1 || len(listed) != 1 || listed[0].ID != assetA.ID {
		t.Fatalf("filtered assets=%#v total=%d err=%v", listed, total, err)
	}
	stats, err := assetService.Stats(tenantA, orgA)
	if err != nil || stats.Total != concurrentCreates+1 || stats.Critical != 1 || stats.PersonalData != 1 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}

	tenantB := setTenant(orgB)
	assetB, err := assetService.Create(tenantB, orgB, userB, models.AssetCreateInput{
		Name: "Tenant B gateway", AssetType: models.AssetTypeNetwork,
	})
	if err != nil || assetB.AssetRef != "AST-000001" {
		t.Fatalf("tenant B asset=%#v err=%v", assetB, err)
	}
	if _, err := assetService.GetByID(tenantB, orgB, assetA.ID); !errors.Is(err, service.ErrAssetNotFound) {
		t.Fatalf("cross-tenant read error=%v", err)
	}
	name := "Cross-tenant overwrite"
	if _, err := assetService.Update(tenantB, orgB, assetA.ID, userB, models.AssetPatch{Name: &name}); !errors.Is(err, service.ErrAssetNotFound) {
		t.Fatalf("cross-tenant update error=%v", err)
	}
	var visible int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM assets`).Scan(&visible); err != nil || visible != 1 {
		t.Fatalf("tenant B RLS visibility=%d err=%v", visible, err)
	}

	// Prove the composite FK added after the incident table rejects an asset
	// owned by another tenant. Resetting the role ensures this tests the FK, not
	// merely the RLS policy.
	if _, err := conn.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	_, crossTenantErr := conn.Exec(ctx, `INSERT INTO incidents
		(organization_id,incident_ref,title,description,category,severity,reporter_id,detected_at,related_asset_id)
		VALUES ($1,'INC-ASSET-FK','Cross tenant asset','Foreign asset link test','operations','low',$2,NOW(),$3)`,
		orgB, userB, assetA.ID)
	if crossTenantErr == nil {
		t.Fatal("cross-tenant incident-to-asset link unexpectedly succeeded")
	}
	if _, err := conn.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
		t.Fatal(err)
	}
	_ = setTenant(orgA)

	if err := assetService.Delete(tenantA, orgA, assetA.ID, userA, &updated.Version); err != nil {
		t.Fatalf("soft deleting asset: %v", err)
	}
	if _, err := assetService.GetByID(tenantA, orgA, assetA.ID); !errors.Is(err, service.ErrAssetNotFound) {
		t.Fatalf("soft-deleted read error=%v", err)
	}
	var deletedEvents int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM asset_events WHERE asset_id=$1`, assetA.ID).Scan(&deletedEvents); err != nil || deletedEvents != 3 {
		t.Fatalf("deleted lifecycle events=%d err=%v", deletedEvents, err)
	}
	var outboxEvents int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM queue_outbox WHERE tenant_id=$1`, orgA).Scan(&outboxEvents); err != nil || outboxEvents != concurrentCreates+3 {
		t.Fatalf("asset outbox events=%d err=%v", outboxEvents, err)
	}
}
