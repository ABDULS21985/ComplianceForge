# Calendar reminder security plan — W45

Status: reviewed architecture only; no W45 worker, notification, grant, posture,
router or migration implementation is included. W42's bounded GET projection is
separate. The runtime matrix must be explicitly reviewed before rollout.

## Current concrete gaps

1. `internal/worker/calendar_worker.go:38` and `:138` select calendar text without
   source type/ID, live-source or lifecycle checks. Deleted, unknown, cross-tenant
   and masked source records are not excluded.
2. `:94` and `:180` check only a non-nil assignee. They do not require a same-tenant,
   active, non-deleted recipient. Schema035's assignee FK references only `users.id`.
3. Neither path evaluates the recipient's persisted RBAC/ABAC object read or field
   obligations. Scheduler database privileges are not user authorization.
4. `:36` and `:136` permit an unscoped pool fallback. Active tenant enumeration
   protects normal entry points, but direct callbacks do not enforce that boundary.
5. Notification insertion (`:238`) and calendar flags (`:112`, `:191`) are separate
   statements. Stale JSON overwrites and crashes/concurrent polls can lose flags.
   Escalation identity incorporates changing `days_overdue`; its delivery key is
   not stable across later polls if flag persistence fails.
6. The insert fills `body_text` but leaves `subject` and legacy `body` NULL.
   Delivery does not repair them, while `notification_handler.go:177` scans those
   fields into strings. Delivered calendar rows can make the inbox fail to scan.
7. Notifications' recipient FK is also ID-only. FORCE RLS checks notification
   organization, not recipient tenant. A cross-tenant recipient reference can be
   persisted. The inbox itself remains organization-and-recipient scoped: this is
   a demonstrated schema/worker integrity gap, not a demonstrated cross-tenant
   notification read bypass.
8. `notification_engine.go:993` dispatches in-app notifications without recipient,
   source or authorization rechecks. `notification_delivery.go:527` then marks
   them delivered. Creation-time checks alone cannot secure pending delivery.
9. Existing runtime fixtures use `source_entity_type='custom'` with the calendar
   event itself as source and expect notification creation. Such unknown-source
   fixtures must instead prove fail-closed suppression or use live supported sources.

## Bounded implementation contract

Keep current reminder offsets and assignee-only escalation; do not introduce
recurrence, SLA/business hours, alternate escalation targets, iCal or calendar
mutations. Use an injected UTC clock and bounded keyset pages of due event IDs.
Malformed source/state/offset/payload/flag data must fail closed, not be guessed or
silently truncated. Bound pages, JSON, reminder arrays and per-cycle error retention.

Retain `NewCalendarWorker(pool)` compatibility by internally constructing the real
persisted RBAC authorizer, policy repository and composite policy authorizer.
Offer a narrow injectable authorizer/clock constructor or options for deterministic
tests. Reject nil and typed-nil dependencies and expose readiness; production
composition/posture must fail before accepting an incomplete worker. Missing PDP
must never become an allow, RBAC-only shortcut or empty successful security check.

### Identity and source checks

Every callback must require `database.QuerierFromContext(ctx,nil)` and verify
`organization_id=get_current_tenant()` and an active, non-deleted organization.
The subject of authorization is the same-tenant active/non-deleted recipient—not
the scheduler, a synthetic service user, JWT claims or a platform-superadmin flag.

Use the existing composite PDP without recreating policy semantics in SQL.
The request has source resource/object ID, action `read`, recipient subject and
organization, a bounded correlated job/request ID, **MFA=false and no trusted IP**.
Do not claim a session, passkey, step-up grant or interactive authentication for a
timer job. Policies requiring that assurance must deny the notification.

Reuse W42's supported live-source semantics: `risk`, `policy`, `audit`, `vendor`,
`incident`, `asset`, tenant `control`/`control_implementation`, and current active
`evidence` with a live parent implementation. Evidence authorization anchors to
that implementation; other sources anchor to their actual source object. Never
resolve calendar event IDs as audit IDs or treat global framework controls as
tenant implementations. Unsupported/deleted/cross-tenant sources are denied.

Any source field hidden or masked excludes the entire notification. Unknown,
non-field, malformed or wrong-resource obligations are denied. An allow still
requires the PDP's immutable durable decision insertion; missing audit privileges
must fail closed. Report only bounded error classifications and correlation IDs,
never source text, recipient profiles, raw errors, tokens or configuration values.

### Enqueue, delivery and flags

Use per-event transactions and `calendar_events FOR UPDATE SKIP LOCKED`, with
current source/recipient/state revalidation. The scheduler already has calendar
UPDATE; do not broaden source/users UPDATE merely to acquire row locks. Recheck
live source and active recipient in the notification insertion statement's
snapshot. This is snapshot validation, not a claim that every concurrent policy
mutation is fenced until commit.

Derive one stable identity from organization, calendar event ID, source deadline,
reminder offset or fixed escalation threshold, and recipient. Exclude mutable
source title, processing time and changing days-overdue from the delivery key.
Use the existing unique notification key and lease fencing; repeated/concurrent
polls must not create duplicate rows or overwrite another reminder key.

Persist static, non-null subject, legacy body and text body. Payload schema must
be a small allowlist of calendar ID, deadline and threshold—not denormalized
source titles/descriptions, names, metadata or references. The PDP/field gate is
still required: even existence or timing can be classified.

Denials, inactive recipients, unknown sources, missing PDP and failed decision
evidence create no notification and consume no reminder/escalation sent flag.
Pending/quiet-hours delivery is not a sent event. Preferred flag semantics are:
enqueue idempotently without sent flags; after successful delivery-time validation,
atomically commit the lease-fenced in-app `delivered` state and calendar reminder
JSON merge/escalation flag in one transaction. Never mark sent/delivered or invoke
external callbacks on a pending, denied or unverifiable row.

Final W45 **must** add a calendar-specific delivery-time guard before Dispatch
and completion, not merely safe enqueue. It must derive current source/assignee
from the current tenant calendar row, revalidate active recipient and source, and
run the same composite object/field checks. Do not trust payload-provided resource
anchors. Only the existing in-app calendar delivery is in scope; reject calendar
rows requesting unsupported channels rather than expanding transport behavior.
PDP-unavailable errors use safe bounded retry; durable denials/invalid payloads are
suppressed with a reasoned failure state, never a success. Preserve the existing
notification retry/lease/backoff framework rather than implementing a second one.

Unrecognized historical calendar payload schemes must fail closed with an explicit
remediation classification. Do not silently infer anchors, rewrite historical
payloads or auto-enable old unknown-source rows. Any migration/remediation requires
a separate approved target list and evidence-preserving strategy.

## Exact scheduler grant delta

| Relation | Additional verb | Actual consumer |
| --- | --- | --- |
| `permissions` | SELECT | Persisted RBAC catalogue |
| `role_permissions` | SELECT | Effective role permissions |
| `access_policies` | SELECT | Composite constraints |
| `access_policy_assignments` | SELECT | Subject applicability |
| `directory_groups` | SELECT | Active group applicability |
| `directory_group_memberships` | SELECT | Group assignments |
| `field_level_permissions` | SELECT | Classified source projection |
| `user_entity_permissions` | SELECT | Approved scoped object grants |
| `access_audit_log` | INSERT | Mandatory immutable PDP decision evidence |

Existing scheduler source/users/organizations/roles/effective-user-roles/SoD
SELECT, calendar SELECT+UPDATE, notifications SELECT+INSERT+UPDATE and durable
queue verbs suffice. Existing `get_current_tenant()` and reviewed invoker scalar
helper execution suffice. No new SECURITY DEFINER, owner membership, BYPASSRLS,
source/user UPDATE, sequence privilege, table or migration is needed for this plan.
Grant manifest and startup posture must both assert this exact delta, including
late provisioning and actual LOGIN tests. Current frozen posture does **not**
claim these additional capabilities are already granted or verified.

## Acceptance and explicit remaining boundary

Prove actual LOGIN NOSUPERUSER/NOBYPASSRLS with no runtime ownership or owner-role
membership, FORCE RLS and current request-scoped tenant checks. Include two-tenant
live sources, deleted sources, inactive/cross-tenant recipients, unsupported sources,
expired roles/SoD denial, real policy denies, source masking, required-MFA/no-IP
constraints, unknown obligations, missing PDP/audit privilege, canonical UUIDs,
static bounded payloads and the existing inbox's non-null response contract.

Test concurrent/crash replay, SKIP LOCKED, JSON key preservation, stable reminder
and escalation keys, cancellation and atomic rollback. Change permissions/source/
recipient between enqueue and delivery and prove suppression without delivery,
sent flags or external callbacks. Unknown legacy payloads must remain suppressed
until explicit remediation. All cleanup must be exact-tenant and error-checked.

Historical delivered notification visibility is a separate mandatory review
boundary: an enqueue/delivery guard cannot revoke text already returned or stored
in an authorized client's memory. Authorization-aware calendar inbox rendering or
fail-closed calendar source projection in notification GET must be implemented
and independently proved before any all-lifecycle calendar-security claim. Until
then W45 may claim only reviewed secure enqueue/delivery subcapabilities, not
historical revocation, unified task workflows, recurrence or SLA maturity.
