import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { resetCsrfToken } from '@/lib/csrf-client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('incident API route contracts', () => {
  const fetchMock = vi.fn<(input: RequestInfo | URL, request?: RequestInit) => Promise<Response>>();
  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input, request) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-incident' });
      if (request?.method === 'DELETE') return new Response(null, { status: 204 });
      return jsonResponse({ id: 'incident-1', version: 2, data: [], pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 } });
    });
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('uses the canonical slash collection, list filters, and normalized pagination', async () => {
    const page = await api.incidents.list({ page: 2, page_size: 20, status: 'reported', severity: 'high', breach_notifiable: true, sort: 'notification_deadline', direction: 'asc' });
    expect(page).toEqual({ items: [], page: 1, page_size: 20, total: 0, total_pages: 0 });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/bff/incidents/?page=2&page_size=20&status=reported&severity=high&breach_notifiable=true&sort=notification_deadline&direction=asc');
  });

  it('uses versioned PATCH, DELETE, transition, close, and escalation contracts', async () => {
    await api.incidents.update('incident-1', { version: 7, title: 'Updated' });
    await api.incidents.delete('incident-1', 8);
    await api.incidents.transition('incident-1', { status: 'triaged', reason: 'Triage complete', version: 8 });
    await api.incidents.close('incident-1', { reason: 'Response complete', version: 9 });
    await api.incidents.escalate('incident-1', { severity: 'critical', reason: 'Material impact', version: 10 });
    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/incidents/incident-1', 'PATCH'],
      ['/api/bff/incidents/incident-1?version=8', 'DELETE'],
      ['/api/bff/incidents/incident-1/transitions', 'POST'],
      ['/api/bff/incidents/incident-1/close', 'POST'],
      ['/api/bff/incidents/incident-1/escalate', 'PUT'],
    ]);
    for (const [, request] of calls) {
      expect(new Headers(request?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-incident');
      expect(new Headers(request?.headers).has('authorization')).toBe(false);
    }
    expect(JSON.parse(String(calls[2][1]?.body))).toMatchObject({ status: 'triaged', version: 8 });
  });

  it('uses exact GDPR, timeline, assignment, and statistics routes', async () => {
    await api.incidents.stats();
    await api.incidents.upcomingBreaches({ horizon_hours: 72, limit: 10 });
    await api.incidents.timeline('incident-1', { page: 1, page_size: 20 });
    await api.incidents.assignments('incident-1', true);
    await api.incidents.assessBreach('incident-1', { version: 2, status: 'notifiable', reason: 'Likely rights risk', is_data_breach: true, awareness_at: '2026-09-14T00:00:00Z', special_category_data: false, cross_border: false });
    await api.incidents.notifyDPA('incident-1', { version: 3, idempotency_key: '82960a6a-6d6c-4b4e-975f-05c6bbcd582c', notified_at: '2026-09-14T01:00:00Z', reference: 'DPA-100', reason: 'Article 33 submission' });
    await api.incidents.assign('incident-1', { version: 4, assignee_id: '79e79c27-f11e-471c-b7f6-e4327165fa70', role: 'primary', reason: 'Response lead' });
    await api.incidents.unassign('incident-1', 'assignment-1', { version: 5, reason: 'Rotation ended' });
    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [url, request?.method])).toEqual([
      ['/api/bff/incidents/statistics', 'GET'],
      ['/api/bff/incidents/breaches/upcoming?horizon_hours=72&limit=10', 'GET'],
      ['/api/bff/incidents/incident-1/timeline?page=1&page_size=20', 'GET'],
      ['/api/bff/incidents/incident-1/assignments?active_only=true', 'GET'],
      ['/api/bff/incidents/incident-1/breach-assessment', 'POST'],
      ['/api/bff/incidents/incident-1/notify-dpa', 'POST'],
      ['/api/bff/incidents/incident-1/assignments', 'POST'],
      ['/api/bff/incidents/incident-1/assignments/assignment-1/unassign', 'POST'],
    ]);
  });
});
