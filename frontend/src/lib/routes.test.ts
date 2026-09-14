import { describe, expect, it } from 'vitest';

import {
  buildQuickCreateRoute,
  getSafePostAuthRedirect,
  isPublicRoute,
  isQuickCreateRequest,
  QUICK_CREATE_ROUTES,
  ROUTES,
} from '@/lib/routes';

describe('route contracts', () => {
  it.each([
    ROUTES.auth.login,
    ROUTES.auth.forgotPassword,
    ROUTES.portals.vendor,
    ROUTES.portals.board,
  ])('marks %s as public', (route) => {
    expect(isPublicRoute(route)).toBe(true);
    expect(isPublicRoute(`${route}/`)).toBe(true);
  });

  it('does not publicize paths that merely share an auth prefix', () => {
    expect(isPublicRoute('/login-not-public')).toBe(false);
    expect(isPublicRoute('/vendor-portal-admin')).toBe(false);
    expect(isPublicRoute(ROUTES.dashboard)).toBe(false);
  });

  it.each([
    ['risk', QUICK_CREATE_ROUTES.risk],
    ['incident', QUICK_CREATE_ROUTES.incident],
    ['policy', QUICK_CREATE_ROUTES.policy],
    ['audit', QUICK_CREATE_ROUTES.audit],
  ] as const)('builds the canonical %s quick-create route', (resource, route) => {
    expect(buildQuickCreateRoute(resource)).toBe(route);
    expect(route).not.toContain('/new');
    expect(isQuickCreateRequest(new URL(route, 'https://app.test').searchParams, resource)).toBe(true);
  });
});

describe('post-auth redirects', () => {
  it('preserves an internal path, query string, and fragment', () => {
    expect(getSafePostAuthRedirect('/risks?page=2#open')).toBe(
      '/risks?page=2#open'
    );
  });

  it.each([
    'https://evil.example/steal',
    '//evil.example/steal',
    '/\\evil.example/steal',
    'dashboard',
    '/login',
    '/forgot-password',
  ])('rejects unsafe or looping destination %s', (destination) => {
    expect(getSafePostAuthRedirect(destination)).toBe(ROUTES.dashboard);
  });
});
