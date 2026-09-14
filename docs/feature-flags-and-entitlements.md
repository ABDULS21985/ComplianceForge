# Feature flags and subscription entitlements

ComplianceForge evaluates product capabilities on the server. Navigation may
use the same result for presentation, but hiding a link is never the security or
commercial enforcement boundary.

## Evaluation order

Every capability is evaluated for the authenticated tenant in this order:

1. The global kill switch denies the capability.
2. An active or trialing subscription plan must include the required feature.
   Tenants without a normalized subscription fall back to the organization's
   cached tier and the catalogue's minimum tier.
3. A currently active tenant override may disable the capability or replace its
   rollout percentage and JSON variant. Scheduled and expired overrides remain
   visible to administrators but do not affect evaluation.
4. Every prerequisite must itself evaluate as enabled. Missing definitions and
   dependency cycles fail closed.
5. The tenant must fall inside the deterministic rollout bucket. The bucket is
   derived from SHA-256 of the tenant ID and capability key, so it is stable
   across API instances and deploys without storing personal data.

Tenant overrides cannot bypass subscription denial, prerequisites, or a global
kill switch. Mutations use optimistic versions and require a reason. The
override, immutable history event, and notification outbox record commit in one
transaction.

## Protected administration API

All endpoints require the existing `settings:read` or `settings:configure`
permission and a tenant-scoped database connection:

- `GET /api/v1/settings/capabilities`
- `GET /api/v1/settings/capabilities/{key}/evaluation`
- `GET /api/v1/settings/entitlements`
- `GET /api/v1/settings/entitlements/limits/{metric}/check?requested=N`
- `GET /api/v1/settings/feature-flags`
- `PUT /api/v1/settings/feature-flags/{key}`
- `POST /api/v1/settings/feature-flags/{key}/reset`
- `GET /api/v1/settings/feature-flags/{key}/history`

Creating an override omits `expected_version`. Updating or resetting supplies
the current positive version; stale and blind overwrites return HTTP 409.
Reasons contain 3–1000 characters, rollout uses 0–10,000 basis points, variants
are JSON objects capped at 16 KiB, and optional activation windows must be
ordered.

## Runtime enforcement and UX

Required premium routes use the same evaluator after authentication and tenant
setup. A missing evaluator or database failure returns HTTP 503 with
`FEATURE_EVALUATION_UNAVAILABLE`; subscription denial returns HTTP 402 with
`ENTITLEMENT_REQUIRED`; an operational or tenant switch returns HTTP 403 with
`FEATURE_DISABLED`. Responses include the request correlation ID and never
expose database errors.

The frontend should turn `ENTITLEMENT_REQUIRED` into an upgrade explanation
that names the capability and current plan. Disabled and unavailable states are
different: disabled features should explain scheduling or administrator state,
while unavailable evaluation should offer retry and support correlation data.

## Operating global switches

The `product_capabilities` catalogue is deployment-managed and tenant APIs are
read-only. A global kill-switch, rollout, prerequisite, maturity, or tier change
therefore follows the normal reviewed database-change process. Capture the
incident/change ticket, expected duration, rollback condition, affected tenants,
and verification evidence. Tenant administrators manage only their own audited,
time-bounded overrides.

The limit check endpoint currently reports live users, adopted frameworks,
risks, vendors, and evidence storage against the normalized plan. A zero limit
means unlimited. Creation paths must use the server-side entitlement guard; a
preflight check is advisory UX and must never be treated as an atomic quota
reservation.
