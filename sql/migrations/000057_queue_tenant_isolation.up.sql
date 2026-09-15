-- Migration 057: tenant isolation for the durable queue outbox and inbox.
--
-- API transactions may produce and inspect only their current tenant's queue
-- records. Workers retain the explicitly granted cross-tenant access needed
-- to dispatch, deduplicate, complete, and purge tenant and system messages.

BEGIN;

-- PUBLIC must never supply the SQL privilege boundary for queue storage. The
-- reviewed runtime grants are applied to the fixed API/scheduler groups when
-- those groups are present; local installations without split roles continue
-- to work through the exact table-owner policies below.
REVOKE ALL PRIVILEGES ON TABLE queue_outbox, queue_inbox FROM PUBLIC;

DO $validate_runtime_groups$
DECLARE
    runtime_group RECORD;
BEGIN
    FOR runtime_group IN
        SELECT role.*
        FROM pg_catalog.pg_roles AS role
        WHERE role.rolname IN ('complianceforge_api', 'complianceforge_scheduler')
    LOOP
        IF runtime_group.rolcanlogin OR runtime_group.rolsuper
           OR runtime_group.rolbypassrls OR runtime_group.rolcreatedb
           OR runtime_group.rolcreaterole OR runtime_group.rolreplication
           OR runtime_group.rolinherit THEN
            RAISE EXCEPTION '% must be NOLOGIN, NOSUPERUSER, NOBYPASSRLS, NOCREATEDB, NOCREATEROLE, NOREPLICATION, and NOINHERIT',
                runtime_group.rolname USING ERRCODE = '42501';
        END IF;
    END LOOP;
END;
$validate_runtime_groups$;

DO $queue_runtime_grants$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_api'
    ) THEN
        GRANT USAGE ON SCHEMA public TO complianceforge_api;
        GRANT SELECT, INSERT ON TABLE queue_outbox TO complianceforge_api;
        REVOKE UPDATE, DELETE, TRUNCATE ON TABLE queue_outbox FROM complianceforge_api;
        GRANT SELECT ON TABLE queue_inbox TO complianceforge_api;
        REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON TABLE queue_inbox FROM complianceforge_api;
    END IF;

    IF EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_scheduler'
    ) THEN
        GRANT USAGE ON SCHEMA public TO complianceforge_scheduler;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE queue_outbox, queue_inbox
            TO complianceforge_scheduler;
        REVOKE TRUNCATE ON TABLE queue_outbox, queue_inbox
            FROM complianceforge_scheduler;
    END IF;
END;
$queue_runtime_grants$;

ALTER TABLE queue_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE queue_outbox FORCE ROW LEVEL SECURITY;
ALTER TABLE queue_inbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE queue_inbox FORCE ROW LEVEL SECURITY;

-- Table privileges select the API producer/diagnostic identities. These
-- PUBLIC policies are only the row boundary, matching the repository-wide
-- get_current_tenant contract and remaining valid when fixed groups are not
-- provisioned in a local bootstrap.
CREATE POLICY queue_outbox_tenant_select ON queue_outbox
    FOR SELECT
    USING (
        tenant_id IS NOT NULL
        AND tenant_id = get_current_tenant()
    );

CREATE POLICY queue_outbox_tenant_insert ON queue_outbox
    FOR INSERT
    WITH CHECK (
        tenant_id IS NOT NULL
        AND tenant_id = get_current_tenant()
    );

CREATE POLICY queue_inbox_tenant_select ON queue_inbox
    FOR SELECT
    USING (
        tenant_id IS NOT NULL
        AND tenant_id = get_current_tenant()
    );

-- Scheduler membership is resolved by OID through pg_roles. If the optional
-- production group is absent, EXISTS is false instead of making migration or
-- policy evaluation fail with an undefined-role error. A role created after a
-- local migration becomes effective as soon as reviewed table grants and
-- membership are applied.
CREATE POLICY queue_outbox_scheduler_all ON queue_outbox
    FOR ALL
    USING (
        EXISTS (
            SELECT 1
            FROM pg_catalog.pg_roles AS scheduler_role
            WHERE scheduler_role.rolname = 'complianceforge_scheduler'
              AND pg_catalog.pg_has_role(
                    current_user, scheduler_role.oid, 'MEMBER'
                  )
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM pg_catalog.pg_roles AS scheduler_role
            WHERE scheduler_role.rolname = 'complianceforge_scheduler'
              AND pg_catalog.pg_has_role(
                    current_user, scheduler_role.oid, 'MEMBER'
                  )
        )
    );

CREATE POLICY queue_inbox_scheduler_all ON queue_inbox
    FOR ALL
    USING (
        EXISTS (
            SELECT 1
            FROM pg_catalog.pg_roles AS scheduler_role
            WHERE scheduler_role.rolname = 'complianceforge_scheduler'
              AND pg_catalog.pg_has_role(
                    current_user, scheduler_role.oid, 'MEMBER'
                  )
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM pg_catalog.pg_roles AS scheduler_role
            WHERE scheduler_role.rolname = 'complianceforge_scheduler'
              AND pg_catalog.pg_has_role(
                    current_user, scheduler_role.oid, 'MEMBER'
                  )
        )
    );

-- FORCE RLS also applies to a non-bypass owner. Bind an explicit local and
-- migration path to each relation's exact current owner without granting that
-- ownership role to either production runtime identity.
DO $owner_policies$
DECLARE
    outbox_owner NAME;
    inbox_owner NAME;
BEGIN
    SELECT owner.rolname INTO STRICT outbox_owner
    FROM pg_catalog.pg_class AS relation
    JOIN pg_catalog.pg_roles AS owner ON owner.oid = relation.relowner
    WHERE relation.oid = 'queue_outbox'::regclass;

    SELECT owner.rolname INTO STRICT inbox_owner
    FROM pg_catalog.pg_class AS relation
    JOIN pg_catalog.pg_roles AS owner ON owner.oid = relation.relowner
    WHERE relation.oid = 'queue_inbox'::regclass;

    EXECUTE format(
        'CREATE POLICY queue_outbox_owner_all ON queue_outbox FOR ALL USING (current_user = session_user AND current_user = %L) WITH CHECK (current_user = session_user AND current_user = %L)',
        outbox_owner, outbox_owner
    );
    EXECUTE format(
        'CREATE POLICY queue_inbox_owner_all ON queue_inbox FOR ALL USING (current_user = session_user AND current_user = %L) WITH CHECK (current_user = session_user AND current_user = %L)',
        inbox_owner, inbox_owner
    );
END;
$owner_policies$;

COMMENT ON POLICY queue_outbox_tenant_select ON queue_outbox IS
    'Tenant-scoped API and diagnostic reads; NULL system messages remain worker-only.';
COMMENT ON POLICY queue_outbox_tenant_insert ON queue_outbox IS
    'Tenant-scoped API outbox production; envelope/tenant consistency remains enforced by chk_queue_outbox_envelope.';
COMMENT ON POLICY queue_inbox_tenant_select ON queue_inbox IS
    'Tenant-scoped API diagnostics; consumer mutations remain worker-only.';
COMMENT ON POLICY queue_outbox_scheduler_all ON queue_outbox IS
    'Cross-tenant and system-message dispatch capability for reviewed scheduler-group members.';
COMMENT ON POLICY queue_inbox_scheduler_all ON queue_inbox IS
    'Cross-tenant and system-message idempotency capability for reviewed scheduler-group members.';

COMMIT;
