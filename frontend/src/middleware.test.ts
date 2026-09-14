import { NextRequest } from 'next/server';
import { afterEach, describe, expect, it } from 'vitest';

import { middleware } from '@/middleware';
import {
  ACCESS_TOKEN_COOKIE,
  DEVELOPMENT_ACCESS_TOKEN_COOKIE,
  REFRESH_TOKEN_COOKIE,
} from '@/lib/auth-constants';
import { AUTH_REDIRECT_QUERY_PARAM, ROUTES } from '@/lib/routes';

const originalAppEnvironment = process.env.APP_ENV;

afterEach(() => {
  if (originalAppEnvironment === undefined) delete process.env.APP_ENV;
  else process.env.APP_ENV = originalAppEnvironment;
});

describe('authentication middleware routing', () => {
  it.each([ROUTES.portals.vendor, ROUTES.portals.board])(
    'allows unauthenticated access to %s',
    (route) => {
      const response = middleware(
        new NextRequest(`https://app.example.test${route}?token=invite-token`),
      );

      expect(response.headers.get('x-middleware-next')).toBe('1');
      expect(response.headers.get('location')).toBeNull();
    },
  );

  it('redirects protected routes and preserves their query string', () => {
    const response = middleware(
      new NextRequest('https://app.example.test/risks?page=3&status=open'),
    );
    const location = response.headers.get('location');

    expect(location).not.toBeNull();
    const redirect = new URL(location as string);
    expect(redirect.pathname).toBe(ROUTES.auth.login);
    expect(redirect.searchParams.get(AUTH_REDIRECT_QUERY_PARAM)).toBe('/risks?page=3&status=open');
  });

  it('allows an authenticated request through', () => {
    const response = middleware(
      new NextRequest('https://app.example.test/dashboard', {
        headers: { cookie: `${ACCESS_TOKEN_COOKIE}=present` },
      }),
    );

    expect(response.headers.get('x-middleware-next')).toBe('1');
    expect(response.headers.get('location')).toBeNull();
  });

  it('allows a refresh-only session through for server-side rotation', () => {
    const response = middleware(
      new NextRequest('https://app.example.test/dashboard', {
        headers: { cookie: `${REFRESH_TOKEN_COOKIE}=present` },
      }),
    );

    expect(response.headers.get('x-middleware-next')).toBe('1');
    expect(response.headers.get('location')).toBeNull();
  });

  it('recognizes the isolated loopback HTTP development cookie', () => {
    process.env.APP_ENV = 'development';
    const response = middleware(
      new NextRequest('http://localhost:3000/dashboard', {
        headers: { cookie: `${DEVELOPMENT_ACCESS_TOKEN_COOKIE}=present` },
      }),
    );

    expect(response.headers.get('x-middleware-next')).toBe('1');
    expect(response.headers.get('location')).toBeNull();
  });

  it('does not bypass auth for paths that only share a static prefix', () => {
    const response = middleware(new NextRequest('https://app.example.test/apiary'));

    expect(response.headers.get('location')).not.toBeNull();
  });

  it('expires a legacy JavaScript-readable token cookie during migration', () => {
    const response = middleware(
      new NextRequest('https://app.example.test/login', {
        headers: {
          cookie: 'cf_access_token=legacy-token; cf_refresh_token=legacy-refresh',
        },
      }),
    );

    expect(response.headers.get('set-cookie')).toContain('cf_access_token=');
    expect(response.headers.get('set-cookie')).toContain('cf_refresh_token=');
    expect(response.headers.get('set-cookie')).toMatch(/Max-Age=0/i);
  });
});
