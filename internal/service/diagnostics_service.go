package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const defaultDiagnosticsProbeTimeout = 2 * time.Second

var diagnosticsKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type DiagnosticsStore interface {
	LoadOperationalDiagnostics(context.Context, string) (*models.OperationalDiagnostics, error)
}

// DependencyProbe is registered by production composition. Check errors are
// used only to select a status; they are never serialized into the API.
type DependencyProbe struct {
	Key      string
	Name     string
	Critical bool
	Timeout  time.Duration
	Check    func(context.Context) error
}

type DiagnosticsService struct {
	store  DiagnosticsStore
	config config.Config
	probes []DependencyProbe
	now    func() time.Time
}

func NewDiagnosticsService(
	store DiagnosticsStore, cfg *config.Config, probes ...DependencyProbe,
) (*DiagnosticsService, error) {
	if interfaceValueIsNil(store) {
		return nil, errors.New("diagnostics store is required")
	}
	if cfg == nil {
		return nil, errors.New("diagnostics configuration is required")
	}
	seen := make(map[string]struct{}, len(probes))
	validated := make([]DependencyProbe, len(probes))
	for index, probe := range probes {
		probe.Key = strings.TrimSpace(strings.ToLower(probe.Key))
		probe.Name = strings.TrimSpace(probe.Name)
		if !diagnosticsKeyPattern.MatchString(probe.Key) {
			return nil, fmt.Errorf("diagnostics probe %d has an invalid key", index)
		}
		if probe.Name == "" || probe.Check == nil {
			return nil, fmt.Errorf("diagnostics probe %q requires a name and check", probe.Key)
		}
		if _, duplicate := seen[probe.Key]; duplicate {
			return nil, fmt.Errorf("diagnostics probe key %q is duplicated", probe.Key)
		}
		seen[probe.Key] = struct{}{}
		if probe.Timeout <= 0 || probe.Timeout > 10*time.Second {
			probe.Timeout = defaultDiagnosticsProbeTimeout
		}
		validated[index] = probe
	}
	return &DiagnosticsService{
		store: store, config: *cfg, probes: validated, now: time.Now,
	}, nil
}

func (s *DiagnosticsService) GetSnapshot(
	ctx context.Context, organizationID string,
) (*models.DiagnosticsSnapshot, error) {
	if _, err := uuid.Parse(organizationID); err != nil {
		return nil, fmt.Errorf("invalid diagnostics organization: %w", err)
	}
	operational, err := s.store.LoadOperationalDiagnostics(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("load operational diagnostics: %w", err)
	}
	if operational == nil {
		return nil, errors.New("load operational diagnostics: empty result")
	}

	generatedAt := s.now().UTC()
	classifyOperationalDiagnostics(operational)
	dependencies := s.runProbes(ctx, generatedAt)
	configuration := configurationDiagnostics(s.config)
	snapshot := &models.DiagnosticsSnapshot{
		OrganizationID: organizationID,
		GeneratedAt:    generatedAt,
		Dependencies:   dependencies,
		Migration:      operational.Migration,
		Queue:          operational.Queue,
		Notifications:  operational.Notifications,
		Connectors:     operational.Connectors,
		Configuration:  configuration,
		OverallStatus:  models.DiagnosticStatusHealthy,
	}
	for _, status := range []models.DiagnosticStatus{
		snapshot.Migration.Status, snapshot.Queue.Status, snapshot.Notifications.Status,
		snapshot.Connectors.Status,
	} {
		snapshot.OverallStatus = worseDiagnosticStatus(snapshot.OverallStatus, status)
	}
	for _, dependency := range dependencies {
		snapshot.OverallStatus = worseDiagnosticStatus(snapshot.OverallStatus, dependency.Status)
	}
	for _, check := range configuration {
		snapshot.OverallStatus = worseDiagnosticStatus(snapshot.OverallStatus, check.Status)
	}
	return snapshot, nil
}

func (s *DiagnosticsService) runProbes(
	ctx context.Context, checkedAt time.Time,
) []models.DependencyDiagnostic {
	results := make([]models.DependencyDiagnostic, len(s.probes))
	var group sync.WaitGroup
	group.Add(len(s.probes))
	for index := range s.probes {
		index, probe := index, s.probes[index]
		go func() {
			defer group.Done()
			results[index] = runDependencyProbe(ctx, checkedAt, probe)
		}()
	}
	group.Wait()
	return results
}

func runDependencyProbe(
	ctx context.Context, checkedAt time.Time, probe DependencyProbe,
) (result models.DependencyDiagnostic) {
	result = models.DependencyDiagnostic{
		Key: probe.Key, Name: probe.Name, Critical: probe.Critical,
		CheckedAt: checkedAt, Status: models.DiagnosticStatusHealthy,
		Message: "Dependency is available.",
	}
	started := time.Now()
	defer func() {
		result.LatencyMS = max(time.Since(started).Milliseconds(), 0)
		if recover() != nil {
			result.Status = unavailableProbeStatus(probe.Critical)
			result.Message = "Dependency check did not complete safely. Review correlated server logs."
		}
	}()
	probeContext, cancel := context.WithTimeout(ctx, probe.Timeout)
	err := probe.Check(probeContext)
	cancel()
	if err != nil {
		result.Status = unavailableProbeStatus(probe.Critical)
		result.Message = "Dependency is unavailable. Review correlated server logs."
	}
	return result
}

func unavailableProbeStatus(critical bool) models.DiagnosticStatus {
	if critical {
		return models.DiagnosticStatusCritical
	}
	return models.DiagnosticStatusWarning
}

func classifyOperationalDiagnostics(value *models.OperationalDiagnostics) {
	value.Migration.SupportedVersion = database.SupportedSchemaVersion
	switch {
	case value.Migration.Dirty || value.Migration.CurrentVersion > database.SupportedSchemaVersion:
		value.Migration.Status = models.DiagnosticStatusCritical
	case value.Migration.CurrentVersion < database.SupportedSchemaVersion:
		value.Migration.PendingCount = database.SupportedSchemaVersion - value.Migration.CurrentVersion
		value.Migration.Status = models.DiagnosticStatusWarning
	default:
		value.Migration.Status = models.DiagnosticStatusHealthy
	}

	value.Queue.Status = models.DiagnosticStatusHealthy
	switch {
	case value.Queue.Dead > 0 || value.Queue.ExpiredLeases > 0 || value.Queue.InboxExpiredLeases > 0 || value.Queue.OldestReadyAgeSeconds >= 3600:
		value.Queue.Status = models.DiagnosticStatusCritical
	case value.Queue.OldestReadyAgeSeconds >= 300:
		value.Queue.Status = models.DiagnosticStatusWarning
	}

	value.Notifications.Status = models.DiagnosticStatusHealthy
	switch {
	case value.Notifications.ExpiredLeases > 0 || value.Notifications.OldestDueAgeSeconds >= 3600:
		value.Notifications.Status = models.DiagnosticStatusCritical
	case value.Notifications.TerminalFailures > 0 || value.Notifications.OldestDueAgeSeconds >= 900:
		value.Notifications.Status = models.DiagnosticStatusWarning
	}

	value.Connectors.Status = models.DiagnosticStatusHealthy
	switch {
	case value.Connectors.Unhealthy > 0:
		value.Connectors.Status = models.DiagnosticStatusCritical
	case value.Connectors.Degraded > 0 || value.Connectors.Unknown > 0 || value.Connectors.RecentFailedSyncs > 0:
		value.Connectors.Status = models.DiagnosticStatusWarning
	}
}

func configurationDiagnostics(cfg config.Config) []models.ConfigurationDiagnostic {
	production := cfg.App.Env == "production" || cfg.App.Env == "staging"
	checks := []models.ConfigurationDiagnostic{
		databaseTransportCheck(cfg, production),
		redisTransportCheck(cfg, production),
		rabbitTransportCheck(cfg, production),
		corsPostureCheck(cfg, production),
		storagePostureCheck(cfg, production),
		evidenceScannerPostureCheck(cfg, production),
		smtpTransportCheck(cfg, production),
		telemetryPostureCheck(cfg, production),
		identityOriginCheck(cfg, production),
		encryptionSeparationCheck(cfg),
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].Key < checks[j].Key })
	return checks
}

func databaseTransportCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	secure := strings.EqualFold(cfg.Database.SSLMode, "verify-full")
	return postureCheck("database_transport", "transport", secure, production,
		"Database certificate and hostname verification is enabled.",
		"Database transport is not configured for full certificate and hostname verification.",
		"Set DATABASE_SSLMODE=verify-full and configure a trusted database certificate chain.")
}

func redisTransportCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	secure := strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.Redis.URL)), "rediss://")
	return postureCheck("redis_transport", "transport", secure, production,
		"Redis transport encryption is configured.",
		"Redis transport encryption cannot be confirmed.",
		"Use a rediss:// endpoint with certificate verification for shared Redis.")
}

func rabbitTransportCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	secure := strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.RabbitMQ.URL)), "amqps://")
	return postureCheck("queue_transport", "transport", secure, production,
		"Queue transport encryption is configured.",
		"Queue transport encryption cannot be confirmed.",
		"Use an amqps:// broker endpoint with certificate verification.")
}

func corsPostureCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	secure := len(cfg.CORS.AllowedOrigins) > 0
	for _, rawOrigin := range cfg.CORS.AllowedOrigins {
		origin, err := url.Parse(strings.TrimSpace(rawOrigin))
		if rawOrigin == "*" || err != nil || origin.Host == "" || (production && origin.Scheme != "https") {
			secure = false
			break
		}
	}
	return postureCheck("cors_origins", "browser_security", secure, production,
		"Browser origins are explicit and use the expected transport.",
		"Browser origins are absent, wildcarded, malformed, or insecure for this environment.",
		"Configure exact HTTPS origins and do not use wildcard origins with authenticated requests.")
}

func storagePostureCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	secure := strings.EqualFold(cfg.Storage.Type, "s3") && strings.TrimSpace(cfg.Storage.S3Bucket) != "" && strings.TrimSpace(cfg.Storage.S3Region) != ""
	return postureCheck("durable_object_storage", "data", secure, production,
		"Durable regional object-storage configuration is declared.",
		"Durable regional object-storage configuration is incomplete.",
		"Configure the supported S3 object-storage backend, region, lifecycle, and recovery controls.")
}

func evidenceScannerPostureCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	network := strings.ToLower(strings.TrimSpace(cfg.Evidence.ScannerNetwork))
	secure := (network == "tcp" || network == "unix") && strings.TrimSpace(cfg.Evidence.ScannerAddress) != "" &&
		cfg.Evidence.ScannerTimeoutSeconds >= 1 && cfg.Evidence.ScannerTimeoutSeconds <= 300 &&
		cfg.Evidence.MaximumUploadBytes >= 1<<20 && cfg.Evidence.MaximumUploadBytes <= int64(2<<30) &&
		cfg.Evidence.SignedDownloadSeconds >= 30 && cfg.Evidence.SignedDownloadSeconds <= 900
	return postureCheck("evidence_security", "data", secure, production,
		"Bounded evidence upload, malware scanning, and signed-download controls are configured.",
		"Evidence malware scanning or upload/download bounds are incomplete.",
		"Configure a reachable ClamAV endpoint, bounded uploads, scan timeout, and short signed-download lifetime.")
}

func smtpTransportCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	mode := strings.ToLower(strings.TrimSpace(cfg.SMTP.TLSMode))
	secure := strings.TrimSpace(cfg.SMTP.Host) != "" && (mode == "tls" || mode == "starttls")
	return postureCheck("smtp_transport", "transport", secure, production,
		"SMTP transport encryption is configured.",
		"SMTP delivery is disabled or transport encryption cannot be confirmed.",
		"Configure SMTP with mandatory TLS or STARTTLS and a verified server name.")
}

func telemetryPostureCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	secure := cfg.Observability.MetricsEnabled && strings.TrimSpace(cfg.Observability.MetricsTokenFile) != "" &&
		cfg.Observability.TracingEnabled && strings.TrimSpace(cfg.Observability.OTLPTraceEndpoint) != ""
	return postureCheck("telemetry", "operations", secure, production,
		"Protected metrics and distributed tracing are configured.",
		"Metrics authentication or distributed tracing is not fully configured.",
		"Enable protected metrics and OTLP tracing before production rollout.")
}

func identityOriginCheck(cfg config.Config, production bool) models.ConfigurationDiagnostic {
	secure := strings.TrimSpace(cfg.Identity.RPID) != "" && len(cfg.Identity.RPOrigins) > 0
	for _, rawOrigin := range cfg.Identity.RPOrigins {
		origin, err := url.Parse(strings.TrimSpace(rawOrigin))
		if err != nil || origin.Host == "" || (production && origin.Scheme != "https") {
			secure = false
			break
		}
	}
	return postureCheck("identity_origins", "identity", secure, production,
		"WebAuthn relying-party origins are explicit and secure.",
		"WebAuthn relying-party settings are absent, malformed, or insecure for this environment.",
		"Configure an exact relying-party ID and HTTPS origins for every supported browser origin.")
}

func encryptionSeparationCheck(cfg config.Config) models.ConfigurationDiagnostic {
	values := []string{
		strings.TrimSpace(cfg.Encryption.DSRKey), strings.TrimSpace(cfg.Encryption.IdentityKey),
		strings.TrimSpace(cfg.Encryption.IntegrationKey), strings.TrimSpace(cfg.Encryption.NotificationKey),
	}
	unique := make(map[string]struct{}, len(values))
	secure := true
	for _, value := range values {
		if value == "" {
			secure = false
			continue
		}
		if _, exists := unique[value]; exists {
			secure = false
		}
		unique[value] = struct{}{}
	}
	status := models.DiagnosticStatusHealthy
	message := "Domain encryption keys are configured and separated."
	remediation := ""
	if !secure {
		status = models.DiagnosticStatusCritical
		message = "Domain encryption key separation cannot be confirmed."
		remediation = "Configure distinct high-entropy keys for identity, DSR, integrations, and notifications."
	}
	return models.ConfigurationDiagnostic{Key: "encryption_key_separation", Category: "cryptography", Status: status, Message: message, Remediation: remediation}
}

func postureCheck(
	key, category string, pass, production bool, success, failure, remediation string,
) models.ConfigurationDiagnostic {
	if pass {
		return models.ConfigurationDiagnostic{Key: key, Category: category, Status: models.DiagnosticStatusHealthy, Message: success}
	}
	status := models.DiagnosticStatusWarning
	if production {
		status = models.DiagnosticStatusCritical
	}
	return models.ConfigurationDiagnostic{Key: key, Category: category, Status: status, Message: failure, Remediation: remediation}
}

func worseDiagnosticStatus(current, candidate models.DiagnosticStatus) models.DiagnosticStatus {
	weight := map[models.DiagnosticStatus]int{
		models.DiagnosticStatusUnknown: 0, models.DiagnosticStatusHealthy: 1,
		models.DiagnosticStatusWarning: 2, models.DiagnosticStatusCritical: 3,
	}
	if weight[candidate] > weight[current] {
		return candidate
	}
	return current
}

func interfaceValueIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
