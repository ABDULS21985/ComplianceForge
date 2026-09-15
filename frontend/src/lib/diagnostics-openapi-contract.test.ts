import type {
  ConfigurationDiagnostic,
  ConnectorDiagnostic,
  DependencyDiagnostic,
  DiagnosticsSnapshot,
  DiagnosticStatus,
  MigrationDiagnostic,
  NotificationDiagnostic,
  QueueDiagnostic,
} from '@/types/diagnostics';
import { describe, expect, it } from 'vitest';

import { DIAGNOSTICS_ROUTE } from '@/lib/diagnostics';
import { fileURLToPath } from 'node:url';
import { readFileSync } from 'node:fs';

interface OpenAPISchema {
  enum?: string[];
  properties?: Record<string, unknown>;
  required?: string[];
}

interface OpenAPIDocument {
  components: { schemas: Record<string, OpenAPISchema> };
  paths: Record<string, Record<string, unknown>>;
}

type ExactStringKeys<T, Keys extends readonly string[]> =
  Exclude<Extract<keyof T, string>, Keys[number]> extends never
    ? Exclude<Keys[number], Extract<keyof T, string>> extends never
      ? true
      : false
    : false;
type Assert<T extends true> = T;

const snapshotKeys = [
  'organization_id',
  'generated_at',
  'overall_status',
  'dependencies',
  'migration',
  'queue',
  'notifications',
  'connectors',
  'configuration',
] as const;
const dependencyKeys = [
  'key',
  'name',
  'status',
  'critical',
  'latency_ms',
  'checked_at',
  'message',
] as const;
const migrationKeys = [
  'current_version',
  'supported_version',
  'pending_count',
  'dirty',
  'status',
] as const;
const queueKeys = [
  'pending',
  'leased',
  'dead',
  'expired_leases',
  'oldest_ready_age_seconds',
  'inbox_processing',
  'inbox_expired_leases',
  'status',
] as const;
const notificationKeys = [
  'due',
  'expired_leases',
  'terminal_failures',
  'oldest_due_age_seconds',
  'status',
] as const;
const connectorKeys = [
  'total',
  'healthy',
  'degraded',
  'unhealthy',
  'unknown',
  'recent_failed_syncs',
  'status',
] as const;
const configurationKeys = ['key', 'category', 'status', 'message', 'remediation'] as const;
const statuses = ['healthy', 'warning', 'critical', 'unknown'] as const satisfies readonly DiagnosticStatus[];

const assertions: [
  Assert<ExactStringKeys<DiagnosticsSnapshot, typeof snapshotKeys>>,
  Assert<ExactStringKeys<DependencyDiagnostic, typeof dependencyKeys>>,
  Assert<ExactStringKeys<MigrationDiagnostic, typeof migrationKeys>>,
  Assert<ExactStringKeys<QueueDiagnostic, typeof queueKeys>>,
  Assert<ExactStringKeys<NotificationDiagnostic, typeof notificationKeys>>,
  Assert<ExactStringKeys<ConnectorDiagnostic, typeof connectorKeys>>,
  Assert<ExactStringKeys<ConfigurationDiagnostic, typeof configurationKeys>>,
] = [true, true, true, true, true, true, true];
void assertions;

const contractPath = fileURLToPath(new URL('../../../api/openapi/openapi.json', import.meta.url));
const contract = JSON.parse(readFileSync(contractPath, 'utf8')) as OpenAPIDocument;

function expectSchemaKeys(name: string, expected: readonly string[], required = expected): void {
  const schema = contract.components.schemas[name];
  expect(Object.keys(schema?.properties ?? {}).sort(), `${name} properties`).toEqual(
    [...expected].sort(),
  );
  expect([...(schema?.required ?? [])].sort(), `${name} required fields`).toEqual(
    [...required].sort(),
  );
}

describe('administrator diagnostics OpenAPI drift contract', () => {
  it('keeps every diagnostic DTO aligned with the generated specification', () => {
    expectSchemaKeys('DiagnosticsSnapshot', snapshotKeys);
    expectSchemaKeys('DependencyDiagnostic', dependencyKeys);
    expectSchemaKeys('MigrationDiagnostic', migrationKeys);
    expectSchemaKeys('QueueDiagnostic', queueKeys);
    expectSchemaKeys('NotificationDiagnostic', notificationKeys);
    expectSchemaKeys('ConnectorDiagnostic', connectorKeys);
    expectSchemaKeys(
      'ConfigurationDiagnostic',
      configurationKeys,
      configurationKeys.filter((key) => key !== 'remediation'),
    );
    expect(contract.components.schemas.DiagnosticStatus?.enum).toEqual(statuses);
  });

  it('documents the canonical read-only settings route and envelope', () => {
    expect(contract.paths[`/api/v1${DIAGNOSTICS_ROUTE}`]?.get).toBeDefined();
    expectSchemaKeys('DiagnosticsEnvelope', ['data']);
  });
});
