#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TEST_DATABASE_URL:-}" ]]; then
  echo "TEST_DATABASE_URL must identify a disposable, fully migrated PostgreSQL database" >&2
  exit 2
fi
if [[ "${RUNTIME_ROLE_SMOKE_CONFIRM_DISPOSABLE:-}" != "yes" ]]; then
  echo "Set RUNTIME_ROLE_SMOKE_CONFIRM_DISPOSABLE=yes; this test creates and drops login roles" >&2
  exit 2
fi
if ! command -v go >/dev/null 2>&1; then
  echo "go is required" >&2
  exit 2
fi

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -n "${PSQL_DOCKER_CONTAINER:-}" ]]; then
  if ! command -v docker >/dev/null 2>&1 || [[ -z "${PSQL_DATABASE_URL:-}" ]]; then
    echo "Docker psql mode requires docker and an internal PSQL_DATABASE_URL" >&2
    exit 2
  fi
elif ! command -v psql >/dev/null 2>&1; then
  echo "psql is required (or set PSQL_DOCKER_CONTAINER and PSQL_DATABASE_URL)" >&2
  exit 2
fi

psql_client() {
  if [[ -n "${PSQL_DOCKER_CONTAINER:-}" ]]; then
    shift # The Go tests still use the host TEST_DATABASE_URL.
    docker run --rm -i --network "container:$PSQL_DOCKER_CONTAINER" \
      -v "$repository_root:$repository_root:ro" \
      "${POSTGRES_CLIENT_IMAGE:-postgres:16.15-alpine@sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685}" \
      psql "$PSQL_DATABASE_URL" "$@"
  else
    psql "$@"
  fi
}
role_suffix="${PPID}_$$"
migration_role="cf_migration_smoke_${role_suffix}"
api_role="cf_api_smoke_${role_suffix}"
worker_role="cf_worker_smoke_${role_suffix}"
nested_role="cf_nested_smoke_${role_suffix}"

cleanup() {
  psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
    -v migration_role="$migration_role" -v api_role="$api_role" -v worker_role="$worker_role" -v nested_role="$nested_role" <<'SQL' >/dev/null 2>&1 || true
REVOKE complianceforge_evidence_chain_owner, complianceforge_evidence_registry_owner FROM :"migration_role";
REVOKE complianceforge_api FROM :"api_role";
REVOKE complianceforge_scheduler FROM :"worker_role";
DROP OWNED BY :"api_role", :"worker_role", :"migration_role";
DROP ROLE IF EXISTS :"api_role", :"worker_role", :"migration_role", :"nested_role";
SQL
}
trap cleanup EXIT

psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
  -v migration_role="$migration_role" -v api_role="$api_role" -v worker_role="$worker_role" <<'SQL'
CREATE ROLE :"migration_role" LOGIN INHERIT NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;
CREATE ROLE :"api_role" LOGIN INHERIT NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;
CREATE ROLE :"worker_role" LOGIN INHERIT NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;
SQL

psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
  -v migration_role="$migration_role" -v api_role="$api_role" -v worker_role="$worker_role" \
  -f "$repository_root/deployments/postgres/runtime-roles.sql"

# NOINHERIT does not prevent SET ROLE. A nested fixed-group membership must be
# rejected by provisioning before any reviewed grant matrix is applied.
psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 -v nested_role="$nested_role" <<'SQL'
CREATE ROLE :"nested_role" NOLOGIN NOSUPERUSER NOBYPASSRLS;
GRANT :"nested_role" TO complianceforge_api;
SQL
if psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
  -v migration_role="$migration_role" -v api_role="$api_role" -v worker_role="$worker_role" \
  -f "$repository_root/deployments/postgres/runtime-roles.sql" >/dev/null 2>&1; then
  echo "runtime-roles accepted an unsafe nested fixed-group membership" >&2
  exit 1
fi
psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 -v nested_role="$nested_role" <<'SQL'
REVOKE :"nested_role" FROM complianceforge_api;
DROP ROLE :"nested_role";
SQL

apply_grants() {
  psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
    -v api_role="$api_role" -v worker_role="$worker_role" \
    -f "$repository_root/deployments/postgres/runtime-grants.sql"
}

# Apply twice to prove the normal path is idempotent, then deliberately add
# stale direct and PUBLIC grants and require a third application to erase them.
apply_grants
apply_grants
psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
  -v api_role="$api_role" -v worker_role="$worker_role" <<'SQL'
GRANT TRUNCATE ON TABLE public.risks TO :"api_role";
GRANT UPDATE ON TABLE public.users TO :"worker_role";
GRANT UPDATE (title) ON TABLE public.risks TO :"api_role";
GRANT SELECT (email) ON TABLE public.users TO PUBLIC;
GRANT SELECT ON TABLE public.queue_outbox TO PUBLIC;
GRANT EXECUTE ON FUNCTION public.notification_due_tenants(INTEGER) TO :"api_role", PUBLIC;
GRANT EXECUTE ON FUNCTION public.resolve_api_key_tenant(TEXT), public.resolve_scim_token_tenant(TEXT) TO PUBLIC;
GRANT CREATE, USAGE ON SCHEMA public TO PUBLIC;
SELECT format('GRANT TEMPORARY ON DATABASE %I TO PUBLIC',current_database()) \gexec
SQL
apply_grants

psql_client "$TEST_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
  -v api_role="$api_role" -v worker_role="$worker_role" <<'SQL'
SELECT set_config('complianceforge.smoke.api_role', :'api_role', false);
SELECT set_config('complianceforge.smoke.worker_role', :'worker_role', false);
DO $assert_converged$
DECLARE
    api_role_name TEXT := current_setting('complianceforge.smoke.api_role');
    worker_role_name TEXT := current_setting('complianceforge.smoke.worker_role');
    stale_count INTEGER;
BEGIN
    IF has_database_privilege('public',current_database(),'CREATE')
       OR has_database_privilege('public',current_database(),'TEMPORARY')
       OR has_database_privilege(api_role_name,current_database(),'TEMPORARY')
       OR has_database_privilege(worker_role_name,current_database(),'TEMPORARY')
       OR has_schema_privilege('public','public','USAGE')
       OR has_schema_privilege('public','public','CREATE') THEN
        RAISE EXCEPTION 'stale PUBLIC/runtime database or schema grant survived convergence';
    END IF;
    IF has_table_privilege(api_role_name, 'public.risks', 'TRUNCATE') THEN
        RAISE EXCEPTION 'stale API TRUNCATE grant survived convergence';
    END IF;
    IF has_table_privilege(worker_role_name, 'public.users', 'UPDATE') THEN
        RAISE EXCEPTION 'stale worker UPDATE grant survived convergence';
    END IF;
    IF has_function_privilege(api_role_name, 'public.notification_due_tenants(integer)', 'EXECUTE') THEN
        RAISE EXCEPTION 'API can execute worker-only tenant discovery after convergence';
    END IF;
    IF has_function_privilege(worker_role_name, 'public.notification_due_tenants(integer)', 'EXECUTE')
       OR has_function_privilege(worker_role_name, 'public.incident_due_tenants(integer)', 'EXECUTE')
       OR has_function_privilege(worker_role_name, 'public.vendor_due_tenants(integer)', 'EXECUTE') THEN
        RAISE EXCEPTION 'worker retains legacy discovery EXECUTE after convergence';
    END IF;

    SELECT count(*) INTO stale_count
    FROM pg_catalog.pg_class AS relation
    JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid=relation.relnamespace
    CROSS JOIN LATERAL aclexplode(COALESCE(
        relation.relacl,
        acldefault(CASE WHEN relation.relkind='S' THEN 'S'::"char" ELSE 'r'::"char" END, relation.relowner)
    )) AS privilege
    WHERE namespace.nspname='public'
      AND relation.relkind IN ('r','p','v','m','f','S')
      AND privilege.grantee=0;
    IF stale_count <> 0 THEN
        RAISE EXCEPTION 'PUBLIC retains % table/sequence grants after convergence', stale_count;
    END IF;

    SELECT count(*) INTO stale_count
    FROM pg_catalog.pg_proc AS routine
    JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid=routine.pronamespace
    CROSS JOIN LATERAL aclexplode(COALESCE(routine.proacl, acldefault('f', routine.proowner))) AS privilege
    WHERE namespace.nspname='public'
      AND privilege.grantee=0
      AND routine.oid IN (
          'public.notification_due_tenants(integer)'::regprocedure,
          'public.incident_due_tenants(integer)'::regprocedure,
          'public.vendor_due_tenants(integer)'::regprocedure,
          'public.evidence_due_tenants(integer,uuid)'::regprocedure,
          'public.expire_due_evidence(uuid,integer)'::regprocedure,
          'public.resolve_api_key_tenant(text)'::regprocedure,
          'public.resolve_scim_token_tenant(text)'::regprocedure,
          'public.submit_evidence_review(uuid,uuid,uuid,uuid,text,text,text)'::regprocedure,
          'public.append_evidence_access_custody_event(uuid,uuid,uuid,uuid,text,text,text)'::regprocedure,
          'public.verify_evidence_custody_chain(uuid,uuid)'::regprocedure
      );
    IF stale_count <> 0 THEN
        RAISE EXCEPTION 'PUBLIC retains EXECUTE on % privileged capabilities', stale_count;
    END IF;

    SELECT count(*) INTO stale_count
    FROM (
        SELECT relation.relacl AS acl, relation.relowner AS owner_oid
        FROM pg_catalog.pg_class AS relation
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid=relation.relnamespace
        WHERE namespace.nspname='public'
        UNION ALL
        SELECT attribute.attacl, relation.relowner
        FROM pg_catalog.pg_attribute AS attribute
        JOIN pg_catalog.pg_class AS relation ON relation.oid=attribute.attrelid
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid=relation.relnamespace
        WHERE namespace.nspname='public' AND attribute.attnum>0 AND NOT attribute.attisdropped
        UNION ALL
        SELECT routine.proacl, routine.proowner
        FROM pg_catalog.pg_proc AS routine
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid=routine.pronamespace
        WHERE namespace.nspname='public'
    ) AS object_acl
    CROSS JOIN LATERAL aclexplode(COALESCE(object_acl.acl, acldefault('f', object_acl.owner_oid))) AS privilege
    WHERE privilege.grantee IN (api_role_name::regrole, worker_role_name::regrole);
    IF stale_count <> 0 THEN
        RAISE EXCEPTION 'runtime logins retain % direct object grants; grants must flow through fixed groups', stale_count;
    END IF;
    SELECT count(*) INTO stale_count
    FROM pg_catalog.pg_attribute AS attribute
    JOIN pg_catalog.pg_class AS relation ON relation.oid=attribute.attrelid
    JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid=relation.relnamespace
    CROSS JOIN LATERAL aclexplode(attribute.attacl) AS privilege
    WHERE namespace.nspname='public' AND privilege.grantee=0;
    IF stale_count <> 0 THEN
        RAISE EXCEPTION 'PUBLIC retains % column grants after convergence', stale_count;
    END IF;
END;
$assert_converged$;
SQL

(
  cd "$repository_root"
  TEST_DATABASE_URL="$TEST_DATABASE_URL" \
  API_POSTURE_TEST_ROLE="$api_role" \
  WORKER_POSTURE_TEST_ROLE="$worker_role" \
  GOTOOLCHAIN="${GOTOOLCHAIN:-go1.26.8}" \
    go test -tags=integration -race ./internal/database ./internal/worker \
      -run 'TestValidateAPIDatabasePostureLive|TestSeparatedRuntimeRolesLive' -count=1
)

echo "runtime database role manifests and split-role smoke passed"
