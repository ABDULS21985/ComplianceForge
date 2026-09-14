import type {
  ManagedRole,
  PermissionGrant,
} from '@/types/access-admin';
import type { ApiError } from '@/lib/api';
import type { PermissionMap } from '@/types/access';
import type { User } from '@/types';

export const ACCESS_ADMIN_ROUTES = {
  permissions: '/access/permissions',
  roles: '/access/roles',
  role: (roleId: string) => `/access/roles/${roleId}`,
  clone: (roleId: string) => `/access/roles/${roleId}/clone`,
  impact: (roleId: string) => `/access/roles/${roleId}/impact-preview`,
  assignments: (roleId: string) => `/access/roles/${roleId}/assignments`,
  assignment: (roleId: string, userId: string) =>
    `/access/roles/${roleId}/assignments/${userId}`,
  events: (roleId: string) => `/access/roles/${roleId}/events`,
} as const;

export const accessAdminKeys = {
  all: ['access-admin'] as const,
  permissions: ['access-admin', 'permissions'] as const,
  roles: (params: Record<string, unknown>) => ['access-admin', 'roles', params] as const,
  role: (roleId: string) => ['access-admin', 'role', roleId] as const,
  assignments: (roleId: string) => ['access-admin', 'assignments', roleId] as const,
  events: (roleId: string, page: number) => ['access-admin', 'events', roleId, page] as const,
};

export function hasAccessAdministrationPermission(
  permissions: PermissionMap | undefined,
  user: User | null | undefined,
  action: 'read' | 'configure'
): boolean {
  if (user?.is_super_admin) return true;
  const explicit = permissions?.settings;
  if (explicit) return explicit.includes(action);
  return Boolean(user?.roles?.some((role) => role.slug === 'org_admin'));
}

export function permissionKey(permission: Pick<PermissionGrant, 'resource' | 'action'>): string {
  return `${permission.resource}:${permission.action}`;
}

export function roleHasPermission(role: ManagedRole, resource: string, action: string): boolean {
  return role.permissions.some(
    (permission) => permission.resource === resource && permission.action === action
  );
}

export function permissionSelection(permissions: readonly PermissionGrant[]): PermissionGrant[] {
  const unique = new Map<string, PermissionGrant>();
  for (const permission of permissions) {
    const resource = permission.resource.trim().toLowerCase();
    const action = permission.action.trim().toLowerCase();
    if (resource && action) unique.set(`${resource}:${action}`, { resource, action });
  }
  return [...unique.values()].sort((left, right) =>
    permissionKey(left).localeCompare(permissionKey(right))
  );
}

export function removedPermissions(
  current: readonly PermissionGrant[],
  proposed: readonly PermissionGrant[]
): PermissionGrant[] {
  const proposedKeys = new Set(proposed.map(permissionKey));
  return current.filter((permission) => !proposedKeys.has(permissionKey(permission)));
}

export function groupPermissions(
  permissions: readonly PermissionGrant[]
): Array<{ resource: string; permissions: PermissionGrant[] }> {
  const grouped = new Map<string, PermissionGrant[]>();
  for (const permission of permissions) {
    const current = grouped.get(permission.resource) ?? [];
    current.push(permission);
    grouped.set(permission.resource, current);
  }
  return [...grouped.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([resource, values]) => ({
      resource,
      permissions: [...values].sort((left, right) => left.action.localeCompare(right.action)),
    }));
}

export function humanizeAccessToken(value: string): string {
  return value.replace(/[_-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

interface AccessErrorDetail {
  details?: unknown;
  error_code?: unknown;
  message?: unknown;
  request_id?: unknown;
}

export function accessErrorDetails(error: unknown): AccessErrorDetail & { status?: number } {
  if (!error || typeof error !== 'object') return {};
  const apiError = error as ApiError;
  const detail = apiError.detail;
  const response = detail && typeof detail === 'object' ? detail as AccessErrorDetail : {};
  return { ...response, status: apiError.status };
}

export function isAccessConflict(error: unknown): boolean {
  return accessErrorDetails(error).status === 409;
}

export function formatAccessError(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (!error || typeof error !== 'object') return fallback;
  const apiError = error as ApiError;
  const details = accessErrorDetails(error);
  const actionable = typeof details.details === 'string' && details.details
    ? details.details
    : typeof details.message === 'string' && details.message
      ? details.message
      : apiError.message || fallback;
  const requestId = typeof details.request_id === 'string' && details.request_id
    ? ` Reference: ${details.request_id}.`
    : '';
  return `${actionable}${requestId}`;
}
