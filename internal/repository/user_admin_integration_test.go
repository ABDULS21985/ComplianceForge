package repository_test

import (
	"context"
	"errors"
	"fmt"
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

type failingDirectoryOutbox struct{}

func (failingDirectoryOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced directory outbox failure")
}

// TestUserAdministrationWithNonSuperuserTenants executes the production user
// and group aggregate through a NOSUPERUSER/NOBYPASSRLS role. It proves RLS,
// last-admin serialization, optimistic versions, ownership transfer,
// append-only history, idempotent imports, quota guards, and outbox atomicity.
func TestUserAdministrationWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB, limitOrg := uuid.NewString(), uuid.NewString(), uuid.NewString()
	adminA, adminA2, executorA, adminB := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleA, roleB, viewerRole := uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleName := "grc_directory_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	permissionID := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Directory Tenant A',$4,'active','unlimited'),($2,'Directory Tenant B',$5,'active','unlimited'),
		($3,'Directory Limit Tenant',$6,'active','starter')`, orgA, orgB, limitOrg,
		"directory-a-"+orgA, "directory-b-"+orgB, "directory-limit-"+limitOrg); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status,invitation_status)
		VALUES($1,$2,$3,'Ada','Admin','active','accepted'),($4,$2,$5,'Grace','Admin','active','accepted'),
		($6,$2,$7,'Execution','User','active','accepted'),($8,$9,$10,'Tenant','B','active','accepted')`,
		adminA, orgA, adminA+"@example.test", adminA2, adminA2+"@example.test", executorA,
		executorA+"@example.test", adminB, orgB, adminB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO permissions(id,resource,action,description)
		VALUES($1,'settings','configure','Directory integration administrator permission')`, permissionID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO roles(id,organization_id,name,slug,description,is_system_role,is_custom,created_by,updated_by)
		VALUES($1,$2,'Directory Admin A',$3,'Tenant A administration',false,true,$4,$4),
		($5,$6,'Directory Admin B',$7,'Tenant B administration',false,true,$8,$8),
		($9,NULL,'Viewer','viewer','Read-only directory account',true,false,NULL,NULL)`,
		roleA, orgA, "directory-admin-a-"+strings.ReplaceAll(roleA[:8], "-", ""), adminA,
		roleB, orgB, "directory-admin-b-"+strings.ReplaceAll(roleB[:8], "-", ""), adminB, viewerRole); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_id) VALUES($1,$3),($2,$3)`, roleA, roleB, permissionID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by) VALUES
		($1,$4,$3,$1),($2,$4,$3,$1),($5,$6,$3,$1),($7,$8,$9,$7)`,
		adminA, adminA2, orgA, roleA, executorA, viewerRole, adminB, roleB, orgB); err != nil {
		t.Fatal(err)
	}

	limitUsers := make([]string, 5)
	for i := range limitUsers {
		limitUsers[i] = uuid.NewString()
		if _, err = pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,status,invitation_status)
			VALUES($1,$2,$3,$4,'active','accepted')`, limitUsers[i], limitOrg,
			fmt.Sprintf("limit-%d@example.test", i), fmt.Sprintf("Limit %d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by)
			VALUES($1,$2,$3,$1)`, limitUsers[i], viewerRole, limitOrg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quotedRole+
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+quotedRole+
		"; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO "+quotedRole); err != nil {
		t.Fatal(err)
	}

	var rolePool *pgxpool.Pool
	var conn *pgxpool.Conn
	defer func() {
		if rolePool != nil {
			rolePool.Close()
		}
		if conn != nil {
			_, _ = conn.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`)
			_, _ = conn.Exec(context.Background(), "RESET ROLE")
			conn.Release()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		cleanupConn, cleanupErr := pool.Acquire(cleanupCtx)
		if cleanupErr == nil {
			_, _ = cleanupConn.Exec(cleanupCtx, `SET session_replication_role='replica'`)
			_, _ = cleanupConn.Exec(cleanupCtx, `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB, limitOrg})
			_, _ = cleanupConn.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB, limitOrg})
			_, _ = cleanupConn.Exec(cleanupCtx, `DELETE FROM roles WHERE id=$1`, viewerRole)
			_, _ = cleanupConn.Exec(cleanupCtx, `DELETE FROM permissions WHERE id=$1`, permissionID)
			_, _ = cleanupConn.Exec(cleanupCtx, `SET session_replication_role='origin'`)
			cleanupConn.Release()
		}
		_, _ = pool.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(cleanupCtx, "DROP ROLE "+quotedRole)
	}()

	conn, err = pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
		t.Fatal(err)
	}
	setTenant := func(organizationID string) context.Context {
		t.Helper()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, organizationID); err != nil {
			t.Fatal(err)
		}
		return database.WithQuerier(ctx, conn)
	}

	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewUserAdministrationRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	directory := service.NewUserAdministrationService(repo, zerolog.Nop())
	tenantA := setTenant(orgA)

	failingRepo, err := repository.NewUserAdministrationRepository(pool, failingDirectoryOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.NewUserAdministrationService(failingRepo, zerolog.Nop()).CreateUser(tenantA, orgA, adminA, models.DirectoryUserCreateInput{
		Email: "rollback@example.test", FirstName: "Rollback", Reason: "Prove outbox rollback",
	})
	if err == nil || !strings.Contains(err.Error(), "forced directory outbox failure") {
		t.Fatalf("forced create rollback error=%v", err)
	}
	var rolledBack int
	if err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE organization_id=$1 AND email='rollback@example.test'`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("outbox rollback user count=%d err=%v", rolledBack, err)
	}

	target, err := directory.CreateUser(tenantA, orgA, adminA, models.DirectoryUserCreateInput{
		Email: "target@example.test", FirstName: "Target", LastName: "Owner", Department: "Operations",
		InitialStatus: models.UserStatusActive, InitialRoleSlug: "viewer", Reason: "Create lifecycle test user",
	})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := directory.CreateUser(tenantA, orgA, adminA, models.DirectoryUserCreateInput{
		Email: "replacement@example.test", FirstName: "Replacement", LastName: "Owner", Department: "Security",
		InitialStatus: models.UserStatusActive, InitialRoleSlug: "viewer", Reason: "Create ownership replacement",
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := directory.CreateUser(tenantA, orgA, adminA, models.DirectoryUserCreateInput{
		Email: "report@example.test", FirstName: "Direct", LastName: "Report", ManagerUserID: &target.ID,
		InitialRoleSlug: "viewer", Reason: "Create reporting relationship",
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.Version != 1 || target.InvitationStatus != models.DirectoryInvitationNotRequired || report.InvitationStatus != models.DirectoryInvitationReady {
		t.Fatalf("target=%#v report=%#v", target, report)
	}
	_, err = directory.CreateUser(tenantA, orgA, adminA, models.DirectoryUserCreateInput{
		Email: "foreign-manager@example.test", FirstName: "Foreign", ManagerUserID: &adminB,
		InitialRoleSlug: "viewer", Reason: "Reject cross tenant manager",
	})
	if !errors.Is(err, service.ErrUserAdministrationConflict) {
		t.Fatalf("cross-tenant manager error=%v", err)
	}

	department := "Engineering"
	target, err = directory.UpdateUser(tenantA, orgA, target.ID, adminA, models.DirectoryUserPatch{
		ExpectedVersion: target.Version, Department: &department, Reason: "Move owner into engineering",
	})
	if err != nil || target.Version != 2 || target.Department != department {
		t.Fatalf("updated target=%#v err=%v", target, err)
	}
	_, err = directory.UpdateUser(tenantA, orgA, target.ID, adminA, models.DirectoryUserPatch{
		ExpectedVersion: target.Version, ManagerUserID: &report.ID, Reason: "Reject circular reporting hierarchy",
	})
	if !errors.Is(err, service.ErrUserAdministrationConflict) {
		t.Fatalf("manager cycle error=%v", err)
	}
	_, err = directory.UpdateUser(tenantA, orgA, target.ID, adminA, models.DirectoryUserPatch{
		ExpectedVersion: 1, Department: &department, Reason: "Stale version must fail",
	})
	if !errors.Is(err, service.ErrUserAdministrationVersion) {
		t.Fatalf("stale version error=%v", err)
	}
	items, total, err := directory.ListUsers(tenantA, orgA, models.DirectoryUserListFilter{PaginationRequest: models.PaginationRequest{Page: 1, PageSize: 20}, Search: "target", Department: "Engineering", SortBy: "email", SortDirection: "asc"})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != target.ID {
		t.Fatalf("filtered users=%#v total=%d err=%v", items, total, err)
	}

	staticGroup, err := directory.CreateGroup(tenantA, orgA, adminA, models.DirectoryGroupCreateInput{Name: "Control Owners", Reason: "Create control owner group"})
	if err != nil {
		t.Fatal(err)
	}
	staticGroup, err = directory.ChangeGroupMembers(tenantA, orgA, staticGroup.ID, adminA, models.DirectoryGroupBulkMembersInput{
		ExpectedVersion: staticGroup.Version, AddUserIDs: []string{target.ID, replacement.ID}, Reason: "Assign control owners",
	})
	if err != nil || staticGroup.Version != 2 || staticGroup.MemberCount != 2 {
		t.Fatalf("static group=%#v err=%v", staticGroup, err)
	}
	members, memberTotal, err := directory.ListGroupMembers(tenantA, orgA, staticGroup.ID, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || memberTotal != 2 || len(members) != 2 {
		t.Fatalf("static members=%#v total=%d err=%v", members, memberTotal, err)
	}
	dynamicGroup, err := directory.CreateGroup(tenantA, orgA, adminA, models.DirectoryGroupCreateInput{
		Name: "Active Engineering", GroupType: models.DirectoryGroupDynamic,
		MembershipRule: []byte(`{"departments":["Engineering"],"statuses":["active"]}`), Reason: "Create safe dynamic group",
	})
	if err != nil {
		t.Fatal(err)
	}
	members, memberTotal, err = directory.ListGroupMembers(tenantA, orgA, dynamicGroup.ID, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || memberTotal != 1 || len(members) != 1 || members[0].ID != target.ID {
		t.Fatalf("dynamic members=%#v total=%d err=%v", members, memberTotal, err)
	}
	filteredMembers, filteredTotal, err := directory.ListUsers(tenantA, orgA, models.DirectoryUserListFilter{
		PaginationRequest: models.PaginationRequest{Page: 1, PageSize: 20}, GroupID: dynamicGroup.ID,
	})
	if err != nil || filteredTotal != 1 || len(filteredMembers) != 1 || filteredMembers[0].ID != target.ID {
		t.Fatalf("dynamic group user filter=%#v total=%d err=%v", filteredMembers, filteredTotal, err)
	}
	_, err = directory.AddGroupMember(tenantA, orgA, dynamicGroup.ID, adminA, models.DirectoryGroupMemberInput{ExpectedVersion: dynamicGroup.Version, UserID: replacement.ID, Reason: "Must be rule managed"})
	if !errors.Is(err, service.ErrUserAdministrationDynamicGroup) {
		t.Fatalf("dynamic mutation error=%v", err)
	}

	target, err = directory.SuspendUser(tenantA, orgA, target.ID, adminA, models.DirectoryUserStateInput{ExpectedVersion: target.Version, Reason: "Temporary access review"})
	if err != nil || target.Status != models.UserStatusInactive {
		t.Fatalf("suspended target=%#v err=%v", target, err)
	}
	target, err = directory.ReactivateUser(tenantA, orgA, target.ID, adminA, models.DirectoryUserStateInput{ExpectedVersion: target.Version, Reason: "Access review completed"})
	if err != nil || target.Status != models.UserStatusActive {
		t.Fatalf("reactivated target=%#v err=%v", target, err)
	}

	csvContent := []byte("email,first_name,last_name,department,manager_email,role_slug\n" +
		"target@example.test,Target,Owner,Engineering,,viewer\n" +
		"imported@example.test,Imported,User,Engineering,replacement@example.test,viewer\n")
	preview, err := directory.PreviewImport(tenantA, orgA, csvContent)
	if err != nil || preview.ValidCount != 2 || preview.CreateCount != 1 || preview.UpdateCount != 1 {
		t.Fatalf("import preview=%#v err=%v", preview, err)
	}
	importResult, err := directory.ApplyImport(tenantA, orgA, adminA, "directory-import-live-001", "Apply validated directory import", csvContent)
	if err != nil || importResult.CreatedCount != 1 || importResult.UpdatedCount != 1 || importResult.Replayed {
		t.Fatalf("import result=%#v err=%v", importResult, err)
	}
	replay, err := directory.ApplyImport(tenantA, orgA, adminA, "directory-import-live-001", "Apply validated directory import", csvContent)
	if err != nil || !replay.Replayed || replay.ID != importResult.ID {
		t.Fatalf("import replay=%#v err=%v", replay, err)
	}
	_, err = directory.ApplyImport(tenantA, orgA, adminA, "directory-import-live-001", "Different input must conflict", []byte("email,first_name\nother@example.test,Other\n"))
	if !errors.Is(err, service.ErrUserAdministrationIdempotency) {
		t.Fatalf("import idempotency error=%v", err)
	}
	failedCSV := []byte("email,first_name,role_slug\nrollback-import@example.test,Rollback,viewer\n")
	_, err = service.NewUserAdministrationService(failingRepo, zerolog.Nop()).ApplyImport(tenantA, orgA, adminA, "directory-import-rollback", "Prove import outbox rollback", failedCSV)
	if err == nil || !strings.Contains(err.Error(), "forced directory outbox failure") {
		t.Fatalf("forced import rollback error=%v", err)
	}
	if err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE organization_id=$1 AND email='rollback-import@example.test'`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("import rollback user count=%d err=%v", rolledBack, err)
	}

	target, err = directory.GetUser(tenantA, orgA, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.NewString()
	if _, err = conn.Exec(ctx, `INSERT INTO assets(id,organization_id,asset_ref,name,asset_type,owner_user_id,created_by)
		VALUES($1::uuid,$2::uuid,'AST-DIRECTORY-001','Directory-owned service','service',$3::uuid,$4::uuid)`, assetID, orgA, target.ID, adminA); err != nil {
		t.Fatal(err)
	}
	impact, err := directory.PreviewOwnership(tenantA, orgA, target.ID)
	if err != nil || impact.Total < 2 || impact.DirectReports != 1 || !impact.RequiresReplacement {
		t.Fatalf("ownership impact=%#v err=%v", impact, err)
	}
	if err = directory.DeprovisionUser(tenantA, orgA, target.ID, adminA, models.DirectoryUserDeprovisionInput{ExpectedVersion: target.Version, Reason: "Owner cannot depart without transfer"}); !errors.Is(err, service.ErrUserAdministrationOwnership) {
		t.Fatalf("missing replacement error=%v", err)
	}
	transferred, target, err := directory.TransferOwnership(tenantA, orgA, target.ID, adminA, models.DirectoryOwnershipTransferInput{
		ExpectedVersion: target.Version, ReplacementUserID: replacement.ID, Reason: "Transfer duties before departure",
	})
	if err != nil || transferred.Total < 2 || target.Version < 2 {
		t.Fatalf("transferred=%#v target=%#v err=%v", transferred, target, err)
	}
	var ownerID, managerID string
	if err = conn.QueryRow(ctx, `SELECT owner_user_id FROM assets WHERE organization_id=$1 AND id=$2`, orgA, assetID).Scan(&ownerID); err != nil || ownerID != replacement.ID {
		t.Fatalf("asset owner=%q err=%v", ownerID, err)
	}
	if err = conn.QueryRow(ctx, `SELECT manager_user_id FROM users WHERE organization_id=$1 AND id=$2`, orgA, report.ID).Scan(&managerID); err != nil || managerID != replacement.ID {
		t.Fatalf("report manager=%q err=%v", managerID, err)
	}
	if err = directory.DeprovisionUser(tenantA, orgA, target.ID, adminA, models.DirectoryUserDeprovisionInput{ExpectedVersion: target.Version, Reason: "Access removed after ownership transfer"}); err != nil {
		t.Fatal(err)
	}
	if _, err = directory.GetUser(tenantA, orgA, target.ID); !errors.Is(err, service.ErrUserAdministrationNotFound) {
		t.Fatalf("deprovisioned user read error=%v", err)
	}
	staticGroup, err = directory.GetGroup(tenantA, orgA, staticGroup.ID)
	if err != nil || staticGroup.MemberCount != 1 || staticGroup.Version != 3 {
		t.Fatalf("deprovisioned group=%#v err=%v", staticGroup, err)
	}

	events, eventTotal, err := directory.ListUserEvents(tenantA, orgA, target.ID, models.PaginationRequest{Page: 1, PageSize: 100})
	if err != nil || eventTotal < 7 || len(events) != eventTotal {
		t.Fatalf("directory events=%d/%d err=%v", len(events), eventTotal, err)
	}
	if tag, err := conn.Exec(ctx, `UPDATE directory_change_events SET reason='tampered' WHERE id=$1`, events[0].ID); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("event update affected=%d err=%v", tag.RowsAffected(), err)
	}
	if tag, err := conn.Exec(ctx, `DELETE FROM directory_imports WHERE id=$1`, importResult.ID); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("import delete affected=%d err=%v", tag.RowsAffected(), err)
	}

	tenantB := setTenant(orgB)
	if _, err = directory.GetUser(tenantB, orgB, replacement.ID); !errors.Is(err, service.ErrUserAdministrationNotFound) {
		t.Fatalf("cross-tenant user read error=%v", err)
	}
	if _, err = directory.GetGroup(tenantB, orgB, staticGroup.ID); !errors.Is(err, service.ErrUserAdministrationNotFound) {
		t.Fatalf("cross-tenant group read error=%v", err)
	}
	var visible int
	if err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM directory_groups`).Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("tenant B visible groups=%d err=%v", visible, err)
	}

	limitTenant := setTenant(limitOrg)
	_, err = directory.CreateUser(limitTenant, limitOrg, limitUsers[0], models.DirectoryUserCreateInput{
		Email: "over-limit@example.test", FirstName: "Over", InitialRoleSlug: "viewer", Reason: "Must honor starter seat limit",
	})
	if !errors.Is(err, service.ErrSubscriptionLimitExceeded) {
		t.Fatalf("seat entitlement error=%v", err)
	}

	roleConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	roleConfig.AfterConnect = func(connectCtx context.Context, roleConn *pgx.Conn) error {
		_, roleErr := roleConn.Exec(connectCtx, "SET ROLE "+quotedRole)
		return roleErr
	}
	rolePool, err = pgxpool.NewWithConfig(ctx, roleConfig)
	if err != nil {
		t.Fatal(err)
	}
	type suspensionResult struct {
		userID string
		err    error
	}
	results := make(chan suspensionResult, 2)
	var group sync.WaitGroup
	for _, request := range []struct{ target, actor string }{{adminA2, adminA}, {adminA, adminA2}} {
		request := request
		group.Add(1)
		go func() {
			defer group.Done()
			roleConn, acquireErr := rolePool.Acquire(ctx)
			if acquireErr != nil {
				results <- suspensionResult{request.target, acquireErr}
				return
			}
			defer roleConn.Release()
			if _, tenantErr := roleConn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); tenantErr != nil {
				results <- suspensionResult{request.target, tenantErr}
				return
			}
			_, suspendErr := directory.SuspendUser(database.WithQuerier(ctx, roleConn), orgA, request.target, request.actor,
				models.DirectoryUserStateInput{ExpectedVersion: 1, Reason: "Concurrent last administrator guard"})
			results <- suspensionResult{request.target, suspendErr}
		}()
	}
	group.Wait()
	close(results)
	succeeded, rejected := 0, 0
	for result := range results {
		if result.err == nil {
			succeeded++
		} else if errors.Is(result.err, service.ErrUserAdministrationLastAdmin) || errors.Is(result.err, service.ErrUserAdministrationConflict) {
			rejected++
		} else {
			t.Fatalf("unexpected concurrent suspension for %s: %v", result.userID, result.err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent suspension succeeded=%d rejected=%d", succeeded, rejected)
	}
	tenantA = setTenant(orgA)
	if err = conn.QueryRow(ctx, `SELECT COUNT(DISTINCT u.id) FROM users u JOIN user_roles ur
		ON ur.organization_id=u.organization_id AND ur.user_id=u.id JOIN role_permissions rp ON rp.role_id=ur.role_id
		JOIN permissions p ON p.id=rp.permission_id WHERE u.organization_id=$1 AND u.status='active'
		AND u.deleted_at IS NULL AND p.resource='settings' AND p.action='configure'`, orgA).Scan(&visible); err != nil || visible != 1 {
		t.Fatalf("remaining active administrators=%d err=%v", visible, err)
	}
	var outboxCount int
	if err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM queue_outbox WHERE tenant_id=$1`, orgA).Scan(&outboxCount); err != nil || outboxCount < 15 {
		t.Fatalf("directory outbox count=%d err=%v", outboxCount, err)
	}
}
