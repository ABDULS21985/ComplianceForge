import {
  ACCESS_TOKEN_COOKIE,
  CSRF_TOKEN_COOKIE,
  CSRF_TOKEN_HEADER,
  REFRESH_TOKEN_COOKIE,
} from '@/lib/auth-constants';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { handleCredentialExchange } from '@/lib/server/session-bff';
import { handlePublicIdentityRoute } from '@/lib/server/identity-bff';
import { NextRequest } from 'next/server';

const origin = 'https://app.example.test';

function request(path: string, csrf = 'csrf-proof', body = '{}'): NextRequest {
  return new NextRequest(`${origin}${path}`, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      cookie: `${CSRF_TOKEN_COOKIE}=${csrf}`,
      host: 'app.example.test',
      origin,
      'sec-fetch-site': 'same-origin',
      [CSRF_TOKEN_HEADER]: csrf,
    },
    body,
  });
}

function tokenPair() {
  return {
    access_token: 'identity-access-secret',
    refresh_token: 'identity-refresh-secret',
    expires_at: new Date(Date.now() + 60_000).toISOString(),
    user: { id: 'user-1', email: 'security@example.test' },
  };
}

describe('public identity BFF', () => {
  beforeEach(() => {
    process.env.API_INTERNAL_URL = 'http://identity.internal.test/api/v1';
  });

  it('rejects missing CSRF proof before any public identity call', async () => {
    const fetcher = vi.fn();
    const response = await handlePublicIdentityRoute(
      new NextRequest(`${origin}/api/auth/password/forgot`, {
        method: 'POST',
        headers: { 'content-type': 'application/json', host: 'app.example.test' },
        body: '{}',
      }),
      ['password', 'forgot'],
      fetcher,
    );

    expect(response.status).toBe(403);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('proxies only an allowlisted route without bearer credentials or upstream cookies', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      Response.json(
        { message: 'If the account is eligible, delivery has been queued.' },
        { status: 202, headers: { 'Set-Cookie': 'upstream=secret' } },
      ),
    );
    const response = await handlePublicIdentityRoute(
      request(
        '/api/auth/password/forgot',
        'csrf-proof',
        JSON.stringify({ organization_id: 'org-1', email: 'person@example.test' }),
      ),
      ['password', 'forgot'],
      fetcher,
    );

    expect(response.status).toBe(202);
    expect(String(fetcher.mock.calls[0][0])).toBe(
      'http://identity.internal.test/api/v1/auth/password/forgot',
    );
    const headers = new Headers(fetcher.mock.calls[0][1]?.headers);
    expect(headers.has('authorization')).toBe(false);
    expect(response.headers.has('set-cookie')).toBe(false);
    expect(response.headers.get('cache-control')).toBe('no-store');
  });

  it('fails closed for a public auth path that is not explicitly mounted', async () => {
    const fetcher = vi.fn();
    const response = await handlePublicIdentityRoute(
      request('/api/auth/admin/export'),
      ['admin', 'export'],
      fetcher,
    );

    expect(response.status).toBe(404);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('turns successful MFA proof into HttpOnly cookies without exposing tokens', async () => {
    const fetcher = vi.fn().mockResolvedValue(Response.json(tokenPair()));
    const response = await handlePublicIdentityRoute(
      request('/api/auth/mfa/verify'),
      ['mfa', 'verify'],
      fetcher,
    );
    const payload = await response.json();

    expect(payload).toEqual({
      expires_at: expect.any(String),
      user: { id: 'user-1', email: 'security@example.test' },
    });
    expect(JSON.stringify(payload)).not.toContain('identity-access-secret');
    expect(JSON.stringify(payload)).not.toContain('identity-refresh-secret');
    const cookies = response.headers.get('set-cookie') ?? '';
    expect(cookies).toContain(`${ACCESS_TOKEN_COOKIE}=identity-access-secret`);
    expect(cookies).toContain(`${REFRESH_TOKEN_COOKIE}=identity-refresh-secret`);
    expect(cookies).toContain('HttpOnly');
    expect(cookies).toContain('Secure');
  });

  it('returns a pending challenge but never creates cookies or reflects upstream tokens', async () => {
    const challenge = {
      challenge_token: 'c'.repeat(40),
      methods: ['totp'],
      expires_at: new Date(Date.now() + 60_000).toISOString(),
    };
    const fetcher = vi.fn().mockResolvedValue(
      Response.json({
        access_token: 'must-not-leak',
        mfa_required: true,
        mfa_challenge: challenge,
        user: { id: 'user-1' },
      }),
    );
    const response = await handleCredentialExchange(
      request('/api/auth/login'),
      'login',
      fetcher,
    );
    const payload = await response.json();

    expect(payload).toEqual({ mfa_challenge: challenge, mfa_required: true, user: { id: 'user-1' } });
    expect(JSON.stringify(payload)).not.toContain('must-not-leak');
    expect(response.headers.has('set-cookie')).toBe(false);
  });
});
