import type { PermissionMap } from '@/types/access';
import type {
  Incident,
  IncidentCollectionEnvelope,
  IncidentPage,
  IncidentSeverity,
  IncidentStatus,
} from '@/types/incident';
import type { User } from '@/types';
import type { ApiError } from '@/lib/api';

export const INCIDENT_RESOURCE = 'incidents';

export const INCIDENT_API_ROUTES = {
  collection: '/incidents/',
  statistics: '/incidents/statistics',
  upcomingBreaches: '/incidents/breaches/upcoming',
  detail: (incidentId: string) => `/incidents/${incidentId}`,
  transition: (incidentId: string) => `/incidents/${incidentId}/transitions`,
  cancel: (incidentId: string) => `/incidents/${incidentId}/cancel`,
  reopen: (incidentId: string) => `/incidents/${incidentId}/reopen`,
  close: (incidentId: string) => `/incidents/${incidentId}/close`,
  escalate: (incidentId: string) => `/incidents/${incidentId}/escalate`,
  breachAssessment: (incidentId: string) => `/incidents/${incidentId}/breach-assessment`,
  notifyDpa: (incidentId: string) => `/incidents/${incidentId}/notify-dpa`,
  timeline: (incidentId: string) => `/incidents/${incidentId}/timeline`,
  assignments: (incidentId: string) => `/incidents/${incidentId}/assignments`,
  unassign: (incidentId: string, assignmentId: string) =>
    `/incidents/${incidentId}/assignments/${assignmentId}/unassign`,
} as const;

export function normalizeIncidentCollection<T>(
  value: IncidentCollectionEnvelope<T>,
): IncidentPage<T> {
  if (!value || !Array.isArray(value.data) || !value.pagination) {
    throw new Error('The incident service returned an invalid collection response.');
  }
  const { page, page_size: pageSize, total_items: total, total_pages: totalPages } =
    value.pagination;
  if (
    ![page, pageSize, total, totalPages].every(
      (candidate) => Number.isInteger(candidate) && candidate >= 0,
    )
  ) {
    throw new Error('The incident service returned invalid pagination metadata.');
  }
  return { items: value.data, page, page_size: pageSize, total, total_pages: totalPages };
}

export type IncidentPermissionAction =
  | 'create'
  | 'read'
  | 'update'
  | 'delete'
  | 'approve'
  | 'assign';

export function hasIncidentPermission(
  permissions: PermissionMap | undefined,
  user: User | null | undefined,
  action: IncidentPermissionAction,
): boolean {
  if (user?.is_super_admin) return true;
  return Boolean(permissions?.[INCIDENT_RESOURCE]?.includes(action));
}

export function incidentNextStatuses(status: IncidentStatus): IncidentStatus[] {
  switch (status) {
    case 'reported':
      return ['triaged'];
    case 'triaged':
      return ['investigating'];
    case 'investigating':
      return ['contained', 'resolved'];
    case 'contained':
      return ['investigating', 'resolved'];
    case 'resolved':
      return ['investigating'];
    default:
      return [];
  }
}

const SEVERITY_RANK: Record<IncidentSeverity, number> = {
  low: 1,
  medium: 2,
  high: 3,
  critical: 4,
};

export function incidentEscalationOptions(severity: IncidentSeverity): IncidentSeverity[] {
  return (['medium', 'high', 'critical'] as const).filter(
    (candidate) => SEVERITY_RANK[candidate] > SEVERITY_RANK[severity],
  );
}

export function humanizeIncidentToken(value: string): string {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function formatIncidentError(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (!error || typeof error !== 'object') return fallback;
  const candidate = error as ApiError;
  if (candidate.detail && typeof candidate.detail === 'object') {
    const detail = candidate.detail as Record<string, unknown>;
    if (typeof detail.details === 'string' && detail.details.trim()) return detail.details;
    if (typeof detail.message === 'string' && detail.message.trim()) return detail.message;
    if (typeof detail.error === 'string' && detail.error.trim()) return detail.error;
  }
  return candidate.message || fallback;
}

export interface DeadlineDisplay {
  state: 'none' | 'notified' | 'overdue' | 'urgent' | 'upcoming';
  hoursRemaining?: number;
}

export function incidentDeadline(
  incident: Pick<Incident, 'notification_deadline' | 'dpa_notified_at' | 'hours_remaining'>,
  now = new Date(),
): DeadlineDisplay {
  if (incident.dpa_notified_at) return { state: 'notified' };
  if (!incident.notification_deadline) return { state: 'none' };
  const deadline = new Date(incident.notification_deadline);
  if (Number.isNaN(deadline.getTime())) return { state: 'none' };
  const hoursRemaining =
    typeof incident.hours_remaining === 'number'
      ? incident.hours_remaining
      : (deadline.getTime() - now.getTime()) / 3_600_000;
  if (hoursRemaining <= 0) return { state: 'overdue', hoursRemaining };
  if (hoursRemaining <= 24) return { state: 'urgent', hoursRemaining };
  return { state: 'upcoming', hoursRemaining };
}

export function canDeleteIncident(incident: Incident, now = new Date()): boolean {
  if (!['closed', 'cancelled'].includes(incident.status) || incident.legal_hold) return false;
  if (!incident.retention_until) return true;
  const retention = new Date(incident.retention_until);
  return !Number.isNaN(retention.getTime()) && retention.getTime() <= now.getTime();
}

export function toLocalDateTimeInput(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

export function localDateTimeToIso(value: string): string | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}
