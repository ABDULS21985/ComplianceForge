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

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

type failingManagedRoleOutbox struct{}

func (failingManagedRoleOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced managed-role outbox failure")
}

// TestAccessAdministrationWithNonSuperuserTenants exercises the administrator
// workflow using a genuine NOSUPERUSER/NOBYPASSRLS role. In addition to normal
// CRUD it proves permission validation, immutable system roles, tenant-safe
// assignments, lockout prevention, optimistic concurrency, append-only audit
// events, and atomic outbox rollback.
func TestAccessAdministrationWithNonSuperuserTenants(t *testing.T) {
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
	actorA, targetA, backupA, userB := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	systemRoleID := uuid.NewString()
	testResource := "access_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	roleName := "grc_access_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Access RLS A',$3,'active','starter'),($2,'Access RLS B',$4,'active','starter')`,
		orgA, orgB, "access-a-"+orgA, "access-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Admin','active'),($4,$2,$5,'Tara','Target','active'),
		($6,$2,$7,'Ben','Backup','active'),($8,$9,$10,'Bob','Other','active')`,
		actorA, orgA, actorA+"@example.test", targetA, targetA+"@example.test",
		backupA, backupA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO permissions(resource,action,description) VALUES
		($1,'read','Access integration read'),($1,'update','Access integration update')`, testResource); err != nil {
		t.Fatal(err)
	}
	var readPermissionID string
	if err := pool.QueryRow(ctx, `SELECT id FROM permissions WHERE resource=$1 AND action='read'`, testResource).Scan(&readPermissionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO roles(id,organization_id,name,slug,is_system_role,is_custom)
		VALUES ($1,NULL,$2,$3,true,false)`, systemRoleID, "Integration system role "+systemRoleID, "integration-system-"+systemRoleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_id) VALUES ($1,$2)`, systemRoleID, readPermissionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role: %v", err)
	}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM roles WHERE id=$1`, systemRoleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM permissions WHERE resource=$1`, testResource)
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}
	defer cleanup()
	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT ON organizations,users,permissions TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON roles,role_permissions,user_roles TO " + quotedRole +
		"; GRANT SELECT,INSERT ON role_change_events,queue_outbox TO " + quotedRole
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
	tenantA := setTenant(orgA)

	failedStore, err := repository.NewAccessAdministrationRepository(pool, failingManagedRoleOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	failedService := service.NewAccessAdministrationService(failedStore, zerolog.Nop())
	if _, err := failedService.CreateRole(tenantA, orgA, actorA, models.ManagedRoleCreateInput{
		Name: "Must roll back", Permissions: []models.PermissionGrant{{Resource: testResource, Action: "read"}},
	}); err == nil || !strings.Contains(err.Error(), "forced managed-role outbox failure") {
		t.Fatalf("forced outbox error=%v", err)
	}
	var rolledBack int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM roles WHERE organization_id=$1`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("role survived outbox failure count=%d err=%v", rolledBack, err)
	}

	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.NewAccessAdministrationRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	accessService := service.NewAccessAdministrationService(store, zerolog.Nop())
	role, err := accessService.CreateRole(tenantA, orgA, actorA, models.ManagedRoleCreateInput{
		Name: "Security Reviewer", Permissions: []models.PermissionGrant{{Resource: testResource, Action: "read"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if role.Slug != "security-reviewer" || role.Version != 1 || role.IsSystemRole || !role.IsCustom || len(role.Permissions) != 1 {
		t.Fatalf("created role=%#v", role)
	}
	if _, err := accessService.CreateRole(tenantA, orgA, actorA, models.ManagedRoleCreateInput{Name: "Duplicate", Slug: role.Slug}); !errors.Is(err, service.ErrManagedRoleConflict) {
		t.Fatalf("duplicate slug error=%v", err)
	}
	if _, err := accessService.CreateRole(tenantA, orgA, actorA, models.ManagedRoleCreateInput{
		Name: "Unknown permission", Permissions: []models.PermissionGrant{{Resource: testResource, Action: "approve"}},
	}); !errors.Is(err, service.ErrManagedRolePermission) {
		t.Fatalf("unknown permission error=%v", err)
	}

	roles, total, err := accessService.ListRoles(tenantA, orgA, models.ManagedRoleListFilter{
		PaginationRequest: models.PaginationRequest{Page: 1, PageSize: 50}, IncludeSystem: true, Search: "Integration system",
	})
	if err != nil || total != 1 || len(roles) != 1 || roles[0].ID != systemRoleID {
		t.Fatalf("system role list=%#v total=%d err=%v", roles, total, err)
	}
	impact, err := accessService.PreviewImpact(tenantA, orgA, role.ID, []models.PermissionGrant{
		{Resource: testResource, Action: "read"}, {Resource: testResource, Action: "update"},
	})
	if err != nil || impact.CurrentPermissions != 1 || impact.ProposedPermissions != 2 || len(impact.Added) != 1 || len(impact.Removed) != 0 {
		t.Fatalf("impact=%#v err=%v", impact, err)
	}
	permissions := []models.PermissionGrant{{Resource: testResource, Action: "read"}, {Resource: testResource, Action: "update"}}
	description := "May review and correct integration resources"
	role, err = accessService.UpdateRole(tenantA, orgA, role.ID, actorA, models.ManagedRolePatch{
		Description: &description, Permissions: &permissions, ExpectedVersion: role.Version,
	})
	if err != nil || role.Version != 2 || len(role.Permissions) != 2 {
		t.Fatalf("updated role=%#v err=%v", role, err)
	}
	if _, err := accessService.UpdateRole(tenantA, orgA, role.ID, actorA, models.ManagedRolePatch{
		Description: &description, ExpectedVersion: 1,
	}); !errors.Is(err, service.ErrManagedRoleConflict) {
		t.Fatalf("stale role update error=%v", err)
	}
	if _, err := accessService.UpdateRole(tenantA, orgA, systemRoleID, actorA, models.ManagedRolePatch{
		Description: &description, ExpectedVersion: 1,
	}); !errors.Is(err, service.ErrManagedRoleImmutable) {
		t.Fatalf("system update error=%v", err)
	}

	if err := accessService.AssignRole(tenantA, orgA, role.ID, actorA, models.ManagedRoleAssignmentInput{
		UserID: targetA, Reason: "Approved by quarterly access review",
	}); err != nil {
		t.Fatal(err)
	}
	if err := accessService.AssignRole(tenantA, orgA, role.ID, actorA, models.ManagedRoleAssignmentInput{
		UserID: targetA, Reason: "Duplicate assignment test",
	}); !errors.Is(err, service.ErrRoleAssignmentInvalid) {
		t.Fatalf("duplicate assignment error=%v", err)
	}
	if err := accessService.AssignRole(tenantA, orgA, role.ID, actorA, models.ManagedRoleAssignmentInput{
		UserID: userB, Reason: "Cross tenant assignment test",
	}); !errors.Is(err, service.ErrRoleAssignmentInvalid) {
		t.Fatalf("cross-tenant assignee error=%v", err)
	}
	assignments, err := accessService.ListAssignments(tenantA, orgA, role.ID)
	if err != nil || len(assignments) != 1 || assignments[0].UserID != targetA || assignments[0].AssignedBy == nil || *assignments[0].AssignedBy != actorA {
		t.Fatalf("assignments=%#v err=%v", assignments, err)
	}

	authorizer, err := service.NewRBACAuthorizer(pool)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := authorizer.Authorize(tenantA, authz.Request{
		SubjectID: targetA, OrganizationID: orgA, Resource: testResource, Action: "update",
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("effective custom role decision=%#v err=%v", decision, err)
	}
	if err := accessService.DeleteRole(tenantA, orgA, role.ID, actorA, role.Version); !errors.Is(err, service.ErrManagedRoleInUse) {
		t.Fatalf("assigned role deletion error=%v", err)
	}
	if err := accessService.UnassignRole(tenantA, orgA, role.ID, targetA, actorA, models.ManagedRoleUnassignmentInput{Reason: "Access no longer required"}); err != nil {
		t.Fatal(err)
	}
	if err := accessService.DeleteRole(tenantA, orgA, role.ID, actorA, role.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := accessService.GetRole(tenantA, orgA, role.ID); !errors.Is(err, service.ErrManagedRoleNotFound) {
		t.Fatalf("deleted role read error=%v", err)
	}
	roleEvents, roleEventTotal, err := accessService.ListEvents(tenantA, orgA, role.ID, models.PaginationRequest{Page: 1, PageSize: 50})
	if err != nil || roleEventTotal != 5 || len(roleEvents) != 5 || roleEvents[0].EventType != "deleted" {
		t.Fatalf("role events=%#v total=%d err=%v", roleEvents, roleEventTotal, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE role_change_events SET reason='tampered' WHERE role_id=$1`, role.ID); err == nil {
		t.Fatal("append-only role history unexpectedly allowed an update")
	}

	clone, err := accessService.CloneRole(tenantA, orgA, systemRoleID, actorA, models.ManagedRoleCloneInput{Name: "Tenant System Clone"})
	if err != nil || clone.IsSystemRole || !clone.IsCustom || len(clone.Permissions) != 1 || clone.Permissions[0].Resource != testResource {
		t.Fatalf("cloned role=%#v err=%v", clone, err)
	}
	if err := accessService.AssignRole(tenantA, orgA, systemRoleID, actorA, models.ManagedRoleAssignmentInput{UserID: targetA, Reason: "Validate system-role assignment"}); err != nil {
		t.Fatalf("assigning system role: %v", err)
	}
	if err := accessService.UnassignRole(tenantA, orgA, systemRoleID, targetA, actorA, models.ManagedRoleUnassignmentInput{Reason: "System-role assignment test complete"}); err != nil {
		t.Fatalf("unassigning system role: %v", err)
	}

	// A role carrying settings:configure cannot be stripped from the final
	// active administrator. Once another administrator exists, removal is safe.
	adminRole, err := accessService.CreateRole(tenantA, orgA, actorA, models.ManagedRoleCreateInput{
		Name: "Tenant Administrator A", Permissions: []models.PermissionGrant{{Resource: "settings", Action: "configure"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := accessService.AssignRole(tenantA, orgA, adminRole.ID, actorA, models.ManagedRoleAssignmentInput{UserID: actorA, Reason: "Establish tenant administrator"}); err != nil {
		t.Fatal(err)
	}
	emptyPermissions := []models.PermissionGrant{}
	if _, err := accessService.UpdateRole(tenantA, orgA, adminRole.ID, actorA, models.ManagedRolePatch{
		Permissions: &emptyPermissions, ExpectedVersion: adminRole.Version,
	}); !errors.Is(err, service.ErrLastTenantAdministrator) {
		t.Fatalf("last-administrator permission removal error=%v", err)
	}
	if err := accessService.UnassignRole(tenantA, orgA, adminRole.ID, actorA, actorA, models.ManagedRoleUnassignmentInput{Reason: "Lockout prevention test"}); !errors.Is(err, service.ErrLastTenantAdministrator) {
		t.Fatalf("last-administrator removal error=%v", err)
	}
	backupAdminRole, err := accessService.CreateRole(tenantA, orgA, actorA, models.ManagedRoleCreateInput{
		Name: "Tenant Administrator B", Permissions: []models.PermissionGrant{{Resource: "settings", Action: "configure"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := accessService.AssignRole(tenantA, orgA, backupAdminRole.ID, actorA, models.ManagedRoleAssignmentInput{UserID: backupA, Reason: "Maintain administrative redundancy"}); err != nil {
		t.Fatal(err)
	}
	adminRole, err = accessService.UpdateRole(tenantA, orgA, adminRole.ID, actorA, models.ManagedRolePatch{
		Permissions: &emptyPermissions, ExpectedVersion: adminRole.Version,
	})
	if err != nil || len(adminRole.Permissions) != 0 {
		t.Fatalf("safe administrative permission removal role=%#v err=%v", adminRole, err)
	}
	if err := accessService.UnassignRole(tenantA, orgA, adminRole.ID, actorA, actorA, models.ManagedRoleUnassignmentInput{Reason: "Backup administrator is active"}); err != nil {
		t.Fatal(err)
	}

	// Two writers racing the same version must produce one commit and one
	// conflict, rather than silently losing an update.
	concurrentRole, err := accessService.CreateRole(tenantA, orgA, actorA, models.ManagedRoleCreateInput{Name: "Concurrent Reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	var updateGroup sync.WaitGroup
	results := make(chan error, 2)
	for _, text := range []string{"Writer one", "Writer two"} {
		text := text
		updateGroup.Add(1)
		go func() {
			defer updateGroup.Done()
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
			if _, tenantErr := worker.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); tenantErr != nil {
				results <- tenantErr
				return
			}
			_, updateErr := accessService.UpdateRole(database.WithQuerier(ctx, worker), orgA, concurrentRole.ID, actorA,
				models.ManagedRolePatch{Description: &text, ExpectedVersion: concurrentRole.Version})
			results <- updateErr
		}()
	}
	updateGroup.Wait()
	close(results)
	successes, conflicts := 0, 0
	for updateErr := range results {
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, service.ErrManagedRoleConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent update error=%v", updateErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent role results successes=%d conflicts=%d", successes, conflicts)
	}

	tenantB := setTenant(orgB)
	if _, err := accessService.GetRole(tenantB, orgB, clone.ID); !errors.Is(err, service.ErrManagedRoleNotFound) {
		t.Fatalf("cross-tenant role read error=%v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by)
		VALUES ($1,$2,$3,$1)`, userB, clone.ID, orgB); err == nil {
		t.Fatal("database accepted a custom role from another tenant")
	}
	var visibleCustom int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM roles WHERE organization_id IS NOT NULL`).Scan(&visibleCustom); err != nil || visibleCustom != 0 {
		t.Fatalf("tenant B custom role visibility=%d err=%v", visibleCustom, err)
	}

	_ = setTenant(orgA)
	var eventCount, outboxCount int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM role_change_events WHERE organization_id=$1`, orgA).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM queue_outbox WHERE tenant_id=$1 AND envelope->>'type'='notification.event'`, orgA).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if eventCount < 12 || outboxCount != eventCount {
		t.Fatalf("audit/outbox event counts audit=%d outbox=%d", eventCount, outboxCount)
	}
}
