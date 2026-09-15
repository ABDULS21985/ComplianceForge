import { AUTH_REDIRECT_QUERY_PARAM, isPublicRoute, ROUTES } from '@/lib/routes';
import {
  LEGACY_ACCESS_TOKEN_KEY,
  LEGACY_REFRESH_TOKEN_KEY,
} from '@/lib/auth-constants';
import type { NextRequest } from 'next/server';
import { NextResponse } from 'next/server';
import { sessionCookiePolicy } from '@/lib/request-security';

function clearLegacyCookie(request: NextRequest, response: NextResponse): NextResponse {
  for (const name of [LEGACY_ACCESS_TOKEN_KEY, LEGACY_REFRESH_TOKEN_KEY]) {
    if (!request.cookies.has(name)) continue;
    response.cookies.set(name, '', {
      expires: new Date(0),
      maxAge: 0,
      path: '/',
      sameSite: 'lax',
      secure: request.nextUrl.protocol === 'https:',
    });
  }
  return response;
}

function isProxyBypassPath(pathname: string): boolean {
  return (
    pathname === '/api' ||
    pathname.startsWith('/api/') ||
    pathname === '/favicon.ico' ||
    pathname.startsWith('/_next/')
  );
}

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;

  if (isProxyBypassPath(pathname)) {
    return clearLegacyCookie(request, NextResponse.next());
  }

  // Public authentication and invitation-based portal entry points.
  if (isPublicRoute(pathname)) {
    return clearLegacyCookie(request, NextResponse.next());
  }

  // A refresh-only session is allowed through so the BFF can rotate it.
  const cookiePolicy = sessionCookiePolicy(request);
  const accessToken = request.cookies.get(cookiePolicy.access)?.value;
  const refreshToken = request.cookies.get(cookiePolicy.refresh)?.value;

  if (!accessToken && !refreshToken) {
    const loginUrl = new URL(ROUTES.auth.login, request.url);
    loginUrl.searchParams.set(
      AUTH_REDIRECT_QUERY_PARAM,
      `${pathname}${request.nextUrl.search}`,
    );
    return clearLegacyCookie(request, NextResponse.redirect(loginUrl));
  }

  return clearLegacyCookie(request, NextResponse.next());
}

export const config = {
  matcher: [
    /*
     * Match all request paths except:
     * - _next/static (static files)
     * - _next/image (image optimization files)
     * - favicon.ico (favicon file)
     */
    '/((?!_next/static|_next/image|favicon.ico).*)',
  ],
};
