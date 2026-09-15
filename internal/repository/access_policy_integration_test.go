package repository_test

import (
	"context"
	"encoding/json"
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

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

type allowingPolicyRBAC struct{}

func (allowingPolicyRBAC) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Reason: "test RBAC permission", ReasonCode: "rbac_allowed"}, nil
}

// TestPolicyAccessWithNonSuperuserTenants proves the migration-054 persistence
// contract through a genuine NOSUPERUSER/NOBYPASSRLS role. It covers tenant
// isolation, composed RBAC/ABAC evaluation, sponsored object access, immutable
// evidence, field controls, certification, and optimistic concurrency.
func TestPolicyAccessWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer adminPool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	adminA, auditorA, targetA, adminB, targetB := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	auditorRole := uuid.NewString()
	databaseRole := "grc_policy_access_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{databaseRole}.Sanitize()
	if _, err = adminPool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Policy Tenant A',$3,'active','unlimited'),($2,'Policy Tenant B',$4,'active','unlimited')`,
		orgA, orgB, "policy-a-"+orgA, "policy-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO users(
		id,organization_id,email,first_name,last_name,department,location,status,invitation_status,email_verified_at
	) VALUES
		($1,$2,$3,'Admin','A','Security','Lagos','active','accepted',NOW()),
		($4,$2,$5,'External','Auditor','Assurance','London','active','accepted',NOW()),
		($6,$2,$7,'Target','A','Finance','Lagos','active','accepted',NOW()),
		($8,$9,$10,'Admin','B','Security','Berlin','active','accepted',NOW()),
		($11,$9,$12,'Target','B','Finance','Berlin','active','accepted',NOW())`,
		adminA, orgA, adminA+"@example.test", auditorA, auditorA+"@example.test",
		targetA, targetA+"@example.test", adminB, orgB, adminB+"@example.test",
		targetB, targetB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO roles(
		id,organization_id,name,slug,description,is_system_role,is_custom,created_by,updated_by
	) VALUES($1,$2,'External auditor','external_auditor','Scoped assurance access',false,true,$3,$3)`,
		auditorRole, orgA, adminA); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by)
		VALUES($1,$2,$3,$4)`, auditorA, auditorRole, orgA, adminA); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quotedRole+
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+quotedRole+
		"; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO "+quotedRole); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		conn, acquireErr := adminPool.Acquire(cleanupCtx)
		if acquireErr == nil {
			_, _ = conn.Exec(cleanupCtx, `SET session_replication_role='replica'`)
			_, _ = conn.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
			_, _ = conn.Exec(cleanupCtx, `SET session_replication_role='origin'`)
			conn.Release()
		}
		_, _ = adminPool.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole)
		_, _ = adminPool.Exec(cleanupCtx, "DROP ROLE "+quotedRole)
	}()

	roleConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	roleConfig.MaxConns = 8
	roleConfig.AfterConnect = func(connectCtx context.Context, connection *pgx.Conn) error {
		_, connectErr := connection.Exec(connectCtx, "SET ROLE "+quotedRole)
		return connectErr
	}
	rolePool, err := pgxpool.NewWithConfig(ctx, roleConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer rolePool.Close()

	store, err := repository.NewPolicyAccessRepository(rolePool)
	if err != nil {
		t.Fatal(err)
	}
	policyService, err := service.NewPolicyAccessService(store, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	authorizer, err := service.NewPolicyBasedAuthorizer(allowingPolicyRBAC{}, store, nil)
	if err != nil {
		t.Fatal(err)
	}

	requestID := uuid.NewString()
	var policy *models.AccessPolicy
	withTenant(t, ctx, rolePool, orgA, func(tenantCtx context.Context) {
		policy, err = policyService.CreatePolicy(tenantCtx, orgA, adminA, requestID, models.AccessPolicyInput{
			Name: "Sponsored external assurance", Priority: 20, Effect: models.AccessPolicyEffectAllow, IsActive: true,
			SubjectConditions: []models.AccessCondition{{
				Attribute: "roles", Operator: models.AccessOperatorContainsAny, Value: json.RawMessage(`["external_auditor"]`),
			}},
			ResourceType: "users", ResourceConditions: []models.AccessCondition{{
				Attribute: "id", Operator: models.AccessOperatorEqualsSubject, Value: json.RawMessage(`"id"`),
			}},
			Actions: []string{"read"}, EnvironmentConditions: []models.AccessCondition{{
				Attribute: "mfa_verified", Operator: models.AccessOperatorEquals, Value: json.RawMessage(`true`),
			}}, Reason: "Require scoped sponsored assurance access",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = policyService.CreateAssignment(tenantCtx, orgA, policy.ID, adminA, uuid.NewString(), models.AccessPolicyAssignmentInput{
			AssigneeType: models.AccessAssigneeAllUsers, Reason: "Apply the constraint to tenant users",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = policyService.UpsertFieldPermission(tenantCtx, orgA, policy.ID, adminA, uuid.NewString(), models.AccessFieldPermissionInput{
			ResourceType: "users", FieldPath: "email", Classification: models.AccessFieldPersonal,
			Visibility: models.AccessFieldHidden, Reason: "Hide personal email from sponsored reviewers",
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	accessRequest := authz.Request{
		SubjectID: auditorA, OrganizationID: orgA, Resource: "users", ResourceID: targetA,
		Action: "read", IPAddress: "192.0.2.10", MFAVerified: true,
	}
	withTenant(t, ctx, rolePool, orgA, func(tenantCtx context.Context) {
		decision, authorizeErr := authorizer.Authorize(tenantCtx, accessRequest)
		if authorizeErr != nil || decision.Allowed || decision.ReasonCode != "external_auditor_grant_required" {
			t.Fatalf("decision without grant=%#v err=%v", decision, authorizeErr)
		}
	})

	var grant *models.AccessObjectGrant
	withTenant(t, ctx, rolePool, orgA, func(tenantCtx context.Context) {
		grant, err = policyService.CreateObjectGrant(tenantCtx, orgA, adminA, uuid.NewString(), models.AccessObjectGrantInput{
			SubjectID: auditorA, ResourceType: "users", ResourceID: targetA, Actions: []string{"read"},
			ValidFrom: time.Now().UTC().Add(-time.Minute), ValidUntil: time.Now().UTC().Add(time.Hour),
			Reason: "Sponsor external auditor for this review object",
		})
		if err != nil || grant.Status != models.AccessObjectGrantPending {
			t.Fatalf("created grant=%#v err=%v", grant, err)
		}
		grant, err = policyService.DecideObjectGrant(tenantCtx, orgA, grant.ID, adminA, uuid.NewString(), models.AccessObjectGrantDecisionInput{
			Decision: "approve", Reason: "Approved for the scheduled assurance review", ExpectedVersion: grant.Version,
		})
		if err != nil || grant.Status != models.AccessObjectGrantApproved {
			t.Fatalf("approved grant=%#v err=%v", grant, err)
		}

		decision, authorizeErr := authorizer.Authorize(tenantCtx, accessRequest)
		if authorizeErr != nil || !decision.Allowed || !decision.ConstraintsApplied || decision.DecisionID == "" {
			t.Fatalf("decision with grant=%#v err=%v", decision, authorizeErr)
		}
		bundle, loadErr := store.LoadEvaluationBundle(tenantCtx, orgA, auditorA, "users", targetA, "read", time.Now().UTC())
		if loadErr != nil || len(bundle.FieldPermissions) != 1 || bundle.FieldPermissions[0].FieldPath != "email" {
			t.Fatalf("evaluation bundle=%#v err=%v", bundle, loadErr)
		}
		evidence, total, listErr := policyService.ListDecisionEvidence(tenantCtx, orgA, models.AccessDecisionEvidenceFilter{
			PaginationRequest: models.PaginationRequest{Page: 1, PageSize: 20}, SubjectID: auditorA,
		})
		if listErr != nil || total != 2 || len(evidence) != 2 {
			t.Fatalf("evidence=%#v total=%d err=%v", evidence, total, listErr)
		}
		if _, immutableErr := database.QuerierFromContext(tenantCtx, rolePool).Exec(tenantCtx,
			`UPDATE access_audit_log SET reason_code='tampered' WHERE organization_id=$1::uuid`, orgA); immutableErr == nil {
			t.Fatal("access decision evidence update unexpectedly succeeded")
		}
	})

	// A tenant-B connection cannot observe tenant-A records even when the caller
	// supplies tenant A explicitly to the repository method.
	withTenant(t, ctx, rolePool, orgB, func(tenantCtx context.Context) {
		if _, getErr := policyService.GetPolicy(tenantCtx, orgA, policy.ID); !errors.Is(getErr, service.ErrAccessPolicyNotFound) {
			t.Fatalf("cross-tenant GetPolicy error=%v", getErr)
		}
		var visible int
		if queryErr := database.QuerierFromContext(tenantCtx, rolePool).QueryRow(tenantCtx,
			`SELECT COUNT(*) FROM access_policies WHERE organization_id=$1::uuid`, orgA).Scan(&visible); queryErr != nil || visible != 0 {
			t.Fatalf("cross-tenant visible policies=%d err=%v", visible, queryErr)
		}
	})

	// The row lock plus expected version permits exactly one concurrent change.
	expectedVersion := policy.Version
	var successes, conflicts int
	var mutex sync.Mutex
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			concurrentErr := database.WithTenantConnection(ctx, rolePool, orgA, func(tenantCtx context.Context) error {
				_, updateErr := policyService.UpdatePolicy(tenantCtx, orgA, policy.ID, adminA, uuid.NewString(), models.AccessPolicyInput{
					Name: "Sponsored external assurance " + string(rune('A'+index)), Priority: 20,
					Effect: models.AccessPolicyEffectAllow, IsActive: true,
					SubjectConditions: policy.SubjectConditions, ResourceType: policy.ResourceType,
					ResourceConditions: policy.ResourceConditions, Actions: policy.Actions,
					EnvironmentConditions: policy.EnvironmentConditions, ExpectedVersion: &expectedVersion,
					Reason: "Concurrent policy update must serialize",
				})
				return updateErr
			})
			mutex.Lock()
			defer mutex.Unlock()
			switch {
			case concurrentErr == nil:
				successes++
			case errors.Is(concurrentErr, service.ErrAccessPolicyConflict):
				conflicts++
			default:
				t.Errorf("concurrent update error=%v", concurrentErr)
			}
		}(index)
	}
	wait.Wait()
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent updates successes=%d conflicts=%d", successes, conflicts)
	}

	withTenant(t, ctx, rolePool, orgA, func(tenantCtx context.Context) {
		current, getErr := policyService.GetPolicy(tenantCtx, orgA, policy.ID)
		if getErr != nil || current.Version != expectedVersion+1 {
			t.Fatalf("current policy=%#v err=%v", current, getErr)
		}
		certification, certifyErr := policyService.CertifyPolicy(tenantCtx, orgA, policy.ID, adminA, uuid.NewString(), models.AccessPolicyCertificationInput{
			Decision: "certified", Reason: "Quarterly access policy review completed", ExpectedVersion: current.Version,
		})
		if certifyErr != nil || len(certification.SnapshotSHA256) != 64 {
			t.Fatalf("certification=%#v err=%v", certification, certifyErr)
		}
		tag, immutableErr := database.QuerierFromContext(tenantCtx, rolePool).Exec(tenantCtx,
			`DELETE FROM access_policy_certifications WHERE organization_id=$1::uuid`, orgA)
		if immutableErr != nil || tag.RowsAffected() != 0 {
			t.Fatalf("policy certification delete affected=%d err=%v", tag.RowsAffected(), immutableErr)
		}
		var certificationCount int
		if countErr := database.QuerierFromContext(tenantCtx, rolePool).QueryRow(tenantCtx,
			`SELECT COUNT(*) FROM access_policy_certifications WHERE organization_id=$1::uuid`, orgA).
			Scan(&certificationCount); countErr != nil || certificationCount != 1 {
			t.Fatalf("immutable policy certification count=%d err=%v", certificationCount, countErr)
		}
	})
}

func withTenant(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	organizationID string,
	fn func(context.Context),
) {
	t.Helper()
	if err := database.WithTenantConnection(ctx, pool, organizationID, func(tenantCtx context.Context) error {
		fn(tenantCtx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
