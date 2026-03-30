-- Migration 038 DOWN: Mobile & Push Notifications
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- user_mobile_preferences
DROP POLICY IF EXISTS user_mobile_preferences_tenant_delete ON user_mobile_preferences;
DROP POLICY IF EXISTS user_mobile_preferences_tenant_update ON user_mobile_preferences;
DROP POLICY IF EXISTS user_mobile_preferences_tenant_insert ON user_mobile_preferences;
DROP POLICY IF EXISTS user_mobile_preferences_tenant_select ON user_mobile_preferences;

-- push_notification_log
DROP POLICY IF EXISTS push_notification_log_tenant_delete ON push_notification_log;
DROP POLICY IF EXISTS push_notification_log_tenant_update ON push_notification_log;
DROP POLICY IF EXISTS push_notification_log_tenant_insert ON push_notification_log;
DROP POLICY IF EXISTS push_notification_log_tenant_select ON push_notification_log;

-- push_notification_tokens
DROP POLICY IF EXISTS push_notification_tokens_tenant_delete ON push_notification_tokens;
DROP POLICY IF EXISTS push_notification_tokens_tenant_update ON push_notification_tokens;
DROP POLICY IF EXISTS push_notification_tokens_tenant_insert ON push_notification_tokens;
DROP POLICY IF EXISTS push_notification_tokens_tenant_select ON push_notification_tokens;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_user_mobile_preferences_updated_at ON user_mobile_preferences;
DROP TRIGGER IF EXISTS trg_push_notification_tokens_updated_at ON push_notification_tokens;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS user_mobile_preferences;
DROP TABLE IF EXISTS push_notification_log;
DROP TABLE IF EXISTS push_notification_tokens;
