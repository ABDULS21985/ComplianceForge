import type {
  OrganizationProfile,
  OrganizationProfileEditable,
  OrganizationProfileEnvelope,
  OrganizationProfileProjection,
  OrganizationProfileUpdateInput,
} from '@/types/organization-profile';
import type { ApiError } from './api';

export const ORGANIZATION_PROFILE_ROUTE = '/settings/organization';
export const ORGANIZATION_PROFILE_MAX_BODY_BYTES = 8 * 1024;
export const ORGANIZATION_PROFILE_LANGUAGES = ['en', 'de', 'fr'] as const;
export const ORGANIZATION_CONTACT_PURPOSES = ['security', 'privacy', 'billing', 'support'] as const;
export const PROFILE_TEXT_FIELDS = ['name', 'legal_name', 'industry', 'country_code', 'timezone', 'default_language', 'employee_count_range'] as const;
export const PROFILE_KEYS = ['id', ...PROFILE_TEXT_FIELDS, 'slug', 'supported_languages', 'status', 'tier',
  'fiscal_year_start_month', 'fiscal_year_start_day', 'contacts', 'version', 'updated_at'] as const;
export const PROFILE_EDITABLE_KEYS = [...PROFILE_TEXT_FIELDS, 'supported_languages', 'fiscal_year_start_month', 'fiscal_year_start_day', 'contacts'] as const;
export const PROFILE_UPDATE_KEYS = ['expected_version', 'reason', ...PROFILE_EDITABLE_KEYS] as const;
export type OrganizationProfileErrors = Partial<Record<typeof PROFILE_UPDATE_KEYS[number] | 'profile', string>>;

export class OrganizationProfileContractError extends Error {
  constructor() { super('The authorized organisation profile could not be safely read.'); }
}

export function isCanonicalOrganizationId(value: string) {
  return /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/.test(value) && value !== '00000000-0000-0000-0000-000000000000';
}

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

/** Pick only reviewed values. Unknown/raw fields never reach component state. */
export function normalizeOrganizationProfile(value: unknown): OrganizationProfileEnvelope {
  if (!record(value) || !record(value.data)) throw new OrganizationProfileContractError();
  const data: OrganizationProfileProjection = {};
  for (const key of ['id', ...PROFILE_TEXT_FIELDS, 'slug', 'status', 'tier', 'updated_at'] as const) {
    if (typeof value.data[key] === 'string') data[key] = value.data[key];
  }
  for (const key of ['fiscal_year_start_month', 'fiscal_year_start_day', 'version'] as const) {
    if (typeof value.data[key] === 'number' && Number.isSafeInteger(value.data[key])) data[key] = value.data[key];
  }
  if (Array.isArray(value.data.supported_languages) && value.data.supported_languages.every((item) => typeof item === 'string')) {
    data.supported_languages = [...value.data.supported_languages];
  }
  if (Array.isArray(value.data.contacts) && value.data.contacts.every((item) => record(item) && typeof item.purpose === 'string' && typeof item.email === 'string')) {
    data.contacts = value.data.contacts.map((item) => ({ purpose: item.purpose as string, email: item.email as string }));
  }
  const meta = value.meta;
  if (record(meta) && Object.keys(meta).length === 3 && meta.schema_version === 1 && meta.scope === 'tenant_organization_profile' && typeof meta.editable === 'boolean') {
    return { data, meta: { schema_version: 1, scope: 'tenant_organization_profile', editable: meta.editable } };
  }
  return { data };
}

export function editableOrganizationProfile(envelope: OrganizationProfileEnvelope, tenantId: string): OrganizationProfile | null {
  const data = envelope.data;
  if (envelope.meta?.editable !== true || data.id !== tenantId || !isCanonicalOrganizationId(tenantId) ||
    !PROFILE_KEYS.every((key) => Object.hasOwn(data, key)) ||
    !data.version || data.version < 1 || data.version >= Number.MAX_SAFE_INTEGER || !data.updated_at || !Number.isFinite(Date.parse(data.updated_at)) ||
    !['active', 'trial'].includes(data.status ?? '') ||
    !Array.isArray(data.contacts) || !Array.isArray(data.supported_languages) ||
    !validFiscalStart(data.fiscal_year_start_month ?? 0, data.fiscal_year_start_day ?? 0)) return null;
  return data as OrganizationProfile;
}

export function organizationProfileDraft(profile: OrganizationProfile): OrganizationProfileEditable {
  const draft: OrganizationProfileEditable = {
    name: profile.name, legal_name: profile.legal_name, industry: profile.industry, country_code: profile.country_code,
    timezone: profile.timezone, default_language: profile.default_language, employee_count_range: profile.employee_count_range,
    supported_languages: [...profile.supported_languages], fiscal_year_start_month: profile.fiscal_year_start_month,
    fiscal_year_start_day: profile.fiscal_year_start_day, contacts: profile.contacts.map((contact) => ({ ...contact })),
  };
  return draft;
}

export function organizationProfileChanges(profile: OrganizationProfile, draft: OrganizationProfileEditable, reason: string): OrganizationProfileUpdateInput {
  const input: OrganizationProfileUpdateInput = { expected_version: profile.version, reason: reason.trim() };
  for (const key of PROFILE_TEXT_FIELDS) if (profile[key] !== draft[key]) input[key] = draft[key];
  for (const key of ['fiscal_year_start_month', 'fiscal_year_start_day'] as const) if (profile[key] !== draft[key]) input[key] = draft[key];
  if (JSON.stringify(profile.supported_languages) !== JSON.stringify(draft.supported_languages)) input.supported_languages = [...draft.supported_languages];
  if (JSON.stringify(profile.contacts) !== JSON.stringify(draft.contacts)) input.contacts = draft.contacts.map((contact) => ({ ...contact }));
  return input;
}

function boundedText(value: string, maximum: number, required = false) {
  return new TextEncoder().encode(value).length <= maximum && !/[\p{Cc}\p{Surrogate}]/u.test(value) && (!required || value.trim().length > 0);
}
export function validFiscalStart(month: number, day: number) {
  const maximum = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1] ?? 0;
  return Number.isInteger(month) && Number.isInteger(day) && day >= 1 && day <= maximum;
}
function validTimezone(value: string) {
  if (value === 'Local' || !/^[A-Za-z0-9/_+\-]{1,50}$/.test(value)) return false;
  try { new Intl.DateTimeFormat('en', { timeZone: value }).format(); return true; } catch { return false; }
}

export function validateOrganizationProfileUpdate(input: OrganizationProfileUpdateInput, current?: OrganizationProfile): OrganizationProfileErrors {
  const errors: OrganizationProfileErrors = {};
  if (!Number.isSafeInteger(input.expected_version) || input.expected_version < 1) errors.expected_version = 'Reload a current profile version before saving.';
  if (!boundedText(input.reason, 500, true)) errors.reason = 'Provide a single-line reason, at most 500 UTF-8 bytes, without control characters.';
  for (const [key, limit, required] of [['name', 255, true], ['legal_name', 500, false], ['industry', 100, false], ['employee_count_range', 20, false]] as const) {
    if (input[key] !== undefined && !boundedText(input[key], limit, required)) errors[key] = `Use ${required ? 'non-empty text' : 'text'} without control characters, at most ${limit} UTF-8 bytes.`;
  }
  if (input.country_code !== undefined && input.country_code !== '' && !/^[A-Z]{2}$/.test(input.country_code)) errors.country_code = 'Use two uppercase letters, or leave this optional field blank. This is syntax validation, not an ISO registry check.';
  if (input.timezone !== undefined && !validTimezone(input.timezone)) errors.timezone = 'Use an IANA timezone such as Africa/Lagos, Europe/Berlin or UTC; Local is not allowed.';
  if (input.default_language !== undefined && !ORGANIZATION_PROFILE_LANGUAGES.some((language) => language === input.default_language)) errors.default_language = 'Choose English, German or French.';
  if (input.supported_languages !== undefined) {
    if (input.supported_languages.length < 1 || input.supported_languages.length > 3 || new Set(input.supported_languages).size !== input.supported_languages.length ||
      !input.supported_languages.every((language) => ORGANIZATION_PROFILE_LANGUAGES.some((supported) => supported === language))) errors.supported_languages = 'Choose one to three unique supported languages: English, German or French.';
  }
  if (input.default_language !== undefined || input.supported_languages !== undefined) {
    if (!(input.supported_languages ?? current?.supported_languages ?? []).includes(input.default_language ?? current?.default_language ?? '')) errors.supported_languages = 'The default language must be included in supported languages.';
  }
  if (input.fiscal_year_start_month !== undefined || input.fiscal_year_start_day !== undefined) {
    if (!validFiscalStart(input.fiscal_year_start_month ?? current?.fiscal_year_start_month ?? 0, input.fiscal_year_start_day ?? current?.fiscal_year_start_day ?? 0)) errors.fiscal_year_start_day = 'Choose a valid recurring date in a non-leap year. February 29 is not supported.';
  }
  if (input.contacts !== undefined) {
    if (input.contacts.length > 4 || new Set(input.contacts.map(({ purpose }) => purpose)).size !== input.contacts.length ||
      !input.contacts.every(({ purpose, email }) => ORGANIZATION_CONTACT_PURPOSES.some((approved) => approved === purpose) &&
        email.length >= 3 && email.length <= 254 && /^[!-~]+$/.test(email) && /^[^@\s<>()\[\],;:"\\]+@[^@\s<>()\[\],;:"\\]+$/.test(email))) {
      errors.contacts = 'Use at most one plain ASCII email for each security, privacy, billing or support purpose (254 bytes maximum). No display names or credentials.';
    }
  }
  if (!PROFILE_EDITABLE_KEYS.some((key) => Object.hasOwn(input, key))) errors.profile = 'Change at least one editable field before saving.';
  if (new TextEncoder().encode(JSON.stringify(input)).length > ORGANIZATION_PROFILE_MAX_BODY_BYTES) errors.profile = 'The profile update exceeds the 8 KiB limit.';
  return errors;
}

/** Explicit allowlist: identifiers, lifecycle, tier, settings and nulls cannot be serialized. */
export function serializeOrganizationProfileUpdate(input: OrganizationProfileUpdateInput): OrganizationProfileUpdateInput {
  const selected: OrganizationProfileUpdateInput = { expected_version: input.expected_version, reason: input.reason };
  for (const key of PROFILE_TEXT_FIELDS) {
    if (Object.hasOwn(input, key)) { if (typeof input[key] !== 'string') throw new OrganizationProfileContractError(); selected[key] = input[key]; }
  }
  for (const key of ['fiscal_year_start_month', 'fiscal_year_start_day'] as const) {
    if (Object.hasOwn(input, key)) { if (!Number.isSafeInteger(input[key])) throw new OrganizationProfileContractError(); selected[key] = input[key]; }
  }
  if (Object.hasOwn(input, 'supported_languages')) {
    if (!Array.isArray(input.supported_languages) || !input.supported_languages.every((item) => typeof item === 'string')) throw new OrganizationProfileContractError(); selected.supported_languages = [...input.supported_languages];
  }
  if (Object.hasOwn(input, 'contacts')) {
    if (!Array.isArray(input.contacts) || !input.contacts.every((contact) => record(contact) && typeof contact.purpose === 'string' && typeof contact.email === 'string')) throw new OrganizationProfileContractError(); selected.contacts = input.contacts.map(({ purpose, email }) => ({ purpose, email }));
  }
  if (!Number.isSafeInteger(selected.expected_version) || selected.expected_version < 1 || selected.expected_version >= Number.MAX_SAFE_INTEGER ||
    typeof selected.reason !== 'string' || !boundedText(selected.reason, 500, true) ||
    new TextEncoder().encode(JSON.stringify(selected)).length > ORGANIZATION_PROFILE_MAX_BODY_BYTES) throw new OrganizationProfileContractError();
  return selected;
}

export function organizationProfileError(error: unknown) {
  const status = error && typeof error === 'object' ? (error as ApiError).status : undefined;
  if (status === 409) return 'Another administrator changed this profile. Reload and review the current values before editing again; nothing will be retried automatically.';
  if (status === 403 || status === 401) return 'Your access could not be confirmed. Profile editing and local drafts have been stopped.';
  if (status === 422 || status === 400) return 'The server rejected this profile update. Review the supported fields and validation rules before submitting again.';
  if (status === 413) return 'The update exceeds the 8 KiB request limit. Reduce the profile text and try again explicitly.';
  if (status === 503) return 'The profile could not be safely prepared or edited. Field restrictions or temporary service issues may apply; no automatic retry will occur.';
  return 'Organisation settings are temporarily unavailable. No raw configuration or provider error is shown.';
}
