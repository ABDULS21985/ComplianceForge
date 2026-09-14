const DEFAULT_API_BASE_URL = '/api/portal';

export const PORTAL_API_ROUTES = {
  vendorSession: '/vendor-portal/session',
  vendorQuestionnaire: '/vendor-portal/questionnaire',
  vendorSave: '/vendor-portal/save',
  vendorSubmit: '/vendor-portal/submit',
  boardSession: '/board-portal/session',
  boardData: '/board-portal',
} as const;

export type PortalApiRoute = (typeof PORTAL_API_ROUTES)[keyof typeof PORTAL_API_ROUTES];

/** Portal capabilities live in scoped HttpOnly cookies after bootstrap. */
export function buildPortalApiUrl(route: PortalApiRoute): string {
  return `${DEFAULT_API_BASE_URL}${route}`;
}

export function getPortalEntryToken(
  queryToken: string | null,
  hash: string,
): string | null {
  const fromQuery = queryToken?.trim();
  if (fromQuery) return fromQuery;

  const fragment = hash.startsWith('#') ? hash.slice(1) : hash;
  const fromFragment = new URLSearchParams(fragment).get('token')?.trim();
  return fromFragment || null;
}

/** Remove a consumed capability from both query and fragment history. */
export function cleanPortalUrl(rawUrl: string): string {
  const url = new URL(rawUrl);
  url.searchParams.delete('token');

  const fragment = url.hash.startsWith('#') ? url.hash.slice(1) : url.hash;
  const hashParams = new URLSearchParams(fragment);
  const containedFragmentToken = hashParams.has('token');
  if (containedFragmentToken) hashParams.delete('token');
  const cleanHash = containedFragmentToken ? hashParams.toString() : fragment;
  return `${url.pathname}${url.search}${cleanHash ? `#${cleanHash}` : ''}`;
}
