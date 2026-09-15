package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
)

const supportTestActor = "10000000-0000-0000-0000-000000000001"

type supportSnapshotStub struct {
	value *models.DiagnosticsSnapshot
	err   error
	calls atomic.Int64
}

func (s *supportSnapshotStub) GetSnapshot(context.Context, string) (*models.DiagnosticsSnapshot, error) {
	s.calls.Add(1)
	return s.value, s.err
}

type supportAuditStub struct {
	mu       sync.Mutex
	evidence []models.SupportBundleAudit
	err      error
}

func (s *supportAuditStub) RecordSupportBundleGeneration(_ context.Context, evidence models.SupportBundleAudit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence = append(s.evidence, evidence)
	return s.err
}

func newSupportFixture(t *testing.T) (*SupportBundleService, *supportSnapshotStub, *supportAuditStub) {
	t.Helper()
	snapshots := &supportSnapshotStub{value: &models.DiagnosticsSnapshot{
		OrganizationID: diagnosticsTestOrganizationID,
		GeneratedAt:    time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC),
		OverallStatus:  models.DiagnosticStatusWarning,
		Queue:          models.QueueDiagnostic{Pending: 7, Status: models.DiagnosticStatusHealthy},
		Notifications:  models.NotificationDiagnostic{Due: 3, Status: models.DiagnosticStatusWarning},
		Connectors:     models.ConnectorDiagnostic{Total: 2, Unhealthy: 1, Status: models.DiagnosticStatusCritical},
	}}
	audit := &supportAuditStub{}
	service, err := NewSupportBundleService(snapshots, audit, secureDiagnosticsConfig())
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 9, 15, 10, 0, 1, 0, time.UTC) }
	return service, snapshots, audit
}

func supportConsent() models.SupportBundleRequest {
	return models.SupportBundleRequest{Consent: true, Scope: models.SupportBundleScope}
}

func supportMembers(t *testing.T, artifact *models.SupportBundleArtifact) map[string][]byte {
	t.Helper()
	if artifact == nil || len(artifact.Bytes) > MaximumSupportBundleBytes || artifact.SHA256 != supportSHA256(artifact.Bytes) {
		t.Fatal("invalid or unbounded support archive")
	}
	reader, err := zip.NewReader(bytes.NewReader(artifact.Bytes), int64(len(artifact.Bytes)))
	if err != nil || len(reader.File) != 4 {
		t.Fatalf("support archive members=%v error=%v", reader, err)
	}
	members := make(map[string][]byte)
	for _, file := range reader.File {
		if file.Method != zip.Store || strings.ContainsAny(file.Name, "/\\") || file.UncompressedSize64 > MaximumSupportBundleBytes {
			t.Fatalf("unsafe member %s", file.Name)
		}
		entry, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(entry)
		closeErr := entry.Close()
		if err != nil || closeErr != nil {
			t.Fatal(errors.Join(err, closeErr))
		}
		if _, duplicate := members[file.Name]; duplicate {
			t.Fatalf("duplicate archive member %s", file.Name)
		}
		members[file.Name] = data
	}
	return members
}

func TestSupportBundleRecordsConsentAndVerifiableManifest(t *testing.T) {
	svc, _, audit := newSupportFixture(t)
	requestID := uuid.NewString()
	artifact, err := svc.Generate(context.Background(), diagnosticsTestOrganizationID, supportTestActor, requestID, supportConsent())
	if err != nil {
		t.Fatal(err)
	}
	members := supportMembers(t, artifact)
	var manifest models.SupportBundleManifest
	if err := json.Unmarshal(members["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.BundleID != artifact.ID || manifest.OrganizationID != diagnosticsTestOrganizationID || manifest.Scope != models.SupportBundleScope ||
		manifest.SchemaVersion != 1 || manifest.RedactionProfile != supportRedactionProfile || manifest.AutomaticallyTransmitted ||
		len(manifest.Files) != 3 || len(manifest.Excluded) != 8 || !manifest.ConsentRecordedAt.Equal(svc.now()) {
		t.Fatalf("manifest = %+v", manifest)
	}
	for _, file := range manifest.Files {
		if file.Bytes != len(members[file.Name]) || file.SHA256 != supportSHA256(members[file.Name]) {
			t.Fatalf("manifest hash mismatch %s", file.Name)
		}
	}
	if manifest.ConfigurationFingerprint != supportSHA256(members["configuration.json"]) {
		t.Fatal("configuration fingerprint did not cover exact safe member")
	}
	if len(audit.evidence) != 1 {
		t.Fatal("generation did not durably record exactly one consent event")
	}
	evidence := audit.evidence[0]
	if evidence.BundleID != artifact.ID || evidence.OrganizationID != diagnosticsTestOrganizationID || evidence.ActorID != supportTestActor ||
		evidence.RequestID != requestID || evidence.ArchiveSHA256 != artifact.SHA256 || evidence.ArchiveBytes != len(artifact.Bytes) ||
		evidence.ConfigurationFingerprint != manifest.ConfigurationFingerprint || evidence.Scope != models.SupportBundleScope {
		t.Fatalf("consent evidence=%+v", evidence)
	}
	for _, file := range members {
		if bytes.Contains(file, []byte(supportTestActor)) || bytes.Contains(file, []byte(requestID)) {
			t.Fatal("archive disclosed server-only actor or request identifier")
		}
	}
}

func TestSupportBundleAllowlistRejectsPoisonedMetadataAndConfig(t *testing.T) {
	svc, snapshots, audit := newSupportFixture(t)
	secret := "support-sentinel-secret-93854"
	cfg := secureDiagnosticsConfig()
	cfg.App.Env = secret
	cfg.App.Name = secret
	cfg.Database.URL, cfg.Database.Password, cfg.Database.Host = secret, secret, secret
	cfg.JWT.Secret, cfg.JWT.Issuer = secret, secret
	cfg.Redis.URL, cfg.Redis.Password = "rediss://"+secret, secret
	cfg.RabbitMQ.URL = "amqps://" + secret
	cfg.Storage.Path, cfg.Storage.S3Bucket, cfg.Storage.S3Endpoint, cfg.Storage.S3KMSKeyID = secret, secret, secret, secret
	cfg.SMTP.Password, cfg.SMTP.User, cfg.SMTP.Host = secret, secret, secret
	cfg.OAuth.ClientID, cfg.OAuth.ClientSecret, cfg.OAuth.RedirectURL = secret, secret, secret
	cfg.Identity.RPID, cfg.Identity.RPOrigins = secret, []string{"https://" + secret}
	cfg.CORS.AllowedOrigins = []string{"https://" + secret}
	cfg.Evidence.ScannerAddress = secret
	cfg.Observability.OTLPTraceEndpoint, cfg.Observability.MetricsTokenFile = secret, secret
	cfg.Observability.ServiceVersion = "1.2.3+" + secret
	cfg.HTTP.ReadTimeoutSeconds, cfg.Database.MaxConns = -1, 1_000_000
	svc.configuration = supportConfiguration(*cfg)
	snapshots.value.OverallStatus = models.DiagnosticStatus(secret)
	snapshots.value.Queue.Status = models.DiagnosticStatus(secret)
	snapshots.value.Dependencies = []models.DependencyDiagnostic{
		{Key: "postgres", Name: secret, Message: secret, Status: models.DiagnosticStatus(secret), LatencyMS: -4},
		{Key: secret, Name: secret, Message: secret},
		{Key: "postgres", Name: "duplicate", Status: models.DiagnosticStatusHealthy},
	}
	snapshots.value.Configuration = []models.ConfigurationDiagnostic{{Key: secret, Message: secret, Remediation: secret}}
	artifact, err := svc.Generate(context.Background(), diagnosticsTestOrganizationID, supportTestActor, "", supportConsent())
	if err != nil {
		t.Fatal(err)
	}
	members := supportMembers(t, artifact)
	for name, value := range members {
		if bytes.Contains(value, []byte(secret)) || bytes.Contains(value, []byte(supportSHA256([]byte(secret)))) {
			t.Fatalf("%s leaked raw metadata or a raw-secret hash", name)
		}
	}
	var health models.SupportBundleHealth
	if err := json.Unmarshal(members["health.json"], &health); err != nil {
		t.Fatal(err)
	}
	if health.OverallStatus != models.DiagnosticStatusUnknown || health.Queue.Status != models.DiagnosticStatusUnknown ||
		len(health.Dependencies) != 1 || health.Dependencies[0].Status != models.DiagnosticStatusUnknown || health.Dependencies[0].LatencyMS != 0 {
		t.Fatalf("allowlisted health=%+v", health)
	}
	if svc.configuration.Environment != "unknown" || svc.configuration.ServiceVersion != "1.2.3" || svc.configuration.DatabaseMaximumConns != 0 || svc.configuration.RequestReadSeconds != 0 || len(audit.evidence) != 1 {
		t.Fatalf("safe configuration=%+v", svc.configuration)
	}
}

func TestSupportConfigurationFingerprintIgnoresSecretRotationAndEndpointNames(t *testing.T) {
	a, b := secureDiagnosticsConfig(), secureDiagnosticsConfig()
	a.JWT.Secret, b.JWT.Secret = "jwt-secret-a", "jwt-secret-b"
	a.Database.URL, b.Database.URL = "postgres://a:secret@db-a", "postgres://b:changed@db-b"
	a.Storage.S3Bucket, b.Storage.S3Bucket = "customer-bucket-a", "customer-bucket-b"
	a.Redis.URL, b.Redis.URL = "rediss://a:secret@cache-a", "rediss://b:changed@cache-b"
	a.Encryption.DSRKey, b.Encryption.DSRKey = "encryption-version-a", "encryption-version-b"
	a.Observability.ServiceVersion, b.Observability.ServiceVersion = "1.2.3+internal-a", "1.2.3+internal-b"
	jsonA, err := supportJSON(supportConfiguration(*a))
	if err != nil {
		t.Fatal(err)
	}
	jsonB, err := supportJSON(supportConfiguration(*b))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jsonA, jsonB) {
		t.Fatalf("raw values affected safe fingerprint:\n%s\n%s", jsonA, jsonB)
	}
	b.Observability.MetricsEnabled = false
	jsonB, _ = supportJSON(supportConfiguration(*b))
	if bytes.Equal(jsonA, jsonB) {
		t.Fatal("safe posture change did not affect fingerprint")
	}
}

func TestSupportBundleRejectsInvalidConsentAndPrincipalBeforeReads(t *testing.T) {
	for _, test := range []struct {
		name, org, actor, request string
		req                       models.SupportBundleRequest
	}{
		{name: "no consent", org: diagnosticsTestOrganizationID, actor: supportTestActor, req: models.SupportBundleRequest{Scope: models.SupportBundleScope}},
		{name: "unknown scope", org: diagnosticsTestOrganizationID, actor: supportTestActor, req: models.SupportBundleRequest{Consent: true, Scope: "raw_logs"}},
		{name: "empty scope", org: diagnosticsTestOrganizationID, actor: supportTestActor, req: models.SupportBundleRequest{Consent: true}},
		{name: "invalid org", org: "not-a-uuid", actor: supportTestActor, req: supportConsent()},
		{name: "nil org", org: uuid.Nil.String(), actor: supportTestActor, req: supportConsent()},
		{name: "invalid actor", org: diagnosticsTestOrganizationID, actor: "", req: supportConsent()},
		{name: "nil actor", org: diagnosticsTestOrganizationID, actor: uuid.Nil.String(), req: supportConsent()},
		{name: "invalid request", org: diagnosticsTestOrganizationID, actor: supportTestActor, request: "raw-internal-detail", req: supportConsent()},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, snapshots, audit := newSupportFixture(t)
			artifact, err := svc.Generate(context.Background(), test.org, test.actor, test.request, test.req)
			if !errors.Is(err, ErrInvalidSupportBundleRequest) || artifact != nil || snapshots.calls.Load() != 0 || len(audit.evidence) != 0 {
				t.Fatalf("artifact=%v error=%v reads=%d audits=%d", artifact, err, snapshots.calls.Load(), len(audit.evidence))
			}
		})
	}
}

func TestSupportBundleFailsClosedOnSnapshotAuditAndCancellationFailures(t *testing.T) {
	for _, name := range []string{"snapshot error", "nil snapshot", "wrong tenant", "audit failure", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			svc, snapshots, audit := newSupportFixture(t)
			ctx := context.Background()
			switch name {
			case "snapshot error":
				snapshots.err = errors.New("postgres://secret@internal")
			case "nil snapshot":
				snapshots.value = nil
			case "wrong tenant":
				snapshots.value.OrganizationID = uuid.NewString()
			case "audit failure":
				audit.err = errors.New("postgres://secret@internal")
			case "cancelled":
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelCtx
			}
			artifact, err := svc.Generate(ctx, diagnosticsTestOrganizationID, supportTestActor, "", supportConsent())
			if !errors.Is(err, ErrSupportBundleUnavailable) || artifact != nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("artifact=%v error=%v", artifact, err)
			}
			if name != "audit failure" && len(audit.evidence) != 0 {
				t.Fatal("invalid snapshot/cancellation left false consent-generation evidence")
			}
		})
	}
}

func TestNewSupportBundleServiceRejectsMissingAndTypedNilDependencies(t *testing.T) {
	var nilSnapshot *supportSnapshotStub
	var nilAudit *supportAuditStub
	for _, test := range []struct {
		snapshots SupportBundleSnapshotProvider
		audit     SupportBundleAuditStore
	}{
		{nil, &supportAuditStub{}}, {nilSnapshot, &supportAuditStub{}},
		{&supportSnapshotStub{}, nil}, {&supportSnapshotStub{}, nilAudit},
	} {
		if _, err := NewSupportBundleService(test.snapshots, test.audit, secureDiagnosticsConfig()); err == nil {
			t.Fatal("constructor accepted missing dependency")
		}
	}
	if _, err := NewSupportBundleService(&supportSnapshotStub{}, &supportAuditStub{}, nil); err == nil {
		t.Fatal("constructor accepted nil configuration")
	}
}

func TestSupportBundleConcurrentGenerationsHaveDistinctAuditedIdentities(t *testing.T) {
	svc, _, audit := newSupportFixture(t)
	const count = 24
	var group sync.WaitGroup
	group.Add(count)
	errorsFound := make(chan error, count)
	for range count {
		go func() {
			defer group.Done()
			artifact, err := svc.Generate(context.Background(), diagnosticsTestOrganizationID, supportTestActor, "", supportConsent())
			if err != nil {
				errorsFound <- err
				return
			}
			if artifact == nil || artifact.SHA256 != supportSHA256(artifact.Bytes) {
				errorsFound <- errors.New("invalid concurrent artifact")
			}
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	seen := make(map[string]bool)
	for _, evidence := range audit.evidence {
		if seen[evidence.BundleID] {
			t.Fatal("concurrent generations reused an identity")
		}
		seen[evidence.BundleID] = true
	}
	if len(seen) != count {
		t.Fatalf("recorded %d distinct events, want %d", len(seen), count)
	}
}

func TestSupportArchiveRejectsOversizedMembersAndMetadata(t *testing.T) {
	if _, err := buildSupportArchive([]supportFile{{name: "health.json", data: make([]byte, MaximumSupportBundleBytes+1)}}); err == nil {
		t.Fatal("archive accepted oversized content")
	}
	if _, err := buildSupportArchive([]supportFile{{name: "health.json", data: make([]byte, MaximumSupportBundleBytes)}}); err == nil {
		t.Fatal("archive overhead exceeded total budget without rejection")
	}
}
