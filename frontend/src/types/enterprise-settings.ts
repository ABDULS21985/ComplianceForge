export interface PaginationMeta {
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
}

export interface DataEnvelope<T> {
  data: T;
}

export interface PaginatedDataEnvelope<T> extends DataEnvelope<T[]> {
  pagination: PaginationMeta;
}

export interface MessageResponse {
  message: string;
}

export interface ResourceCreatedResponse extends MessageResponse {
  id: string;
}

export interface NotificationRecord {
  id: string;
  organization_id: string;
  event_type: string;
  recipient_user_id: string;
  channel_type: string;
  subject: string;
  body: string;
  status: string;
  created_at: string;
  read_at: string | null;
}

export interface MarkAllNotificationsReadResponse extends MessageResponse {
  count: number;
}

export type DigestFrequency = 'immediate' | 'hourly' | 'daily' | 'weekly';

export interface NotificationPreference {
  id?: string;
  user_id: string;
  organization_id: string;
  event_type: '*';
  email_enabled: boolean;
  in_app_enabled: boolean;
  slack_enabled: boolean;
  digest_frequency: DigestFrequency;
  quiet_hours_start?: string | null;
  quiet_hours_end?: string | null;
  quiet_hours_timezone?: string | null;
  created_at?: string;
  updated_at?: string;
}

export interface UpdateNotificationPreferenceInput {
  email_enabled?: boolean;
  in_app_enabled?: boolean;
  slack_enabled?: boolean;
  digest_frequency?: DigestFrequency;
  quiet_hours_start?: string;
  quiet_hours_end?: string;
  quiet_hours_timezone?: string;
}

export type NotificationSeverity = 'low' | 'medium' | 'high' | 'critical';
export type NotificationRecipientType =
  | 'user'
  | 'custom'
  | 'role'
  | 'owner'
  | 'assignee'
  | 'dpo'
  | 'ciso';

export interface NotificationRuleInput {
  name: string;
  event_type: string;
  severity_filter: NotificationSeverity[];
  conditions: Record<string, unknown>;
  channel_ids: string[];
  recipient_type: NotificationRecipientType;
  recipient_ids: string[];
  template_id: string | null;
  is_active?: boolean;
  cooldown_minutes: number;
}

export interface NotificationRule extends Omit<NotificationRuleInput, 'is_active'> {
  id: string;
  organization_id: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export type NotificationChannelType = 'email' | 'in_app' | 'webhook' | 'slack';

export interface NotificationChannelInput {
  name: string;
  channel_type: NotificationChannelType;
  config: Record<string, unknown>;
  is_active?: boolean;
}

/** Config is always a redacted descriptor. It never contains a credential. */
export interface NotificationChannel {
  id: string;
  organization_id: string;
  name: string;
  channel_type: NotificationChannelType;
  config: Record<string, unknown>;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface NotificationChannelTestResponse extends MessageResponse {
  channel_type: NotificationChannelType;
}

export interface NotificationTemplateInput {
  name: string;
  event_type: string;
  subject_template: string;
  body_html_template: string;
  body_text_template: string;
  variables: string[];
}

export interface NotificationTemplate extends NotificationTemplateInput {
  id: string;
  organization_id: string | null;
  is_system: boolean;
  created_at: string;
  updated_at: string;
}

export type IntegrationType =
  | 'sso_saml'
  | 'sso_oidc'
  | 'cloud_aws'
  | 'cloud_azure'
  | 'cloud_gcp'
  | 'siem_splunk'
  | 'siem_elastic'
  | 'siem_sentinel'
  | 'itsm_servicenow'
  | 'itsm_jira'
  | 'itsm_freshservice'
  | 'email_smtp'
  | 'email_sendgrid'
  | 'slack'
  | 'teams'
  | 'webhook_inbound'
  | 'webhook_outbound'
  | 'custom_api';

export interface Integration {
  id: string;
  organization_id: string;
  integration_type: IntegrationType;
  name: string;
  description: string | null;
  status: string;
  health_status: string;
  last_health_check_at: string | null;
  last_sync_at: string | null;
  sync_frequency_minutes: number;
  error_count: number;
  last_error_message: string | null;
  capabilities: string[];
  created_at: string;
}

export interface IntegrationInput {
  integration_type?: IntegrationType;
  name?: string;
  description?: string | null;
  sync_frequency_minutes?: number;
  capabilities?: string[];
  /** Omit on update to preserve the encrypted configuration. */
  configuration?: Record<string, unknown>;
}

export interface IntegrationTestResponse {
  status: string;
  message: string;
}

export interface IntegrationSyncInput {
  sync_type?: string;
}

export interface IntegrationSyncLog {
  id: string;
  integration_id: string;
  sync_type: string;
  status: string;
  records_processed: number;
  records_created: number;
  records_updated: number;
  records_failed: number;
  duration_ms: number | null;
  error_message: string | null;
  created_at: string;
}

export type SSOProtocol = 'saml2' | 'oidc';

export interface SSOConfiguration {
  id?: string;
  organization_id: string;
  protocol: SSOProtocol;
  is_enabled: boolean;
  is_enforced: boolean;
  saml_entity_id?: string | null;
  saml_sso_url?: string | null;
  saml_slo_url?: string | null;
  saml_certificate?: string | null;
  saml_name_id_format?: string | null;
  saml_attribute_mapping: Record<string, unknown>;
  oidc_issuer_url?: string | null;
  oidc_client_id?: string | null;
  oidc_client_secret_configured: boolean;
  oidc_scopes: string[];
  oidc_claim_mapping: Record<string, unknown>;
  auto_provision_users: boolean;
  default_role_id?: string | null;
  allowed_domains: string[];
  group_to_role_mapping: Record<string, unknown>;
  jit_provisioning: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface UpdateSSOConfigurationInput {
  protocol: SSOProtocol;
  is_enabled?: boolean;
  is_enforced?: boolean;
  saml_entity_id?: string;
  saml_sso_url?: string;
  saml_slo_url?: string;
  saml_certificate?: string;
  saml_name_id_format?: string;
  saml_attribute_mapping?: Record<string, unknown>;
  oidc_issuer_url?: string;
  oidc_client_id?: string;
  /** Write-only. Omit to preserve the existing encrypted secret. */
  oidc_client_secret?: string;
  oidc_scopes?: string[];
  oidc_claim_mapping?: Record<string, unknown>;
  auto_provision_users?: boolean;
  default_role_id?: string;
  allowed_domains?: string[];
  group_to_role_mapping?: Record<string, unknown>;
  jit_provisioning?: boolean;
}

export interface APIKeyRecord {
  id: string;
  organization_id: string;
  name: string;
  key_prefix: string;
  permissions: string[];
  rate_limit_per_minute: number;
  expires_at: string | null;
  last_used_at: string | null;
  is_active: boolean;
  created_at: string;
}

export interface CreateAPIKeyInput {
  name: string;
  permissions: string[];
  rate_limit: number;
  expires_at?: string;
}

export interface CreateAPIKeyResponse extends DataEnvelope<APIKeyRecord> {
  /** Returned exactly once by the create endpoint. */
  key: string;
  note: string;
}
