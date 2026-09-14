import type {
  AuditCollectionEnvelope,
  AuditFinding,
  AuditLifecycleAction,
  AuditPage,
  AuditStatus,
  FindingStatus,
} from '@/types/audit';
import type { ApiError } from '@/lib/api';
import type { PermissionMap } from '@/types/access';
import type { User } from '@/types';

export const AUDIT_RESOURCE = 'audits';

export const AUDIT_API_ROUTES = {
  collection: '/audits/',
  detail: (auditId: string) => `/audits/${auditId}`,
  lifecycle: (auditId: string, action: AuditLifecycleAction) => `/audits/${auditId}/${action}`,
  findings: (auditId: string) => `/audits/${auditId}/findings`,
  findingStats: (auditId: string) => `/audits/${auditId}/findings/stats`,
  finding: (auditId: string, findingId: string) => `/audits/${auditId}/findings/${findingId}`,
} as const;

export function normalizeAuditCollection<T>(value: AuditCollectionEnvelope<T>): AuditPage<T> {
  if (!value || !Array.isArray(value.data) || !value.pagination) {
    throw new Error('The audit service returned an invalid collection response.');
  }

  const {
    page,
    page_size: pageSize,
    total_items: total,
    total_pages: totalPages,
  } = value.pagination;
  if (
    ![page, pageSize, total, totalPages].every(
      (candidate) => Number.isInteger(candidate) && candidate >= 0,
    )
  ) {
    throw new Error('The audit service returned invalid pagination metadata.');
  }

  return {
    items: value.data,
    page,
    page_size: pageSize,
    total,
    total_pages: totalPages,
  };
}

export function hasAuditPermission(
  permissions: PermissionMap | undefined,
  user: User | null | undefined,
  action: 'create' | 'read' | 'update' | 'delete',
): boolean {
  if (user?.is_super_admin) return true;
  return Boolean(permissions?.[AUDIT_RESOURCE]?.includes(action));
}

export function auditLifecycleActions(status: AuditStatus): AuditLifecycleAction[] {
  switch (status) {
    case 'planned':
      return ['start', 'cancel'];
    case 'in_progress':
      return ['complete'];
    case 'completed':
      return ['close'];
    default:
      return [];
  }
}

export function findingNextStatuses(status: FindingStatus): FindingStatus[] {
  switch (status) {
    case 'open':
      return ['in_progress', 'resolved', 'accepted'];
    case 'in_progress':
      return ['open', 'resolved', 'accepted'];
    case 'resolved':
      return ['in_progress', 'closed'];
    case 'accepted':
      return ['in_progress', 'closed'];
    default:
      return [];
  }
}

export function isFindingOverdue(
  finding: Pick<AuditFinding, 'due_date' | 'status'>,
  now = new Date(),
): boolean {
  if (!finding.due_date || ['resolved', 'closed', 'accepted'].includes(finding.status)) {
    return false;
  }
  const due = new Date(finding.due_date);
  if (Number.isNaN(due.getTime())) return false;
  const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  return due.getTime() < today.getTime();
}

export function formatAuditError(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (!error || typeof error !== 'object') return fallback;

  const candidate = error as ApiError;
  const detail = candidate.detail;
  if (detail && typeof detail === 'object') {
    const record = detail as Record<string, unknown>;
    if (typeof record.details === 'string' && record.details.trim()) return record.details;
    if (typeof record.message === 'string' && record.message.trim()) return record.message;
  }
  return candidate.message || fallback;
}

export function auditPersonName(
  person: { first_name: string; last_name: string } | undefined,
  fallbackId?: string,
): string {
  const name = person ? `${person.first_name} ${person.last_name}`.trim() : '';
  return name || (fallbackId ? `User ${fallbackId.slice(0, 8)}` : 'Unassigned');
}

export function toDateInputValue(value: string | undefined): string {
  return value ? value.slice(0, 10) : '';
}

export function humanizeAuditToken(value: string): string {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}
