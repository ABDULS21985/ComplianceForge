# Data lifecycle governance

ComplianceForge provides tenant-scoped policy, retention, exception, legal-hold,
and tamper-evident history controls under
`/api/v1/settings/data-governance`. The feature is protected by the
`data_lifecycle` entitlement and the persisted `settings:read` or
`settings:configure` permission for each operation.

## Control model

1. A tenant administrator records the approved primary region, allowed
   regions, cross-border transfer posture, default retention period, archive
   timing, deletion grace period, and disposition approval mode.
2. A records manager publishes scoped retention schedules. A schedule selects
   a supported record type, classification, jurisdiction, trigger, legal
   basis, duration, disposition action, priority, and effective dates.
3. A materialised assignment binds a specific record UUID to the schedule and
   calculates its archive and disposition dates. The assignment persists even
   after disposition so it can remain evidence.
4. If review is required, an authorised reviewer records an approve or reject
   decision with an optimistic version and mandatory reason.
5. A retention exception puts the assignment into a fail-closed pending state.
   Approval extends the disposition date and resets any required review;
   rejection returns the assignment to its prior active lifecycle.
6. A legal hold can identify an owner and custodians and bind any supported
   record. An active hold always takes precedence over retention eligibility.
7. Every governance mutation appends an actor-, tenant-, request-, reason-, and
   before/after-attributed event and a durable transactional-outbox message.

Supported governed record types are assets, audits, audit findings, comments,
control implementations, evidence, incidents, policies, report runs, risks,
and vendors.

## Fail-closed database enforcement

Once a tenant has configured its governance policy, the database rejects hard
or supported soft deletion when any of these conditions is true:

- the record has no retention assignment;
- its disposition date has not elapsed;
- a disposition review is required but not approved;
- a retention exception is pending; or
- an active legal-hold record protects it.

The pre-existing `legal_hold` flags on incidents and vendors remain enforced
as an additional migration-safe guard. The checks execute in PostgreSQL, not
only in HTTP handlers, so direct application writes and cascades cannot bypass
them. Tenant identifiers are compared with the request-scoped PostgreSQL
tenant context and governance tables use forced row-level security.

The migration deliberately permits a privileged connection with no tenant
context to perform migration or controlled teardown work. Application roles
must never receive ownership, superuser, `BYPASSRLS`, trigger-disable, or
unrestricted function privileges.

## API surface

| Area | Operations |
| --- | --- |
| Policy | Read and versioned create/update |
| Schedules | List, create, read, versioned update, and reasoned retirement |
| Assignments | Create, read, evaluate disposition, review, list exceptions, and request an exception |
| Exceptions | Versioned approve or reject decision |
| Legal holds | List, create, read, versioned update, release/cancel, list records, add record, and release record |
| History | Paginated event search and full tenant-chain verification |

All JSON bodies are strictly decoded. Mutations require a human-readable
reason, IDs are UUID-validated, list sizes and text lengths are bounded, and
mutable resources use optimistic versions. List responses use the canonical
`data` plus `pagination` envelope; errors use the canonical safe error envelope
with a stable machine code and request ID.

## Tamper-evident history

Events are ordered per tenant. PostgreSQL locks a tenant chain head, derives
the next sequence, hashes the canonical event fields with SHA-256, records the
previous hash, and advances the head in the same transaction. Update and delete
triggers make the event table append-only. The verification endpoint recomputes
the complete chain and reports the first failure without exposing another
tenant's events.

This is database-level tamper evidence, not external notarisation. A database
owner could disable controls. High-assurance deployments should additionally
export signed checkpoints to immutable storage and restrict database ownership
through separation of duties.

## Operational verification

Before enabling the capability for a tenant:

- obtain Legal/Privacy approval for the policy and schedules;
- confirm that every in-scope record family has an assignment backfill plan;
- verify the application database role is non-superuser and cannot bypass RLS;
- exercise a held-record and pending-exception deletion denial in staging;
- verify the event chain and outbox backlog are healthy; and
- document the authorised disposition operator and approval separation.

Run the focused acceptance checks with an isolated migrated PostgreSQL database:

```sh
TEST_DATABASE_URL='postgres://…' \
  go test -tags=integration -race -run TestDataGovernanceWithNonSuperuserTenants \
  -count=1 -v ./internal/repository
```

The acceptance test covers two-tenant forced-RLS isolation, optimistic
conflicts, outbox rollback, concurrent hash-chain allocation, schedule and
assignment decisions, pending and approved exceptions, both modern and legacy
legal holds, database deletion denial, history immutability, and chain
verification.

## Incident handling

If chain verification fails, stop governance mutations for the affected tenant,
preserve database and application logs, record the last known signed backup or
checkpoint, and escalate to Security and Legal. Do not rewrite, truncate, or
"repair" event rows in place. Reconciliation must produce a separately
approved evidence record and retain the original database snapshot.

If a required deletion is blocked, inspect the disposition endpoint first.
Resolve the reported missing assignment, active duration, review, exception, or
hold through its normal audited workflow. Never disable the enforcement trigger
to satisfy an operational request.

## Deliberate boundaries

This release establishes policy and enforcement foundations. It does not yet
physically relocate data between regions, propagate deletion to processors,
execute automatic purge/anonymisation/archive jobs, issue signed deletion
certificates, or notarise chain checkpoints outside PostgreSQL. Those controls
must remain marked partial until their infrastructure and operator workflows
are implemented and tested.
