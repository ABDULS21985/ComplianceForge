const DEFAULT_API_BASE_URL = '/api/portal';

export const PORTAL_API_ROUTES = {
  vendorQuestionnaire: '/vendor-portal/questionnaire',
  vendorSave: '/vendor-portal/save',
  vendorSubmit: '/vendor-portal/submit',
  boardData: '/board-portal',
} as const;

export type PortalApiRoute = (typeof PORTAL_API_ROUTES)[keyof typeof PORTAL_API_ROUTES];

/** Build a portal endpoint without interpolating unescaped invite tokens. */
export function buildPortalApiUrl(
  route: PortalApiRoute,
  token: string,
): string {
  const params = new URLSearchParams({ token });
  return `${DEFAULT_API_BASE_URL}${route}?${params.toString()}`;
}
