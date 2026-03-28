-- Migration 006: Audit Log (Partitioned)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - Partitioned by RANGE on created_at (monthly) for performance. GRC platforms
--     generate massive audit trails — partitioning enables efficient retention
--     policies (drop old partitions) and faster queries on recent data.
--   - JSONB changes column stores field-level diffs: {"field": {"old": x, "new": y}}
--     This supports the compliance requirement to show exactly what changed, when,
--     and by whom (ISO 27001 A.12.4, GDPR Art. 30, NIST AU-3).
--   - No updated_at or deleted_at — audit logs are immutable by design.
--   - Separate from application audit_logs to avoid confusion: this is the
--     platform-level audit trail, not a GRC audit management table.
--   - We create 12 months of partitions upfront plus a default partition for
--     any data outside the pre-created ranges.

-- ============================================================================
-- TABLE: audit_logs (partitioned parent)
-- ============================================================================

CREATE TABLE audit_logs (
    id               UUID DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL,
    user_id          UUID,
    action           VARCHAR(50) NOT NULL,        -- CREATE, UPDATE, DELETE, LOGIN, LOGOUT, EXPORT, APPROVE, REJECT, etc.
    entity_type      VARCHAR(100) NOT NULL,        -- users, organizations, frameworks, controls, risks, policies, etc.
    entity_id        UUID,
    changes          JSONB,                        -- {"field": {"old": val, "new": val}}
    ip_address       INET,
    user_agent       TEXT,
    request_id       UUID,                         -- correlation ID from the request middleware
    metadata         JSONB DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Primary key must include the partition key.
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- ============================================================================
-- PARTITIONS: create 12 months from 2026-01 to 2026-12 + default
-- ============================================================================

CREATE TABLE audit_logs_2026_01 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE audit_logs_2026_02 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE audit_logs_2026_03 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE audit_logs_2026_04 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE audit_logs_2026_05 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE audit_logs_2026_06 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE audit_logs_2026_07 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE audit_logs_2026_08 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE audit_logs_2026_09 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE audit_logs_2026_10 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE audit_logs_2026_11 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE audit_logs_2026_12 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Default partition for data outside pre-created ranges.
-- A background job should create future partitions ahead of time.
CREATE TABLE audit_logs_default PARTITION OF audit_logs DEFAULT;

-- ============================================================================
-- INDEXES (created on the parent; PostgreSQL propagates to partitions)
-- ============================================================================

CREATE INDEX idx_audit_logs_org ON audit_logs(organization_id);
CREATE INDEX idx_audit_logs_user ON audit_logs(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_request ON audit_logs(request_id) WHERE request_id IS NOT NULL;
-- Combined index for the most common query: "show me all changes to entity X in org Y"
CREATE INDEX idx_audit_logs_org_entity ON audit_logs(organization_id, entity_type, entity_id, created_at DESC);

-- ============================================================================
-- RLS on audit_logs
-- ============================================================================

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;

CREATE POLICY audit_logs_tenant_isolation_select
    ON audit_logs FOR SELECT
    USING (organization_id = get_current_tenant());

CREATE POLICY audit_logs_tenant_isolation_insert
    ON audit_logs FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

-- No UPDATE or DELETE policies — audit logs are immutable.
-- Even with RLS, the application should not attempt to modify audit records.

COMMENT ON TABLE audit_logs IS 'Immutable, partitioned audit trail. Supports ISO 27001 A.12.4, GDPR Art. 30, NIST AU-3 requirements.';
COMMENT ON COLUMN audit_logs.changes IS 'Field-level diff: {"field_name": {"old": previous_value, "new": new_value}}';
COMMENT ON COLUMN audit_logs.request_id IS 'Correlation ID from the X-Request-ID header — links HTTP request to audit entries.';

-- ============================================================================
-- FUNCTION: create_audit_log_partition
-- Utility to create future monthly partitions (called by cron/background job).
-- ============================================================================

CREATE OR REPLACE FUNCTION create_audit_log_partition(target_date DATE)
RETURNS VOID AS $$
DECLARE
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    start_date := date_trunc('month', target_date)::DATE;
    end_date := (start_date + INTERVAL '1 month')::DATE;
    partition_name := 'audit_logs_' || to_char(start_date, 'YYYY_MM');

    -- Check if partition already exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_class WHERE relname = partition_name
    ) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF audit_logs FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        RAISE NOTICE 'Created partition: %', partition_name;
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION create_audit_log_partition(DATE) IS 'Creates a monthly partition for audit_logs. Should be called by a scheduled job to stay ahead of data.';
