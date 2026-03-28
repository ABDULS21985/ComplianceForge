-- Migration 022: Integration Hub
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - Integrations table is the central registry for all external system connections:
--     SSO/IdP, cloud providers, SIEMs, ITSM tools, notifications, and webhooks.
--   - Configuration is stored as AES-256-GCM encrypted text. The application layer
--     handles encryption/decryption; the DB never sees plaintext secrets.
--   - SSO configuration is a dedicated table (one per org) because it controls the
--     authentication flow and has complex protocol-specific fields for SAML2 and OIDC.
--   - API keys store only a hash — the raw key is shown once at creation time and
--     never persisted. The key_prefix (e.g., "cf_live_") enables lookup without
--     scanning every hash.
--   - Sync logs are append-only operational records. They support troubleshooting
--     integration failures and demonstrating data-flow compliance (ISO 27001 A.13).
--   - All tables are tenant-scoped with RLS via organization_id.

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

CREATE TYPE integration_type AS ENUM (
    'sso_saml',
    'sso_oidc',
    'cloud_aws',
    'cloud_azure',
    'cloud_gcp',
    'siem_splunk',
    'siem_elastic',
    'siem_sentinel',
    'itsm_servicenow',
    'itsm_jira',
    'itsm_freshservice',
    'email_smtp',
    'email_sendgrid',
    'slack',
    'teams',
    'webhook_inbound',
    'webhook_outbound',
    'custom_api'
);

CREATE TYPE integration_status AS ENUM (
    'active',
    'inactive',
    'error',
    'pending_setup'
);

CREATE TYPE integration_health AS ENUM (
    'healthy',
    'degraded',
    'unhealthy',
    'unknown'
);

CREATE TYPE sync_status AS ENUM (
    'started',
    'completed',
    'failed',
    'partial'
);

CREATE TYPE sso_protocol AS ENUM (
    'saml2',
    'oidc'
);

-- ============================================================================
-- TABLE: integrations
-- ============================================================================

CREATE TABLE integrations (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    integration_type         integration_type NOT NULL,
    name                     VARCHAR(200) NOT NULL,
    description              TEXT,
    status                   integration_status NOT NULL DEFAULT 'pending_setup',
    configuration_encrypted  TEXT NOT NULL,                -- AES-256-GCM ciphertext; app-layer encrypt/decrypt
    health_status            integration_health NOT NULL DEFAULT 'unknown',
    last_health_check_at     TIMESTAMPTZ,
    last_sync_at             TIMESTAMPTZ,
    sync_frequency_minutes   INT NOT NULL DEFAULT 0,       -- 0 = manual/event-driven only
    error_count              INT NOT NULL DEFAULT 0,
    last_error_message       TEXT,
    capabilities             TEXT[],                        -- e.g., '{read_assets,write_tickets,sync_users}'
    created_by               UUID NOT NULL REFERENCES users(id),
    metadata                 JSONB NOT NULL DEFAULT '{}',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_integrations_org ON integrations(organization_id);
CREATE INDEX idx_integrations_org_type ON integrations(organization_id, integration_type);
CREATE INDEX idx_integrations_org_status ON integrations(organization_id, status);
CREATE INDEX idx_integrations_health ON integrations(organization_id, health_status)
    WHERE health_status IN ('degraded', 'unhealthy');

-- Trigger
CREATE TRIGGER trg_integrations_updated_at
    BEFORE UPDATE ON integrations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE integrations IS 'Central registry of all external system integrations (SSO, cloud, SIEM, ITSM, notifications, webhooks).';
COMMENT ON COLUMN integrations.configuration_encrypted IS 'AES-256-GCM encrypted JSON blob containing connection credentials and settings. Never stored in plaintext.';
COMMENT ON COLUMN integrations.sync_frequency_minutes IS '0 means manual or event-driven sync only; positive values trigger scheduled sync.';
COMMENT ON COLUMN integrations.capabilities IS 'Declared capabilities of this integration instance, e.g. read_assets, write_tickets.';

-- ============================================================================
-- TABLE: integration_sync_logs
-- ============================================================================

CREATE TABLE integration_sync_logs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    integration_id      UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    sync_type           VARCHAR(100) NOT NULL,             -- e.g., 'full', 'incremental', 'assets', 'users'
    status              sync_status NOT NULL,
    records_processed   INT NOT NULL DEFAULT 0,
    records_created     INT NOT NULL DEFAULT 0,
    records_updated     INT NOT NULL DEFAULT 0,
    records_failed      INT NOT NULL DEFAULT 0,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    duration_ms         INT,
    error_message       TEXT,
    details             JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_sync_logs_integration_created ON integration_sync_logs(integration_id, created_at DESC);
CREATE INDEX idx_sync_logs_org_status ON integration_sync_logs(organization_id, status);

COMMENT ON TABLE integration_sync_logs IS 'Append-only log of integration sync operations. Supports troubleshooting and data-flow compliance auditing.';
COMMENT ON COLUMN integration_sync_logs.sync_type IS 'Describes the sync scope: full, incremental, or a specific entity type like assets or users.';
COMMENT ON COLUMN integration_sync_logs.duration_ms IS 'Wall-clock duration of the sync in milliseconds. Computed as completed_at - started_at.';

-- ============================================================================
-- TABLE: sso_configurations
-- ============================================================================

CREATE TABLE sso_configurations (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    protocol                    sso_protocol NOT NULL,
    is_enabled                  BOOLEAN NOT NULL DEFAULT false,
    is_enforced                 BOOLEAN NOT NULL DEFAULT false,   -- when true, password login is disabled

    -- SAML 2.0 fields
    saml_entity_id              VARCHAR(500),
    saml_sso_url                VARCHAR(2000),
    saml_slo_url                VARCHAR(2000),
    saml_certificate            TEXT,                              -- IdP X.509 signing certificate (PEM)
    saml_name_id_format         VARCHAR(200),                      -- e.g., 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress'
    saml_attribute_mapping      JSONB,                             -- maps IdP attributes → user fields

    -- OIDC fields
    oidc_issuer_url             VARCHAR(2000),
    oidc_client_id              VARCHAR(500),
    oidc_client_secret_encrypted TEXT,                              -- AES-256-GCM encrypted
    oidc_scopes                 TEXT[] NOT NULL DEFAULT '{openid,profile,email}',
    oidc_claim_mapping          JSONB,                             -- maps OIDC claims → user fields

    -- Common SSO settings
    auto_provision_users        BOOLEAN NOT NULL DEFAULT true,     -- create user on first SSO login
    default_role_id             UUID REFERENCES roles(id),         -- role assigned to auto-provisioned users
    allowed_domains             TEXT[],                             -- restrict SSO to these email domains
    group_to_role_mapping       JSONB,                             -- maps IdP groups → platform roles
    jit_provisioning            BOOLEAN NOT NULL DEFAULT true,     -- just-in-time provisioning

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_sso_config_org ON sso_configurations(organization_id);

-- Trigger
CREATE TRIGGER trg_sso_configurations_updated_at
    BEFORE UPDATE ON sso_configurations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE sso_configurations IS 'Per-organization SSO/IdP configuration. Supports SAML 2.0 and OpenID Connect protocols.';
COMMENT ON COLUMN sso_configurations.is_enforced IS 'When true, password-based login is disabled for all non-super-admin users in the organization.';
COMMENT ON COLUMN sso_configurations.oidc_client_secret_encrypted IS 'AES-256-GCM encrypted OIDC client secret. Never stored in plaintext.';
COMMENT ON COLUMN sso_configurations.jit_provisioning IS 'Just-in-time provisioning: create or update user profile attributes on each SSO login.';

-- ============================================================================
-- TABLE: api_keys
-- ============================================================================

CREATE TABLE api_keys (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                   VARCHAR(200) NOT NULL,
    key_prefix             VARCHAR(10) NOT NULL,             -- e.g., 'cf_live_ab' — enables quick lookup
    key_hash               VARCHAR(128) NOT NULL,            -- SHA-256 hash of the full key
    permissions            TEXT[],                            -- e.g., '{read:risks,write:controls,read:policies}'
    rate_limit_per_minute  INT NOT NULL DEFAULT 60,
    expires_at             TIMESTAMPTZ,
    last_used_at           TIMESTAMPTZ,
    last_used_ip           VARCHAR(45),                      -- IPv4 or IPv6
    is_active              BOOLEAN NOT NULL DEFAULT true,
    created_by             UUID NOT NULL REFERENCES users(id),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_api_keys_prefix UNIQUE (key_prefix)
);

-- Indexes
CREATE INDEX idx_api_keys_org ON api_keys(organization_id);
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_active ON api_keys(organization_id, is_active) WHERE is_active = true;

-- Trigger
CREATE TRIGGER trg_api_keys_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE api_keys IS 'API keys for programmatic access. Only the hash is stored; the raw key is shown once at creation.';
COMMENT ON COLUMN api_keys.key_prefix IS 'Short prefix of the key used for identification and fast lookup without exposing the full key.';
COMMENT ON COLUMN api_keys.key_hash IS 'SHA-256 hash of the full API key. Used for authentication lookups.';
COMMENT ON COLUMN api_keys.permissions IS 'Scoped permissions granted to this key, e.g. read:risks, write:controls.';

-- ============================================================================
-- ROW-LEVEL SECURITY
-- ============================================================================

-- Enable RLS on all integration hub tables
ALTER TABLE integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE integrations FORCE ROW LEVEL SECURITY;

ALTER TABLE integration_sync_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE integration_sync_logs FORCE ROW LEVEL SECURITY;

ALTER TABLE sso_configurations ENABLE ROW LEVEL SECURITY;
ALTER TABLE sso_configurations FORCE ROW LEVEL SECURITY;

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys FORCE ROW LEVEL SECURITY;

-- integrations
CREATE POLICY integrations_tenant_isolation_select
    ON integrations FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY integrations_tenant_isolation_insert
    ON integrations FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY integrations_tenant_isolation_update
    ON integrations FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY integrations_tenant_isolation_delete
    ON integrations FOR DELETE
    USING (organization_id = get_current_tenant());

-- integration_sync_logs
CREATE POLICY sync_logs_tenant_isolation_select
    ON integration_sync_logs FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY sync_logs_tenant_isolation_insert
    ON integration_sync_logs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY sync_logs_tenant_isolation_update
    ON integration_sync_logs FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY sync_logs_tenant_isolation_delete
    ON integration_sync_logs FOR DELETE
    USING (organization_id = get_current_tenant());

-- sso_configurations
CREATE POLICY sso_config_tenant_isolation_select
    ON sso_configurations FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY sso_config_tenant_isolation_insert
    ON sso_configurations FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY sso_config_tenant_isolation_update
    ON sso_configurations FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY sso_config_tenant_isolation_delete
    ON sso_configurations FOR DELETE
    USING (organization_id = get_current_tenant());

-- api_keys
CREATE POLICY api_keys_tenant_isolation_select
    ON api_keys FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY api_keys_tenant_isolation_insert
    ON api_keys FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY api_keys_tenant_isolation_update
    ON api_keys FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());

CREATE POLICY api_keys_tenant_isolation_delete
    ON api_keys FOR DELETE
    USING (organization_id = get_current_tenant());
