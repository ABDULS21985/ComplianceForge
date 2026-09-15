package models

import "time"

// SupportBundleScope is deliberately not extensible through a request. Adding
// logs, record contents, or new configuration fields needs a new reviewed scope.
const (
	SupportBundleScope            = "health_and_posture"
	SupportBundleRedactionProfile = "health_posture_allowlist_v1"
	MaximumSupportBundleBytes     = 64 << 10
)

// SupportBundleExclusions returns a fresh copy of the reviewed v1 exclusions.
// Neither a provider nor a caller can mutate the policy used by later bundles.
func SupportBundleExclusions() []string {
	return []string{"credentials", "dependency_endpoints", "origins_and_paths", "raw_logs", "event_payloads", "business_records", "user_identifiers", "domain_record_identifiers"}
}

const SupportBundleReadme = `ComplianceForge support bundle - health/posture scope v1

This ZIP contains tenant-scoped aggregate health, reviewed configuration posture,
and operational budgets. It does not contain raw configuration, credentials,
endpoints, file paths, browser origins, logs, event payloads, business records,
or user identifiers. The organization UUID is included to identify the tenant.
Budget zero means unspecified or outside the reviewed bounds. Unknown dependency
keys and non-enumerated statuses are omitted or replaced with "unknown".

SHA256 hashes in manifest.json cover each listed member's exact bytes. The
configuration fingerprint covers configuration.json only, never raw secrets.
These hashes detect accidental changes; they are not signatures or proof of
origin. The archive hash is recorded in the server's consent audit trail and
returned in the X-Support-Bundle-SHA256 download header.

Explicit consent was durably recorded before these bytes were released. The
application does not send this ZIP to any support provider or retain a copy.
Review all members before sharing through an approved secure support channel.
The audit records generation, not confirmation of download or provider receipt.
`

type SupportBundleRequest struct {
	Consent bool   `json:"consent"`
	Scope   string `json:"scope"`
}

// SupportBundleArtifact is an internal result, never a JSON response model.
type SupportBundleArtifact struct {
	ID     string
	SHA256 string
	Bytes  []byte
}

type SupportBundleFile struct {
	Name   string `json:"name"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type SupportBundleManifest struct {
	SchemaVersion            int                 `json:"schema_version"`
	BundleID                 string              `json:"bundle_id"`
	OrganizationID           string              `json:"organization_id"`
	GeneratedAt              time.Time           `json:"generated_at"`
	Scope                    string              `json:"scope"`
	ConsentRecordedAt        time.Time           `json:"consent_recorded_at"`
	RedactionProfile         string              `json:"redaction_profile"`
	ConfigurationFingerprint string              `json:"configuration_fingerprint"`
	AutomaticallyTransmitted bool                `json:"automatically_transmitted"`
	Files                    []SupportBundleFile `json:"files"`
	Excluded                 []string            `json:"excluded"`
}

// SupportDependency intentionally omits operator-provided names, messages,
// errors, connection strings, and server identities.
type SupportDependency struct {
	Key       string           `json:"key"`
	Status    DiagnosticStatus `json:"status"`
	Critical  bool             `json:"critical"`
	LatencyMS int64            `json:"latency_ms"`
}

type SupportBundleHealth struct {
	OrganizationID string                 `json:"organization_id"`
	GeneratedAt    time.Time              `json:"generated_at"`
	OverallStatus  DiagnosticStatus       `json:"overall_status"`
	Dependencies   []SupportDependency    `json:"dependencies"`
	Migration      MigrationDiagnostic    `json:"migration"`
	Queue          QueueDiagnostic        `json:"queue"`
	Notifications  NotificationDiagnostic `json:"notifications"`
	Connectors     ConnectorDiagnostic    `json:"connectors"`
}

type SupportPosture struct {
	Key    string           `json:"key"`
	Status DiagnosticStatus `json:"status"`
}

// SupportBundleConfiguration contains only enumerations, booleans, and bounded
// operational budgets. Fingerprints are of this allowlist, NOT hashes of raw
// secrets, origins, bucket names, file paths, endpoints, or complete config.
type SupportBundleConfiguration struct {
	SchemaVersion          int              `json:"schema_version"`
	Environment            string           `json:"environment"`
	ServiceVersion         string           `json:"service_version"`
	MetricsEnabled         bool             `json:"metrics_enabled"`
	TracingEnabled         bool             `json:"tracing_enabled"`
	DatabaseMaximumConns   int64            `json:"database_maximum_connections"`
	DatabaseMinimumConns   int64            `json:"database_minimum_connections"`
	MaximumEvidenceBytes   int64            `json:"maximum_evidence_bytes"`
	ScannerTimeoutSeconds  int64            `json:"scanner_timeout_seconds"`
	RequestReadSeconds     int64            `json:"request_read_seconds"`
	RequestWriteSeconds    int64            `json:"request_write_seconds"`
	ShutdownTimeoutSeconds int64            `json:"shutdown_timeout_seconds"`
	Posture                []SupportPosture `json:"posture"`
}

// SupportBundleAudit contains safe generation evidence. The principal and
// request identity stay in the server-side audit trail, not in the ZIP.
type SupportBundleAudit struct {
	BundleID                 string
	OrganizationID           string
	ActorID                  string
	RequestID                string
	GeneratedAt              time.Time
	Scope                    string
	RedactionProfile         string
	ConfigurationFingerprint string
	ArchiveSHA256            string
	ArchiveBytes             int
}
