import { afterEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import api from '@/lib/api';
import type { DirectoryUser } from '@/types/directory';
import type { PaginatedDataEnvelope } from '@/types/enterprise-settings';

describe('directory user API contract', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('uses the canonical paginated active-user search through the secure BFF', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      data: [],
      pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    expectTypeOf(api.directory.listUsers).returns.toEqualTypeOf<Promise<PaginatedDataEnvelope<DirectoryUser>>>();
    await api.directory.listUsers({
      search: 'ada@example.com',
      status: 'active',
      sort_by: 'name',
      sort_dir: 'asc',
      page: 1,
      page_size: 20,
    });

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(String(fetchMock.mock.calls[0][0])).toBe(
      '/api/bff/directory/users/?search=ada%40example.com&status=active&sort_by=name&sort_dir=asc&page=1&page_size=20'
    );
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      credentials: 'same-origin',
      method: 'GET',
    });
    expect(new Headers(fetchMock.mock.calls[0][1]?.headers).has('authorization')).toBe(false);
  });
});
