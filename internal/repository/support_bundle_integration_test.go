//go:build integration

package repository_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

func TestSupportBundleGenerationWithForcedRLSTenants(t *testing.T) {
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
	orgA, orgB, actorA, actorB, suspendedA := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleName := "grc_support_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	roleCreated := false
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Support A',$3,'active','enterprise'),($2,'Support B',$4,'active','enterprise')`,
		orgA, orgB, "support-a-"+orgA, "support-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, statement := range []string{
			`DELETE FROM audit_logs WHERE organization_id=ANY($1::uuid[])`,
			`DELETE FROM organizations WHERE id=ANY($1::uuid[])`,
		} {
			if _, err := pool.Exec(cleanupCtx, statement, []string{orgA, orgB}); err != nil {
				t.Errorf("clean support fixtures: %v", err)
			}
		}
		if roleCreated {
			if _, err := pool.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole); err != nil {
				t.Errorf("clean support role grants: %v", err)
			}
			if _, err := pool.Exec(cleanupCtx, "DROP ROLE "+quotedRole); err != nil {
				t.Errorf("clean support role: %v", err)
			}
		}
	}()
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	roleCreated = true
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Operator','active'),($4,$5,$6,'Bob','Operator','active'),($7,$2,$8,'Suspended','Operator','inactive')`,
		actorA, orgA, actorA+"@private-a.example.test", actorB, orgB, actorB+"@private-b.example.test", suspendedA, suspendedA+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET suspended_at=NOW(),suspended_by=$2::uuid WHERE id=$1::uuid`, suspendedA, actorA); err != nil {
		t.Fatal(err)
	}
	secret := "SUPPORT_DB_RECORD_SENTINEL_SECRET_29348"
	for _, fixture := range []struct {
		org, actor string
		count      int
	}{
		{orgA, actorA, 1}, {orgB, actorB, 4},
	} {
		for i := 0; i < fixture.count; i++ {
			if _, err := pool.Exec(ctx, `INSERT INTO integrations
				(id,organization_id,integration_type,name,status,configuration_encrypted,health_status,created_by)
				VALUES ($1,$2,'custom_api',$3::text,'active',$3::text,'healthy',$4)`,
				uuid.NewString(), fixture.org, secret, fixture.actor); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO notifications
				(organization_id,event_type,event_payload,recipient_user_id,channel_type,status,delivery_key,body_text,scheduled_for)
				VALUES ($1,'support.test',jsonb_build_object('secret',$3::text),$2,'in_app','pending',encode(digest($4::text,'sha256'),'hex'),$3,NOW()-INTERVAL '2 minutes')`,
				fixture.org, fixture.actor, secret, uuid.NewString()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := pool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quotedRole+
		"; GRANT SELECT ON users,schema_migrations,queue_outbox,queue_inbox,notifications,integrations,integration_sync_logs,audit_logs TO "+quotedRole+
		"; GRANT INSERT ON audit_logs TO "+quotedRole); err != nil {
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
	defer func() {
		if _, err := conn.Exec(context.Background(), "RESET ROLE"); err != nil {
			t.Errorf("reset support runtime role: %v", err)
		}
		if _, err := conn.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`); err != nil {
			t.Errorf("reset support tenant: %v", err)
		}
	}()
	var superuser, bypass bool
	if err := conn.QueryRow(ctx, `SELECT rolsuper,rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&superuser, &bypass); err != nil || superuser || bypass {
		t.Fatalf("unsafe test role super=%v bypass=%v error=%v", superuser, bypass, err)
	}
	var rls, force bool
	if err := conn.QueryRow(ctx, `SELECT relrowsecurity,relforcerowsecurity FROM pg_class WHERE oid='audit_logs'::regclass`).Scan(&rls, &force); err != nil || !rls || !force {
		t.Fatalf("audit destination lacks FORCE RLS rls=%v force=%v error=%v", rls, force, err)
	}
	requestCtx := database.WithQuerier(ctx, conn)
	diagnosticsRepo, err := repository.NewDiagnosticsRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{App: config.AppConfig{Env: "test"}, JWT: config.JWTConfig{Secret: secret}, Database: config.DatabaseConfig{URL: "postgres://" + secret}}
	diagnosticsSvc, err := service.NewDiagnosticsService(diagnosticsRepo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	auditRepo, err := repository.NewSupportBundleRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	bundleSvc, err := service.NewSupportBundleService(diagnosticsSvc, auditRepo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	setTenant := func(tenant string) {
		t.Helper()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, tenant); err != nil {
			t.Fatal(err)
		}
	}
	consent := models.SupportBundleRequest{Consent: true, Scope: models.SupportBundleScope}
	generate := func(org, actor string) *models.SupportBundleArtifact {
		t.Helper()
		artifact, err := bundleSvc.Generate(requestCtx, org, actor, uuid.NewString(), consent)
		if err != nil {
			t.Fatal(err)
		}
		return artifact
	}
	readHealth := func(artifact *models.SupportBundleArtifact, forbidden ...string) models.SupportBundleHealth {
		t.Helper()
		reader, err := zip.NewReader(bytes.NewReader(artifact.Bytes), int64(len(artifact.Bytes)))
		if err != nil {
			t.Fatal(err)
		}
		var health models.SupportBundleHealth
		for _, file := range reader.File {
			entry, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(entry)
			_ = entry.Close()
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range append(forbidden, secret) {
				if bytes.Contains(data, []byte(value)) {
					t.Fatalf("support %s disclosed forbidden tenant/PII/secret metadata", file.Name)
				}
			}
			if file.Name == "health.json" {
				if err := json.Unmarshal(data, &health); err != nil {
					t.Fatal(err)
				}
			}
		}
		return health
	}
	setTenant(orgA)
	artifactA := generate(orgA, actorA)
	healthA := readHealth(artifactA, orgB, actorA, actorB)
	if healthA.OrganizationID != orgA || healthA.Connectors.Total != 1 || healthA.Notifications.Due != 1 {
		t.Fatalf("tenant A health=%+v", healthA)
	}
	var storedHash, scope string
	var recordedConsent bool
	if err := conn.QueryRow(ctx, `SELECT metadata->>'archive_sha256',metadata->>'scope',(metadata->>'consent')::boolean
		FROM audit_logs WHERE organization_id=$1::uuid AND entity_id=$2::uuid AND action='SUPPORT_BUNDLE_GENERATED'`,
		orgA, artifactA.ID).Scan(&storedHash, &scope, &recordedConsent); err != nil || storedHash != artifactA.SHA256 || scope != models.SupportBundleScope || !recordedConsent {
		t.Fatalf("missing/mismatched durable consent hash=%s scope=%s consent=%v error=%v", storedHash, scope, recordedConsent, err)
	}
	if _, err := bundleSvc.Generate(requestCtx, orgA, actorB, "", consent); !errors.Is(err, service.ErrSupportBundleUnavailable) {
		t.Fatalf("cross-tenant actor generation error=%v", err)
	}
	if _, err := bundleSvc.Generate(requestCtx, orgA, suspendedA, "", consent); !errors.Is(err, service.ErrSupportBundleUnavailable) {
		t.Fatalf("suspended actor generation error=%v", err)
	}
	if _, err := conn.Exec(ctx, `UPDATE audit_logs SET metadata='{}' WHERE entity_id=$1::uuid`, artifactA.ID); err == nil {
		t.Fatal("runtime role mutated support consent evidence")
	} else {
		var pgError *pgconn.PgError
		if !errors.As(err, &pgError) || pgError.Code != "42501" {
			t.Fatalf("unexpected consent mutation error=%v", err)
		}
	}
	setTenant(orgB)
	artifactB := generate(orgB, actorB)
	healthB := readHealth(artifactB, orgA, actorA, actorB)
	if healthB.OrganizationID != orgB || healthB.Connectors.Total != 4 || healthB.Notifications.Due != 4 {
		t.Fatalf("tenant B health=%+v", healthB)
	}
	if artifact, err := bundleSvc.Generate(requestCtx, orgA, actorA, "", consent); !errors.Is(err, service.ErrSupportBundleUnavailable) || artifact != nil {
		t.Fatalf("session/request tenant mismatch artifact=%v error=%v", artifact, err)
	}
	evidence := models.SupportBundleAudit{
		BundleID: uuid.NewString(), OrganizationID: orgA, ActorID: actorA,
		GeneratedAt: time.Now().UTC(), Scope: models.SupportBundleScope, RedactionProfile: models.SupportBundleRedactionProfile,
		ConfigurationFingerprint: strings.Repeat("a", 64), ArchiveSHA256: strings.Repeat("b", 64), ArchiveBytes: 4096,
	}
	if err := auditRepo.RecordSupportBundleGeneration(requestCtx, evidence); !errors.Is(err, repository.ErrSupportBundleAuditDenied) {
		t.Fatalf("direct mismatched tenant audit error=%v", err)
	}
	setTenant("")
	if err := auditRepo.RecordSupportBundleGeneration(requestCtx, evidence); !errors.Is(err, repository.ErrSupportBundleAuditDenied) {
		t.Fatalf("missing tenant audit error=%v", err)
	}
	setTenant(orgA)
	var tenantCount int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='SUPPORT_BUNDLE_GENERATED'`).Scan(&tenantCount); err != nil || tenantCount != 1 {
		t.Fatalf("tenant-visible consent records=%d error=%v", tenantCount, err)
	}
	var totalCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE organization_id=ANY($1::uuid[]) AND action='SUPPORT_BUNDLE_GENERATED'`, []string{orgA, orgB}).Scan(&totalCount); err != nil || totalCount != 2 {
		t.Fatalf("consent failures left evidence total=%d error=%v", totalCount, err)
	}
}
