//go:build integration

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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

// TestOrganizationProfileActualLogin exercises the request-scoped repository
// through an independently authenticated, non-owner, non-bypass LOGIN. The
// privileged pool is restricted to fixture creation, inspection and disposal.
func TestOrganizationProfileActualLogin(t *testing.T) {
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
	if err := admin.QueryRow(ctx, `SELECT version,dirty FROM public.schema_migrations`).Scan(&version, &dirty); err != nil || version < 59 || dirty {
		t.Fatalf("clean schema >=59 required: version=%d dirty=%v err=%v", version, dirty, err)
	}
	orgA, orgB := uuid.NewString(), uuid.NewString()
	actorA, actorB, inactiveA := uuid.NewString(), uuid.NewString(), uuid.NewString()
	requestID := uuid.NewString()
	runtime := "grc_profile_login_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{runtime}.Sanitize()
	roleCreated := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		for _, statement := range []string{
			`DELETE FROM public.audit_logs WHERE organization_id=ANY($1::uuid[])`,
			`DELETE FROM public.organizations WHERE id=ANY($1::uuid[])`,
		} {
			if _, err := admin.Exec(cleanupCtx, statement, []string{orgA, orgB}); err != nil {
				t.Errorf("exact profile fixture cleanup failed: %v", err)
			}
		}
		var remaining int
		if err := admin.QueryRow(cleanupCtx, `SELECT count(*) FROM public.organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB}).Scan(&remaining); err != nil || remaining != 0 {
			t.Errorf("profile fixture organizations remaining=%d err=%v", remaining, err)
		}
		if roleCreated {
			if _, err := admin.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole); err != nil {
				t.Error(err)
				return
			}
			if _, err := admin.Exec(cleanupCtx, "DROP ROLE "+quotedRole); err != nil {
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
	privateSentinel := "PROFILE_PRIVATE_SETTINGS_SENTINEL_84621"
	exec(`INSERT INTO public.organizations
		(id,name,slug,legal_name,industry,country_code,status,tier,settings,branding,metadata)
		VALUES($1,'Profile A',$3,'Profile A Legal','Services','NG','active','enterprise',
		       jsonb_build_object('secret',$5::text),jsonb_build_object('secret',$5::text),jsonb_build_object('secret',$5::text)),
		      ($2,'Profile B',$4,'Profile B Legal','Research','GB','active','enterprise','{}','{}','{}')`,
		orgA, orgB, "profile-a-"+orgA, "profile-b-"+orgB, privateSentinel)
	exec(`INSERT INTO public.users(id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Profile','Administrator','active'),($4,$5,$6,'Other','Administrator','active'),
		($7,$2,$8,'Inactive','Administrator','inactive')`,
		actorA, orgA, actorA+"@example.test", actorB, orgB, actorB+"@example.test", inactiveA, inactiveA+"@example.test")
	password := uuid.NewString() + uuid.NewString()
	// Generated fixture credentials have only UUID characters; no operator
	// credential, user input or production DSN is interpolated into this DDL.
	exec("CREATE ROLE " + quotedRole + " LOGIN NOSUPERUSER NOBYPASSRLS NOINHERIT NOCREATEDB NOCREATEROLE PASSWORD '" + password + "'")
	roleCreated = true
	exec("GRANT USAGE ON SCHEMA public TO " + quotedRole)
	exec("GRANT SELECT ON public.organizations,public.users,public.audit_logs TO " + quotedRole)
	exec("GRANT UPDATE ON public.organizations TO " + quotedRole)
	exec("GRANT INSERT ON public.audit_logs TO " + quotedRole)
	exec("GRANT EXECUTE ON FUNCTION public.get_current_tenant(),public.valid_organization_profile_contacts(jsonb) TO " + quotedRole)
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.User, config.ConnConfig.Password = runtime, password
	config.MaxConns = 4
	config.AfterConnect = nil
	delete(config.ConnConfig.RuntimeParams, "role")
	delete(config.ConnConfig.RuntimeParams, "options")
	runtimePool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimePool.Close)
	conn, err := runtimePool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	err = conn.QueryRow(ctx, `SELECT current_user<>$1 OR session_user<>$1 OR rolsuper OR rolbypassrls
		OR EXISTS(SELECT 1 FROM pg_class WHERE relowner=(SELECT oid FROM pg_roles WHERE rolname=current_user))
		OR EXISTS(SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname=current_user))
		FROM pg_roles WHERE rolname=current_user`, runtime).Scan(&unsafe)
	conn.Release()
	if err != nil || unsafe {
		t.Fatalf("unsafe authenticated profile LOGIN=%v err=%v", unsafe, err)
	}
	for _, table := range []string{"organizations", "users", "audit_logs"} {
		var forced bool
		if err := admin.QueryRow(ctx, `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid=$1::regclass`, "public."+table).Scan(&forced); err != nil || !forced {
			t.Fatalf("profile relation %s lacks FORCE RLS: %v", table, err)
		}
	}
	store, err := repository.NewOrganizationProfileRepository(runtimePool)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewOrganizationProfileService(store)
	if err != nil {
		t.Fatal(err)
	}
	get := func(tenant, organization, actor string) (*models.OrganizationProfile, error) {
		var profile *models.OrganizationProfile
		err := database.WithTenantConnection(ctx, runtimePool, tenant, func(scoped context.Context) error {
			var err error
			profile, err = svc.GetProfile(scoped, organization, actor)
			return err
		})
		return profile, err
	}
	update := func(tenant, organization, actor string, input models.OrganizationProfileUpdateInput) (*models.OrganizationProfile, error) {
		var profile *models.OrganizationProfile
		err := database.WithTenantConnection(ctx, runtimePool, tenant, func(scoped context.Context) error {
			var err error
			profile, err = svc.UpdateProfile(scoped, organization, actor, requestID, input)
			return err
		})
		return profile, err
	}
	stringPointer := func(value string) *string { return &value }
	intPointer := func(value int) *int { return &value }
	input := func(version int64, name string) models.OrganizationProfileUpdateInput {
		return models.OrganizationProfileUpdateInput{ExpectedVersion: version, Reason: "Approved fixture profile review", Name: stringPointer(name)}
	}
	auditCount := func() int {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM public.audit_logs WHERE organization_id=$1 AND action='ORGANIZATION_PROFILE_UPDATED'`, orgA).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	initial, err := get(orgA, orgA, actorA)
	if err != nil || initial == nil || initial.Version != 1 || initial.Status != "active" || initial.Tier != "enterprise" || initial.FiscalYearStartMonth != 1 || len(initial.Contacts) != 0 {
		t.Fatalf("initial profile projection invalid: %+v err=%v", initial, err)
	}
	encoded, err := json.Marshal(initial)
	if err != nil || strings.Contains(string(encoded), privateSentinel) || strings.Contains(string(encoded), "settings") || strings.Contains(string(encoded), "metadata") || strings.Contains(string(encoded), "branding") {
		t.Fatalf("profile leaked unreviewed configuration: err=%v", err)
	}
	if _, err := svc.GetProfile(ctx, orgA, actorA); !errors.Is(err, models.ErrOrganizationProfileScope) {
		t.Fatalf("missing request executor used pool fallback: %v", err)
	}
	for _, scope := range []struct{ tenant, organization, actor string }{
		{orgA, orgB, actorB}, {orgA, orgA, actorB}, {orgA, orgA, inactiveA}, {orgB, orgA, actorA}, {"", orgA, actorA},
	} {
		if _, err := get(scope.tenant, scope.organization, scope.actor); !errors.Is(err, models.ErrOrganizationProfileNotFound) {
			t.Fatalf("invalid tenant/actor read was not denied: %v", err)
		}
		if _, err := update(scope.tenant, scope.organization, scope.actor, input(1, "Forbidden profile")); !errors.Is(err, models.ErrOrganizationProfileNotFound) {
			t.Fatalf("invalid tenant/actor write was not denied: %v", err)
		}
	}
	if auditCount() != 0 {
		t.Fatal("denied scope produced profile audit events")
	}
	contacts := []models.OrganizationContact{{Purpose: "security", Email: "security@example.test"}, {Purpose: "privacy", Email: "privacy@example.test"}}
	languages := []string{"fr", "en"}
	change := input(1, "Reviewed Profile A")
	change.Contacts, change.SupportedLanguages = &contacts, &languages
	change.Timezone, change.DefaultLanguage = stringPointer("Africa/Lagos"), stringPointer("fr")
	change.FiscalYearStartMonth, change.FiscalYearStartDay = intPointer(4), intPointer(1)
	updated, err := update(orgA, orgA, actorA, change)
	if err != nil || updated == nil || updated.Version != 2 || updated.Name != "Reviewed Profile A" || updated.LegalName != "Profile A Legal" ||
		updated.Slug != initial.Slug || updated.Tier != initial.Tier || updated.Status != initial.Status || updated.Timezone != "Africa/Lagos" ||
		updated.FiscalYearStartMonth != 4 || updated.FiscalYearStartDay != 1 || updated.Contacts[0].Purpose != "privacy" || auditCount() != 1 {
		t.Fatalf("atomic presence-aware profile update invalid: %+v err=%v", updated, err)
	}
	var auditRequest, action, entityType, entityID string
	var auditMetadata []byte
	if err := admin.QueryRow(ctx, `SELECT request_id::text,action,entity_type,entity_id::text,metadata FROM public.audit_logs
		WHERE organization_id=$1 AND action='ORGANIZATION_PROFILE_UPDATED'`, orgA).Scan(&auditRequest, &action, &entityType, &entityID, &auditMetadata); err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		SchemaVersion   int      `json:"schema_version"`
		PreviousVersion int64    `json:"previous_version"`
		Version         int64    `json:"version"`
		Reason          string   `json:"reason"`
		ChangedFields   []string `json:"changed_fields"`
	}
	if json.Unmarshal(auditMetadata, &metadata) != nil || metadata.SchemaVersion != 1 || metadata.PreviousVersion != 1 || metadata.Version != 2 ||
		metadata.Reason != change.Reason || len(metadata.ChangedFields) != 6 || auditRequest != requestID || action != "ORGANIZATION_PROFILE_UPDATED" || entityType != "organizations" || entityID != orgA ||
		strings.Contains(string(auditMetadata), "@example.test") || strings.Contains(string(auditMetadata), privateSentinel) || strings.Contains(string(auditMetadata), "Reviewed Profile A") {
		t.Fatalf("profile audit contains invalid contract or duplicate sensitive values: %s", auditMetadata)
	}
	var settings, branding, rawMetadata []byte
	if err := admin.QueryRow(ctx, `SELECT settings,branding,metadata FROM public.organizations WHERE id=$1`, orgA).Scan(&settings, &branding, &rawMetadata); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{settings, branding, rawMetadata} {
		if !strings.Contains(string(raw), privateSentinel) {
			t.Fatal("profile update overwrote unrelated raw configuration")
		}
	}
	if _, err := update(orgA, orgA, actorA, input(1, "Stale write")); !errors.Is(err, models.ErrOrganizationProfileConflict) || auditCount() != 1 {
		t.Fatalf("stale profile update had effects: %v", err)
	}

	// A real permission failure after UPDATE must roll the transaction back:
	// the profile version and fields cannot commit without the audit INSERT.
	exec("REVOKE INSERT ON public.audit_logs FROM " + quotedRole)
	if _, err := update(orgA, orgA, actorA, input(2, "Must roll back")); !errors.Is(err, models.ErrOrganizationProfileUnavailable) {
		t.Fatalf("audit failure did not fail closed: %v", err)
	}
	afterFailure, err := get(orgA, orgA, actorA)
	if err != nil || afterFailure.Version != 2 || afterFailure.Name != updated.Name || auditCount() != 1 {
		t.Fatalf("unaudited profile update committed: %+v err=%v", afterFailure, err)
	}
	exec("GRANT INSERT ON public.audit_logs TO " + quotedRole)

	// Separate authenticated connections race the same CAS version. Regardless
	// of whether their initial reads overlap, exactly one write can commit.
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, name := range []string{"Concurrent review one", "Concurrent review two"} {
		workers.Add(1)
		go func(name string) {
			defer workers.Done()
			<-start
			_, err := update(orgA, orgA, actorA, input(2, name))
			results <- err
		}(name)
	}
	close(start)
	workers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, models.ErrOrganizationProfileConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent profile result: %v", err)
		}
	}
	afterRace, err := get(orgA, orgA, actorA)
	if err != nil || successes != 1 || conflicts != 1 || afterRace.Version != 3 || auditCount() != 2 {
		t.Fatalf("CAS race successes=%d conflicts=%d profile=%+v audits=%d err=%v", successes, conflicts, afterRace, auditCount(), err)
	}

	expectSQLFailure := func(statement, code string, args ...any) {
		t.Helper()
		err := database.WithTenantConnection(ctx, runtimePool, orgA, func(scoped context.Context) error {
			_, err := database.QuerierFromContext(scoped, nil).Exec(scoped, statement, args...)
			return err
		})
		var pgError *pgconn.PgError
		if !errors.As(err, &pgError) || pgError.Code != code {
			t.Fatalf("database guard expected SQLSTATE %s, got %v", code, err)
		}
	}
	expectSQLFailure(`UPDATE public.organizations SET profile_version=1 WHERE id=$1`, "23514", orgA)
	expectSQLFailure(`UPDATE public.organizations SET id=$2 WHERE id=$1`, "23514", orgA, uuid.NewString())
	expectSQLFailure(`UPDATE public.organizations SET fiscal_year_start_month=2,fiscal_year_start_day=29 WHERE id=$1`, "23514", orgA)
	for _, contacts := range []string{
		`{}`, `[null]`, `[{"purpose":"security","email":"a@example.test","secret":"ignored"}]`,
		`[{"purpose":"security","email":"a@example.test"},{"purpose":"security","email":"b@example.test"}]`,
		`[{"purpose":"billing","email":"bad address@example.test"}]`,
	} {
		expectSQLFailure(`UPDATE public.organizations SET enterprise_contacts=$2::jsonb WHERE id=$1`, "23514", orgA, contacts)
	}
	if auditCount() != 2 {
		t.Fatal("failed direct database guards emitted profile API audit events")
	}
	// The v59 trigger also versions an existing non-profile writer. It does not
	// claim to audit direct SQL; profile API audit remains its separate contract.
	err = database.WithTenantConnection(ctx, runtimePool, orgA, func(scoped context.Context) error {
		_, err := database.QuerierFromContext(scoped, nil).Exec(scoped, `UPDATE public.organizations SET industry='Legacy writer' WHERE id=$1`, orgA)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	afterLegacy, err := get(orgA, orgA, actorA)
	if err != nil || afterLegacy.Version != 4 || afterLegacy.Industry != "Legacy writer" || auditCount() != 2 {
		t.Fatalf("legacy writer did not participate in profile CAS: %+v err=%v", afterLegacy, err)
	}
	if _, err := update(orgA, orgA, actorA, input(3, "Pre-legacy stale write")); !errors.Is(err, models.ErrOrganizationProfileConflict) {
		t.Fatalf("legacy write failed to invalidate stale profile: %v", err)
	}
	exec(`UPDATE public.users SET status='inactive' WHERE id=$1`, actorA)
	if _, err := update(orgA, orgA, actorA, input(4, "Inactive actor")); !errors.Is(err, models.ErrOrganizationProfileNotFound) {
		t.Fatalf("inactive actor changed profile: %v", err)
	}
	exec(`UPDATE public.users SET status='active' WHERE id=$1`, actorA)
	exec(`UPDATE public.organizations SET status='suspended' WHERE id=$1`, orgB)
	if _, err := get(orgB, orgB, actorB); !errors.Is(err, models.ErrOrganizationProfileNotFound) {
		t.Fatalf("suspended tenant profile was readable: %v", err)
	}
	if _, err := update(orgB, orgB, actorB, input(2, "Suspended tenant")); !errors.Is(err, models.ErrOrganizationProfileNotFound) {
		t.Fatalf("suspended tenant changed profile: %v", err)
	}

	// The privileged fixture owner sets the exhaustion boundary transactionally.
	// The trigger is re-enabled before commit, and rollback restores its state on
	// any setup failure. Runtime access never disables or owns this trigger.
	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	for _, statement := range []string{
		`ALTER TABLE public.organizations DISABLE TRIGGER organizations_profile_version_guard`,
		fmt.Sprintf(`UPDATE public.organizations SET profile_version=%d WHERE id='%s'::uuid`, models.OrganizationProfileMaximumVersion, orgA),
		`ALTER TABLE public.organizations ENABLE TRIGGER organizations_profile_version_guard`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	expectSQLFailure(`UPDATE public.organizations SET name='Exhausted direct write' WHERE id=$1`, "22003", orgA)
	if _, err := update(orgA, orgA, actorA, input(models.OrganizationProfileMaximumVersion, "Exhausted API write")); !errors.Is(err, models.ErrOrganizationProfileUnavailable) {
		t.Fatalf("exhausted version did not fail closed: %v", err)
	}
	exhausted, err := get(orgA, orgA, actorA)
	if err != nil || exhausted.Version != models.OrganizationProfileMaximumVersion || exhausted.Name != afterRace.Name || auditCount() != 2 {
		t.Fatalf("exhausted version mutated profile: %+v err=%v", exhausted, err)
	}
	for range 4 {
		connection, err := runtimePool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var tenant *string
		err = connection.QueryRow(ctx, `SELECT public.get_current_tenant()::text`).Scan(&tenant)
		connection.Release()
		if err != nil || tenant != nil {
			t.Fatalf("request tenant scope leaked into returned pool connection: %v err=%v", tenant, err)
		}
	}
}
