import type { AttachmentResponse } from '../lib/attachment';
import { createHash } from 'node:crypto';
import { SUPPORT_BUNDLE_EXCLUSIONS } from '../lib/support-bundle-contract';
import type { SupportBundleManifest } from '../types/support-bundle';

export const SUPPORT_TEST_TENANT = '7a2423cd-bdeb-472f-a60a-1b6cfec87aa8';
export const SUPPORT_TEST_BUNDLE = '19bb8bdc-1073-48fe-978c-ce11f3f4e2dc';

export function fixtureSha256(data: Uint8Array): string {
  return createHash('sha256').update(data).digest('hex');
}

export function fixtureCrc32(data: Uint8Array): number {
  let value = 0xffffffff;
  for (const byte of data) {
    value ^= byte;
    for (let bit = 0; bit < 8; bit += 1) value = (value >>> 1) ^ (value & 1 ? 0xedb88320 : 0);
  }
  return (value ^ 0xffffffff) >>> 0;
}

export function supportBundleFixture(options: {
  organizationId?: string;
  healthOrganizationId?: string;
  manifestPatch?: Record<string, unknown>;
  memberName?: string;
  dataDescriptor?: boolean;
  extraField?: (name: string) => Uint8Array;
  creatorVersion?: number;
  externalAttributes?: number;
  localVersion?: number;
} = {}): {
  attachment: AttachmentResponse;
  bytes: Uint8Array<ArrayBuffer>;
  headers: Record<string, string>;
  manifest: SupportBundleManifest;
} {
  const organizationId = options.organizationId ?? SUPPORT_TEST_TENANT;
  const generatedAt = '2026-09-15T07:00:00Z';
  const encode = (value: unknown) => new TextEncoder().encode(JSON.stringify(value, null, 2) + '\n');
  const files = [
    { name: 'configuration.json', data: encode({ schema_version: 1, environment: 'test', service_version: '1.2.3' }) },
    { name: 'health.json', data: encode({ organization_id: options.healthOrganizationId ?? organizationId, generated_at: generatedAt, overall_status: 'healthy' }) },
    { name: 'README.txt', data: new TextEncoder().encode('Reviewed test support scope. Hashes are not signatures.\n') },
  ];
  const manifest = {
    schema_version: 1, bundle_id: SUPPORT_TEST_BUNDLE, organization_id: organizationId,
    generated_at: generatedAt, consent_recorded_at: generatedAt, scope: 'health_and_posture',
    redaction_profile: 'health_posture_allowlist_v1', automatically_transmitted: false,
    configuration_fingerprint: fixtureSha256(files[0].data),
    files: files.map((file) => ({ name: file.name, bytes: file.data.length, sha256: fixtureSha256(file.data) })),
    excluded: [...SUPPORT_BUNDLE_EXCLUSIONS], ...options.manifestPatch,
  } as SupportBundleManifest;
  files.push({ name: options.memberName ?? 'manifest.json', data: encode(manifest) });
  const descriptor = options.dataDescriptor !== false;
  const localChunks: Uint8Array[] = [];
  const centralChunks: Uint8Array[] = [];
  let offset = 0;
  for (const file of files) {
    const name = new TextEncoder().encode(file.name);
    const timestamp = new Uint8Array(9);
    const timestampView = new DataView(timestamp.buffer);
    timestampView.setUint16(0, 0x5455, true); timestampView.setUint16(2, 5, true);
    timestamp[4] = 1; timestampView.setUint32(5, 315532800, true);
    const extra = options.extraField?.(file.name) ?? timestamp;
    const checksum = fixtureCrc32(file.data);
    const dataStart = 30 + name.length + extra.length;
    const local = new Uint8Array(dataStart + file.data.length + (descriptor ? 16 : 0));
    const lv = new DataView(local.buffer);
    lv.setUint32(0, 0x04034b50, true); lv.setUint16(4, options.localVersion ?? 20, true);
    lv.setUint16(6, descriptor ? 8 : 0, true); lv.setUint16(12, 33, true);
    if (!descriptor) {
      lv.setUint32(14, checksum, true);
      lv.setUint32(18, file.data.length, true); lv.setUint32(22, file.data.length, true);
    }
    lv.setUint16(26, name.length, true); lv.setUint16(28, extra.length, true);
    local.set(name, 30); local.set(extra, 30 + name.length); local.set(file.data, dataStart);
    if (descriptor) {
      const position = dataStart + file.data.length;
      lv.setUint32(position, 0x08074b50, true); lv.setUint32(position + 4, checksum, true);
      lv.setUint32(position + 8, file.data.length, true); lv.setUint32(position + 12, file.data.length, true);
    }
    localChunks.push(local);
    const central = new Uint8Array(46 + name.length + extra.length);
    const cv = new DataView(central.buffer);
    cv.setUint32(0, 0x02014b50, true); cv.setUint16(4, options.creatorVersion ?? 20, true); cv.setUint16(6, 20, true);
    cv.setUint16(8, descriptor ? 8 : 0, true); cv.setUint16(14, 33, true);
    cv.setUint32(16, checksum, true); cv.setUint32(20, file.data.length, true); cv.setUint32(24, file.data.length, true);
    cv.setUint16(28, name.length, true); cv.setUint16(30, extra.length, true);
    cv.setUint32(38, options.externalAttributes ?? 0, true); cv.setUint32(42, offset, true);
    central.set(name, 46); central.set(extra, 46 + name.length);
    centralChunks.push(central);
    offset += local.length;
  }
  const centralBytes = centralChunks.reduce((total, chunk) => total + chunk.length, 0);
  const end = new Uint8Array(22);
  const ev = new DataView(end.buffer);
  ev.setUint32(0, 0x06054b50, true); ev.setUint16(8, 4, true); ev.setUint16(10, 4, true);
  ev.setUint32(12, centralBytes, true); ev.setUint32(16, offset, true);
  const bytes = new Uint8Array(offset + centralBytes + end.length);
  let position = 0;
  for (const chunk of [...localChunks, ...centralChunks, end]) { bytes.set(chunk, position); position += chunk.length; }
  const headers = {
    'content-type': 'application/octet-stream',
    'content-disposition': `attachment; filename=complianceforge-support-${SUPPORT_TEST_BUNDLE}.zip`,
    'x-support-bundle-sha256': fixtureSha256(bytes),
  };
  return {
    bytes, headers, manifest,
    attachment: {
      blob: new Blob([bytes.buffer], { type: headers['content-type'] }),
      contentType: headers['content-type'], contentDisposition: headers['content-disposition'],
      supportBundleSha256: headers['x-support-bundle-sha256'],
    },
  };
}
