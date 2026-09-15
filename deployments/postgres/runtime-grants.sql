\set ON_ERROR_STOP on

-- Reviewed runtime grant manifest for schema version 59. Run as the migration
-- identity after migrations. The exact-version guard forces a new privilege
-- review whenever a migration adds or changes database objects.
\if :{?api_role}
\else
  \echo 'runtime-grants.sql requires -v api_role=<login>'
  \quit 3
\endif
\if :{?worker_role}
\else
  \echo 'runtime-grants.sql requires -v worker_role=<login>'
  \quit 3
\endif

BEGIN;
SELECT pg_catalog.set_config('complianceforge.provision.api_role', :'api_role', true);
SELECT pg_catalog.set_config('complianceforge.provision.worker_role', :'worker_role', true);

DO $schema_guard$
DECLARE
    current_version BIGINT;
    migration_dirty BOOLEAN;
    api_role_name TEXT := current_setting('complianceforge.provision.api_role');
    worker_role_name TEXT := current_setting('complianceforge.provision.worker_role');
BEGIN
    SELECT version, dirty INTO current_version, migration_dirty FROM public.schema_migrations;
    IF current_version IS DISTINCT FROM 59 OR migration_dirty IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION 'runtime-grants.sql requires clean schema version 59 (found %, dirty=%)',
            current_version, migration_dirty USING ERRCODE = '55000';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_api')
       OR NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'complianceforge_scheduler') THEN
        RAISE EXCEPTION 'runtime-roles.sql must be applied before runtime-grants.sql'
            USING ERRCODE = '42704';
    END IF;
    IF api_role_name = worker_role_name
       OR NOT pg_catalog.pg_has_role(api_role_name, 'complianceforge_api', 'MEMBER')
       OR NOT pg_catalog.pg_has_role(worker_role_name, 'complianceforge_scheduler', 'MEMBER') THEN
        RAISE EXCEPTION 'runtime login roles must be distinct and bound by runtime-roles.sql'
            USING ERRCODE = '42501';
    END IF;
END;
$schema_guard$;

-- Converge from a deny-by-default baseline. This removes stale direct grants
-- from both the fixed groups and the concrete logins before rebuilding the
-- reviewed schema-59 matrix. PUBLIC has no table/sequence privileges in the
-- supported deployment, and is reset here in case a prior operator grant
-- widened it. PUBLIC function EXECUTE is removed too: callable SECURITY
-- DEFINER capabilities are an explicit matrix, not PostgreSQL's legacy default.
DO $reset_runtime_grants$
DECLARE
    runtime_role TEXT;
BEGIN
    FOREACH runtime_role IN ARRAY ARRAY[
        'complianceforge_api',
        'complianceforge_scheduler',
        current_setting('complianceforge.provision.api_role'),
        current_setting('complianceforge.provision.worker_role')
    ] LOOP
        EXECUTE format('REVOKE CREATE, TEMPORARY ON DATABASE %I FROM %I', current_database(), runtime_role);
        EXECUTE format('REVOKE ALL PRIVILEGES ON SCHEMA public FROM %I', runtime_role);
        EXECUTE format('REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %I', runtime_role);
        EXECUTE format('REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %I', runtime_role);
        EXECUTE format('REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM %I', runtime_role);
    END LOOP;
END;
$reset_runtime_grants$;

DO $public_database_grants$
BEGIN
    EXECUTE format('REVOKE CREATE, TEMPORARY ON DATABASE %I FROM PUBLIC', current_database());
END;
$public_database_grants$;
REVOKE ALL PRIVILEGES ON SCHEMA public FROM PUBLIC;

REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM PUBLIC;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC;
REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC;

-- pg_temp is otherwise implicitly searched before trusted relation schemas,
-- even when omitted from search_path. Pin it last for callable and trigger-only
-- definers, protecting both fresh and already-open sessions with temp objects.
DO $definer_search_paths$
DECLARE
    routine_signature TEXT;
BEGIN
    FOR routine_signature IN
        SELECT routine.oid::regprocedure::text
        FROM pg_catalog.pg_proc AS routine
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = routine.pronamespace
        WHERE namespace.nspname = 'public' AND routine.prosecdef
    LOOP
        EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, public, pg_temp', routine_signature);
    END LOOP;
END;
$definer_search_paths$;
ALTER FUNCTION public.verify_evidence_custody_chain(UUID, UUID)
SET search_path TO pg_catalog, public, pg_temp;

GRANT USAGE ON SCHEMA public TO complianceforge_api, complianceforge_scheduler;

-- SECURITY INVOKER scalar helpers (including pgcrypto/check/hash helpers)
-- run only with the caller's reviewed table privileges and RLS context.
-- Trigger-only routines need no caller EXECUTE at trigger runtime. The two
-- non-login evidence owners also need these helpers for their narrow wrappers.
DO $invoker_helpers$
DECLARE
    routine_signature TEXT;
BEGIN
    FOR routine_signature IN
        SELECT routine.oid::regprocedure::text
        FROM pg_catalog.pg_proc AS routine
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = routine.pronamespace
        WHERE namespace.nspname = 'public'
          AND NOT routine.prosecdef
          AND routine.prorettype <> 'pg_catalog.trigger'::regtype
    LOOP
        EXECUTE format(
            'GRANT EXECUTE ON FUNCTION %s TO complianceforge_api, complianceforge_scheduler, complianceforge_evidence_chain_owner, complianceforge_evidence_registry_owner',
            routine_signature
        );
    END LOOP;
END;
$invoker_helpers$;
-- Audited legacy session getter: it exposes only the caller's session GUC,
-- which the connection-scoping helper establishes before every tenant query.
GRANT EXECUTE ON FUNCTION public.get_current_tenant()
TO complianceforge_api, complianceforge_scheduler,
   complianceforge_evidence_chain_owner, complianceforge_evidence_registry_owner;

-- Explicit API matrix for BuildDependencies' composed handlers. Domain delete
-- routes soft-delete with UPDATE; they receive no physical DELETE. Catalogs,
-- counters, draft/version projections, immutable evidence and offboarding
-- bindings have separate verb sets below. New RLS tables receive nothing.
GRANT SELECT, INSERT, UPDATE ON TABLE
    public.access_policies,
    public.access_review_campaigns,
    public.access_sod_rules,
    public.access_sod_exceptions,
    public.api_keys,
    public.asset_reference_sequences,
    public.assets,
    public.audit_findings,
    public.audit_reference_sequences,
    public.audits,
    public.control_evidence,
    public.control_implementations,
    public.directory_group_memberships,
    public.directory_groups,
    public.identity_authentication_challenges,
    public.identity_email_verification_tokens,
    public.identity_invitations,
    public.identity_passkeys,
    public.identity_step_up_grants,
    public.incident_assignments,
    public.incident_reference_sequences,
    public.incidents,
    public.integrations,
    public.legal_hold_custodians,
    public.legal_hold_records,
    public.legal_hold_reference_sequences,
    public.legal_holds,
    public.notification_channels,
    public.notification_preferences,
    public.notification_rules,
    public.notifications,
    public.organization_frameworks,
    public.organizations,
    public.policies,
    public.policy_approval_steps,
    public.policy_approval_workflows,
    public.policy_attestations,
    public.policy_exceptions,
    public.policy_reviews,
    public.policy_versions,
    public.record_retention_assignments,
    public.retention_exceptions,
    public.retention_schedules,
    public.risk_appetite_statements,
    public.risk_indicators,
    public.risk_treatments,
    public.risks,
    public.roles,
    public.scim_tokens,
    public.sso_configurations,
    public.tenant_data_governance_policies,
    public.tenant_identity_policies,
    public.user_sessions,
    public.users,
    public.vendor_certifications,
    public.vendor_contacts,
    public.vendor_contracts,
    public.vendor_reference_sequences,
    public.vendor_subprocessors,
    public.vendors
TO complianceforge_api;

-- Policy assignment/permission replacement uses physical deletes with immutable
-- change evidence. Timed user-role windows additionally require UPDATE below.
GRANT SELECT, INSERT, DELETE ON TABLE
    public.access_policy_assignments,
    public.role_permissions
TO complianceforge_api;

-- Explicit replace/reset/remove APIs and bounded credential offboarding need
-- DELETE; no other tenant projection receives it.
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    public.field_level_permissions,
    public.identity_mfa_recovery_codes,
    public.notification_templates,
    public.password_reset_tokens,
    public.tenant_feature_flag_overrides,
    public.user_entity_permissions,
    public.user_mfa,
    public.user_roles
TO complianceforge_api;

GRANT SELECT, INSERT ON TABLE
    public.access_audit_log,
    public.access_policy_certifications,
    public.access_policy_change_events,
    public.access_review_items,
    public.access_review_decisions,
    public.access_sod_violations,
    public.access_governance_events,
    public.asset_events,
    public.audit_logs,
    public.data_governance_events,
    public.directory_change_events,
    public.directory_imports,
    public.feature_flag_change_events,
    public.identity_security_events,
    public.incident_events,
    public.integration_sync_logs,
    public.risk_assessments,
    public.risk_indicator_values,
    public.role_change_events,
    public.scim_resource_events,
    public.scim_token_events,
    public.vendor_events
TO complianceforge_api;

-- Offboarding must inspect/transfer ownership even in disabled legacy
-- modules. Only SELECT/UPDATE are authorized for these exact bindings in
-- user_admin_repo.go; this does not enable their create/delete handlers.
GRANT SELECT, UPDATE ON TABLE
    public.analytics_custom_dashboards,
    public.board_decisions,
    public.business_processes,
    public.calendar_events,
    public.continuity_plans,
    public.dsr_requests,
    public.dsr_tasks,
    public.evidence_requirements,
    public.exception_reviews,
    public.nis2_security_measures,
    public.processing_activities,
    public.remediation_actions,
    public.remediation_plans,
    public.workflow_step_executions
TO complianceforge_api;

-- Canonical catalog reads, evidence provenance validation, diagnostics,
-- entitlements, and governed-record existence checks only.
GRANT SELECT ON TABLE
    public.comments,
    public.compliance_frameworks,
    public.data_governance_event_chain_heads,
    public.effective_user_roles,
    public.evidence_collection_configs,
    public.evidence_collection_runs,
    public.evidence_reviews,
    public.evidence_custody_events,
    public.evidence_custody_chain_heads,
    public.framework_control_mappings,
    public.framework_controls,
    public.organization_subscriptions,
    public.organization_subscriptions_v2,
    public.permissions,
    public.policy_categories,
    public.product_capabilities,
    public.report_definitions,
    public.report_runs,
    public.risk_categories,
    public.risk_matrices,
    public.schema_migrations,
    public.subscription_plans
TO complianceforge_api;

-- API transactions may produce durable messages but cannot dispatch, purge,
-- consume, or acquire global worker leases.
GRANT SELECT, INSERT ON TABLE public.queue_outbox TO complianceforge_api;
REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.queue_outbox FROM complianceforge_api;
GRANT SELECT ON TABLE public.queue_inbox TO complianceforge_api;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON TABLE public.queue_inbox FROM complianceforge_api;
REVOKE ALL ON TABLE
    public.scheduler_leases,
    public.evidence_scheduler_tenants
FROM complianceforge_api;

GRANT EXECUTE ON FUNCTION
    public.submit_evidence_review(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT),
    public.append_evidence_access_custody_event(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT),
    public.verify_evidence_custody_chain(UUID, UUID),
    public.resolve_api_key_tenant(TEXT),
    public.resolve_scim_token_tenant(TEXT)
TO complianceforge_api;
-- These seven cross-tenant capabilities would otherwise be executable through
-- PostgreSQL's default PUBLIC function grant.
REVOKE EXECUTE ON FUNCTION
    public.notification_due_tenants(INTEGER),
    public.incident_due_tenants(INTEGER),
    public.vendor_due_tenants(INTEGER),
    public.evidence_due_tenants(INTEGER, UUID),
    public.expire_due_evidence(UUID, INTEGER),
    public.resolve_api_key_tenant(TEXT),
    public.resolve_scim_token_tenant(TEXT)
FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION
    public.notification_due_tenants(INTEGER),
    public.incident_due_tenants(INTEGER),
    public.vendor_due_tenants(INTEGER),
    public.evidence_due_tenants(INTEGER, UUID),
    public.expire_due_evidence(UUID, INTEGER)
FROM complianceforge_api;

-- Worker privilege matrix. Every relation below maps to a concrete queue,
-- lease, notification-delivery, or scheduled-job query in cmd/worker.
GRANT SELECT ON TABLE
    public.access_sod_rules,
    public.access_sod_exceptions,
    public.analytics_compliance_trends,
    public.assets,
    public.audit_findings,
    public.audits,
    public.compliance_exceptions,
    public.compliance_frameworks,
    public.control_evidence,
    public.control_implementations,
    public.dsr_requests,
    public.effective_user_roles,
    public.evidence_collection_configs,
    public.framework_controls,
    public.incidents,
    public.nis2_incident_reports,
    public.notification_channels,
    public.notification_preferences,
    public.notification_rules,
    public.notification_templates,
    public.organization_frameworks,
    public.organizations,
    public.policies,
    public.policy_versions,
    public.report_definitions,
    public.risks,
    public.roles,
    public.schema_migrations,
    public.user_roles,
    public.users,
    public.vendors,
    public.workflow_steps
TO complianceforge_scheduler;

GRANT SELECT, INSERT, UPDATE ON TABLE
    public.analytics_compliance_trends,
    public.analytics_snapshots,
    public.notifications
TO complianceforge_scheduler;
GRANT SELECT, UPDATE ON TABLE
    public.calendar_events,
    public.compliance_exceptions,
    public.dsr_requests,
    public.report_schedules,
    public.workflow_instances,
    public.workflow_step_executions
TO complianceforge_scheduler;
GRANT SELECT, INSERT ON TABLE public.report_runs TO complianceforge_scheduler;
GRANT SELECT, INSERT ON TABLE public.exception_audit_trail TO complianceforge_scheduler;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    public.queue_inbox,
    public.queue_outbox,
    public.scheduler_leases,
    public.search_index
TO complianceforge_scheduler;

GRANT EXECUTE ON FUNCTION
    public.evidence_due_tenants(INTEGER, UUID),
    public.expire_due_evidence(UUID, INTEGER)
TO complianceforge_scheduler;

-- Legacy table-scanning discovery depends on a privileged migration owner and
-- can silently return no FORCE-RLS rows under an ordinary migrator. Runtime
-- discovery now streams the active registry and checks each tenant separately.
REVOKE EXECUTE ON FUNCTION
    public.notification_due_tenants(INTEGER),
    public.incident_due_tenants(INTEGER),
    public.vendor_due_tenants(INTEGER)
FROM complianceforge_scheduler;

-- The worker cannot mint API review/access events or write protected ledgers.
REVOKE EXECUTE ON FUNCTION
    public.submit_evidence_review(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT),
    public.append_evidence_access_custody_event(UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT),
    public.verify_evidence_custody_chain(UUID, UUID)
FROM complianceforge_scheduler;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON TABLE
    public.evidence_reviews,
    public.evidence_custody_events,
    public.evidence_custody_chain_heads,
    public.evidence_scheduler_tenants
FROM complianceforge_scheduler;

COMMIT;
