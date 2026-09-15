export interface AttachmentResponse {
  blob: Blob;
  contentType: string;
  contentDisposition: string | null;
  supportBundleSha256: string | null;
}

export class AttachmentValidationError extends Error {
  constructor() {
    super('The attachment could not be safely verified. No download was prepared.');
    this.name = 'AttachmentValidationError';
  }
}

/** Bounded, selected attachment metadata only; never expose a raw Headers object. */
export async function readBoundedAttachment(
  response: Response,
  maximumBytes: number,
  signal?: AbortSignal,
): Promise<AttachmentResponse> {
  const headerLength = response.headers.get('content-length');
  const declared = Number(headerLength);
  if (!Number.isSafeInteger(maximumBytes) || maximumBytes < 1 ||
    (headerLength !== null && (!/^(?:0|[1-9]\d*)$/.test(headerLength) || !Number.isSafeInteger(declared))) ||
    declared > maximumBytes) {
    await response.body?.cancel().catch(() => undefined);
    throw new AttachmentValidationError();
  }
  const chunks: ArrayBuffer[] = [];
  let length = 0;
  const reader = response.body?.getReader();
  if (!reader) throw new AttachmentValidationError();
  const onAbort = () => { void reader.cancel().catch(() => undefined); };
  signal?.addEventListener('abort', onAbort, { once: true });
  try {
    while (true) {
      signal?.throwIfAborted();
      const { done, value } = await reader.read();
      signal?.throwIfAborted();
      if (done) break;
      length += value.byteLength;
      if (length > maximumBytes) {
        await reader.cancel().catch(() => undefined);
        throw new AttachmentValidationError();
      }
      chunks.push(new Uint8Array(value).buffer);
    }
  } catch (error) {
    await reader.cancel().catch(() => undefined);
    throw error;
  } finally {
    signal?.removeEventListener('abort', onAbort);
    reader.releaseLock();
  }
  const contentType = response.headers.get('content-type') ?? '';
  return {
    blob: new Blob(chunks, { type: contentType }),
    contentType,
    contentDisposition: response.headers.get('content-disposition'),
    supportBundleSha256: response.headers.get('x-support-bundle-sha256'),
  };
}

/** Keep only machine-readable access/reconfirmation metadata, never driver detail. */
export async function attachmentErrorMetadata(attachment: AttachmentResponse): Promise<Record<string, string>> {
  if (!attachment.contentType.toLowerCase().includes('application/json') || attachment.blob.size > 8 * 1024) return {};
  try {
    const value: unknown = JSON.parse(await attachment.blob.text());
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
    const source = value as Record<string, unknown>;
    const selected: Record<string, string> = {};
    for (const key of ['error_code', 'code']) {
      if (typeof source[key] === 'string' && /^[A-Z][A-Z0-9_]{0,79}$/.test(source[key])) selected[key] = source[key];
    }
    if (typeof source.request_id === 'string' && /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/.test(source.request_id)) {
      selected.request_id = source.request_id;
    }
    return selected;
  } catch {
    return {};
  }
}
