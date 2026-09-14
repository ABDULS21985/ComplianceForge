/**
 * Canonical application route contracts.
 *
 * Keep paths in this module free of browser and React dependencies so the
 * same contracts can be used by middleware, server components, client pages,
 * and unit tests.
 */
export const ROUTES = {
  home: '/',
  dashboard: '/dashboard',
  auth: {
    login: '/login',
    forgotPassword: '/forgot-password',
  },
  portals: {
    vendor: '/vendor-portal',
    board: '/board-portal',
  },
  risks: '/risks',
  incidents: '/incidents',
  policies: '/policies',
  audits: '/audits',
  frameworks: '/frameworks',
  settings: '/settings',
} as const;

export const AUTH_REDIRECT_QUERY_PARAM = 'redirect';
export const QUICK_CREATE_QUERY_PARAM = 'create';

export type QuickCreateResource = 'risk' | 'incident' | 'policy' | 'audit';

const QUICK_CREATE_LIST_ROUTES: Record<QuickCreateResource, string> = {
  risk: ROUTES.risks,
  incident: ROUTES.incidents,
  policy: ROUTES.policies,
  audit: ROUTES.audits,
};

export function buildQuickCreateRoute(resource: QuickCreateResource): string {
  const params = new URLSearchParams({
    [QUICK_CREATE_QUERY_PARAM]: resource,
  });
  return `${QUICK_CREATE_LIST_ROUTES[resource]}?${params.toString()}`;
}

export const QUICK_CREATE_ROUTES = {
  risk: buildQuickCreateRoute('risk'),
  incident: buildQuickCreateRoute('incident'),
  policy: buildQuickCreateRoute('policy'),
  audit: buildQuickCreateRoute('audit'),
} as const;

export const PUBLIC_ROUTES = [
  ROUTES.auth.login,
  ROUTES.auth.forgotPassword,
  ROUTES.portals.vendor,
  ROUTES.portals.board,
] as const;

const AUTH_ROUTES = [ROUTES.auth.login, ROUTES.auth.forgotPassword] as const;

function normalizePathname(pathname: string): string {
  if (pathname.length <= 1) return pathname || ROUTES.home;
  return pathname.replace(/\/+$/, '');
}

export function isPublicRoute(pathname: string): boolean {
  const normalized = normalizePathname(pathname);
  return PUBLIC_ROUTES.some((route) => route === normalized);
}

export function isQuickCreateRequest(
  searchParams: Pick<URLSearchParams, 'get'>,
  resource: QuickCreateResource
): boolean {
  return searchParams.get(QUICK_CREATE_QUERY_PARAM) === resource;
}

/**
 * Accept only same-origin, application-relative post-auth destinations.
 * Invalid, external, and auth-loop destinations fall back to the dashboard.
 */
export function getSafePostAuthRedirect(
  candidate: string | null | undefined,
  fallback = ROUTES.dashboard
): string {
  if (!candidate) return fallback;

  const value = candidate.trim();
  if (!value.startsWith('/') || value.startsWith('//')) return fallback;

  try {
    const base = new URL('https://complianceforge.invalid');
    const parsed = new URL(value, base);
    if (parsed.origin !== base.origin) return fallback;

    const pathname = normalizePathname(parsed.pathname);
    if (AUTH_ROUTES.some((route) => route === pathname)) return fallback;

    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return fallback;
  }
}
