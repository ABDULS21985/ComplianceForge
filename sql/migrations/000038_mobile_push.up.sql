-- Migration 038: Mobile & Push Notifications (Prompt 34)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - push_notification_tokens stores device registration tokens for iOS (APNs),
--     Android (FCM), and Web Push. Token hash provides fast lookup without
--     exposing raw tokens in queries. Multiple devices per user supported.
--   - push_notification_log is an append-only delivery log for debugging,
--     analytics, and retry logic. Tracks sent/delivered/failed status.
--   - user_mobile_preferences gives users fine-grained control over which
--     notification types trigger push, plus quiet hours with critical override.
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: push_notification_tokens
-- ============================================================================

CREATE TABLE push_notification_tokens (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    platform                    VARCHAR(10) NOT NULL
                                CHECK (platform IN ('ios', 'android', 'web')),
    token                       TEXT NOT NULL,
    token_hash                  VARCHAR(128) UNIQUE,
    device_name                 VARCHAR(200),
    device_model                VARCHAR(100),
    os_version                  VARCHAR(50),
    app_version                 VARCHAR(20),
    is_active                   BOOLEAN NOT NULL DEFAULT true,
    last_used_at                TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_push_tokens_org ON push_notification_tokens(organization_id);
CREATE INDEX idx_push_tokens_user ON push_notification_tokens(user_id);
CREATE INDEX idx_push_tokens_org_user ON push_notification_tokens(organization_id, user_id);
CREATE INDEX idx_push_tokens_platform ON push_notification_tokens(platform);
CREATE INDEX idx_push_tokens_active ON push_notification_tokens(user_id, is_active) WHERE is_active = true;
CREATE INDEX idx_push_tokens_hash ON push_notification_tokens(token_hash) WHERE token_hash IS NOT NULL;
CREATE INDEX idx_push_tokens_last_used ON push_notification_tokens(last_used_at DESC) WHERE last_used_at IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_push_notification_tokens_updated_at
    BEFORE UPDATE ON push_notification_tokens
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE push_notification_tokens IS 'Device push notification registration tokens for iOS (APNs), Android (FCM), and Web Push. Multiple devices per user supported.';
COMMENT ON COLUMN push_notification_tokens.token_hash IS 'SHA-256 hash of the push token for fast deduplication lookups.';
COMMENT ON COLUMN push_notification_tokens.is_active IS 'Deactivated when token is invalidated by the push service or user logs out.';

-- ============================================================================
-- TABLE: push_notification_log
-- ============================================================================

CREATE TABLE push_notification_log (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_id                    UUID REFERENCES push_notification_tokens(id) ON DELETE SET NULL,
    notification_type           VARCHAR(100) NOT NULL,
    title                       VARCHAR(300) NOT NULL,
    body                        TEXT,
    data                        JSONB,
    status                      VARCHAR(20) NOT NULL
                                CHECK (status IN ('sent', 'delivered', 'failed', 'invalid_token')),
    platform                    VARCHAR(10)
                                CHECK (platform IS NULL OR platform IN ('ios', 'android', 'web')),
    sent_at                     TIMESTAMPTZ,
    error_message               TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_push_log_org ON push_notification_log(organization_id);
CREATE INDEX idx_push_log_user ON push_notification_log(user_id);
CREATE INDEX idx_push_log_org_user ON push_notification_log(organization_id, user_id);
CREATE INDEX idx_push_log_token ON push_notification_log(token_id) WHERE token_id IS NOT NULL;
CREATE INDEX idx_push_log_type ON push_notification_log(notification_type);
CREATE INDEX idx_push_log_status ON push_notification_log(status);
CREATE INDEX idx_push_log_org_status ON push_notification_log(organization_id, status);
CREATE INDEX idx_push_log_sent ON push_notification_log(sent_at DESC) WHERE sent_at IS NOT NULL;
CREATE INDEX idx_push_log_created ON push_notification_log(created_at DESC);
CREATE INDEX idx_push_log_failed ON push_notification_log(organization_id, status, created_at DESC) WHERE status IN ('failed', 'invalid_token');
CREATE INDEX idx_push_log_platform ON push_notification_log(platform) WHERE platform IS NOT NULL;

COMMENT ON TABLE push_notification_log IS 'Append-only delivery log for push notifications. Tracks send status, errors, and delivery for debugging and analytics.';
COMMENT ON COLUMN push_notification_log.notification_type IS 'Notification category: "breach_alert", "approval_request", "incident_alert", "deadline_reminder", "mention", "comment", etc.';
COMMENT ON COLUMN push_notification_log.data IS 'JSONB payload sent with the push notification for deep-linking: {"entity_type": "incident", "entity_id": "uuid", "action": "view"}.';
COMMENT ON COLUMN push_notification_log.status IS 'Delivery status: sent (accepted by push service), delivered (confirmed), failed (error), invalid_token (token revoked).';

-- ============================================================================
-- TABLE: user_mobile_preferences
-- ============================================================================

CREATE TABLE user_mobile_preferences (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    push_enabled                BOOLEAN NOT NULL DEFAULT true,
    push_breach_alerts          BOOLEAN NOT NULL DEFAULT true,
    push_approval_requests      BOOLEAN NOT NULL DEFAULT true,
    push_incident_alerts        BOOLEAN NOT NULL DEFAULT true,
    push_deadline_reminders     BOOLEAN NOT NULL DEFAULT true,
    push_mentions               BOOLEAN NOT NULL DEFAULT true,
    push_comments               BOOLEAN NOT NULL DEFAULT false,
    quiet_hours_enabled         BOOLEAN NOT NULL DEFAULT false,
    quiet_hours_start           TIME NOT NULL DEFAULT '22:00',
    quiet_hours_end             TIME NOT NULL DEFAULT '07:00',
    quiet_hours_timezone        VARCHAR(50),
    quiet_hours_override_critical BOOLEAN NOT NULL DEFAULT true,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_user_mobile_preferences_user UNIQUE (user_id)
);

-- Indexes
CREATE INDEX idx_mobile_prefs_org ON user_mobile_preferences(organization_id);
CREATE INDEX idx_mobile_prefs_user ON user_mobile_preferences(user_id);
CREATE INDEX idx_mobile_prefs_push_enabled ON user_mobile_preferences(user_id, push_enabled) WHERE push_enabled = true;
CREATE INDEX idx_mobile_prefs_quiet_hours ON user_mobile_preferences(quiet_hours_enabled) WHERE quiet_hours_enabled = true;

-- Trigger
CREATE TRIGGER trg_user_mobile_preferences_updated_at
    BEFORE UPDATE ON user_mobile_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE user_mobile_preferences IS 'Per-user mobile push notification preferences with fine-grained type controls and quiet hours.';
COMMENT ON COLUMN user_mobile_preferences.quiet_hours_override_critical IS 'When true, critical notifications (breach alerts, major incidents) bypass quiet hours.';
COMMENT ON COLUMN user_mobile_preferences.quiet_hours_timezone IS 'Timezone for quiet hours evaluation. Falls back to user profile timezone if NULL.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- push_notification_tokens
ALTER TABLE push_notification_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE push_notification_tokens FORCE ROW LEVEL SECURITY;

CREATE POLICY push_notification_tokens_tenant_select ON push_notification_tokens FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY push_notification_tokens_tenant_insert ON push_notification_tokens FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY push_notification_tokens_tenant_update ON push_notification_tokens FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY push_notification_tokens_tenant_delete ON push_notification_tokens FOR DELETE
    USING (organization_id = get_current_tenant());

-- push_notification_log
ALTER TABLE push_notification_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE push_notification_log FORCE ROW LEVEL SECURITY;

CREATE POLICY push_notification_log_tenant_select ON push_notification_log FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY push_notification_log_tenant_insert ON push_notification_log FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY push_notification_log_tenant_update ON push_notification_log FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY push_notification_log_tenant_delete ON push_notification_log FOR DELETE
    USING (organization_id = get_current_tenant());

-- user_mobile_preferences
ALTER TABLE user_mobile_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_mobile_preferences FORCE ROW LEVEL SECURITY;

CREATE POLICY user_mobile_preferences_tenant_select ON user_mobile_preferences FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY user_mobile_preferences_tenant_insert ON user_mobile_preferences FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY user_mobile_preferences_tenant_update ON user_mobile_preferences FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY user_mobile_preferences_tenant_delete ON user_mobile_preferences FOR DELETE
    USING (organization_id = get_current_tenant());
