# OpenAPI contract lifecycle

ComplianceForge's required production HTTP surface is published as an OpenAPI 3.1 contract at `api/openapi/openapi.json`. The generated artifact is suitable for documentation, client generation, security review, and compatibility testing. It must not be edited by hand.

## Sources of truth

- `api/openapi/base.json` owns API metadata, tenant and credential semantics, reusable parameters, error responses, and component schemas.
- `api/openapi/routes.csv` is the human-reviewable inventory of required production operations. Each row declares the method, path, unique lower-camel operation ID, domain tag, security scheme, request and response schemas, success status, behavioral flags, and operation-specific query parameters.
- `api/openapi/openapi.json` is the deterministic generated artifact. Run `make openapi-generate` after changing either source.
- `internal/router/openapi_contract_test.go` walks the real Chi router assembled with every required dependency and all optional legacy domain handlers unset. It fails for an undocumented mounted route or a documented route that is not part of the required production composition.

Nil-mounted legacy modules are intentionally absent. A module becomes a production contract only after its dependency is required, its routes are unconditionally mounted, and its routes and schemas enter the catalog in the same change.

## Security and tenant boundary

Browser and first-party operations declare `bearerAuth`. The organization boundary comes exclusively from the validated `organization_id` access-token claim. Read-only automation operations declare `apiKeyAuth`; their organization and exact read scope come from the API-key principal. Neither surface accepts a caller-selected tenant header or tenant body field as authority.

Every protected operation documents 401, 403, 429, and 500 responses. Feature-gated operations also document the fail-closed 402 and 503 responses:

- `ENTITLEMENT_REQUIRED` and `ENTITLEMENT_LIMIT_EXCEEDED` use HTTP 402.
- `FEATURE_DISABLED` uses HTTP 403.
- `FEATURE_EVALUATION_UNAVAILABLE` and `ENTITLEMENT_EVALUATION_UNAVAILABLE` use HTTP 503.

Resource creation quotas are preflighted for user experience and enforced atomically by the repository. The OpenAPI response is the client contract; it does not replace repository enforcement.

All application errors use `application/json` and the canonical `ErrorResponse` fields: numeric `code`, stable machine-readable `error_code`, safe `message`, optional actionable `details`, and a required `request_id` that matches the `X-Request-ID` response header. Server-side dependency and persistence details are never serialized. Clients must branch on `error_code`, not English text. Authentication, API-key, authorization, tenant, rate-limit, plan-limit, feature-gate, and internal metrics-auth failures all use this envelope.

This replaces the pre-contract middleware-only `application/problem+json` bodies and legacy `{ "error": "..." }` bodies. A client that parsed `type`, `title`, `status`, `detail`, or the one-field `error` body must migrate to `code`, `error_code`, `message`, `details`, and `request_id`. HTTP statuses, `Retry-After`, `WWW-Authenticate`, and authorization semantics are unchanged.

The browser-facing BFF session/CSRF endpoints are a separate same-origin compatibility boundary. They intentionally retain their compact `{ "code": "...", "message": "..." }` response because they are consumed before the application API client is established; they are not `/api/v1` operations and are not a defect in this contract. Changing that envelope requires a coordinated BFF/client migration and its own contract versioning decision.

Directory imports use `text/csv`, are bounded to 2 MiB, and require `Idempotency-Key` and `X-Change-Reason` on apply. Pagination uses one-based `page` and a bounded `page_size`; paginated responses expose `data` and `pagination` with `page`, `page_size`, `total_items`, and `total_pages`.

## Change workflow

1. Change the handler DTO or route and its authorization/tenant tests.
2. Update the reusable schema in `base.json` and the operation row in `routes.csv`.
3. Run `make openapi-generate` and commit all three OpenAPI files.
4. Run `make openapi-validate`. This checks deterministic generation, OpenAPI 3.1 validity, references, unique and present operation IDs, explicit security, required responses, exact mounted-route coverage, representative live handler JSON, and the material frontend contracts.
5. Include compatibility impact and client migration notes in the pull request.

CI runs the same validation as a dedicated blocking job. Backend and frontend build jobs depend on it. `Incident`, `Asset`, and `ManagedRole` currently have exact Go/OpenAPI/frontend property-key checks; expand that set whenever another domain gains a separately maintained frontend DTO. Schemas still named `GenericResponse`, `GenericMutation`, or `PaginatedGeneric` are inventoried and secured but are not yet eligible for generated typed clients.

## Versioning and compatibility

The URL major (`/api/v1`) is the compatibility boundary. `info.version` follows semantic versioning for the contract artifact:

- Patch: descriptions, examples, and constraint corrections that do not reject a previously valid request or response.
- Minor: additive paths, optional request properties, optional response properties, or new enum values when consumers are required to tolerate unknown values.
- Major: removing or renaming an operation or property, changing a method/status/type, narrowing an accepted value, adding a required request value, or changing authorization or tenant semantics.

Breaking changes require a new URL major unless they close an actively exploited security defect. Emergency security changes require an incident record, named API owner, affected-consumer inventory, and explicit migration communication.

Compatibility aliases are marked `deprecated: true` in the contract. Deprecation alone does not authorize removal. The runtime emits the [RFC 9745](https://www.rfc-editor.org/rfc/rfc9745.html) structured-date `Deprecation` header, the [RFC 8594](https://www.rfc-editor.org/rfc/rfc8594.html) HTTP-date `Sunset` header, and an [RFC 8288](https://www.rfc-editor.org/rfc/rfc8288.html) `Link` with `rel="successor-version"`. For the current alias cohort these are:

```http
Deprecation: @1789344000
Sunset: Thu, 01 Apr 2027 00:00:00 GMT
Link: </api/v1/canonical-resource>; rel="successor-version"
```

| Deprecated operation | Successor |
| --- | --- |
| `GET /health` | `GET /health/live` |
| `GET /api/v1/incidents/breach-notifiable` | `GET /api/v1/incidents/breaches/upcoming` |
| `GET /api/v1/vendors/stats` | `GET /api/v1/vendors/statistics` |
| `POST /api/v1/vendors/{id}/assess` | `POST /api/v1/vendors/{id}/assessments` |
| `PUT /api/v1/policies/{id}/approve` | `POST /api/v1/policies/{id}/approval/decision` |
| `PUT /api/v1/policies/{id}/submit-review` | `POST /api/v1/policies/{id}/submit` |

The cohort became deprecated on 14 September 2026 and has a declared sunset of 1 April 2027, a window longer than 180 days. The sunset is the earliest eligible removal date, not an automatic removal authorization: the aliases must also remain for at least two production releases, API owners must inventory and notify known consumers, and release approval must confirm migration before removal.

The only required production `202 Accepted` operation currently is integration sync. Its response keeps the existing domain record under `data` and adds a stable `job` descriptor (`id`, `type`, `status`, `submitted_at`, and optional `status_url`). This is additive for tolerant JSON clients; strict clients that reject unknown top-level properties must be updated before deployment. Optional nil-mounted legacy domains still have heterogeneous accepted-response payloads and are not production contracts; they must adopt the shared envelope before promotion.

Generated clients must pin an exact committed contract version. Consumers should ignore unknown response properties and unknown enum values where their language permits it, retain the `X-Request-ID` value in support telemetry, and treat undocumented status codes as failures rather than successful fallbacks.
