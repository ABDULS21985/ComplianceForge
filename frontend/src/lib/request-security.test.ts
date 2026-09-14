import { NextRequest } from 'next/server';
import { afterEach, describe, expect, it } from 'vitest';

import {
  ACCESS_TOKEN_COOKIE,
  DEVELOPMENT_ACCESS_TOKEN_COOKIE,
} from '@/lib/auth-constants';
import {
  publicRequestOrigin,
  requestUsesSecureCookies,
  sessionCookiePolicy,
} from '@/lib/request-security';

const originalEnvironment = {
  APP_ENV: process.env.APP_ENV,
  APP_ORIGIN: process.env.APP_ORIGIN,
  TRUST_PROXY_HEADERS: process.env.TRUST_PROXY_HEADERS,
};

function restoreEnvironment(name: keyof typeof originalEnvironment): void {
  const value = originalEnvironment[name];
  if (value === undefined) delete process.env[name];
  else process.env[name] = value;
}

afterEach(() => {
  restoreEnvironment('APP_ENV');
  restoreEnvironment('APP_ORIGIN');
  restoreEnvironment('TRUST_PROXY_HEADERS');
});

describe('public request origin', () => {
  it('uses explicitly trusted proxy protocol and host instead of the internal Next URL', () => {
    process.env.TRUST_PROXY_HEADERS = 'true';
    const request = new NextRequest('http://frontend:3000/api/bff/risks', {
      headers: {
        host: 'app.example.test',
        'x-forwarded-host': 'app.example.test',
        'x-forwarded-proto': 'https',
      },
    });

    expect(publicRequestOrigin(request)).toBe('https://app.example.test');
  });

  it('ignores forwarding headers unless the deployment opts in', () => {
    delete process.env.TRUST_PROXY_HEADERS;
    const request = new NextRequest('http://frontend:3000/api/bff/risks', {
      headers: {
        host: 'app.example.test',
        'x-forwarded-host': 'attacker.example',
        'x-forwarded-proto': 'https',
      },
    });

    expect(publicRequestOrigin(request)).toBe('http://frontend:3000');
  });

  it('rejects ambiguous forwarded values and non-origin APP_ORIGIN values', () => {
    process.env.TRUST_PROXY_HEADERS = 'true';
    const ambiguous = new NextRequest('http://frontend:3000/api/bff/risks', {
      headers: {
        host: 'app.example.test',
        'x-forwarded-proto': 'https,http',
      },
    });
    expect(() => publicRequestOrigin(ambiguous)).toThrow(/single value/);

    process.env.APP_ORIGIN = 'https://user:password@app.example.test/path?query=1';
    expect(() => publicRequestOrigin(ambiguous)).toThrow(/only scheme, host/);
  });
});

describe('session cookie environment policy', () => {
  it('uses isolated non-prefix cookies only for explicit loopback HTTP development', () => {
    process.env.APP_ENV = 'development';
    const request = new NextRequest('http://localhost:3000/api/auth/login');

    expect(requestUsesSecureCookies(request)).toBe(false);
    expect(sessionCookiePolicy(request).access).toBe(DEVELOPMENT_ACCESS_TOKEN_COOKIE);
  });

  it('forces Secure host cookies for production even on a misconfigured HTTP URL', () => {
    process.env.APP_ENV = 'production';
    const request = new NextRequest('http://localhost:3000/api/auth/login');

    expect(requestUsesSecureCookies(request)).toBe(true);
    expect(sessionCookiePolicy(request).access).toBe(ACCESS_TOKEN_COOKIE);
  });

  it('never permits insecure cookies on a non-loopback development host', () => {
    process.env.APP_ENV = 'development';
    const request = new NextRequest('http://dev.example.test/api/auth/login');

    expect(requestUsesSecureCookies(request)).toBe(true);
  });
});
