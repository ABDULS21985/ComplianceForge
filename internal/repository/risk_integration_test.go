package repository_test

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
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

// TestRiskRepositoryWithNonSuperuserTenants exercises the complete risk slice
// through a genuine NOSUPERUSER/NOBYPASSRLS role. TEST_DATABASE_URL must point
// at a fully migrated database and have CREATE ROLE authority.
func TestRiskRepositoryWithNonSuperuserTenants(t *testing.T) {
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
	categoryA, categoryB := uuid.NewString(), uuid.NewString()
	roleName := "grc_risk_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()

	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Risk RLS A',$3,'active','starter'),($2,'Risk RLS B',$4,'active','starter')`,
		orgA, orgB, "risk-a-"+orgA, "risk-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,organization_id,email,status) VALUES
		($1,$2,$3,'active'),($4,$5,$6,'active')`, userA, orgA, userA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO risk_categories (id,organization_id,name,code) VALUES
		($1,$2,'Tenant A','TENANT_A'),($3,$4,'Tenant B','TENANT_B')`, categoryA, orgA, categoryB, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role: %v", err)
	}
	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole + "; GRANT SELECT ON users,risk_categories,risk_matrices TO " + quotedRole + "; GRANT SELECT,INSERT,UPDATE,DELETE ON risks,risk_assessments,risk_treatments,risk_appetite_statements,risk_indicators,risk_indicator_values TO " + quotedRole
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

	repo := repository.NewRiskRepository(pool)
	tenantA := setTenant(orgA)
	likelihood, impact := 4, 5
	risk, err := repo.Create(tenantA, orgA, models.RiskCreateInput{
		Title: "Tenant A availability", RiskCategoryID: &categoryA, OwnerUserID: &userA,
		InherentLikelihood: &likelihood, InherentImpact: &impact,
		ImpactCategories: []byte(`{}`), Attachments: []byte(`[]`), Metadata: []byte(`{}`),
		LinkedRegulations: []string{}, LinkedControlIDs: []string{}, Tags: []string{},
	})
	if err != nil {
		t.Fatalf("creating risk as non-superuser: %v", err)
	}
	if risk.OrganizationID != orgA || risk.RiskRef == "" || risk.InherentRiskScore == nil || *risk.InherentRiskScore != 20 || risk.InherentRiskLevel == nil || *risk.InherentRiskLevel != "critical" {
		t.Fatalf("created risk=%#v", risk)
	}
	items, total, err := repo.List(tenantA, orgA, models.RiskListFilter{PaginationRequest: models.PaginationRequest{Page: 1, PageSize: 20}})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("listing own risks total=%d len=%d err=%v", total, len(items), err)
	}
	categories, err := repo.ListCategories(tenantA, orgA)
	if err != nil {
		t.Fatalf("listing risk categories: %v", err)
	}
	for _, category := range categories {
		if category.ID == categoryB {
			t.Fatal("cross-tenant risk category was visible")
		}
	}

	assessmentLikelihood, assessmentImpact := 3, 4
	assessmentScore, assessmentLevel := 12.0, "high"
	assessment, err := repo.CreateAssessment(tenantA, orgA, risk.ID, userA, models.RiskAssessmentInput{
		AssessmentType: "periodic", LikelihoodAfter: &assessmentLikelihood, ImpactAfter: &assessmentImpact,
		DataSources: []string{"incident register"},
	}, nil, nil, &assessmentScore, &assessmentLevel)
	if err != nil || assessment.RiskID != risk.ID || assessment.ScoreAfter == nil || *assessment.ScoreAfter != 12 {
		t.Fatalf("assessment=%#v err=%v", assessment, err)
	}
	assessedRisk, err := repo.GetByID(tenantA, orgA, risk.ID)
	if err != nil || assessedRisk.Status != models.RiskStatusAssessed || assessedRisk.ResidualRiskScore == nil || *assessedRisk.ResidualRiskScore != 12 {
		t.Fatalf("assessment did not update register risk=%#v err=%v", assessedRisk, err)
	}
	treatment, err := repo.CreateTreatment(tenantA, orgA, risk.ID, models.RiskTreatmentInput{
		TreatmentType: "mitigate", Title: "Add redundancy", OwnerUserID: &userA, LinkedControlIDs: []string{},
	})
	if err != nil || treatment.RiskID != risk.ID {
		t.Fatalf("treatment=%#v err=%v", treatment, err)
	}
	indicator, err := repo.CreateIndicator(tenantA, orgA, risk.ID, models.RiskIndicatorInput{
		Name: "Downtime", MetricType: "duration", CollectionFrequency: "monthly",
		OwnerUserID: &userA, AutomationConfig: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("indicator=%#v err=%v", indicator, err)
	}
	value, err := repo.RecordIndicatorValue(tenantA, orgA, risk.ID, indicator.ID, userA, models.RiskIndicatorValueInput{Value: 2})
	if err != nil || value.IndicatorID != indicator.ID {
		t.Fatalf("indicator value=%#v err=%v", value, err)
	}
	indicators, err := repo.ListIndicators(tenantA, orgA, risk.ID)
	if err != nil || len(indicators) != 1 || indicators[0].CurrentValue == nil || *indicators[0].CurrentValue != 2 {
		t.Fatalf("indicator current value was not updated: items=%#v err=%v", indicators, err)
	}
	appetite, err := repo.UpsertAppetite(tenantA, orgA, categoryA, userA, models.RiskAppetiteInput{
		AppetiteLevel: "cautious", ToleranceLevel: "low", Status: "approved",
	})
	if err != nil || appetite.OrganizationID != orgA || appetite.ApprovedBy == nil || *appetite.ApprovedBy != userA {
		t.Fatalf("appetite=%#v err=%v", appetite, err)
	}

	tenantB := setTenant(orgB)
	if _, err := repo.GetByID(tenantB, orgB, risk.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant read error=%v", err)
	}
	if err := repo.Delete(tenantB, orgB, risk.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant delete error=%v", err)
	}
	if _, err := repo.Create(tenantB, orgB, models.RiskCreateInput{
		Title: "Cross-tenant owner", OwnerUserID: &userA,
		ImpactCategories: []byte(`{}`), Attachments: []byte(`[]`), Metadata: []byte(`{}`),
		LinkedRegulations: []string{}, LinkedControlIDs: []string{}, Tags: []string{},
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant owner reference error=%v", err)
	}

	tenantA = setTenant(orgA)
	if err := repo.Delete(tenantA, orgA, risk.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := repo.GetByID(tenantA, orgA, risk.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted risk read error=%v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant','',false)`); err != nil {
		t.Fatal(fmt.Errorf("clearing tenant: %w", err))
	}
	if _, err := conn.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
}
