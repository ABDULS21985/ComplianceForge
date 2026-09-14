import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';

import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { resetCsrfToken } from '@/lib/csrf-client';
import { normalizePermissionMap } from '@/lib/navigation';
import type { PermissionMap } from '@/types/access';
import type { DataEnvelope } from '@/types/enterprise-settings';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('enterprise settings API route contracts', () => {
  const fetchMock = vi.fn<
    (input: RequestInfo | URL, request?: RequestInit) => Promise<Response>
  >();

  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-test' });
      return jsonResponse({ data: [], pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 } });
    });
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('uses the canonical paginated notification collection route', async () => {
    await api.notifications.list({ page: 2, page_size: 20 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/bff/notifications/?page=2&page_size=20');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'GET', credentials: 'same-origin' });
  });

  it('sends notification-channel credentials only in a CSRF-protected request body', async () => {
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-test' });
      return jsonResponse({ id: 'channel-1', message: 'Notification channel created' }, 201);
    });
    await api.notificationAdmin.createChannel({
      name: 'Signed webhook',
      channel_type: 'webhook',
      config: { url: 'https://hooks.example.test', secret: 'x'.repeat(32) },
      is_active: true,
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [url, request] = fetchMock.mock.calls[1];
    expect(url).toBe('/api/bff/settings/notification-channels/');
    expect(request?.method).toBe('POST');
    expect(new Headers(request?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-test');
    expect(JSON.parse(String(request?.body))).toEqual({
      name: 'Signed webhook',
      channel_type: 'webhook',
      config: { url: 'https://hooks.example.test', secret: 'x'.repeat(32) },
      is_active: true,
    });
  });

  it('uses the backend API-key field names without browser bearer credentials', async () => {
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-test' });
      return jsonResponse({ data: {}, key: 'cf_live_once', note: 'Store securely' }, 201);
    });
    await api.integrations.createAPIKey({
      name: 'Evidence reader',
      permissions: ['read:controls'],
      rate_limit: 60,
      expires_at: '2027-01-01T00:00:00.000Z',
    });

    const [url, request] = fetchMock.mock.calls[1];
    expect(url).toBe('/api/bff/settings/api-keys');
    const headers = new Headers(request?.headers);
    expect(headers.has('authorization')).toBe(false);
    expect(JSON.parse(String(request?.body))).toMatchObject({
      permissions: ['read:controls'],
      rate_limit: 60,
    });
  });

  it('keeps the permissions endpoint on its canonical typed envelope', async () => {
    expectTypeOf(api.access.myPermissions)
      .returns.resolves.toEqualTypeOf<DataEnvelope<PermissionMap>>();
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ data: { audits: ['create', 'read'], reports: ['read'] } }),
    );

    const response = await api.access.myPermissions();

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/bff/access/my-permissions',
      expect.objectContaining({ method: 'GET', credentials: 'same-origin' }),
    );
    expect(normalizePermissionMap(response)).toEqual({
      audits: ['create', 'read'],
      reports: ['read'],
    });
  });
});
