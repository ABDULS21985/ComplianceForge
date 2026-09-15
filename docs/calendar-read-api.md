# Bounded compliance calendar read API

This is a schema-aligned read projection of migration035 `calendar_events`, not
a new scheduling, task-management or SLA engine. It is compatible with schema58
and requires no new migration or runtime grants.

## Composition and routes

Compose `repository.NewCalendarReadRepository(pool)`,
`service.NewComplianceCalendarService(repository, compositeAuthorizer)`, then
`handler.NewCalendarReadHandler(service)`. The handler exposes `Ready()`.
The service requires the persisted RBAC/ABAC composite authorizer; an RBAC-only
replacement is not equivalent. The legacy `CalendarService` and `CalendarHandler`
must not be used for these routes.

| GET route | Handler | Response |
| --- | --- | --- |
| `/api/v1/calendar/events` | `ListEvents` | `CalendarEventPage` |
| `/api/v1/calendar/upcoming` | `Upcoming` | `CalendarEventPage` |
| `/api/v1/calendar/overdue` | `Overdue` | `CalendarEventPage` |
| `/api/v1/calendar/summary` | `Summary` | `CalendarSummaryResponse` |

Keep the existing trusted composite `audits:read` collection authorization gate
without an event-ID-to-audit-ID resolver. Sponsored external auditors are excluded
from this collection until a separate grant-filtered collection entry point is
implemented; do not circumvent the policy decision point to admit them.

No calendar mutation, detail, public iCal, sync, completion, reschedule,
subscription or deadline route is part of this verified read contract. Legacy
optional mounts for those methods must be removed from production composition.

## Requests and responses

All DTOs live in `internal/models/calendar_read.go`. Service methods take
`(ctx, organizationID, userID, CalendarEventQuery, CalendarAccessContext)`.
Organization, subject, role, IP address, MFA assurance and request ID come only
from verified request middleware, never query parameters or tenant headers.

Common query parameters are `start_date`, `end_date`, `event_type`, `category`,
`priority`, `status`, `assigned_to`, `search`, `sort_by` and `sort_order`.
`from_date`/`to_date` are compatibility aliases for the paired date bounds;
conflicting alias values are rejected. Dates are canonical `YYYY-MM-DD`, years
1900–2100, inclusive bounds, with at most 366 dates. Missing bounds default to
the current UTC month. Supplying only one bound is invalid.

Lists accept `page` (default 1, maximum 100000) and `page_size` (default 50,
maximum 100). Upcoming alone accepts `days_ahead` (1–365, default 30), instead
of explicit dates. Its default window is today through today+30 days inclusive;
explicit dates cannot start before today. Overdue defaults to the preceding 90
days through yesterday; its explicit end must remain before today. Summary
does not accept pagination or `days_ahead`.

Sorting is allowlisted to `start_date`, `title`, `priority` or `updated_at`;
order is `asc` or `desc` with a stable UUID tie-break. Search is at most 200
Unicode characters, trimmed, without control characters; SQL `%`, `_` and
backslash have literal semantics. UUID filters require canonical lowercase UUIDs
and reject nil, uppercase, URN, compact and brace aliases. Unknown parameters,
empty/repeated values, malformed query encoding and oversized query strings
are rejected, not silently ignored.

Event types use exactly migration035's 28 values. Priorities are `critical`,
`high`, `medium`, `low`; states are `upcoming`, `in_progress`, `completed`,
`overdue`, `cancelled`, `snoozed`. Supported category filters are `policy`,
`risk`, `audit`, `control`, `evidence`, `vendor`, `incident`, `asset`, `custom`.

List envelopes contain `data`, `pagination` and `meta`; there is no `items`,
`due_date`, `all_day` or invented `pending` state. Events expose `start_date`,
optional `end_date`/times, `is_all_day`, the stored timezone, bounded title and
description, source reference, safe assignee ID and canonical state. Open past
events are derived as `overdue`, and stale future `overdue` rows as `upcoming`,
without a GET mutation. `is_due_today` is a boolean, not a stored status.
Upcoming includes open events due today; summary `upcoming_events` counts only
open future events, while `due_today_events` counts open events today.

Summary has permission-filtered totals, counts by event type/category/priority/
status, and a zero-filled date/count array for every requested date. Metadata
records the UTC `as_of`, date bounds, `date_timezone="UTC"`,
`source_table="calendar_events"`, `sync_verified=false` and candidate limit 2000.
Stored timezone is provenance only: dates are compared as UTC date-only values,
not converted into per-user timezone instants.

## Security and boundedness

The repository requires `database.QuerierFromContext(ctx,nil)` and never falls
back to the unscoped pool. It verifies the current tenant and an active,
non-deleted organization and subject. Both calendar rows and live sources are
read under request-scoped FORCE RLS.

Supported source mappings are singular `risk`, `policy`, `audit`, `vendor`,
`incident`, `asset`, `control`/`control_implementation` and `evidence`.
Control sources must identify tenant `control_implementations`, not global
framework controls. Evidence sources require live, current, active evidence and
a live control implementation; their permission anchor is that implementation.
Deleted, cross-tenant, missing or unsupported source types are excluded before
any result. Deleted, suspended or cross-tenant assignees are projected as absent,
with `assignment_available=false`; filtering for them yields no rows. No user
profile fields, raw metadata, watchers or completion notes are returned.

Every distinct source resource/object is passed to the composite authorizer for
`read`. Denied sources are excluded before all totals, sorting and pagination.
Any source field marked masked or hidden excludes that source's entire calendar
projection, because its denormalized title/description/reference cannot safely
be attributed to individual source fields. Malformed or unsupported obligations
also exclude the source. Unknown collection obligations fail closed with 503;
valid collection field obligations use the common classified-JSON writer.
Policy allows still require immutable durable authorization decision evidence.

The SQL loads at most 2001 live candidates. A 2001st candidate produces 503 and
no partial page or fabricated truncated total. Authorization is memoized only
within one request and by source object. Context cancellation is propagated.
Errors are generic 400/401/403/503, without raw SQL or provider error details.

## Proof and remaining scope

Focused unit tests cover canonical bounds, aliases, strict query decoding,
safe error mapping, unsupported obligations, field-mask exclusion, source
identity propagation, counts-before-pagination, derived dates and cancellation.
`TestCalendarReadNonBypass` is opt-in using `TEST_DATABASE_URL` against clean
schema>=58. It creates an actual LOGIN NOSUPERUSER/NOBYPASSRLS role with no owner
membership, narrow SELECT access and decision INSERT, then exercises two tenants
and real persisted RBAC/ABAC source controls. Fixture cleanup is transactional,
exact-tenant and reports errors. Run only against a disposable fixture database:
the setup connection needs CREATEROLE and privileged trigger-disabled cleanup;
it is deliberately separate from the restricted runtime login under test.

Executed W42 gates: focused units and focused `-race` across handler/service/
repository passed; scoped `go vet -p 1` passed; the expanded actual-LOGIN live
test passed under `-race` on independent PostgreSQL 16 schema58. It includes
ten concurrent scoped reads across two tenants and an actual 2001-row SQL
candidate-limit rejection. Post-run catalog checks found zero fixture
organizations, runtime roles or capacity-source risks. Shared production router
composition and generated OpenAPI are owned by the root integration workstream,
not these isolated gates.

Roadmap83 remains partial: these four bounded reads are not a unified task center,
source reconciliation or assignment workflow. Roadmap89 remains unimplemented:
there is no SLA clock, working-day/holiday computation or breach forecast.
Existing calendar producers, recurrence expansion, reminders/escalations,
completed/snoozed workflow, durable delivery, subscriptions and iCal are not
claimed verified. In particular the incompatible legacy sync service must not
be composed merely to make unsupported mutation routes appear available.
The existing calendar reminder/escalation worker still reads denormalized titles
without live-source/object-permission checks and inserts notifications directly;
the GET projection's field-mask exclusion does not secure that separate path.
Frontend calendar adapters must align with the new typed envelopes before the
existing UI can be treated as an end-to-end verified feature.
