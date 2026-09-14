import { assetNextStatuses, hasAssetPermission, normalizeAssetCollection, normalizeAssetTags } from '@/lib/asset';
import { describe, expect, it } from 'vitest';
import type { User } from '@/types';

const USER = { id: 'user-1', organization_id: 'org-1', email: 'owner@example.test', first_name: 'Ari', last_name: 'Owner', status: 'active', is_super_admin: false, language: 'en', created_at: '2026-09-14T00:00:00Z', updated_at: '2026-09-14T00:00:00Z' } satisfies User;

describe('asset contract helpers', () => {
  it('normalizes the handler data/pagination envelope and rejects drift', () => {
    expect(normalizeAssetCollection({ data: [{ id: 'asset-1' }], pagination: { page: 2, page_size: 10, total_items: 12, total_pages: 2 } })).toEqual({ items: [{ id: 'asset-1' }], page: 2, page_size: 10, total: 12, total_pages: 2 });
    expect(() => normalizeAssetCollection({ data: null } as never)).toThrow('invalid collection');
    expect(() => normalizeAssetCollection({ data: [], pagination: { page: 1, page_size: 10, total_items: -1, total_pages: 0 } })).toThrow('invalid pagination');
  });

  it('matches the one-way decommissioning state machine', () => {
    expect(assetNextStatuses('active')).toEqual(['inactive', 'decommissioned']);
    expect(assetNextStatuses('inactive')).toEqual(['active', 'decommissioned']);
    expect(assetNextStatuses('decommissioned')).toEqual([]);
  });

  it('normalizes, deduplicates, sorts, and bounds API tags', () => {
    expect(normalizeAssetTags(' Production, pii,production, EU-West ')).toEqual(['eu-west', 'pii', 'production']);
    expect(normalizeAssetTags(Array.from({ length: 55 }, (_, index) => `tag-${index}`).join(','))).toHaveLength(50);
  });

  it('fails closed on absent or incorrectly singular permissions', () => {
    expect(hasAssetPermission(undefined, USER, 'read')).toBe(false);
    expect(hasAssetPermission({ asset: ['read'] }, USER, 'read')).toBe(false);
    expect(hasAssetPermission({ assets: ['read', 'update'] }, USER, 'update')).toBe(true);
    expect(hasAssetPermission({}, { ...USER, is_super_admin: true }, 'delete')).toBe(true);
  });
});
