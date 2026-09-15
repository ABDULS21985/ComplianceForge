import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import { CSRF_TOKEN_HEADER, SESSION_EXPIRED_EVENT } from '@/lib/auth-constants';
import type {
  IdentityMFAFactor,
  IdentityPasskey,
  IdentityPolicy,
  IdentitySecurityEventList,
  IdentitySession,
} from '@/types/identity';
import api from '@/lib/api';
import type { DataEnvelope } from '@/types/enterprise-settings';
import { resetCsrfToken } from '@/lib/csrf-client';
import { STEP_UP_TOKEN_HEADER } from '@/lib/identity';

function jsonResponse(body: unknown, status = 200): Response {
  return Response.json(body, { status });
}

describe('identity lifecycle API contracts', () => {
  const fetchMock = vi.fn<
    (input: RequestInfo | URL, request?: RequestInit) => Promise<Response>
  >();

  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-proof' });
      return jsonResponse({ data: [] });
    });
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps protected response envelopes exactly typed', () => {
    expectTypeOf(api.identity.getPolicy).returns.resolves.toEqualTypeOf<IdentityPolicy>();
    expectTypeOf(api.identity.listSessions)
      .returns.resolves.toEqualTypeOf<DataEnvelope<IdentitySession[]>>();
    expectTypeOf(api.identity.listFactors)
      .returns.resolves.toEqualTypeOf<DataEnvelope<IdentityMFAFactor[]>>();
    expectTypeOf(api.identity.listPasskeys)
      .returns.resolves.toEqualTypeOf<DataEnvelope<IdentityPasskey[]>>();
    expectTypeOf(api.identity.listHistory)
      .returns.resolves.toEqualTypeOf<IdentitySecurityEventList>();
  });

  it('uses the canonical session and paginated history routes through the BFF', async () => {
    await api.identity.listSessions();
    await api.identity.listHistory({ user_id: 'user-1', page: 2, page_size: 20 });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/bff/identity/sessions');
    expect(fetchMock.mock.calls[1][0]).toBe(
      '/api/bff/identity/history?user_id=user-1&page=2&page_size=20',
    );
    expect(new Headers(fetchMock.mock.calls[0][1]?.headers).has('authorization')).toBe(false);
  });

  it('sends optimistic versions and step-up proof only in protected request headers', async () => {
    await api.identity.disableFactor(
      'factor/unsafe',
      { expected_version: 3, reason: 'Replace compromised device' },
      'single-use-step-up',
    );

    const [url, request] = fetchMock.mock.calls[1];
    expect(url).toBe('/api/bff/identity/mfa/factors/factor%2Funsafe');
    expect(request?.method).toBe('DELETE');
    expect(JSON.parse(String(request?.body))).toEqual({
      expected_version: 3,
      reason: 'Replace compromised device',
    });
    const headers = new Headers(request?.headers);
    expect(headers.get(CSRF_TOKEN_HEADER)).toBe('csrf-proof');
    expect(headers.get(STEP_UP_TOKEN_HEADER)).toBe('single-use-step-up');
    expect(headers.has('authorization')).toBe(false);
    expect(String(url)).not.toContain('single-use-step-up');
  });

  it('keeps public password credentials in a CSRF-protected body on the same-origin route', async () => {
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-proof' });
      return new Response(null, { status: 204 });
    });
    await api.auth.resetPassword({ token: 'r'.repeat(40), new_password: 'new password value' });

    const [url, request] = fetchMock.mock.calls[1];
    expect(url).toBe('/api/auth/password/reset');
    expect(JSON.parse(String(request?.body))).toEqual({
      token: 'r'.repeat(40),
      new_password: 'new password value',
    });
    expect(new Headers(request?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-proof');
    expect(new Headers(request?.headers).has('authorization')).toBe(false);
  });

  it('does not treat a rejected public proof as a terminal browser session', async () => {
    const dispatchEvent = vi.fn();
    const assign = vi.fn();
    vi.stubGlobal('window', {
      dispatchEvent,
      location: {
        assign,
        origin: 'https://app.example.test',
        pathname: '/login',
        search: '',
      },
    });
    fetchMock.mockImplementation(async (input) => {
      if (String(input) === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-proof' });
      return jsonResponse({ message: 'Proof rejected' }, 401);
    });

    await expect(
      api.auth.verifyMFA({
        challenge_token: 'c'.repeat(40),
        method: 'totp',
        code: '123456',
      }),
    ).rejects.toMatchObject({ status: 401, message: 'Unauthorized' });
    expect(dispatchEvent).not.toHaveBeenCalledWith(expect.objectContaining({ type: SESSION_EXPIRED_EVENT }));
    expect(assign).not.toHaveBeenCalled();
  });
});
