import {
  connectorRemediation,
  diagnosticsFreshness,
  diagnosticsPollInterval,
  formatDiagnosticAge,
  formatDiagnosticLatency,
  migrationRemediation,
  notificationRemediation,
  queueRemediation,
  statusCounts,
} from '@/lib/diagnostics';
import { describe, expect, it } from 'vitest';
import type { DiagnosticsSnapshot } from '@/types/diagnostics';

const snapshot: DiagnosticsSnapshot = {
  organization_id: '7a2423cd-bdeb-472f-a60a-1b6cfec87aa8',
  generated_at: '2026-09-14T11:59:30Z',
  overall_status: 'critical',
  dependencies: [
    {
      key: 'database',
      name: 'Primary database',
      status: 'healthy',
      critical: true,
      latency_ms: 4,
      checked_at: '2026-09-14T11:59:29Z',
      message: 'Available.',
    },
    {
      key: 'object_storage',
      name: 'Object storage',
      status: 'unknown',
      critical: false,
      latency_ms: 0,
      checked_at: '2026-09-14T11:59:28Z',
      message: 'Probe unavailable.',
    },
  ],
  migration: {
    current_version: 52,
    supported_version: 53,
    pending_count: 1,
    dirty: false,
    status: 'warning',
  },
  queue: {
    pending: 8,
    leased: 2,
    dead: 1,
    expired_leases: 0,
    oldest_ready_age_seconds: 400,
    inbox_processing: 3,
    inbox_expired_leases: 0,
    status: 'critical',
  },
  notifications: {
    due: 4,
    expired_leases: 0,
    terminal_failures: 2,
    oldest_due_age_seconds: 901,
    status: 'warning',
  },
  connectors: {
    total: 2,
    healthy: 1,
    degraded: 1,
    unhealthy: 0,
    unknown: 0,
    recent_failed_syncs: 1,
    status: 'warning',
  },
  configuration: [
    {
      key: 'webhook_signing',
      category: 'security',
      status: 'healthy',
      message: 'Signing is configured.',
    },
  ],
};

describe('administrator diagnostics helpers', () => {
  it('backs polling off without polling hidden or offline documents', () => {
    expect(diagnosticsPollInterval(0, false, true)).toBe(60_000);
    expect(diagnosticsPollInterval(2, false, true)).toBe(240_000);
    expect(diagnosticsPollInterval(20, false, true)).toBe(300_000);
    expect(diagnosticsPollInterval(0, true, true)).toBe(false);
    expect(diagnosticsPollInterval(0, false, false)).toBe(false);
  });

  it('distinguishes fresh, stale, invalid, and future timestamps', () => {
    const now = new Date('2026-09-14T12:00:00Z');
    expect(diagnosticsFreshness('2026-09-14T11:58:00Z', now)).toEqual({
      ageSeconds: 120,
      state: 'fresh',
    });
    expect(diagnosticsFreshness('2026-09-14T11:57:59Z', now).state).toBe('stale');
    expect(diagnosticsFreshness('2026-09-14T12:02:00Z', now).state).toBe('clock_skew');
    expect(diagnosticsFreshness('not-a-date', now)).toEqual({
      ageSeconds: null,
      state: 'unknown',
    });
  });

  it('formats bounded latency and age values for operators', () => {
    expect(formatDiagnosticLatency(0)).toBe('<1 ms');
    expect(formatDiagnosticLatency(850)).toBe('850 ms');
    expect(formatDiagnosticLatency(1_500)).toBe('1.5 s');
    expect(formatDiagnosticLatency(-1)).toBe('Unknown');
    expect(formatDiagnosticAge(59)).toBe('59s');
    expect(formatDiagnosticAge(120)).toBe('2m');
    expect(formatDiagnosticAge(7_200)).toBe('2h');
    expect(formatDiagnosticAge(172_800)).toBe('2d');
  });

  it('maps status totals and actionable remediation from the snapshot', () => {
    expect(statusCounts(snapshot)).toEqual({ healthy: 2, warning: 3, critical: 1, unknown: 1 });
    expect(migrationRemediation(snapshot.migration)).toContain('1 pending migration');
    expect(queueRemediation(snapshot.queue)).toContain('dead jobs');
    expect(notificationRemediation(snapshot.notifications)).toContain('delivery logs');
    expect(connectorRemediation(snapshot.connectors)).toContain('Integration Hub');
  });
});
