package database

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// APIPostureQuerier is the catalog-query subset required by the API database
// identity check. Both pgx pools and request-independent connections implement
// it.
type APIPostureQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type apiDatabasePosture struct {
	roleName                  string
	superuser                 bool
	bypassRLS                 bool
	ownsCurrentDatabase       bool
	canCreateCurrentDatabase  bool
	ownsPublicSchema          bool
	canCreatePublicSchema     bool
	missingAPIMembership      bool
	unexpectedMemberships     []string
	evidenceOwnerMemberships  []string
	ownedPublicRelations      []string
	ownedProtectedRelations   []string
	writableImmutableLedgers  []string
	missingTablePrivileges    []string
	unexpectedTablePrivileges []string
	unexpectedSequenceGrants  []string
	missingCapabilities       []string
	misconfiguredCapabilities []string
	workerCapabilities        []string
	publicPrivileges          []string
	unexpectedDefiners        []string
	queueIsolationViolations  []string
}

// apiDatabasePostureQuery keeps the production API login outside every trust
// root introduced by migrations 056–058. The API may execute only the reviewed
// tenant-scoped capabilities; it must not inherit either SECURITY DEFINER
// owner, own schema/ledger objects, write immutable ledgers directly, or use
// the worker's cross-tenant scheduler capability.
const apiDatabasePostureQuery = `
WITH evidence_owner_roles(role_name) AS (
    VALUES
        ('complianceforge_evidence_chain_owner'::TEXT),
        ('complianceforge_evidence_registry_owner'::TEXT)
),
api_table_manifest(qualified_name, privileges) AS (
    VALUES
        ('public.access_audit_log'::TEXT, ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.access_policies', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.access_policy_assignments', ARRAY['SELECT', 'INSERT', 'DELETE']::TEXT[]),
        ('public.access_policy_certifications', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.access_policy_change_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.access_review_campaigns', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.access_review_items', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.access_review_decisions', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.access_sod_rules', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.access_sod_exceptions', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.access_sod_violations', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.access_governance_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.analytics_custom_dashboards', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.api_keys', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.asset_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.asset_reference_sequences', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.assets', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.audit_findings', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.audit_logs', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.audit_reference_sequences', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.audits', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.board_decisions', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.business_processes', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.calendar_events', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.comments', ARRAY['SELECT']::TEXT[]),
        ('public.compliance_frameworks', ARRAY['SELECT']::TEXT[]),
        ('public.continuity_plans', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.control_evidence', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.control_implementations', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.data_governance_event_chain_heads', ARRAY['SELECT']::TEXT[]),
        ('public.data_governance_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.effective_user_roles', ARRAY['SELECT']::TEXT[]),
        ('public.directory_change_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.directory_group_memberships', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.directory_groups', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.directory_imports', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.dsr_requests', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.dsr_tasks', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.evidence_collection_configs', ARRAY['SELECT']::TEXT[]),
        ('public.evidence_collection_runs', ARRAY['SELECT']::TEXT[]),
        ('public.evidence_custody_chain_heads', ARRAY['SELECT']::TEXT[]),
        ('public.evidence_custody_events', ARRAY['SELECT']::TEXT[]),
        ('public.evidence_requirements', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.evidence_reviews', ARRAY['SELECT']::TEXT[]),
        ('public.exception_reviews', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.feature_flag_change_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.field_level_permissions', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.framework_control_mappings', ARRAY['SELECT']::TEXT[]),
        ('public.framework_controls', ARRAY['SELECT']::TEXT[]),
        ('public.identity_authentication_challenges', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.identity_email_verification_tokens', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.identity_invitations', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.identity_mfa_recovery_codes', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.identity_passkeys', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.identity_security_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.identity_step_up_grants', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.incident_assignments', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.incident_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.incident_reference_sequences', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.incidents', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.integration_sync_logs', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.integrations', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.legal_hold_custodians', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.legal_hold_records', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.legal_hold_reference_sequences', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.legal_holds', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.nis2_security_measures', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.notification_channels', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.notification_preferences', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.notification_rules', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.notification_templates', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.notifications', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.organization_frameworks', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.organization_subscriptions', ARRAY['SELECT']::TEXT[]),
        ('public.organization_subscriptions_v2', ARRAY['SELECT']::TEXT[]),
        ('public.organizations', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.password_reset_tokens', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.permissions', ARRAY['SELECT']::TEXT[]),
        ('public.policies', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.policy_approval_steps', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.policy_approval_workflows', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.policy_attestations', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.policy_categories', ARRAY['SELECT']::TEXT[]),
        ('public.policy_exceptions', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.policy_reviews', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.policy_versions', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.processing_activities', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.product_capabilities', ARRAY['SELECT']::TEXT[]),
        ('public.queue_inbox', ARRAY['SELECT']::TEXT[]),
        ('public.queue_outbox', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.record_retention_assignments', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.remediation_actions', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.remediation_plans', ARRAY['SELECT', 'UPDATE']::TEXT[]),
        ('public.report_definitions', ARRAY['SELECT']::TEXT[]),
        ('public.report_runs', ARRAY['SELECT']::TEXT[]),
        ('public.retention_exceptions', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.retention_schedules', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.risk_appetite_statements', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.risk_assessments', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.risk_categories', ARRAY['SELECT']::TEXT[]),
        ('public.risk_indicator_values', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.risk_indicators', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.risk_matrices', ARRAY['SELECT']::TEXT[]),
        ('public.risk_treatments', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.risks', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.role_change_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.role_permissions', ARRAY['SELECT', 'INSERT', 'DELETE']::TEXT[]),
        ('public.roles', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.schema_migrations', ARRAY['SELECT']::TEXT[]),
        ('public.scim_resource_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.scim_token_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.scim_tokens', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.sso_configurations', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.subscription_plans', ARRAY['SELECT']::TEXT[]),
        ('public.tenant_data_governance_policies', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.tenant_feature_flag_overrides', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.tenant_identity_policies', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.user_entity_permissions', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.user_mfa', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.user_roles', ARRAY['SELECT', 'INSERT', 'UPDATE', 'DELETE']::TEXT[]),
        ('public.user_sessions', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.users', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.vendor_certifications', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.vendor_contacts', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.vendor_contracts', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.vendor_events', ARRAY['SELECT', 'INSERT']::TEXT[]),
        ('public.vendor_reference_sequences', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.vendor_subprocessors', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.vendors', ARRAY['SELECT', 'INSERT', 'UPDATE']::TEXT[]),
        ('public.workflow_step_executions', ARRAY['SELECT', 'UPDATE']::TEXT[])
),
required_api_table_privileges(qualified_name, privilege) AS (
    SELECT manifest.qualified_name, unnest(manifest.privileges)
    FROM api_table_manifest AS manifest
),
table_privilege_kinds(privilege) AS (
    VALUES ('SELECT'::TEXT), ('INSERT'::TEXT), ('UPDATE'::TEXT),
           ('DELETE'::TEXT), ('TRUNCATE'::TEXT), ('REFERENCES'::TEXT), ('TRIGGER'::TEXT)
),
sequence_privilege_kinds(privilege) AS (
    VALUES ('USAGE'::TEXT), ('SELECT'::TEXT), ('UPDATE'::TEXT)
),
protected_relations(qualified_name) AS (
    VALUES
        ('public.control_evidence'::TEXT),
        ('public.evidence_reviews'::TEXT),
        ('public.evidence_custody_chain_heads'::TEXT),
        ('public.evidence_custody_events'::TEXT),
        ('public.evidence_scheduler_tenants'::TEXT)
),
immutable_ledgers(qualified_name) AS (
    VALUES
        ('public.evidence_reviews'::TEXT),
        ('public.evidence_custody_chain_heads'::TEXT),
        ('public.evidence_custody_events'::TEXT),
        ('public.evidence_scheduler_tenants'::TEXT)
),
required_capabilities(signature, expected_owner, security_definer) AS (
    VALUES
        ('public.submit_evidence_review(uuid,uuid,uuid,uuid,text,text,text)'::TEXT,
         'complianceforge_evidence_chain_owner'::TEXT, TRUE),
        ('public.append_evidence_access_custody_event(uuid,uuid,uuid,uuid,text,text,text)'::TEXT,
         'complianceforge_evidence_chain_owner'::TEXT, TRUE),
        ('public.verify_evidence_custody_chain(uuid,uuid)'::TEXT, NULL::TEXT, FALSE),
        ('public.resolve_api_key_tenant(text)'::TEXT, NULL::TEXT, TRUE),
        ('public.resolve_scim_token_tenant(text)'::TEXT, NULL::TEXT, TRUE)
),
capability_posture AS (
    SELECT required.signature, required.expected_owner, required.security_definer,
           routine.oid, routine.prosecdef, routine.proconfig,
           owner_role.rolname AS owner_name,
           owner_role.rolcanlogin AS owner_can_login,
           owner_role.rolsuper AS owner_superuser,
           owner_role.rolbypassrls AS owner_bypass_rls,
           owner_role.rolcreatedb AS owner_create_database,
           owner_role.rolcreaterole AS owner_create_role,
           owner_role.rolreplication AS owner_replication,
           owner_role.rolinherit AS owner_inherit
    FROM required_capabilities AS required
    LEFT JOIN pg_catalog.pg_proc AS routine
      ON routine.oid = pg_catalog.to_regprocedure(required.signature)
    LEFT JOIN pg_catalog.pg_roles AS owner_role ON owner_role.oid = routine.proowner
),
worker_capabilities(signature) AS (
    VALUES
        ('public.notification_due_tenants(integer)'::TEXT),
        ('public.incident_due_tenants(integer)'::TEXT),
        ('public.vendor_due_tenants(integer)'::TEXT),
        ('public.evidence_due_tenants(integer,uuid)'::TEXT),
        ('public.expire_due_evidence(uuid,integer)'::TEXT)
)
SELECT runtime_role.rolname::TEXT,
       runtime_role.rolsuper,
       runtime_role.rolbypassrls,
       EXISTS (
           SELECT 1 FROM pg_catalog.pg_database AS database
           WHERE database.datname = current_database()
             AND pg_catalog.pg_has_role(current_user, database.datdba, 'MEMBER')
       ),
       pg_catalog.has_database_privilege(current_user, current_database(), 'CREATE'),
       EXISTS (
           SELECT 1 FROM pg_catalog.pg_namespace AS namespace
           WHERE namespace.nspname = 'public'
             AND pg_catalog.pg_has_role(current_user, namespace.nspowner, 'MEMBER')
       ),
       pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE'),
       NOT EXISTS (
           SELECT 1 FROM pg_catalog.pg_roles AS api_role
           WHERE api_role.rolname = 'complianceforge_api'
             AND pg_catalog.pg_has_role(current_user, api_role.oid, 'MEMBER')
       ),
       ARRAY(
           SELECT inherited_role.rolname
           FROM pg_catalog.pg_roles AS inherited_role
           WHERE inherited_role.rolname <> 'complianceforge_api'
             AND inherited_role.rolname <> current_user
             AND pg_catalog.pg_has_role(current_user, inherited_role.oid, 'MEMBER')
           ORDER BY inherited_role.rolname
       )::TEXT[],
       ARRAY(
           SELECT owner_role.role_name
           FROM evidence_owner_roles AS owner_role
           JOIN pg_catalog.pg_roles AS existing_role
             ON existing_role.rolname = owner_role.role_name
           WHERE pg_catalog.pg_has_role(current_user, existing_role.oid, 'MEMBER')
           ORDER BY owner_role.role_name
       )::TEXT[],
       ARRAY(
           SELECT format('public.%I', relation.relname)
           FROM pg_catalog.pg_class AS relation
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
           WHERE namespace.nspname = 'public'
             AND relation.relkind IN ('r', 'p', 'v', 'm', 'f')
             AND pg_catalog.pg_has_role(current_user, relation.relowner, 'MEMBER')
           ORDER BY relation.relname
       )::TEXT[],
       ARRAY(
           SELECT protected.qualified_name
           FROM protected_relations AS protected
           JOIN pg_catalog.pg_class AS relation
             ON relation.oid = pg_catalog.to_regclass(protected.qualified_name)
           WHERE pg_catalog.pg_has_role(current_user, relation.relowner, 'MEMBER')
           ORDER BY protected.qualified_name
       )::TEXT[],
       ARRAY(
           SELECT required.qualified_name || ':' || required.privilege
           FROM required_api_table_privileges AS required
           WHERE pg_catalog.to_regclass(required.qualified_name) IS NULL
              OR NOT COALESCE(pg_catalog.has_table_privilege(
                     current_user, pg_catalog.to_regclass(required.qualified_name), required.privilege), FALSE)
           ORDER BY required.qualified_name, required.privilege
       )::TEXT[],
       ARRAY(
           SELECT format('public.%I:%s', relation.relname, privilege_kind.privilege)
           FROM pg_catalog.pg_class AS relation
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
           CROSS JOIN table_privilege_kinds AS privilege_kind
           WHERE namespace.nspname = 'public'
             AND relation.relkind IN ('r', 'p', 'v', 'm', 'f')
             AND pg_catalog.has_table_privilege(current_user, relation.oid, privilege_kind.privilege)
             AND NOT EXISTS (
                 SELECT 1 FROM required_api_table_privileges AS allowed
                 WHERE pg_catalog.to_regclass(allowed.qualified_name) = relation.oid
                   AND allowed.privilege = privilege_kind.privilege
             )
           ORDER BY relation.relname, privilege_kind.privilege
       )::TEXT[],
       ARRAY(
           SELECT format('public.%I:%s', sequence.relname, privilege_kind.privilege)
           FROM pg_catalog.pg_class AS sequence
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = sequence.relnamespace
           CROSS JOIN sequence_privilege_kinds AS privilege_kind
           WHERE namespace.nspname = 'public'
             AND sequence.relkind = 'S'
             AND pg_catalog.has_sequence_privilege(current_user, sequence.oid, privilege_kind.privilege)
           ORDER BY sequence.relname, privilege_kind.privilege
       )::TEXT[],
       ARRAY(
           SELECT ledger.qualified_name
           FROM immutable_ledgers AS ledger
           WHERE COALESCE(pg_catalog.has_table_privilege(
                     current_user, pg_catalog.to_regclass(ledger.qualified_name), 'INSERT'), FALSE)
              OR COALESCE(pg_catalog.has_any_column_privilege(
                     current_user, pg_catalog.to_regclass(ledger.qualified_name), 'INSERT'), FALSE)
              OR COALESCE(pg_catalog.has_table_privilege(
                     current_user, pg_catalog.to_regclass(ledger.qualified_name), 'UPDATE'), FALSE)
              OR COALESCE(pg_catalog.has_any_column_privilege(
                     current_user, pg_catalog.to_regclass(ledger.qualified_name), 'UPDATE'), FALSE)
              OR COALESCE(pg_catalog.has_table_privilege(
                     current_user, pg_catalog.to_regclass(ledger.qualified_name), 'DELETE'), FALSE)
              OR COALESCE(pg_catalog.has_table_privilege(
                     current_user, pg_catalog.to_regclass(ledger.qualified_name), 'TRUNCATE'), FALSE)
           ORDER BY ledger.qualified_name
       )::TEXT[],
       ARRAY(
           SELECT capability.signature
           FROM capability_posture AS capability
           WHERE capability.oid IS NULL
              OR NOT COALESCE(pg_catalog.has_function_privilege(
                     current_user, capability.oid, 'EXECUTE'), FALSE)
           ORDER BY capability.signature
       )::TEXT[],
       ARRAY(
           SELECT capability.signature
           FROM capability_posture AS capability
           WHERE capability.oid IS NOT NULL AND (
               capability.prosecdef IS DISTINCT FROM capability.security_definer
               OR NOT EXISTS (
                   SELECT 1
                   FROM unnest(COALESCE(capability.proconfig, ARRAY[]::TEXT[])) AS setting(value)
                   WHERE replace(setting.value, ' ', '') = 'search_path=pg_catalog,public,pg_temp'
               )
               OR (capability.expected_owner IS NOT NULL AND (
                   capability.owner_name IS DISTINCT FROM capability.expected_owner
                   OR COALESCE(capability.owner_can_login, TRUE)
                   OR COALESCE(capability.owner_superuser, TRUE)
                   OR COALESCE(capability.owner_bypass_rls, TRUE)
                   OR COALESCE(capability.owner_create_database, TRUE)
                   OR COALESCE(capability.owner_create_role, TRUE)
                   OR COALESCE(capability.owner_replication, TRUE)
                   OR COALESCE(capability.owner_inherit, TRUE)
               ))
           )
           ORDER BY capability.signature
       )::TEXT[],
       ARRAY(
           SELECT worker.signature
           FROM worker_capabilities AS worker
           WHERE COALESCE(pg_catalog.has_function_privilege(
               current_user, pg_catalog.to_regprocedure(worker.signature), 'EXECUTE'), FALSE)
           ORDER BY worker.signature
       )::TEXT[]
` + runtimeBoundaryProjection + `
FROM pg_catalog.pg_roles AS runtime_role
WHERE runtime_role.rolname = current_user`

// ValidateAPIDatabasePosture fails closed unless the connected role is a
// narrowly privileged runtime identity with the exact reviewed API
// capabilities. It is intended for production API startup before listeners
// or background components are created.
func ValidateAPIDatabasePosture(ctx context.Context, querier APIPostureQuerier) error {
	if querier == nil {
		return fmt.Errorf("API database posture query requires a database connection")
	}

	var posture apiDatabasePosture
	err := querier.QueryRow(ctx, apiDatabasePostureQuery, []string{
		"public.get_current_tenant()",
		"public.resolve_api_key_tenant(text)",
		"public.resolve_scim_token_tenant(text)",
		"public.submit_evidence_review(uuid,uuid,uuid,uuid,text,text,text)",
		"public.append_evidence_access_custody_event(uuid,uuid,uuid,uuid,text,text,text)",
	}, reviewedRuntimeSchemaVersion).Scan(
		&posture.roleName,
		&posture.superuser,
		&posture.bypassRLS,
		&posture.ownsCurrentDatabase,
		&posture.canCreateCurrentDatabase,
		&posture.ownsPublicSchema,
		&posture.canCreatePublicSchema,
		&posture.missingAPIMembership,
		&posture.unexpectedMemberships,
		&posture.evidenceOwnerMemberships,
		&posture.ownedPublicRelations,
		&posture.ownedProtectedRelations,
		&posture.missingTablePrivileges,
		&posture.unexpectedTablePrivileges,
		&posture.unexpectedSequenceGrants,
		&posture.writableImmutableLedgers,
		&posture.missingCapabilities,
		&posture.misconfiguredCapabilities,
		&posture.workerCapabilities,
		&posture.publicPrivileges,
		&posture.unexpectedDefiners,
		&posture.queueIsolationViolations,
	)
	if err != nil {
		return fmt.Errorf("inspect API database posture: %w", err)
	}
	return validateAPIDatabasePosture(posture)
}

func validateAPIDatabasePosture(posture apiDatabasePosture) error {
	posture.roleName = strings.TrimSpace(posture.roleName)
	if posture.roleName == "" {
		return fmt.Errorf("API database posture returned an empty runtime role")
	}

	violations := make([]string, 0, 20)
	if posture.superuser {
		violations = append(violations, "role is SUPERUSER")
	}
	if posture.bypassRLS {
		violations = append(violations, "role has BYPASSRLS")
	}
	if posture.ownsCurrentDatabase {
		violations = append(violations, "role owns or can assume the current database owner")
	}
	if posture.canCreateCurrentDatabase {
		violations = append(violations, "role can create schemas in the current database")
	}
	if posture.ownsPublicSchema {
		violations = append(violations, "role owns or can assume the public schema owner")
	}
	if posture.canCreatePublicSchema {
		violations = append(violations, "role can create objects in the public schema")
	}
	if posture.missingAPIMembership {
		violations = append(violations, "role is not a member of complianceforge_api")
	}
	appendPostureListViolation(&violations, "role has unexpected memberships", posture.unexpectedMemberships)
	appendPostureListViolation(&violations, "role is a member of evidence capability owner roles", posture.evidenceOwnerMemberships)
	appendPostureListViolation(&violations, "role owns or can assume owners of public relations", posture.ownedPublicRelations)
	appendPostureListViolation(&violations, "role owns protected evidence relations", posture.ownedProtectedRelations)
	appendPostureListViolation(&violations, "role lacks required API table privileges", posture.missingTablePrivileges)
	appendPostureListViolation(&violations, "role has table privileges outside the API allowlist", posture.unexpectedTablePrivileges)
	appendPostureListViolation(&violations, "role has unexpected sequence privileges", posture.unexpectedSequenceGrants)
	appendPostureListViolation(&violations, "role can write immutable evidence ledgers directly", posture.writableImmutableLedgers)
	appendPostureListViolation(&violations, "role lacks required evidence capabilities", posture.missingCapabilities)
	appendPostureListViolation(&violations, "evidence capabilities have unsafe definitions", posture.misconfiguredCapabilities)
	appendPostureListViolation(&violations, "role can execute worker-only capabilities", posture.workerCapabilities)
	appendPostureListViolation(&violations, "PUBLIC has runtime data/function privileges", posture.publicPrivileges)
	appendPostureListViolation(&violations, "role can execute unreviewed SECURITY DEFINER routines", posture.unexpectedDefiners)
	appendPostureListViolation(&violations, "runtime schema/temporary/queue isolation has unsafe definitions", posture.queueIsolationViolations)
	if len(violations) > 0 {
		return fmt.Errorf("unsafe production API database role %q: %s", posture.roleName, strings.Join(violations, "; "))
	}
	return nil
}

func appendPostureListViolation(violations *[]string, message string, values []string) {
	if len(values) == 0 {
		return
	}
	values = append([]string(nil), values...)
	sort.Strings(values)
	*violations = append(*violations, message+": "+strings.Join(values, ", "))
}
