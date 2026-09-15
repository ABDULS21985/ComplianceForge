# Tenant organisation profile

Reviewed 2026-09-15. This is the bounded organisation-profile slice for roadmap
item 181, not complete enterprise tenant administration.

## Contract and authority

`GET /api/v1/settings/organization` requires the trusted composite
`settings:read` decision. `PUT` at the same URL requires `settings:configure`;
an authenticated tenant and active, nondeleted actor are required for both.
The repository also checks an active/trial, nondeleted organisation against the
request connection's current tenant. There is no unscoped database-pool fallback.

The response is `{data, meta}`. Metadata has `schema_version: 1`,
`scope: "tenant_organization_profile"`, and `editable`. This last flag proves
that the returned projection was not hidden/masked; it does **not** confer a
configure permission. A masked read is supported, but has `editable: false`.
Unsupported/malformed obligations fail closed, and a mutation with any
hidden/masked field is rejected before reading or writing profile data.

Editable fields are name, legal name, industry, two-uppercase-letter country
code, IANA timezone, default/supported languages (`en`, `de`, `fr`), employee
count range, fiscal-year start month/day, and informational contacts. Read-only
fields include organisation ID, slug, lifecycle status, subscription tier,
version and updated timestamp. Raw settings, branding, metadata, credentials,
parent-tenant references, registration numbers and tax identifiers are never
part of this projection or mutation contract.

Country-code validation is a syntax boundary, not an ISO registry lookup.
Language choices do not assert that every product screen is translated. The
fiscal date is a recurring non-leap date; February 29 is deliberately rejected.
Contacts allow at most one plain ASCII email for each reviewed purpose:
security, privacy, billing and support. They do not configure delivery, verify
domains, change billing authority, or grant administrator privileges.

## Safe editing and audit

PUT has presence-aware fields: omitted values are preserved; explicit null,
unknown fields, read-only fields, duplicate object keys (including nested
contacts), multiple JSON values and bodies over 8 KiB are rejected. A positive
`expected_version`, at least one editable field, and a bounded, nonempty
`reason` are mandatory. The maximum version is `9007199254740991`, the largest
exact JavaScript integer.

The service validates canonical nonnil UUIDs, text lengths/control characters,
timezone, language consistency, fiscal date and contact shape. The repository
locks the tenant organisation, rechecks its current version, updates only the
reviewed columns, and inserts `ORGANIZATION_PROFILE_UPDATED` into `audit_logs`
in the **same transaction**. Audit metadata contains schema/previous/current
versions, the submitted change reason and changed-field labels; it does not
duplicate contact values, before/after business values, or raw settings.
Operators should not put credentials or unnecessary personal data in reasons.

Migration 059 gives every organisation UPDATE a database-managed version
increment, including older non-profile writers. Organisation identity and
explicit version resets are rejected. This makes older writes invalidate stale
profile drafts; it does not retrofit API audit onto arbitrary direct SQL.
Version exhaustion fails closed rather than wrapping or losing precision.

A stale draft returns 409 and requires explicit reload/review. Do not blindly
retry writes after a conflict, authentication refresh, timeout or ambiguous
network completion. A successful response follows transaction commit, but a
lost commit acknowledgement cannot prove that the write did not occur; reload
the profile and its audit history before deciding on another edit. This
endpoint does not promise an idempotency-key protocol.

## Migration and rollout boundary

Migration 059 is additive and historical migrations remain unchanged. Its
contact-check helper and version trigger are SECURITY INVOKER with fixed
`pg_catalog, public, pg_temp` search paths. The reviewed runtime manifest grants
the scalar CHECK helper; it does not grant a new table or increase CRUD verbs.
Deploy against a clean supported schema 59 and reapply/review the exact runtime
manifest. Previous actual-LOGIN schema-58 acceptance is historical evidence,
not acceptance of this new schema or API.

The downgrade refuses to discard configured contacts/fiscal settings **or any
profile version history**. Its ordinary table-owner inspection temporarily
removes FORCE RLS inside an ACCESS EXCLUSIVE transaction so it cannot mistake
an empty RLS projection for an empty database; a failure rolls back the change,
and a successful downgrade restores FORCE before commit. Do not force
migration bookkeeping, reset profile versions, or delete tenant data to make a
rollback pass. A populated rollback needs a reviewed export/restore plan.

## Verification and remaining work

Root independently applied frozen migrations 0 to 59 on a fresh disposable
PostgreSQL 16 database: `59:false`. Focused service/handler/repository tests
passed. `TestOrganizationProfileActualLogin` passed normally and under `-race`
using a genuinely authenticated NOSUPERUSER/NOBYPASSRLS non-owner LOGIN with no
role memberships, against FORCE-RLS organisations/users/audit logs. It covers
tenant/actor denial, presence-aware preservation of raw settings, atomic audit
failure rollback, concurrent CAS edits, stale writes after a legacy update,
contact/fiscal/identity/version database guards and version exhaustion. Exact
fixture cleanup left zero organisations/fixture roles and an enabled trigger.
The first live run had an incorrect test-only changed-field count; that
assertion was corrected before the passing normal and race runs.

Shared composition, generated OpenAPI, current-schema runtime acceptance,
full combined regression gates and profile UI/browser gates must be recorded
after their final runs. Populated downgrade refusal and an empty downgrade/
re-up cycle still require independent live verification; the design above is
not a claim that these tests have already passed.

Verified domains, residency selection, delegated tenant administration,
hierarchical organisation management, domain/SSO ownership checks, transport
configuration, billing workflows, invitations and current-session step-up are
separate capabilities. This slice does not silently claim or stub them.
