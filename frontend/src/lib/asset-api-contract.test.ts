import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { resetCsrfToken } from '@/lib/csrf-client';

function jsonResponse(body: unknown, status = 200): Response { return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }); }

describe('asset API route contracts', () => {
  const fetchMock = vi.fn<(input: RequestInfo | URL, request?: RequestInit) => Promise<Response>>();
  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input, request) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-asset' });
      if (request?.method === 'DELETE') return new Response(null, { status: 204 });
      return jsonResponse({ id: 'asset-1', version: 2, data: [], pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 } });
    });
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('uses every canonical list filter and normalizes pagination', async () => {
    const page = await api.assets.list({ page: 2, page_size: 20, asset_type: 'data', criticality: 'critical', classification: 'restricted', status: 'active', tag: 'pii', search: 'customer', processes_personal_data: true, sort_by: 'name', sort_dir: 'asc' });
    expect(page).toEqual({ items: [], page: 1, page_size: 20, total: 0, total_pages: 0 });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/bff/assets/?page=2&page_size=20&asset_type=data&criticality=critical&classification=restricted&status=active&tag=pii&search=customer&processes_personal_data=true&sort_by=name&sort_dir=asc');
  });

  it('uses canonical create, versioned patch, and versioned delete contracts', async () => {
    await api.assets.create({ name: 'Customer database', asset_type: 'data', criticality: 'critical', classification: 'restricted', processes_personal_data: true });
    await api.assets.update('asset-1', { expected_version: 4, status: 'inactive' });
    await api.assets.delete('asset-1', 5);
    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/assets/', 'POST'], ['/api/bff/assets/asset-1', 'PATCH'], ['/api/bff/assets/asset-1?expected_version=5', 'DELETE'],
    ]);
    for (const [, request] of calls) {
      expect(new Headers(request?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-asset');
      expect(new Headers(request?.headers).has('authorization')).toBe(false);
    }
    expect(JSON.parse(String(calls[1][1]?.body))).toEqual({ expected_version: 4, status: 'inactive' });
  });

  it('uses dedicated statistics and paginated history routes', async () => {
    await api.assets.stats();
    const page = await api.assets.events('asset-1', { page: 3, page_size: 10 });
    expect(page).toMatchObject({ page: 1, page_size: 20, total: 0, total_pages: 0 });
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/bff/assets/stats', '/api/bff/assets/asset-1/events?page=3&page_size=10']);
  });
});
