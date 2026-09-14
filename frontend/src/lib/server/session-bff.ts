import { createHash } from 'node:crypto';

import type { NextRequest } from 'next/server';
import { NextResponse } from 'next/server';

import { ACCESS_TOKEN_COOKIE, REFRESH_TOKEN_COOKIE } from '@/lib/auth-constants';
import {
  clearSessionCookies,
  csrfErrorResponse,
  jsonError,
  type SessionTokenPair,
  setSessionCookies,
  verifyCsrf,
} from '@/lib/server/session-security';
import {
  buildUpstreamUrl,
  forwardedRequestHeaders,
  PayloadTooLargeError,
  proxyResponse,
  readLimitedRequestBody,
  serverFetchInit,
  type ServerFetch,
} from '@/lib/server/upstream';

const MAX_AUTH_BODY_BYTES = 1024 * 1024;
const MAX_PROXY_BODY_BYTES = 50 * 1024 * 1024;
const REFRESH_COMPLETION_GRACE_MS = 5_000;

type RefreshResult =
  | { kind: 'success'; pair: SessionTokenPair }
  | { kind: 'terminal' }
  | { kind: 'unavailable' };

const refreshes = new Map<string, Promise<RefreshResult>>();

function isTokenPair(value: unknown): value is SessionTokenPair {
  if (!value || typeof value !== 'object') return false;
  const pair = value as Partial<SessionTokenPair>;
  return (
    typeof pair.access_token === 'string' &&
    pair.access_token.length > 0 &&
    pair.access_token.length <= 8192 &&
    typeof pair.refresh_token === 'string' &&
    pair.refresh_token.length > 0 &&
    pair.refresh_token.length <= 8192 &&
    typeof pair.expires_at === 'string' &&
    Number.isFinite(Date.parse(pair.expires_at)) &&
    Date.parse(pair.expires_at) > Date.now() &&
    typeof pair.user === 'object' &&
    pair.user !== null &&
    !Array.isArray(pair.user)
  );
}

async function performRefresh(refreshToken: string, fetcher: ServerFetch): Promise<RefreshResult> {
  try {
    const upstream = await fetcher(
      buildUpstreamUrl('/auth/refresh'),
      serverFetchInit({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      }),
    );

    if ([400, 401, 403].includes(upstream.status)) {
      return { kind: 'terminal' };
    }
    if (!upstream.ok) return { kind: 'unavailable' };

    const data: unknown = await upstream.json().catch(() => null);
    if (!isTokenPair(data)) return { kind: 'unavailable' };
    return { kind: 'success', pair: data };
  } catch {
    return { kind: 'unavailable' };
  }
}

/**
 * Coalesce refresh attempts for the same token. This matters when several
 * queries discover an expired access token at the same time and the backend
 * rotates refresh credentials on use.
 */
export function refreshSession(
  refreshToken: string,
  fetcher: ServerFetch = fetch,
): Promise<RefreshResult> {
  const key = createHash('sha256').update(refreshToken).digest('base64url');
  const active = refreshes.get(key);
  if (active) return active;

  const pending = performRefresh(refreshToken, fetcher).then((result) => {
    if (result.kind !== 'success') {
      if (refreshes.get(key) === pending) refreshes.delete(key);
      return result;
    }

    // Keep a successful result briefly so requests that were concurrent in
    // the browser but arrived just after rotation reuse the same token pair
    // instead of replaying the now-consumed refresh credential.
    const timer = setTimeout(() => {
      if (refreshes.get(key) === pending) refreshes.delete(key);
    }, REFRESH_COMPLETION_GRACE_MS);
    timer.unref?.();
    return result;
  });
  refreshes.set(key, pending);
  return pending;
}

function terminalUnauthorized(): NextResponse {
  const response = jsonError(401, 'SESSION_EXPIRED', 'Session expired');
  clearSessionCookies(response);
  return response;
}

function refreshUnavailable(): NextResponse {
  return jsonError(
    503,
    'AUTH_SERVICE_UNAVAILABLE',
    'Authentication service is temporarily unavailable',
  );
}

function preserveRotatedSession(
  response: NextResponse,
  pair: SessionTokenPair | null,
): NextResponse {
  if (pair) setSessionCookies(response, pair);
  return response;
}

function validateProxyPath(path: string[]): string | null {
  if (
    path.length === 0 ||
    path.some(
      (segment) =>
        !segment ||
        segment === '.' ||
        segment === '..' ||
        segment.includes('/') ||
        segment.includes('\\') ||
        segment.length > 256,
    )
  ) {
    return null;
  }

  // Authentication exchanges must use the dedicated handlers so token
  // responses can never pass through to browser JavaScript.
  if (path[0] === 'auth' || path[0] === 'vendor-portal' || path[0] === 'board-portal') {
    return null;
  }

  return `/${path.map(encodeURIComponent).join('/')}`;
}

async function authenticatedFetch(
  request: NextRequest,
  pathname: string,
  body: ArrayBuffer | undefined,
  accessToken: string,
  fetcher: ServerFetch,
): Promise<Response> {
  const headers = forwardedRequestHeaders(request);
  headers.set('Authorization', `Bearer ${accessToken}`);

  return fetcher(
    buildUpstreamUrl(pathname, request.nextUrl.search),
    serverFetchInit({
      method: request.method,
      headers,
      body: body ? body.slice(0) : undefined,
    }),
  );
}

export async function proxyAuthenticatedRequest(
  request: NextRequest,
  path: string[],
  fetcher: ServerFetch = fetch,
): Promise<NextResponse> {
  const pathname = validateProxyPath(path);
  if (!pathname) {
    return jsonError(404, 'ROUTE_NOT_FOUND', 'API route not found');
  }

  if (!['GET', 'HEAD'].includes(request.method)) {
    const csrf = verifyCsrf(request);
    if (!csrf.ok) return csrfErrorResponse(csrf.message);
  }

  let body: ArrayBuffer | undefined;
  try {
    body = await readLimitedRequestBody(request, MAX_PROXY_BODY_BYTES);
  } catch (error) {
    if (error instanceof PayloadTooLargeError) {
      return jsonError(413, 'PAYLOAD_TOO_LARGE', error.message);
    }
    return jsonError(400, 'INVALID_REQUEST_BODY', 'Unable to read request body');
  }

  let accessToken = request.cookies.get(ACCESS_TOKEN_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_TOKEN_COOKIE)?.value;
  let refreshedPair: SessionTokenPair | null = null;

  if (!accessToken) {
    if (!refreshToken) return terminalUnauthorized();
    const refreshed = await refreshSession(refreshToken, fetcher);
    if (refreshed.kind === 'terminal') return terminalUnauthorized();
    if (refreshed.kind === 'unavailable') return refreshUnavailable();
    refreshedPair = refreshed.pair;
    accessToken = refreshed.pair.access_token;
  }

  let upstream: Response;
  try {
    upstream = await authenticatedFetch(request, pathname, body, accessToken, fetcher);
  } catch {
    return preserveRotatedSession(
      jsonError(502, 'UPSTREAM_UNAVAILABLE', 'API service is unavailable'),
      refreshedPair,
    );
  }

  if (upstream.status === 401 && refreshToken && !refreshedPair) {
    const refreshed = await refreshSession(refreshToken, fetcher);
    if (refreshed.kind === 'terminal') return terminalUnauthorized();
    if (refreshed.kind === 'unavailable') return refreshUnavailable();
    refreshedPair = refreshed.pair;

    try {
      upstream = await authenticatedFetch(
        request,
        pathname,
        body,
        refreshed.pair.access_token,
        fetcher,
      );
    } catch {
      return preserveRotatedSession(
        jsonError(502, 'UPSTREAM_UNAVAILABLE', 'API service is unavailable'),
        refreshedPair,
      );
    }
  }

  if (upstream.status === 401) return terminalUnauthorized();

  return preserveRotatedSession(proxyResponse(upstream), refreshedPair);
}

export async function handleCredentialExchange(
  request: NextRequest,
  operation: 'login' | 'register',
  fetcher: ServerFetch = fetch,
): Promise<NextResponse> {
  const csrf = verifyCsrf(request);
  if (!csrf.ok) return csrfErrorResponse(csrf.message);

  let body: ArrayBuffer | undefined;
  try {
    body = await readLimitedRequestBody(request, MAX_AUTH_BODY_BYTES);
  } catch (error) {
    if (error instanceof PayloadTooLargeError) {
      return jsonError(413, 'PAYLOAD_TOO_LARGE', error.message);
    }
    return jsonError(400, 'INVALID_REQUEST_BODY', 'Unable to read request body');
  }

  let upstream: Response;
  try {
    upstream = await fetcher(
      buildUpstreamUrl(`/auth/${operation}`),
      serverFetchInit({
        method: 'POST',
        headers: forwardedRequestHeaders(request),
        body,
      }),
    );
  } catch {
    return jsonError(502, 'UPSTREAM_UNAVAILABLE', 'Authentication service is unavailable');
  }

  if (!upstream.ok) return proxyResponse(upstream);

  const data: unknown = await upstream.json().catch(() => null);
  if (!isTokenPair(data)) {
    return jsonError(
      502,
      'INVALID_UPSTREAM_RESPONSE',
      'Authentication service returned an invalid response',
    );
  }

  const response = NextResponse.json(
    { expires_at: data.expires_at, user: data.user },
    {
      status: upstream.status,
      headers: {
        'Cache-Control': 'no-store',
        'X-Content-Type-Options': 'nosniff',
      },
    },
  );
  setSessionCookies(response, data);
  return response;
}

async function currentUser(accessToken: string, fetcher: ServerFetch): Promise<Response> {
  return fetcher(
    buildUpstreamUrl('/auth/me'),
    serverFetchInit({
      method: 'GET',
      headers: {
        Accept: 'application/json',
        Authorization: `Bearer ${accessToken}`,
      },
    }),
  );
}

export async function handleSession(
  request: NextRequest,
  fetcher: ServerFetch = fetch,
): Promise<NextResponse> {
  let accessToken = request.cookies.get(ACCESS_TOKEN_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_TOKEN_COOKIE)?.value;
  let refreshedPair: SessionTokenPair | null = null;

  if (!accessToken) {
    if (!refreshToken) return terminalUnauthorized();
    const refreshed = await refreshSession(refreshToken, fetcher);
    if (refreshed.kind === 'terminal') return terminalUnauthorized();
    if (refreshed.kind === 'unavailable') return refreshUnavailable();
    refreshedPair = refreshed.pair;
    accessToken = refreshed.pair.access_token;
  }

  let upstream: Response;
  try {
    upstream = await currentUser(accessToken, fetcher);
  } catch {
    return preserveRotatedSession(
      jsonError(502, 'UPSTREAM_UNAVAILABLE', 'Authentication service is unavailable'),
      refreshedPair,
    );
  }

  if (upstream.status === 401 && refreshToken && !refreshedPair) {
    const refreshed = await refreshSession(refreshToken, fetcher);
    if (refreshed.kind === 'terminal') return terminalUnauthorized();
    if (refreshed.kind === 'unavailable') return refreshUnavailable();
    refreshedPair = refreshed.pair;

    try {
      upstream = await currentUser(refreshed.pair.access_token, fetcher);
    } catch {
      return preserveRotatedSession(
        jsonError(502, 'UPSTREAM_UNAVAILABLE', 'Authentication service is unavailable'),
        refreshedPair,
      );
    }
  }

  if (upstream.status === 401) return terminalUnauthorized();

  return preserveRotatedSession(proxyResponse(upstream), refreshedPair);
}

export async function handleRefresh(
  request: NextRequest,
  fetcher: ServerFetch = fetch,
): Promise<NextResponse> {
  const csrf = verifyCsrf(request);
  if (!csrf.ok) return csrfErrorResponse(csrf.message);

  const refreshToken = request.cookies.get(REFRESH_TOKEN_COOKIE)?.value;
  if (!refreshToken) return terminalUnauthorized();

  const refreshed = await refreshSession(refreshToken, fetcher);
  if (refreshed.kind === 'terminal') return terminalUnauthorized();
  if (refreshed.kind === 'unavailable') return refreshUnavailable();

  const response = NextResponse.json(
    {
      expires_at: refreshed.pair.expires_at,
      user: refreshed.pair.user,
    },
    { headers: { 'Cache-Control': 'no-store' } },
  );
  setSessionCookies(response, refreshed.pair);
  return response;
}

async function revokeAccessToken(
  accessToken: string,
  fetcher: ServerFetch,
): Promise<Response | null> {
  try {
    return await fetcher(
      buildUpstreamUrl('/auth/logout'),
      serverFetchInit({
        method: 'POST',
        headers: { Authorization: `Bearer ${accessToken}` },
      }),
    );
  } catch {
    return null;
  }
}

export async function handleLogout(
  request: NextRequest,
  fetcher: ServerFetch = fetch,
): Promise<NextResponse> {
  const csrf = verifyCsrf(request);
  if (!csrf.ok) return csrfErrorResponse(csrf.message);

  let accessToken = request.cookies.get(ACCESS_TOKEN_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_TOKEN_COOKIE)?.value;
  const result = accessToken ? await revokeAccessToken(accessToken, fetcher) : null;

  if ((!result || result.status === 401) && refreshToken) {
    const refreshed = await refreshSession(refreshToken, fetcher);
    if (refreshed.kind === 'success') {
      accessToken = refreshed.pair.access_token;
      await revokeAccessToken(accessToken, fetcher);
    }
  }

  // Logout is deliberately idempotent from the browser's perspective. Local
  // credentials are cleared even when the upstream is unavailable.
  const response = new NextResponse(null, {
    status: 204,
    headers: { 'Cache-Control': 'no-store' },
  });
  clearSessionCookies(response);
  return response;
}
