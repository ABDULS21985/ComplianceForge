import { describe, expect, it } from 'vitest';

import nextConfig from '../../next.config.js';

describe('direct frontend response security headers', () => {
  it('applies the security baseline without relying on nginx', async () => {
    const rules = await nextConfig.headers!();
    const baseline = rules.find((rule) => rule.source === '/:path*')?.headers ?? [];
    const headers = new Map(baseline.map(({ key, value }) => [key.toLowerCase(), value]));

    expect(headers.get('content-security-policy')).toContain("default-src 'self'");
    expect(headers.get('content-security-policy')).toContain("frame-ancestors 'none'");
    expect(headers.get('permissions-policy')).toContain('camera=()');
    expect(headers.get('referrer-policy')).toBe('strict-origin-when-cross-origin');
    expect(headers.get('x-content-type-options')).toBe('nosniff');
    expect(headers.get('x-frame-options')).toBe('DENY');
  });

  it.each([
    '/vendor-portal',
    '/board-portal',
    '/accept-invitation',
    '/reset-password',
    '/verify-email',
  ])(
    'prevents caching, indexing, and one-time credential referrer leakage on %s',
    async (source) => {
      const rules = await nextConfig.headers!();
      const sensitiveRoute = rules.find((rule) => rule.source === source)?.headers ?? [];
      const headers = new Map(
        sensitiveRoute.map(({ key, value }) => [key.toLowerCase(), value]),
      );

      expect(headers.get('cache-control')).toBe('no-store');
      expect(headers.get('referrer-policy')).toBe('no-referrer');
      expect(headers.get('x-robots-tag')).toContain('noindex');
    },
  );
});
