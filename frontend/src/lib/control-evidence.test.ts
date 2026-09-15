import {
  CONTROL_EVIDENCE_ROUTES,
  EVIDENCE_MAX_UPLOAD_BYTES,
  evidenceDownloadHref,
  evidenceFileValidationError,
  evidenceFreshness,
  formatEvidenceError,
  parseEvidenceMetadata,
  safeEvidenceFilename,
  serializeEvidenceSupersede,
  serializeEvidenceUpload,
  versionReasonError,
} from '@/lib/control-evidence';
import { describe, expect, it } from 'vitest';
import type { ControlEvidence } from '@/types/control-evidence';

function evidence(overrides: Partial<ControlEvidence> = {}): ControlEvidence {
  return {
    id: 'evidence-1',
    organization_id: 'organization-1',
    control_implementation_id: 'implementation-1',
    title: 'Quarterly access review',
    evidence_type: 'document',
    collection_method: 'manual_upload',
    collected_at: '2026-09-01T00:00:00Z',
    is_current: true,
    review_status: 'pending',
    metadata: {},
    series_id: 'series-1',
    version_number: 1,
    lifecycle_status: 'active',
    version_reason: 'Initial evidence upload',
    content_fingerprint: 'b'.repeat(64),
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
    ...overrides,
  };
}

describe('control evidence input boundary', () => {
  it('accepts exactly the server-supported extensions and enforces the 25 MiB default', () => {
    for (const [name, type] of [
      ['report.pdf', 'application/pdf'],
      ['screen.png', 'image/png'],
      ['photo.jpeg', 'image/jpeg'],
      ['events.txt', 'text/plain'],
      ['export.csv', 'text/csv'],
      ['snapshot.json', 'application/json'],
      ['policy.docx', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'],
      ['register.xlsx', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'],
      [
        'briefing.pptx',
        'application/vnd.openxmlformats-officedocument.presentationml.presentation',
      ],
    ]) {
      expect(evidenceFileValidationError(new File(['safe'], name, { type })), name).toBeNull();
    }

    expect(
      evidenceFileValidationError(
        {
          name: 'large.pdf',
          size: EVIDENCE_MAX_UPLOAD_BYTES + 1,
          type: 'application/pdf',
        },
      ),
    ).toContain('25 MiB');
    expect(
      evidenceFileValidationError(new File(['<svg/>'], 'active.svg', { type: 'image/svg+xml' })),
    ).toContain('supported formats');
    expect(
      evidenceFileValidationError(new File(['text'], 'renamed.pdf', { type: 'text/plain' })),
    ).toContain('do not match');
  });

  it('validates bounded JSON metadata and rejects the server-managed security field', () => {
    expect(parseEvidenceMetadata(' {"source":"hr"} ')).toEqual({ source: 'hr' });
    expect(parseEvidenceMetadata('')).toBeUndefined();
    expect(() => parseEvidenceMetadata('[]')).toThrow('JSON object');
    expect(() => parseEvidenceMetadata('{bad')).toThrow('valid JSON');
    expect(() => parseEvidenceMetadata('{"object_security":{}}')).toThrow('reserved');
    expect(() => parseEvidenceMetadata(`{"value":"${'x'.repeat(17_000)}"}`)).toThrow('16 KiB');
  });

  it('serializes one file and only the canonical multipart fields once', () => {
    const body = serializeEvidenceUpload({
      file: new File(['a,b\n1,2'], 'report.csv', { type: 'application/vnd.ms-excel' }),
      title: ' Quarterly report ',
      evidence_type: 'report',
      description: ' Approved export ',
      valid_from: '2026-09-01',
      valid_until: '2026-12-01',
      metadata: { source: 'finance' },
    });

    expect([...body.keys()]).toEqual([
      'file',
      'title',
      'evidence_type',
      'description',
      'valid_from',
      'valid_until',
      'metadata',
    ]);
    for (const key of body.keys()) expect(body.getAll(key)).toHaveLength(1);
    expect(body.has('files')).toBe(false);
    expect(body.has('file_hash')).toBe(false);
    expect(body.has('object_key')).toBe(false);
    expect(body.get('title')).toBe('Quarterly report');
    expect((body.get('file') as File).type).toBe('text/csv');
  });

  it('serializes a replacement version with one bounded, single-line reason', () => {
    const body = serializeEvidenceSupersede({
      file: new File(['%PDF-v2'], 'review-v2.pdf', { type: 'application/pdf' }),
      title: 'Access review',
      evidence_type: 'document',
      version_reason: ' Quarterly refresh ',
    });

    expect(body.getAll('file')).toHaveLength(1);
    expect(body.getAll('version_reason')).toEqual(['Quarterly refresh']);
    expect(versionReasonError('ab')).toContain('3-1000');
    expect(versionReasonError('unsafe\nsecond line')).toContain('single line');
    expect(() =>
      serializeEvidenceSupersede({
        file: new File(['%PDF-v2'], 'review-v2.pdf', { type: 'application/pdf' }),
        title: 'Access review',
        evidence_type: 'document',
        version_reason: ' ',
      }),
    ).toThrow('Version reason');
  });
});

describe('control evidence presentation safety', () => {
  it('gives actionable scan, payload, and storage-capacity failures without leaking internals', () => {
    expect(formatEvidenceError({ status: 413, message: 'raw' }, 'upload')).toContain('25 MiB');
    expect(formatEvidenceError({ status: 402, message: 'raw' }, 'upload')).toContain('storage quota');
    expect(formatEvidenceError({ status: 422, message: 'virus-name' }, 'upload')).toBe(
      'The file was rejected by content validation or malware scanning. Choose a trusted supported file.',
    );
    expect(formatEvidenceError({ status: 503, message: 'clamd.internal:3310' }, 'upload')).toBe(
      'Security scanning is temporarily unavailable. The file was not accepted; retry later.',
    );
  });

  it('distinguishes scheduled, expiring, expired, superseded, and current evidence', () => {
    const now = new Date('2026-09-14T12:00:00Z');
    expect(evidenceFreshness(evidence({ valid_from: '2026-10-01T00:00:00Z' }), now)).toBe(
      'scheduled',
    );
    expect(evidenceFreshness(evidence({ valid_until: '2026-09-20T00:00:00Z' }), now)).toBe(
      'expiring',
    );
    expect(evidenceFreshness(evidence({ valid_until: '2026-09-01T00:00:00Z' }), now)).toBe(
      'expired',
    );
    expect(
      evidenceFreshness(evidence({ expires_at: '2026-09-01T00:00:00Z' }), now),
    ).toBe('expired');
    expect(evidenceFreshness(evidence({ lifecycle_status: 'superseded' }), now)).toBe(
      'superseded',
    );
    expect(evidenceFreshness(evidence({ is_current: false }), now)).toBe('superseded');
    expect(evidenceFreshness(evidence(), now)).toBe('current');
  });

  it('encodes route identifiers and neutralizes unsafe download filenames', () => {
    expect(CONTROL_EVIDENCE_ROUTES.download('control/../one', 'evidence?token=secret')).toBe(
      '/controls/control%2F..%2Fone/evidence/evidence%3Ftoken%3Dsecret/download',
    );
    expect(evidenceDownloadHref('control/one', 'evidence/two')).toBe(
      '/api/bff/controls/control%2Fone/evidence/evidence%2Ftwo/download',
    );
    expect(safeEvidenceFilename('../../payroll\u202ereport.pdf')).toBe('_.._payroll_report.pdf');
    expect(safeEvidenceFilename(undefined)).toBe('evidence-download');
  });
});
