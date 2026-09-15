//go:build integration

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

// TestDataQualitySnapshotLive never operates on a production database. The
// administrator creates isolated legacy-corruption fixtures; every diagnostic
// read authenticates as a new NOSUPERUSER/NOBYPASSRLS API-group LOGIN.
func TestDataQualitySnapshotLive(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" || os.Getenv("DATA_QUALITY_TEST_CONFIRM_DISPOSABLE") != "yes" {
		t.Skip("disposable TEST_DATABASE_URL and DATA_QUALITY_TEST_CONFIRM_DISPOSABLE=yes are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var version int64
	var dirty bool
	if err := admin.QueryRow(ctx, `SELECT version,dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil || version != database.SupportedSchemaVersion || dirty {
		t.Fatalf("fixture requires current clean supported schema: version=%d dirty=%v error=%v", version, dirty, err)
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	role := "cf_data_quality_" + suffix
	password := uuid.NewString() + uuid.NewString()
	var createLogin string
	if err := admin.QueryRow(ctx, `SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION INHERIT', $1::text,$2::text)`, role, password).Scan(&createLogin); err != nil {
		t.Fatal("could not prepare isolated runtime login")
	}
	if _, err := admin.Exec(ctx, createLogin); err != nil {
		t.Fatal("could not create isolated runtime login")
	}
	ids := make(map[string]string)
	id := func(name string) string {
		if ids[name] == "" {
			ids[name] = uuid.NewString()
		}
		return ids[name]
	}
	organizationIDs := []string{id("org-a"), id("org-b"), id("org-empty"), id("org-inactive"), id("org-deleted")}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		transaction, err := admin.Begin(cleanupCtx)
		if err != nil {
			t.Error("could not begin fixture cleanup")
			return
		}
		defer func() { _ = transaction.Rollback(context.Background()) }()
		// Explicit targets, not cascade assumptions while replica mode suppresses
		// FK triggers. Only this test's organizations/globals are removed.
		if _, err := transaction.Exec(cleanupCtx, `SET LOCAL session_replication_role=replica`); err != nil {
			t.Error("could not enter isolated fixture cleanup")
			return
		}
		for _, table := range []string{
			"directory_group_memberships", "directory_groups", "user_roles", "roles", "notifications", "notification_channels",
			"assets", "vendors", "audit_findings", "audits", "risk_indicator_values", "risk_indicators", "risk_treatments", "risk_assessments", "risks",
			"policy_approval_steps", "policy_approval_workflows", "policy_versions", "policies", "control_implementations", "organization_frameworks", "framework_controls", "compliance_frameworks",
			"users", "organizations",
		} {
			column := "organization_id"
			if table == "framework_controls" {
				if _, err := transaction.Exec(cleanupCtx, `DELETE FROM framework_controls WHERE framework_id=ANY($1::uuid[])`, []string{id("framework-global"), id("framework-other"), id("framework-b")}); err != nil {
					t.Errorf("could not clean fixture controls: %v", err)
					return
				}
				continue
			}
			if table == "organizations" {
				column = "id"
			}
			query := "DELETE FROM " + pgx.Identifier{table}.Sanitize() + " WHERE " + pgx.Identifier{column}.Sanitize() + "=ANY($1::uuid[])"
			if _, err := transaction.Exec(cleanupCtx, query, organizationIDs); err != nil {
				t.Errorf("could not clean fixture table %s: %v", table, err)
				return
			}
		}
		if _, err := transaction.Exec(cleanupCtx, `DELETE FROM compliance_frameworks WHERE id=ANY($1::uuid[])`, []string{id("framework-global"), id("framework-other")}); err != nil {
			t.Error("could not clean global fixture frameworks")
			return
		}
		if _, err := transaction.Exec(cleanupCtx, `DELETE FROM roles WHERE id=ANY($1::uuid[])`, []string{id("role-system"), id("role-false-global")}); err != nil {
			t.Error("could not clean global fixture roles")
			return
		}
		if _, err := transaction.Exec(cleanupCtx, `SET LOCAL session_replication_role=origin`); err != nil {
			t.Error("could not restore fixture replication setting")
			return
		}
		if err := transaction.Commit(cleanupCtx); err != nil {
			t.Errorf("could not commit fixture cleanup: %v", err)
			return
		}
		if _, err := admin.Exec(cleanupCtx, "DROP ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Errorf("could not remove isolated runtime login: %v", err)
			return
		}
		var organizations, logins, globals int
		if err := admin.QueryRow(cleanupCtx, `SELECT
			(SELECT count(*) FROM organizations WHERE id=ANY($1::uuid[])),
			(SELECT count(*) FROM pg_roles WHERE rolname=$2),
			(SELECT count(*) FROM compliance_frameworks WHERE id=ANY($3::uuid[])) +
			(SELECT count(*) FROM roles WHERE id=ANY($4::uuid[]))`, organizationIDs, role,
			[]string{id("framework-global"), id("framework-other")}, []string{id("role-system"), id("role-false-global")}).Scan(&organizations, &logins, &globals); err != nil || organizations != 0 || logins != 0 || globals != 0 {
			t.Errorf("fixture cleanup is incomplete: org=%d login=%d globals=%d error=%v", organizations, logins, globals, err)
		} else {
			t.Log("fixture cleanup: 0 organizations, 0 logins, 0 global framework/role fixtures")
		}
	})
	if _, err := admin.Exec(ctx, "GRANT complianceforge_api TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.User, config.ConnConfig.Password = role, password
	config.ConnConfig.RuntimeParams = map[string]string{}
	config.MaxConns = 2
	api, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("could not open isolated runtime reader")
	}
	t.Cleanup(api.Close)
	if err := database.ValidateRuntimeDatabaseLoginIdentity(ctx, api, config.ConnConfig.User); err != nil {
		t.Fatalf("actual runtime login binding: %v", err)
	}
	if err := database.ValidateAPIDatabasePosture(ctx, api); err != nil {
		t.Fatalf("reviewed API-group availability and denial matrix: %v", err)
	}

	seedDataQualityFixtures(t, ctx, admin, id, suffix)
	var replicationSetting string
	if err := admin.QueryRow(ctx, `SHOW session_replication_role`).Scan(&replicationSetting); err != nil || replicationSetting != "origin" {
		t.Fatalf("administrator fixture setting was not restored: %q error=%v", replicationSetting, err)
	}
	if err := api.QueryRow(ctx, `SHOW session_replication_role`).Scan(&replicationSetting); err != nil || replicationSetting != "origin" {
		t.Fatalf("runtime reads are not under normal guards: %q error=%v", replicationSetting, err)
	}
	reader := repository.NewDataQualityRepository()
	load := func(tenant, organization, actor string) (*models.DataQualityCounts, error) {
		queryCtx, queryCancel := context.WithTimeout(ctx, 2*time.Second)
		defer queryCancel()
		var result *models.DataQualityCounts
		err := database.WithTenantConnection(queryCtx, api, tenant, func(scoped context.Context) error {
			var err error
			result, err = reader.LoadDataQualityCounts(scoped, organization, actor)
			return err
		})
		return result, err
	}

	t.Run("all eighteen real predicates and retained lifecycle exceptions", func(t *testing.T) {
		before := time.Now()
		counts, err := load(id("org-a"), id("org-a"), id("actor-a"))
		if err != nil || counts == nil || len(counts.Counts) != 18 || counts.AsOf.Before(before.Add(-time.Second)) || counts.AsOf.After(time.Now().Add(time.Second)) {
			t.Fatalf("single-MVCC diagnostic failed: %#v error=%v", counts, err)
		}
		for _, check := range counts.Counts {
			want := int64(1)
			if check.Key == models.DQUserRoleScope || check.Key == models.DQUnremovedMembership {
				want = 2
			}
			if check.Count != want {
				t.Errorf("real predicate %s=%d, want %d", check.Key, check.Count, want)
			}
		}
		for _, other := range []struct{ organization, actor string }{{id("org-b"), id("actor-b")}, {id("org-empty"), id("actor-empty")}} {
			otherCounts, err := load(other.organization, other.organization, other.actor)
			if err != nil || otherCounts == nil || len(otherCounts.Counts) != 18 {
				t.Fatalf("other-tenant read failed: %#v error=%v", otherCounts, err)
			}
			for _, check := range otherCounts.Counts {
				if check.Count != 0 {
					t.Errorf("other tenant observed %s=%d from corrupt tenant", check.Key, check.Count)
				}
			}
		}
	})
	t.Run("active actor organization and current-tenant binding", func(t *testing.T) {
		for _, test := range []struct{ tenant, organization, actor string }{
			{id("org-a"), id("org-a"), id("actor-b")},
			{id("org-b"), id("org-a"), id("actor-a")},
			{id("org-a"), id("org-a"), id("actor-inactive")},
			{id("org-a"), id("org-a"), id("actor-deleted")},
			{id("org-inactive"), id("org-inactive"), id("actor-inactive-org")},
			{id("org-deleted"), id("org-deleted"), id("actor-deleted-org")},
		} {
			value, err := load(test.tenant, test.organization, test.actor)
			if value != nil || !errors.Is(err, models.ErrDataQualityScope) {
				t.Errorf("invalid bound scope produced counts: %#v error=%v", value, err)
			}
		}
		if value, err := reader.LoadDataQualityCounts(ctx, id("org-a"), id("actor-a")); value != nil || !errors.Is(err, models.ErrDataQualityUnavailable) {
			t.Fatalf("missing request querier did not fail closed: %#v error=%v", value, err)
		}
	})
	t.Run("same snapshot rejects dirty or unknown schema", func(t *testing.T) {
		transaction, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = transaction.Rollback(context.Background()) }()
		for _, schema := range []struct {
			version int64
			dirty   bool
		}{{database.SupportedSchemaVersion, true}, {database.SupportedSchemaVersion - 1, false}, {database.SupportedSchemaVersion + 1, false}} {
			if _, err := transaction.Exec(ctx, `UPDATE schema_migrations SET version=$1,dirty=$2`, schema.version, schema.dirty); err != nil {
				t.Fatal(err)
			}
			// A temporary committed marker would disrupt other work. This scoped
			// test reads the injected marker only inside an API-role transaction;
			// actual-login reads are proved by all other subtests.
			if _, err := transaction.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			if _, err := transaction.Exec(ctx, `SELECT set_config('app.current_tenant',$1,true)`, id("org-a")); err != nil {
				t.Fatal(err)
			}
			value, err := reader.LoadDataQualityCounts(database.WithQuerier(ctx, transaction), id("org-a"), id("actor-a"))
			if value != nil || !errors.Is(err, models.ErrDataQualityUnavailable) {
				t.Fatalf("unready schema emitted counts: %#v error=%v", value, err)
			}
			if _, err := transaction.Exec(ctx, `RESET ROLE`); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("retained softdeleted framework makes all checks unavailable", func(t *testing.T) {
		if _, err := admin.Exec(ctx, `UPDATE compliance_frameworks SET deleted_at=now() WHERE id=$1`, id("framework-global")); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := admin.Exec(context.Background(), `UPDATE compliance_frameworks SET deleted_at=NULL WHERE id=$1`, id("framework-global")); err != nil {
				t.Errorf("restore fixture framework visibility: %v", err)
			}
		}()
		if value, err := load(id("org-a"), id("org-a"), id("actor-a")); value != nil || !errors.Is(err, models.ErrDataQualityUnavailable) {
			t.Fatalf("valid retention was labelled critical/healthy: %#v error=%v", value, err)
		}
	})
	t.Run("SQL cancellation returns no partial result", func(t *testing.T) {
		// The read cannot acquire its table snapshot while this administrator
		// lock is held. Runtime deadline must cancel the real PostgreSQL query.
		locker, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = locker.Rollback(context.Background()) }()
		if _, err := locker.Exec(ctx, `LOCK TABLE risk_indicator_values IN ACCESS EXCLUSIVE MODE`); err != nil {
			t.Fatal(err)
		}
		conn, err := api.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, id("org-a")); err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = conn.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`) }()
		queryCtx, queryCancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer queryCancel()
		value, err := reader.LoadDataQualityCounts(database.WithQuerier(queryCtx, conn), id("org-a"), id("actor-a"))
		if value != nil || !errors.Is(err, models.ErrDataQualityUnavailable) {
			t.Fatalf("canceled PostgreSQL scan emitted data: %#v error=%v", value, err)
		}
	})
}

func seedDataQualityFixtures(t *testing.T, ctx context.Context, admin *pgxpool.Pool, id func(string) string, suffix string) {
	t.Helper()
	transaction, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := transaction.Exec(ctx, query, args...); err != nil {
			t.Fatalf("isolated data-quality fixture failed: %v", err)
		}
	}
	exec(`SET LOCAL session_replication_role=replica`)
	for _, organization := range []struct{ key, status string }{{"org-a", "active"}, {"org-b", "active"}, {"org-empty", "active"}, {"org-inactive", "deactivated"}, {"org-deleted", "active"}} {
		exec(`INSERT INTO organizations(id,name,slug,status,deleted_at) VALUES($1,'Diagnostic fixture',$2,$3::org_status,CASE WHEN $4 THEN now() ELSE NULL END)`, id(organization.key), "quality-"+organization.key+"-"+suffix, organization.status, organization.key == "org-deleted")
	}
	for _, user := range []struct{ key, organization, status string }{
		{"actor-a", "org-a", "active"}, {"actor-b", "org-b", "active"}, {"actor-empty", "org-empty", "active"},
		{"actor-inactive-org", "org-inactive", "active"}, {"actor-deleted-org", "org-deleted", "active"},
		{"actor-inactive", "org-a", "inactive"}, {"actor-deleted", "org-a", "active"}, {"actor-deprovisioned", "org-a", "inactive"},
	} {
		exec(`INSERT INTO users(id,organization_id,email,status,deleted_at,deprovisioned_at,deprovisioned_by,deprovision_reason)
			VALUES($1,$2,$3,$4::user_status,CASE WHEN $5 THEN now() ELSE NULL END,CASE WHEN $6 THEN now() ELSE NULL END,
			CASE WHEN $6 THEN $7::uuid ELSE NULL END,CASE WHEN $6 THEN 'Fixture deprovision' ELSE NULL END)`, id(user.key), id(user.organization), user.key+"-"+suffix+"@example.test", user.status, user.key == "actor-deleted", user.key == "actor-deprovisioned", id("actor-a"))
	}
	for _, framework := range []struct{ key, organization string }{{"framework-global", ""}, {"framework-other", ""}, {"framework-b", "org-b"}} {
		var organization any
		if framework.organization != "" {
			organization = id(framework.organization)
		}
		exec(`INSERT INTO compliance_frameworks(id,organization_id,code,name,version,is_system_framework) VALUES($1,$2,$3,'Diagnostic framework','1', $4)`, id(framework.key), organization, "quality-"+framework.key+"-"+suffix, framework.organization == "")
	}
	exec(`INSERT INTO framework_controls(id,framework_id,code,title) VALUES($1,$2,'Q-1','Diagnostic control'),($3,$4,'Q-2','Other-tenant control')`, id("control-global"), id("framework-global"), id("control-b"), id("framework-b"))
	exec(`INSERT INTO organization_frameworks(id,organization_id,framework_id) VALUES($1,$2,$3),($4,$5,$3)`, id("adoption-a"), id("org-a"), id("framework-global"), id("adoption-b"), id("org-b"))
	exec(`INSERT INTO control_implementations(organization_id,org_framework_id,framework_control_id) VALUES($1,$2,$3),($1,$4,$5),($1,$4,$3)`, id("org-a"), id("adoption-b"), id("control-global"), id("adoption-a"), id("control-b"))
	exec(`INSERT INTO policies(id,organization_id,policy_ref,title,current_version,current_version_id,deleted_at) VALUES
		($1,$2,'Q-A','Retained policy',1,$3,now()),($4,$2,'Q-C','Bad current pointer',1,$5,NULL),($6,$7,'Q-B','Other policy',1,NULL,NULL)`, id("policy-a"), id("org-a"), id("version-a"), id("policy-current-bad"), id("version-b"), id("policy-b"), id("org-b"))
	exec(`INSERT INTO policy_versions(id,organization_id,policy_id,version_number,version_label,title) VALUES
		($1,$2,$3,1,'1','Retained version'),($4,$5,$6,1,'1','Other version'),($7,$2,$6,2,'2','Wrong tenant parent')`, id("version-a"), id("org-a"), id("policy-a"), id("version-b"), id("org-b"), id("policy-b"), id("version-bad"))
	exec(`INSERT INTO policy_approval_workflows(id,organization_id,policy_id,policy_version_id,workflow_type) VALUES
		($1,$2,$3,$4,'review'),($5,$6,$7,$8,'review'),($9,$2,$7,$8,'review')`, id("workflow-a"), id("org-a"), id("policy-a"), id("version-a"), id("workflow-b"), id("org-b"), id("policy-b"), id("version-b"), id("workflow-bad"))
	exec(`INSERT INTO policy_approval_steps(organization_id,workflow_id,step_number) VALUES($1,$2,1),($1,$3,1)`, id("org-a"), id("workflow-a"), id("workflow-b"))
	exec(`INSERT INTO risks(id,organization_id,risk_ref,title,deleted_at) VALUES($1,$2,'Q-A','Retained risk',now()),($3,$4,'Q-B','Other risk',NULL)`, id("risk-a"), id("org-a"), id("risk-b"), id("org-b"))
	exec(`INSERT INTO risk_assessments(organization_id,risk_id,assessment_type) VALUES($1,$2,'initial'),($1,$3,'initial')`, id("org-a"), id("risk-a"), id("risk-b"))
	exec(`INSERT INTO risk_treatments(organization_id,risk_id,treatment_type,title) VALUES($1,$2,'mitigate','Retained treatment'),($1,$3,'mitigate','Other risk treatment')`, id("org-a"), id("risk-a"), id("risk-b"))
	exec(`INSERT INTO risk_indicators(id,organization_id,risk_id,name,metric_type) VALUES
		($1,$2,NULL,'Legitimate null risk','count'),($3,$2,$4,'Retained risk','count'),($5,$2,$6,'Wrong risk scope','count'),($7,$8,$6,'Other indicator','count')`, id("indicator-a"), id("org-a"), id("indicator-retained"), id("risk-a"), id("indicator-bad"), id("risk-b"), id("indicator-b"), id("org-b"))
	exec(`INSERT INTO risk_indicator_values(organization_id,indicator_id,value,status) VALUES($1,$2,1,'green'),($1,$3,1,'green')`, id("org-a"), id("indicator-a"), id("indicator-b"))
	exec(`INSERT INTO audits(id,organization_id,audit_ref,title,audit_type,lead_auditor_id,scope,scheduled_start_date,scheduled_end_date,deleted_at) VALUES
		($1,$2,'Q-A','Retained audit','internal',$3,'Fixture',CURRENT_DATE,CURRENT_DATE,now()),($4,$5,'Q-B','Other audit','internal',$6,'Fixture',CURRENT_DATE,CURRENT_DATE,NULL)`, id("audit-a"), id("org-a"), id("actor-a"), id("audit-b"), id("org-b"), id("actor-b"))
	exec(`INSERT INTO audit_findings(organization_id,audit_id,finding_ref,title,description,severity,finding_type,recommendation,responsible_user_id,due_date,created_by) VALUES
		($1,$2,'Q-A','Retained finding','Private fixture narrative','low','observation','Fixture',$3,CURRENT_DATE,$3),
		($1,$4,'Q-X','Other audit finding','Private fixture narrative','low','observation','Fixture',$3,CURRENT_DATE,$3)`, id("org-a"), id("audit-a"), id("actor-a"), id("audit-b"))
	exec(`INSERT INTO vendors(id,organization_id,vendor_ref,name,created_by,deleted_at,legal_hold) VALUES($1,$2,'Q-A','Retained held vendor',$3,now(),true),($4,$5,'Q-B','Other vendor',$6,NULL,false)`, id("vendor-a"), id("org-a"), id("actor-a"), id("vendor-b"), id("org-b"), id("actor-b"))
	exec(`INSERT INTO assets(organization_id,asset_ref,name,asset_type,created_by,linked_vendor_id) VALUES($1,'Q-A','Retained vendor asset','hardware',$2,$3),($1,'Q-X','Other vendor asset','hardware',$2,$4),($1,'Q-N','Legitimate null vendor','hardware',$2,NULL)`, id("org-a"), id("actor-a"), id("vendor-a"), id("vendor-b"))
	exec(`INSERT INTO notification_channels(id,organization_id,channel_type,name,configuration,deleted_at) VALUES($1,$2,'email','Retained channel','{}',now()),($3,$4,'email','Other channel','{}',NULL)`, id("channel-a"), id("org-a"), id("channel-b"), id("org-b"))
	type notificationFixture struct {
		key, organization, recipient, channelType, status, channel, parent string
		scheduledOffset, retryOffset, leaseOffset, retryCount              int
		dead                                                               bool
	}
	for _, notification := range []notificationFixture{
		{key: "notification-b", organization: "org-b", recipient: "actor-b", channelType: "in_app", status: "delivered"},
		{key: "notification-bad-recipient", recipient: "actor-b", channelType: "in_app", status: "delivered"},
		{key: "notification-bad-channel", recipient: "actor-a", channelType: "in_app", status: "delivered", channel: "channel-b"},
		{key: "notification-bad-parent", recipient: "actor-a", channelType: "in_app", status: "delivered", parent: "notification-b"},
		{key: "notification-retained-channel", recipient: "actor-a", channelType: "email", status: "sent", channel: "channel-a"},
		{key: "notification-due-inactive", recipient: "actor-inactive", channelType: "email", status: "pending", scheduledOffset: -60},
		{key: "notification-future-inactive", recipient: "actor-inactive", channelType: "email", status: "pending", scheduledOffset: 3600},
		{key: "notification-sent-inactive", recipient: "actor-inactive", channelType: "email", status: "sent", scheduledOffset: -60},
		{key: "notification-dead-inactive", recipient: "actor-inactive", channelType: "email", status: "failed", scheduledOffset: -60, dead: true},
		{key: "notification-leased-inactive", recipient: "actor-inactive", channelType: "email", status: "pending", scheduledOffset: -60, leaseOffset: 3600},
		{key: "notification-exhausted-inactive", recipient: "actor-inactive", channelType: "email", status: "failed", scheduledOffset: -60, retryCount: 3},
		{key: "notification-retry-future-inactive", recipient: "actor-inactive", channelType: "email", status: "failed", scheduledOffset: -60, retryOffset: 3600},
		{key: "notification-due-active", recipient: "actor-a", channelType: "email", status: "pending", scheduledOffset: -60},
	} {
		if notification.organization == "" {
			notification.organization = "org-a"
		}
		var channel, parent any
		if notification.channel != "" {
			channel = id(notification.channel)
		}
		if notification.parent != "" {
			parent = id(notification.parent)
		}
		exec(`INSERT INTO notifications(id,organization_id,event_type,event_payload,recipient_user_id,channel_type,status,channel_id,parent_notification_id,delivery_key,scheduled_for,next_retry_at,leased_until,lease_owner,lease_token,retry_count,dead_at)
			VALUES($1,$2,'quality.fixture','{"private":"must never be returned"}',$3,$4::notification_channel_type,$5::notification_status,$6,$7,
			repeat('0',28)||replace($1::text,'-',''),now()+make_interval(secs=>$8),
			CASE WHEN $9<>0 THEN now()+make_interval(secs=>$9) ELSE NULL END,
			CASE WHEN $10<>0 THEN now()+make_interval(secs=>$10) ELSE NULL END,
			CASE WHEN $10<>0 THEN $1::uuid ELSE NULL END,CASE WHEN $10<>0 THEN $1::uuid ELSE NULL END,$11,
			CASE WHEN $12 THEN now() ELSE NULL END)`, id(notification.key), id(notification.organization), id(notification.recipient), notification.channelType, notification.status, channel, parent, notification.scheduledOffset, notification.retryOffset, notification.leaseOffset, notification.retryCount, notification.dead)
	}
	for _, role := range []struct {
		key, organization string
		system            bool
	}{{"role-a", "org-a", false}, {"role-b", "org-b", false}, {"role-system", "", true}, {"role-false-global", "", false}} {
		var organization any
		if role.organization != "" {
			organization = id(role.organization)
		}
		exec(`INSERT INTO roles(id,organization_id,name,slug,is_system_role,is_custom,deleted_at) VALUES($1,$2,$3,$3,$4,NOT $4,CASE WHEN $5 THEN now() ELSE NULL END)`, id(role.key), organization, "quality-"+role.key+"-"+suffix, role.system, role.key == "role-a")
	}
	for _, role := range []string{"role-a", "role-b", "role-system", "role-false-global"} {
		exec(`INSERT INTO user_roles(user_id,organization_id,role_id,assigned_by) VALUES($1,$2,$3,$1)`, id("actor-a"), id("org-a"), id(role))
	}
	exec(`INSERT INTO directory_groups(id,organization_id,name,slug,created_by,updated_by,deleted_at) VALUES
		($1,$2,'Active fixture group',$3,$4,$4,NULL),($5,$2,'Deleted fixture group',$6,$4,$4,now())`, id("group-active"), id("org-a"), "quality-active-"+suffix, id("actor-a"), id("group-deleted"), "quality-deleted-"+suffix)
	for _, membership := range []struct {
		group, user string
		removed     bool
	}{{"group-deleted", "actor-a", false}, {"group-active", "actor-deprovisioned", false}, {"group-active", "actor-inactive", false}, {"group-deleted", "actor-deprovisioned", true}} {
		exec(`INSERT INTO directory_group_memberships(organization_id,group_id,user_id,added_by,add_reason,removed_at,removed_by,remove_reason)
			VALUES($1,$2,$3,$4,'Fixture membership',CASE WHEN $5 THEN now() ELSE NULL END,
			CASE WHEN $5 THEN $4::uuid ELSE NULL END,CASE WHEN $5 THEN 'Fixture removal' ELSE NULL END)`, id("org-a"), id(membership.group), id(membership.user), id("actor-a"), membership.removed)
	}
	exec(`SET LOCAL session_replication_role=origin`)
	if err := transaction.Commit(ctx); err != nil {
		t.Fatalf("commit isolated legacy fixture: %v", err)
	}
	t.Log(fmt.Sprintf("legacy fixture committed with normal guards restored; %d scoped checks", len(models.DataQualityCheckDefinitions())))
}
