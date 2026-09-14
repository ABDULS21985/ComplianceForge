// ComplianceForge Auth Utilities
// Pure JWT parsing helpers. Browser session credentials are intentionally not
// exposed here: the same-origin BFF owns them in HttpOnly cookies.

import { LEGACY_ACCESS_TOKEN_KEY, LEGACY_REFRESH_TOKEN_KEY } from './auth-constants';

/** Remove credentials left in Web Storage by pre-BFF application releases. */
export function purgeLegacyBrowserCredentials(): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.removeItem(LEGACY_ACCESS_TOKEN_KEY);
    window.localStorage.removeItem(LEGACY_REFRESH_TOKEN_KEY);
  } catch {
    // Storage can be unavailable in hardened/private browsing contexts.
  }
}

// ---------------------------------------------------------------------------
// JWT decoding (no verification -- that is the server's job)
// ---------------------------------------------------------------------------

export interface JwtPayload {
  sub: string;
  email: string;
  exp: number;
  iat: number;
  roles?: string[];
  org_id?: string;
  first_name?: string;
  last_name?: string;
  [key: string]: unknown;
}

export function decodeJwt(token: string): JwtPayload | null {
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const payload = parts[1];
    // Base64url to Base64
    const base64 = payload.replace(/-/g, '+').replace(/_/g, '/');
    const json = atob(base64);
    return JSON.parse(json) as JwtPayload;
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Token helpers (useful for isolated validation/tests, not browser sessions)
// ---------------------------------------------------------------------------

export function isTokenAuthenticated(token: string | null): boolean {
  if (!token) return false;
  const payload = decodeJwt(token);
  if (!payload) return false;
  // exp is in seconds, Date.now() is in milliseconds
  const nowSec = Math.floor(Date.now() / 1000);
  return payload.exp > nowSec;
}
