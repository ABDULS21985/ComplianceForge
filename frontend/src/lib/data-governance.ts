import type {
  DataClassification,
  GovernanceRecordType,
  RetentionTrigger,
} from '@/types/data-governance';
import type { ApiError } from '@/lib/api';

export const DATA_GOVERNANCE_CAPABILITY = 'data_lifecycle';

export const DATA_GOVERNANCE_ROUTES = {
  root: '/settings/data-governance',
  policy: '/settings/data-governance/policy',
  schedules: '/settings/data-governance/retention-schedules',
  schedule: (id: string) =>
    `/settings/data-governance/retention-schedules/${encodeURIComponent(id)}`,
  assignments: '/settings/data-governance/retention-assignments',
  assignment: (id: string) =>
    `/settings/data-governance/retention-assignments/${encodeURIComponent(id)}`,
  reviewAssignment: (id: string) =>
    `/settings/data-governance/retention-assignments/${encodeURIComponent(id)}/review`,
  assignmentExceptions: (id: string) =>
    `/settings/data-governance/retention-assignments/${encodeURIComponent(id)}/exceptions`,
  decideException: (id: string) =>
    `/settings/data-governance/retention-exceptions/${encodeURIComponent(id)}/decision`,
  disposition: (recordType: string, recordId: string) =>
    `/settings/data-governance/records/${encodeURIComponent(recordType)}/${encodeURIComponent(recordId)}/disposition`,
  holds: '/settings/data-governance/legal-holds',
  hold: (id: string) => `/settings/data-governance/legal-holds/${encodeURIComponent(id)}`,
  releaseHold: (id: string) =>
    `/settings/data-governance/legal-holds/${encodeURIComponent(id)}/release`,
  holdRecords: (id: string) =>
    `/settings/data-governance/legal-holds/${encodeURIComponent(id)}/records`,
  releaseHoldRecord: (holdId: string, recordId: string) =>
    `/settings/data-governance/legal-holds/${encodeURIComponent(holdId)}/records/${encodeURIComponent(recordId)}/release`,
  events: '/settings/data-governance/events',
  verifyEvents: '/settings/data-governance/events/verify',
} as const;

export const dataGovernanceKeys = {
  all: ['data-governance'] as const,
  capability: ['feature-flags', 'evaluation', DATA_GOVERNANCE_CAPABILITY] as const,
  policy: ['data-governance', 'policy'] as const,
  schedules: (params: object) => ['data-governance', 'schedules', params] as const,
  assignment: (id: string) => ['data-governance', 'assignment', id] as const,
  exceptions: (id: string) => ['data-governance', 'assignment', id, 'exceptions'] as const,
  disposition: (recordType: string, recordId: string) =>
    ['data-governance', 'disposition', recordType, recordId] as const,
  holds: (params: object) => ['data-governance', 'holds', params] as const,
  hold: (id: string) => ['data-governance', 'hold', id] as const,
  holdRecords: (id: string, activeOnly: boolean) =>
    ['data-governance', 'hold', id, 'records', activeOnly] as const,
  events: (params: object) => ['data-governance', 'events', params] as const,
  verification: ['data-governance', 'verification'] as const,
};

export const GOVERNANCE_RECORD_TYPES: readonly GovernanceRecordType[] = [
  'asset',
  'audit',
  'audit_finding',
  'comment',
  'control',
  'evidence',
  'incident',
  'policy',
  'report',
  'risk',
  'vendor',
];

export const DATA_CLASSIFICATIONS: readonly DataClassification[] = [
  'public',
  'internal',
  'confidential',
  'restricted',
  'personal',
  'special_category',
];

export const RETENTION_TRIGGERS: readonly RetentionTrigger[] = [
  'record_created',
  'record_closed',
  'contract_ended',
  'employment_ended',
  'consent_withdrawn',
  'superseded',
  'case_closed',
];

export function humanizeGovernanceToken(value: string): string {
  return value.replace(/[_.-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function formatGovernanceDate(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
    date,
  );
}

export function formatGovernanceError(error: unknown, fallback: string): string {
  if (!error || typeof error !== 'object') return fallback;
  const candidate = error as ApiError;
  const detail =
    candidate.detail && typeof candidate.detail === 'object'
      ? (candidate.detail as Record<string, unknown>)
      : undefined;
  const message =
    typeof detail?.details === 'string' && detail.details
      ? detail.details
      : typeof detail?.detail === 'string' && detail.detail
        ? detail.detail
        : typeof detail?.message === 'string' && detail.message
          ? detail.message
          : candidate.message || fallback;
  const requestId =
    typeof detail?.request_id === 'string' && detail.request_id
      ? ` Reference: ${detail.request_id}.`
      : '';
  return `${message}${requestId}`;
}

export function isGovernanceConflict(error: unknown): boolean {
  return Boolean(error && typeof error === 'object' && (error as ApiError).status === 409);
}

export function parseGovernanceObject(value: string, label: string): Record<string, unknown> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || '{}');
  } catch {
    throw new Error(`${label} must be valid JSON.`);
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(`${label} must be a JSON object.`);
  }
  return parsed as Record<string, unknown>;
}

export function localDateTimeInput(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return '';
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.valueOf() - offset).toISOString().slice(0, 16);
}

export function toISOStringOrUndefined(value: string): string | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? undefined : date.toISOString();
}
