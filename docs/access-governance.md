# Access governance backend (schema 58)

This is a bounded backend capability, not a claim that roadmap item30 is fully verified. No frontend changes are included.

## Authority and administration

`effective_user_roles` is a PostgreSQL SECURITY INVOKER/security-barrier view over FORCE-RLS protected tenant data. It grants current authority only for active, non-deleted subjects and live tenant/system roles, within half-open assignment windows. An enabled pair-of-role SoD conflict suppresses **both** implicated roles unless an independently approved exception covers the current instant. Expiry takes effect at statement time without a queue timer or cache refresh.

RBAC permission checks/catalogues, ABAC subject/role selectors, authentication read models, directory role filters/dynamic groups, identity administrative safeguards, notification/search/workflow/collaboration role lookups use this view. Raw `user_roles` remains deliberately used for assignment inventory/counts, review snapshots, conflict discovery, provisioning inserts and complete deprovisioning/removal evidence. Expired rows are not silently deleted.

Timeboxed assignments have a separate tenant-bound `window_approved_by`, positive versions, a reason and a maximum 90-day window. Self-timeboxing and noncanonical UUID aliases are rejected. An expiring administrator must leave a different permanent effective administrator. Enabled SoD rules involving administrative permissions require an unaffected permanent administrator. Administrative removal/deprovisioning cannot use temporary or SoD-exception-dependent authority as its last remaining backup. Mutating governance and role administration transactions lock the tenant administration row, then the same tenant advisory key; this shares the directory's last-admin serialization boundary.

## Routes and contracts

All routes require browser authentication and centralized `settings:read` for GET or `settings:configure` for mutations. SCIM and automation/API-key credentials cannot use this surface. There is no additional entitlement gate: core access governance is a security control, not a plan-dependent authorization bypass.

Under `/api/v1/access/governance`:

- `GET/POST /campaigns`, `GET /campaigns/{id}`, `GET /campaigns/{id}/items`.
- `POST /campaigns/{id}/items/{itemID}/decision` accepts `{decision:"retain"|"revoke",reason,request_id,snapshot_sha256}`. Same exact request replay returns the same immutable decision; a changed reuse conflicts. Snapshot/grant/role version drift, access expiry and removed subjects fail closed. Revoke removes a role assignment or revokes an existing approved object grant atomically with decision/history/outbox.
- `POST /campaigns/{id}/complete|cancel` accepts `{expected_version,reason}`. Completion requires every immutable item to have a decision; closed campaigns cannot reopen.
- `GET/POST /sod-rules`, `POST /sod-rules/{id}/disable` with version/reason.
- `POST /sod-rules/{id}/exceptions` accepts `{subject_id,valid_from,expires_at,reason}`; `GET /sod-exceptions`; `POST /sod-exceptions/{id}/approve|reject|revoke` with version/reason. Approval excludes subject and requester. Expired windows remain recorded; explicit reasoned revoke is needed before replacement.
- `GET /sod-violations` lists immutable conflicts detected at rule creation, not a silently mutable current-status projection. `GET /events` lists immutable reasoned governance history.

`PUT /api/v1/access/roles/{id}/assignments/{userID}/window` accepts `{assignment_id,valid_from,expires_at,expected_version,reason}`. Obtain `assignment_id` and `version` from the existing assignment-list route; incarnation matching prevents stale requests from changing a removed-and-regranted assignment. Existing assignment POST also accepts optional paired `valid_from`/`expires_at`. Versioned window updates cannot extend one's own authority.

Campaign creation accepts `{name,reviewer_id,subject_ids,due_at,reason}` with 1–200 distinct active subject IDs, an independent reviewer, due date within90 days and at most1000 role/object-grant items. Empty/oversized campaigns roll back. Object-grant sponsors and original approvers cannot recertify their own grants. Existing immutable policy-version certification and object-grant approval routes remain available; campaign decisions do not replace or widen their RBAC/ABAC constraints.

List routes use `page` and `page_size` (capped100), returning `{data,pagination}`. Single-resource/mutation responses are direct typed objects. JSON rejects unknown/trailing fields and is size bounded. Errors are400 invalid,403 separation,404 tenant-scoped missing,409 stale/version/replay/admin safeguard; internal SQL errors are not returned.

## Deployment and rollback

Schema58 is required by this build. New tables are ENABLE+FORCE RLS, composite tenant-user/campaign/rule FKs, PUBLIC-revoked and tenant-policy scoped. API mutable campaign/rule/exception projections need SELECT/INSERT/UPDATE; items/decisions/violations/events need SELECT/INSERT only. API and scheduler need SELECT on the effective view and underlying SoD rule/exception tables, plus their existing user/role/permission access. The migration conditionally grants pre-existing fixed groups; late group provisioning must apply the reviewed58 matrix. Schema57 runtime-manifest evidence remains a historical snapshot until the platform's58 posture/grants are separately proved.

Empty and legacy-populated upgrades are reversible without changing legacy assignments. Down refuses any governance campaigns/rules/events or timed assignments because removing them would destroy immutable evidence or make restricted authority perpetual. This security-sensitive guard is checked owner-visibly inside the migration transaction; failure rolls back temporary NO-FORCE changes.

## Deferred

Review UI; recurring campaign scheduling/reminders/escalation/delegation; manager-selected reviewers; permission-level/transactional-business SoD; JIT access-request broker/step-up binding; current-violation status/reconciliation and notifications when a previously approved exception expires; per-user exception revocation approvals; migration58 runtime posture repin/late-provisioning matrix proof. Existing policy certifications and sponsored object-grant decisions retain their original administrative approval semantics; campaigns add stronger reviewer independence without rewriting historical evidence.
