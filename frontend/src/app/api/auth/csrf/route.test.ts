import { NextRequest } from 'next/server';
import { describe, expect, it } from 'vitest';

import { CSRF_TOKEN_COOKIE } from '@/lib/auth-constants';
import { GET } from './route';

describe('CSRF bootstrap route', () => {
  it('issues a secure cookie and reuses it across browser tabs', async () => {
    const first = GET(
      new NextRequest('https://app.example.test/api/auth/csrf', {
        headers: { 'sec-fetch-site': 'same-origin' },
      }),
    );
    const firstPayload = await first.json();
    const token = firstPayload.csrf_token as string;

    expect(token).toMatch(/^[A-Za-z0-9_-]{43}$/);
    expect(first.headers.get('set-cookie')).toContain(`${CSRF_TOKEN_COOKIE}=`);

    const second = GET(
      new NextRequest('https://app.example.test/api/auth/csrf', {
        headers: {
          cookie: `${CSRF_TOKEN_COOKIE}=${token}`,
          'sec-fetch-site': 'same-origin',
        },
      }),
    );
    await expect(second.json()).resolves.toEqual({ csrf_token: token });
    expect(second.headers.get('set-cookie')).toBeNull();
  });

  it('does not set a cookie for a cross-site subresource request', () => {
    const response = GET(
      new NextRequest('https://app.example.test/api/auth/csrf', {
        headers: { 'sec-fetch-site': 'cross-site' },
      }),
    );

    expect(response.status).toBe(403);
    expect(response.headers.get('set-cookie')).toBeNull();
  });
});
