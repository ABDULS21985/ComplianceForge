-- Migration 044: durable notification delivery reliability
--
-- Notifications remain tenant-owned and FORCE RLS protected.  The worker uses
-- notification_due_tenants() only to discover which tenant context to enter;
-- all notification content, claims, state transitions and channel lookups are
-- performed after app.current_tenant has been set.

ALTER TABLE notifications
    ADD COLUMN event_id UUID,
    ADD COLUMN delivery_key CHAR(64),
    ADD COLUMN body_text TEXT,
    ADD COLUMN body_html TEXT,
    ADD COLUMN digest_frequency digest_frequency NOT NULL DEFAULT 'immediate',
    ADD COLUMN scheduled_for TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN lease_owner UUID,
    ADD COLUMN lease_token UUID,
    ADD COLUMN leased_until TIMESTAMPTZ,
    ADD COLUMN last_attempt_at TIMESTAMPTZ,
    ADD COLUMN dead_at TIMESTAMPTZ,
    ADD COLUMN failure_code VARCHAR(64),
    ADD COLUMN acknowledgement_due_at TIMESTAMPTZ,
    ADD COLUMN escalated_at TIMESTAMPTZ,
    ADD COLUMN parent_notification_id UUID REFERENCES notifications(id) ON DELETE SET NULL;

-- Existing rows predate event idempotency.  Give each of them a collision-free
-- key so the uniqueness constraint can be introduced without rewriting their
-- historical event payloads.
UPDATE notifications
SET max_retries = GREATEST(max_retries, 1),
    retry_count = GREATEST(retry_count, 0);

UPDATE notifications
SET retry_count = LEAST(retry_count, max_retries);

UPDATE notifications
SET delivery_key = encode(digest(id::TEXT, 'sha256'), 'hex'),
    body_text = CASE WHEN channel_type <> 'email' THEN body ELSE NULL END,
    body_html = CASE WHEN channel_type = 'email' THEN body ELSE NULL END,
    scheduled_for = COALESCE(next_retry_at, created_at),
    status = CASE
        WHEN retry_count >= max_retries AND status = 'pending' THEN 'failed'::notification_status
        ELSE status
    END,
    dead_at = CASE
        WHEN retry_count >= max_retries AND status IN ('pending', 'failed', 'bounced') THEN NOW()
        ELSE NULL
    END;

-- Historical synchronous delivery stored raw provider errors, which may
-- contain recipient addresses, endpoint URLs or transport credentials. Keep
-- the failure signal while removing unsafe legacy detail.
UPDATE notifications
SET failure_code = 'legacy_failure',
    error_message = 'legacy notification delivery failure (redacted)'
WHERE error_message IS NOT NULL;

ALTER TABLE notifications
    ALTER COLUMN delivery_key SET NOT NULL,
    ADD CONSTRAINT chk_notifications_delivery_key
        CHECK (delivery_key ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT chk_notifications_retry_counts
        CHECK (retry_count >= 0 AND max_retries > 0 AND retry_count <= max_retries),
    ADD CONSTRAINT chk_notifications_lease_tuple
        CHECK (
            (lease_owner IS NULL AND lease_token IS NULL AND leased_until IS NULL)
            OR
            (lease_owner IS NOT NULL AND lease_token IS NOT NULL AND leased_until IS NOT NULL)
        ),
    ADD CONSTRAINT chk_notifications_dead_state
        CHECK (dead_at IS NULL OR status IN ('failed', 'bounced')),
    ADD CONSTRAINT chk_notifications_escalation_parent
        CHECK (parent_notification_id IS NULL OR parent_notification_id <> id);

-- Repair historically unconstrained escalation settings before enforcing the
-- pair invariant. Incomplete configurations are disabled rather than guessed.
UPDATE notification_rules
SET cooldown_minutes = LEAST(GREATEST(cooldown_minutes, 0), 525600),
    escalation_after_minutes = NULL,
    escalation_channel_ids = NULL
WHERE escalation_after_minutes IS NULL
   OR escalation_after_minutes <= 0
   OR escalation_after_minutes > 525600
   OR cardinality(COALESCE(escalation_channel_ids, '{}'::uuid[])) = 0;

UPDATE notification_rules
SET cooldown_minutes = LEAST(GREATEST(cooldown_minutes, 0), 525600)
WHERE cooldown_minutes NOT BETWEEN 0 AND 525600;

ALTER TABLE notification_rules
    ADD CONSTRAINT chk_notification_rules_cooldown
		CHECK (cooldown_minutes BETWEEN 0 AND 525600),
    ADD CONSTRAINT chk_notification_rules_escalation
        CHECK (
            (escalation_after_minutes IS NULL AND cardinality(COALESCE(escalation_channel_ids, '{}'::uuid[])) = 0)
            OR
			(escalation_after_minutes BETWEEN 1 AND 525600
			 AND cardinality(COALESCE(escalation_channel_ids, '{}'::uuid[])) > 0)
        );

CREATE UNIQUE INDEX uq_notifications_delivery_key
    ON notifications(organization_id, delivery_key);

DROP INDEX IF EXISTS idx_notifications_retry;

CREATE INDEX idx_notifications_delivery_due
    ON notifications(organization_id, COALESCE(next_retry_at, scheduled_for), created_at)
    WHERE status IN ('pending', 'failed') AND dead_at IS NULL;

CREATE INDEX idx_notifications_expired_lease
    ON notifications(organization_id, leased_until)
    WHERE leased_until IS NOT NULL AND dead_at IS NULL;

CREATE INDEX idx_notifications_escalation_due
    ON notifications(organization_id, acknowledgement_due_at, created_at)
    WHERE acknowledgement_due_at IS NOT NULL
      AND acknowledged_at IS NULL
      AND escalated_at IS NULL
      AND parent_notification_id IS NULL;

CREATE INDEX idx_notifications_parent
    ON notifications(parent_notification_id)
    WHERE parent_notification_id IS NOT NULL;

COMMENT ON COLUMN notifications.delivery_key IS
    'SHA-256 idempotency key for one logical event/rule/recipient/channel delivery.';
COMMENT ON COLUMN notifications.scheduled_for IS
    'Earliest delivery time after digest-window and quiet-hours calculation.';
COMMENT ON COLUMN notifications.dead_at IS
    'Terminal retry exhaustion timestamp. Status remains failed/bounced for backwards compatibility.';
COMMENT ON COLUMN notifications.failure_code IS
    'Bounded non-sensitive failure classification; error_message contains only a redacted summary.';
COMMENT ON COLUMN notifications.lease_token IS
    'Per-claim fencing token. Completion is accepted only from the current lease owner and token.';

-- Tenant discovery is deliberately narrow: it returns UUIDs only, never
-- notification content, recipient data, channel configuration or counts.  A
-- caller must subsequently establish tenant context and pass normal FORCE RLS.
CREATE FUNCTION notification_due_tenants(requested_limit INTEGER DEFAULT 100)
RETURNS TABLE (organization_id UUID)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT due.organization_id
    FROM (
        SELECT n.organization_id,
               MIN(COALESCE(n.next_retry_at, n.scheduled_for)) AS due_at
        FROM public.notifications AS n
        WHERE n.status IN ('pending', 'failed')
          AND n.dead_at IS NULL
          AND n.retry_count < n.max_retries
          AND COALESCE(n.next_retry_at, n.scheduled_for) <= statement_timestamp()
          AND (n.leased_until IS NULL OR n.leased_until <= statement_timestamp())
        GROUP BY n.organization_id

        UNION ALL

        SELECT n.organization_id,
               MIN(n.acknowledgement_due_at) AS due_at
        FROM public.notifications AS n
        WHERE n.parent_notification_id IS NULL
          AND n.status IN ('sent', 'delivered')
          AND n.acknowledgement_due_at IS NOT NULL
          AND n.acknowledgement_due_at <= statement_timestamp()
          AND n.acknowledged_at IS NULL
          AND n.escalated_at IS NULL
        GROUP BY n.organization_id
    ) AS due
    GROUP BY due.organization_id
    ORDER BY MIN(due.due_at), due.organization_id
    LIMIT LEAST(GREATEST(COALESCE(requested_limit, 100), 1), 1000)
$$;

REVOKE ALL ON FUNCTION notification_due_tenants(INTEGER) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION notification_due_tenants(INTEGER) TO PUBLIC;

COMMENT ON FUNCTION notification_due_tenants(INTEGER) IS
    'Returns at most 1000 tenant UUIDs with due notification work; content remains protected by FORCE RLS.';
