import { ACCESS_TOKEN_COOKIE, CSRF_TOKEN_COOKIE, CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NextRequest } from 'next/server';
import { proxyAuthenticatedRequest } from '@/lib/server/session-bff';
import { proxyResponse } from '@/lib/server/upstream';

const origin = 'https://app.example.test';

describe('evidence BFF transport', () => {
  beforeEach(() => {
    process.env.API_INTERNAL_URL = 'http://api.internal.test/api/v1';
  });

  it('preserves a multipart file body and boundary while keeping bearer credentials server-side', async () => {
    const body = new FormData();
    body.append('file', new File(['%PDF-evidence'], 'review.pdf', { type: 'application/pdf' }));
    body.append('title', 'Access review');
    body.append('evidence_type', 'document');
    const request = new NextRequest(`${origin}/api/bff/controls/control-1/evidence`, {
      method: 'POST',
      body,
      headers: {
        cookie: `${ACCESS_TOKEN_COOKIE}=server-access; ${CSRF_TOKEN_COOKIE}=csrf-evidence`,
        host: 'app.example.test',
        origin,
        'sec-fetch-site': 'same-origin',
        [CSRF_TOKEN_HEADER]: 'csrf-evidence',
      },
    });
    const fetcher = vi.fn().mockResolvedValue(Response.json({ id: 'evidence-1' }, { status: 201 }));

    const response = await proxyAuthenticatedRequest(
      request,
      ['controls', 'control-1', 'evidence'],
      fetcher,
    );

    expect(response.status).toBe(201);
    const upstream = fetcher.mock.calls[0];
    expect(upstream[0]).toBe('http://api.internal.test/api/v1/controls/control-1/evidence');
    const headers = new Headers(upstream[1]?.headers);
    expect(headers.get('authorization')).toBe('Bearer server-access');
    expect(headers.get('content-type')).toMatch(/^multipart\/form-data; boundary=/);
    expect(headers.has('cookie')).toBe(false);
    const serialized = new TextDecoder().decode(upstream[1]?.body as ArrayBuffer);
    expect(serialized).toContain('name="file"; filename="review.pdf"');
    expect(serialized).toContain('name="title"');
    expect(serialized).not.toContain('server-access');
  });

  it('preserves one replacement file and version reason through the authenticated BFF', async () => {
    const body = new FormData();
    body.append('file', new File(['%PDF-v2'], 'review-v2.pdf', { type: 'application/pdf' }));
    body.append('title', 'Access review');
    body.append('evidence_type', 'document');
    body.append('version_reason', 'Quarterly refresh');
    const request = new NextRequest(
      `${origin}/api/bff/controls/control-1/evidence/evidence-1/supersede`,
      {
        method: 'POST',
        body,
        headers: {
          cookie: `${ACCESS_TOKEN_COOKIE}=server-access; ${CSRF_TOKEN_COOKIE}=csrf-evidence`,
          host: 'app.example.test',
          origin,
          'sec-fetch-site': 'same-origin',
          [CSRF_TOKEN_HEADER]: 'csrf-evidence',
        },
      },
    );
    const fetcher = vi.fn().mockResolvedValue(Response.json({ id: 'evidence-2' }, { status: 201 }));

    const response = await proxyAuthenticatedRequest(
      request,
      ['controls', 'control-1', 'evidence', 'evidence-1', 'supersede'],
      fetcher,
    );

    expect(response.status).toBe(201);
    expect(fetcher.mock.calls[0][0]).toBe(
      'http://api.internal.test/api/v1/controls/control-1/evidence/evidence-1/supersede',
    );
    const serialized = new TextDecoder().decode(fetcher.mock.calls[0][1]?.body as ArrayBuffer);
    expect(serialized.match(/name="file"/g)).toHaveLength(1);
    expect(serialized.match(/name="version_reason"/g)).toHaveLength(1);
    expect(serialized).toContain('Quarterly refresh');
    expect(serialized).not.toContain('server-access');
  });

  it('forwards a canonical signed 307 with no-store/no-referrer and no cookies', () => {
    const signed = 'https://objects.example.test/private/report.pdf?signature=short-lived';
    const response = proxyResponse(
      new Response(null, {
        status: 307,
        headers: { Location: signed, 'Referrer-Policy': 'no-referrer', 'Set-Cookie': 'leak=1' },
      }),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get('location')).toBe(signed);
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(response.headers.get('referrer-policy')).toBe('no-referrer');
    expect(response.headers.has('set-cookie')).toBe(false);
  });

  it.each([
    'javascript:alert(1)',
    'http://objects.example.test/private/file?signature=leaked-in-transit',
    'https://user:password@objects.example.test/file',
    'https://objects.example.test/file#secret',
    '/relative/object',
  ])('fails closed on unsafe upstream download redirect %s', async (location) => {
    const response = proxyResponse(
      new Response(null, { status: 307, headers: { Location: location } }),
    );

    expect(response.status).toBe(502);
    expect(response.headers.has('location')).toBe(false);
    await expect(response.json()).resolves.toMatchObject({
      error_code: 'INVALID_UPSTREAM_REDIRECT',
    });
  });

  it('permits loopback HTTP object storage only outside production', async () => {
    const local = 'http://localhost:9000/evidence/file?signature=development';
    expect(proxyResponse(new Response(null, { status: 307, headers: { Location: local } })).status)
      .toBe(307);

    vi.stubEnv('NODE_ENV', 'production');
    const productionResponse = proxyResponse(
      new Response(null, { status: 307, headers: { Location: local } }),
    );
    vi.unstubAllEnvs();

    expect(productionResponse.status).toBe(502);
    expect(productionResponse.headers.has('location')).toBe(false);
  });
});
