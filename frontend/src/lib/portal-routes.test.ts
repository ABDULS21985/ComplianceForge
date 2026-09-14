import { describe, expect, it } from 'vitest';

import {
  buildPortalApiUrl,
  cleanPortalUrl,
  getPortalEntryToken,
  PORTAL_API_ROUTES,
} from '@/lib/portal-routes';

describe('portal API route contracts', () => {
  it('uses the same-origin portal BFF by default', () => {
    expect(
      buildPortalApiUrl(PORTAL_API_ROUTES.boardData),
    ).toBe('/api/portal/board-portal');
  });

  it.each(Object.values(PORTAL_API_ROUTES))(
    'never places invite tokens in the BFF URL for %s',
    (route) => {
      const url = new URL(
        buildPortalApiUrl(route),
        'https://app.example.test',
      );

      expect(url.pathname).toBe(`/api/portal${route}`);
      expect(url.search).toBe('');
    }
  );

  it('reads query and fragment entry capabilities and cleans browser history', () => {
    expect(getPortalEntryToken(' query-token ', '#token=fragment-token')).toBe('query-token');
    expect(getPortalEntryToken(null, '#token=fragment-token')).toBe('fragment-token');
    expect(
      cleanPortalUrl('https://app.example.test/vendor-portal?token=secret&locale=en#token=other'),
    ).toBe('/vendor-portal?locale=en');
    expect(
      cleanPortalUrl('https://app.example.test/vendor-portal?token=secret#questions'),
    ).toBe('/vendor-portal#questions');
  });
});
