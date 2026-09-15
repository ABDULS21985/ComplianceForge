/** Reviewed tenant profile DTO. Never includes settings, branding or domains. */
export interface OrganizationContact { purpose: string; email: string }
export type OrganizationContactPurpose = 'security' | 'privacy' | 'billing' | 'support';
export type OrganizationProfileLanguage = 'en' | 'de' | 'fr';
export interface OrganizationProfile {
  id: string;
  name: string;
  slug: string;
  legal_name: string;
  industry: string;
  country_code: string;
  timezone: string;
  default_language: string;
  supported_languages: string[];
  employee_count_range: string;
  status: string;
  tier: string;
  fiscal_year_start_month: number;
  fiscal_year_start_day: number;
  contacts: OrganizationContact[];
  version: number;
  updated_at: string;
}

/** ABAC may omit fields or mask values. This is not an editable full DTO. */
export type OrganizationProfileProjection = Partial<OrganizationProfile>;
export interface OrganizationProfileMeta {
  schema_version: 1;
  scope: 'tenant_organization_profile';
  editable: boolean;
}
export interface OrganizationProfileEnvelope {
  data: OrganizationProfileProjection;
  /** Missing/invalid metadata fails closed for editing. */
  meta?: OrganizationProfileMeta;
}
export type OrganizationProfileEditable = Pick<OrganizationProfile,
  'name' | 'legal_name' | 'industry' | 'country_code' | 'timezone' | 'default_language' |
  'supported_languages' | 'employee_count_range' | 'fiscal_year_start_month' | 'fiscal_year_start_day' | 'contacts'>;
export interface OrganizationProfileUpdateInput extends Partial<OrganizationProfileEditable> {
  expected_version: number;
  reason: string;
}
