# Consented support bundles

The authenticated `POST /api/v1/settings/diagnostics/support-bundle` endpoint generates a local ZIP for an administrator with `settings:configure` permission. ABAC must allow the request with no hidden/masked fields or unsupported attachment obligations. There is no public route, automatic provider transfer, stored ZIP, reusable sharing token, or implicit consent from viewing diagnostics.

Request content type is `application/json`; the maximum request size is 1,024 bytes. Exactly one JSON object is accepted and unknown fields are rejected:

```json
{"consent": true, "scope": "health_and_posture"}
```

Success is an `application/octet-stream` attachment named `complianceforge-support-<bundle UUID>.zip`, with `private, no-store`, `nosniff`, attachment sandbox headers, and `X-Support-Bundle-SHA256`. It is at most 64 KiB including ZIP overhead, uses no compression, and has exactly four fixed root-level members:

| Member | Contents |
| --- | --- |
| `manifest.json` | Versioned scope, tenant UUID, generation/consent time, excluded classes, safe configuration fingerprint, member sizes and SHA256 hashes |
| `health.json` | Tenant-only queue, notification and connector aggregates; migration state; allowlisted dependency key/status/latency metadata |
| `configuration.json` | Environment enum, numeric release version without internal build labels, safe feature booleans, bounded operational budgets and configuration-posture statuses |
| `README.txt` | Scope, integrity and sharing limitations |

The tenant UUID is intentionally included. Actor and request UUIDs remain in server-side audit evidence, not in the archive. Business records, record UUIDs, user identifiers, raw logs, event payloads, errors, credentials, origins, endpoints, bucket names, KMS identifiers and file paths are excluded. Unknown dependency keys are omitted; invalid status strings become `unknown`. Budget zero means unspecified or outside the reviewed bounds.

The configuration fingerprint hashes the exact `configuration.json` bytes. It does **not** hash complete configuration or individual secrets. Secret rotation or endpoint renaming does not change that fingerprint when safe posture and budgets are unchanged. Member and archive hashes are integrity checks, not signatures, encryption, or proof of origin.

## Generation and consent audit

An append-only `audit_logs` event, `SUPPORT_BUNDLE_GENERATED`, must be inserted before bytes are released. One guarded insert requires a matching RLS tenant and an active, non-deleted actor belonging to that tenant. The event records scope, consent, redaction profile, member-independent archive hash/size, safe configuration fingerprint, actor, request and generation time. It records **generation**, not successful client receipt or support-provider receipt.

Deploy the convergent split-runtime role grants and startup-posture checks. The API needs tenant-scoped diagnostics reads plus `SELECT` on users and `INSERT` on the audit-log parent. Do not grant UPDATE/DELETE on audit evidence or direct access to audit partitions. FORCE RLS and parent-table-only grants are part of the security boundary. Generation fails closed if the consent audit cannot be written; raw database/dependency errors are never returned.

## Sharing procedure

1. Explain the scope and obtain explicit consent for this particular generation.
2. Download and inspect all four members. Confirm the tenant UUID is appropriate to share.
3. Compare the archive SHA256 to the download header or a server-side audit record. Verify member hashes against the exact member bytes in the manifest. A hash embedded in a modified archive alone cannot establish authenticity.
4. Share only through the customer's approved secure support channel. Use its access, expiry and retention controls; this feature does not transmit data or establish those controls.
5. Treat the local ZIP as customer operational metadata. Remove local/provider copies according to the agreed support retention policy. The application cannot recall a downloaded copy.

## Failure handling and verification

Missing/false consent, an unsupported scope, noncanonical principal identifiers, unknown fields, trailing JSON or an invalid media type produce a safe 400 response; oversized requests produce 413. Missing authentication produces 401. Operational, consent-audit or attachment-obligation failures produce a safe 503 with no ZIP bytes, filename or data-derived hash header. Protected-router permission denial is 403.

Evidence lives in `internal/service/support_bundle_service_test.go`, `internal/handler/support_bundle_handler_test.go`, and the forced-RLS PostgreSQL integration test `internal/repository/support_bundle_integration_test.go`. Run the live test with `TEST_DATABASE_URL` set; a skipped integration test is not live evidence. Tests cover cross-tenant sessions/actors, suspended users, audit immutability, failed-consent paths, poisoned metadata, secret-independent fingerprints, member integrity, bounded archives, restrictive field rules and concurrent generation identities.

## Diagnostics browser workflow

At `/settings/diagnostics`, the **Preview support bundle** action requires a verified, effective `settings:configure` permission and a matching authenticated tenant. Read-only administrators can inspect diagnostics but cannot prepare a bundle. The preview identifies the tenant UUID, exact scope, four members, excluded data classes and sharing limitations; opening it never grants consent or generates an archive.

Select the explicit consent checkbox for each generation. Consent is consumed when submission starts. Network/server failures, rejected CSRF, refreshed authentication and redirects never automatically replay this POST; any further attempt requires a new consent action. A refresh-only rejection can rotate the HttpOnly session cookies and return `409 CONSENT_RECONFIRMATION_REQUIRED` without another generation request. Unexpected support redirects fail closed; evidence GET download redirects retain their existing behavior.

The same-origin authenticated/CSRF/BFF path receives only selected attachment metadata. The browser bounds response bodies at 64 KiB even on failure; safe error codes/correlation identifiers can be retained, but raw messages and diagnostic payloads are not rendered. Before enabling a local ZIP download, it checks the canonical filename UUID, archive SHA256 header, four ZIPStore members, fixed reviewed timestamp metadata, regular-file attributes, nonoverlapping entry spans, CRCs, member sizes/hashes, configuration fingerprint, tenant binding, scope and exclusions. It renders verified manifest metadata, not raw health/configuration contents. These checks do not independently establish server authenticity or audit redaction; the server's canonical archive guard owns that boundary.

Prepared bytes and consent stay only in component memory, never browser storage or a provider transport. Cancellation, dialog dismissal, tenant/user/permission change, offline transition, session termination, sign-out intent and unmount abort local work and discard prepared state. Sign-out clears local authentication immediately even if the revocation transport hangs or fails; it still attempts server-cookie revocation. Late responses cannot restore the artifact. A download is an explicit user action with a canonical filename; temporary object URLs are revoked, and receipt is not asserted. Downloaded local/provider copies remain outside application recall or retention control.

Frontend evidence is in `frontend/src/lib/support-bundle*.test.ts`, `frontend/src/lib/server/support-bundle-bff.test.ts`, `frontend/src/components/diagnostics/support-bundle-panel.test.tsx`, `frontend/src/store/auth-store.test.ts`, and `frontend/e2e/support-bundles.spec.ts`. Browser tests exercise a production Next build with mocked same-origin API fixtures; they are not live-backend or manual assistive-technology evidence.

W39 verification: serialized full Vitest passed 83 files / 400 tests with unchanged timeouts; type-check and production build passed; the dependency audit found zero vulnerabilities; full ESLint stayed at zero errors / 315 existing warnings with no added warnings. The production Chromium accessibility suite passed 13 tests, including real browser ZIP/hash verification and local download, keyboard/dialog focus, axe/320-CSS-pixel reflow, permission denial, safe failures/fresh consent and an unexpected same-origin 307 receiver that received no replayed POST (CSP therefore cannot mask the redirect-policy assertion). This is bounded automated evidence, not a full manual WCAG, device, provider-sharing or retention acceptance claim.

Provider-specific secure transfer, signed authenticity evidence, persisted sharing grants/revocation, support-case lifecycle and local/provider retention automation remain separate work. This endpoint is not an impersonation or remote-support capability; roadmap item 148 remains Partial.
