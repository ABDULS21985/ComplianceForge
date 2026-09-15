-- Rollback Migration 057: remove queue row-level isolation while preserving
-- all durable messages and restoring the schema-056 runtime grant surface.

BEGIN;

DROP POLICY IF EXISTS queue_outbox_owner_all ON queue_outbox;
DROP POLICY IF EXISTS queue_outbox_scheduler_all ON queue_outbox;
DROP POLICY IF EXISTS queue_outbox_tenant_insert ON queue_outbox;
DROP POLICY IF EXISTS queue_outbox_tenant_select ON queue_outbox;

DROP POLICY IF EXISTS queue_inbox_owner_all ON queue_inbox;
DROP POLICY IF EXISTS queue_inbox_scheduler_all ON queue_inbox;
DROP POLICY IF EXISTS queue_inbox_tenant_select ON queue_inbox;

ALTER TABLE queue_outbox NO FORCE ROW LEVEL SECURITY;
ALTER TABLE queue_outbox DISABLE ROW LEVEL SECURITY;
ALTER TABLE queue_inbox NO FORCE ROW LEVEL SECURITY;
ALTER TABLE queue_inbox DISABLE ROW LEVEL SECURITY;

-- Schema 056 allowed API outbox production but did not expose the inbox.
-- Scheduler DML and all queue rows are data-preserving across this downgrade.
DO $restore_schema_056_grants$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_api'
    ) THEN
        GRANT SELECT, INSERT ON TABLE queue_outbox TO complianceforge_api;
        REVOKE UPDATE, DELETE, TRUNCATE ON TABLE queue_outbox FROM complianceforge_api;
        REVOKE ALL PRIVILEGES ON TABLE queue_inbox FROM complianceforge_api;
    END IF;

    IF EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_scheduler'
    ) THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE queue_outbox, queue_inbox
            TO complianceforge_scheduler;
        REVOKE TRUNCATE ON TABLE queue_outbox, queue_inbox
            FROM complianceforge_scheduler;
    END IF;
END;
$restore_schema_056_grants$;

REVOKE ALL PRIVILEGES ON TABLE queue_outbox, queue_inbox FROM PUBLIC;

COMMIT;
