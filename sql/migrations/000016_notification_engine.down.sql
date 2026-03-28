-- Rollback Migration 016: Drop notification engine tables and types.

-- Drop RLS policies
DROP POLICY IF EXISTS notif_prefs_tenant_select ON notification_preferences;
DROP POLICY IF EXISTS notif_prefs_tenant_insert ON notification_preferences;
DROP POLICY IF EXISTS notif_prefs_tenant_update ON notification_preferences;
DROP POLICY IF EXISTS notif_prefs_tenant_delete ON notification_preferences;

DROP POLICY IF EXISTS notifications_tenant_select ON notifications;
DROP POLICY IF EXISTS notifications_tenant_insert ON notifications;
DROP POLICY IF EXISTS notifications_tenant_update ON notifications;
DROP POLICY IF EXISTS notifications_tenant_delete ON notifications;

DROP POLICY IF EXISTS notif_rules_tenant_select ON notification_rules;
DROP POLICY IF EXISTS notif_rules_tenant_insert ON notification_rules;
DROP POLICY IF EXISTS notif_rules_tenant_update ON notification_rules;
DROP POLICY IF EXISTS notif_rules_tenant_delete ON notification_rules;

DROP POLICY IF EXISTS notif_templates_tenant_select ON notification_templates;
DROP POLICY IF EXISTS notif_templates_tenant_insert ON notification_templates;
DROP POLICY IF EXISTS notif_templates_tenant_update ON notification_templates;
DROP POLICY IF EXISTS notif_templates_tenant_delete ON notification_templates;

DROP POLICY IF EXISTS notif_channels_tenant_select ON notification_channels;
DROP POLICY IF EXISTS notif_channels_tenant_insert ON notification_channels;
DROP POLICY IF EXISTS notif_channels_tenant_update ON notification_channels;
DROP POLICY IF EXISTS notif_channels_tenant_delete ON notification_channels;

-- Drop triggers
DROP TRIGGER IF EXISTS trg_notification_preferences_updated_at ON notification_preferences;
DROP TRIGGER IF EXISTS trg_notification_rules_updated_at ON notification_rules;
DROP TRIGGER IF EXISTS trg_notification_templates_updated_at ON notification_templates;
DROP TRIGGER IF EXISTS trg_notification_channels_updated_at ON notification_channels;

-- Drop tables in dependency order
DROP TABLE IF EXISTS notification_preferences CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
DROP TABLE IF EXISTS notification_rules CASCADE;
DROP TABLE IF EXISTS notification_templates CASCADE;
DROP TABLE IF EXISTS notification_channels CASCADE;

-- Drop enum types
DROP TYPE IF EXISTS digest_frequency;
DROP TYPE IF EXISTS notification_recipient_type;
DROP TYPE IF EXISTS notification_status;
DROP TYPE IF EXISTS notification_channel_type;
