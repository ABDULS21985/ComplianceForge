# Secure development lifecycle

## Ownership

Product Security owns this process. Each of Identity, API Platform, Frontend,
Data/Database, Reliability, and Integrations appoints a security champion and a
backup. Repository administrators record the named rotation in the private
on-call/ownership system and configure matching CODEOWNERS or repository rules;
personal names are deliberately not embedded in this public codebase.

Champions review design changes in their boundary, maintain test evidence,
triage scanner findings, and escalate overdue architecture risks. Product
Security owns exceptions, threat-model quality, penetration-test scope, and
verification of critical/high remediation.

## Change classification

Every pull request answers the security section in the repository template.
The author identifies affected threat-model IDs and assigns one class:

- `standard`: no trust-boundary or sensitive-data change;
- `security-sensitive`: changes authn/authz, tenant scoping, secrets,
  cryptography, uploads, public portals, external network calls, queues,
  database privileges, audit/legal-hold records, AI context, or security
  configuration;
- `architecture`: introduces or materially changes a trust boundary, privileged
  identity, public interface, data residency/retention behavior, or executable
  extension.

Security-sensitive changes require a domain security champion. Architecture
changes also require Product Security and an updated threat model/risk record.
An author cannot provide the sole security approval for their own change.

## Required gates

Before merge:

1. Unit, integration, fuzz/property, migration, OpenAPI, frontend, and relevant
   browser tests cover both the allowed path and a meaningful denial/failure
   path.
2. Gosec, govulncheck, Gitleaks, npm audit, Trivy vulnerability/secret/IaC and
   license checks run without an unapproved actionable finding.
3. New dependencies and actions are pinned and pass dependency/license review.
4. New logs and errors are checked for credentials, tokens, signed URLs,
   personal data, and provider internals.
5. Schema and role changes include least-privilege grants, FORCE-RLS behavior,
   clean install, populated upgrade, and safe downgrade evidence.
6. Public or automation contract changes update OpenAPI and exact frontend
   contract checks; breaking changes have a version/deprecation plan.
7. The threat model and architecture register change when the design or
   residual risk changes.

Before production promotion, require the exact artifact's test/scan/SBOM/
provenance/signature evidence, migration and rollback plan, environment
approval, open-risk review, restore-drill freshness, and a canary/rollback
decision. Emergency changes may shorten lead time but may not skip attribution,
artifact scanning, independent approval, or a dated follow-up review.

## Security testing cadence

- Continuous: blocking static, dependency, secret, image, IaC, unit,
  integration, and bounded fuzz checks on changes.
- Monthly: architecture-risk and vulnerability-SLA review; privileged database
  posture and tenant-isolation smoke evidence.
- Quarterly: threat-model review, queue replay and restore exercises, malicious
  upload corpus, access certification, and targeted abuse-case testing.
- Annually and before first production launch: independent penetration test of
  authenticated application, public portals, APIs, tenant isolation, object
  pipeline, cloud/deployment configuration, and social/operational paths in
  agreed scope.
- After a material auth, tenant, cryptographic, public-portal, upload/parser,
  extension, or network-boundary change: targeted independent retest before
  broad rollout.

Critical findings block release. High findings block release unless the CISO
records a time-bounded acceptance with compensating controls. Retests, not
developer assertions, close critical/high penetration findings. Reports and
customer data stay in the restricted evidence system; the repository stores
only sanitized finding identifiers and remediation status.

## Exception record

An exception must include affected artifact and tenants, finding/control ID,
exploitability, impact, compensating controls, telemetry, accountable owner,
independent approver, created/expiry dates, and a remediation ticket. CI
suppression without that record is invalid. Expired exceptions automatically
return to blocking status.
