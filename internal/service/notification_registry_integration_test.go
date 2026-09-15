package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
)

// Opt-in disposable-database proof: actual login authentication (not SET ROLE
// from a superuser session), more than the legacy 1000-tenant ceiling, active
// registry lifecycle, tenant reset and repeat-poll delivery fencing.
func TestNotificationRegistryDeliveryPastLegacyTenantCeiling(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; use a disposable migrated database")
	}
	// This scale fixture seeds/removes 1027 organizations and performs two
	// complete passes on one connection; it is not a latency/SLO assertion.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var ready bool
	if err := admin.QueryRow(ctx, `SELECT to_regprocedure('public.evidence_due_tenants(integer,uuid)') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		t.Fatalf("active-tenant registry migration 000056 is required: %v", err)
	}
	const activeTenants = 1025
	organizationIDs, userIDs, notificationIDs := make([]string, activeTenants+2), make([]string, activeTenants+2), make([]string, activeTenants+2)
	slugs := make([]string, len(organizationIDs))
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	for i := range organizationIDs {
		organizationIDs[i], userIDs[i], notificationIDs[i] = uuid.NewString(), uuid.NewString(), uuid.NewString()
		slugs[i] = fmt.Sprintf("notification-registry-%s-%d", suffix, i)
	}
	roleName := "grc_notification_registry_" + suffix
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	password := "integration_notification_" + uuid.NewString()
	if _, err := admin.Exec(ctx, "CREATE ROLE "+quotedRole+" LOGIN INHERIT NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD '"+password+"'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, organizationIDs); err != nil {
			t.Errorf("clean notification registry test tenants: %v", err)
		}
		if _, err := admin.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole); err != nil {
			t.Errorf("clean notification registry test grants: %v", err)
		}
		if _, err := admin.Exec(cleanupCtx, "DROP ROLE "+quotedRole); err != nil {
			t.Errorf("clean notification registry test role: %v", err)
		}
	})
	if _, err := admin.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier)
		SELECT fixture.id,'Notification registry proof',fixture.slug,'active','starter'
		FROM unnest($1::uuid[],$2::text[]) AS fixture(id,slug)`, organizationIDs, slugs); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO users(id,organization_id,email,status)
		SELECT fixture.user_id,fixture.org_id,fixture.user_id::text||'@example.test','active'
		FROM unnest($1::uuid[],$2::uuid[]) AS fixture(user_id,org_id)`, userIDs, organizationIDs); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO notifications
		(id,organization_id,event_type,event_payload,recipient_user_id,channel_type,subject,body,body_text,status,delivery_key,scheduled_for)
		SELECT fixture.notification_id,fixture.org_id,'registry.proof','{}',fixture.user_id,'in_app',
		       'Registry proof','Registry proof','Registry proof','pending',encode(digest(fixture.notification_id::text,'sha256'),'hex'),NOW()-INTERVAL '1 minute'
		FROM unnest($1::uuid[],$2::uuid[],$3::uuid[]) AS fixture(notification_id,org_id,user_id)`, notificationIDs, organizationIDs, userIDs); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE organizations
		SET status=CASE WHEN id=$1 THEN 'suspended'::org_status ELSE status END,
		    deleted_at=CASE WHEN id=$2 THEN NOW() ELSE deleted_at END
		WHERE id IN($1,$2)`, organizationIDs[activeTenants], organizationIDs[activeTenants+1]); err != nil {
		t.Fatal(err)
	}
	grants := "GRANT USAGE ON SCHEMA public TO " + quotedRole + ";" +
		"GRANT SELECT ON users,notification_channels,notification_rules,notifications TO " + quotedRole + ";" +
		"GRANT INSERT,UPDATE ON notifications TO " + quotedRole + ";" +
		"GRANT EXECUTE ON FUNCTION get_current_tenant(),evidence_due_tenants(INTEGER,UUID) TO " + quotedRole
	if _, err := admin.Exec(ctx, grants); err != nil {
		t.Fatal(err)
	}
	appConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	appConfig.ConnConfig.User, appConfig.ConnConfig.Password = roleName, password
	appConfig.MaxConns, appConfig.MinConns = 1, 0
	app, err := pgxpool.NewWithConfig(ctx, appConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	var current, session string
	var superuser, bypass, forced bool
	if err := app.QueryRow(ctx, `SELECT current_user,session_user,role.rolsuper,role.rolbypassrls,
		(SELECT relforcerowsecurity FROM pg_class WHERE oid='public.notifications'::regclass)
		FROM pg_roles role WHERE role.rolname=current_user`).Scan(&current, &session, &superuser, &bypass, &forced); err != nil {
		t.Fatal(err)
	}
	if current != roleName || session != roleName || superuser || bypass || !forced {
		t.Fatalf("invalid real-role FORCE-RLS proof current=%s session=%s super=%v bypass=%v forced=%v", current, session, superuser, bypass, forced)
	}
	var visible int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("unscoped notification content visible=%d error=%v", visible, err)
	}
	engine := NewNotificationEngine(app, NewEventBus())
	config := NotificationDeliveryConfig{OwnerID: uuid.NewString(), TenantBatch: 31, ClaimBatch: 1,
		LeaseDuration: 3 * time.Minute, RetryBaseDelay: time.Second, RetryMaxDelay: time.Minute, PollInterval: time.Second}
	if err := engine.RunDeliveryCycle(ctx, config); err != nil {
		t.Fatalf("paged active-tenant notification delivery: %v", err)
	}
	var delivered, controlPending int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE organization_id=ANY($1::uuid[]) AND status='delivered' AND retry_count=1`, organizationIDs[:activeTenants]).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE organization_id=ANY($1::uuid[]) AND status='pending' AND retry_count=0`, organizationIDs[activeTenants:]).Scan(&controlPending); err != nil {
		t.Fatal(err)
	}
	if delivered != activeTenants || controlPending != 2 {
		t.Fatalf("active delivered=%d want=%d suspended/deleted untouched=%d want=2", delivered, activeTenants, controlPending)
	}
	if err := engine.RunDeliveryCycle(ctx, config); err != nil {
		t.Fatalf("repeat paged poll: %v", err)
	}
	if err := app.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("tenant context leaked after pooled delivery visible=%d error=%v", visible, err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE organization_id=ANY($1::uuid[]) AND status='delivered' AND retry_count=1`, organizationIDs[:activeTenants]).Scan(&delivered); err != nil || delivered != activeTenants {
		t.Fatalf("repeat delivery changed fenced attempts delivered=%d error=%v", delivered, err)
	}
	if err := database.WithTenantConnection(ctx, app, organizationIDs[0], func(scopedCtx context.Context) error {
		if err := engine.RunDeliveryCycle(scopedCtx, config); err == nil || !strings.Contains(err.Error(), "unscoped worker context") {
			return fmt.Errorf("request-scoped delivery did not fail closed: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
