import type { OrganizationProfile, OrganizationProfileEnvelope } from '@/types/organization-profile';

export const ORGANIZATION_TEST_TENANT = '7a2423cd-bdeb-472f-a60a-1b6cfec87aa8';
export const ORGANIZATION_TEST_ACTOR = 'bba53cd8-dabc-4c88-97bc-319aa1bc79ed';
export function organizationProfileFixture(patch: Partial<OrganizationProfile> = {}): OrganizationProfileEnvelope & { data: OrganizationProfile } {
  return {
    data: {
      id: ORGANIZATION_TEST_TENANT, name: 'Example Enterprise', slug: 'example-enterprise', legal_name: 'Example Enterprise Ltd', industry: 'Technology',
      country_code: 'NG', timezone: 'Africa/Lagos', default_language: 'en', supported_languages: ['en'], employee_count_range: '50-249',
      status: 'active', tier: 'enterprise', fiscal_year_start_month: 1, fiscal_year_start_day: 1,
      contacts: [{ purpose: 'security', email: 'security@example.test' }], version: 4, updated_at: '2026-09-15T07:00:00Z', ...patch,
    },
    meta: { schema_version: 1, scope: 'tenant_organization_profile', editable: true },
  };
}
