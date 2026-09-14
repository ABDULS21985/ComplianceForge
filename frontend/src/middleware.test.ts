import { NextRequest } from 'next/server';
import { describe, expect, it } from 'vitest';

import { middleware } from '@/middleware';
import { ACCESS_TOKEN_KEY } from '@/lib/auth-constants';
import { AUTH_REDIRECT_QUERY_PARAM, ROUTES } from '@/lib/routes';

describe('authentication middleware routing', () => {
  it.each([ROUTES.portals.vendor, ROUTES.portals.board])(
    'allows unauthenticated access to %s',
    (route) => {
      const response = middleware(
        new NextRequest(`https://app.example.test${route}?token=invite-token`)
      );

      expect(response.headers.get('x-middleware-next')).toBe('1');
      expect(response.headers.get('location')).toBeNull();
    }
  );

  it('redirects protected routes and preserves their query string', () => {
    const response = middleware(
      new NextRequest('https://app.example.test/risks?page=3&status=open')
    );
    const location = response.headers.get('location');

    expect(location).not.toBeNull();
    const redirect = new URL(location as string);
    expect(redirect.pathname).toBe(ROUTES.auth.login);
    expect(redirect.searchParams.get(AUTH_REDIRECT_QUERY_PARAM)).toBe(
      '/risks?page=3&status=open'
    );
  });

  it('allows an authenticated request through', () => {
    const response = middleware(
      new NextRequest('https://app.example.test/dashboard', {
        headers: { cookie: `${ACCESS_TOKEN_KEY}=present` },
      })
    );

    expect(response.headers.get('x-middleware-next')).toBe('1');
    expect(response.headers.get('location')).toBeNull();
  });

  it('does not bypass auth for paths that only share a static prefix', () => {
    const response = middleware(
      new NextRequest('https://app.example.test/apiary')
    );

    expect(response.headers.get('location')).not.toBeNull();
  });
});
