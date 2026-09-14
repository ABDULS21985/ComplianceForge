# Release and supply-chain runbook

## Required repository controls

Configure these outside the repository before treating the workflows as a production release boundary:

- Protect `main`, `develop`, and release tags. Require the CI, dependency review, and restore-drill checks; block force pushes and tag movement.
- Restrict GitHub Actions to approved actions pinned by full commit SHA. Require review for `.github/workflows/**`, Dockerfiles, migration/seed files, and `scripts/ci/**` through CODEOWNERS or an equivalent ruleset.
- Configure the `release`, `staging`, and `production` environments with separate approvers and least-privilege secrets. The release job needs GitHub OIDC (`id-token: write`) and GHCR package write access.
- Private repositories need GitHub Advanced Security for Dependency Review and an eligible GitHub plan for hosted artifact attestations. The checksum-pinned CLI scans still run without those products.
- Supply base-image registry credentials or a pull-through cache with retention and malware controls. CI scanners need outbound access to GitHub release assets, the Go checksum/module services, npm advisory services, and the Trivy vulnerability database.
- Configure `KUBE_CONFIG_STAGING`, `KUBE_CONFIG_PRODUCTION`, deployment/namespace variables, and cluster RBAC so each environment credential can update only its named workloads. Prefer short-lived workload identity over long-lived kubeconfig material when the cluster provider supports it.

## Pull-request gates

The main CI workflow performs:

- Go 1.26.8 compilation/tests, Go vet, golangci-lint 2.13.2, Gosec 2.28.0, and govulncheck 1.8.0.
- Frontend lint/type/unit/E2E checks on supported Node.js 24 LTS.
- Migration pair/continuity validation, complete seed-manifest validation, clean `up/down/up`, and double seed application.
- Full-history Gitleaks 8.30.1 scanning and Trivy 0.74.0 source, secret, IaC, dependency, license, and image scanning.
- SPDX JSON SBOM creation with Syft 1.51.1 for API, worker, migrator, and frontend images.
- High/critical fixed-vulnerability image gates and retained JSON/SARIF evidence.

The Go lint job currently uses the action's changed-code ratchet: pull requests and
pushes fail when they introduce a new finding, while pre-existing findings remain
visible without blocking unrelated delivery. Run `golangci-lint run --timeout=5m
./...` periodically to measure and burn down the repository-wide baseline; remove
`only-new-issues` only after that command is clean.

Third-party actions use immutable commit SHAs. Scanner release archives are HTTPS-only, version-pinned, and verified against upstream SHA-256 values in `scripts/ci/install-security-tools.sh`. The Go vulnerability client is pinned to the current release supported by the CI toolchain and is verified through Go's module checksum mechanism.

Gitleaks scans the complete Git history. Its ignore file contains only exact
historical fingerprints for reviewed synthetic configuration-test fixtures;
never add a path-wide, rule-wide, or regular-expression exemption there.

The separate Dependency Review workflow blocks moderate-or-higher vulnerabilities across runtime and development scopes and enforces an explicit permissive-license allowlist for newly introduced dependencies. Security/Legal must approve changes to that allowlist; do not use per-package exceptions without an owner, expiry date, and recorded license analysis.

## Produce a release

1. Confirm all required checks passed at the exact commit.
2. Create a protected, annotated semantic-version tag such as `v1.4.2`, or manually dispatch `Signed Container Release` with that full version.
3. Approve the `release` environment gate.
4. The workflow builds all four runtime targets and publishes only the full semantic version and full commit-SHA tags. BuildKit publishes SBOM and maximal provenance attestations. GitHub/Sigstore provenance and a keyless Cosign signature are attached to each immutable digest.
5. Record the four image digests in the change ticket. Deploy and roll back by digest where the target platform supports it; the branch deployment workflow uses full commit-SHA tags and never `latest` or `develop`.

Example verification:

```sh
cosign verify \
  --certificate-identity-regexp='^https://github.com/ORG/REPO/.github/workflows/release.yml@' \
  --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
  ghcr.io/ORG/REPO/api@sha256:...

gh attestation verify \
  oci://ghcr.io/ORG/REPO/api@sha256:... \
  --repo ORG/REPO
```

Verify the worker, migrator, and frontend digests independently. Admission policy should reject unsigned images and unapproved workflow identities before production rollout.

## Proxy and container runtime contract

The production Compose topology sets `API_TRUST_PROXY_HEADERS=true` only on the API because nginx is the sole ingress and overwrites the forwarded client headers. The frontend separately sets `TRUST_PROXY_HEADERS=true`, `APP_ENV=production`, and the server-only `API_INTERNAL_URL=http://api:8080/api/v1`. Nginx forwards `Host`, `X-Forwarded-Host`, `X-Forwarded-Proto`, and a normalized forwarding chain. Do not expose either container directly while trust is enabled.

Development keeps both trust switches false. For deployments behind a different trusted proxy, set optional `APP_ORIGIN=https://public.example` when the proxy cannot supply a stable external host/protocol. Never use `NEXT_PUBLIC_API_URL`; browser traffic stays same-origin through the frontend BFF and nginx.

API and frontend images have readiness/health probes and graceful stop signals. Compose gives API/worker 45 seconds to drain and frontend/nginx 30 seconds. Kubernetes manifests must preserve equivalent startup, readiness, liveness, termination-grace, resource, and disruption-budget settings; the repository does not currently own those manifests.

## Version updates and exceptions

Dependabot proposes weekly GitHub Action, Go, npm, and Docker updates. A scanner or base-image update must include upstream release-note review, checksum/digest update, CI evidence, and a successful restore drill when PostgreSQL tooling changes. Never replace a full action SHA with a mutable major tag.

Security findings are blocking by default. A time-bounded exception must include affected artifact/digest, finding identifier, exploitability assessment, compensating control, accountable owner, approval, and expiration. Store the decision in the risk register; do not suppress it only in workflow YAML.

## Current external boundaries

The repository produces and signs evidence, but the organization must still configure tag/ruleset protection, environment approvals, OIDC trust, GHCR retention, admission verification, a binary-transparency or artifact-retention policy, vulnerability alert routing, legal review, and credential rotation. The Kubernetes deploy jobs do not provision clusters or rollback policy; those remain infrastructure responsibilities.
