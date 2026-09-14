import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

import {
  AUTH_REDIRECT_QUERY_PARAM,
  isPublicRoute,
  ROUTES,
} from "@/lib/routes";
import { ACCESS_TOKEN_KEY } from "@/lib/auth-constants";

function isMiddlewareBypassPath(pathname: string): boolean {
  return (
    pathname === "/api" ||
    pathname.startsWith("/api/") ||
    pathname === "/favicon.ico" ||
    pathname.startsWith("/_next/")
  );
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // Skip middleware for static assets and API routes
  if (isMiddlewareBypassPath(pathname)) {
    return NextResponse.next();
  }

  // Public authentication and invitation-based portal entry points.
  if (isPublicRoute(pathname)) {
    return NextResponse.next();
  }

  // Check for auth token in cookies
  const token = request.cookies.get(ACCESS_TOKEN_KEY)?.value;

  // If no token and trying to access a protected page, redirect to login
  if (!token) {
    const loginUrl = new URL(ROUTES.auth.login, request.url);
    loginUrl.searchParams.set(
      AUTH_REDIRECT_QUERY_PARAM,
      `${pathname}${request.nextUrl.search}`
    );
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    /*
     * Match all request paths except:
     * - _next/static (static files)
     * - _next/image (image optimization files)
     * - favicon.ico (favicon file)
     */
    "/((?!_next/static|_next/image|favicon.ico).*)",
  ],
};
