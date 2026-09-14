import { describe, expect, it } from 'vitest';

import {
  buildPortalApiUrl,
  PORTAL_API_ROUTES,
} from '@/lib/portal-routes';

describe('portal API route contracts', () => {
  it('uses the same-origin portal BFF by default', () => {
    expect(
      buildPortalApiUrl(PORTAL_API_ROUTES.boardData, 'invite-token'),
    ).toBe('/api/portal/board-portal?token=invite-token');
  });

  it.each(Object.values(PORTAL_API_ROUTES))(
    'encodes invite tokens for %s',
    (route) => {
      const url = new URL(
        buildPortalApiUrl(route, 'signed+/token=?&value'),
        'https://app.example.test',
      );

      expect(url.pathname).toBe(`/api/portal${route}`);
      expect(url.searchParams.get('token')).toBe('signed+/token=?&value');
    }
  );
});
