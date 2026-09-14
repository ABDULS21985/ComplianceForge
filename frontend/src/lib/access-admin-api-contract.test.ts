import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import type { ManagedRole, PermissionGrant } from '@/types/access-admin';
import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { resetCsrfToken } from '@/lib/csrf-client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('access administration API contracts', () => {
  const fetchMock = vi.fn<(input: RequestInfo | URL, request?: RequestInit) => Promise<Response>>();
  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input, request) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-access' });
      if (request?.method === 'DELETE') return new Response(null, { status: 204 });
      if (String(input).endsWith('/assignments')) return jsonResponse({ data: [] });
      if (String(input).includes('/events')) return jsonResponse({ data: [], pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 } });
      if (String(input).endsWith('/permissions')) return jsonResponse({ data: [] });
      if (String(input).includes('/impact-preview')) return jsonResponse({ role_id: 'role-1', assigned_users: 2, current_permissions: 2, proposed_permissions: 1, added: [], removed: [] });
      return jsonResponse({ id: 'role-1', version: 2, data: [], pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 } });
    });
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('uses the canonical catalogue and paginated role list query', async () => {
    expectTypeOf(api.access.permissionCatalogue).returns.toEqualTypeOf<Promise<{ data: PermissionGrant[] }>>();
    expectTypeOf(api.access.getRole).returns.toEqualTypeOf<Promise<ManagedRole>>();
    await api.access.permissionCatalogue();
    await api.access.listRoles({ search: 'reviewer', include_system: false, page: 2, page_size: 20 });
    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      '/api/bff/access/permissions',
      '/api/bff/access/roles?search=reviewer&include_system=false&page=2&page_size=20',
    ]);
  });

  it('sends versioned role updates and deletion through the secure BFF', async () => {
    await api.access.updateRole('role-1', { expected_version: 4, name: 'Reviewer' });
    await api.access.deleteRole('role-1', 5);
    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/access/roles/role-1', 'PATCH'],
      ['/api/bff/access/roles/role-1?expected_version=5', 'DELETE'],
    ]);
    expect(JSON.parse(String(calls[0][1]?.body))).toEqual({ expected_version: 4, name: 'Reviewer' });
    for (const [, request] of calls) {
      const headers = new Headers(request?.headers);
      expect(headers.get(CSRF_TOKEN_HEADER)).toBe('csrf-access');
      expect(headers.has('authorization')).toBe(false);
    }
  });

  it('uses canonical create, detail, clone, and replace routes', async () => {
    await api.access.createRole({ name: 'Reviewer', permissions: [] });
    await api.access.getRole('role-1');
    await api.access.cloneRole('role-1', { name: 'Reviewer copy' });
    await api.access.replaceRole('role-1', { expected_version: 2, permissions: [] });
    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/access/roles', 'POST'],
      ['/api/bff/access/roles/role-1', 'GET'],
      ['/api/bff/access/roles/role-1/clone', 'POST'],
      ['/api/bff/access/roles/role-1', 'PUT'],
    ]);
  });

  it('uses exact impact, assignment, unassignment, and history contracts', async () => {
    await api.access.previewRoleImpact('role-1', [{ resource: 'audits', action: 'read' }]);
    await api.access.assignRole('role-1', { user_id: 'user-1', reason: 'New reviewer duties' });
    await api.access.unassignRole('role-1', 'user-1', { reason: 'Moved teams' });
    await api.access.listRoleEvents('role-1', { page: 3, page_size: 10 });
    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/access/roles/role-1/impact-preview', 'POST'],
      ['/api/bff/access/roles/role-1/assignments', 'POST'],
      ['/api/bff/access/roles/role-1/assignments/user-1', 'DELETE'],
      ['/api/bff/access/roles/role-1/events?page=3&page_size=10', 'GET'],
    ]);
    expect(JSON.parse(String(calls[2][1]?.body))).toEqual({ reason: 'Moved teams' });
  });
});
