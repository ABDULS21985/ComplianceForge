import type {
  ConnectorDiagnostic,
  DependencyDiagnostic,
  DiagnosticsSnapshot,
  DiagnosticStatus,
  MigrationDiagnostic,
  NotificationDiagnostic,
  QueueDiagnostic,
} from '@/types/diagnostics';

export const DIAGNOSTICS_ROUTE = '/settings/diagnostics';

export const diagnosticsKeys = {
  snapshot: ['settings', 'diagnostics'] as const,
};

export type DiagnosticsFreshness = 'fresh' | 'stale' | 'clock_skew' | 'unknown';

export function diagnosticsPollInterval(
  failureCount: number,
  hidden: boolean,
  online: boolean,
): number | false {
  if (hidden || !online) return false;
  return Math.min(5 * 60_000, 60_000 * 2 ** Math.min(Math.max(failureCount, 0), 3));
}

export function diagnosticsFreshness(
  generatedAt: string,
  now = new Date(),
): { ageSeconds: number | null; state: DiagnosticsFreshness } {
  const generated = new Date(generatedAt);
  if (Number.isNaN(generated.valueOf())) return { ageSeconds: null, state: 'unknown' };
  const ageSeconds = Math.round((now.valueOf() - generated.valueOf()) / 1000);
  if (ageSeconds < -60) return { ageSeconds, state: 'clock_skew' };
  return { ageSeconds: Math.max(0, ageSeconds), state: ageSeconds <= 120 ? 'fresh' : 'stale' };
}

export function formatDiagnosticLatency(milliseconds: number): string {
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return 'Unknown';
  if (milliseconds < 1) return '<1 ms';
  if (milliseconds < 1000) return `${Math.round(milliseconds)} ms`;
  return `${(milliseconds / 1000).toFixed(milliseconds < 10_000 ? 1 : 0)} s`;
}

export function formatDiagnosticAge(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return 'Unknown';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86_400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86_400)}d`;
}

export function humanizeDiagnosticToken(value: string): string {
  return value.replace(/[_.-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function statusCounts(snapshot: DiagnosticsSnapshot): Record<DiagnosticStatus, number> {
  const counts: Record<DiagnosticStatus, number> = {
    healthy: 0,
    warning: 0,
    critical: 0,
    unknown: 0,
  };
  const statuses = [
    snapshot.migration.status,
    snapshot.queue.status,
    snapshot.notifications.status,
    snapshot.connectors.status,
    ...snapshot.dependencies.map((item) => item.status),
    ...snapshot.configuration.map((item) => item.status),
  ];
  for (const status of statuses) counts[status] += 1;
  return counts;
}

export function dependencyRemediation(item: DependencyDiagnostic): string {
  if (item.status === 'healthy') return 'No action is required.';
  return item.critical
    ? 'Treat this as service-impacting. Use the support reference from server logs and restore the dependency before retrying affected workflows.'
    : 'Review the dependency in platform operations and restore it before using related optional workflows.';
}

export function migrationRemediation(item: MigrationDiagnostic): string {
  if (item.dirty)
    return 'Stop application rollouts and have an operator repair the dirty migration before serving further schema changes.';
  if (item.current_version > item.supported_version)
    return 'Deploy an application version that supports this database schema; do not attempt an automatic downgrade.';
  if (item.pending_count > 0)
    return `Apply the ${item.pending_count} pending migration${item.pending_count === 1 ? '' : 's'} through the controlled deployment process.`;
  return 'The database schema matches this application version.';
}

export function queueRemediation(item: QueueDiagnostic): string {
  if (item.dead > 0)
    return 'Inspect dead jobs using tenant-safe operational tooling, remediate their cause, then replay only confirmed idempotent work.';
  if (item.expired_leases > 0 || item.inbox_expired_leases > 0)
    return 'Check worker availability and lease renewal health before retrying abandoned work.';
  if (item.oldest_ready_age_seconds >= 300)
    return 'Review worker capacity and queue consumers; the oldest ready item is outside the expected processing window.';
  return 'Queue and inbox processing are within expected thresholds.';
}

export function notificationRemediation(item: NotificationDiagnostic): string {
  if (item.expired_leases > 0)
    return 'Restore notification workers and review expired leases before retrying delivery.';
  if (item.terminal_failures > 0)
    return 'Review redacted delivery logs and channel posture, then retry only after fixing the destination or transport.';
  if (item.oldest_due_age_seconds >= 900)
    return 'Check notification worker capacity and channel health; due work is aging.';
  return 'Notification delivery is within expected thresholds.';
}

export function connectorRemediation(item: ConnectorDiagnostic): string {
  if (item.unhealthy > 0)
    return 'Open Integration Hub, test affected connectors, and restore credentials or upstream availability before syncing again.';
  if (item.degraded > 0 || item.unknown > 0 || item.recent_failed_syncs > 0)
    return 'Review Integration Hub test results and recent sync history for affected connectors.';
  return item.total === 0
    ? 'No connectors are configured for this organization.'
    : 'Configured connectors report healthy posture.';
}
