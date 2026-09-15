# Tenant data-quality diagnostics

`GET /api/v1/settings/data-quality` is a bodyless, read-only, `settings:read`
diagnostic for the fixed `tenant_structure_and_delivery_v1` ruleset. It accepts
no query parameters, tenant overrides, pagination, record filters, repair, or
reconciliation options. It returns no records, principal IDs, business values,
messages, attachment bytes, or raw metadata.

The request needs a trusted authorization allow decision and a request-bound
tenant connection. PostgreSQL verifies the current tenant, active/nondeleted
organization, and active/nondeleted same-tenant actor before exposing counts.
All tables retain the reviewed API grants and FORCE-RLS boundaries. One SQL
statement produces one MVCC snapshot, full counts, and PostgreSQL
`statement_timestamp()` as the shared UTC `as_of`. Counts must fit the exact
JSON/JavaScript integer range `0..9007199254740991`.

The default and maximum query budget is two seconds; cancellation, missing
grants/connection, dirty/wrong schema, incomplete projection, unknown rules,
invalid counts, or an unverifiable scope yields a safe 503 with no partial or
sampled results. Source and the exact runtime manifest must agree with the
current clean `SupportedSchemaVersion` (currently 59). An inactive organization
or actor, actor/organization mismatch, or wrong current tenant yields safe 403.
Missing authentication yields 401. Unknown query/body scope yields 400.

ABAC obligations are checked *before* querying. Only valid, settings-scoped
field-visibility obligations marked `visible` can be enforced here. A hidden or
masked settings field, unknown field parameter, or any non-field obligation
withholds the entire response with 503. This intentionally conservative boundary
also rejects unrelated hidden settings fields: aggregate count and derived
overall/check statuses must not disclose a value that cannot be exposed.

## Reviewed checks

Positive structural counts are `critical`; positive delivery/deprovisioning
precondition counts are `warning`; zero counts are `healthy`. Overall status is
the worst check status. Do not add counts together: one record may violate more
than one independent invariant. A healthy result means only that these eighteen
checks found no issue at `as_of`, not that every domain has mature data quality.

| # | Key | Reviewed same-tenant invariant / warning precondition |
| --- | --- | --- |
| 1 | `control_adoption_tenant_parent` | Implementation references a retained same-tenant framework adoption. |
| 2 | `control_adoption_framework_alignment` | The referenced framework control belongs to that adoption's framework; a missing/hidden foreign control cannot silently disappear in a join. |
| 3 | `policy_version_tenant_parent` | Policy version references a retained same-tenant policy. |
| 4 | `policy_current_version_alignment` | Non-NULL current version ID matches the policy, tenant, and current version number. A NULL draft/SET-NULL pointer is not automatically corruption. |
| 5 | `policy_workflow_parent_alignment` | Workflow references its same-tenant policy; optional version references that same policy and tenant. |
| 6 | `policy_step_tenant_workflow` | Approval step references a retained same-tenant workflow. |
| 7 | `risk_assessment_tenant_parent` | Assessment references a retained same-tenant risk. |
| 8 | `risk_treatment_tenant_parent` | Treatment references a retained same-tenant risk. |
| 9 | `risk_indicator_tenant_parent` | Non-NULL indicator risk reference is same-tenant. Legitimate ON DELETE SET NULL is accepted. |
| 10 | `risk_value_tenant_indicator` | Indicator measurement references a retained same-tenant indicator. |
| 11 | `finding_tenant_audit` | Finding references a retained same-tenant audit. |
| 12 | `asset_tenant_vendor` | Non-NULL linked vendor references a retained same-tenant vendor, including legacy rows predating FK validation. |
| 13 | `notification_tenant_recipient` | Notification references a retained same-tenant recipient. |
| 14 | `notification_channel_alignment` | Non-NULL channel ID matches tenant and channel type. |
| 15 | `notification_tenant_parent` | Non-NULL escalation parent references a retained same-tenant notification. |
| 16 | `user_role_scope_alignment` | Assignment references a retained same-tenant custom role, or a NULL-organization role genuinely marked `is_system_role=true`. |
| 17 | `due_email_recipient_unavailable` | Pending/failed email, not dead, retry count below maximum, effective retry/schedule at or before `as_of`, and no live lease, has an inactive/deleted retained same-tenant recipient. Future, sent, exhausted, dead, and currently leased deliveries are excluded. |
| 18 | `unremoved_tombstoned_group_membership` | A persisted membership remains open after its same-tenant group was deleted or its same-tenant user was deprovisioned. Merely inactive/suspended users and correctly closed historical memberships are accepted. |

Structural checks intentionally do not remove retained parents or children based
on soft deletion, legal hold, inactivity, resolved status, or completed lifecycle
state. A retained historical parent is valid. With RLS, an unresolved reference
means “missing or not visible in this tenant,” not proof that a record is absent
globally; never reveal or query another tenant's identifiers to diagnose it.

### Retention visibility boundary

Current framework-control SELECT RLS hides controls whose parent framework is
soft-deleted even though the framework/adoption/implementation may be retained
valid history. If any scoped implementation's retained adoption references a
same-tenant or genuine global-system soft-deleted framework, the same-MVCC
completeness gate withholds **all eighteen checks** with generic 503. It does not
mislabel retention as critical, silently omit the alignment check, return a
partial status, or bypass RLS. A reviewed retention-aware control projection is
future work. Operators should investigate visibility/retention scope through an
authorized workflow, not restore or delete business records to make this
endpoint green.

## Triage and operating limits

Record the request ID and `as_of` of a successful snapshot. A 503 exposes no
counts/status; check schema readiness, runtime grant convergence, cancellation,
query latency, and the documented retention visibility boundary using operator
access. Do not infer that an unavailable tenant is healthy or corrupt.

Assign structural issues to the relevant domain owner and Security if tenant
boundaries appear inconsistent. Delivery precondition warnings go to the
notification or identity owner; inspect authorized source workflows rather than
using diagnostic output as a deletion list. Legal holds, retention, immutable
event history, and business lineage must be reviewed before a separately
approved remediation. This feature performs **no writes or repairs** and does
not implement comprehensive ownership/lineage or controlled reconciliation.
Roadmap items 119 and 120 remain Partial.

## Validation

Focused tests:

```sh
GOTOOLCHAIN=go1.26.8 go test -race -p 2 \
  ./internal/models ./internal/repository ./internal/service ./internal/handler \
  -run DataQuality -count=1
```

The opt-in PostgreSQL integration test needs a *disposable*, fully migrated and
runtime-manifest-provisioned PostgreSQL database with an administrator test URL:

```sh
DATA_QUALITY_TEST_CONFIRM_DISPOSABLE=yes \
TEST_DATABASE_URL='<disposable PostgreSQL administrator URL>' \
GOTOOLCHAIN=go1.26.8 go test -tags=integration -race \
  ./internal/repository -run TestDataQualitySnapshotLive -count=1 -v
```

The runtime reader is a new actual LOGIN, NOSUPERUSER/NOBYPASSRLS API-group
member—not SET ROLE over administrator credentials. Structural positive fixtures
that modern FK/assignment guards prevent are injected solely in an isolated
administrator transaction with `SET LOCAL session_replication_role=replica`,
representing legacy corruption. The test restores the setting before runtime
reads, never disables FORCE RLS, and removes only its own fixtures/login during
cleanup. The production repository executes one SELECT and has no repair path.
