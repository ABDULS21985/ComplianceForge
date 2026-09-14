-- Migration 043: durable asynchronous delivery and worker coordination

-- Transactional producer outbox. Domain mutations and their message envelope
-- can be committed in one PostgreSQL transaction, then leased by any worker.
CREATE TABLE queue_outbox (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id          UUID NOT NULL UNIQUE,
    queue_name          VARCHAR(240) NOT NULL,
    tenant_id           UUID,
    message_type        VARCHAR(255) NOT NULL,
    schema_version      INTEGER NOT NULL,
    correlation_id      VARCHAR(255) NOT NULL,
    causation_id        VARCHAR(255),
    envelope            JSONB NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',
    dispatch_attempts   INTEGER NOT NULL DEFAULT 0,
    max_attempts        INTEGER NOT NULL DEFAULT 10,
    available_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner         UUID,
    lease_token         UUID,
    leased_until        TIMESTAMPTZ,
    last_error          TEXT,
    published_at        TIMESTAMPTZ,
    dead_lettered_at    TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_queue_outbox_name CHECK (
        queue_name <> '' AND queue_name = BTRIM(queue_name)
    ),
    CONSTRAINT chk_queue_outbox_message_type CHECK (
        message_type <> '' AND message_type = BTRIM(message_type)
    ),
    CONSTRAINT chk_queue_outbox_schema_version CHECK (schema_version > 0),
    CONSTRAINT chk_queue_outbox_envelope CHECK (
        JSONB_TYPEOF(envelope) = 'object'
        AND envelope ->> 'id' = message_id::TEXT
        AND envelope ->> 'type' = message_type
        AND (envelope ->> 'schema_version')::INTEGER = schema_version
        AND COALESCE(envelope ->> 'tenant_id', '') = COALESCE(tenant_id::TEXT, '')
        AND envelope ->> 'correlation_id' = correlation_id
        AND COALESCE(envelope ->> 'causation_id', '') = COALESCE(causation_id, '')
        AND (envelope ->> 'attempt')::INTEGER > 0
    ),
    CONSTRAINT chk_queue_outbox_status CHECK (
        status IN ('pending', 'leased', 'published', 'dead')
    ),
    CONSTRAINT chk_queue_outbox_attempts CHECK (
        dispatch_attempts >= 0 AND max_attempts > 0
        AND dispatch_attempts <= max_attempts
    ),
    CONSTRAINT chk_queue_outbox_lease CHECK (
        (status = 'leased' AND lease_owner IS NOT NULL AND lease_token IS NOT NULL AND leased_until IS NOT NULL)
        OR
        (status <> 'leased' AND lease_owner IS NULL AND lease_token IS NULL AND leased_until IS NULL)
    ),
    CONSTRAINT chk_queue_outbox_terminal_timestamps CHECK (
        (status = 'published' AND published_at IS NOT NULL AND dead_lettered_at IS NULL)
        OR (status = 'dead' AND dead_lettered_at IS NOT NULL AND published_at IS NULL)
        OR (status IN ('pending', 'leased') AND published_at IS NULL AND dead_lettered_at IS NULL)
    )
);

CREATE INDEX idx_queue_outbox_pending
    ON queue_outbox(available_at, created_at, id)
    WHERE status = 'pending';

CREATE INDEX idx_queue_outbox_expired_leases
    ON queue_outbox(leased_until, created_at, id)
    WHERE status = 'leased';

CREATE INDEX idx_queue_outbox_tenant_created
    ON queue_outbox(tenant_id, created_at DESC)
    WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_queue_outbox_terminal_retention
    ON queue_outbox(status, published_at, dead_lettered_at)
    WHERE status IN ('published', 'dead');

COMMENT ON TABLE queue_outbox IS
    'Transactional message outbox leased with FOR UPDATE SKIP LOCKED and published with RabbitMQ confirms.';
COMMENT ON COLUMN queue_outbox.dispatch_attempts IS
    'Number of PostgreSQL-to-broker dispatch leases, independent of consumer delivery attempts in the envelope.';

-- Durable consumer inbox. The compound key permits an intentional retry
-- attempt while suppressing duplicate delivery of that same attempt.
CREATE TABLE queue_inbox (
    consumer_name       VARCHAR(255) NOT NULL,
    message_id          UUID NOT NULL,
    delivery_attempt    INTEGER NOT NULL,
    tenant_id           UUID,
    message_type        VARCHAR(255) NOT NULL,
    envelope_hash       BYTEA NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'processing',
    lease_owner         UUID,
    leased_until        TIMESTAMPTZ,
    received_count      INTEGER NOT NULL DEFAULT 1,
    first_received_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_received_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ,
    last_error          TEXT,

    PRIMARY KEY (consumer_name, message_id, delivery_attempt),
    CONSTRAINT chk_queue_inbox_consumer CHECK (
        consumer_name <> '' AND consumer_name = BTRIM(consumer_name)
    ),
    CONSTRAINT chk_queue_inbox_attempt CHECK (delivery_attempt > 0),
    CONSTRAINT chk_queue_inbox_status CHECK (status IN ('processing', 'completed')),
    CONSTRAINT chk_queue_inbox_envelope_hash CHECK (OCTET_LENGTH(envelope_hash) = 32),
    CONSTRAINT chk_queue_inbox_received_count CHECK (received_count > 0),
    CONSTRAINT chk_queue_inbox_lease CHECK (
        (status = 'processing' AND lease_owner IS NOT NULL AND leased_until IS NOT NULL AND completed_at IS NULL AND expires_at IS NULL)
        OR
        (status = 'completed' AND lease_owner IS NULL AND leased_until IS NULL AND completed_at IS NOT NULL AND expires_at IS NOT NULL)
    )
);

CREATE INDEX idx_queue_inbox_expired_leases
    ON queue_inbox(leased_until)
    WHERE status = 'processing';

CREATE INDEX idx_queue_inbox_retention
    ON queue_inbox(expires_at)
    WHERE status = 'completed';

CREATE INDEX idx_queue_inbox_tenant_received
    ON queue_inbox(tenant_id, last_received_at DESC)
    WHERE tenant_id IS NOT NULL;

COMMENT ON TABLE queue_inbox IS
    'Durable consumer idempotency and processing leases keyed by consumer, logical message, and explicit delivery attempt.';
COMMENT ON COLUMN queue_inbox.envelope_hash IS
    'SHA-256 of the canonical envelope, preventing a reused idempotency key from hiding changed payload or metadata.';

-- Cross-replica leases for periodic worker tasks. Rows are retained after a
-- release to preserve operational history and are atomically reacquirable.
CREATE TABLE scheduler_leases (
    task_name           VARCHAR(255) PRIMARY KEY,
    lease_owner         UUID,
    lease_token         UUID,
    leased_until        TIMESTAMPTZ,
    acquired_at         TIMESTAMPTZ,
    heartbeat_at        TIMESTAMPTZ,
    last_owner          UUID,
    last_started_at     TIMESTAMPTZ,
    last_completed_at   TIMESTAMPTZ,
    last_error          TEXT,
    run_count           BIGINT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_scheduler_lease_name CHECK (
        task_name <> '' AND task_name = BTRIM(task_name)
    ),
    CONSTRAINT chk_scheduler_lease_state CHECK (
        (lease_owner IS NULL AND lease_token IS NULL AND leased_until IS NULL AND acquired_at IS NULL AND heartbeat_at IS NULL)
        OR
        (lease_owner IS NOT NULL AND lease_token IS NOT NULL AND leased_until IS NOT NULL AND acquired_at IS NOT NULL AND heartbeat_at IS NOT NULL)
    ),
    CONSTRAINT chk_scheduler_lease_runs CHECK (run_count >= 0)
);

CREATE INDEX idx_scheduler_leases_expiration
    ON scheduler_leases(leased_until)
    WHERE lease_owner IS NOT NULL;

COMMENT ON TABLE scheduler_leases IS
    'Renewable cross-replica leases ensuring each periodic task has at most one active worker owner.';
