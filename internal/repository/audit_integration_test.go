package repository_test

import (
	"context"
	"encoding/json"
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
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

// TestAuditRepositoryWithNonSuperuserTenants exercises audit planning,
// findings, lifecycle persistence, reference allocation, and cross-tenant
// denial through a genuine NOSUPERUSER/NOBYPASSRLS role.
func TestAuditRepositoryWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB := uuid.NewString(), uuid.NewString()
	frameworkA, frameworkB := uuid.NewString(), uuid.NewString()
	controlA := uuid.NewString()
	roleName := "grc_audit_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()

	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Audit RLS A',$3,'active','starter'),($2,'Audit RLS B',$4,'active','starter')`,
		orgA, orgB, "audit-a-"+orgA, "audit-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Auditor','active'),($4,$5,$6,'Bob','Auditor','active')`,
		userA, orgA, userA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO compliance_frameworks
		(id,organization_id,code,name,version,category) VALUES
		($1,$2,'AUD_A','Audit A','1','security'),($3,$4,'AUD_B','Audit B','1','security')`,
		frameworkA, orgA, frameworkB, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO framework_controls
		(id,framework_id,code,title) VALUES ($1,$2,'A.1','Audit control')`, controlA, frameworkA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role: %v", err)
	}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}
	defer cleanup()
	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT ON users,compliance_frameworks,framework_controls TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON audit_reference_sequences,audits,audit_findings TO " + quotedRole
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

	repo := repository.NewAuditRepository(pool)
	start := time.Now().UTC().AddDate(0, 0, 7)
	end := start.AddDate(0, 0, 5)
	tenantA := setTenant(orgA)
	audit, err := repo.Create(tenantA, &models.Audit{
		TenantModel: models.TenantModel{OrganizationID: orgA}, Title: "Annual assurance review",
		Description: "Independent review", Type: models.AuditTypeInternal,
		Status: models.AuditStatusPlanned, LeadAuditorID: userA, Scope: "Production systems",
		ScheduledStartDate: &start, ScheduledEndDate: &end, FrameworkID: &frameworkA,
		CreatedBy: userA, Metadata: json.RawMessage(`{"source":"integration-test"}`),
	})
	if err != nil {
		t.Fatalf("creating audit as non-superuser: %v", err)
	}
	if audit.OrganizationID != orgA || audit.AuditRef != "AUD-0001" || audit.LeadAuditor == nil || audit.LeadAuditor.ID != userA || audit.Framework == nil || audit.Framework.ID != frameworkA {
		t.Fatalf("created audit=%#v", audit)
	}

	second, err := repo.Create(tenantA, &models.Audit{
		TenantModel: models.TenantModel{OrganizationID: orgA}, Title: "Supplier assurance",
		Description: "Second review", Type: models.AuditTypeExternal,
		Status: models.AuditStatusPlanned, LeadAuditorID: userA, Scope: "Critical supplier",
		ScheduledStartDate: &start, ScheduledEndDate: &end, CreatedBy: userA, Metadata: json.RawMessage(`{}`),
	})
	if err != nil || second.AuditRef != "AUD-0002" {
		t.Fatalf("second audit=%#v err=%v", second, err)
	}

	due := end.AddDate(0, 0, 30)
	finding, err := repo.CreateFinding(tenantA, &models.AuditFinding{
		TenantModel: models.TenantModel{OrganizationID: orgA}, AuditID: audit.ID,
		ControlID: &controlA, Title: "Missing quarterly review", Description: "Evidence was unavailable",
		Severity: "critical", Status: models.FindingStatusOpen, FindingType: "non_conformity",
		Recommendation: "Implement and evidence a quarterly review", ResponsibleUserID: userA,
		DueDate: &due, CreatedBy: userA, Metadata: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("creating finding: %v", err)
	}
	if finding.FindingRef != "FND-0001" || finding.ResponsibleUser == nil || finding.ResponsibleUser.ID != userA {
		t.Fatalf("created finding=%#v", finding)
	}
	stats, err := repo.FindingStats(tenantA, orgA, audit.ID)
	if err != nil || stats.Total != 1 || stats.Open != 1 || stats.CriticalOpen != 1 {
		t.Fatalf("finding stats=%#v err=%v", stats, err)
	}
	items, total, err := repo.List(tenantA, orgA, models.AuditListFilter{PaginationRequest: models.PaginationRequest{Page: 1, PageSize: 20}, Search: "annual"})
	if err != nil || total != 1 || len(items) != 1 || items[0].FindingsCount != 1 || items[0].CriticalOpen != 1 {
		t.Fatalf("audits=%#v total=%d err=%v", items, total, err)
	}

	now := time.Now().UTC()
	finding.Status = models.FindingStatusResolved
	finding.ResolvedAt = &now
	finding, err = repo.UpdateFinding(tenantA, orgA, audit.ID, finding)
	if err != nil || finding.Status != models.FindingStatusResolved || finding.ResolvedAt == nil {
		t.Fatalf("resolved finding=%#v err=%v", finding, err)
	}

	tenantB := setTenant(orgB)
	if _, err := repo.GetByID(tenantB, orgB, audit.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant audit read error=%v", err)
	}
	if _, err := repo.GetFindingByID(tenantB, orgB, audit.ID, finding.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant finding read error=%v", err)
	}
	if _, err := repo.Create(tenantB, &models.Audit{
		TenantModel: models.TenantModel{OrganizationID: orgB}, Title: "Cross-tenant lead",
		Description: "Must fail", Type: models.AuditTypeInternal, Status: models.AuditStatusPlanned,
		LeadAuditorID: userA, Scope: "Invalid", ScheduledStartDate: &start,
		ScheduledEndDate: &end, CreatedBy: userB, Metadata: json.RawMessage(`{}`),
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant lead reference error=%v", err)
	}

	tenantA = setTenant(orgA)
	if err := repo.Delete(tenantA, orgA, audit.ID); err != nil {
		t.Fatalf("soft-delete audit: %v", err)
	}
	if _, err := repo.GetByID(tenantA, orgA, audit.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted audit read error=%v", err)
	}
	if _, err := repo.GetFindingByID(tenantA, orgA, audit.ID, finding.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted finding read error=%v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant','',false)`); err != nil {
		t.Fatal(fmt.Errorf("clearing tenant: %w", err))
	}
	if _, err := conn.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
}
