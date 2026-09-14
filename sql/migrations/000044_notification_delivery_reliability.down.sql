-- Rollback Migration 044: durable notification delivery reliability.

DROP FUNCTION IF EXISTS notification_due_tenants(INTEGER);

DROP INDEX IF EXISTS idx_notifications_parent;
DROP INDEX IF EXISTS idx_notifications_escalation_due;
DROP INDEX IF EXISTS idx_notifications_expired_lease;
DROP INDEX IF EXISTS idx_notifications_delivery_due;
DROP INDEX IF EXISTS uq_notifications_delivery_key;

ALTER TABLE notification_rules
    DROP CONSTRAINT IF EXISTS chk_notification_rules_escalation,
    DROP CONSTRAINT IF EXISTS chk_notification_rules_cooldown;

ALTER TABLE notifications
    DROP CONSTRAINT IF EXISTS chk_notifications_escalation_parent,
    DROP CONSTRAINT IF EXISTS chk_notifications_dead_state,
    DROP CONSTRAINT IF EXISTS chk_notifications_lease_tuple,
    DROP CONSTRAINT IF EXISTS chk_notifications_retry_counts,
    DROP CONSTRAINT IF EXISTS chk_notifications_delivery_key;

ALTER TABLE notifications
    DROP COLUMN IF EXISTS parent_notification_id,
    DROP COLUMN IF EXISTS escalated_at,
    DROP COLUMN IF EXISTS acknowledgement_due_at,
    DROP COLUMN IF EXISTS failure_code,
    DROP COLUMN IF EXISTS dead_at,
    DROP COLUMN IF EXISTS last_attempt_at,
    DROP COLUMN IF EXISTS leased_until,
    DROP COLUMN IF EXISTS lease_token,
    DROP COLUMN IF EXISTS lease_owner,
    DROP COLUMN IF EXISTS scheduled_for,
    DROP COLUMN IF EXISTS digest_frequency,
    DROP COLUMN IF EXISTS body_html,
    DROP COLUMN IF EXISTS body_text,
    DROP COLUMN IF EXISTS delivery_key,
    DROP COLUMN IF EXISTS event_id;

CREATE INDEX idx_notifications_retry
    ON notifications(status, next_retry_at)
    WHERE status IN ('pending', 'failed') AND next_retry_at IS NOT NULL;
