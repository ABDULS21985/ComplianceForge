import { ACCESS_TOKEN_COOKIE, CSRF_TOKEN_COOKIE, CSRF_TOKEN_HEADER, REFRESH_TOKEN_COOKIE } from '@/lib/auth-constants';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SUPPORT_BUNDLE_ROUTE, SUPPORT_BUNDLE_SCOPE } from '@/lib/support-bundle-contract';
import { NextRequest } from 'next/server';
import { proxyAuthenticatedRequest } from './session-bff';
import { proxyResponse } from './upstream';
import { supportBundleFixture } from '@/test/support-bundle-fixture';

const origin = 'https://app.example.test';
function request(path = SUPPORT_BUNDLE_ROUTE, method = 'POST', body?: string) {
  return new NextRequest(`${origin}/api/bff${path}`, {
    method, body: method === 'POST' ? body ?? JSON.stringify({ consent: true, scope: SUPPORT_BUNDLE_SCOPE }) : undefined,
    headers: {
      cookie: `${ACCESS_TOKEN_COOKIE}=server-only-access; ${REFRESH_TOKEN_COOKIE}=support-test-refresh-${path}; ${CSRF_TOKEN_COOKIE}=csrf-proof`,
      host: 'app.example.test', origin, 'sec-fetch-site': 'same-origin',
      'content-type': 'application/json', [CSRF_TOKEN_HEADER]: 'csrf-proof',
    },
  });
}
function pair() {
  return { access_token: 'rotated-access', refresh_token: 'rotated-refresh',
    expires_at: new Date(Date.now() + 60_000).toISOString(), user: { id: 'current-user' } };
}

describe('support bundle BFF boundary', () => {
  beforeEach(() => { process.env.API_INTERNAL_URL = 'http://api.internal.test/api/v1'; });

  it('adds only the exact safe digest response header; cookies/raw headers remain excluded', () => {
    const fixture = supportBundleFixture();
    const response = proxyResponse(new Response(fixture.bytes.buffer, {
      headers: { ...fixture.headers, 'set-cookie': 'secret=session', 'x-internal-error': 'secret-diagnostics' },
    }));
    expect(response.headers.get('x-support-bundle-sha256')).toBe(fixture.headers['x-support-bundle-sha256']);
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(response.headers.get('x-content-type-options')).toBe('nosniff');
    expect(response.headers.has('set-cookie')).toBe(false);
    expect(response.headers.has('x-internal-error')).toBe(false);
  });

  it('refreshes cookies after 401 but never replays the generation POST', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(Response.json({}, { status: 401 })).mockResolvedValueOnce(Response.json(pair()));
    const incoming = request();
    const response = await proxyAuthenticatedRequest(incoming, ['settings', 'diagnostics', 'support-bundle'], fetcher);
    expect(response.status).toBe(409);
    await expect(response.json()).resolves.toMatchObject({ code: 'CONSENT_RECONFIRMATION_REQUIRED' });
    expect(fetcher.mock.calls.filter(([url]) => String(url).includes('/diagnostics/support-bundle'))).toHaveLength(1);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(fetcher.mock.calls[0][1]).toMatchObject({ method: 'POST', signal: incoming.signal });
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get('authorization')).toBe('Bearer server-only-access');
    expect(response.headers.get('set-cookie')).toContain('HttpOnly');
    expect(response.headers.has('x-support-bundle-sha256')).toBe(false);
  });

  it('retains existing authenticated GET retry after refresh on other endpoints', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(Response.json({}, { status: 401 }))
      .mockResolvedValueOnce(Response.json(pair())).mockResolvedValueOnce(Response.json({ data: {} }));
    const response = await proxyAuthenticatedRequest(request('/settings/diagnostics', 'GET'), ['settings', 'diagnostics'], fetcher);
    expect(response.status).toBe(200);
    expect(fetcher.mock.calls.filter(([url]) => String(url).endsWith('/settings/diagnostics'))).toHaveLength(2);
  });

  it('rejects oversized support requests before upstream generation', async () => {
    const fetcher = vi.fn();
    const response = await proxyAuthenticatedRequest(request(SUPPORT_BUNDLE_ROUTE, 'POST', 'x'.repeat(1025)), ['settings', 'diagnostics', 'support-bundle'], fetcher);
    expect(response.status).toBe(413);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it.each([301, 302, 303, 307, 308])('rejects support generation redirects (%s) without forwarding a replay location', async (status) => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status, headers: { location: 'https://support.example.test/receive' } }));
    const response = await proxyAuthenticatedRequest(request(), ['settings', 'diagnostics', 'support-bundle'], fetcher);
    expect(response.status).toBe(502);
    expect(response.headers.has('location')).toBe(false);
    expect(fetcher).toHaveBeenCalledOnce();
  });

  it('preserves the existing safe evidence GET download redirect', async () => {
    const location = 'https://downloads.example.test/signed-evidence';
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 307, headers: { location } }));
    const response = await proxyAuthenticatedRequest(request('/controls/control-id/evidence/evidence-id/download', 'GET'),
      ['controls', 'control-id', 'evidence', 'evidence-id', 'download'], fetcher);
    expect(response.status).toBe(307);
    expect(response.headers.get('location')).toBe(location);
    expect(fetcher).toHaveBeenCalledOnce();
  });
});
