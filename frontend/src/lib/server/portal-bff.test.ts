import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  CSRF_TOKEN_COOKIE,
  CSRF_TOKEN_HEADER,
  VENDOR_PORTAL_TOKEN_COOKIE,
} from '@/lib/auth-constants';
import { NextRequest } from 'next/server';
import { proxyPortalRequest } from '@/lib/server/portal-bff';

const origin = 'https://app.example.test';

function portalRequest(
  path: string,
  method = 'GET',
  body?: string,
  portalToken?: string,
) {
  const unsafe = method !== 'GET';
  const cookies = [
    unsafe && `${CSRF_TOKEN_COOKIE}=csrf-token`,
    portalToken && `${VENDOR_PORTAL_TOKEN_COOKIE}=${portalToken}`,
  ].filter(Boolean);
  return new NextRequest(`${origin}${path}`, {
    method,
    body,
    headers:
      unsafe || cookies.length
        ? {
          cookie: cookies.join('; '),
          host: 'app.example.test',
          ...(unsafe
            ? {
                origin,
                'sec-fetch-site': 'same-origin',
                [CSRF_TOKEN_HEADER]: 'csrf-token',
                'content-type': 'application/json',
              }
            : {}),
        }
      : undefined,
  });
}

describe('public portal BFF allowlist', () => {
  beforeEach(() => {
    process.env.API_INTERNAL_URL = 'http://api.internal.test/api/v1';
  });

  it('exchanges a vendor invitation body for scoped HttpOnly state without bearer auth', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      Response.json({
        assessment_id: 'assessment-1',
        title: 'Security review',
        vendor_name: 'Example Vendor',
        sections: [],
      }),
    );
    const response = await proxyPortalRequest(
      portalRequest(
        '/api/portal/vendor-portal/session',
        'POST',
        JSON.stringify({ token: 'signed/token' }),
      ),
      ['vendor-portal', 'session'],
      fetcher,
    );

    expect(response.status).toBe(200);
    expect(fetcher.mock.calls[0][0]).toContain('/vendor-portal/signed%2Ftoken');
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).has('authorization')).toBe(false);
    expect(response.headers.get('set-cookie')).toContain(
      `${VENDOR_PORTAL_TOKEN_COOKIE}=signed%2Ftoken`,
    );
    expect(response.headers.get('set-cookie')).toContain('HttpOnly');
    expect(response.headers.get('set-cookie')).toContain('Secure');
    expect(response.headers.get('set-cookie')).toContain('SameSite=strict');
    expect(response.headers.get('set-cookie')).toContain('Path=/api/portal/vendor-portal');
    await expect(response.json()).resolves.toMatchObject({
      id: 'assessment-1',
      name: 'Security review',
    });
  });

  it('maps browser saves to the backend response contract', async () => {
    const fetcher = vi.fn().mockResolvedValue(Response.json({ message: 'saved' }));
    const response = await proxyPortalRequest(
      portalRequest(
        '/api/portal/vendor-portal/save',
        'POST',
        JSON.stringify({
          answers: {
            q1: { value: 'Yes' },
            q2: { value: ['A', 'B'] },
          },
        }),
        'invite',
      ),
      ['vendor-portal', 'save'],
      fetcher,
    );

    expect(response.status).toBe(200);
    expect(fetcher.mock.calls[0][1]?.method).toBe('PUT');
    expect(fetcher.mock.calls[0][1]?.body).toBe(
      JSON.stringify({
        responses: [
          { question_id: 'q1', answer: 'Yes' },
          { question_id: 'q2', answers: ['A', 'B'] },
        ],
      }),
    );
  });

  it('preserves the backend board overview counts without inventing posture scores', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      Response.json({
        member_name: 'Board Member',
        pending_decisions: 4,
        unread_reports: 2,
        upcoming_meetings: 3,
      }),
    );
    const response = await proxyPortalRequest(
      portalRequest(
        '/api/portal/board-portal/session',
        'POST',
        JSON.stringify({ token: 'invite' }),
      ),
      ['board-portal', 'session'],
      fetcher,
    );

    await expect(response.json()).resolves.toMatchObject({
      compliance_score: null,
      pending_decisions: [],
      pending_decisions_count: 4,
      unread_reports: 2,
      upcoming_meetings: 3,
    });
  });

  it('rejects unknown portal paths and unsafe requests without CSRF proof', async () => {
    const fetcher = vi.fn();
    const unknown = await proxyPortalRequest(
      portalRequest('/api/portal/arbitrary?token=invite'),
      ['arbitrary'],
      fetcher,
    );
    const missingCsrf = await proxyPortalRequest(
      new NextRequest(`${origin}/api/portal/vendor-portal/submit?token=invite`, { method: 'POST' }),
      ['vendor-portal', 'submit'],
      fetcher,
    );

    expect(unknown.status).toBe(404);
    expect(missingCsrf.status).toBe(403);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('does not accept a capability from a BFF query string after bootstrap', async () => {
    const fetcher = vi.fn();
    const response = await proxyPortalRequest(
      portalRequest('/api/portal/vendor-portal/questionnaire?token=leaked-token'),
      ['vendor-portal', 'questionnaire'],
      fetcher,
    );

    expect(response.status).toBe(401);
    expect(fetcher).not.toHaveBeenCalled();
  });
});
