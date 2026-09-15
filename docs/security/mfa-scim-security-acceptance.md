# MFA and SCIM security-gate acceptance

This is the bounded W41 backend remediation evidence dated 2026-09-15,
not a claim that the whole enterprise security roadmap is verified.
Migration 058, runtime role grants, support-bundle production code and
frontend code were not changed by this workstream.

## Meaningful fixes

- `internal/service/identity_mfa_service.go` rejects negative TOTP moving
  factors before converting to `uint64`. Verification also rejects a
  pre-Unix-epoch injected clock and skips the negative adjacent window at
  the epoch. Nonnegative counters and the existing replay guard remain
  supported.
- `internal/repository/identity_mfa_repo.go` checks the PostgreSQL BIGINT
  signature counter against `0..4294967295` before converting to WebAuthn's
  `uint32`; scan errors and invalid counters return no usable credential.
  Authentication updates also require a stored counter within that domain,
  including the clone-warning path. Failure rolls back both the credential
  mutation and challenge consumption.

Passkey model and mutation inputs already use `uint32`, so incoming typed
values cannot exceed the WebAuthn counter domain. The legacy migration 053
database CHECK still requires only `sign_count >= 0`: no historical migration
was rewritten. Existing rows above the WebAuthn maximum therefore fail closed
and need a separately reviewed data repair; they are never silently wrapped.

## Narrow reviewed scanner annotations

There are seven rule-specific source annotations; no rule was disabled
globally and neither integer conversion is suppressed.

- One G505 annotation covers only the `crypto/sha1` import used as the HMAC
  primitive for the existing six-digit RFC 4226/6238 TOTP profile. This is
  not unkeyed SHA-1, a password hash, a signature algorithm or a new general
  hashing choice. The enrollment algorithm remains SHA1 to avoid invalidating
  existing authenticators. RFC 4226 Appendix D vectors are regression-tested.
- Four G101 annotations identify the public SCIM permission labels in
  `internal/models/scim.go`; these strings are scope names, not credentials.
- One G101 annotation identifies the typed context key in
  `internal/middleware/scim_auth.go`; its value is a token-record UUID key,
  not the bearer token.
- One G101 annotation identifies the static SQL projection in
  `internal/repository/scim_repo.go`. `token_hash` is a column identifier,
  not a hardcoded hash or secret.

The SCIM protocol labels, credential namespace, hashed token storage,
tenant binding, exact-scope authorization and token logging behavior are
unchanged.

References: [RFC 4226](https://www.rfc-editor.org/rfc/rfc4226.html),
[RFC 6238](https://www.rfc-editor.org/rfc/rfc6238.html).

## Executed proof

- Focused identity/TOTP, counter-scanning and SCIM unit tests passed.
- `go test -p 2 ./...` passed against the frozen shared backend source,
  including the latest root notification and evidence changes.
- The same focused service/repository/middleware/model tests passed with
  the race detector; scoped and whole-repository `go vet -p 2 ./...` passed.
- The expanded `TestIdentityLifecycleWithNonSuperuserTenants` passed with
  `-race` on an independent PostgreSQL 16 database at clean schema 58. The
  fixture uses an actual NOSUPERUSER/NOBYPASSRLS/NOINHERIT role under SET ROLE,
  not a claim of production login/ACL-manifest certification. Proof includes
  cross-tenant credential rejection, a maximum-uint32 insert/read, an
  above-maximum stored BIGINT rejection, clone-warning mutation rejection,
  unchanged credential state and challenge rollback followed by a successful
  valid-counter update. Post-test catalog checks found zero identity fixture
  organizations and zero identity fixture roles.
- New `identity_mfa_security_test.go` files in service and repository cover
  minimum/maximum signed values, negative values, both valid WebAuthn bounds,
  scan-error propagation without a partial credential, RFC HOTP vectors,
  pre-epoch clocks, the epoch window and used-step rejection.
- Pinned Gosec 2.28.0 with `GOTOOLCHAIN=go1.26.8` and the scoped
  `G101,G115,G505` rules passed with exit 0. Root independently reported the
  frozen-source full `gosec -quiet ./...` run passing with exit 0 and zero
  findings. That is a scanner result, not a blanket security certification.
- Touched Go files are gofmt-clean and `git diff --check` passed.

Live PostgreSQL tests require `TEST_DATABASE_URL`; without it they skip.
No external MFA provider certification, recovery ceremony audit, FIPS claim
or production runtime-role repin is included in this evidence.
