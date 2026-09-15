package models

import "time"

// DiagnosticStatus is intentionally small and stable so both operators and
// automated support tooling can reason about a diagnostics snapshot without
// parsing human-readable text.
type DiagnosticStatus string

const (
	DiagnosticStatusHealthy  DiagnosticStatus = "healthy"
	DiagnosticStatusWarning  DiagnosticStatus = "warning"
	DiagnosticStatusCritical DiagnosticStatus = "critical"
	DiagnosticStatusUnknown  DiagnosticStatus = "unknown"
)

// DependencyDiagnostic reports only bounded, non-sensitive health metadata.
// Raw dependency errors, endpoints, credentials, and server identities must
// never be included in this response.
type DependencyDiagnostic struct {
	Key       string           `json:"key"`
	Name      string           `json:"name"`
	Status    DiagnosticStatus `json:"status"`
	Critical  bool             `json:"critical"`
	LatencyMS int64            `json:"latency_ms"`
	CheckedAt time.Time        `json:"checked_at"`
	Message   string           `json:"message"`
}

type MigrationDiagnostic struct {
	CurrentVersion   int64            `json:"current_version"`
	SupportedVersion int64            `json:"supported_version"`
	PendingCount     int64            `json:"pending_count"`
	Dirty            bool             `json:"dirty"`
	Status           DiagnosticStatus `json:"status"`
}

type QueueDiagnostic struct {
	Pending               int64            `json:"pending"`
	Leased                int64            `json:"leased"`
	Dead                  int64            `json:"dead"`
	ExpiredLeases         int64            `json:"expired_leases"`
	OldestReadyAgeSeconds int64            `json:"oldest_ready_age_seconds"`
	InboxProcessing       int64            `json:"inbox_processing"`
	InboxExpiredLeases    int64            `json:"inbox_expired_leases"`
	Status                DiagnosticStatus `json:"status"`
}

type NotificationDiagnostic struct {
	Due                 int64            `json:"due"`
	ExpiredLeases       int64            `json:"expired_leases"`
	TerminalFailures    int64            `json:"terminal_failures"`
	OldestDueAgeSeconds int64            `json:"oldest_due_age_seconds"`
	Status              DiagnosticStatus `json:"status"`
}

type ConnectorDiagnostic struct {
	Total             int64            `json:"total"`
	Healthy           int64            `json:"healthy"`
	Degraded          int64            `json:"degraded"`
	Unhealthy         int64            `json:"unhealthy"`
	Unknown           int64            `json:"unknown"`
	RecentFailedSyncs int64            `json:"recent_failed_syncs"`
	Status            DiagnosticStatus `json:"status"`
}

// ConfigurationDiagnostic represents a safe boolean posture check. Message
// and Remediation are curated strings; configuration values are deliberately
// absent from the type to prevent accidental credential disclosure.
type ConfigurationDiagnostic struct {
	Key         string           `json:"key"`
	Category    string           `json:"category"`
	Status      DiagnosticStatus `json:"status"`
	Message     string           `json:"message"`
	Remediation string           `json:"remediation,omitempty"`
}

// OperationalDiagnostics contains database-derived, tenant-scoped signals.
// It is kept separate from runtime dependency probes so repositories remain
// deterministic and easy to integration-test under forced RLS.
type OperationalDiagnostics struct {
	Migration     MigrationDiagnostic    `json:"migration"`
	Queue         QueueDiagnostic        `json:"queue"`
	Notifications NotificationDiagnostic `json:"notifications"`
	Connectors    ConnectorDiagnostic    `json:"connectors"`
}

type DiagnosticsSnapshot struct {
	OrganizationID string                    `json:"organization_id"`
	GeneratedAt    time.Time                 `json:"generated_at"`
	OverallStatus  DiagnosticStatus          `json:"overall_status"`
	Dependencies   []DependencyDiagnostic    `json:"dependencies"`
	Migration      MigrationDiagnostic       `json:"migration"`
	Queue          QueueDiagnostic           `json:"queue"`
	Notifications  NotificationDiagnostic    `json:"notifications"`
	Connectors     ConnectorDiagnostic       `json:"connectors"`
	Configuration  []ConfigurationDiagnostic `json:"configuration"`
}
