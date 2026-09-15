# Production database identity and runtime grants

Use three distinct secret-manager credentials; do not reuse a connection URL.

| Identity | Input | Responsibility |
| --- | --- | --- |
| Migration | `MIGRATION_DATABASE_URL` | One-shot reviewed schema changes and transfer to non-login capability owners. Never an API/worker credential. |
| API | `API_DATABASE_URL` | Tenant-scoped request queries, explicit composed-handler table/verb matrix, and approved evidence/API-key/SCIM capabilities. |
| Worker | `WORKER_DATABASE_URL` | Tenant-scoped jobs, bounded tenant discovery/expiry capabilities, cross-tenant queue leasing and scheduler coordination. |

Production Compose requires API/worker-specific inputs and maps each to its own
process-local `DATABASE_URL`. The migrator prefers `MIGRATION_DATABASE_URL`;
`DATABASE_URL` remains a local/legacy fallback only. Production connections must
require TLS with verified trust. Never supply migration credentials to runtime
containers or call the API identity the schema owner.

## Provision, migrate, converge

Create distinct login roles through your DBA/secret-management process, then
run reviewed SQL as a privileged provisioning identity. The manifest does not
create or print passwords.

The DBA must also provision the migration login's database/schema DDL authority
and existing-object ownership before the one-shot migration job. For a fresh
deployment, create the application database with that login as owner and grant
its reviewed public-schema authority. This runtime-role script intentionally
does not transfer an existing database or arbitrary objects to a new login.
Runtime logins must never inherit that ownership/DDL authority.

```sh
psql "$PROVISION_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
  -v migration_role=app_migrator -v api_role=app_api -v worker_role=app_worker \
  -f deployments/postgres/runtime-roles.sql

MIGRATION_DATABASE_URL="$MIGRATOR_SECRET_URL" ./migrate up

psql "$PROVISION_DATABASE_URL" -X -v ON_ERROR_STOP=1 \
  -v api_role=app_api -v worker_role=app_worker \
  -f deployments/postgres/runtime-grants.sql
```

`runtime-roles.sql` validates safe attributes, unrelated runtime group
memberships, and absence of nested memberships on the fixed groups.
`NOINHERIT` alone does not prevent `SET ROLE`. Only the migrator is a member of
the two evidence capability-owner roles; neither runtime login may assume them.
Capability owners are `NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE
NOREPLICATION NOINHERIT`.

`runtime-grants.sql` is an exact-version, clean-migration guarded deployment
step. It revokes existing group/login/PUBLIC ACLs before rebuilding an explicit
per-table/per-verb matrix. It does not grant API CRUD to every RLS relation.
Immutable event/certification stores are read/append only; soft-delete domains
receive UPDATE, not physical DELETE. Explicit offboarding bindings receive only
reviewed read/update permissions even when the legacy domain is disabled.
Run the manifest after every reviewed migration/grant change; newly introduced
tables receive no automatic runtime grant.
The current reviewed matrix is schema 59; runtime posture also requires exactly
one clean schema-migration row at that version. Timed role windows require API
user_roles UPDATE in addition to membership replacement; immutable assignment
identity/version and SoD/independent-approver guards remain database enforced.
The security-invoker effective role view requires reviewed base/SoD SELECT
permissions for both identities, not merely SELECT on the view itself.
Organization profile migration 059 adds no table/verb or elevated capability;
the existing scalar invoker helper class grants the contact CHECK validator.
Schema-58 acceptance remains historical evidence, not a schema-59 rollout proof.

PUBLIC has no public-schema, table, sequence, or function privileges after
convergence, and no database CREATE/TEMPORARY. Database CONNECT remains the
documented PostgreSQL default; authentication/network policy still controls
connections. Runtime identities cannot create temporary objects. Reviewed
definers (including trigger-only functions) pin `search_path` to
`pg_catalog,public,pg_temp`, with the temporary schema explicitly last.
Callable `SECURITY DEFINER` routines are allowlisted separately per identity;
API cannot discover other tenants through scheduler functions, and worker
cannot use API evidence submission/access wrappers. Trigger-only definers are
not executable by runtime callers. One audited legacy definer getter reads only
the caller's tenant session setting. Scalar `SECURITY INVOKER` helpers (including
pgcrypto/check/hash functions) are a documented helper class, not an exact
function-name matrix: they execute with the caller's reviewed table privileges
and RLS, never owner privileges. Review this class if a new helper adds effects.

## Fail-closed startup and row boundaries

Before listeners/jobs start, production API and worker verify required and
unexpected table/sequence privileges, callable definer capabilities, safe
function owners/search paths, group memberships, PUBLIC leakage, and protected
relation ownership. Both `current_user` and `session_user` must equal the
configured pgx connection login. A migration credential disguised with
`SET ROLE` or `SET SESSION AUTHORIZATION` is rejected before catalog posture;
startup errors never echo its login or connection URL. Proxies must preserve
the dedicated database identity rather than multiplexing privileged credentials.
Superuser, BYPASSRLS, database/schema ownership or CREATE,
owner membership, stale privileges, or missing availability permissions abort
startup. API additionally verifies `schema_migrations` read availability.
Local development may retain a single-role database; that is not a production
posture proof.

Both queue tables must have ENABLE+FORCE RLS and exact reviewed policies.
API outbox is same-tenant SELECT/INSERT; inbox is same-tenant SELECT only.
NULL/system and other-tenant messages are invisible/uninsertable to API.
Scheduler-group policies allow bounded cross-tenant delivery; neither identity
gets queue TRUNCATE. The local/migration owner policy requires exact current
and session owner identity and is not a runtime escape hatch.

Schedulers discover active/nondeleted tenants through a narrow registry
capability in keyset pages, close discovery rows, and process one tenant-scoped
connection per callback. Never replace this with BYPASSRLS, unscoped table
queries, or an unbounded global tenant slice.
Legacy notification/incident/vendor due-tenant functions are not runtime
capabilities. They depend on privileged migration ownership when scanning
FORCE-RLS tables; ordinary migrators are intentionally supported without
elevating them. Registry keyset traversal avoids that dependency and the
old first-1000-tenant starvation. Notification TenantBatch is a page size,
not a global tenant ceiling.

## Executable split-role smoke

Run only against a disposable, fully migrated PostgreSQL database. The script
creates/drops temporary login roles, rejects nested group escalation, reapplies
the manifest, injects stale login/PUBLIC grants, and proves convergence. Real
PostgreSQL race tests exercise API tenant read/write and denial, outbox/inbox
tenant+system rows, distributed leases, scheduler SQL, populated due/not-due
fixtures, calendar notification deduplication, indexing and report claims.
The end-to-end runtime pools use actual distinct LOGIN authentication with
random disposable-only passwords, and reject a privileged connection hidden
by `SET ROLE`. Password provisioning is restricted to the script-created
temporary role prefixes and explicit disposable-test confirmation; never
point this fixture at existing application credentials. The isolated read-only catalog-query test also uses an assumed
role; that test alone is not authenticated-login startup proof.

```sh
TEST_DATABASE_URL="$DISPOSABLE_ADMIN_URL" \
RUNTIME_ROLE_SMOKE_CONFIRM_DISPOSABLE=yes \
  ./scripts/validate-runtime-database-roles.sh
```

If local `psql` is unavailable, also set `PSQL_DOCKER_CONTAINER` to the disposable
database container and `PSQL_DATABASE_URL` to its internal URL. Go tests use the
host `TEST_DATABASE_URL`; Docker psql uses a pinned PostgreSQL client and
read-only repository mount.

A startup/smoke failure is an availability/security incident: repair grants or
secret mapping, restart one canary replica, then roll out. Do not weaken the
gate. Rotate credentials independently and repeat checks after role,
function-owner, policy, or grant changes.

Recycle API/worker pools through a controlled rolling restart after tightening
ACLs. Revoking TEMPORARY does not remove existing temporary relations from old
sessions. The hardened definer search path protects those sessions during
rollout, but a fresh canary session plus complete runtime pool recycling is the
required final posture. Do not terminate arbitrary user sessions automatically.

## Remaining delivery/workflow boundaries

Stable scheduled event IDs suppress repeats of the same entity/deadline/
threshold in the durable outbox. The first immutable payload wins if record
metadata changes within that threshold. Domain events still pass through a
bounded process-local EventBus before the outbox; a crash/full channel can lose
an event there. Move remaining producers to transactional enqueue before
claiming complete durable delivery.

Report claim/run creation/schedule advancement is transactional and guarded by
the exact due timestamp; calendar-time advancement preserves configured local
time across DST. Actual report artifact generation/delivery remains a separate
workflow. Workflow timer handling currently marks timers complete but does not
advance/create the next execution; workflow roadmap items remain partial.
