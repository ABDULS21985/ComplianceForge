import { describe, expect, it } from 'vitest';

import {
  buildPortalApiUrl,
  PORTAL_API_ROUTES,
} from '@/lib/portal-routes';

describe('portal API route contracts', () => {
  it.each(Object.values(PORTAL_API_ROUTES))(
    'encodes invite tokens for %s',
    (route) => {
      const url = buildPortalApiUrl(
        route,
        'signed+/token=?&value',
        'https://api.example.test/api/v1/'
      );
      const parsed = new URL(url);

      expect(parsed.pathname).toBe(`/api/v1${route}`);
      expect(parsed.searchParams.get('token')).toBe('signed+/token=?&value');
    }
  );
});
