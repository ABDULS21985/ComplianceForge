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

// TestPolicyRepositoryWithNonSuperuserTenants exercises policy authoring,
// immutable versions, approval, publication, review, attestation, exception,
// soft deletion, and cross-tenant denial through a genuine forced-RLS role.
// TEST_DATABASE_URL must point to a fully migrated database with CREATE ROLE.
func TestPolicyRepositoryWithNonSuperuserTenants(t *testing.T) {
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
	categoryA, categoryB := uuid.NewString(), uuid.NewString()
	riskA := uuid.NewString()
	frameworkA, controlA := uuid.NewString(), uuid.NewString()
	roleName := "grc_policy_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()

	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Policy RLS A',$3,'active','starter'),($2,'Policy RLS B',$4,'active','starter')`,
		orgA, orgB, "policy-a-"+orgA, "policy-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,organization_id,email,status) VALUES
		($1,$2,$3,'active'),($4,$5,$6,'active')`,
		userA, orgA, userA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO policy_categories (id,organization_id,name,code) VALUES
		($1,$2,'Tenant A','TENANT_A'),($3,$4,'Tenant B','TENANT_B')`, categoryA, orgA, categoryB, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO risks (id,organization_id,risk_ref,title,status) VALUES
		($1,$2,'RSK-POLICY','Policy integration risk','identified')`, riskA, orgA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO compliance_frameworks
		(id,organization_id,code,name,version,category) VALUES ($1,$2,'POLICY_TEST','Policy Test','1','security')`, frameworkA, orgA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO framework_controls (id,framework_id,code,title) VALUES
		($1,$2,'P.1','Policy integration control')`, controlA, frameworkA); err != nil {
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
		"; GRANT SELECT ON users,policy_categories,compliance_frameworks,framework_controls,risks TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON policies,policy_versions,policy_approval_workflows,policy_approval_steps,policy_reviews,policy_attestations,policy_exceptions TO " + quotedRole
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

	repo := repository.NewPolicyRepository(pool)
	tenantA := setTenant(orgA)
	all, mandatory, attest := true, true, true
	v1Text := "All employees must protect confidential information."
	policy, err := repo.Create(tenantA, orgA, userA, models.PolicyCreateInput{
		Title: "Information security policy", CategoryID: &categoryA, OwnerUserID: &userA,
		Classification: "internal", ReviewFrequencyMonths: 12, AppliesToAll: &all,
		ApplicableDepartments: []string{}, ApplicableRoles: []string{}, ApplicableLocations: []string{},
		LinkedFrameworkIDs: []string{frameworkA}, LinkedControlIDs: []string{controlA}, LinkedRiskIDs: []string{riskA},
		Tags: []string{"security"}, IsMandatory: &mandatory, RequiresAttestation: &attest,
		AttestationFrequencyMonths: 12, Metadata: []byte(`{"owner":"security"}`),
		InitialVersion: models.PolicyVersionInput{ContentText: &v1Text, Language: "en", Metadata: []byte(`{}`)},
	}, 6)
	if err != nil {
		t.Fatalf("creating policy as non-superuser: %v", err)
	}
	if policy.OrganizationID != orgA || policy.PolicyRef == "" || policy.CurrentVersionID == nil || policy.CurrentVersionRecord == nil {
		t.Fatalf("created policy=%#v", policy)
	}
	if categories, err := repo.ListCategories(tenantA, orgA); err != nil {
		t.Fatal(err)
	} else {
		for _, category := range categories {
			if category.ID == categoryB {
				t.Fatal("cross-tenant policy category was visible")
			}
		}
	}

	workflow, err := repo.CreateApprovalWorkflow(tenantA, orgA, policy.ID, userA, models.PolicySubmitInput{
		WorkflowType: "new_policy", Approvers: []models.PolicyApprovalStepInput{{ApproverUserID: &userA}},
	})
	if err != nil || workflow.Status != "in_progress" || len(workflow.Steps) != 1 {
		t.Fatalf("workflow=%#v err=%v", workflow, err)
	}
	workflow, err = repo.DecideApproval(tenantA, orgA, policy.ID, userA, "compliance_manager", models.PolicyApprovalDecisionInput{Decision: "approved"})
	if err != nil || workflow.Status != "approved" || workflow.Steps[0].Status != "approved" {
		t.Fatalf("approved workflow=%#v err=%v", workflow, err)
	}
	policy, err = repo.Publish(tenantA, orgA, policy.ID, userA)
	if err != nil || policy.Status != models.PolicyStatePublished || policy.EffectiveDate == nil || policy.NextReviewDate == nil || policy.CurrentVersionRecord.Status != "published" {
		t.Fatalf("published policy=%#v err=%v", policy, err)
	}

	attestation, err := repo.Acknowledge(tenantA, orgA, policy.ID, userA, "192.0.2.20", models.PolicyAttestationInput{
		Decision: "attest", AttestationMethod: "digital_click", Metadata: []byte(`{}`),
	})
	if err != nil || attestation.Status != "attested" || attestation.PolicyVersionID == nil {
		t.Fatalf("attestation=%#v err=%v", attestation, err)
	}
	review, err := repo.CreateReview(tenantA, orgA, policy.ID, models.PolicyReviewInput{ReviewType: "scheduled", ReviewerUserID: &userA})
	if err != nil {
		t.Fatalf("creating review: %v", err)
	}
	review.Status = "completed"
	outcome := "no_change"
	review.Outcome = &outcome
	review, err = repo.UpdateReview(tenantA, orgA, policy.ID, review)
	if err != nil || review.CompletedDate == nil {
		t.Fatalf("completed review=%#v err=%v", review, err)
	}
	expiry := time.Now().UTC().AddDate(0, 2, 0)
	exception, err := repo.CreateException(tenantA, orgA, policy.ID, userA, models.PolicyExceptionInput{
		Title: "Temporary legacy exception", Justification: "Legacy system replacement is underway", RiskLevel: stringPointer("medium"), ExpiryDate: &expiry,
	})
	if err != nil || exception.ExceptionRef == "" || exception.Status != "requested" {
		t.Fatalf("exception=%#v err=%v", exception, err)
	}
	exception, err = repo.UpdateExceptionDecision(tenantA, orgA, policy.ID, exception.ID, userA, models.PolicyExceptionDecisionInput{Decision: "approved", ExpiryDate: &expiry})
	if err != nil || exception.Status != "approved" || exception.ApprovedBy == nil {
		t.Fatalf("approved exception=%#v err=%v", exception, err)
	}

	v2Text := "All employees and contractors must protect confidential information."
	v2, err := repo.CreateVersion(tenantA, orgA, policy.ID, userA, models.PolicyVersionInput{
		ContentText: &v2Text, ChangeType: stringPointer("minor"), Language: "en", Metadata: []byte(`{}`),
	}, 8)
	if err != nil || v2.VersionNumber != 2 || v2.Status != "draft" {
		t.Fatalf("version two=%#v err=%v", v2, err)
	}
	if _, err := repo.CreateApprovalWorkflow(tenantA, orgA, policy.ID, userA, models.PolicySubmitInput{
		WorkflowType: "amendment", Approvers: []models.PolicyApprovalStepInput{{ApproverRole: stringPointer("compliance_manager")}},
	}); err != nil {
		t.Fatalf("creating amendment workflow: %v", err)
	}
	if _, err := repo.DecideApproval(tenantA, orgA, policy.ID, userA, "compliance_manager", models.PolicyApprovalDecisionInput{Decision: "approved"}); err != nil {
		t.Fatalf("approving amendment: %v", err)
	}
	policy, err = repo.Publish(tenantA, orgA, policy.ID, userA)
	if err != nil || policy.CurrentVersion != 2 || policy.CurrentVersionRecord.ID != v2.ID {
		t.Fatalf("republished policy=%#v err=%v", policy, err)
	}
	versions, versionTotal, err := repo.ListVersions(tenantA, orgA, policy.ID, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || versionTotal != 2 || len(versions) != 2 || versions[0].Status != "published" || versions[1].Status != "archived" {
		t.Fatalf("versions=%#v total=%d err=%v", versions, versionTotal, err)
	}
	if _, err := repo.CreateApprovalWorkflow(tenantA, orgA, policy.ID, userA, models.PolicySubmitInput{
		WorkflowType: "retirement", Approvers: []models.PolicyApprovalStepInput{{ApproverUserID: &userA}},
	}); err != nil {
		t.Fatalf("creating retirement workflow: %v", err)
	}
	if _, err := repo.DecideApproval(tenantA, orgA, policy.ID, userA, "compliance_manager", models.PolicyApprovalDecisionInput{Decision: "approved"}); err != nil {
		t.Fatalf("approving retirement: %v", err)
	}
	policy, err = repo.GetByID(tenantA, orgA, policy.ID)
	if err != nil || policy.Status != models.PolicyStateRetired || policy.CurrentVersionRecord.Status != "archived" {
		t.Fatalf("retired policy=%#v err=%v", policy, err)
	}

	tenantB := setTenant(orgB)
	if _, err := repo.GetByID(tenantB, orgB, policy.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant policy read error=%v", err)
	}
	if _, err := repo.GetVersion(tenantB, orgB, policy.ID, v2.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant version read error=%v", err)
	}
	if err := repo.Delete(tenantB, orgB, policy.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant delete error=%v", err)
	}
	if _, err := repo.Create(tenantB, orgB, userB, models.PolicyCreateInput{
		Title: "Cross-tenant reference", OwnerUserID: &userA, Classification: "internal",
		ReviewFrequencyMonths: 12, AppliesToAll: &all, ApplicableDepartments: []string{},
		ApplicableRoles: []string{}, ApplicableLocations: []string{}, LinkedFrameworkIDs: []string{},
		LinkedControlIDs: []string{}, LinkedRiskIDs: []string{}, Tags: []string{}, IsMandatory: &mandatory,
		RequiresAttestation: &attest, AttestationFrequencyMonths: 12, Metadata: []byte(`{}`),
		InitialVersion: models.PolicyVersionInput{ContentText: &v1Text, Language: "en", Metadata: []byte(`{}`)},
	}, 6); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant owner reference error=%v", err)
	}

	tenantA = setTenant(orgA)
	if err := repo.Delete(tenantA, orgA, policy.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := repo.GetByID(tenantA, orgA, policy.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("soft-deleted policy read error=%v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant','',false)`); err != nil {
		t.Fatal(fmt.Errorf("clearing tenant: %w", err))
	}
	if _, err := conn.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
}

func stringPointer(value string) *string { return &value }
