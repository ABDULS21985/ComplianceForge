package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const diagnosticsTestOrganizationID = "20000000-0000-0000-0000-000000000001"

type diagnosticsStoreStub struct {
	result         *models.OperationalDiagnostics
	err            error
	organizationID string
}

func (s *diagnosticsStoreStub) LoadOperationalDiagnostics(_ context.Context, organizationID string) (*models.OperationalDiagnostics, error) {
	s.organizationID = organizationID
	return s.result, s.err
}

func secureDiagnosticsConfig() *config.Config {
	return &config.Config{
		App:      config.AppConfig{Env: "production"},
		Database: config.DatabaseConfig{SSLMode: "verify-full"},
		Redis:    config.RedisConfig{URL: "rediss://cache.example.invalid:6379"},
		RabbitMQ: config.RabbitMQConfig{URL: "amqps://queue.example.invalid"},
		CORS:     config.CORSConfig{AllowedOrigins: []string{"https://app.example.invalid"}},
		Storage:  config.StorageConfig{Type: "s3", S3Bucket: "private-bucket-secret-123", S3Region: "eu-west-1"},
		Evidence: config.EvidenceConfig{MaximumUploadBytes: 25 << 20, ScannerNetwork: "tcp", ScannerAddress: "scanner.example.invalid:3310", ScannerTimeoutSeconds: 30, SignedDownloadSeconds: 300},
		SMTP:     config.SMTPConfig{Host: "smtp.example.invalid", TLSMode: "starttls"},
		Observability: config.ObservabilityConfig{
			MetricsEnabled: true, MetricsTokenFile: "/run/secrets/metrics-token",
			TracingEnabled: true, OTLPTraceEndpoint: "https://telemetry.example.invalid",
		},
		Identity: config.IdentityConfig{RPID: "app.example.invalid", RPOrigins: []string{"https://app.example.invalid"}},
		Encryption: config.EncryptionConfig{
			DSRKey: "key-a", IdentityKey: "key-b", IntegrationKey: "key-c", NotificationKey: "key-d",
		},
	}
}

func TestNewDiagnosticsServiceValidatesDependenciesAndProbes(t *testing.T) {
	configuration := secureDiagnosticsConfig()
	if _, err := NewDiagnosticsService(nil, configuration); err == nil {
		t.Fatal("constructor accepted a nil store")
	}
	var typedNil *diagnosticsStoreStub
	if _, err := NewDiagnosticsService(typedNil, configuration); err == nil {
		t.Fatal("constructor accepted a typed nil store")
	}
	if _, err := NewDiagnosticsService(&diagnosticsStoreStub{}, nil); err == nil {
		t.Fatal("constructor accepted nil configuration")
	}
	valid := DependencyProbe{Key: "postgres", Name: "PostgreSQL", Check: func(context.Context) error { return nil }}
	if _, err := NewDiagnosticsService(&diagnosticsStoreStub{}, configuration, valid, valid); err == nil {
		t.Fatal("constructor accepted duplicate probe keys")
	}
	invalid := DependencyProbe{Key: "Not valid!", Name: "Invalid", Check: func(context.Context) error { return nil }}
	if _, err := NewDiagnosticsService(&diagnosticsStoreStub{}, configuration, invalid); err == nil {
		t.Fatal("constructor accepted an invalid probe key")
	}
}

func TestDiagnosticsSnapshotClassifiesSignalsAndRedactsProbeFailure(t *testing.T) {
	store := &diagnosticsStoreStub{result: &models.OperationalDiagnostics{
		Migration:     models.MigrationDiagnostic{CurrentVersion: database.SupportedSchemaVersion},
		Queue:         models.QueueDiagnostic{Pending: 2, OldestReadyAgeSeconds: 301},
		Notifications: models.NotificationDiagnostic{TerminalFailures: 1},
		Connectors:    models.ConnectorDiagnostic{Total: 2, Healthy: 1, Unhealthy: 1},
	}}
	service, err := NewDiagnosticsService(store, secureDiagnosticsConfig(),
		DependencyProbe{Key: "postgres", Name: "PostgreSQL", Critical: true, Check: func(context.Context) error { return nil }},
		DependencyProbe{Key: "redis", Name: "Redis", Critical: true, Check: func(context.Context) error {
			return errors.New("rediss://username:password@secret-host")
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) }

	snapshot, err := service.GetSnapshot(context.Background(), diagnosticsTestOrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	if store.organizationID != diagnosticsTestOrganizationID {
		t.Fatalf("organization = %q", store.organizationID)
	}
	if snapshot.OverallStatus != models.DiagnosticStatusCritical {
		t.Fatalf("overall status = %q", snapshot.OverallStatus)
	}
	if snapshot.Queue.Status != models.DiagnosticStatusWarning || snapshot.Notifications.Status != models.DiagnosticStatusWarning {
		t.Fatalf("unexpected queue/notification status: %#v %#v", snapshot.Queue, snapshot.Notifications)
	}
	if snapshot.Connectors.Status != models.DiagnosticStatusCritical {
		t.Fatalf("connector status = %q", snapshot.Connectors.Status)
	}
	if len(snapshot.Dependencies) != 2 || snapshot.Dependencies[1].Status != models.DiagnosticStatusCritical {
		t.Fatalf("dependency results = %#v", snapshot.Dependencies)
	}
	if strings.Contains(snapshot.Dependencies[1].Message, "password") || strings.Contains(snapshot.Dependencies[1].Message, "secret-host") {
		t.Fatalf("probe error leaked: %q", snapshot.Dependencies[1].Message)
	}
	if !snapshot.GeneratedAt.Equal(service.now()) {
		t.Fatalf("generated_at = %v", snapshot.GeneratedAt)
	}
}

func TestDiagnosticsSnapshotReportsMigrationDrift(t *testing.T) {
	tests := []struct {
		name    string
		version int64
		dirty   bool
		status  models.DiagnosticStatus
		pending int64
	}{
		{name: "current", version: database.SupportedSchemaVersion, status: models.DiagnosticStatusHealthy},
		{name: "pending", version: database.SupportedSchemaVersion - 2, status: models.DiagnosticStatusWarning, pending: 2},
		{name: "dirty", version: database.SupportedSchemaVersion, dirty: true, status: models.DiagnosticStatusCritical},
		{name: "database newer than application", version: database.SupportedSchemaVersion + 1, status: models.DiagnosticStatusCritical},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := &models.OperationalDiagnostics{Migration: models.MigrationDiagnostic{CurrentVersion: test.version, Dirty: test.dirty}}
			classifyOperationalDiagnostics(value)
			if value.Migration.Status != test.status || value.Migration.PendingCount != test.pending {
				t.Fatalf("migration = %#v", value.Migration)
			}
		})
	}
}

func TestDiagnosticsProbePanicAndTimeoutAreContained(t *testing.T) {
	panicResult := runDependencyProbe(context.Background(), time.Now(), DependencyProbe{
		Key: "panic", Name: "Panic", Critical: true, Timeout: time.Second,
		Check: func(context.Context) error { panic("credential detail") },
	})
	if panicResult.Status != models.DiagnosticStatusCritical || strings.Contains(panicResult.Message, "credential") {
		t.Fatalf("panic result = %#v", panicResult)
	}

	timeoutResult := runDependencyProbe(context.Background(), time.Now(), DependencyProbe{
		Key: "timeout", Name: "Timeout", Timeout: time.Millisecond,
		Check: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	})
	if timeoutResult.Status != models.DiagnosticStatusWarning {
		t.Fatalf("timeout result = %#v", timeoutResult)
	}
}

func TestDiagnosticsConfigurationChecksExposePostureNotValues(t *testing.T) {
	configuration := secureDiagnosticsConfig()
	checks := configurationDiagnostics(*configuration)
	if len(checks) != 10 {
		t.Fatalf("checks = %d, want 10", len(checks))
	}
	for _, check := range checks {
		if check.Status != models.DiagnosticStatusHealthy {
			t.Fatalf("secure check %q = %#v", check.Key, check)
		}
		serialized := check.Message + check.Remediation
		for _, secretValue := range []string{"cache.example.invalid", "queue.example.invalid", "scanner.example.invalid", "key-a", "private-bucket-secret-123"} {
			if strings.Contains(serialized, secretValue) {
				t.Fatalf("check %q exposed configuration value %q", check.Key, secretValue)
			}
		}
	}

	insecure := *configuration
	insecure.Redis.URL = "redis://username:password@internal"
	insecure.CORS.AllowedOrigins = []string{"*"}
	insecure.Encryption.IdentityKey = insecure.Encryption.DSRKey
	checks = configurationDiagnostics(insecure)
	critical := 0
	for _, check := range checks {
		if check.Status == models.DiagnosticStatusCritical {
			critical++
		}
		if strings.Contains(check.Message+check.Remediation, "password@internal") || strings.Contains(check.Message+check.Remediation, "key-a") {
			t.Fatalf("check %q exposed an unsafe value", check.Key)
		}
	}
	if critical < 3 {
		t.Fatalf("critical checks = %d, want at least 3", critical)
	}
}

func TestDiagnosticsSnapshotRejectsInvalidTenantAndStoreFailure(t *testing.T) {
	service, err := NewDiagnosticsService(&diagnosticsStoreStub{}, secureDiagnosticsConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetSnapshot(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("invalid tenant was accepted")
	}

	secret := "postgres://user:password@host"
	service, err = NewDiagnosticsService(&diagnosticsStoreStub{err: errors.New(secret)}, secureDiagnosticsConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetSnapshot(context.Background(), diagnosticsTestOrganizationID); err == nil || !strings.Contains(err.Error(), secret) {
		t.Fatal("service should retain store detail for server-side correlated logging")
	}
}
