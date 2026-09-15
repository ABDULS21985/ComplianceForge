import { describe, expect, it } from 'vitest';
import type { SupportBundleManifest, SupportBundleRequest } from '@/types/support-bundle';
import { fileURLToPath } from 'node:url';
import { readFileSync } from 'node:fs';
import { SUPPORT_BUNDLE_ROUTE } from './support-bundle-contract';

type Assert<T extends true> = T;
const requestKeys = ['consent', 'scope'] as const satisfies readonly (keyof SupportBundleRequest)[];
const manifestKeys = [
  'schema_version', 'bundle_id', 'organization_id', 'generated_at', 'scope', 'consent_recorded_at',
  'redaction_profile', 'configuration_fingerprint', 'automatically_transmitted', 'files', 'excluded',
] as const satisfies readonly (keyof SupportBundleManifest)[];
const exact: [Assert<Exclude<keyof SupportBundleRequest, typeof requestKeys[number]> extends never ? true : false>,
  Assert<Exclude<keyof SupportBundleManifest, typeof manifestKeys[number]> extends never ? true : false>] = [true, true];
void exact;
interface Schema { properties: Record<string, { enum?: unknown[] }>; required?: string[]; additionalProperties?: boolean }
const spec = JSON.parse(readFileSync(fileURLToPath(new URL('../../../api/openapi/openapi.json', import.meta.url)), 'utf8')) as {
  components: { schemas: Record<string, Schema> };
  paths: Record<string, { post?: { responses: Record<string, { content?: Record<string, unknown>; headers?: Record<string, unknown> }> } }>;
};

describe('support bundle binary/ZIP metadata contract', () => {
  it('binds exact consent/scope request and attachment/hash response to the generated route', () => {
    const schema = spec.components.schemas.SupportBundleRequest;
    expect(Object.keys(schema.properties).sort()).toEqual([...requestKeys].sort());
    expect(schema.required?.sort()).toEqual([...requestKeys].sort());
    expect(schema.additionalProperties).toBe(false);
    expect(schema.properties.consent.enum).toEqual([true]);
    expect(schema.properties.scope.enum).toEqual(['health_and_posture']);
    const operation = spec.paths[`/api/v1${SUPPORT_BUNDLE_ROUTE}`]?.post;
    expect(operation?.responses['200'].content?.['application/octet-stream']).toBeDefined();
    expect(operation?.responses['200'].headers?.['X-Support-Bundle-SHA256']).toBeDefined();
  });

  it('keeps typed internal ZIP manifest fields aligned with the Go authority', () => {
    const source = readFileSync(fileURLToPath(new URL('../../../internal/models/support_bundle.go', import.meta.url)), 'utf8');
    const manifest = source.match(/type SupportBundleManifest struct \{([\s\S]*?)\n\}/)?.[1] ?? '';
    const keys = [...manifest.matchAll(/json:"([a-z_]+)"/g)].map((match) => match[1]);
    expect(keys.sort()).toEqual([...manifestKeys].sort());
  });
});
