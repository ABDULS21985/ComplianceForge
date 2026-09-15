# Architecture security risk register

Register owner: Product Security  
Last reviewed: 2026-09-15  
Review cadence: monthly and before every production promotion

Status values are `open`, `mitigating`, `accepted`, or `closed`. Acceptance
requires a named accountable executive, compensating controls, and an expiry;
none of the entries below is implicitly accepted.

| ID | Threat IDs | Risk and impact | Current control | Required treatment and exit evidence | Accountable role | Target review | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| AR-001 | TM-01, TM-07 | A runtime or scheduler database identity could bypass or fail to enter tenant RLS context, or hide a privileged authenticated login behind SET ROLE, causing disclosure or silently skipped jobs | FORCE RLS, request tenant context, separate evidence owners, configured/current/session login binding before production posture checks, integration tests | Complete API/worker/migrator grant manifest; verify actual-login startup and masquerading denial; split-role smoke test for representative CRUD, queues, and every scheduler; exercise deployment credential separation and pool recycle | Platform Security Lead | 2026-09-21 | mitigating |
| AR-002 | TM-02 | OIDC and SAML paths do not yet have complete production conformance evidence, increasing account-takeover risk | Local auth, MFA, secure BFF sessions, rotating refresh tokens | Complete discovery/signature/nonce/state/PKCE/clock-skew/certificate-rotation tests and tenant-routing E2E | Identity Engineering Lead | 2026-10-15 | open |
| AR-003 | TM-03 | New connectors may bypass the pinned safe-HTTP path or exceed intended egress | Shared safe-HTTP client and webhook destination pinning | Inventory every outbound call, enforce the shared transport, and verify runtime egress allowlists | Integrations Lead | 2026-10-15 | mitigating |
| AR-004 | TM-04 | Parser/scanner coverage can miss novel malicious document structures | Quarantine, structural validation, ClamAV, private storage, byte/digest checks | Maintain malicious corpus; add scanner signature-age SLO; commission upload-pipeline adversarial test | Product Security Lead | 2026-10-31 | mitigating |
| AR-005 | TM-05 | Privileged database or storage administration can still alter evidence outside application controls | Hash-chain verification, checksums, non-login capability owners, legal holds | Export chain heads to an independently retained log; define break-glass monitoring and periodic verification job | Data Governance Lead | 2026-11-15 | open |
| AR-006 | TM-06 | Poison-message investigation and approved replay tooling are incomplete | Durable inbox/outbox, bounded retries, dead-letter topology | Add redacted inspection, approval-bound replay, ownership, and a quarterly replay drill | Reliability Lead | 2026-11-15 | open |
| AR-007 | TM-08 | Heterogeneous legacy error/response paths may expose internal details; support-sharing mistakes can disclose tenant metadata | Classified response helpers, credential redaction, bounded allowlisted support ZIP, explicit per-generation consent and durable audit; no automatic upload | Finish response/error conformance for every mounted production route; verify support preview/consent/download lifecycle and controlled sharing/retention; sample production-shaped logs | API Platform Lead | 2026-10-31 | mitigating |
| AR-008 | TM-09 | Vendor and board portal expiry/revocation/browser flows lack complete E2E evidence | Scoped portal handlers and token controls | Add browser E2E for expiry, revocation, uploads, partial saves, and object authorization | Product Engineering Lead | 2026-10-15 | open |
| AR-009 | TM-10 | `sharp`/libvips licensing and external repository protection/admission settings remain operational dependencies | Pinned CI tools/actions, scans, SBOM/provenance, signed release workflow | Close legal review; enforce branch/environment/admission rules; retain a verified release evidence pack | Engineering Director | 2026-09-30 | mitigating |
| AR-010 | TM-11 | AI assistance is not yet governed by a production evaluation, retention, and prompt-injection control plane | Human-facing draft workflow and tenant feature control | Implement provider/model policy, redaction, grounded citations, evaluation gates, audit log, and kill switch | AI Governance Owner | 2026-11-30 | open |
| AR-011 | TM-12 | Regional failover and dependency game-day evidence is incomplete | Durable queues, health checks, backup and restore tooling | Define SLO/RTO/RPO; run object-store, queue, credential, database, and regional failure exercises | Reliability Lead | 2026-12-15 | open |
| AR-012 | TM-01–TM-14 | No independent penetration-test report has yet been attached to a release decision | Internal automated and adversarial tests | Complete an independent test before production launch and annually; retest critical/high findings before release | CISO / Security Executive | 2026-10-31 | open |
| AR-013 | TM-07, TM-13 | Existing pool sessions or a migration that resets function search paths can reintroduce temporary-relation shadowing of privileged queries | Runtime grant manifest and catalog posture hardening in progress; NOLOGIN capability owners; no automatic termination of customer sessions | Complete clean schema-58 split-role live shadowing/denial proof; reapply reviewed manifest after each migration; document and exercise safe pool recycle before accepting traffic | Database Platform Lead | 2026-09-21 | mitigating |
| AR-014 | TM-05, TM-14 | Access-governance identity aliases, assignment reincarnation or expiry/SoD edge cases could permit self-approval, stale revocation or eventual administrative lockout | Canonical UUID validation, immutable assignment incarnation, independent approval, guarded effective-role view and permanent-backup checks in progress | Complete expanded schema-58 non-owner forced-RLS and race proofs; independently exercise UUID aliases, delete/regrant ABA, expiry, exception revocation and every ordinary admin-deprovision mutation | Identity Engineering Lead | 2026-09-21 | mitigating |

## Decision record requirements

Each update must retain the old decision in version control and include the
evidence location, reviewer, review date, treatment rationale, and any changed
deadline. A risk cannot become `closed` solely because code was merged: its
exit evidence must have run against the production-shaped path. An `accepted`
risk must state the accepting person's name and role, expiry date, affected
tenants/data, and compensating monitoring in a linked decision record.
