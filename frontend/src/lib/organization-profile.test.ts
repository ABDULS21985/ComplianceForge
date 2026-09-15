import { describe, expect, it } from 'vitest';
import {
  editableOrganizationProfile, normalizeOrganizationProfile, organizationProfileChanges, organizationProfileDraft,
  organizationProfileError, serializeOrganizationProfileUpdate, validateOrganizationProfileUpdate, validFiscalStart,
} from './organization-profile';
import { ORGANIZATION_TEST_TENANT, organizationProfileFixture } from '@/test/organization-profile-fixture';
import type { OrganizationProfileUpdateInput } from '@/types/organization-profile';

describe('tenant organisation projection and presence-aware validation', () => {
  it.each([undefined, {}, { schema_version: 2, scope: 'tenant_organization_profile', editable: true },
    { schema_version: 1, scope: 'other', editable: true }, { schema_version: 1, scope: 'tenant_organization_profile', editable: 'true' },
    { schema_version: 1, scope: 'tenant_organization_profile', editable: false }])('never creates editable drafts from missing/invalid/restricted metadata %j', (meta) => {
    const envelope = normalizeOrganizationProfile({ ...organizationProfileFixture(), meta });
    expect(editableOrganizationProfile(envelope, ORGANIZATION_TEST_TENANT)).toBeNull();
  });
  it('requires complete safe fields and matching tenant independently of editable metadata', () => {
    const fixture = organizationProfileFixture();
    expect(editableOrganizationProfile(fixture, ORGANIZATION_TEST_TENANT)).toEqual(fixture.data);
    const { legal_name: _hidden, ...partial } = fixture.data;
    expect(editableOrganizationProfile({ ...fixture, data: partial }, ORGANIZATION_TEST_TENANT)).toBeNull();
    expect(editableOrganizationProfile(fixture, 'df840b54-d4e3-42ba-aa18-d0f9cd5f3a61')).toBeNull();
    expect(editableOrganizationProfile(organizationProfileFixture({ version: Number.MAX_SAFE_INTEGER }), ORGANIZATION_TEST_TENANT)).toBeNull();
  });
  it('picks only reviewed projection/contact fields and preserves omissions instead of defaults', () => {
    const envelope = normalizeOrganizationProfile({ data: { name: 'Masked name', contacts: [{ purpose: '***', email: '***', credentials: 'secret' }], settings: { secret: true } } });
    expect(envelope).toEqual({ data: { name: 'Masked name', contacts: [{ purpose: '***', email: '***' }] } });
    expect(Object.hasOwn(envelope.data, 'legal_name')).toBe(false);
  });
  it('copies a draft and submits only explicit changed fields with current version and reason', () => {
    const profile = organizationProfileFixture({ timezone: 'Legacy/Unknown', supported_languages: [] }).data;
    const draft = organizationProfileDraft(profile); draft.name = 'Revised name';
    expect(organizationProfileChanges(profile, draft, ' Correct organisation name ')).toEqual({ expected_version: 4, reason: 'Correct organisation name', name: 'Revised name' });
    draft.contacts[0].email = 'revised@example.test'; expect(profile.contacts[0].email).toBe('security@example.test');
  });
  it('serializes an allowlisted explicit clear without identifiers, raw settings or contact extras', () => {
    const input = { expected_version: 4, reason: 'Clear optional name', legal_name: '', id: 'other', tier: 'premium', settings: { secret: true }, contacts: [{ purpose: 'support', email: 'help@example.test', secret: 'excluded' }] };
    expect(serializeOrganizationProfileUpdate(input)).toEqual({ expected_version: 4, reason: 'Clear optional name', legal_name: '', contacts: [{ purpose: 'support', email: 'help@example.test' }] });
  });
  it.each([{ name: null }, { contacts: null }, { supported_languages: [null] }, { fiscal_year_start_day: NaN }, { reason: '' }])('rejects null/invalid structural inputs before transport %j', (patch) => {
    expect(() => serializeOrganizationProfileUpdate({ expected_version: 4, reason: 'Reviewed change', ...patch } as unknown as OrganizationProfileUpdateInput)).toThrow();
  });
  it.each([
    [{ reason: '' }, 'reason'], [{ name: '' }, 'name'], [{ name: 'é'.repeat(128) }, 'name'], [{ legal_name: 'Line\nBreak' }, 'legal_name'],
    [{ country_code: 'ng' }, 'country_code'], [{ timezone: 'Local' }, 'timezone'], [{ timezone: 'Unknown/Zone' }, 'timezone'],
    [{ default_language: 'es' }, 'default_language'], [{ supported_languages: ['en', 'en'] }, 'supported_languages'],
    [{ default_language: 'fr' }, 'supported_languages'], [{ fiscal_year_start_month: 2, fiscal_year_start_day: 29 }, 'fiscal_year_start_day'],
    [{ contacts: [{ purpose: 'security', email: 'Name <a@example.test>' }] }, 'contacts'],
    [{ contacts: [{ purpose: 'security', email: 'a@example.test' }, { purpose: 'security', email: 'b@example.test' }] }, 'contacts'],
  ] as const)('validates reviewed field errors %j', (patch, field) => {
    const input = { expected_version: 4, reason: 'Reviewed change', name: 'Changed', ...patch } as OrganizationProfileUpdateInput;
    expect(validateOrganizationProfileUpdate(input, organizationProfileFixture().data)[field]).toBeTruthy();
  });
  it('accepts syntax-only country ZZ, UTC/IANA, supported language pairing and ordinary optional clears', () => {
    const input = { expected_version: 4, reason: 'Reviewed change', country_code: 'ZZ', timezone: 'UTC', default_language: 'fr', supported_languages: ['en', 'fr'], legal_name: '', contacts: [] };
    expect(validateOrganizationProfileUpdate(input, organizationProfileFixture().data)).toEqual({});
    expect(validFiscalStart(2, 28)).toBe(true); expect(validFiscalStart(4, 31)).toBe(false);
  });
  it('never renders raw database/provider error text', () => {
    for (const status of [400, 401, 403, 409, 413, 422, 503, 500]) expect(organizationProfileError({ status, message: 'password=secret; host=private.internal' })).not.toMatch(/password|private.internal/);
  });
});
