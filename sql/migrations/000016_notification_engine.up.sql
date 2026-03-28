-- Migration 016: Notification Engine
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - Multi-channel notification delivery: email, in-app, webhook, Slack, Teams
--   - Template system with organization-level overrides of system defaults
--   - Rule-based routing with severity filtering, cooldown, and escalation
--   - User preferences with per-event-type granularity and quiet hours
--   - Retry logic with exponential backoff tracked per notification
--   - notification_templates with NULL organization_id are system-wide defaults
--     visible to all tenants (mirrors policy_categories pattern)
--   - Digest frequency support for batching non-urgent notifications

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

CREATE TYPE notification_channel_type AS ENUM ('email', 'in_app', 'webhook', 'slack', 'teams');
CREATE TYPE notification_status AS ENUM ('pending', 'sent', 'delivered', 'failed', 'bounced');
CREATE TYPE notification_recipient_type AS ENUM ('role', 'user', 'owner', 'assignee', 'dpo', 'ciso', 'custom');
CREATE TYPE digest_frequency AS ENUM ('immediate', 'hourly', 'daily', 'weekly');

-- ============================================================================
-- TABLE: notification_channels
-- ============================================================================

CREATE TABLE notification_channels (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_type        notification_channel_type NOT NULL,
    name                VARCHAR(200) NOT NULL,
    configuration       JSONB NOT NULL,
    is_active           BOOLEAN NOT NULL DEFAULT true,
    is_default          BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);

CREATE INDEX idx_notif_channels_org ON notification_channels(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_notif_channels_type ON notification_channels(organization_id, channel_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_notif_channels_active ON notification_channels(organization_id, is_active) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_notification_channels_updated_at
    BEFORE UPDATE ON notification_channels
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE notification_channels ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_channels FORCE ROW LEVEL SECURITY;

CREATE POLICY notif_channels_tenant_select ON notification_channels FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY notif_channels_tenant_insert ON notification_channels FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_channels_tenant_update ON notification_channels FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_channels_tenant_delete ON notification_channels FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE notification_channels IS 'Configured delivery channels per organization. Configuration JSONB holds channel-specific settings: SMTP for email, webhook URL+secret for webhook, Slack webhook+channel for Slack, etc.';

-- ============================================================================
-- TABLE: notification_templates
-- ============================================================================

CREATE TABLE notification_templates (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name                    VARCHAR(200) NOT NULL,
    event_type              VARCHAR(100) NOT NULL,
    subject_template        TEXT,
    body_html_template      TEXT,
    body_text_template      TEXT,
    in_app_title_template   TEXT,
    in_app_body_template    TEXT,
    slack_template          JSONB,
    webhook_payload_template JSONB,
    variables               TEXT[],
    is_system               BOOLEAN NOT NULL DEFAULT false,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_notif_template_org_event_name UNIQUE NULLS NOT DISTINCT (organization_id, event_type, name)
);

CREATE INDEX idx_notif_templates_org ON notification_templates(organization_id);
CREATE INDEX idx_notif_templates_event ON notification_templates(event_type);
CREATE INDEX idx_notif_templates_system ON notification_templates(is_system) WHERE is_system = true;

CREATE TRIGGER trg_notification_templates_updated_at
    BEFORE UPDATE ON notification_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE notification_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_templates FORCE ROW LEVEL SECURITY;

-- System templates (organization_id IS NULL) are visible to all tenants
CREATE POLICY notif_templates_tenant_select ON notification_templates FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY notif_templates_tenant_insert ON notification_templates FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_templates_tenant_update ON notification_templates FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_templates_tenant_delete ON notification_templates FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE notification_templates IS 'Notification templates with per-channel content. System templates (organization_id NULL) provide defaults; organizations can override with custom templates for the same event_type.';

-- ============================================================================
-- TABLE: notification_rules
-- ============================================================================

CREATE TABLE notification_rules (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                        VARCHAR(200) NOT NULL,
    event_type                  VARCHAR(100) NOT NULL,
    severity_filter             TEXT[],
    conditions                  JSONB,
    channel_ids                 UUID[],
    recipient_type              notification_recipient_type NOT NULL,
    recipient_ids               UUID[],
    template_id                 UUID REFERENCES notification_templates(id) ON DELETE SET NULL,
    is_active                   BOOLEAN NOT NULL DEFAULT true,
    cooldown_minutes            INT NOT NULL DEFAULT 0,
    escalation_after_minutes    INT,
    escalation_channel_ids      UUID[],
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notif_rules_org ON notification_rules(organization_id);
CREATE INDEX idx_notif_rules_event ON notification_rules(organization_id, event_type);
CREATE INDEX idx_notif_rules_active ON notification_rules(organization_id, is_active) WHERE is_active = true;

CREATE TRIGGER trg_notification_rules_updated_at
    BEFORE UPDATE ON notification_rules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE notification_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_rules FORCE ROW LEVEL SECURITY;

CREATE POLICY notif_rules_tenant_select ON notification_rules FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY notif_rules_tenant_insert ON notification_rules FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_rules_tenant_update ON notification_rules FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_rules_tenant_delete ON notification_rules FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE notification_rules IS 'Event-driven routing rules that determine which channels and recipients receive notifications. Supports severity filtering, JSONB conditions, cooldown periods, and escalation chains.';

-- ============================================================================
-- TABLE: notifications
-- ============================================================================

CREATE TABLE notifications (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    rule_id             UUID REFERENCES notification_rules(id) ON DELETE SET NULL,
    event_type          VARCHAR(100) NOT NULL,
    event_payload       JSONB NOT NULL,
    recipient_user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_type        notification_channel_type NOT NULL,
    channel_id          UUID REFERENCES notification_channels(id) ON DELETE SET NULL,
    subject             TEXT,
    body                TEXT,
    status              notification_status NOT NULL DEFAULT 'pending',
    sent_at             TIMESTAMPTZ,
    delivered_at        TIMESTAMPTZ,
    read_at             TIMESTAMPTZ,
    acknowledged_at     TIMESTAMPTZ,
    error_message       TEXT,
    retry_count         INT NOT NULL DEFAULT 0,
    max_retries         INT NOT NULL DEFAULT 3,
    next_retry_at       TIMESTAMPTZ,
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_org ON notifications(organization_id);
CREATE INDEX idx_notifications_recipient_status ON notifications(recipient_user_id, status);
CREATE INDEX idx_notifications_event ON notifications(organization_id, event_type);
CREATE INDEX idx_notifications_retry ON notifications(status, next_retry_at) WHERE status IN ('pending', 'failed') AND next_retry_at IS NOT NULL;
CREATE INDEX idx_notifications_unread ON notifications(recipient_user_id, read_at) WHERE read_at IS NULL;

ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;

CREATE POLICY notifications_tenant_select ON notifications FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY notifications_tenant_insert ON notifications FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notifications_tenant_update ON notifications FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notifications_tenant_delete ON notifications FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE notifications IS 'Individual notification delivery records. Tracks full lifecycle from pending through sent/delivered/read, with retry logic for failed deliveries.';

-- ============================================================================
-- TABLE: notification_preferences
-- ============================================================================

CREATE TABLE notification_preferences (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                 UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    event_type              VARCHAR(100) NOT NULL DEFAULT '*',
    email_enabled           BOOLEAN NOT NULL DEFAULT true,
    in_app_enabled          BOOLEAN NOT NULL DEFAULT true,
    slack_enabled           BOOLEAN NOT NULL DEFAULT false,
    digest_frequency        digest_frequency NOT NULL DEFAULT 'immediate',
    quiet_hours_start       TIME,
    quiet_hours_end         TIME,
    quiet_hours_timezone    VARCHAR(50),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_notif_pref_user_org_event UNIQUE (user_id, organization_id, event_type)
);

CREATE INDEX idx_notif_prefs_user ON notification_preferences(user_id);
CREATE INDEX idx_notif_prefs_org ON notification_preferences(organization_id);

CREATE TRIGGER trg_notification_preferences_updated_at
    BEFORE UPDATE ON notification_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE notification_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_preferences FORCE ROW LEVEL SECURITY;

CREATE POLICY notif_prefs_tenant_select ON notification_preferences FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY notif_prefs_tenant_insert ON notification_preferences FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_prefs_tenant_update ON notification_preferences FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY notif_prefs_tenant_delete ON notification_preferences FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE notification_preferences IS 'Per-user notification preferences scoped to organization and event type. Supports channel opt-in/out, digest batching, and quiet hours with timezone awareness.';
