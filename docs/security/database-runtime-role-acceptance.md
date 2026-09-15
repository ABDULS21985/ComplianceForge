# Database runtime identity and scheduler acceptance

Bounded W34 acceptance dated 2026-09-15, reviewed against clean schema 58.
This is executable identity/ACL/RLS and scheduler evidence, not a claim that
all enterprise features or production disaster recovery are certified.
The [operator runbook](../runbooks/database-role-separation.md) defines the
provisioning, migration, convergence and controlled pool-recycling sequence.

## Implemented boundaries

- Migration, API and worker credentials are separate. The migrator prefers
  `MIGRATION_DATABASE_URL`; production Compose requires `API_DATABASE_URL`
  and `WORKER_DATABASE_URL` without passing the migration secret to runtime.
- Production startup binds both `current_user` and `session_user` to the
  actual parsed pgx login before checking catalog posture. A privileged
  credential hidden by `SET ROLE` or `SET SESSION AUTHORIZATION` fails closed;
  binding errors contain no role, credential or connection URL.
- Reviewed fixed groups and logins cannot inherit unexpected roles, schema
  ownership, superuser/BYPASSRLS, or either evidence capability owner.
  Provisioning also rejects nested fixed-group memberships.
- The explicit per-table/per-verb API and scheduler manifests are guarded by
  exactly one clean schema-58 row. Convergence removes stale group/login/
  PUBLIC table, column, sequence and function ACLs, database CREATE/TEMPORARY,
  and public-schema access before rebuilding the reviewed grants. API
  availability is checked against the same exact matrix, not only evidence
  wrapper capabilities. Read/append ledgers and soft-delete projections have
  narrower grants than mutable membership/credential bindings.
- Both queues require ENABLE+FORCE RLS and the exact reviewed policies.
  API has same-tenant outbox SELECT/INSERT and inbox SELECT only; other-tenant
  and NULL/system rows remain invisible and uninsertable. Scheduler policies
  permit tenant/system delivery without BYPASSRLS. Neither gets TRUNCATE.
- Cross-tenant runtime discovery uses the active/nondeleted registry's
  bounded keyset pages and one tenant-scoped connection per callback. The
  old notification/incident/vendor due functions are denied to PUBLIC/API/
  worker, avoiding privileged-owner dependence and first-1000 starvation.
  Worker capabilities are the reviewed registry and bounded evidence-expiry
  wrappers only, in addition to the documented invoker-helper/getter class.
- Definer search paths explicitly place `pg_temp` last. Runtime TEMP is
  revoked; already-open pools still require a controlled rolling recycle,
  never automated termination of unrelated sessions.

## Executed proof

All Go commands below used `GOTOOLCHAIN=go1.26.8`. PostgreSQL proof used an
independent disposable PostgreSQL 16 database; no root validation database
was mutated by this workstream.

| Gate | Observed result |
| --- | --- |
| Real LOGIN ordinary migrator, NOSUPERUSER/NOBYPASSRLS, database owner | Clean 0→58 apply passed; 58→57→58 passed, dirty=false throughout successful states. No migrator BYPASSRLS elevation. |
| Final `scripts/validate-runtime-database-roles.sh` | Passed. Manifest applied twice, stale privileges injected, third apply converged; nested-group escalation rejected. |
| Final actual LOGIN database/worker integration with `-race` | Passed: database 3.815s, worker 11.826s. Runtime pools used distinct disposable logins with random test-only passwords, not SET ROLE. |
| Final `go test -race -p 2 ./...` after login-binding changes | Passed across all packages. |
| Final `go vet -p 2 ./...` | Passed. |
| Login-binding adversarial unit and real PostgreSQL regression | Passed, including privileged authenticated connection hidden by SET ROLE and safe query-error redaction. |
| Exact matrix, append-only, queue policy and schema-pin unit checks | Passed as part of the database/race gates. |
| OpenAPI operator-selected file parsing tests | Passed: base 8 MiB ceiling, catalog 2 MiB ceiling, trailing JSON rejected; two narrowly justified operator-CLI G304 annotations only. |
| Pinned Gosec 2.28.0 | Full shared-source scan passed with zero findings before the binding delta; final database/API/worker delta scan also exited 0 with zero findings. Quiet mode produced no report file when clean. |
| Actionlint 1.7.12, Shellcheck 0.11.0, bash syntax | Passed for the updated workflow and runtime-role smoke script. CI uses the pinned PostgreSQL Docker client. |
| Development and production Compose config | Both passed; production used explicit synthetic validation inputs, including independent database URLs and existing TLS/SMTP/encryption/scanner/S3 settings. |
| `git diff --check` | Passed. |

The final catalog reported schema `58, dirty=false`, both queue RLS+FORCE
flags true, and zero temporary login roles from the smoke run. Earlier
schema-57 populated-race evidence is point-in-time only, not source-58
production readiness.

### Reproduce the bounded live acceptance

Provision an ordinary migration login with reviewed DDL/object ownership
and only the required non-login capability-owner memberships. On a fresh
disposable database:

```sh
MIGRATION_DATABASE_URL="$ORDINARY_MIGRATOR_TEST_URL" \
GOTOOLCHAIN=go1.26.8 go run ./cmd/migrate up

MIGRATION_DATABASE_URL="$ORDINARY_MIGRATOR_TEST_URL" \
GOTOOLCHAIN=go1.26.8 go run ./cmd/migrate down 1

MIGRATION_DATABASE_URL="$ORDINARY_MIGRATOR_TEST_URL" \
GOTOOLCHAIN=go1.26.8 go run ./cmd/migrate up 1

TEST_DATABASE_URL="$DISPOSABLE_PROVISIONING_URL" \
RUNTIME_ROLE_SMOKE_CONFIRM_DISPOSABLE=yes \
GOTOOLCHAIN=go1.26.8 ./scripts/validate-runtime-database-roles.sh
```

Without local psql, provide `PSQL_DOCKER_CONTAINER` and the internal
`PSQL_DATABASE_URL` as described in the runbook. The actual-login fixture
requires explicit disposable confirmation and script-created temporary
role prefixes; it never rotates an existing application login password.

### Populated scheduler coverage

Real non-owner runtime connections exercised API tenant read/insert/update/
soft-delete and physical-delete denial; invoker effective-role dependencies;
tenant/system outbox/inbox delivery and cross-tenant denial; and distributed
lease acquisition, renewal and release. Keyset traversal used page size 1
to prove both active tenants were visited while an inactive tenant was not.

Scheduler smoke executed all composed query classes plus populated fixtures:
the three analytics snapshot types; seven regulatory checks with due/future
and cross-tenant records; encrypted DSR sentinel non-disclosure and extended
deadlines; calendar reminders, escalation and valid status transitions;
repeat-poll notification/event identities; guarded exception expiry and one
audit append; evidence capabilities; all search source definitions and
current-version policy content in full/incremental indexing; workflow timer
queries; and concurrent report replicas producing one occurrence with atomic
run creation/advancement and configured calendar time. Unit tests additionally
cover DST wall-clock advancement and missed-occurrence coalescing. Root's
independent actual-LOGIN notification fairness fixture covered 1025 active
tenants plus disabled/soft-deleted organizations.

## Explicit remaining limitations

- Production rollout still needs DBA/secret-manager identities, verified
  database TLS, network controls, canary startup and complete pool recycling.
  The disposable PostgreSQL lab is not evidence of that external rollout.
- The scalar SECURITY INVOKER helper class is documented and constrained by
  caller privileges/RLS, not an exact function-name allowlist. PUBLIC database
  CONNECT retains PostgreSQL's documented default; authentication/network
  policy must be separately enforced.
- New schema versions need a separately reviewed exact ACL/capability delta,
  source readiness pin, convergence and split-role proof. Schema 58 evidence
  must not be relabeled as certification of later migrations.
- Process-local EventBus handoff can lose events before durable outbox
  enqueue on crash/full channel; stable IDs only deduplicate events that
  reached the outbox. Exception expiry audit/status is transactional, but its
  bus publication remains outside that transaction.
- Report occurrence claim/run/advancement is atomic; report artifact
  generation and delivery remain separate work. Workflow timer completion
  does not yet advance/create the next execution; those roadmap items remain
  partial.
- Optional/uncomposed legacy domains are not claimed as production APIs by
  this manifest. Explicit legacy read/update offboarding bindings support
  actual composed user cleanup without enabling those modules.
- The separate LGPL Security/Legal review remains visibly pending; no
  exception, license approval or weakened vulnerability gate was introduced.
