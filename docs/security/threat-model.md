# ComplianceForge platform threat model

Document owner: Product Security  
Technical co-owners: Platform Engineering, Identity Engineering, Data Governance  
Last reviewed: 2026-09-15  
Mandatory review: before production launch, after a trust-boundary change, and at least quarterly

## Scope and security objectives

This model covers the browser application and same-origin BFF, public vendor and
board portals, Go API and worker processes, PostgreSQL, Redis, RabbitMQ, private
object storage, malware scanning, outbound notification and connector traffic,
deployment/migration identities, and the software delivery pipeline.

The primary objectives are:

1. A tenant can never read, mutate, schedule, or infer another tenant's data.
2. Authentication, authorization, evidence, audit, and legal-hold history fail
   closed and remain attributable.
3. Untrusted uploads, connector responses, webhook destinations, and AI content
   cannot cross execution or network trust boundaries.
4. Runtime compromise does not automatically grant schema-owner, migration,
   custody-ledger-owner, or cross-tenant privileges.
5. Recovery preserves confidentiality and auditability as well as availability.

## Assets and trust boundaries

| Boundary | Untrusted or less-trusted side | Privileged side | Required control |
| --- | --- | --- | --- |
| Browser to BFF | Browser state, URLs, form data | HttpOnly session cookies | Same-origin routing, CSRF and Origin checks, output encoding, no browser bearer tokens |
| Public portal to API | Expiring invitation/access token | Tenant records and files | Purpose-bound hashed tokens, object authorization, expiry/revocation, audited downloads |
| API to PostgreSQL | Authenticated application claims | Tenant data and immutable ledgers | Persisted RBAC/ABAC, request tenant context, FORCE RLS, non-owner login, narrow capabilities |
| Worker to PostgreSQL | Queue messages and schedules | Cross-tenant job discovery | Dedicated non-bypass login, bounded registry capability, per-tenant connections, idempotency |
| API to object pipeline | Multipart bytes and metadata | Private evidence bucket | Size/type/structure checks, quarantine, malware scan, checksum, tenant-prefixed keys |
| API/worker to external network | User-configured URLs and providers | Internal network and credentials | Destination resolution/pinning, private-range denial, TLS, timeouts, egress policy, secret redaction |
| Broker to worker | Delayed, duplicated, forged, or stale messages | Domain mutations | Versioned envelopes, tenant match, publisher confirms, inbox deduplication, leases, dead letter handling |
| Deployment to database | Reviewed migration artifact | Schema and capability ownership | Dedicated migration identity, advisory locking, deterministic migration, no runtime credential reuse |
| CI to release | Source and third-party dependencies | Signed deployable images | Pinned actions/tools/images, tests/scans, SBOM/provenance, protected environments, digest deployment |

High-value assets include tenant records, personal and regulatory data, evidence
objects and hashes, legal holds, audit/custody ledgers, signing/encryption keys,
session and portal tokens, connector credentials, database backups, queue payloads,
release credentials, and customer-visible reports.

## Threat scenarios and required verification

| ID | Threat | Required prevention/detection | Verification evidence | Residual-risk link |
| --- | --- | --- | --- | --- |
| TM-01 | Cross-tenant query, relationship, cache key, or object reference | Tenant-scoped repositories; composite tenant foreign keys; FORCE RLS; tenant-prefixed objects; deny-by-default routes | Non-superuser/NOBYPASS integration tests with cross-tenant denial assertions | AR-001 |
| TM-02 | Session theft, fixation, refresh replay, CSRF, or confused origin | Rotating one-time refresh sessions; Secure/HttpOnly/SameSite cookies; issuer/audience/type/JTI checks; CSRF and Origin validation | Auth replay, wrong-token-type, proxy-origin, and logout-revocation tests | AR-002 |
| TM-03 | SSRF, DNS rebinding, metadata access, or redirect escape | HTTPS policy; resolved-address pinning; private/loopback/link-local/metadata denial; redirect revalidation; bounded response | Safe-HTTP unit/fuzz tests and controlled egress monitoring | AR-003 |
| TM-04 | Malware, active content, archive bomb, path traversal, or symlink escape in evidence | Multipart cap; format allowlist and structural inspection; quarantine; ClamAV; immutable random key; private attachment responses | Malicious corpus tests, storage traversal fuzzing, scanner health metrics | AR-004 |
| TM-05 | Evidence, review, legal-hold, or custody history tampering | Server-derived hashes; append-only review/custody records; chained event hashes; owner-separated SECURITY DEFINER writers; legal-hold deletion guards | Adversarial direct-DML tests, chain verification, migration downgrade-loss guard | AR-005 |
| TM-06 | Queue replay, tenant substitution, poison payload, or duplicate side effect | Signed/trusted producer boundary; versioned tenant envelope; payload/envelope tenant equality; durable inbox/outbox; retry/dead-letter limits | RabbitMQ/PostgreSQL contract tests and replay drills | AR-006 |
| TM-07 | Runtime role becomes database owner, BYPASSRLS, superuser, or trusted-owner member, or a privileged configured login masquerades as a restricted current role | Separate API/worker/migrator URLs; bind configured login to both current_user and session_user before production catalog checks; NOLOGIN capability owners; audited grant manifest | Actual-login PostgreSQL smoke tests, SET ROLE/SET SESSION AUTHORIZATION masquerading rejection and startup-failure tests | AR-001 |
| TM-08 | Credentials or regulated values leak through logs, errors, exports, or support tooling | Classified response rules; safe error taxonomy; secret redaction; no signed-URL/provider-error logging; bounded allowlisted support archives with explicit consent and durable tenant-scoped generation audit | Response-contract tests; support-bundle poisoned-source, wrong-tenant, failed-consent-audit and forced-RLS integration tests; secret scan and log sampling | AR-007 |
| TM-09 | Public portal token grants excess scope or remains useful after revocation | Hash at rest; explicit tenant/resource/purpose; short expiry; one-time or revocable state; download audit; sponsor policy | Portal expiry/revocation/object-authorization E2E tests | AR-008 |
| TM-10 | Dependency, CI action, base image, or release artifact compromise | Immutable pins/checksums; dependency review; Gosec/govulncheck/Trivy/Gitleaks; SBOM; provenance; signature verification | Blocking CI and immutable release evidence | AR-009 |
| TM-11 | AI prompt injection causes disclosure or unauthorized action | Tenant-controlled enablement; grounded allowlisted context; structured output; no direct mutation; human approval; provider retention/redaction policy | Adversarial evaluation set and audit-log review before production enablement | AR-010 |
| TM-12 | Dependency outage or crafted workload causes loss of service or partial commit | Deadlines, quotas, circuit breakers, durable transactions, graceful degradation, backups and restore drills | Load/failure tests, SLO alerts, restore evidence, game days | AR-011 |
| TM-13 | Runtime-created temporary relations shadow privileged-function queries, or obsolete pool sessions retain shadow objects after grants are tightened | Revoke runtime/PUBLIC database TEMP and schema CREATE; explicitly place pg_temp last in reviewed privileged-function search paths; validate catalog posture; recycle deployment pools after tightening | Split-login live tests for CREATE TEMP denial, hostile temporary-relation shadowing, manifest reapplication and startup rejection; migration/runbook review | AR-013 |
| TM-14 | Alternative UUID spellings defeat self-approval checks, delete/regrant resets defeat optimistic concurrency, or timed/exception-backed administrative authority leaves a tenant locked out | Canonical non-nil principal and resource UUIDs; immutable assignment incarnation plus expected version; independent current approver; statement-time role windows/SoD enforcement; another unaffected permanent administrator and consistent mutation locks | Adversarial UUID, assignment ABA, expired/future window, independent approval, direct-DML, concurrent revocation and last-permanent-admin live forced-RLS tests | AR-014 |

## Data-flow review rules

A change requires a threat-model review when it adds a listener, public route,
authentication factor, token, database role or SECURITY DEFINER function,
cross-tenant scheduler, external destination, executable content path, object
format, encryption key, AI provider/tool, queue message type, or release
credential. The pull request must name affected threat IDs and add a scenario
when no existing row covers the new flow.

The model is design evidence, not proof that controls are operating. Open or
partially verified controls remain in the architecture risk register and cannot
be removed merely because their target design appears here.
