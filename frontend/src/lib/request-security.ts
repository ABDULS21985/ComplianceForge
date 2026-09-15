import {
  ACCESS_TOKEN_COOKIE,
  BOARD_PORTAL_TOKEN_COOKIE,
  CSRF_TOKEN_COOKIE,
  DEVELOPMENT_ACCESS_TOKEN_COOKIE,
  DEVELOPMENT_BOARD_PORTAL_TOKEN_COOKIE,
  DEVELOPMENT_CSRF_TOKEN_COOKIE,
  DEVELOPMENT_REFRESH_TOKEN_COOKIE,
  DEVELOPMENT_VENDOR_PORTAL_TOKEN_COOKIE,
  REFRESH_TOKEN_COOKIE,
  VENDOR_PORTAL_TOKEN_COOKIE,
} from '@/lib/auth-constants';
import type { NextRequest } from 'next/server';

type RequestContext = Pick<NextRequest, 'headers' | 'nextUrl'>;

function canonicalOrigin(value: string, label: string): string {
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`${label} must be an absolute URL`);
  }

  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new Error(`${label} must use http or https`);
  }
  if (
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    (parsed.pathname && parsed.pathname !== '/')
  ) {
    throw new Error(`${label} must contain only scheme, host, and optional port`);
  }
  return parsed.origin;
}

function unambiguousHeader(request: RequestContext, name: string): string | null {
  const value = request.headers.get(name)?.trim();
  if (!value) return null;
  if (value.includes(',')) throw new Error(`${name} must contain a single value`);
  return value;
}

/**
 * Resolve the browser-visible origin without implicitly trusting spoofable
 * forwarding headers. APP_ORIGIN is preferred; proxy headers are considered
 * only when the deployment explicitly opts in and the edge proxy overwrites
 * them.
 */
export function publicRequestOrigin(request: RequestContext): string {
  const configured = process.env.APP_ORIGIN?.trim();
  if (configured) return canonicalOrigin(configured, 'APP_ORIGIN');

  if (process.env.TRUST_PROXY_HEADERS === 'true') {
    const protocol = unambiguousHeader(request, 'x-forwarded-proto');
    const hostname =
      unambiguousHeader(request, 'x-forwarded-host') ??
      unambiguousHeader(request, 'host');
    if (!protocol || !hostname) {
      throw new Error('Trusted proxy requests require forwarded protocol and host');
    }
    return canonicalOrigin(`${protocol}://${hostname}`, 'Forwarded public origin');
  }

  return canonicalOrigin(request.nextUrl.origin, 'Request origin');
}

function explicitlyLocalDevelopment(): boolean {
  if (process.env.APP_ENV) return process.env.APP_ENV === 'development';
  return process.env.NODE_ENV === 'development';
}

function isLoopbackHostname(hostname: string): boolean {
  return (
    hostname === 'localhost' ||
    hostname.endsWith('.localhost') ||
    hostname === '[::1]' ||
    /^127(?:\.\d{1,3}){3}$/.test(hostname)
  );
}

/** Secure is disabled only for explicit development on an HTTP loopback URL. */
export function requestUsesSecureCookies(request: RequestContext): boolean {
  const origin = new URL(publicRequestOrigin(request));
  return !(
    explicitlyLocalDevelopment() &&
    origin.protocol === 'http:' &&
    isLoopbackHostname(origin.hostname)
  );
}

export function sessionCookiePolicy(request: RequestContext) {
  const secure = requestUsesSecureCookies(request);
  return {
    access: secure ? ACCESS_TOKEN_COOKIE : DEVELOPMENT_ACCESS_TOKEN_COOKIE,
    csrf: secure ? CSRF_TOKEN_COOKIE : DEVELOPMENT_CSRF_TOKEN_COOKIE,
    refresh: secure ? REFRESH_TOKEN_COOKIE : DEVELOPMENT_REFRESH_TOKEN_COOKIE,
    secure,
  } as const;
}

export function portalCookiePolicy(
  request: RequestContext,
  portal: 'board' | 'vendor',
) {
  const secure = requestUsesSecureCookies(request);
  const name =
    portal === 'vendor'
      ? secure
        ? VENDOR_PORTAL_TOKEN_COOKIE
        : DEVELOPMENT_VENDOR_PORTAL_TOKEN_COOKIE
      : secure
        ? BOARD_PORTAL_TOKEN_COOKIE
        : DEVELOPMENT_BOARD_PORTAL_TOKEN_COOKIE;
  return { name, secure } as const;
}
