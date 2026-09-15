import type {
  IntegrationType,
  NotificationChannel,
  NotificationPreference,
} from '@/types/enterprise-settings';
import type { ApiError } from '@/lib/api';
import type { PermissionMap } from '@/types/access';
import type { User } from '@/types';

export interface IntegrationCatalogEntry {
  type: IntegrationType;
  name: string;
  category: 'Identity' | 'Cloud' | 'Security' | 'IT service management' | 'Messaging' | 'Webhooks';
  description: string;
}

export const INTEGRATION_CATALOG: readonly IntegrationCatalogEntry[] = [
  { type: 'sso_saml', name: 'SAML connector', category: 'Identity', description: 'Exchange identity events with a SAML 2.0 provider.' },
  { type: 'sso_oidc', name: 'OIDC connector', category: 'Identity', description: 'Exchange identity events with an OpenID Connect provider.' },
  { type: 'cloud_aws', name: 'Amazon Web Services', category: 'Cloud', description: 'Collect evidence and inventory from AWS.' },
  { type: 'cloud_azure', name: 'Microsoft Azure', category: 'Cloud', description: 'Collect evidence and inventory from Azure.' },
  { type: 'cloud_gcp', name: 'Google Cloud', category: 'Cloud', description: 'Collect evidence and inventory from Google Cloud.' },
  { type: 'siem_splunk', name: 'Splunk', category: 'Security', description: 'Connect security events and findings from Splunk.' },
  { type: 'siem_elastic', name: 'Elastic', category: 'Security', description: 'Connect security events from Elastic Security.' },
  { type: 'siem_sentinel', name: 'Microsoft Sentinel', category: 'Security', description: 'Connect security events from Microsoft Sentinel.' },
  { type: 'itsm_servicenow', name: 'ServiceNow', category: 'IT service management', description: 'Coordinate incidents and remediation in ServiceNow.' },
  { type: 'itsm_jira', name: 'Jira', category: 'IT service management', description: 'Coordinate issues and remediation in Jira.' },
  { type: 'itsm_freshservice', name: 'Freshservice', category: 'IT service management', description: 'Coordinate service work in Freshservice.' },
  { type: 'email_smtp', name: 'SMTP', category: 'Messaging', description: 'Deliver platform mail through an SMTP service.' },
  { type: 'email_sendgrid', name: 'SendGrid', category: 'Messaging', description: 'Deliver platform mail through SendGrid.' },
  { type: 'slack', name: 'Slack', category: 'Messaging', description: 'Send operational messages to Slack.' },
  { type: 'teams', name: 'Microsoft Teams', category: 'Messaging', description: 'Send operational messages to Microsoft Teams.' },
  { type: 'webhook_inbound', name: 'Inbound webhook', category: 'Webhooks', description: 'Accept events from an approved external source.' },
  { type: 'webhook_outbound', name: 'Outbound webhook', category: 'Webhooks', description: 'Send events to an approved external endpoint.' },
  { type: 'custom_api', name: 'Custom API', category: 'Webhooks', description: 'Configure a tenant-specific API connection.' },
] as const;

export const API_KEY_ACTIONS = [
  'create',
  'read',
  'update',
  'delete',
  'approve',
  'assign',
  'export',
  'configure',
] as const;

export const API_KEY_RESOURCES = [
  'organizations',
  'frameworks',
  'controls',
  'risks',
  'policies',
  'audits',
  'incidents',
  'vendors',
  'reports',
  'users',
  'settings',
] as const;

export const DEFAULT_NOTIFICATION_PREFERENCE: NotificationPreference = {
  user_id: '',
  organization_id: '',
  event_type: '*',
  email_enabled: true,
  in_app_enabled: true,
  slack_enabled: false,
  digest_frequency: 'immediate',
  quiet_hours_start: null,
  quiet_hours_end: null,
  quiet_hours_timezone: null,
};

export function parseJSONObject(value: string, label = 'JSON'): Record<string, unknown> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || '{}');
  } catch {
    throw new Error(`${label} must be valid JSON.`);
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(`${label} must be a JSON object.`);
  }
  return parsed as Record<string, unknown>;
}

export function parseCommaList(value: string): string[] {
  return [...new Set(value.split(',').map((item) => item.trim()).filter(Boolean))];
}

export function isNotificationToken(value: string): boolean {
  return value === '*' || /^[a-z0-9][a-z0-9_.-]{0,99}$/.test(value);
}

export function isUuid(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}

export function formatApiError(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (!error || typeof error !== 'object') return fallback;
  const candidate = error as ApiError & { detail?: unknown };
  const detail = candidate.detail;
  if (detail && typeof detail === 'object') {
    const record = detail as Record<string, unknown>;
    const detailed = record.detail;
    if (typeof detailed === 'string' && detailed) return detailed;
    if (typeof record.message === 'string' && record.message) return record.message;
  }
  return candidate.message || fallback;
}

export function describeChannelConfiguration(channel: NotificationChannel): string {
  if (channel.channel_type === 'email') return 'Platform email transport';
  if (channel.channel_type === 'in_app') return 'In-app delivery';
  if (channel.channel_type === 'slack') {
    return channel.config.webhook_configured ? 'Webhook configured' : 'Webhook missing';
  }
  const origin = typeof channel.config.endpoint_origin === 'string'
    ? channel.config.endpoint_origin
    : 'Endpoint configured';
  return `${origin} · ${channel.config.secret_configured ? 'secret configured' : 'secret missing'}`;
}

export function hasSettingsPermission(
  permissions: PermissionMap | undefined,
  user: User | null | undefined,
  action: 'read' | 'configure'
): boolean {
  if (user?.is_super_admin) return true;
  const explicit = permissions?.settings;
  if (explicit) return explicit.includes(action);
  return Boolean(user?.roles?.some((role) => role.slug === 'org_admin'));
}

export function notificationPollInterval(
  failureCount: number,
  hidden: boolean,
  online: boolean
): number | false {
  if (hidden || !online) return false;
  return Math.min(5 * 60_000, 60_000 * 2 ** Math.min(failureCount, 3));
}

export function localTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
}
