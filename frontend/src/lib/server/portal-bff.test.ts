import { NextRequest } from 'next/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { CSRF_TOKEN_COOKIE, CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { proxyPortalRequest } from '@/lib/server/portal-bff';

const origin = 'https://app.example.test';

function portalRequest(path: string, method = 'GET', body?: string) {
  const unsafe = method !== 'GET';
  return new NextRequest(`${origin}${path}`, {
    method,
    body,
    headers: unsafe
      ? {
          cookie: `${CSRF_TOKEN_COOKIE}=csrf-token`,
          host: 'app.example.test',
          origin,
          'sec-fetch-site': 'same-origin',
          [CSRF_TOKEN_HEADER]: 'csrf-token',
          'content-type': 'application/json',
        }
      : undefined,
  });
}

describe('public portal BFF allowlist', () => {
  beforeEach(() => {
    process.env.API_INTERNAL_URL = 'http://api.internal.test/api/v1';
  });

  it('maps a vendor invitation query onto the backend token path without bearer auth', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      Response.json({
        assessment_id: 'assessment-1',
        title: 'Security review',
        vendor_name: 'Example Vendor',
        sections: [],
      }),
    );
    const response = await proxyPortalRequest(
      portalRequest('/api/portal/vendor-portal/questionnaire?token=signed%2Ftoken'),
      ['vendor-portal', 'questionnaire'],
      fetcher,
    );

    expect(response.status).toBe(200);
    expect(fetcher.mock.calls[0][0]).toContain('/vendor-portal/signed%2Ftoken');
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).has('authorization')).toBe(false);
    await expect(response.json()).resolves.toMatchObject({
      id: 'assessment-1',
      name: 'Security review',
    });
  });

  it('maps browser saves to the backend response contract', async () => {
    const fetcher = vi.fn().mockResolvedValue(Response.json({ message: 'saved' }));
    const response = await proxyPortalRequest(
      portalRequest(
        '/api/portal/vendor-portal/save?token=invite',
        'POST',
        JSON.stringify({
          answers: {
            q1: { value: 'Yes' },
            q2: { value: ['A', 'B'] },
          },
        }),
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
      portalRequest('/api/portal/board-portal?token=invite'),
      ['board-portal'],
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
});
