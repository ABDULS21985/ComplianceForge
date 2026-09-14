import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { resetCsrfToken } from '@/lib/csrf-client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('audit API route contracts', () => {
  const fetchMock = vi.fn<
    (input: RequestInfo | URL, request?: RequestInit) => Promise<Response>
  >();

  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-audit' });
      return jsonResponse({
        data: [],
        pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('uses the canonical collection route and normalizes audit pagination', async () => {
    const page = await api.audits.list({
      page: 2,
      page_size: 20,
      search: 'SOC 2',
      status: 'planned',
      audit_type: 'external',
    });

    expect(page).toEqual({ items: [], page: 1, page_size: 20, total: 0, total_pages: 0 });
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/bff/audits/?page=2&page_size=20&search=SOC+2&status=planned&audit_type=external',
    );
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'GET', credentials: 'same-origin' });
  });

  it('uses PATCH for edits and PUT for explicit lifecycle transitions', async () => {
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-audit' });
      return jsonResponse({ id: 'audit-1', status: 'in_progress' });
    });

    await api.audits.update('audit-1', { title: 'Updated scope' });
    await api.audits.transition('audit-1', 'start');

    const mutationCalls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(mutationCalls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/audits/audit-1', 'PATCH'],
      ['/api/bff/audits/audit-1/start', 'PUT'],
    ]);
    for (const [, request] of mutationCalls) {
      expect(new Headers(request?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-audit');
      expect(new Headers(request?.headers).has('authorization')).toBe(false);
    }
  });

  it('uses canonical read routes for an audit and its nested finding', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ id: 'record-1' }));

    await api.audits.get('audit-1');
    await api.audits.getFinding('audit-1', 'finding-1');

    expect(fetchMock.mock.calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/audits/audit-1', 'GET'],
      ['/api/bff/audits/audit-1/findings/finding-1', 'GET'],
    ]);
  });

  it('creates audits and findings at their exact collection routes', async () => {
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-audit' });
      return jsonResponse({ id: 'created-record' }, 201);
    });

    await api.audits.create({
      title: 'Certification audit',
      description: 'Annual certification review',
      audit_type: 'certification',
      lead_auditor_id: '79e79c27-f11e-471c-b7f6-e4327165fa70',
      scope: 'Production platform',
      scheduled_start_date: '2026-10-01',
      scheduled_end_date: '2026-10-05',
    });
    await api.audits.createFinding('audit-1', {
      title: 'Privileged access review gap',
      description: 'Quarterly review evidence is incomplete.',
      severity: 'high',
      finding_type: 'non_conformity',
      recommendation: 'Complete and evidence the access review.',
      responsible_user_id: '79e79c27-f11e-471c-b7f6-e4327165fa70',
      due_date: '2026-10-31',
    });

    const mutationCalls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(mutationCalls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/audits/', 'POST'],
      ['/api/bff/audits/audit-1/findings', 'POST'],
    ]);
    expect(JSON.parse(String(mutationCalls[1][1]?.body))).toMatchObject({
      severity: 'high',
      due_date: '2026-10-31',
    });
  });

  it('normalizes finding pages and keeps stats on the dedicated route', async () => {
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse({
          data: [],
          pagination: { page: 3, page_size: 10, total_items: 22, total_pages: 3 },
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ total: 22, open: 3, in_progress: 2 }));

    const page = await api.audits.getFindings('audit-1', { page: 3, page_size: 10 });
    await api.audits.findingsStats('audit-1');

    expect(page).toMatchObject({ items: [], page: 3, page_size: 10, total: 22, total_pages: 3 });
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/bff/audits/audit-1/findings?page=3&page_size=10',
      '/api/bff/audits/audit-1/findings/stats',
    ]);
  });

  it('uses the nested finding identity for update and delete operations', async () => {
    fetchMock.mockImplementation(async (input, request) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-audit' });
      return request?.method === 'DELETE'
        ? new Response(null, { status: 204 })
        : jsonResponse({ id: 'finding-1', status: 'resolved' });
    });

    await api.audits.updateFinding('audit-1', 'finding-1', { status: 'resolved' });
    await api.audits.deleteFinding('audit-1', 'finding-1');

    const mutationCalls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(mutationCalls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/audits/audit-1/findings/finding-1', 'PATCH'],
      ['/api/bff/audits/audit-1/findings/finding-1', 'DELETE'],
    ]);
  });

  it('deletes an audit through the detail route without a request body', async () => {
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-audit' });
      return new Response(null, { status: 204 });
    });

    await api.audits.delete('audit-1');

    const [, request] = fetchMock.mock.calls[1];
    expect(fetchMock.mock.calls[1][0]).toBe('/api/bff/audits/audit-1');
    expect(request?.method).toBe('DELETE');
    expect(request?.body).toBeUndefined();
  });
});
