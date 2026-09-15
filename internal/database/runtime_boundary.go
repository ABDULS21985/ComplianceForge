package database

// This pin moves only with a reviewed SQL privilege manifest, not automatically
// with a schema bump. Tests force the deployment matrix and supported schema to
// move together before new production source can start.
const reviewedRuntimeSchemaVersion int64 = 59

// runtimeBoundaryProjection is shared by the production API and worker
// catalog gates. $1 is the role's explicit callable SECURITY DEFINER allowlist;
// $2 is the separately reviewed runtime privilege schema version.
// Invoker scalar helpers cannot elevate the reviewed table/RLS privileges;
// trigger-only functions have no runtime caller EXECUTE requirement.
const runtimeBoundaryProjection = `,
       ARRAY(
           SELECT 'database:' || privilege.kind || ':PUBLIC'
           FROM (VALUES ('CREATE'::TEXT), ('TEMPORARY'::TEXT)) AS privilege(kind)
           WHERE pg_catalog.has_database_privilege('public', current_database(), privilege.kind)
           UNION ALL
           SELECT 'public:' || privilege.privilege_type || ':PUBLIC'
           FROM pg_catalog.pg_namespace AS namespace
           CROSS JOIN LATERAL pg_catalog.aclexplode(COALESCE(namespace.nspacl, pg_catalog.acldefault('n', namespace.nspowner))) AS privilege
           WHERE namespace.nspname = 'public' AND privilege.grantee = 0
           UNION ALL
           SELECT format('public.%I:%s:PUBLIC', relation.relname, privilege.privilege_type)
           FROM pg_catalog.pg_class AS relation
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
           CROSS JOIN LATERAL pg_catalog.aclexplode(COALESCE(
               relation.relacl,
               pg_catalog.acldefault(CASE WHEN relation.relkind = 'S' THEN 'S'::"char" ELSE 'r'::"char" END, relation.relowner)
           )) AS privilege
           WHERE namespace.nspname = 'public'
             AND relation.relkind IN ('r', 'p', 'v', 'm', 'f', 'S')
             AND privilege.grantee = 0
           UNION ALL
           SELECT format('public.%I.%I:%s:PUBLIC', relation.relname, attribute.attname, privilege.privilege_type)
           FROM pg_catalog.pg_attribute AS attribute
           JOIN pg_catalog.pg_class AS relation ON relation.oid = attribute.attrelid
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
           CROSS JOIN LATERAL pg_catalog.aclexplode(attribute.attacl) AS privilege
           WHERE namespace.nspname = 'public'
             AND attribute.attnum > 0 AND NOT attribute.attisdropped
             AND privilege.grantee = 0
           UNION ALL
           SELECT routine.oid::regprocedure::text || ':EXECUTE:PUBLIC'
           FROM pg_catalog.pg_proc AS routine
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = routine.pronamespace
           WHERE namespace.nspname = 'public'
             AND pg_catalog.has_function_privilege('public', routine.oid, 'EXECUTE')
           ORDER BY 1
       )::TEXT[],
       ARRAY(
           SELECT routine.oid::regprocedure::text
           FROM pg_catalog.pg_proc AS routine
           JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = routine.pronamespace
           WHERE namespace.nspname = 'public'
             AND routine.prosecdef
             AND pg_catalog.has_function_privilege(current_user, routine.oid, 'EXECUTE')
             AND NOT EXISTS (
                 SELECT 1 FROM unnest($1::TEXT[]) AS allowed(signature)
                 WHERE pg_catalog.to_regprocedure(allowed.signature) = routine.oid
             )
           ORDER BY 1
       )::TEXT[],
       ARRAY(
           WITH queue_relations AS (
               SELECT relation.oid, relation.relname, relation.relowner,
                      relation.relrowsecurity, relation.relforcerowsecurity
               FROM pg_catalog.pg_class AS relation
               WHERE relation.oid IN (
                   pg_catalog.to_regclass('public.queue_outbox'),
                   pg_catalog.to_regclass('public.queue_inbox')
               )
           ), expected_policies AS (
               SELECT relation.oid AS relation_oid,
                      relation.relname || suffix.name AS policy_name,
                      suffix.command,
                      CASE suffix.kind
                          WHEN 'tenant' THEN '((tenant_id IS NOT NULL) AND (tenant_id = get_current_tenant()))'
                          WHEN 'scheduler' THEN '(EXISTS ( SELECT 1 FROM pg_roles scheduler_role WHERE ((scheduler_role.rolname = ''complianceforge_scheduler''::name) AND pg_has_role(CURRENT_USER, scheduler_role.oid, ''MEMBER''::text))))'
                          WHEN 'owner' THEN format('((CURRENT_USER = SESSION_USER) AND (CURRENT_USER = %L::name))', pg_catalog.pg_get_userbyid(relation.relowner))
                      END AS expression,
                      suffix.use_qual, suffix.use_check
               FROM queue_relations AS relation
               CROSS JOIN (VALUES
                   ('_tenant_select'::TEXT, 'r'::"char", 'tenant'::TEXT, TRUE, FALSE),
                   ('_scheduler_all', '*'::"char", 'scheduler', TRUE, TRUE),
                   ('_owner_all', '*'::"char", 'owner', TRUE, TRUE)
               ) AS suffix(name, command, kind, use_qual, use_check)
               UNION ALL
               SELECT pg_catalog.to_regclass('public.queue_outbox'),
                      'queue_outbox_tenant_insert', 'a'::"char",
                      '((tenant_id IS NOT NULL) AND (tenant_id = get_current_tenant()))',
                      FALSE, TRUE
           )
           SELECT required.name || ':FORCE_RLS'
           FROM (VALUES ('queue_outbox'::TEXT), ('queue_inbox'::TEXT)) AS required(name)
           LEFT JOIN queue_relations AS relation ON relation.relname = required.name
           WHERE relation.oid IS NULL OR NOT relation.relrowsecurity OR NOT relation.relforcerowsecurity
           UNION ALL
           SELECT expected.policy_name || ':POLICY'
           FROM expected_policies AS expected
           LEFT JOIN pg_catalog.pg_policy AS policy
             ON policy.polrelid = expected.relation_oid AND policy.polname = expected.policy_name
           WHERE policy.oid IS NULL
              OR policy.polcmd <> expected.command
              OR NOT policy.polpermissive
              OR policy.polroles <> ARRAY[0::OID]
              OR regexp_replace(replace(replace(COALESCE(pg_catalog.pg_get_expr(policy.polqual, policy.polrelid), ''), 'public.', ''), 'pg_catalog.', ''), '\s', '', 'g')
                 <> regexp_replace(CASE WHEN expected.use_qual THEN expected.expression ELSE '' END, '\s', '', 'g')
              OR regexp_replace(replace(replace(COALESCE(pg_catalog.pg_get_expr(policy.polwithcheck, policy.polrelid), ''), 'public.', ''), 'pg_catalog.', ''), '\s', '', 'g')
                 <> regexp_replace(CASE WHEN expected.use_check THEN expected.expression ELSE '' END, '\s', '', 'g')
           UNION ALL
           SELECT policy.polname || ':UNEXPECTED_POLICY'
           FROM pg_catalog.pg_policy AS policy
           JOIN queue_relations AS relation ON relation.oid = policy.polrelid
           WHERE NOT EXISTS (
               SELECT 1 FROM expected_policies AS expected
               WHERE expected.relation_oid = policy.polrelid AND expected.policy_name = policy.polname
           )
           UNION ALL
           SELECT 'get_current_tenant():EXECUTE'
           WHERE NOT COALESCE(pg_catalog.has_function_privilege(
               current_user, pg_catalog.to_regprocedure('public.get_current_tenant()'), 'EXECUTE'
           ), FALSE)
           UNION ALL
           SELECT 'runtime role:TEMPORARY'
           WHERE pg_catalog.has_database_privilege(current_user, current_database(), 'TEMPORARY')
           UNION ALL
           SELECT 'runtime schema:VERSION_OR_DIRTY'
           WHERE NOT EXISTS (
               SELECT 1 FROM public.schema_migrations
               HAVING count(*) = 1 AND bool_and(version = $2::BIGINT AND NOT dirty)
           )
           ORDER BY 1
       )::TEXT[]
`
