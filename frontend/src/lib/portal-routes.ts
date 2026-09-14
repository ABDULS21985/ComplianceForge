const DEFAULT_API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080/api/v1';

export const PORTAL_API_ROUTES = {
  vendorQuestionnaire: '/vendor-portal/questionnaire',
  vendorSave: '/vendor-portal/save',
  vendorSubmit: '/vendor-portal/submit',
  boardData: '/board-portal',
} as const;

export type PortalApiRoute =
  (typeof PORTAL_API_ROUTES)[keyof typeof PORTAL_API_ROUTES];

/** Build a portal endpoint without interpolating unescaped invite tokens. */
export function buildPortalApiUrl(
  route: PortalApiRoute,
  token: string,
  baseUrl = DEFAULT_API_BASE_URL
): string {
  const normalizedBase = baseUrl.replace(/\/+$/, '');
  const params = new URLSearchParams({ token });
  return `${normalizedBase}${route}?${params.toString()}`;
}
