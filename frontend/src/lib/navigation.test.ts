import { describe, expect, it } from 'vitest';

import {
  getEntityRoute,
  getNavigationItemForPath,
  getVisibleNavigationGroups,
  NAVIGATION_GROUPS,
  NAVIGATION_ITEMS,
  normalizeGlobalAutocompleteResults,
  normalizeGlobalSearchResponse,
} from '@/lib/navigation';

describe('enterprise navigation model', () => {
  it('groups every canonical destination and assigns a unique icon', () => {
    expect(NAVIGATION_GROUPS).toHaveLength(6);
    expect(NAVIGATION_ITEMS).toHaveLength(28);
    expect(new Set(NAVIGATION_ITEMS.map((item) => item.id)).size).toBe(28);
    expect(new Set(NAVIGATION_ITEMS.map((item) => item.icon)).size).toBe(28);
  });

  it('uses resolved permissions to hide inaccessible domains', () => {
    const groups = getVisibleNavigationGroups({
      isSuperAdmin: false,
      roleSlugs: ['viewer'],
      permissions: {
        organization: ['read'],
        report: [],
        risk: ['read'],
      },
    });
    const ids = groups.flatMap((group) => group.items.map((item) => item.id));

    expect(ids).toContain('risks');
    expect(ids).not.toContain('frameworks');
    expect(ids).not.toContain('board');
    expect(ids).not.toContain('settings');
  });

  it('keeps all navigation available to a super administrator', () => {
    const groups = getVisibleNavigationGroups({
      isSuperAdmin: true,
      roleSlugs: [],
      permissions: {},
    });

    expect(groups.flatMap((group) => group.items)).toHaveLength(28);
  });

  it('selects the most specific navigation item for nested settings paths', () => {
    expect(
      getNavigationItemForPath('/settings/notifications/email')?.id
    ).toBe('notifications');
  });
});

describe('search response contracts', () => {
  it('normalizes the backend autocomplete envelope', () => {
    expect(
      normalizeGlobalAutocompleteResults({
        data: [
          {
            entity_type: 'vendor',
            entity_id: 'vendor-1',
            title: 'Acme Hosting',
            subtitle: 'Critical supplier',
          },
        ],
      })
    ).toEqual([
      {
        entity_type: 'vendor',
        entity_id: 'vendor-1',
        title: 'Acme Hosting',
        subtitle: 'Critical supplier',
        entity_ref: undefined,
      },
    ]);
  });

  it('normalizes the backend full-search contract and canonical route', () => {
    const response = normalizeGlobalSearchResponse({
      results: [
        {
          entity_type: 'risk',
          entity_id: 'risk-1',
          title: 'Cloud concentration',
          description: 'A material supplier dependency',
        },
      ],
      total_hits: 21,
      page: 2,
      page_size: 20,
      time_taken_ms: 7,
    });

    expect(response).toMatchObject({
      total: 21,
      page: 2,
      total_pages: 2,
      query_time_ms: 7,
    });
    expect(getEntityRoute(response.items[0])).toBe('/risks/risk-1');
  });
});
