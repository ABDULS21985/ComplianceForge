package database

import (
	"context"
	"fmt"
	"strings"
)

type workerDatabasePosture struct {
	roleName                   string
	superuser                  bool
	bypassRLS                  bool
	ownsCurrentDatabase        bool
	canCreateCurrentDatabase   bool
	ownsPublicSchema           bool
	canCreatePublicSchema      bool
	missingSchedulerMembership bool
	unexpectedMemberships      []string
	forbiddenMemberships       []string
	ownedPublicRelations       []string
	ownedProtectedRelations    []string
	writableEvidenceLedgers    []string
	missingTablePrivileges     []string
	unexpectedTablePrivileges  []string
	unexpectedSequenceGrants   []string
	missingCapabilities        []string
	misconfiguredCapabilities  []string
	forbiddenCapabilities      []string
	publicPrivileges           []string
	unexpectedDefiners         []string
	queueIsolationViolations   []string
}

// workerDatabasePostureQuery mirrors deployments/postgres/runtime-grants.sql.
// Every table/verb pair maps to a concrete worker queue, lease, delivery, or
// scheduler query. A later schema version must update both artifacts together.
const workerDatabasePostureQuery = `
WITH required_table_privileges(qualified_name, privilege) AS (
    VALUES
        ('public.access_sod_rules', 'SELECT'),
        ('public.access_sod_exceptions', 'SELECT'),
        ('public.analytics_compliance_trends'::TEXT, 'SELECT'::TEXT),
        ('public.analytics_compliance_trends', 'INSERT'),
        ('public.analytics_compliance_trends', 'UPDATE'),
        ('public.analytics_snapshots', 'SELECT'),
        ('public.analytics_snapshots', 'INSERT'),
        ('public.analytics_snapshots', 'UPDATE'),
        ('public.assets', 'SELECT'),
        ('public.audit_findings', 'SELECT'),
        ('public.audits', 'SELECT'),
        ('public.calendar_events', 'SELECT'),
        ('public.calendar_events', 'UPDATE'),
        ('public.compliance_exceptions', 'SELECT'),
        ('public.compliance_exceptions', 'UPDATE'),
        ('public.compliance_frameworks', 'SELECT'),
        ('public.control_evidence', 'SELECT'),
        ('public.control_implementations', 'SELECT'),
        ('public.dsr_requests', 'SELECT'),
        ('public.dsr_requests', 'UPDATE'),
        ('public.effective_user_roles', 'SELECT'),
        ('public.evidence_collection_configs', 'SELECT'),
        ('public.exception_audit_trail', 'SELECT'),
        ('public.exception_audit_trail', 'INSERT'),
        ('public.framework_controls', 'SELECT'),
        ('public.incidents', 'SELECT'),
        ('public.nis2_incident_reports', 'SELECT'),
        ('public.notification_channels', 'SELECT'),
        ('public.notification_preferences', 'SELECT'),
        ('public.notification_rules', 'SELECT'),
        ('public.notification_templates', 'SELECT'),
        ('public.notifications', 'SELECT'),
        ('public.notifications', 'INSERT'),
        ('public.notifications', 'UPDATE'),
        ('public.organization_frameworks', 'SELECT'),
        ('public.organizations', 'SELECT'),
        ('public.policies', 'SELECT'),
        ('public.policy_versions', 'SELECT'),
        ('public.queue_inbox', 'SELECT'),
        ('public.queue_inbox', 'INSERT'),
        ('public.queue_inbox', 'UPDATE'),
        ('public.queue_inbox', 'DELETE'),
        ('public.queue_outbox', 'SELECT'),
        ('public.queue_outbox', 'INSERT'),
        ('public.queue_outbox', 'UPDATE'),
        ('public.queue_outbox', 'DELETE'),
        ('public.report_definitions', 'SELECT'),
        ('public.report_runs', 'SELECT'),
        ('public.report_runs', 'INSERT'),
        ('public.report_schedules', 'SELECT'),
        ('public.report_schedules', 'UPDATE'),
        ('public.risks', 'SELECT'),
        ('public.roles', 'SELECT'),
        ('public.schema_migrations', 'SELECT'),
        ('public.scheduler_leases', 'SELECT'),
        ('public.scheduler_leases', 'INSERT'),
        ('public.scheduler_leases', 'UPDATE'),
        ('public.scheduler_leases', 'DELETE'),
        ('public.search_index', 'SELECT'),
        ('public.search_index', 'INSERT'),
        ('public.search_index', 'UPDATE'),
        ('public.search_index', 'DELETE'),
        ('public.user_roles', 'SELECT'),
        ('public.users', 'SELECT'),
        ('public.vendors', 'SELECT'),
        ('public.workflow_instances', 'SELECT'),
        ('public.workflow_instances', 'UPDATE'),
        ('public.workflow_step_executions', 'SELECT'),
        ('public.workflow_step_executions', 'UPDATE'),
        ('public.workflow_steps', 'SELECT')
),
required_capabilities(signature, expected_owner) AS (
    VALUES
        ('public.evidence_due_tenants(integer,uuid)'::TEXT,
         'complianceforge_evidence_registry_owner'::TEXT),
        ('public.expire_due_evidence(uuid,integer)'::TEXT,
         'complianceforge_evidence_chain_owner'::TEXT)
),
table_privilege_kinds(privilege) AS (
    VALUES ('SELECT'::TEXT), ('INSERT'::TEXT), ('UPDATE'::TEXT),
           ('DELETE'::TEXT), ('TRUNCATE'::TEXT), ('REFERENCES'::TEXT), ('TRIGGER'::TEXT)
),
sequence_privilege_kinds(privilege) AS (
    VALUES ('USAGE'::TEXT), ('SELECT'::TEXT), ('UPDATE'::TEXT)
),
capability_posture AS (
    SELECT required.signature, required.expected_owner,
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
forbidden_memberships(role_name) AS (
    VALUES
        ('complianceforge_api'::TEXT),
        ('complianceforge_evidence_chain_owner'::TEXT),
        ('complianceforge_evidence_registry_owner'::TEXT)
),
protected_relations(qualified_name) AS (
    VALUES
        ('public.control_evidence'::TEXT),
        ('public.evidence_reviews'::TEXT),
        ('public.evidence_custody_chain_heads'::TEXT),
        ('public.evidence_custody_events'::TEXT),
        ('public.evidence_scheduler_tenants'::TEXT),
        ('public.queue_inbox'::TEXT),
        ('public.queue_outbox'::TEXT),
        ('public.scheduler_leases'::TEXT)
),
immutable_ledgers(qualified_name) AS (
    VALUES
        ('public.evidence_reviews'::TEXT),
        ('public.evidence_custody_chain_heads'::TEXT),
        ('public.evidence_custody_events'::TEXT),
        ('public.evidence_scheduler_tenants'::TEXT)
),
forbidden_capabilities(signature) AS (
    VALUES
        ('public.submit_evidence_review(uuid,uuid,uuid,uuid,text,text,text)'::TEXT),
        ('public.append_evidence_access_custody_event(uuid,uuid,uuid,uuid,text,text,text)'::TEXT),
        ('public.verify_evidence_custody_chain(uuid,uuid)'::TEXT),
        ('public.notification_due_tenants(integer)'::TEXT),
        ('public.incident_due_tenants(integer)'::TEXT),
        ('public.vendor_due_tenants(integer)'::TEXT)
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
           SELECT 1 FROM pg_catalog.pg_roles AS scheduler_role
           WHERE scheduler_role.rolname = 'complianceforge_scheduler'
             AND pg_catalog.pg_has_role(current_user, scheduler_role.oid, 'MEMBER')
       ),
       ARRAY(
           SELECT inherited_role.rolname
           FROM pg_catalog.pg_roles AS inherited_role
           WHERE inherited_role.rolname <> 'complianceforge_scheduler'
             AND inherited_role.rolname <> current_user
             AND pg_catalog.pg_has_role(current_user, inherited_role.oid, 'MEMBER')
           ORDER BY inherited_role.rolname
       )::TEXT[],
       ARRAY(
           SELECT forbidden.role_name
           FROM forbidden_memberships AS forbidden
           JOIN pg_catalog.pg_roles AS existing_role ON existing_role.rolname = forbidden.role_name
           WHERE pg_catalog.pg_has_role(current_user, existing_role.oid, 'MEMBER')
           ORDER BY forbidden.role_name
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
           SELECT format('public.%I:%s', relation.relname, privilege_kind.privilege)
           FROM pg_catalog.pg_class AS relation
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
           CROSS JOIN table_privilege_kinds AS privilege_kind
           WHERE namespace.nspname = 'public'
             AND relation.relkind IN ('r', 'p', 'v', 'm', 'f')
             AND pg_catalog.has_table_privilege(current_user, relation.oid, privilege_kind.privilege)
             AND NOT EXISTS (
                 SELECT 1 FROM required_table_privileges AS allowed
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
           SELECT required.qualified_name || ':' || required.privilege
           FROM required_table_privileges AS required
           WHERE pg_catalog.to_regclass(required.qualified_name) IS NULL
              OR NOT COALESCE(pg_catalog.has_table_privilege(
                     current_user, pg_catalog.to_regclass(required.qualified_name), required.privilege), FALSE)
           ORDER BY required.qualified_name, required.privilege
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
               capability.prosecdef IS DISTINCT FROM TRUE
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
               OR capability.owner_name = current_user
           )
           ORDER BY capability.signature
       )::TEXT[],
       ARRAY(
           SELECT forbidden.signature
           FROM forbidden_capabilities AS forbidden
           WHERE COALESCE(pg_catalog.has_function_privilege(
               current_user, pg_catalog.to_regprocedure(forbidden.signature), 'EXECUTE'), FALSE)
           ORDER BY forbidden.signature
       )::TEXT[]
` + runtimeBoundaryProjection + `
FROM pg_catalog.pg_roles AS runtime_role
WHERE runtime_role.rolname = current_user`

// ValidateWorkerDatabasePosture verifies that the worker can execute every
// currently composed queue and scheduler query. Production additionally
// requires a distinct non-owner role and rejects API-only capabilities.
func ValidateWorkerDatabasePosture(ctx context.Context, querier APIPostureQuerier, production bool) error {
	if querier == nil {
		return fmt.Errorf("worker database posture query requires a database connection")
	}

	var posture workerDatabasePosture
	err := querier.QueryRow(ctx, workerDatabasePostureQuery, []string{
		"public.get_current_tenant()",
		"public.evidence_due_tenants(integer,uuid)",
		"public.expire_due_evidence(uuid,integer)",
	}, reviewedRuntimeSchemaVersion).Scan(
		&posture.roleName,
		&posture.superuser,
		&posture.bypassRLS,
		&posture.ownsCurrentDatabase,
		&posture.canCreateCurrentDatabase,
		&posture.ownsPublicSchema,
		&posture.canCreatePublicSchema,
		&posture.missingSchedulerMembership,
		&posture.unexpectedMemberships,
		&posture.forbiddenMemberships,
		&posture.ownedPublicRelations,
		&posture.ownedProtectedRelations,
		&posture.unexpectedTablePrivileges,
		&posture.unexpectedSequenceGrants,
		&posture.writableEvidenceLedgers,
		&posture.missingTablePrivileges,
		&posture.missingCapabilities,
		&posture.misconfiguredCapabilities,
		&posture.forbiddenCapabilities,
		&posture.publicPrivileges,
		&posture.unexpectedDefiners,
		&posture.queueIsolationViolations,
	)
	if err != nil {
		return fmt.Errorf("inspect worker database posture: %w", err)
	}
	return validateWorkerDatabasePosture(posture, production)
}

func validateWorkerDatabasePosture(posture workerDatabasePosture, production bool) error {
	posture.roleName = strings.TrimSpace(posture.roleName)
	if posture.roleName == "" {
		return fmt.Errorf("worker database posture returned an empty runtime role")
	}
	violations := make([]string, 0, 20)
	appendPostureListViolation(&violations, "role lacks required worker table privileges", posture.missingTablePrivileges)
	appendPostureListViolation(&violations, "role lacks required worker capabilities", posture.missingCapabilities)
	appendPostureListViolation(&violations, "worker capabilities have unsafe definitions", posture.misconfiguredCapabilities)
	if production {
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
		if posture.missingSchedulerMembership {
			violations = append(violations, "role is not a member of complianceforge_scheduler")
		}
		appendPostureListViolation(&violations, "role has unexpected memberships", posture.unexpectedMemberships)
		appendPostureListViolation(&violations, "role has forbidden API/owner memberships", posture.forbiddenMemberships)
		appendPostureListViolation(&violations, "role owns or can assume owners of public relations", posture.ownedPublicRelations)
		appendPostureListViolation(&violations, "role owns or can assume owners of protected relations", posture.ownedProtectedRelations)
		appendPostureListViolation(&violations, "role has table privileges outside the worker allowlist", posture.unexpectedTablePrivileges)
		appendPostureListViolation(&violations, "role has unexpected sequence privileges", posture.unexpectedSequenceGrants)
		appendPostureListViolation(&violations, "role can write protected evidence ledgers directly", posture.writableEvidenceLedgers)
		appendPostureListViolation(&violations, "role can execute API-only evidence capabilities", posture.forbiddenCapabilities)
		appendPostureListViolation(&violations, "PUBLIC has runtime data/function privileges", posture.publicPrivileges)
		appendPostureListViolation(&violations, "role can execute unreviewed SECURITY DEFINER routines", posture.unexpectedDefiners)
		appendPostureListViolation(&violations, "runtime schema/temporary/queue isolation has unsafe definitions", posture.queueIsolationViolations)
	}
	if len(violations) > 0 {
		return fmt.Errorf("unsafe worker database role %q: %s", posture.roleName, strings.Join(violations, "; "))
	}
	return nil
}
