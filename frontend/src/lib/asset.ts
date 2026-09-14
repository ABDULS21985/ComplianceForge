import type { AssetCollectionEnvelope, AssetPage, AssetStatus } from '@/types/asset';
import type { ApiError } from '@/lib/api';
import type { PermissionMap } from '@/types/access';
import type { User } from '@/types';

export const ASSET_RESOURCE = 'assets';

export const ASSET_API_ROUTES = {
  collection: '/assets/',
  statistics: '/assets/stats',
  detail: (assetId: string) => `/assets/${assetId}`,
  events: (assetId: string) => `/assets/${assetId}/events`,
} as const;

export function normalizeAssetCollection<T>(value: AssetCollectionEnvelope<T>): AssetPage<T> {
  if (!value || !Array.isArray(value.data) || !value.pagination) {
    throw new Error('The asset service returned an invalid collection response.');
  }
  const { page, page_size: pageSize, total_items: total, total_pages: totalPages } = value.pagination;
  if (![page, pageSize, total, totalPages].every((candidate) => Number.isInteger(candidate) && candidate >= 0)) {
    throw new Error('The asset service returned invalid pagination metadata.');
  }
  return { items: value.data, page, page_size: pageSize, total, total_pages: totalPages };
}

export function hasAssetPermission(
  permissions: PermissionMap | undefined,
  user: User | null | undefined,
  action: 'create' | 'read' | 'update' | 'delete',
): boolean {
  if (user?.is_super_admin) return true;
  return Boolean(permissions?.[ASSET_RESOURCE]?.includes(action));
}

export function assetNextStatuses(status: AssetStatus): AssetStatus[] {
  if (status === 'active') return ['inactive', 'decommissioned'];
  if (status === 'inactive') return ['active', 'decommissioned'];
  return [];
}

export function normalizeAssetTags(value: string): string[] {
  return [...new Set(value.split(',').map((tag) => tag.trim().toLowerCase()).filter(Boolean))]
    .sort()
    .slice(0, 50);
}

export function humanizeAssetToken(value: string): string {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function formatAssetError(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (!error || typeof error !== 'object') return fallback;
  const candidate = error as ApiError;
  if (candidate.detail && typeof candidate.detail === 'object') {
    const detail = candidate.detail as Record<string, unknown>;
    if (typeof detail.details === 'string' && detail.details.trim()) return detail.details;
    if (typeof detail.message === 'string' && detail.message.trim()) return detail.message;
  }
  return candidate.message || fallback;
}

export function assetPersonName(person: { first_name: string; last_name: string } | undefined, fallbackId?: string): string {
  const name = person ? `${person.first_name} ${person.last_name}`.trim() : '';
  return name || (fallbackId ? `User ${fallbackId.slice(0, 8)}` : 'Unassigned');
}
