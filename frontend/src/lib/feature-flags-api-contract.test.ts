import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import type { EntitlementSnapshot, FeatureFlagEvaluation } from '@/types/feature-flag';
import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { resetCsrfToken } from '@/lib/csrf-client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('feature flag and entitlement API contracts', () => {
  const fetchMock = vi.fn<(input: RequestInfo | URL, request?: RequestInit) => Promise<Response>>();
  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input, request) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-feature' });
      if (request?.method === 'POST' && String(input).endsWith('/reset')) return new Response(null, { status: 204 });
      if (String(input).includes('/history')) return jsonResponse({ data: [], pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 } });
      if (String(input).endsWith('/capabilities') || String(input).endsWith('/feature-flags')) return jsonResponse({ data: [] });
      return jsonResponse({});
    });
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('types and calls catalogue, evaluation, entitlement, and limit routes', async () => {
    expectTypeOf(api.featureFlags.evaluate).returns.toEqualTypeOf<Promise<FeatureFlagEvaluation>>();
    expectTypeOf(api.featureFlags.entitlements).returns.toEqualTypeOf<Promise<EntitlementSnapshot>>();
    await api.featureFlags.listCapabilities();
    await api.featureFlags.listEvaluations();
    await api.featureFlags.evaluate('advanced_reporting');
    await api.featureFlags.entitlements();
    await api.featureFlags.checkLimit('users', 2);
    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      '/api/bff/settings/capabilities',
      '/api/bff/settings/feature-flags',
      '/api/bff/settings/capabilities/advanced_reporting/evaluation',
      '/api/bff/settings/entitlements',
      '/api/bff/settings/entitlements/limits/users/check?requested=2',
    ]);
  });

  it('omits expected_version on create and includes it for update and reset', async () => {
    await api.featureFlags.upsertOverride('advanced_reporting', { enabled: true, reason: 'Enable pilot', variant: {} });
    await api.featureFlags.upsertOverride('advanced_reporting', { enabled: false, reason: 'Pause pilot', expected_version: 4 });
    await api.featureFlags.resetOverride('advanced_reporting', { expected_version: 5, reason: 'Return to default' });
    await api.featureFlags.history('advanced_reporting', { page: 2, page_size: 20 });
    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/settings/feature-flags/advanced_reporting', 'PUT'],
      ['/api/bff/settings/feature-flags/advanced_reporting', 'PUT'],
      ['/api/bff/settings/feature-flags/advanced_reporting/reset', 'POST'],
      ['/api/bff/settings/feature-flags/advanced_reporting/history?page=2&page_size=20', 'GET'],
    ]);
    expect(JSON.parse(String(calls[0][1]?.body))).not.toHaveProperty('expected_version');
    expect(JSON.parse(String(calls[1][1]?.body))).toHaveProperty('expected_version', 4);
    expect(JSON.parse(String(calls[2][1]?.body))).toEqual({ expected_version: 5, reason: 'Return to default' });
    for (const [, request] of calls.slice(0, 3)) {
      expect(new Headers(request?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-feature');
      expect(new Headers(request?.headers).has('authorization')).toBe(false);
    }
  });
});
