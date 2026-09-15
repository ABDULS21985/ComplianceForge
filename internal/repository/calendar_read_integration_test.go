package repository_test

import (
	"context"
	"encoding/json"
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

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

// TestCalendarReadNonBypass uses a real LOGIN, not SET ROLE on a superuser
// connection. The runtime has only source/authorization SELECT and durable
// decision INSERT: no calendar mutation, table ownership or bypass privileges.
func TestCalendarReadNonBypass(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var version int
	var dirty bool
	if err := admin.QueryRow(ctx, `SELECT version,dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil || version < 58 || dirty {
		t.Fatalf("clean schema >=58 required: version=%d dirty=%v err=%v", version, dirty, err)
	}
	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB, suspended := uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleA, roleB := uuid.NewString(), uuid.NewString()
	riskVisible, riskDenied, riskMasked, riskDeleted, riskB := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	runtime := "grc_calendar_read_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{runtime}.Sanitize()
	runtimeCreated := false
	createdPermissions := []string{}
	// Cleanup is exact-tenant, privileged and transactional. Disabling triggers
	// here is solely fixture disposal, never part of runtime behavior. All failures
	// are surfaced; immutable decision evidence must not leak into later gates.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		tx, err := admin.Begin(cleanupCtx)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(cleanupCtx)
		if _, err := tx.Exec(cleanupCtx, `SET LOCAL session_replication_role='replica'`); err != nil {
			t.Error(err)
			return
		}
		rows, err := tx.Query(cleanupCtx, `SELECT relation.relname,attribute.attname
			FROM pg_class relation JOIN pg_namespace namespace ON namespace.oid=relation.relnamespace
			JOIN pg_attribute attribute ON attribute.attrelid=relation.oid AND NOT attribute.attisdropped
			WHERE namespace.nspname='public' AND relation.relkind IN ('r','p')
			AND attribute.attname IN ('organization_id','tenant_id') ORDER BY relation.relname`)
		if err != nil {
			t.Error(err)
			return
		}
		type tenantTable struct{ table, column string }
		tables := []tenantTable{}
		for rows.Next() {
			var table tenantTable
			if err := rows.Scan(&table.table, &table.column); err != nil {
				rows.Close()
				t.Error(err)
				return
			}
			tables = append(tables, table)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Error(err)
			return
		}
		if _, err := tx.Exec(cleanupCtx, `DELETE FROM role_permissions WHERE role_id=ANY($1::uuid[])`, []string{roleA, roleB}); err != nil {
			t.Error(err)
			return
		}
		for _, table := range tables {
			statement := "DELETE FROM " + pgx.Identifier{table.table}.Sanitize() + " WHERE " + pgx.Identifier{table.column}.Sanitize() + "=ANY($1::uuid[])"
			if _, err := tx.Exec(cleanupCtx, statement, []string{orgA, orgB}); err != nil {
				t.Errorf("cleanup %s: %v", table.table, err)
				return
			}
		}
		if _, err := tx.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB}); err != nil {
			t.Error(err)
			return
		}
		if _, err := tx.Exec(cleanupCtx, `DELETE FROM permissions WHERE id=ANY($1::uuid[])`, createdPermissions); err != nil {
			t.Error(err)
			return
		}
		if err := tx.Commit(cleanupCtx); err != nil {
			t.Error(err)
			return
		}
		var leaked int
		if err := admin.QueryRow(cleanupCtx, `SELECT count(*) FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB}).Scan(&leaked); err != nil || leaked != 0 {
			t.Errorf("calendar fixture cleanup leaked organizations=%d err=%v", leaked, err)
		}
		if runtimeCreated {
			if _, err := admin.Exec(cleanupCtx, "DROP OWNED BY "+quoted); err != nil {
				t.Error(err)
				return
			}
			if _, err := admin.Exec(cleanupCtx, "DROP ROLE "+quoted); err != nil {
				t.Error(err)
			}
		}
	})
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO organizations(id,name,slug,status) VALUES($1,'Calendar A',$3,'active'),($2,'Calendar B',$4,'active')`, orgA, orgB, "calendar-a-"+orgA, "calendar-b-"+orgB)
	exec(`INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Calendar','Reader','active'),($4,$5,$6,'Other','Reader','active'),($7,$2,$8,'Inactive','Assignee','inactive')`,
		userA, orgA, userA+"@example.test", userB, orgB, userB+"@example.test", suspended, suspended+"@example.test")
	exec(`INSERT INTO roles(id,organization_id,name,slug,is_system_role,is_custom,created_by) VALUES
		($1,$2,'Calendar Reader',$3,false,true,$4),($5,$6,'Calendar Reader',$7,false,true,$8)`,
		roleA, orgA, "calendar-"+roleA, userA, roleB, orgB, "calendar-"+roleB, userB)
	permissionRows, err := admin.Query(ctx, `INSERT INTO permissions(resource,action,description)
		VALUES('risks','read','Calendar read acceptance'),('audits','read','Calendar read acceptance')
		ON CONFLICT(resource,action) DO NOTHING RETURNING id::text`)
	if err != nil {
		t.Fatal(err)
	}
	for permissionRows.Next() {
		var id string
		if err := permissionRows.Scan(&id); err != nil {
			permissionRows.Close()
			t.Fatal(err)
		}
		createdPermissions = append(createdPermissions, id)
	}
	permissionRows.Close()
	if err := permissionRows.Err(); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO role_permissions(role_id,permission_id)
		SELECT role_id,permission.id FROM unnest($1::uuid[]) role_id CROSS JOIN permissions permission
		WHERE permission.resource IN ('risks','audits') AND permission.action='read'`, []string{roleA, roleB})
	exec(`INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by) VALUES($1,$2,$3,$1),($4,$5,$6,$4)`, userA, roleA, orgA, userB, roleB, orgB)
	for index, id := range []string{riskVisible, riskDenied, riskMasked, riskDeleted, riskB} {
		org, owner := orgA, userA
		if id == riskB {
			org, owner = orgB, userB
		}
		exec(`INSERT INTO risks(id,organization_id,risk_ref,title,owner_user_id) VALUES($1,$2,$3,'Calendar source',$4)`, id, org, "RSK-CALENDAR-"+string(rune('A'+index)), owner)
	}
	exec(`UPDATE risks SET deleted_at=NOW() WHERE id=$1`, riskDeleted)
	addEvent := func(org, source, sourceType, eventType, title, date, status, assignee string) {
		t.Helper()
		exec(`INSERT INTO calendar_events(id,organization_id,event_ref,title,description,event_type,category,priority,source_entity_type,source_entity_id,start_date,status,assigned_to)
			VALUES($1,$2,$3,$4,'Source detail',$5,'risk','high',$6,$7,$8,$9,NULLIF($10,'')::uuid)`,
			uuid.NewString(), org, "CEV-"+uuid.NewString()[:18], title, eventType, sourceType, source, date, status, assignee)
	}
	addEvent(orgA, riskVisible, "risk", "risk_review", "Visible %_ deadline", "2026-09-15", "upcoming", userA)
	addEvent(orgA, riskVisible, "risk", "risk_reassessment", "Future", "2026-09-20", "overdue", suspended)
	addEvent(orgA, riskVisible, "risk", "risk_treatment_due", "Past", "2026-09-10", "upcoming", userB)
	addEvent(orgA, riskDenied, "risk", "risk_review", "Denied source", "2026-09-16", "upcoming", "")
	addEvent(orgA, riskMasked, "risk", "risk_review", "Sensitive source title", "2026-09-17", "upcoming", "")
	addEvent(orgA, riskDeleted, "risk", "risk_review", "Deleted source", "2026-09-18", "upcoming", "")
	addEvent(orgA, riskB, "risk", "risk_review", "Cross tenant source", "2026-09-19", "upcoming", "")
	addEvent(orgA, uuid.NewString(), "unsupported", "custom", "Unanchored custom", "2026-09-19", "upcoming", "")
	addEvent(orgB, riskB, "risk", "risk_review", "Tenant B only", "2026-09-15", "upcoming", userB)

	password := uuid.NewString() + uuid.NewString()
	// Password consists only of cryptographically generated UUID characters.
	exec("CREATE ROLE " + quoted + " LOGIN NOSUPERUSER NOBYPASSRLS NOINHERIT NOCREATEDB NOCREATEROLE PASSWORD '" + password + "'")
	runtimeCreated = true
	exec("GRANT USAGE ON SCHEMA public TO " + quoted)
	tables := []string{"calendar_events", "users", "organizations", "risks", "policies", "audits", "vendors", "incidents", "assets", "control_implementations", "control_evidence", "roles", "user_roles", "effective_user_roles", "role_permissions", "permissions", "access_policies", "access_policy_assignments", "directory_group_memberships", "directory_groups", "access_sod_rules", "access_sod_exceptions", "field_level_permissions", "user_entity_permissions"}
	for _, table := range tables {
		exec("GRANT SELECT ON " + pgx.Identifier{table}.Sanitize() + " TO " + quoted)
	}
	exec("GRANT INSERT ON access_audit_log TO " + quoted)
	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	configuration.ConnConfig.User, configuration.ConnConfig.Password = runtime, password
	configuration.MaxConns = 3
	configuration.AfterConnect = nil
	runtimePool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimePool.Close)
	conn, err := runtimePool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Release)
	var unsafe bool
	if err := conn.QueryRow(ctx, `SELECT current_user<>$1 OR session_user<>$1 OR rolsuper OR rolbypassrls
		OR EXISTS(SELECT 1 FROM pg_class WHERE relowner=(SELECT oid FROM pg_roles WHERE rolname=current_user))
		OR EXISTS(SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname=current_user))
		FROM pg_roles WHERE rolname=current_user`, runtime).Scan(&unsafe); err != nil || unsafe {
		t.Fatalf("unsafe LOGIN identity=%v err=%v", unsafe, err)
	}
	for _, table := range []string{"calendar_events", "users", "risks", "policies", "audits", "vendors", "incidents", "assets", "control_implementations", "control_evidence"} {
		var forced bool
		if err := admin.QueryRow(ctx, `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid=$1::regclass`, table).Scan(&forced); err != nil || !forced {
			t.Fatalf("source %s is not FORCE RLS: %v", table, err)
		}
	}
	setTenant := func(org string) context.Context {
		t.Helper()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, org); err != nil {
			t.Fatal(err)
		}
		return database.WithQuerier(ctx, conn)
	}
	tenantA := setTenant(orgA)
	var leaked int
	if err := conn.QueryRow(tenantA, `SELECT count(*) FROM calendar_events WHERE organization_id=$1`, orgB).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("cross-tenant direct read leaked=%d err=%v", leaked, err)
	}
	if _, err := conn.Exec(tenantA, `UPDATE calendar_events SET title='forbidden' WHERE organization_id=$1`, orgA); err == nil {
		t.Fatal("read runtime unexpectedly mutated calendar")
	}
	store, err := repository.NewCalendarReadRepository(runtimePool)
	if err != nil {
		t.Fatal(err)
	}
	policyStore, err := repository.NewPolicyAccessRepository(runtimePool)
	if err != nil {
		t.Fatal(err)
	}
	rbac, err := service.NewRBACAuthorizer(runtimePool)
	if err != nil {
		t.Fatal(err)
	}
	authorizer, err := service.NewPolicyBasedAuthorizer(rbac, policyStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	app, err := service.NewComplianceCalendarService(store, authorizer, service.WithComplianceCalendarClock(func() time.Time {
		return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Build source-specific real ABAC policies with the privileged fixture pool.
	// The actual reads and durable decision writes below use the narrow LOGIN.
	fixtureStore, err := repository.NewPolicyAccessRepository(admin)
	if err != nil {
		t.Fatal(err)
	}
	fixtureService, err := service.NewPolicyAccessService(fixtureStore, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	fixtureConn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixtureConn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); err != nil {
		fixtureConn.Release()
		t.Fatal(err)
	}
	fixtureCtx := database.WithQuerier(ctx, fixtureConn)
	for _, scenario := range []struct {
		name, source string
		effect       models.AccessPolicyEffect
		priority     int
		masked       bool
	}{{"Allow risk calendar", "", models.AccessPolicyEffectAllow, 100, false}, {"Deny one risk", riskDenied, models.AccessPolicyEffectDeny, 10, false}, {"Mask one risk", riskMasked, models.AccessPolicyEffectAllow, 20, true}} {
		conditions := []models.AccessCondition{}
		if scenario.source != "" {
			value, _ := json.Marshal(scenario.source)
			conditions = append(conditions, models.AccessCondition{Attribute: "id", Operator: models.AccessOperatorEquals, Value: value})
		}
		policy, err := fixtureService.CreatePolicy(fixtureCtx, orgA, userA, uuid.NewString(), models.AccessPolicyInput{
			Name: scenario.name, Priority: scenario.priority, Effect: scenario.effect, IsActive: true,
			SubjectConditions: []models.AccessCondition{}, EnvironmentConditions: []models.AccessCondition{},
			ResourceType: "risks", ResourceConditions: conditions, Actions: []string{"read"}, Reason: "Calendar isolation acceptance fixture",
		})
		if err != nil {
			fixtureConn.Release()
			t.Fatal(err)
		}
		if _, err := fixtureService.CreateAssignment(fixtureCtx, orgA, policy.ID, userA, uuid.NewString(), models.AccessPolicyAssignmentInput{
			AssigneeType: models.AccessAssigneeAllUsers, Reason: "Bound calendar source collection",
		}); err != nil {
			fixtureConn.Release()
			t.Fatal(err)
		}
		if scenario.masked {
			if _, err := fixtureService.UpsertFieldPermission(fixtureCtx, orgA, policy.ID, userA, uuid.NewString(), models.AccessFieldPermissionInput{
				ResourceType: "risks", FieldPath: "title", Classification: models.AccessFieldConfidential,
				Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskRedact, Reason: "Source-derived text cannot bypass field controls",
			}); err != nil {
				fixtureConn.Release()
				t.Fatal(err)
			}
		}
	}
	if _, err := fixtureConn.Exec(ctx, `RESET app.current_tenant`); err != nil {
		fixtureConn.Release()
		t.Fatal(err)
	}
	fixtureConn.Release()
	query := models.CalendarEventQuery{StartDate: "2026-09-01", EndDate: "2026-09-30", PageSize: 1}
	access := models.CalendarAccessContext{RequestID: uuid.NewString(), IPAddress: "192.0.2.1", MFAVerified: true}
	collection, err := authorizer.Authorize(tenantA, authz.Request{SubjectID: userA, OrganizationID: orgA, Resource: "audits", Action: "read"})
	if err != nil || !collection.Allowed {
		t.Fatalf("real composite collection gate=%#v err=%v", collection, err)
	}
	page, err := app.ListEvents(tenantA, orgA, userA, query, access)
	if err != nil || page.Pagination.TotalItems != 3 || len(page.Data) != 1 || page.Data[0].Title != "Past" || page.Data[0].AssignedTo != nil {
		t.Fatalf("permission-filtered page=%#v err=%v", page, err)
	}
	summary, err := app.Summary(tenantA, orgA, userA, query, access)
	if err != nil || summary.Data.TotalEvents != 3 || summary.Data.OverdueEvents != 1 || summary.Data.DueTodayEvents != 1 || summary.Data.UpcomingEvents != 1 {
		t.Fatalf("permission-filtered summary=%#v err=%v", summary, err)
	}
	upcoming, err := app.Upcoming(tenantA, orgA, userA, models.CalendarEventQuery{}, access)
	if err != nil || upcoming.Pagination.TotalItems != 2 || upcoming.Data[1].AssignedTo != nil || upcoming.Data[1].Status != "upcoming" {
		t.Fatalf("upcoming/active assignee=%#v err=%v", upcoming, err)
	}
	overdue, err := app.Overdue(tenantA, orgA, userA, models.CalendarEventQuery{}, access)
	if err != nil || overdue.Pagination.TotalItems != 1 || overdue.Data[0].Status != "overdue" {
		t.Fatalf("derived overdue=%#v err=%v", overdue, err)
	}
	query.PageSize, query.AssignedTo = 100, suspended
	assigned, err := app.ListEvents(tenantA, orgA, userA, query, access)
	if err != nil || assigned.Pagination.TotalItems != 0 {
		t.Fatalf("inactive assigned-user filter=%#v err=%v", assigned, err)
	}
	query.AssignedTo, query.Search = "", "%_"
	searched, err := app.ListEvents(tenantA, orgA, userA, query, access)
	if err != nil || searched.Pagination.TotalItems != 1 {
		t.Fatalf("search wildcards must be literal: page=%#v err=%v", searched, err)
	}
	if _, err := app.ListEvents(tenantA, orgB, userB, models.CalendarEventQuery{}, access); !errors.Is(err, service.ErrComplianceCalendarScope) {
		t.Fatalf("cross-context tenant bypass: %v", err)
	}
	if _, err := app.ListEvents(ctx, orgA, userA, models.CalendarEventQuery{}, access); !errors.Is(err, service.ErrComplianceCalendarScope) {
		t.Fatalf("unscoped pool fallback: %v", err)
	}
	withoutTenant := setTenant("")
	if _, err := app.ListEvents(withoutTenant, orgA, userA, models.CalendarEventQuery{}, access); !errors.Is(err, service.ErrComplianceCalendarScope) {
		t.Fatalf("unset database tenant bypass: %v", err)
	}
	tenantB := setTenant(orgB)
	other, err := app.ListEvents(tenantB, orgB, userB, models.CalendarEventQuery{}, access)
	if err != nil || other.Pagination.TotalItems != 1 || other.Data[0].Title != "Tenant B only" {
		t.Fatalf("tenant B isolation=%#v err=%v", other, err)
	}
	var concurrent sync.WaitGroup
	failures := make(chan error, 2)
	for _, principal := range []struct {
		org, user string
		total     int
	}{{orgA, userA, 3}, {orgB, userB, 1}} {
		concurrent.Add(1)
		go func(org, user string, total int) {
			defer concurrent.Done()
			for iteration := 0; iteration < 5; iteration++ {
				if err := database.WithTenantConnection(ctx, runtimePool, org, func(scoped context.Context) error {
					result, err := app.ListEvents(scoped, org, user, models.CalendarEventQuery{}, access)
					if err != nil {
						return err
					}
					if result.Pagination.TotalItems != total {
						return fmt.Errorf("concurrent tenant total=%d want=%d", result.Pagination.TotalItems, total)
					}
					for _, event := range result.Data {
						if event.OrganizationID != org {
							return errors.New("concurrent calendar tenant projection leaked")
						}
					}
					return nil
				}); err != nil {
					failures <- err
					return
				}
			}
		}(principal.org, principal.user, principal.total)
	}
	concurrent.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	tenantA = setTenant(orgA)
	// A true SQL limit+1 proof: more live candidates must return an explicit
	// unavailable error, not a plausible but incomplete page or summary total.
	exec(`WITH inserted AS (
		INSERT INTO risks(organization_id,risk_ref,title,owner_user_id)
		SELECT $1::uuid,'RSK-CAP-'||number::text,'Capacity source',$2::uuid
		FROM generate_series(1,$3::int) number RETURNING id
	) INSERT INTO calendar_events(organization_id,event_ref,title,event_type,category,source_entity_type,source_entity_id,start_date)
	SELECT $1::uuid,'CAP-'||right(id::text,20),'Capacity event','custom','custom','risk',id,'2026-09-15'::date FROM inserted`, orgA, userA, models.CalendarMaximumCandidates+1)
	limited, err := app.ListEvents(tenantA, orgA, userA, models.CalendarEventQuery{EventType: "custom"}, access)
	if !errors.Is(err, service.ErrComplianceCalendarUnavailable) || limited != nil {
		t.Fatalf("live candidate limit produced a partial page=%#v err=%v", limited, err)
	}
	exec(`UPDATE risks SET deleted_at=NOW() WHERE id=$1`, riskVisible)
	deleted, err := app.ListEvents(tenantA, orgA, userA, models.CalendarEventQuery{EventType: "risk_review"}, access)
	if err != nil || deleted.Pagination.TotalItems != 0 {
		t.Fatalf("live source deletion left projected history=%#v err=%v", deleted, err)
	}
	exec(`UPDATE users SET status='inactive' WHERE id=$1`, userA)
	if _, err := app.ListEvents(tenantA, orgA, userA, models.CalendarEventQuery{}, access); !errors.Is(err, service.ErrComplianceCalendarScope) {
		t.Fatalf("inactive principal admitted: %v", err)
	}
	exec(`UPDATE users SET status='active' WHERE id=$1`, userA)
	exec(`UPDATE organizations SET status='suspended' WHERE id=$1`, orgA)
	if _, err := app.ListEvents(tenantA, orgA, userA, models.CalendarEventQuery{}, access); !errors.Is(err, service.ErrComplianceCalendarScope) {
		t.Fatalf("inactive organization admitted: %v", err)
	}
}
