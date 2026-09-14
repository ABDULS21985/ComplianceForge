import { randomBytes, timingSafeEqual } from 'node:crypto';

import type { NextRequest } from 'next/server';
import { NextResponse } from 'next/server';

import { CSRF_ERROR_HEADER, CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { publicRequestOrigin, sessionCookiePolicy } from '@/lib/request-security';

const ACCESS_TOKEN_FALLBACK_MAX_AGE_SECONDS = 15 * 60;
const ACCESS_TOKEN_MAX_AGE_SECONDS = 7 * 24 * 60 * 60;
// The Go API currently issues refresh credentials for seven days. Keeping the
// browser cookie at the same upper bound avoids retaining an unusable token.
const REFRESH_TOKEN_MAX_AGE_SECONDS = 7 * 24 * 60 * 60;
const CSRF_TOKEN_MAX_AGE_SECONDS = 60 * 60;

export interface SessionTokenPair {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  user: unknown;
}

const sessionCookieOptions = {
  httpOnly: true,
  path: '/',
  sameSite: 'lax' as const,
};

const csrfCookieOptions = {
  httpOnly: true,
  path: '/',
  sameSite: 'strict' as const,
};

export function createCsrfToken(): string {
  return randomBytes(32).toString('base64url');
}

export function setCsrfCookie(
  request: NextRequest,
  response: NextResponse,
  token: string,
): void {
  const policy = sessionCookiePolicy(request);
  response.cookies.set(policy.csrf, token, {
    ...csrfCookieOptions,
    maxAge: CSRF_TOKEN_MAX_AGE_SECONDS,
    secure: policy.secure,
  });
}

function accessTokenMaxAge(expiresAt: string): number {
  const expiration = Date.parse(expiresAt);
  if (!Number.isFinite(expiration)) {
    return ACCESS_TOKEN_FALLBACK_MAX_AGE_SECONDS;
  }

  const remaining = Math.floor((expiration - Date.now()) / 1000);
  return Math.max(1, Math.min(remaining, ACCESS_TOKEN_MAX_AGE_SECONDS));
}

export function setSessionCookies(
  request: NextRequest,
  response: NextResponse,
  pair: SessionTokenPair,
): void {
  const policy = sessionCookiePolicy(request);
  response.cookies.set(policy.access, pair.access_token, {
    ...sessionCookieOptions,
    maxAge: accessTokenMaxAge(pair.expires_at),
    secure: policy.secure,
  });
  response.cookies.set(policy.refresh, pair.refresh_token, {
    ...sessionCookieOptions,
    maxAge: REFRESH_TOKEN_MAX_AGE_SECONDS,
    secure: policy.secure,
  });
}

export function clearSessionCookies(request: NextRequest, response: NextResponse): void {
  const policy = sessionCookiePolicy(request);
  for (const [name, options] of [
    [policy.access, sessionCookieOptions],
    [policy.refresh, sessionCookieOptions],
    [policy.csrf, csrfCookieOptions],
  ] as const) {
    response.cookies.set(name, '', {
      ...options,
      expires: new Date(0),
      maxAge: 0,
      secure: policy.secure,
    });
  }
}

function securelyEqual(left: string, right: string): boolean {
  const leftBytes = Buffer.from(left);
  const rightBytes = Buffer.from(right);
  return leftBytes.length === rightBytes.length && timingSafeEqual(leftBytes, rightBytes);
}

export type CsrfVerification = { ok: true } | { ok: false; message: string };

/**
 * Unsafe browser requests must prove both exact same-origin provenance and
 * possession of the synchronizer token returned by /api/auth/csrf.
 */
export function verifyCsrf(request: NextRequest): CsrfVerification {
  const fetchSite = request.headers.get('sec-fetch-site');
  if (fetchSite && fetchSite !== 'same-origin') {
    return { ok: false, message: 'Cross-origin request rejected' };
  }

  const origin = request.headers.get('origin');
  if (!origin) {
    return { ok: false, message: 'Origin header required' };
  }

  try {
    if (new URL(origin).origin !== publicRequestOrigin(request)) {
      return { ok: false, message: 'Request origin does not match' };
    }
  } catch {
    return { ok: false, message: 'Invalid request origin' };
  }

  const cookieToken = request.cookies.get(sessionCookiePolicy(request).csrf)?.value;
  const headerToken = request.headers.get(CSRF_TOKEN_HEADER);
  if (!cookieToken || !headerToken || !securelyEqual(cookieToken, headerToken)) {
    return { ok: false, message: 'CSRF token is missing or invalid' };
  }

  return { ok: true };
}

export function csrfErrorResponse(message: string): NextResponse {
  return NextResponse.json(
    { code: 'CSRF_INVALID', message },
    {
      status: 403,
      headers: {
        'Cache-Control': 'no-store',
        [CSRF_ERROR_HEADER]: '1',
        'X-Content-Type-Options': 'nosniff',
      },
    },
  );
}

export function jsonError(status: number, code: string, message: string): NextResponse {
  return NextResponse.json(
    { code, message },
    {
      status,
      headers: {
        'Cache-Control': 'no-store',
        'X-Content-Type-Options': 'nosniff',
      },
    },
  );
}
