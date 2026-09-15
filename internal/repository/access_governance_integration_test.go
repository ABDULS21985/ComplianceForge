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

// TestAccessGovernanceNonBypass exercises the real PostgreSQL policy boundary,
// immutable evidence, optimistic/replay controls, timed authority, and SoD.
func TestAccessGovernanceNonBypass(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM schema_migrations`).Scan(&version); err != nil || version != 58 {
		t.Fatalf("schema58 required, version=%d err=%v", version, err)
	}
	org, otherOrg := uuid.NewString(), uuid.NewString()
	actor, reviewer, subject, otherUser := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleA, roleB, roleOther := uuid.NewString(), uuid.NewString(), uuid.NewString()
	adminRole, backupRole := uuid.NewString(), uuid.NewString()
	resource := "governance_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	runtime := "grc_governance_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{runtime}.Sanitize()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status) VALUES($1,'Governance A',$3,'active'),($2,'Governance B',$4,'active')`, org, otherOrg, "governance-a-"+org, "governance-b-"+otherOrg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		tx, err := pool.Begin(cleanupCtx)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(cleanupCtx)
		// Fixture disposal is privileged, explicit, tenant-bounded, transactional.
		// The runtime itself cannot disable triggers or mutate these ledgers.
		if _, err := tx.Exec(cleanupCtx, `SET LOCAL session_replication_role='replica'`); err != nil {
			t.Error(err)
			return
		}
		for _, table := range []string{"queue_outbox", "queue_inbox"} {
			if _, err := tx.Exec(cleanupCtx, "DELETE FROM "+table+" WHERE tenant_id=ANY($1::uuid[])", []string{org, otherOrg}); err != nil {
				t.Error(err)
				return
			}
		}
		for _, table := range []string{"access_review_decisions", "access_review_items", "access_review_campaigns", "access_sod_violations", "access_sod_exceptions", "access_sod_rules", "access_governance_events", "access_audit_log", "role_change_events", "role_permissions", "user_roles", "roles", "users", "organizations"} {
			query := "DELETE FROM " + table + " WHERE organization_id=ANY($1::uuid[])"
			if table == "organizations" {
				query = "DELETE FROM organizations WHERE id=ANY($1::uuid[])"
			}
			if table == "role_permissions" {
				query = "DELETE FROM role_permissions WHERE role_id=ANY($1::uuid[])"
				if _, err := tx.Exec(cleanupCtx, query, []string{roleA, roleB, roleOther, adminRole, backupRole}); err != nil {
					t.Error(err)
					return
				}
				continue
			}
			if _, err := tx.Exec(cleanupCtx, query, []string{org, otherOrg}); err != nil {
				t.Error(err)
				return
			}
		}
		if _, err := tx.Exec(cleanupCtx, `DELETE FROM evidence_scheduler_tenants WHERE organization_id=ANY($1::uuid[])`, []string{org, otherOrg}); err != nil {
			t.Error(err)
			return
		}
		if _, err := tx.Exec(cleanupCtx, `DELETE FROM permissions WHERE resource=$1`, resource); err != nil {
			t.Error(err)
			return
		}
		if err := tx.Commit(cleanupCtx); err != nil {
			t.Error(err)
			return
		}
		if _, err := pool.Exec(cleanupCtx, "DROP OWNED BY "+quoted); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanupCtx, "DROP ROLE "+quoted); err != nil {
			t.Error(err)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES
	 ($1,$2,$3,'Sponsor','Admin','active'),($4,$2,$5,'Independent','Reviewer','active'),($6,$2,$7,'Review','Subject','active'),($8,$9,$10,'Other','Tenant','active')`, actor, org, actor+"@example.test", reviewer, reviewer+"@example.test", subject, subject+"@example.test", otherUser, otherOrg, otherUser+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO roles(id,organization_id,name,slug,is_system_role,is_custom,created_by) VALUES
	 ($1::uuid,$2::uuid,'Prepare','prepare-'||$1::text,false,true,$3::uuid),($4::uuid,$2::uuid,'Approve','approve-'||$4::text,false,true,$3::uuid),($5::uuid,$6::uuid,'Other','other-'||$5::text,false,true,$7::uuid)`, roleA, org, actor, roleB, roleOther, otherOrg, otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO permissions(resource,action) VALUES($1,'read'),($1,'update')`, resource); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_id) SELECT CASE WHEN action='read' THEN $2::uuid ELSE $3::uuid END,id FROM permissions WHERE resource=$1`, resource, roleA, roleB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quoted+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOINHERIT NOCREATEDB NOCREATEROLE"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quoted+"; GRANT SELECT ON organizations,users,permissions,effective_user_roles TO "+quoted+"; GRANT UPDATE ON organizations TO "+quoted+"; GRANT SELECT,INSERT,UPDATE,DELETE ON user_roles,roles,role_permissions,user_entity_permissions TO "+quoted+"; GRANT SELECT,INSERT ON access_review_items,access_review_decisions,access_sod_violations,access_governance_events,role_change_events,queue_outbox TO "+quoted+"; GRANT SELECT,INSERT,UPDATE ON access_review_campaigns,access_sod_rules,access_sod_exceptions TO "+quoted); err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET ROLE "+quoted); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), "RESET ROLE")
	var unsafe bool
	if err := conn.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR EXISTS(SELECT 1 FROM pg_class WHERE relname='user_roles' AND relowner=(SELECT oid FROM pg_roles WHERE rolname=current_user)) FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatalf("unsafe runtime=%v err=%v", unsafe, err)
	}
	setTenant := func(id string) context.Context {
		t.Helper()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, id); err != nil {
			t.Fatal(err)
		}
		return database.WithQuerier(ctx, conn)
	}
	tenant := setTenant(org)
	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.NewAccessGovernanceRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	app, err := service.NewAccessGovernanceService(store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	roleStore, err := repository.NewAccessAdministrationRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	roles := service.NewAccessAdministrationService(roleStore, zerolog.Nop())
	setWindow := func(role, user, windowActor string, in models.ManagedRoleWindowInput) (*models.ManagedRoleAssignment, error) {
		assignments, err := roles.ListAssignments(tenant, org, role)
		if err != nil {
			return nil, err
		}
		for _, assignment := range assignments {
			if assignment.UserID == user {
				in.AssignmentID = assignment.AssignmentID
				break
			}
		}
		return app.SetAssignmentWindow(tenant, org, role, user, windowActor, in)
	}
	if err := roles.AssignRole(tenant, org, roleA, actor, models.ManagedRoleAssignmentInput{UserID: subject, Reason: "Approved prepare assignment"}); err != nil {
		t.Fatal(err)
	}
	if err := roles.AssignRole(tenant, org, roleOther, actor, models.ManagedRoleAssignmentInput{UserID: subject, Reason: "Cross tenant must be rejected"}); err == nil {
		t.Fatal("cross tenant assignment allowed")
	}
	in := models.AccessReviewCampaignInput{Name: "Quarterly access review", ReviewerID: reviewer, SubjectIDs: []string{subject}, DueAt: time.Now().Add(24 * time.Hour), Reason: "Quarterly independent recertification"}
	campaign, err := app.CreateCampaign(tenant, org, actor, in)
	if err != nil {
		t.Fatal(err)
	}
	items, total, err := app.ListItems(tenant, org, campaign.ID, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || total != 1 {
		t.Fatalf("items=%+v total=%d err=%v", items, total, err)
	}
	decisionIn := models.AccessReviewDecisionInput{Decision: "retain", Reason: "Business need remains valid", RequestID: uuid.NewString(), SnapshotSHA256: items[0].SnapshotSHA256}
	if _, err := app.DecideItem(tenant, org, campaign.ID, items[0].ID, subject, decisionIn); !errors.Is(err, service.ErrGovernanceSeparation) {
		t.Fatalf("self review err=%v", err)
	}
	if _, err := app.DecideItem(tenant, org, campaign.ID, items[0].ID, actor, decisionIn); !errors.Is(err, service.ErrGovernanceSeparation) {
		t.Fatalf("sponsor review err=%v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO access_review_decisions(organization_id,item_id,reviewer_id,decision,reason,request_id,snapshot_sha256) VALUES($1,$2,$3,'retain','Direct self review',$4,$5)`, org, items[0].ID, subject, uuid.NewString(), items[0].SnapshotSHA256); err == nil {
		t.Fatal("DB allowed direct self review")
	}
	// Two genuinely separate tenant connections race the same request: one
	// durable decision/outbox row, with the other receiving the same result.
	var wg sync.WaitGroup
	results := make(chan string, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := pool.Acquire(ctx)
			if err != nil {
				failures <- err
				return
			}
			defer c.Release()
			if _, err := c.Exec(ctx, "SET ROLE "+quoted); err != nil {
				failures <- err
				return
			}
			defer c.Exec(context.Background(), "RESET ROLE")
			if _, err := c.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, org); err != nil {
				failures <- err
				return
			}
			result, err := app.DecideItem(database.WithQuerier(ctx, c), org, campaign.ID, items[0].ID, reviewer, decisionIn)
			if err != nil {
				failures <- err
				return
			}
			results <- result.ID
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	first, second := <-results, <-results
	if first != second {
		t.Fatalf("duplicate decision IDs %s %s", first, second)
	}
	decisionIn.Reason = "Changed replay must conflict"
	if _, err := app.DecideItem(tenant, org, campaign.ID, items[0].ID, reviewer, decisionIn); !errors.Is(err, service.ErrGovernanceConflict) {
		t.Fatalf("changed replay err=%v", err)
	}
	if _, err := conn.Exec(ctx, `UPDATE access_review_decisions SET reason='Tampered' WHERE organization_id=$1`, org); err == nil {
		t.Fatal("decision UPDATE allowed")
	}
	if _, err := conn.Exec(ctx, `DELETE FROM access_review_items WHERE organization_id=$1`, org); err == nil {
		t.Fatal("snapshot DELETE allowed")
	}
	if _, err := app.TransitionCampaign(tenant, org, campaign.ID, actor, "completed", models.AccessGovernanceTransitionInput{ExpectedVersion: campaign.Version, Reason: "All review items completed"}); err != nil {
		t.Fatal(err)
	}
	revokeCampaign, err := app.CreateCampaign(tenant, org, actor, in)
	if err != nil {
		t.Fatal(err)
	}
	revokeItems, _, err := app.ListItems(tenant, org, revokeCampaign.ID, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	oldAssignments, err := roles.ListAssignments(tenant, org, roleA)
	if err != nil {
		t.Fatal(err)
	}
	oldAssignmentID := oldAssignments[0].AssignmentID
	if _, err := app.DecideItem(tenant, org, revokeCampaign.ID, revokeItems[0].ID, reviewer, models.AccessReviewDecisionInput{Decision: "revoke", Reason: "Project access no longer needed", RequestID: uuid.NewString(), SnapshotSHA256: revokeItems[0].SnapshotSHA256}); err != nil {
		t.Fatalf("non-admin revocation failed: %v", err)
	}
	if err := roles.AssignRole(tenant, org, roleA, actor, models.ManagedRoleAssignmentInput{UserID: subject, Reason: "New separately approved access incarnation"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetAssignmentWindow(tenant, org, roleA, subject, actor, models.ManagedRoleWindowInput{AssignmentID: oldAssignmentID, ValidFrom: time.Now(), ExpiresAt: time.Now().Add(time.Hour), ExpectedVersion: 1, Reason: "Stale incarnation must conflict"}); !errors.Is(err, service.ErrGovernanceConflict) {
		t.Fatalf("ABA window extension err=%v", err)
	}
	setTenant(otherOrg)
	if _, err := app.GetCampaign(database.WithQuerier(ctx, conn), otherOrg, campaign.ID); !errors.Is(err, service.ErrGovernanceNotFound) {
		t.Fatalf("cross tenant campaign err=%v", err)
	}
	if _, err := app.GetCampaign(database.WithQuerier(ctx, conn), org, campaign.ID); !errors.Is(err, service.ErrGovernanceNotFound) {
		t.Fatalf("explicit wrong current tenant err=%v", err)
	}
	tenant = setTenant(org)
	stale, err := app.CreateCampaign(tenant, org, actor, in)
	if err != nil {
		t.Fatal(err)
	}
	staleItems, _, err := app.ListItems(tenant, org, stale.ID, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC()
	end := start.Add(30 * time.Minute)
	if _, err := setWindow(roleA, subject, actor, models.ManagedRoleWindowInput{ValidFrom: start, ExpiresAt: end, ExpectedVersion: 1, Reason: "Timeboxed project access"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DecideItem(tenant, org, stale.ID, staleItems[0].ID, reviewer, models.AccessReviewDecisionInput{Decision: "revoke", Reason: "Stale evidence must not revoke", RequestID: uuid.NewString(), SnapshotSHA256: staleItems[0].SnapshotSHA256}); !errors.Is(err, service.ErrGovernanceConflict) {
		t.Fatalf("stale snapshot err=%v", err)
	}
	rule, err := app.CreateSoDRule(tenant, org, actor, models.AccessSoDRuleInput{Name: "Prepare versus approve", RoleAID: roleA, RoleBID: roleB, Reason: "Independent approval required"})
	if err != nil {
		t.Fatal(err)
	}
	if err := roles.AssignRole(tenant, org, roleB, actor, models.ManagedRoleAssignmentInput{UserID: subject, Reason: "Unapproved conflicting assignment", ValidFrom: &start, ExpiresAt: &end}); err == nil {
		t.Fatal("unapproved SoD assignment allowed")
	}
	exception, err := app.RequestException(tenant, org, rule.ID, actor, models.AccessSoDExceptionInput{SubjectID: subject, ValidFrom: start.Add(-time.Second), ExpiresAt: end.Add(time.Minute), Reason: "Supervised temporary coverage"})
	if err != nil {
		t.Fatal(err)
	}
	transition := models.AccessGovernanceTransitionInput{ExpectedVersion: 1, Reason: "Independent exception decision"}
	if _, err := app.DecideException(tenant, org, exception.ID, actor, "approved", transition); !errors.Is(err, service.ErrGovernanceSeparation) {
		t.Fatalf("exception sponsor approved err=%v", err)
	}
	if _, err := app.DecideException(tenant, org, exception.ID, subject, "approved", transition); !errors.Is(err, service.ErrGovernanceSeparation) {
		t.Fatalf("exception subject approved err=%v", err)
	}
	exception, err = app.DecideException(tenant, org, exception.ID, reviewer, "approved", transition)
	if err != nil {
		t.Fatal(err)
	}
	if err := roles.AssignRole(tenant, org, roleB, actor, models.ManagedRoleAssignmentInput{UserID: subject, Reason: "Approved conflicting timebox", ValidFrom: &start, ExpiresAt: &end}); err != nil {
		t.Fatal(err)
	}
	rbac, err := service.NewRBACAuthorizer(pool)
	if err != nil {
		t.Fatal(err)
	}
	assertAuthority := func(allowed bool) {
		t.Helper()
		d, err := rbac.Authorize(tenant, authz.Request{OrganizationID: org, SubjectID: subject, Resource: resource, Action: "read"})
		if err != nil || d.Allowed != allowed {
			t.Fatalf("RBAC allowed=%v expected=%v err=%v", d.Allowed, allowed, err)
		}
		permissions, err := rbac.GetUserPermissions(tenant, org, subject)
		if err != nil {
			t.Fatal(err)
		}
		if (len(permissions[resource]) > 0) != allowed {
			t.Fatalf("permissions=%v expectedauthority=%v", permissions, allowed)
		}
	}
	assertAuthority(true)
	transition.ExpectedVersion = exception.Version
	if _, err := app.DecideException(tenant, org, exception.ID, reviewer, "revoked", transition); err != nil {
		t.Fatal(err)
	}
	assertAuthority(false)
	// Disabling a rule restores otherwise-valid roles; expiry still denies.
	if _, err := app.DisableSoDRule(tenant, org, rule.ID, actor, models.AccessGovernanceTransitionInput{ExpectedVersion: 1, Reason: "Rule replaced after review"}); err != nil {
		t.Fatal(err)
	}
	assertAuthority(true)
	if _, err := setWindow(roleA, subject, actor, models.ManagedRoleWindowInput{ValidFrom: time.Now(), ExpiresAt: time.Now().Add(150 * time.Millisecond), ExpectedVersion: 2, Reason: "Expiry boundary proof"}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_sleep(0.2)`); err != nil {
		t.Fatal(err)
	}
	d, err := rbac.Authorize(tenant, authz.Request{OrganizationID: org, SubjectID: subject, Resource: resource, Action: "read"})
	if err != nil || d.Allowed {
		t.Fatalf("expired assignment allowed=%v err=%v", d.Allowed, err)
	}
	// A deliberately failing outbox rolls back snapshot/campaign creation.
	failingStore, err := repository.NewAccessGovernanceRepository(pool, failingManagedRoleOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	failingApp, _ := service.NewAccessGovernanceService(failingStore, time.Now)
	before, n, err := app.ListCampaigns(tenant, org, models.PaginationRequest{Page: 1, PageSize: 20})
	_ = before
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failingApp.CreateCampaign(tenant, org, actor, in); err == nil {
		t.Fatal("failed outbox did not fail campaign")
	}
	_, afterCount, err := app.ListCampaigns(tenant, org, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || afterCount != n {
		t.Fatalf("campaign survived rollback before=%d after=%d err=%v", n, afterCount, err)
	}
	// A sole administrator cannot be timeboxed (including a future start),
	// and SoD cannot put all administrative authority on exception clocks.
	if _, err := pool.Exec(ctx, `INSERT INTO permissions(resource,action) VALUES('settings','configure') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO roles(id,organization_id,name,slug,is_system_role,is_custom,created_by) VALUES
	 ($1::uuid,$2::uuid,'Governance admin','governance-admin-'||$1::text,false,true,$3::uuid),
	 ($4::uuid,$2::uuid,'Permanent break glass','break-glass-'||$4::text,false,true,$3::uuid)`, adminRole, org, actor, backupRole); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,p.id FROM unnest($1::uuid[]) role_id CROSS JOIN permissions p WHERE p.resource='settings' AND p.action='configure'`, []string{adminRole, backupRole}); err != nil {
		t.Fatal(err)
	}
	if err := roles.AssignRole(tenant, org, adminRole, actor, models.ManagedRoleAssignmentInput{UserID: subject, Reason: "Permanent primary tenant administrator"}); err != nil {
		t.Fatal(err)
	}
	for _, windowStart := range []time.Time{time.Now(), time.Now().Add(time.Hour)} {
		if _, err := setWindow(adminRole, subject, actor, models.ManagedRoleWindowInput{ValidFrom: windowStart, ExpiresAt: windowStart.Add(time.Hour), ExpectedVersion: 1, Reason: "Sole admin must never expire"}); !errors.Is(err, service.ErrLastTenantAdministrator) {
			t.Fatalf("sole admin window err=%v", err)
		}
	}
	for _, alias := range []string{strings.ToUpper(subject), "urn:uuid:" + subject, "{" + subject + "}", strings.ReplaceAll(subject, "-", "")} {
		if _, err := setWindow(adminRole, alias, subject, models.ManagedRoleWindowInput{ValidFrom: time.Now(), ExpiresAt: time.Now().Add(time.Hour), ExpectedVersion: 1, Reason: "Aliased self approval rejected"}); !errors.Is(err, service.ErrGovernanceInvalid) {
			t.Fatalf("alias=%s err=%v", alias, err)
		}
	}
	adminRuleIn := models.AccessSoDRuleInput{Name: "Administrator and approval separation", RoleAID: adminRole, RoleBID: roleB, Reason: "Protect independent approval"}
	if _, err := app.CreateSoDRule(tenant, org, actor, adminRuleIn); err == nil {
		t.Fatal("SoD allowed sole administrators on an exception clock")
	}
	if err := roles.AssignRole(tenant, org, backupRole, actor, models.ManagedRoleAssignmentInput{UserID: reviewer, Reason: "Permanent independent break glass"}); err != nil {
		t.Fatal(err)
	}
	adminRule, err := app.CreateSoDRule(tenant, org, actor, adminRuleIn)
	if err != nil {
		t.Fatal(err)
	}
	violations, count, err := app.ListViolations(tenant, org, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || count < 1 || violations[0].SubjectID != subject {
		t.Fatalf("violation evidence=%v count=%d err=%v", violations, count, err)
	}
	adminException, err := app.RequestException(tenant, org, adminRule.ID, actor, models.AccessSoDExceptionInput{SubjectID: subject, ValidFrom: time.Now().Add(-time.Second), ExpiresAt: time.Now().Add(250 * time.Millisecond), Reason: "Expiry proof with permanent backup"})
	if err != nil {
		t.Fatal(err)
	}
	adminException, err = app.DecideException(tenant, org, adminException.ID, reviewer, "approved", models.AccessGovernanceTransitionInput{ExpectedVersion: 1, Reason: "Independent timeboxed approval"})
	if err != nil {
		t.Fatal(err)
	}
	if err := roles.UnassignRole(tenant, org, backupRole, reviewer, actor, models.ManagedRoleUnassignmentInput{Reason: "Must retain permanent break glass"}); !errors.Is(err, service.ErrLastTenantAdministrator) {
		t.Fatalf("removed future-safe backup err=%v", err)
	}
	if _, err := setWindow(backupRole, reviewer, actor, models.ManagedRoleWindowInput{ValidFrom: time.Now(), ExpiresAt: time.Now().Add(time.Hour), ExpectedVersion: 1, Reason: "Cannot put final safe admin on clock"}); !errors.Is(err, service.ErrLastTenantAdministrator) {
		t.Fatalf("timeboxed future-safe backup err=%v", err)
	}
	if _, err := conn.Exec(ctx, `UPDATE user_roles SET valid_from=statement_timestamp(),expires_at=statement_timestamp()+INTERVAL '1 hour',window_approved_by=$4::uuid,assignment_reason='Direct unsafe final admin timer' WHERE organization_id=$1::uuid AND role_id=$2::uuid AND user_id=$3::uuid`, org, backupRole, reviewer, actor); err == nil {
		t.Fatal("DB allowed final safe admin timer")
	}
	if _, err := conn.Exec(ctx, `SELECT pg_sleep(0.3)`); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		user    string
		allowed bool
	}{{subject, false}, {reviewer, true}} {
		d, err := rbac.Authorize(tenant, authz.Request{OrganizationID: org, SubjectID: tt.user, Resource: "settings", Action: "configure"})
		if err != nil || d.Allowed != tt.allowed {
			t.Fatalf("exception expiry admin=%s allowed=%v expected=%v err=%v", tt.user, d.Allowed, tt.allowed, err)
		}
	}
	if _, err := app.DecideException(tenant, org, adminException.ID, reviewer, "revoked", models.AccessGovernanceTransitionInput{ExpectedVersion: adminException.Version, Reason: "Explicit expiry closure recorded"}); err != nil {
		t.Fatal(err)
	}
}
