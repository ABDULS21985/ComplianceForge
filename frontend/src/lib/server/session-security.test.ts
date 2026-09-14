import { NextRequest, NextResponse } from 'next/server';
import { describe, expect, it } from 'vitest';

import {
  ACCESS_TOKEN_COOKIE,
  CSRF_ERROR_HEADER,
  CSRF_TOKEN_COOKIE,
  CSRF_TOKEN_HEADER,
  REFRESH_TOKEN_COOKIE,
} from '@/lib/auth-constants';
import {
  clearSessionCookies,
  csrfErrorResponse,
  setCsrfCookie,
  setSessionCookies,
  verifyCsrf,
} from '@/lib/server/session-security';

function csrfRequest(overrides: Record<string, string> = {}) {
  return new NextRequest('https://app.example.test/api/bff/risks', {
    method: 'POST',
    headers: {
      cookie: `${CSRF_TOKEN_COOKIE}=known-token`,
      host: 'app.example.test',
      origin: 'https://app.example.test',
      'sec-fetch-site': 'same-origin',
      [CSRF_TOKEN_HEADER]: 'known-token',
      ...overrides,
    },
  });
}

describe('session cookie security', () => {
  it('sets bearer credentials only as host-scoped secure HttpOnly cookies', () => {
    const response = NextResponse.json({ ok: true });
    setSessionCookies(response, {
      access_token: 'access-secret',
      refresh_token: 'refresh-secret',
      expires_at: new Date(Date.now() + 60_000).toISOString(),
      user: { id: 'user-1' },
    });

    const cookieHeader = response.headers.get('set-cookie') ?? '';
    expect(cookieHeader).toContain(`${ACCESS_TOKEN_COOKIE}=access-secret`);
    expect(cookieHeader).toContain(`${REFRESH_TOKEN_COOKIE}=refresh-secret`);
    expect(cookieHeader).toMatch(/HttpOnly/i);
    expect(cookieHeader).toMatch(/Secure/i);
    expect(cookieHeader).toMatch(/SameSite=lax/i);
    expect(cookieHeader).toMatch(/Path=\//i);
    expect(cookieHeader).not.toMatch(/Domain=/i);
  });

  it('sets the CSRF synchronizer cookie as strict, secure, and HttpOnly', () => {
    const response = NextResponse.json({ ok: true });
    setCsrfCookie(response, 'csrf-secret');

    const cookieHeader = response.headers.get('set-cookie') ?? '';
    expect(cookieHeader).toContain(`${CSRF_TOKEN_COOKIE}=csrf-secret`);
    expect(cookieHeader).toMatch(/HttpOnly/i);
    expect(cookieHeader).toMatch(/Secure/i);
    expect(cookieHeader).toMatch(/SameSite=strict/i);
  });

  it('expires access, refresh, and CSRF cookies together', () => {
    const response = new NextResponse(null, { status: 204 });
    clearSessionCookies(response);

    expect(response.cookies.getAll().map(({ name }) => name)).toEqual([
      ACCESS_TOKEN_COOKIE,
      REFRESH_TOKEN_COOKIE,
      CSRF_TOKEN_COOKIE,
    ]);
    expect(response.headers.get('set-cookie')).toMatch(/Max-Age=0/i);
  });
});

describe('CSRF validation', () => {
  it('accepts an exact same-origin request with a matching token', () => {
    expect(verifyCsrf(csrfRequest())).toEqual({ ok: true });
  });

  it.each([
    ['missing origin', { origin: '' }],
    ['cross origin', { origin: 'https://attacker.example' }],
    ['cross-site fetch metadata', { 'sec-fetch-site': 'cross-site' }],
    ['mismatched token', { [CSRF_TOKEN_HEADER]: 'other-token' }],
  ])('rejects %s', (_label, headers) => {
    expect(verifyCsrf(csrfRequest(headers))).toMatchObject({ ok: false });
  });

  it('marks CSRF errors so the client can bootstrap a fresh token once', () => {
    const response = csrfErrorResponse('invalid');
    expect(response.status).toBe(403);
    expect(response.headers.get(CSRF_ERROR_HEADER)).toBe('1');
  });
});
