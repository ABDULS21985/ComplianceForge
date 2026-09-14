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
	workerpkg "github.com/complianceforge/platform/internal/worker"
)

type failingVendorOutbox struct{}

func (failingVendorOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced vendor outbox failure")
}

// TestVendorRepositoryWithNonSuperuserTenants exercises the vendor aggregate
// through a NOSUPERUSER/NOBYPASSRLS role and validates reference allocation,
// lifecycle/outbox atomicity, related records, due scheduling, optimistic
// concurrency, deferred tenant FKs, and cross-tenant isolation.
func TestVendorRepositoryWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, ownerA, userB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleName := "grc_vendor_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err = pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier)VALUES($1,'Vendor RLS A',$3,'active','starter'),($2,'Vendor RLS B',$4,'active','starter')`, orgA, orgB, "vendor-a-"+orgA, "vendor-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status)VALUES($1,$2,$3,'Alice','Admin','active'),($4,$2,$5,'Olivia','Owner','active'),($6,$7,$8,'Bob','Admin','active')`, userA, orgA, userA+"@example.test", ownerA, ownerA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
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
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON vendor_reference_sequences,vendors,vendor_contacts,vendor_contracts,vendor_certifications,vendor_subprocessors TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON vendor_events TO " + quotedRole +
		"; GRANT SELECT,INSERT ON queue_outbox TO " + quotedRole
	if _, err = pool.Exec(ctx, grant); err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
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
	repo, err := repository.NewVendorRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	vendorService := service.NewVendorService(repo, zerolog.Nop())
	tenantA := setTenant(orgA)
	now := time.Now().UTC().Truncate(time.Microsecond)
	nextAssessment := now.AddDate(0, 0, 5)
	contractEnd := now.AddDate(0, 1, 0)
	certExpiry := now.AddDate(0, 0, 10)
	create := models.VendorCreateInput{Name: "Nimbus Hosting", LegalName: "Nimbus Hosting Limited", CountryCode: "NG", OwnerUserID: &ownerA, Criticality: models.VendorCriticalityHigh, VendorTier: models.VendorTierOne, RiskTier: models.VendorRiskHigh, ServiceDescription: "Managed production hosting", Services: []string{"hosting"}, NextAssessmentDate: &nextAssessment, ContactName: "Ada Vendor", ContactEmail: "ada.vendor@example.test", Certifications: []string{"ISO 27001"}}

	failingRepo, err := repository.NewVendorRepository(pool, failingVendorOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.NewVendorService(failingRepo, zerolog.Nop()).Create(tenantA, orgA, userA, create); err == nil || !strings.Contains(err.Error(), "forced vendor outbox failure") {
		t.Fatalf("forced outbox error=%v", err)
	}
	var rolledBack int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM vendors WHERE organization_id=$1`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("vendor survived outbox rollback count=%d err=%v", rolledBack, err)
	}

	vendorA, err := vendorService.Create(tenantA, orgA, userA, create)
	if err != nil {
		t.Fatalf("create vendor A: %v", err)
	}
	if vendorA.VendorRef != "VND-000001" || vendorA.Version != 1 || len(vendorA.Contacts) != 1 || len(vendorA.CertificationDetails) != 1 || vendorA.Owner == nil {
		t.Fatalf("vendor A=%#v", vendorA)
	}
	if _, err = vendorService.Create(tenantA, orgA, userA, models.VendorCreateInput{Name: "Invalid owner", OwnerUserID: &userB}); !errors.Is(err, service.ErrVendorUserNotFound) {
		t.Fatalf("cross-tenant owner error=%v", err)
	}

	const concurrentCreates = 8
	references := make(chan string, concurrentCreates)
	createErrors := make(chan error, concurrentCreates)
	var group sync.WaitGroup
	for index := 0; index < concurrentCreates; index++ {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
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
			created, createErr := vendorService.Create(database.WithQuerier(ctx, workerConn), orgA, userA, models.VendorCreateInput{Name: fmt.Sprintf("Concurrent vendor %d", index)})
			if createErr != nil {
				createErrors <- createErr
				return
			}
			references <- created.VendorRef
		}()
	}
	group.Wait()
	close(references)
	close(createErrors)
	for createErr := range createErrors {
		t.Fatalf("concurrent create: %v", createErr)
	}
	refs := []string{}
	for ref := range references {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	if len(refs) != concurrentCreates || refs[0] != "VND-000002" || refs[len(refs)-1] != "VND-000009" {
		t.Fatalf("references=%v", refs)
	}

	vendorA, err = vendorService.Transition(tenantA, orgA, userA, vendorA.ID, models.VendorTransitionInput{Status: models.VendorStatusOnboarding, Version: vendorA.Version})
	if err != nil {
		t.Fatal(err)
	}
	vendorA, err = vendorService.Transition(tenantA, orgA, userA, vendorA.ID, models.VendorTransitionInput{Status: models.VendorStatusActive, Version: vendorA.Version})
	if err != nil {
		t.Fatal(err)
	}
	contractInput := models.VendorContractInput{Version: vendorA.Version, ContractRef: "CTR-001", Name: "Master services agreement", Status: "active", StartDate: &now, EndDate: &contractEnd, RenewalDate: &contractEnd, ValueAmount: float64Pointer(125000), Currency: "EUR", OwnerUserID: &ownerA}
	contract, vendorA, err := vendorService.SaveContract(tenantA, orgA, userA, vendorA.ID, "", contractInput)
	if err != nil {
		t.Fatal(err)
	}
	certInput := models.VendorCertificationInput{Version: vendorA.Version, Name: "SOC 2 Type II", Status: "active", IssuedOn: &now, ExpiresOn: &certExpiry}
	cert, vendorA, err := vendorService.SaveCertification(tenantA, orgA, userA, vendorA.ID, "", certInput)
	if err != nil {
		t.Fatal(err)
	}
	subInput := models.VendorSubprocessorInput{Version: vendorA.Version, Name: "Compute Partner", Purpose: "Provides managed compute capacity", CountryCode: "IE", DataCategories: []string{"customer data"}, Status: "approved"}
	_, vendorA, err = vendorService.SaveSubprocessor(tenantA, orgA, userA, vendorA.ID, "", subInput)
	if err != nil {
		t.Fatal(err)
	}
	assessment := models.VendorAssessmentInput{Version: vendorA.Version, Status: models.VendorAssessmentCompleted, RiskTier: models.VendorRiskMedium, RiskScore: float64Pointer(42), AssessedAt: now, NextAssessmentDate: &nextAssessment, Notes: "Reviewed assurance reports and open remediation items"}
	vendorA, err = vendorService.RecordAssessment(tenantA, orgA, userA, vendorA.ID, assessment)
	if err != nil {
		t.Fatal(err)
	}
	if vendorA.RiskTier != models.VendorRiskMedium || vendorA.AssessmentStatus != models.VendorAssessmentCompleted {
		t.Fatalf("assessed vendor=%#v", vendorA)
	}

	due, err := vendorService.ListDueForAssessment(tenantA, orgA, 30, 100)
	if err != nil || len(due) != 1 || due[0].ID != vendorA.ID {
		t.Fatalf("due vendors=%#v err=%v", due, err)
	}
	dueContracts, err := vendorService.ListDueContracts(tenantA, orgA, 60, 100)
	if err != nil || len(dueContracts) != 1 || dueContracts[0].Contract.ID != contract.ID {
		t.Fatalf("due contracts=%#v err=%v", dueContracts, err)
	}
	expiring, err := vendorService.ListExpiringCertifications(tenantA, orgA, 30, 100)
	if err != nil || len(expiring) != 1 || expiring[0].Certification.ID != cert.ID {
		t.Fatalf("expiring certifications=%#v err=%v", expiring, err)
	}
	stats, err := vendorService.Statistics(tenantA, orgA)
	if err != nil || stats.Total != concurrentCreates+1 || stats.Active != 1 || stats.TotalContractValueEUR != 125000 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}

	roleConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	roleConfig.AfterConnect = func(connectCtx context.Context, roleConn *pgx.Conn) error {
		_, roleErr := roleConn.Exec(connectCtx, "SET ROLE "+quotedRole)
		return roleErr
	}
	rolePool, err := pgxpool.NewWithConfig(ctx, roleConfig)
	if err != nil {
		t.Fatal(err)
	}
	bus := service.NewEventBus()
	scheduled := bus.Subscribe("*")
	scheduler := workerpkg.NewRegulatoryScheduler(rolePool, bus)
	if err = scheduler.CheckVendorAssessments(ctx); err != nil {
		rolePool.Close()
		t.Fatal(err)
	}
	select {
	case event := <-scheduled:
		if event.OrgID != orgA || event.EntityID != vendorA.ID || event.EntityRef != vendorA.VendorRef {
			t.Fatalf("scheduler event=%#v", event)
		}
	default:
		t.Fatal("vendor scheduler did not publish due assessment")
	}
	bus.Close()
	rolePool.Close()

	events, eventTotal, err := vendorService.ListEvents(tenantA, orgA, vendorA.ID, models.PaginationRequest{Page: 1, PageSize: 100})
	if err != nil || eventTotal != 7 || len(events) != 7 {
		t.Fatalf("events=%d/%d err=%v", len(events), eventTotal, err)
	}
	if tag, err := conn.Exec(ctx, `UPDATE vendor_events SET summary='tampered' WHERE id=$1`, events[0].ID); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("timeline update affected=%d err=%v", tag.RowsAffected(), err)
	}
	if tag, err := conn.Exec(ctx, `DELETE FROM vendor_events WHERE id=$1`, events[0].ID); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("timeline delete affected=%d err=%v", tag.RowsAffected(), err)
	}

	tenantB := setTenant(orgB)
	vendorB, err := vendorService.Create(tenantB, orgB, userB, models.VendorCreateInput{Name: "Tenant B supplier"})
	if err != nil || vendorB.VendorRef != "VND-000001" {
		t.Fatalf("vendor B=%#v err=%v", vendorB, err)
	}
	if _, err = vendorService.GetByID(tenantB, orgB, vendorA.ID); !errors.Is(err, service.ErrVendorNotFound) {
		t.Fatalf("cross-tenant read error=%v", err)
	}
	name := "Cross-tenant overwrite"
	if _, err = vendorService.Update(tenantB, orgB, userB, vendorA.ID, models.VendorPatch{Version: vendorA.Version, Name: &name}); !errors.Is(err, service.ErrVendorNotFound) {
		t.Fatalf("cross-tenant update error=%v", err)
	}
	if _, _, err = vendorService.SaveContact(tenantB, orgB, userB, vendorB.ID, "", models.VendorContactInput{Version: vendorB.Version, Name: "Foreign", Email: "foreign@example.test", ContactType: "business"}); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM vendors`).Scan(&visible); err != nil || visible != 1 {
		t.Fatalf("tenant B visible=%d err=%v", visible, err)
	}

	if _, err = conn.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	_, crossAssetErr := conn.Exec(ctx, `INSERT INTO assets(organization_id,asset_ref,name,asset_type,criticality,classification,status,linked_vendor_id,created_by)VALUES($1,'AST-VENDOR-FK','Foreign vendor asset','service','medium','internal','active',$2,$3)`, orgB, vendorA.ID, userB)
	if crossAssetErr == nil {
		t.Fatal("cross-tenant asset-to-vendor link succeeded")
	}
	questionnaireID := uuid.NewString()
	if _, err = conn.Exec(ctx, `INSERT INTO assessment_questionnaires(id,name,questionnaire_type,status,is_system)VALUES($1,'FK test','security','active',true)`, questionnaireID); err != nil {
		t.Fatal(err)
	}
	_, crossAssessmentErr := conn.Exec(ctx, `INSERT INTO vendor_assessments(organization_id,vendor_id,questionnaire_id,assessment_ref)VALUES($1,$2,$3,'VAS-CROSS')`, orgB, vendorA.ID, questionnaireID)
	if crossAssessmentErr == nil {
		t.Fatal("cross-tenant assessment-to-vendor link succeeded")
	}
	_, _ = conn.Exec(ctx, `DELETE FROM assessment_questionnaires WHERE id=$1`, questionnaireID)
	if _, err = conn.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
		t.Fatal(err)
	}
	tenantA = setTenant(orgA)

	contractInput.Version = vendorA.Version
	contractInput.Status = "terminated"
	contract, vendorA, err = vendorService.SaveContract(tenantA, orgA, userA, vendorA.ID, contract.ID, contractInput)
	if err != nil || contract.Status != "terminated" {
		t.Fatalf("terminated contract=%#v err=%v", contract, err)
	}
	vendorA, err = vendorService.Transition(tenantA, orgA, userA, vendorA.ID, models.VendorTransitionInput{Status: models.VendorStatusOffboarding, Version: vendorA.Version, Reason: "Service migrated to replacement provider"})
	if err != nil {
		t.Fatal(err)
	}
	vendorA, err = vendorService.Transition(tenantA, orgA, userA, vendorA.ID, models.VendorTransitionInput{Status: models.VendorStatusOffboarded, Version: vendorA.Version, Reason: "Access revoked and data return verified"})
	if err != nil {
		t.Fatal(err)
	}
	if err = vendorService.Delete(tenantA, orgA, userA, vendorA.ID, vendorA.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = vendorService.GetByID(tenantA, orgA, vendorA.ID); !errors.Is(err, service.ErrVendorNotFound) {
		t.Fatalf("soft delete read error=%v", err)
	}
	var outboxEvents int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM queue_outbox WHERE tenant_id=$1`, orgA).Scan(&outboxEvents); err != nil || outboxEvents != concurrentCreates+11 {
		t.Fatalf("outbox count=%d err=%v", outboxEvents, err)
	}
}

func float64Pointer(value float64) *float64 { return &value }
