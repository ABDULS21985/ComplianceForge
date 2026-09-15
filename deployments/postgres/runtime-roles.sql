\set ON_ERROR_STOP on

-- Run as a PostgreSQL role with CREATEROLE before applying migrations.
-- Runtime passwords stay in the deployment secret manager; this script only
-- creates fixed NOLOGIN groups/owners and binds pre-created login roles.
\if :{?migration_role}
\else
  \echo 'runtime-roles.sql requires -v migration_role=<login>'
  \quit 3
\endif
\if :{?api_role}
\else
  \echo 'runtime-roles.sql requires -v api_role=<login>'
  \quit 3
\endif
\if :{?worker_role}
\else
  \echo 'runtime-roles.sql requires -v worker_role=<login>'
  \quit 3
\endif

BEGIN;
SELECT pg_catalog.set_config('complianceforge.provision.migration_role', :'migration_role', true);
SELECT pg_catalog.set_config('complianceforge.provision.api_role', :'api_role', true);
SELECT pg_catalog.set_config('complianceforge.provision.worker_role', :'worker_role', true);

DO $provision$
DECLARE
    fixed_role TEXT;
    candidate RECORD;
    migration_role_name TEXT := current_setting('complianceforge.provision.migration_role');
    api_role_name TEXT := current_setting('complianceforge.provision.api_role');
    worker_role_name TEXT := current_setting('complianceforge.provision.worker_role');
BEGIN
    IF migration_role_name = api_role_name
       OR migration_role_name = worker_role_name
       OR api_role_name = worker_role_name THEN
        RAISE EXCEPTION 'migration, API, and worker login roles must be distinct'
            USING ERRCODE = '22023';
    END IF;

    FOREACH fixed_role IN ARRAY ARRAY[
        'complianceforge_api',
        'complianceforge_scheduler',
        'complianceforge_evidence_chain_owner',
        'complianceforge_evidence_registry_owner'
    ] LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = fixed_role) THEN
            EXECUTE format(
                'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS',
                fixed_role
            );
        END IF;
        SELECT role.* INTO candidate
        FROM pg_catalog.pg_roles AS role WHERE role.rolname = fixed_role;
        IF candidate.rolcanlogin OR candidate.rolsuper OR candidate.rolbypassrls
           OR candidate.rolcreatedb OR candidate.rolcreaterole OR candidate.rolreplication
           OR candidate.rolinherit THEN
            RAISE EXCEPTION '% must be NOLOGIN, NOSUPERUSER, NOBYPASSRLS, NOCREATEDB, NOCREATEROLE, NOREPLICATION, and NOINHERIT',
                fixed_role USING ERRCODE = '42501';
        END IF;
		IF EXISTS (
			SELECT 1
			FROM pg_catalog.pg_auth_members AS membership
			WHERE membership.member = candidate.oid
		) THEN
			RAISE EXCEPTION 'fixed capability role % must not inherit any other role', fixed_role
				USING ERRCODE = '42501';
		END IF;
    END LOOP;

    FOREACH fixed_role IN ARRAY ARRAY[migration_role_name, api_role_name, worker_role_name] LOOP
        SELECT role.* INTO candidate
        FROM pg_catalog.pg_roles AS role WHERE role.rolname = fixed_role;
        IF NOT FOUND OR NOT candidate.rolcanlogin THEN
            RAISE EXCEPTION 'login role % must be pre-created', fixed_role USING ERRCODE = '42704';
        END IF;
    END LOOP;

    FOREACH fixed_role IN ARRAY ARRAY[api_role_name, worker_role_name] LOOP
        SELECT role.* INTO candidate
        FROM pg_catalog.pg_roles AS role WHERE role.rolname = fixed_role;
        IF candidate.rolsuper OR candidate.rolbypassrls OR candidate.rolcreatedb
           OR candidate.rolcreaterole OR candidate.rolreplication OR NOT candidate.rolinherit THEN
            RAISE EXCEPTION 'runtime login % must be INHERIT, NOSUPERUSER, NOBYPASSRLS, NOCREATEDB, NOCREATEROLE, and NOREPLICATION',
                fixed_role USING ERRCODE = '42501';
        END IF;
    END LOOP;

    IF pg_catalog.pg_has_role(api_role_name, 'complianceforge_scheduler', 'MEMBER')
       OR pg_catalog.pg_has_role(api_role_name, 'complianceforge_evidence_chain_owner', 'MEMBER')
       OR pg_catalog.pg_has_role(api_role_name, 'complianceforge_evidence_registry_owner', 'MEMBER') THEN
        RAISE EXCEPTION 'API login % has a forbidden worker/owner membership', api_role_name
            USING ERRCODE = '42501';
    END IF;
    IF pg_catalog.pg_has_role(worker_role_name, 'complianceforge_api', 'MEMBER')
       OR pg_catalog.pg_has_role(worker_role_name, 'complianceforge_evidence_chain_owner', 'MEMBER')
       OR pg_catalog.pg_has_role(worker_role_name, 'complianceforge_evidence_registry_owner', 'MEMBER') THEN
        RAISE EXCEPTION 'worker login % has a forbidden API/owner membership', worker_role_name
            USING ERRCODE = '42501';
    END IF;

    EXECUTE format('GRANT complianceforge_evidence_chain_owner TO %I', migration_role_name);
    EXECUTE format('GRANT complianceforge_evidence_registry_owner TO %I', migration_role_name);
    EXECUTE format('GRANT complianceforge_api TO %I', api_role_name);
    EXECUTE format('GRANT complianceforge_scheduler TO %I', worker_role_name);
END;
$provision$;
COMMIT;
