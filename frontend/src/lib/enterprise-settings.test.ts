import { describe, expect, it } from 'vitest';
import {
  describeChannelConfiguration,
  hasSettingsPermission,
  INTEGRATION_CATALOG,
  notificationPollInterval,
  parseJSONObject,
} from '@/lib/enterprise-settings';
import type { NotificationChannel } from '@/types/enterprise-settings';
import type { User } from '@/types';

const ADMIN: User = {
  id: 'user-1',
  organization_id: 'org-1',
  email: 'admin@example.test',
  first_name: 'Admin',
  last_name: 'User',
  status: 'active',
  is_super_admin: false,
  language: 'en',
  created_at: '2026-09-14T00:00:00Z',
  updated_at: '2026-09-14T00:00:00Z',
  roles: [{ id: 'role-1', name: 'Admin', slug: 'org_admin', is_system_role: true, is_custom: false }],
};

describe('enterprise settings helpers', () => {
  it('covers every integration type accepted by the backend contract', () => {
    expect(INTEGRATION_CATALOG.map((entry) => entry.type)).toEqual([
      'sso_saml', 'sso_oidc',
      'cloud_aws', 'cloud_azure', 'cloud_gcp',
      'siem_splunk', 'siem_elastic', 'siem_sentinel',
      'itsm_servicenow', 'itsm_jira', 'itsm_freshservice',
      'email_smtp', 'email_sendgrid', 'slack', 'teams',
      'webhook_inbound', 'webhook_outbound', 'custom_api',
    ]);
  });

  it('accepts JSON objects and rejects arrays so configuration matches the API', () => {
    expect(parseJSONObject('{"endpoint":"https://example.test"}')).toEqual({
      endpoint: 'https://example.test',
    });
    expect(() => parseJSONObject('[]')).toThrow('must be a JSON object');
    expect(() => parseJSONObject('{')).toThrow('must be valid JSON');
  });

  it('uses an admin role only when the permissions API has no settings decision', () => {
    expect(hasSettingsPermission(undefined, ADMIN, 'configure')).toBe(true);
    expect(hasSettingsPermission({ settings: ['read'] }, ADMIN, 'configure')).toBe(false);
    expect(hasSettingsPermission({ settings: ['read', 'configure'] }, ADMIN, 'configure')).toBe(true);
  });

  it('backs off polling and stops it while hidden or offline', () => {
    expect(notificationPollInterval(0, false, true)).toBe(60_000);
    expect(notificationPollInterval(2, false, true)).toBe(240_000);
    expect(notificationPollInterval(20, false, true)).toBe(300_000);
    expect(notificationPollInterval(0, true, true)).toBe(false);
    expect(notificationPollInterval(0, false, false)).toBe(false);
  });

  it('describes only redacted notification channel configuration', () => {
    const channel: NotificationChannel = {
      id: 'channel-1',
      organization_id: 'org-1',
      name: 'Signed endpoint',
      channel_type: 'webhook',
      config: { endpoint_origin: 'https://hooks.example.test', secret_configured: true },
      is_active: true,
      created_at: '2026-09-14T00:00:00Z',
      updated_at: '2026-09-14T00:00:00Z',
    };
    const description = describeChannelConfiguration(channel);
    expect(description).toContain('https://hooks.example.test');
    expect(description).toContain('secret configured');
    expect(description).not.toContain('credential');
  });
});
