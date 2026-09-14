/** Cookie and header names shared by browser, route handlers, and Edge middleware. */
export const ACCESS_TOKEN_COOKIE = '__Host-cf_access_token';
export const REFRESH_TOKEN_COOKIE = '__Host-cf_refresh_token';
export const CSRF_TOKEN_COOKIE = '__Host-cf_csrf_token';
export const CSRF_TOKEN_HEADER = 'x-cf-csrf-token';
export const CSRF_ERROR_HEADER = 'x-cf-csrf-error';

/** Names used by releases that persisted bearer credentials in JavaScript. */
export const LEGACY_ACCESS_TOKEN_KEY = 'cf_access_token';
export const LEGACY_REFRESH_TOKEN_KEY = 'cf_refresh_token';

export const SESSION_EXPIRED_EVENT = 'complianceforge:session-expired';

export function isStateChangingMethod(method: string): boolean {
  return !['GET', 'HEAD', 'OPTIONS'].includes(method.toUpperCase());
}
