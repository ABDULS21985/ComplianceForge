import { AttachmentValidationError, readBoundedAttachment } from './attachment';
import { describe, expect, it } from 'vitest';
import { fixtureCrc32, fixtureSha256, SUPPORT_TEST_TENANT, supportBundleFixture } from '@/test/support-bundle-fixture';
import { supportBundleError, supportBundleFilename, verifySupportBundle } from './support-bundle';
import { SUPPORT_BUNDLE_MAX_BYTES } from './support-bundle-contract';

describe('bounded support ZIP verification', () => {
  it.each([true, false])('verifies ZIPStore structure and every member hash (descriptor=%s)', async (dataDescriptor) => {
    const fixture = supportBundleFixture({ dataDescriptor });
    const result = await verifySupportBundle(fixture.attachment, SUPPORT_TEST_TENANT);
    expect(result.manifest).toEqual(fixture.manifest);
    expect(result.sha256).toBe(fixture.headers['x-support-bundle-sha256']);
    expect(result.filename).toMatch(/^complianceforge-support-[0-9a-f-]+\.zip$/);
  });

  it.each([
    null, 'inline; filename=test.zip', 'attachment; filename="../report.zip"',
    'attachment; filename="complianceforge-support-00000000-0000-0000-0000-000000000000.zip"',
    'attachment; filename*=UTF-8\'\'complianceforge-support-19bb8bdc-1073-48fe-978c-ce11f3f4e2dc.zip',
    'attachment; filename=complianceforge-support-19BB8BDC-1073-48FE-978C-CE11F3F4E2DC.zip',
    'attachment; filename="complianceforge-support-19bb8bdc-1073-48fe-978c-ce11f3f4e2dc.zip"; extra=1',
  ])('rejects noncanonical download filenames: %s', (value) => {
    expect(() => supportBundleFilename(value)).toThrow(AttachmentValidationError);
  });

  it.each([
    { scope: 'raw_logs' }, { organization_id: 'a23fec3b-3544-49f3-99eb-af26b93eec07' },
    { bundle_id: 'a23fec3b-3544-49f3-99eb-af26b93eec07' }, { automatically_transmitted: true },
    { excluded: ['credentials'] }, { files: [] }, { configuration_fingerprint: '0'.repeat(64) },
    { secret_field: 'do-not-render' }, { consent_recorded_at: '2026-09-15T08:00:00Z' },
  ])('rejects a rehashed archive with unreviewed manifest metadata: %j', async (manifestPatch) => {
    const fixture = supportBundleFixture({ manifestPatch });
    await expect(verifySupportBundle(fixture.attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
  });

  it('rejects mismatched health tenant and paths even when outer archive hash is valid', async () => {
    const crossTenant = supportBundleFixture({ healthOrganizationId: 'a23fec3b-3544-49f3-99eb-af26b93eec07' });
    const path = supportBundleFixture({ memberName: '../manifest.json' });
    await expect(verifySupportBundle(crossTenant.attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
    await expect(verifySupportBundle(path.attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
  });

  it('allows only the reviewed fixed UTC timestamp extra; rejects Unicode path/extraction differentials', async () => {
    const timestamp = new Uint8Array(9);
    const tv = new DataView(timestamp.buffer);
    tv.setUint16(0, 0x5455, true); tv.setUint16(2, 5, true); timestamp[4] = 1;
    tv.setUint32(5, 315532800, true);
    await expect(verifySupportBundle(supportBundleFixture({ extraField: () => timestamp }).attachment, SUPPORT_TEST_TENANT)).resolves.toBeDefined();
    await expect(verifySupportBundle(supportBundleFixture({ extraField: () => new Uint8Array() }).attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
    const unicode = supportBundleFixture({ extraField: (name) => {
      const path = new TextEncoder().encode('../../../unreviewed.txt');
      const extra = new Uint8Array(9 + path.length);
      const view = new DataView(extra.buffer);
      view.setUint16(0, 0x7075, true); view.setUint16(2, 5 + path.length, true); extra[4] = 1;
      view.setUint32(5, fixtureCrc32(new TextEncoder().encode(name)), true); extra.set(path, 9);
      return extra;
    } });
    await expect(verifySupportBundle(unicode.attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
    const changedTime = timestamp.slice(); new DataView(changedTime.buffer).setUint32(5, 1, true);
    await expect(verifySupportBundle(supportBundleFixture({ extraField: () => changedTime }).attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
  });

  it.each([
    { creatorVersion: 0x0314, externalAttributes: 0xa1ff0000 },
    { externalAttributes: 0x10 }, { localVersion: 63 },
  ])('rejects symlink/directory attributes and required-version mismatch: %j', async (options) => {
    await expect(verifySupportBundle(supportBundleFixture(options).attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
  });

  it.each(['hash', 'type', 'size', 'trailing', 'compressed', 'crc', 'cardinality'])('rejects invalid archive/header %s', async (kind) => {
    const fixture = supportBundleFixture();
    const attachment = { ...fixture.attachment };
    if (kind === 'hash') attachment.supportBundleSha256 = '0'.repeat(64);
    if (kind === 'type') attachment.contentType = 'text/html';
    if (kind === 'size') attachment.blob = new Blob([new Uint8Array(SUPPORT_BUNDLE_MAX_BYTES + 1)]);
    if (['trailing', 'compressed', 'crc', 'cardinality'].includes(kind)) {
      const bytes = new Uint8Array(fixture.bytes.length + (kind === 'trailing' ? 1 : 0));
      bytes.set(fixture.bytes);
      const view = new DataView(bytes.buffer);
      if (kind === 'compressed') view.setUint16(view.getUint32(fixture.bytes.length - 6, true) + 10, 8, true);
      if (kind === 'crc') bytes[60] ^= 1;
      if (kind === 'cardinality') view.setUint16(bytes.length - 12, 3, true);
      attachment.blob = new Blob([bytes.buffer]);
      attachment.supportBundleSha256 = fixtureSha256(bytes);
    }
    await expect(verifySupportBundle(attachment, SUPPORT_TEST_TENANT)).rejects.toBeInstanceOf(AttachmentValidationError);
  });

  it('enforces the byte bound during streaming and never exposes arbitrary headers', async () => {
    const stream = new ReadableStream({ start(controller) {
      controller.enqueue(new Uint8Array(SUPPORT_BUNDLE_MAX_BYTES));
      controller.enqueue(new Uint8Array(1)); controller.close();
    } });
    await expect(readBoundedAttachment(new Response(stream), SUPPORT_BUNDLE_MAX_BYTES)).rejects.toBeInstanceOf(AttachmentValidationError);
    const fixture = supportBundleFixture();
    const response = new Response(fixture.bytes.buffer, { headers: { ...fixture.headers, 'x-internal-secret': 'never-exposed' } });
    const attachment = await readBoundedAttachment(response, SUPPORT_BUNDLE_MAX_BYTES);
    expect(Object.keys(attachment).sort()).toEqual(['blob', 'contentDisposition', 'contentType', 'supportBundleSha256']);
  });

  it('respects cancellation and never renders raw failure text', async () => {
    const abort = new AbortController(); abort.abort();
    await expect(verifySupportBundle(supportBundleFixture().attachment, SUPPORT_TEST_TENANT, abort.signal)).rejects.toMatchObject({ name: 'AbortError' });
    expect(supportBundleError({ status: 503, message: 'password=secret; host=internal' })).not.toMatch(/password|internal/);
    expect(supportBundleError({ status: 409, detail: { error_code: 'CONSENT_RECONFIRMATION_REQUIRED' } })).toContain('without replaying');
  });

  it.each(['-1', 'NaN', '999999999999999999999'])('rejects malformed declared byte lengths: %s', async (length) => {
    await expect(readBoundedAttachment(new Response('data', { headers: { 'content-length': length } }), SUPPORT_BUNDLE_MAX_BYTES)).rejects.toBeInstanceOf(AttachmentValidationError);
  });

  it('wakes a pending stream read on abort without relying on the fetch implementation', async () => {
    const abort = new AbortController();
    const response = new Response(new ReadableStream());
    const pending = readBoundedAttachment(response, SUPPORT_BUNDLE_MAX_BYTES, abort.signal);
    abort.abort();
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  });
});
