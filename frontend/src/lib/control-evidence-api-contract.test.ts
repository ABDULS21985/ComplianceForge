import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import { resetCsrfToken } from '@/lib/csrf-client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const evidence = {
  id: 'evidence-1',
  organization_id: 'organization-1',
  control_implementation_id: 'implementation-1',
  title: 'Access review',
  evidence_type: 'document' as const,
  collection_method: 'manual_upload' as const,
  collected_at: '2026-09-14T00:00:00Z',
  is_current: true,
  review_status: 'pending' as const,
  metadata: {},
  series_id: 'series-1',
  version_number: 1,
  lifecycle_status: 'active' as const,
  version_reason: 'Initial evidence upload',
  content_fingerprint: 'b'.repeat(64),
  created_at: '2026-09-14T00:00:00Z',
  updated_at: '2026-09-14T00:00:00Z',
};

const lifecycle = {
  evidence,
  versions: [evidence],
  reviews: [],
  custody_events: [],
  chain: {
    evidence_id: evidence.id,
    valid: true,
    event_count: 1,
    head_sequence: 1,
    head_hash: 'c'.repeat(64),
  },
  legal_hold_active: false,
};

describe('control evidence API contracts', () => {
  const fetchMock = vi.fn<(input: RequestInfo | URL, request?: RequestInit) => Promise<Response>>();

  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-evidence' });
      if (url.endsWith('/download')) {
        return new Response(new Uint8Array([1, 2, 3]), {
          headers: {
            'Content-Disposition': 'attachment; filename="evidence.json"',
            'Content-Type': 'application/json',
          },
        });
      }
      if (url.endsWith('/history')) return jsonResponse(lifecycle);
      if (url.endsWith('/verify-integrity')) {
        return jsonResponse({
          evidence_id: evidence.id,
          valid: true,
          sha256: 'a'.repeat(64),
          size_bytes: 3,
          verified_at: '2026-09-14T12:00:00Z',
        });
      }
      if (url.endsWith('/supersede')) {
        return jsonResponse(
          {
            ...evidence,
            id: 'evidence-2',
            version_number: 2,
            supersedes_evidence_id: evidence.id,
            version_reason: 'Quarterly refresh',
          },
          201,
        );
      }
      if (url.includes('?page=')) {
        return jsonResponse({
          data: [],
          pagination: { page: 2, page_size: 10, total_items: 0, total_pages: 0 },
        });
      }
      return jsonResponse(evidence, url.endsWith('/evidence') ? 201 : 200);
    });
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => vi.unstubAllGlobals());

  it('uses canonical control and paginated evidence routes without invented filters', async () => {
    await api.controls.list({ page: 2, page_size: 10 });
    await api.controls.get('control/one');
    const page = await api.controls.listEvidence('control/one', { page: 2, page_size: 10 });

    expect(page.pagination.page).toBe(2);
    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      '/api/bff/controls/?page=2&page_size=10',
      '/api/bff/controls/control%2Fone',
      '/api/bff/controls/control%2Fone/evidence?page=2&page_size=10',
    ]);
  });

  it('sends one canonical multipart file through CSRF-protected same-origin BFF', async () => {
    const controller = new AbortController();
    await api.controls.uploadEvidence(
      'control-1',
      {
        file: new File(['%PDF'], 'review.pdf', { type: 'application/pdf' }),
        title: 'Access review',
        evidence_type: 'document',
        metadata: { source: 'manual' },
      },
      controller.signal,
    );

    const [, request] =
      fetchMock.mock.calls.find(([url]) => String(url).endsWith('/evidence')) ?? [];
    const headers = new Headers(request?.headers);
    const body = request?.body as FormData;
    expect(headers.get(CSRF_TOKEN_HEADER)).toBe('csrf-evidence');
    expect(headers.has('authorization')).toBe(false);
    expect(headers.has('content-type')).toBe(false);
    expect(request?.credentials).toBe('same-origin');
    expect(request?.signal).toBe(controller.signal);
    expect(body.getAll('file')).toHaveLength(1);
    expect(body.getAll('title')).toEqual(['Access review']);
    expect([...body.keys()].sort()).toEqual(['evidence_type', 'file', 'metadata', 'title']);
  });

  it('uses the exact review JSON route and keeps JSON evidence downloads as private blobs', async () => {
    await api.controls.reviewEvidence('control-1', 'evidence/1', {
      status: 'rejected',
      comment: 'Incorrect period',
    });
    const downloaded = await api.controls.downloadEvidence('control-1', 'evidence/1');

    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [String(url), request?.method])).toEqual([
      ['/api/bff/controls/control-1/evidence/evidence%2F1/review', 'POST'],
      ['/api/bff/controls/control-1/evidence/evidence%2F1/download', 'GET'],
    ]);
    expect(JSON.parse(String(calls[0][1]?.body))).toEqual({
      status: 'rejected',
      comment: 'Incorrect period',
    });
    expect(downloaded).toBeInstanceOf(Blob);
    expect(downloaded.size).toBe(3);
  });

  it('uses exact lifecycle routes and sends one replacement file with a version reason', async () => {
    const historyController = new AbortController();
    const result = await api.controls.evidenceHistory(
      'control-1',
      'evidence/1',
      historyController.signal,
    );
    await api.controls.verifyEvidenceIntegrity('control-1', 'evidence/1');
    const replacementController = new AbortController();
    const replacement = await api.controls.supersedeEvidence(
      'control-1',
      'evidence/1',
      {
        file: new File(['%PDF-v2'], 'review-v2.pdf', { type: 'application/pdf' }),
        title: 'Access review',
        evidence_type: 'document',
        version_reason: 'Quarterly refresh',
      },
      replacementController.signal,
    );

    expect(result.chain.valid).toBe(true);
    expect(replacement.version_number).toBe(2);
    const lifecycleCalls = fetchMock.mock.calls.filter(([url]) =>
      /history|verify-integrity|supersede/.test(String(url)),
    );
    expect(lifecycleCalls.map(([url, request]) => [String(url), request?.method])).toEqual([
      ['/api/bff/controls/control-1/evidence/evidence%2F1/history', 'GET'],
      ['/api/bff/controls/control-1/evidence/evidence%2F1/verify-integrity', 'POST'],
      ['/api/bff/controls/control-1/evidence/evidence%2F1/supersede', 'POST'],
    ]);
    expect(lifecycleCalls[0][1]?.signal).toBe(historyController.signal);
    expect(lifecycleCalls[1][1]?.body).toBeUndefined();
    expect(new Headers(lifecycleCalls[1][1]?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-evidence');
    const supersedeBody = lifecycleCalls[2][1]?.body as FormData;
    expect(lifecycleCalls[2][1]?.signal).toBe(replacementController.signal);
    expect([...supersedeBody.keys()].sort()).toEqual([
      'evidence_type',
      'file',
      'title',
      'version_reason',
    ]);
    expect(supersedeBody.getAll('file')).toHaveLength(1);
    expect(supersedeBody.getAll('version_reason')).toEqual(['Quarterly refresh']);
    expect(new Headers(lifecycleCalls[2][1]?.headers).has('authorization')).toBe(false);
  });
});
