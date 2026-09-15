import type {
  ControlEvidence,
  EvidenceCustodyChainVerification,
  EvidenceCustodyEvent,
  EvidenceIntegrityResult,
  EvidenceLifecycleRecord,
  EvidenceReview,
} from '@/types/control-evidence';
import { describe, expect, it } from 'vitest';

import { CONTROL_EVIDENCE_ROUTES } from '@/lib/control-evidence';
import { fileURLToPath } from 'node:url';
import { readFileSync } from 'node:fs';

interface OpenAPISchema {
  enum?: string[];
  properties?: Record<string, unknown>;
  required?: string[];
}

interface OpenAPIDocument {
  components: { schemas: Record<string, OpenAPISchema> };
  paths: Record<string, Record<string, { responses?: Record<string, unknown> }>>;
}

type ExactStringKeys<T, Keys extends readonly string[]> =
  Exclude<Extract<keyof T, string>, Keys[number]> extends never
    ? Exclude<Keys[number], Extract<keyof T, string>> extends never
      ? true
      : false
    : false;
type Assert<T extends true> = T;

const evidenceKeys = [
  'id',
  'organization_id',
  'control_implementation_id',
  'title',
  'description',
  'evidence_type',
  'content_fingerprint',
  'expires_at',
  'file_name',
  'file_size_bytes',
  'mime_type',
  'file_hash',
  'collection_method',
  'collected_at',
  'collected_by',
  'valid_from',
  'valid_until',
  'is_current',
  'review_status',
  'reviewed_by',
  'reviewed_at',
  'review_notes',
  'metadata',
  'series_id',
  'version_number',
  'supersedes_evidence_id',
  'superseded_by_evidence_id',
  'superseded_at',
  'lifecycle_status',
  'version_reason',
  'created_at',
  'updated_at',
] as const;

const assertion: Assert<ExactStringKeys<ControlEvidence, typeof evidenceKeys>> = true;
void assertion;

const reviewKeys = [
  'id',
  'organization_id',
  'evidence_id',
  'decision',
  'comment',
  'reviewer_id',
  'evidence_sha256',
  'request_id',
  'metadata',
  'created_at',
] as const;
const custodyKeys = [
  'id',
  'organization_id',
  'evidence_id',
  'series_id',
  'sequence',
  'previous_hash',
  'event_hash',
  'event_type',
  'actor_user_id',
  'actor_type',
  'reason',
  'object_sha256',
  'request_id',
  'details',
  'created_at',
] as const;
const chainKeys = [
  'evidence_id',
  'valid',
  'event_count',
  'head_sequence',
  'head_hash',
  'first_invalid_sequence',
] as const;
const lifecycleKeys = [
  'evidence',
  'versions',
  'reviews',
  'custody_events',
  'chain',
  'legal_hold_active',
] as const;
const integrityKeys = ['evidence_id', 'valid', 'sha256', 'size_bytes', 'verified_at'] as const;

const reviewAssertion: Assert<ExactStringKeys<EvidenceReview, typeof reviewKeys>> = true;
const custodyAssertion: Assert<ExactStringKeys<EvidenceCustodyEvent, typeof custodyKeys>> = true;
const chainAssertion: Assert<
  ExactStringKeys<EvidenceCustodyChainVerification, typeof chainKeys>
> = true;
const lifecycleAssertion: Assert<
  ExactStringKeys<EvidenceLifecycleRecord, typeof lifecycleKeys>
> = true;
const integrityAssertion: Assert<ExactStringKeys<EvidenceIntegrityResult, typeof integrityKeys>> =
  true;
void reviewAssertion;
void custodyAssertion;
void chainAssertion;
void lifecycleAssertion;
void integrityAssertion;

const contractPath = fileURLToPath(new URL('../../../api/openapi/openapi.json', import.meta.url));
const contract = JSON.parse(readFileSync(contractPath, 'utf8')) as OpenAPIDocument;

function apiPath(path: string): string {
  return `/api/v1${path}`;
}

describe('control evidence OpenAPI drift contract', () => {
  it('keeps the exact browser-safe evidence DTO and excludes storage keys', () => {
    const schema = contract.components.schemas.ControlEvidence;
    expect(Object.keys(schema.properties ?? {}).sort()).toEqual([...evidenceKeys].sort());
    expect(Object.keys(schema.properties ?? {})).not.toContain('object_key');
    expect(Object.keys(schema.properties ?? {})).not.toContain('file_path');
    expect((schema.properties?.review_status as OpenAPISchema).enum).toEqual([
      'pending',
      'accepted',
      'rejected',
      'expired',
    ]);
    expect((schema.properties?.lifecycle_status as OpenAPISchema).enum).toEqual([
      'active',
      'superseded',
      'expired',
    ]);
  });

  it('documents exact multipart cardinality, review, paging, and download behavior', () => {
    const templatePath = (path: string) =>
      apiPath(path).replace('%7Bid%7D', '{id}').replace('%7BevidenceID%7D', '{evidenceID}');
    const collectionPath = templatePath(CONTROL_EVIDENCE_ROUTES.evidence('{id}'));
    const downloadPath = templatePath(CONTROL_EVIDENCE_ROUTES.download('{id}', '{evidenceID}'));
    const reviewPath = templatePath(CONTROL_EVIDENCE_ROUTES.review('{id}', '{evidenceID}'));
    const historyPath = templatePath(CONTROL_EVIDENCE_ROUTES.history('{id}', '{evidenceID}'));
    const integrityPath = templatePath(
      CONTROL_EVIDENCE_ROUTES.verifyIntegrity('{id}', '{evidenceID}'),
    );
    const supersedePath = templatePath(
      CONTROL_EVIDENCE_ROUTES.supersede('{id}', '{evidenceID}'),
    );
    const multipart = contract.components.schemas.ControlEvidenceCreateMultipart;
    const supersedeMultipart = contract.components.schemas.ControlEvidenceSupersedeMultipart;

    expect(contract.paths[collectionPath]?.get).toBeDefined();
    expect(contract.paths[collectionPath]?.post?.responses).toHaveProperty('413');
    expect(contract.paths[collectionPath]?.post?.responses).toHaveProperty('503');
    expect(contract.paths[downloadPath]?.get?.responses).toHaveProperty('200');
    expect(contract.paths[downloadPath]?.get?.responses).toHaveProperty('307');
    expect(contract.paths[reviewPath]?.post).toBeDefined();
    expect(contract.paths[historyPath]?.get?.responses).toHaveProperty('200');
    expect(contract.paths[integrityPath]?.post?.responses).toHaveProperty('409');
    expect(contract.paths[supersedePath]?.post?.responses).toHaveProperty('201');
    expect(multipart.required).toEqual(['file', 'title', 'evidence_type']);
    expect(Object.keys(multipart.properties ?? {}).sort()).toEqual(
      [
        'description',
        'evidence_type',
        'file',
        'metadata',
        'title',
        'valid_from',
        'valid_until',
      ].sort(),
    );
    expect(
      (contract.components.schemas.ControlEvidenceReview.properties?.status as OpenAPISchema).enum,
    ).toEqual(['accepted', 'rejected']);
    expect(supersedeMultipart.required).toEqual([
      'file',
      'title',
      'evidence_type',
      'version_reason',
    ]);
    expect(Object.keys(supersedeMultipart.properties ?? {}).sort()).toEqual(
      [...Object.keys(multipart.properties ?? {}), 'version_reason'].sort(),
    );
  });

  it('keeps immutable history, custody, and integrity DTOs exact', () => {
    expect(Object.keys(contract.components.schemas.EvidenceReview.properties ?? {}).sort()).toEqual(
      [...reviewKeys].sort(),
    );
    expect(
      Object.keys(contract.components.schemas.EvidenceCustodyEvent.properties ?? {}).sort(),
    ).toEqual([...custodyKeys].sort());
    expect(
      (contract.components.schemas.EvidenceCustodyEvent.properties?.event_type as OpenAPISchema)
        .enum,
    ).toEqual([
      'uploaded',
      'download_authorized',
      'reviewed',
      'superseded',
      'expired',
      'deleted',
      'integrity_verified',
      'integrity_failed',
      'legal_hold_placed',
      'legal_hold_released',
    ]);
    expect(
      Object.keys(contract.components.schemas.EvidenceCustodyChainVerification.properties ?? {}).sort(),
    ).toEqual([...chainKeys].sort());
    expect(
      Object.keys(contract.components.schemas.EvidenceLifecycleRecord.properties ?? {}).sort(),
    ).toEqual([...lifecycleKeys].sort());
    expect(
      Object.keys(contract.components.schemas.EvidenceIntegrityResult.properties ?? {}).sort(),
    ).toEqual([...integrityKeys].sort());
  });
});
