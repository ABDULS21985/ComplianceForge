import {
  ACCESS_TOKEN_COOKIE,
  CSRF_TOKEN_COOKIE,
  CSRF_TOKEN_HEADER,
  REFRESH_TOKEN_COOKIE,
} from '@/lib/auth-constants';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  handleCredentialExchange,
  handleLogout,
  handleRefresh,
  proxyAuthenticatedRequest,
  refreshSession,
} from '@/lib/server/session-bff';
import { NextRequest } from 'next/server';

const origin = 'https://app.example.test';

function pair(access = 'access-new', refresh = 'refresh-new') {
  return {
    access_token: access,
    refresh_token: refresh,
    expires_at: new Date(Date.now() + 60 * 60_000).toISOString(),
    user: { id: 'user-1', email: 'user@example.test' },
  };
}

function request(
  path: string,
  options: {
    access?: string;
    body?: string;
    csrf?: string;
    method?: string;
    refresh?: string;
  } = {},
) {
  const cookies = [
    options.access && `${ACCESS_TOKEN_COOKIE}=${options.access}`,
    options.refresh && `${REFRESH_TOKEN_COOKIE}=${options.refresh}`,
    options.csrf && `${CSRF_TOKEN_COOKIE}=${options.csrf}`,
  ].filter(Boolean);
  const headers: Record<string, string> = {
    host: 'app.example.test',
    ...(cookies.length ? { cookie: cookies.join('; ') } : {}),
  };
  if (options.csrf) {
    headers.origin = origin;
    headers['sec-fetch-site'] = 'same-origin';
    headers[CSRF_TOKEN_HEADER] = options.csrf;
    headers['content-type'] = 'application/json';
  }

  return new NextRequest(`${origin}${path}`, {
    method: options.method ?? 'GET',
    headers,
    body: options.body,
  });
}

describe('session BFF', () => {
  beforeEach(() => {
    process.env.API_INTERNAL_URL = 'http://api.internal.test/api/v1';
  });

  it('strips upstream bearer tokens from a successful login response', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(Response.json(pair('access-secret', 'refresh-secret')));
    const response = await handleCredentialExchange(
      request('/api/auth/login', {
        body: JSON.stringify({ email: 'user@example.test', password: 'password' }),
        csrf: 'csrf-token',
        method: 'POST',
      }),
      'login',
      fetcher,
    );

    expect(response.status).toBe(200);
    const payload = await response.json();
    expect(payload).toEqual({
      expires_at: expect.any(String),
      user: { id: 'user-1', email: 'user@example.test' },
    });
    expect(JSON.stringify(payload)).not.toContain('access-secret');
    expect(JSON.stringify(payload)).not.toContain('refresh-secret');
    expect(response.headers.get('set-cookie')).toContain('HttpOnly');
    expect(response.headers.get('set-cookie')).toContain('Secure');
  });

  it('rejects a state-changing request before contacting upstream when CSRF proof is absent', async () => {
    const fetcher = vi.fn();
    const response = await proxyAuthenticatedRequest(
      request('/api/bff/risks', {
        access: 'access-old',
        method: 'POST',
        body: '{}',
      }),
      ['risks'],
      fetcher,
    );

    expect(response.status).toBe(403);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('rotates both cookies and retries the original request after an access-token 401', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(Response.json({ message: 'expired' }, { status: 401 }))
      .mockResolvedValueOnce(Response.json(pair()))
      .mockResolvedValueOnce(Response.json({ id: 'risk-1' }));

    const response = await proxyAuthenticatedRequest(
      request('/api/bff/risks/risk-1', {
        access: 'access-old',
        refresh: 'refresh-old',
      }),
      ['risks', 'risk-1'],
      fetcher,
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ id: 'risk-1' });
    expect(fetcher).toHaveBeenCalledTimes(3);
    expect(fetcher.mock.calls[1][0]).toContain('/auth/refresh');
    expect(fetcher.mock.calls[1][1]?.body).toContain('refresh-old');
    expect(new Headers(fetcher.mock.calls[2][1]?.headers).get('authorization')).toBe(
      'Bearer access-new',
    );
    const setCookie = response.headers.get('set-cookie') ?? '';
    expect(setCookie).toContain(`${ACCESS_TOKEN_COOKIE}=access-new`);
    expect(setCookie).toContain(`${REFRESH_TOKEN_COOKIE}=refresh-new`);
  });

  it('returns only browser-safe fields from an explicit refresh rotation', async () => {
    const fetcher = vi.fn().mockResolvedValue(Response.json(pair()));
    const response = await handleRefresh(
      request('/api/auth/refresh', {
        csrf: 'csrf-token',
        method: 'POST',
        refresh: 'refresh-old',
      }),
      fetcher,
    );

    const payload = await response.json();
    expect(payload).toEqual({
      expires_at: expect.any(String),
      user: { id: 'user-1', email: 'user@example.test' },
    });
    expect(payload).not.toHaveProperty('access_token');
    expect(payload).not.toHaveProperty('refresh_token');
    expect(response.headers.get('set-cookie')).toContain(
      `${REFRESH_TOKEN_COOKIE}=refresh-new`,
    );
  });

  it('preserves newly rotated cookies when the retried API call is unavailable', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(Response.json(pair()))
      .mockRejectedValueOnce(new Error('downstream unavailable'));

    const response = await proxyAuthenticatedRequest(
      request('/api/bff/risks', { refresh: 'refresh-before-network-error' }),
      ['risks'],
      fetcher,
    );

    expect(response.status).toBe(502);
    const setCookie = response.headers.get('set-cookie') ?? '';
    expect(setCookie).toContain(`${ACCESS_TOKEN_COOKIE}=access-new`);
    expect(setCookie).toContain(`${REFRESH_TOKEN_COOKIE}=refresh-new`);
  });

  it('coalesces concurrent exchanges for a rotating refresh credential', async () => {
    let release!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => {
      release = resolve;
    });
    const fetcher = vi.fn().mockReturnValue(pending);

    const first = refreshSession('unique-concurrent-refresh', fetcher);
    const second = refreshSession('unique-concurrent-refresh', fetcher);
    expect(fetcher).toHaveBeenCalledTimes(1);

    release(Response.json(pair()));
    await expect(first).resolves.toMatchObject({ kind: 'success' });
    await expect(second).resolves.toMatchObject({ kind: 'success' });
  });

  it('reuses a completed rotation briefly for late concurrent requests', async () => {
    const fetcher = vi.fn().mockResolvedValue(Response.json(pair()));

    await expect(refreshSession('unique-late-refresh', fetcher)).resolves.toMatchObject({
      kind: 'success',
    });
    await expect(refreshSession('unique-late-refresh', fetcher)).resolves.toMatchObject({
      kind: 'success',
    });

    expect(fetcher).toHaveBeenCalledOnce();
  });

  it('clears every session cookie after terminal refresh rejection', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(Response.json({ message: 'expired' }, { status: 401 }))
      .mockResolvedValueOnce(Response.json({ message: 'invalid' }, { status: 401 }));

    const response = await proxyAuthenticatedRequest(
      request('/api/bff/risks', {
        access: 'access-old',
        refresh: 'refresh-terminal',
      }),
      ['risks'],
      fetcher,
    );

    expect(response.status).toBe(401);
    expect(response.cookies.getAll().map(({ name }) => name)).toEqual([
      ACCESS_TOKEN_COOKIE,
      REFRESH_TOKEN_COOKIE,
      CSRF_TOKEN_COOKIE,
    ]);
    expect(response.headers.get('set-cookie')).toMatch(/Max-Age=0/i);
  });

  it('revokes server-side and clears cookies on logout', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    const response = await handleLogout(
      request('/api/auth/logout', {
        access: 'access-to-revoke',
        refresh: 'refresh-to-revoke',
        csrf: 'csrf-token',
        method: 'POST',
      }),
      fetcher,
    );

    expect(response.status).toBe(204);
    expect(fetcher).toHaveBeenCalledOnce();
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get('authorization')).toBe(
      'Bearer access-to-revoke',
    );
    expect(response.headers.get('set-cookie')).toMatch(/Max-Age=0/i);
  });
});
