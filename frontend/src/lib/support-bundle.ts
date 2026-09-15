import { type AttachmentResponse, AttachmentValidationError } from './attachment';
import {
  SUPPORT_BUNDLE_EXCLUSIONS,
  SUPPORT_BUNDLE_MAX_BYTES,
  SUPPORT_BUNDLE_MEMBERS,
  SUPPORT_BUNDLE_REDACTION_PROFILE,
  SUPPORT_BUNDLE_SCOPE,
} from './support-bundle-contract';
import type { SupportBundleFile, SupportBundleManifest, VerifiedSupportBundle } from '@/types/support-bundle';

const SHA256 = /^[0-9a-f]{64}$/;
const UUID = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/;
const NIL_UUID = '00000000-0000-0000-0000-000000000000';
const decoder = new TextDecoder('utf-8', { fatal: true });

function requireValid(condition: unknown): asserts condition {
  if (!condition) throw new AttachmentValidationError();
}

export function isCanonicalSupportUuid(value: string): boolean {
  return UUID.test(value) && value !== NIL_UUID;
}

export function supportBundleFilename(disposition: string | null): { filename: string; id: string } {
  const match = disposition?.match(/^attachment;\s*filename=(?:"([^";]+)"|([^\s;]+))$/);
  const filename = match?.[1] ?? match?.[2] ?? '';
  const id = filename.match(/^complianceforge-support-(.+)\.zip$/)?.[1] ?? '';
  requireValid(isCanonicalSupportUuid(id));
  return { filename, id };
}

export async function supportSha256(data: Uint8Array | ArrayBuffer): Promise<string> {
  const sum = await crypto.subtle.digest('SHA-256', new Uint8Array(data).buffer);
  return Array.from(new Uint8Array(sum), (value) => value.toString(16).padStart(2, '0')).join('');
}

function crc32(data: Uint8Array): number {
  let value = 0xffffffff;
  for (const byte of data) {
    value ^= byte;
    for (let bit = 0; bit < 8; bit += 1) {
      value = (value >>> 1) ^ (value & 1 ? 0xedb88320 : 0);
    }
  }
  return (value ^ 0xffffffff) >>> 0;
}

function reviewedExtra(bytes: Uint8Array) {
  requireValid(bytes.length === 9);
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  // Go's reviewed writer adds only the fixed 1980 UTC modification timestamp.
  requireValid(view.getUint16(0, true) === 0x5455 && view.getUint16(2, true) === 5);
  requireValid(bytes[4] === 1 && view.getUint32(5, true) === 315532800);
}

/**
 * Deliberately not a general ZIP reader: four bounded ZIPStore entries only,
 * with no ZIP64, encryption, compression, comments, paths, or trailing bytes.
 * Only Go's reviewed fixed extended timestamp field is permitted.
 */
function storedMembers(bytes: Uint8Array): Map<string, Uint8Array> {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const end = bytes.length - 22;
  requireValid(end >= 0 && view.getUint32(end, true) === 0x06054b50);
  requireValid(view.getUint16(end + 4, true) === 0 && view.getUint16(end + 6, true) === 0);
  requireValid(view.getUint16(end + 8, true) === 4 && view.getUint16(end + 10, true) === 4);
  requireValid(view.getUint16(end + 20, true) === 0);
  const centralBytes = view.getUint32(end + 12, true);
  const centralStart = view.getUint32(end + 16, true);
  requireValid(centralStart + centralBytes === end);

  const files = new Map<string, Uint8Array>();
  const spans: { start: number; end: number }[] = [];
  let position = centralStart;
  for (let index = 0; index < 4; index += 1) {
    requireValid(position + 46 <= end && view.getUint32(position, true) === 0x02014b50);
    const flags = view.getUint16(position + 8, true);
    const method = view.getUint16(position + 10, true);
    const checksum = view.getUint32(position + 16, true);
    const size = view.getUint32(position + 24, true);
    const nameBytes = view.getUint16(position + 28, true);
    const extraBytes = view.getUint16(position + 30, true);
    const commentBytes = view.getUint16(position + 32, true);
    const local = view.getUint32(position + 42, true);
    requireValid(view.getUint16(position + 4, true) === 20 && view.getUint16(position + 6, true) === 20);
    requireValid((flags & ~0x808) === 0);
    requireValid(view.getUint16(position + 12, true) === 0 && view.getUint16(position + 14, true) === 33);
    requireValid(view.getUint16(position + 36, true) === 0 && view.getUint32(position + 38, true) === 0);
    requireValid(method === 0 && size === view.getUint32(position + 20, true));
    requireValid(size <= SUPPORT_BUNDLE_MAX_BYTES && nameBytes > 0 && nameBytes <= 32);
    requireValid(commentBytes === 0 && view.getUint16(position + 34, true) === 0);
    const next = position + 46 + nameBytes + extraBytes;
    requireValid(next <= end);
    const name = decoder.decode(bytes.subarray(position + 46, position + 46 + nameBytes));
    requireValid(SUPPORT_BUNDLE_MEMBERS.some((member) => member === name) && !files.has(name));
    const centralExtra = bytes.subarray(position + 46 + nameBytes, next);
    reviewedExtra(centralExtra);

    requireValid(local + 30 <= centralStart && view.getUint32(local, true) === 0x04034b50);
    requireValid(view.getUint16(local + 4, true) === 20);
    requireValid(view.getUint16(local + 6, true) === flags && view.getUint16(local + 8, true) === 0);
    requireValid(view.getUint16(local + 10, true) === 0 && view.getUint16(local + 12, true) === 33);
    const localNameBytes = view.getUint16(local + 26, true);
    const localExtraBytes = view.getUint16(local + 28, true);
    const dataStart = local + 30 + localNameBytes + localExtraBytes;
    const dataEnd = dataStart + size;
    requireValid(localNameBytes === nameBytes && dataEnd <= centralStart);
    requireValid(decoder.decode(bytes.subarray(local + 30, local + 30 + nameBytes)) === name);
    const localExtra = bytes.subarray(local + 30 + nameBytes, dataStart);
    reviewedExtra(localExtra);
    requireValid(localExtra.length === centralExtra.length && localExtra.every((value, index) => value === centralExtra[index]));
    const data = bytes.subarray(dataStart, dataEnd);
    requireValid(crc32(data) === checksum);
    let spanEnd = dataEnd;
    if (flags & 8) {
      requireValid(spanEnd + 12 <= centralStart);
      if (view.getUint32(spanEnd, true) === 0x08074b50) spanEnd += 4;
      requireValid(spanEnd + 12 <= centralStart);
      requireValid(view.getUint32(spanEnd, true) === checksum);
      requireValid(view.getUint32(spanEnd + 4, true) === size && view.getUint32(spanEnd + 8, true) === size);
      spanEnd += 12;
    } else {
      requireValid(view.getUint32(local + 14, true) === checksum);
      requireValid(view.getUint32(local + 18, true) === size && view.getUint32(local + 22, true) === size);
    }
    spans.push({ start: local, end: spanEnd });
    files.set(name, data);
    position = next;
  }
  requireValid(position === end && files.size === 4);
  spans.sort((a, b) => a.start - b.start);
  let previousEnd = 0;
  for (const span of spans) {
    requireValid(span.start === previousEnd);
    previousEnd = span.end;
  }
  requireValid(previousEnd === centralStart);
  return files;
}

function object(value: unknown): Record<string, unknown> {
  requireValid(value !== null && typeof value === 'object' && !Array.isArray(value));
  return value as Record<string, unknown>;
}

function exactKeys(value: Record<string, unknown>, keys: readonly string[]) {
  const actual = Object.keys(value);
  requireValid(actual.length === keys.length && actual.every((key) => keys.includes(key)));
}

function json(bytes: Uint8Array | undefined): Record<string, unknown> {
  requireValid(bytes);
  return object(JSON.parse(decoder.decode(bytes)));
}

function timestamp(value: unknown): value is string {
  return typeof value === 'string' && value.length <= 64 &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) &&
    Number.isFinite(Date.parse(value));
}

export async function verifySupportBundle(
  attachment: AttachmentResponse,
  organizationId: string,
  signal?: AbortSignal,
): Promise<VerifiedSupportBundle> {
  try {
    signal?.throwIfAborted();
    requireValid(isCanonicalSupportUuid(organizationId));
    requireValid(attachment.contentType.toLowerCase().trim() === 'application/octet-stream');
    requireValid(attachment.blob.size > 0 && attachment.blob.size <= SUPPORT_BUNDLE_MAX_BYTES);
    requireValid(attachment.supportBundleSha256 && SHA256.test(attachment.supportBundleSha256));
    const { filename, id } = supportBundleFilename(attachment.contentDisposition);
    const archive = new Uint8Array(await attachment.blob.arrayBuffer());
    requireValid(await supportSha256(archive) === attachment.supportBundleSha256);
    signal?.throwIfAborted();
    const members = storedMembers(archive);
    const value = json(members.get('manifest.json'));
    exactKeys(value, [
      'schema_version', 'bundle_id', 'organization_id', 'generated_at', 'scope',
      'consent_recorded_at', 'redaction_profile', 'configuration_fingerprint',
      'automatically_transmitted', 'files', 'excluded',
    ]);
    requireValid(value.schema_version === 1 && value.bundle_id === id && value.organization_id === organizationId);
    requireValid(value.scope === SUPPORT_BUNDLE_SCOPE && value.redaction_profile === SUPPORT_BUNDLE_REDACTION_PROFILE);
    requireValid(value.automatically_transmitted === false && timestamp(value.generated_at));
    requireValid(value.consent_recorded_at === value.generated_at);
    requireValid(Array.isArray(value.excluded) && value.excluded.every((entry) => typeof entry === 'string'));
    const excluded = value.excluded;
    requireValid(excluded.length === SUPPORT_BUNDLE_EXCLUSIONS.length);
    requireValid(SUPPORT_BUNDLE_EXCLUSIONS.every((entry) => excluded.includes(entry)));
    requireValid(Array.isArray(value.files) && value.files.length === 3);
    const descriptors: SupportBundleFile[] = [];
    const seen = new Set<string>();
    for (const entry of value.files) {
      const file = object(entry);
      exactKeys(file, ['name', 'bytes', 'sha256']);
      requireValid(typeof file.name === 'string' && file.name !== 'manifest.json' && !seen.has(file.name));
      const bytes = members.get(file.name);
      requireValid(bytes && Number.isSafeInteger(file.bytes) && file.bytes === bytes.length);
      requireValid(typeof file.sha256 === 'string' && SHA256.test(file.sha256));
      requireValid(await supportSha256(bytes) === file.sha256);
      seen.add(file.name);
      descriptors.push(file as unknown as SupportBundleFile);
    }
    requireValid(value.configuration_fingerprint === descriptors.find((file) => file.name === 'configuration.json')?.sha256);
    requireValid(json(members.get('health.json')).organization_id === organizationId);
    signal?.throwIfAborted();
    const manifest = { ...value, files: descriptors } as unknown as SupportBundleManifest;
    return { blob: attachment.blob, filename, sha256: attachment.supportBundleSha256, manifest };
  } catch (error) {
    if (signal?.aborted) throw signal.reason;
    if (error instanceof AttachmentValidationError) throw error;
    throw new AttachmentValidationError();
  }
}

/** Never render arbitrary server error text, endpoints, or configuration detail. */
export function supportBundleError(error: unknown): string {
  if (error instanceof AttachmentValidationError) return error.message;
  const candidate = error && typeof error === 'object' ? error as { status?: number; detail?: unknown } : {};
  const detail = candidate.detail && typeof candidate.detail === 'object' ?
    candidate.detail as { error_code?: string; code?: string } : {};
  if ((detail.error_code ?? detail.code) === 'CONSENT_RECONFIRMATION_REQUIRED') {
    return 'Your session was renewed without replaying generation. Review the preview and consent again.';
  }
  if (candidate.status === 401) return 'Your session ended. Sign in before reviewing and consenting again.';
  if (candidate.status === 403) return 'Generation is not permitted by your current access policy. Contact your organization administrator.';
  if (candidate.status === 413) return 'The support bundle exceeded a reviewed size limit. No download was prepared.';
  if (candidate.status === 429) return 'Too many requests. Wait before reviewing and explicitly consenting again.';
  return 'The support bundle is temporarily unavailable. No automatic retry occurred. Review and consent again to retry.';
}
