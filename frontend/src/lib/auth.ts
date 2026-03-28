// ComplianceForge Auth Utilities
// JWT token management, decoding, and session helpers

const ACCESS_TOKEN_KEY = "cf_access_token";
const REFRESH_TOKEN_KEY = "cf_refresh_token";

// ---------------------------------------------------------------------------
// Token storage
// ---------------------------------------------------------------------------

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function setToken(token: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(ACCESS_TOKEN_KEY, token);
  // Also set as cookie so middleware can read it
  document.cookie = `${ACCESS_TOKEN_KEY}=${token}; path=/; max-age=${60 * 60 * 24 * 7}; SameSite=Lax`;
}

export function getRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function setRefreshToken(token: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(REFRESH_TOKEN_KEY, token);
}

export function clearToken(): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  document.cookie = `${ACCESS_TOKEN_KEY}=; path=/; max-age=0`;
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

function decodeJwt(token: string): JwtPayload | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const payload = parts[1];
    // Base64url to Base64
    const base64 = payload.replace(/-/g, "+").replace(/_/g, "/");
    const json = atob(base64);
    return JSON.parse(json) as JwtPayload;
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Session helpers
// ---------------------------------------------------------------------------

/**
 * Returns true if a non-expired access token exists in localStorage.
 */
export function isAuthenticated(): boolean {
  const token = getToken();
  if (!token) return false;
  const payload = decodeJwt(token);
  if (!payload) return false;
  // exp is in seconds, Date.now() is in milliseconds
  const nowSec = Math.floor(Date.now() / 1000);
  return payload.exp > nowSec;
}

/**
 * Decode and return the user information embedded in the JWT.
 * Returns null if no valid token exists.
 */
export function getUserFromToken(): JwtPayload | null {
  const token = getToken();
  if (!token) return null;
  return decodeJwt(token);
}

/**
 * Full logout: clear tokens, wipe the React Query cache, and redirect.
 *
 * Accepts an optional QueryClient so we can call `clear()` on it.
 * If called without one (e.g. from outside React), it still clears storage
 * and redirects.
 */
export function logout(queryClient?: { clear: () => void }): void {
  clearToken();
  if (queryClient) {
    queryClient.clear();
  }
  if (typeof window !== "undefined") {
    window.location.href = "/login";
  }
}
