import { afterEach, describe, expect, expectTypeOf, it, vi } from 'vitest';

import api from '@/lib/api';
import type { DataEnvelope } from '@/types/enterprise-settings';
import type { DiagnosticsSnapshot } from '@/types/diagnostics';

function response(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('administrator diagnostics API contract', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('uses the canonical secure-BFF route and forwards cancellation', async () => {
    const controller = new AbortController();
    const fetchMock = vi.fn().mockResolvedValue(response({ data: {} }));
    vi.stubGlobal('fetch', fetchMock);

    expectTypeOf(api.diagnostics.snapshot).returns.toEqualTypeOf<
      Promise<DataEnvelope<DiagnosticsSnapshot>>
    >();
    await api.diagnostics.snapshot(controller.signal);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/bff/settings/diagnostics');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      credentials: 'same-origin',
      method: 'GET',
      signal: controller.signal,
    });
    expect(new Headers(fetchMock.mock.calls[0][1]?.headers).has('authorization')).toBe(false);
  });

  it('does not retry an aborted diagnostic request', async () => {
    const controller = new AbortController();
    controller.abort();
    const abortError = new DOMException('Request was cancelled', 'AbortError');
    const fetchMock = vi.fn().mockRejectedValue(abortError);
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.diagnostics.snapshot(controller.signal)).rejects.toBe(abortError);
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('cancels transport backoff without issuing another diagnostic request', async () => {
    const controller = new AbortController();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ message: 'Unavailable' }), {
        status: 503,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const pending = api.diagnostics.snapshot(controller.signal);
    controller.abort();

    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
