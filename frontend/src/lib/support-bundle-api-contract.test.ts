import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import { type AttachmentResponse, AttachmentValidationError } from './attachment';
import { CSRF_ERROR_HEADER, CSRF_TOKEN_HEADER } from './auth-constants';
import { fetchWithCsrf, resetCsrfToken } from './csrf-client';
import { SUPPORT_BUNDLE_ROUTE, SUPPORT_BUNDLE_SCOPE } from './support-bundle-contract';
import api from './api';
import { supportBundleFixture } from '@/test/support-bundle-fixture';

const consent = { consent: true, scope: SUPPORT_BUNDLE_SCOPE } as const;
function csrf() { return Response.json({ csrf_token: 'request-protection' }); }
function install(post: () => Promise<Response>) {
  const mock = vi.fn().mockImplementation((url: string) => url === '/api/auth/csrf' ? Promise.resolve(csrf()) : post());
  vi.stubGlobal('fetch', mock);
  return mock;
}

describe('consent-bound support attachment request', () => {
  beforeEach(resetCsrfToken);
  afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); resetCsrfToken(); });

  it('uses the exact body, same-origin BFF, CSRF and selected bounded attachment metadata', async () => {
    const fixture = supportBundleFixture();
    const mock = install(async () => new Response(fixture.bytes.buffer, { headers: fixture.headers }));
    const abort = new AbortController();
    expectTypeOf(api.diagnostics.generateSupportBundle).returns.toEqualTypeOf<Promise<AttachmentResponse>>();
    const response = await api.diagnostics.generateSupportBundle(consent, abort.signal);
    const [url, init] = mock.mock.calls.find(([url]) => url !== '/api/auth/csrf')!;
    expect(url).toBe(`/api/bff${SUPPORT_BUNDLE_ROUTE}`);
    expect(init).toMatchObject({ method: 'POST', credentials: 'same-origin', signal: abort.signal, redirect: 'error' });
    expect(JSON.parse(init.body)).toEqual(consent);
    expect(new Headers(init.headers).get(CSRF_TOKEN_HEADER)).toBe('request-protection');
    expect(new Headers(init.headers).has('authorization')).toBe(false);
    expect(response.supportBundleSha256).toBe(fixture.headers['x-support-bundle-sha256']);
  });

  it.each(['ambiguous transport', '503', 'CSRF rejection'])('never automatically replays POST after %s', async (kind) => {
    const mock = install(async () => {
      if (kind === 'ambiguous transport') throw new Error('Network disconnected');
      return Response.json({ message: 'Unavailable' }, {
        status: kind === '503' ? 503 : 403,
        headers: kind === 'CSRF rejection' ? { [CSRF_ERROR_HEADER]: '1' } : {},
      });
    });
    await expect(api.diagnostics.generateSupportBundle(consent)).rejects.toBeDefined();
    expect(mock.mock.calls.filter(([url]) => url !== '/api/auth/csrf')).toHaveLength(1);
    expect(mock.mock.calls.filter(([url]) => url === '/api/auth/csrf')).toHaveLength(1);
  });

  it('serializes exactly two fields, never caller-supplied tenant/user metadata', async () => {
    const fixture = supportBundleFixture();
    const mock = install(async () => new Response(fixture.bytes.buffer, { headers: fixture.headers }));
    const input = { ...consent, tenant_override: 'other-tenant', user_id: 'excluded' };
    await api.diagnostics.generateSupportBundle(input);
    expect(JSON.parse(mock.mock.calls[1][1].body)).toEqual(consent);
  });

  it('preserves default CSRF recovery for other existing mutation endpoints', async () => {
    let attempts = 0;
    const mock = install(async () => ++attempts === 1 ?
      Response.json({}, { status: 403, headers: { [CSRF_ERROR_HEADER]: '1' } }) : Response.json({ saved: true }));
    const response = await fetchWithCsrf('/api/bff/settings/other', { method: 'PUT' });
    expect(response.status).toBe(200);
    expect(mock.mock.calls.filter(([url]) => url !== '/api/auth/csrf')).toHaveLength(2);
  });

  it.each(['application/json', 'text/plain', 'application/octet-stream'])('bounds non-OK %s before any decode and discards driver detail', async (contentType) => {
    install(async () => new Response('x'.repeat(65537), { status: 503, headers: { 'content-type': contentType } }));
    await expect(api.diagnostics.generateSupportBundle(consent)).rejects.toBeInstanceOf(AttachmentValidationError);
  });

  it('retains only safe failure codes/request identity, not server messages or detail', async () => {
    install(async () => Response.json({
      error_code: 'CONSENT_RECONFIRMATION_REQUIRED', message: 'password=secret; host=private.internal',
      detail: { database_password: 'secret' }, request_id: '19bb8bdc-1073-48fe-978c-ce11f3f4e2dc',
    }, { status: 409 }));
    await expect(api.diagnostics.generateSupportBundle(consent)).rejects.toMatchObject({
      status: 409, message: 'The support bundle request failed',
      detail: { error_code: 'CONSENT_RECONFIRMATION_REQUIRED', request_id: '19bb8bdc-1073-48fe-978c-ce11f3f4e2dc' },
    });
    try { await api.diagnostics.generateSupportBundle(consent); } catch (error) { expect(JSON.stringify(error)).not.toMatch(/password|private.internal/); }
  });

  it('retains idempotent GET retry behavior without retrying consent generation', async () => {
    vi.useFakeTimers();
    let attempts = 0;
    const mock = install(async () => ++attempts === 1 ? Response.json({}, { status: 503 }) : Response.json({ data: {} }));
    const request = api.diagnostics.snapshot();
    await vi.advanceTimersByTimeAsync(500);
    await expect(request).resolves.toEqual({ data: {} });
    expect(mock).toHaveBeenCalledTimes(2);
  });

  it('does not issue a POST after cancellation during CSRF initialization', async () => {
    let resolve: (response: Response) => void = () => undefined;
    const mock = vi.fn().mockImplementation(() => new Promise<Response>((done) => { resolve = done; }));
    vi.stubGlobal('fetch', mock);
    const abort = new AbortController();
    const pending = api.diagnostics.generateSupportBundle(consent, abort.signal);
    abort.abort(); resolve(csrf());
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(mock).toHaveBeenCalledOnce();
    expect(mock.mock.calls[0][0]).toBe('/api/auth/csrf');
  });
});
