## Outcome

Describe the user or operational outcome and the production path exercised.

## Verification

- [ ] Relevant unit/integration/browser tests cover success and denial/failure paths
- [ ] API/OpenAPI/frontend contracts and migration up/down evidence are updated where applicable
- [ ] Logs, errors, exports, and screenshots contain no secrets or unnecessary regulated data
- [ ] Rollout, compatibility, monitoring, and rollback implications are described

## Security and privacy review

Change class: `standard` / `security-sensitive` / `architecture`

Affected threat-model IDs (or `none` with rationale):

- [ ] Tenant and object authorization remain explicit and fail closed
- [ ] New external destinations use the pinned safe-HTTP path and bounded timeouts
- [ ] New uploads/content formats pass quarantine, validation, and resource limits
- [ ] Database changes preserve FORCE RLS, least privilege, and immutable history
- [ ] Tokens, credentials, personal data, retention, residency, and legal holds were considered
- [ ] Queue/event changes validate tenant, version, replay, and idempotency semantics
- [ ] AI changes are grounded, redacted, evaluated, audited, and require human approval for mutations
- [ ] Threat model and architecture-risk register were updated, or this change does not alter them

Security reviewer (required for `security-sensitive` or `architecture`):

Product Security reviewer (required for `architecture`):
