export type DiagnosticStatus = 'healthy' | 'warning' | 'critical' | 'unknown';

export interface DependencyDiagnostic {
  key: string;
  name: string;
  status: DiagnosticStatus;
  critical: boolean;
  latency_ms: number;
  checked_at: string;
  message: string;
}

export interface MigrationDiagnostic {
  current_version: number;
  supported_version: number;
  pending_count: number;
  dirty: boolean;
  status: DiagnosticStatus;
}

export interface QueueDiagnostic {
  pending: number;
  leased: number;
  dead: number;
  expired_leases: number;
  oldest_ready_age_seconds: number;
  inbox_processing: number;
  inbox_expired_leases: number;
  status: DiagnosticStatus;
}

export interface NotificationDiagnostic {
  due: number;
  expired_leases: number;
  terminal_failures: number;
  oldest_due_age_seconds: number;
  status: DiagnosticStatus;
}

export interface ConnectorDiagnostic {
  total: number;
  healthy: number;
  degraded: number;
  unhealthy: number;
  unknown: number;
  recent_failed_syncs: number;
  status: DiagnosticStatus;
}

export interface ConfigurationDiagnostic {
  key: string;
  category: string;
  status: DiagnosticStatus;
  message: string;
  remediation?: string;
}

export interface DiagnosticsSnapshot {
  organization_id: string;
  generated_at: string;
  overall_status: DiagnosticStatus;
  dependencies: DependencyDiagnostic[];
  migration: MigrationDiagnostic;
  queue: QueueDiagnostic;
  notifications: NotificationDiagnostic;
  connectors: ConnectorDiagnostic;
  configuration: ConfigurationDiagnostic[];
}
