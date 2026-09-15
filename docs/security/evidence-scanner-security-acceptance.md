# Evidence scanner and download security-gate acceptance

This records the bounded W41 evidence workstream reviewed on 2026-09-15.
It is not a penetration-test result, a storage-endpoint certification, or
completion of the 200-item enterprise programme.

## Implemented boundaries

- JSON and OpenXML inspection use the already-open, owner-only spool file,
  rather than reopening its pathname. Bounded section readers preserve the
  original stream offset. JSON structure inspection remains limited to
  25 MiB; increasing the general upload budget does not remove that limit.
- Upload configuration above 2 GiB is rejected before ingestion. Office
  compressed-size validation precedes unsigned conversion and expansion
  budget multiplication. Each entry is checked against the remaining
  expanded budget before addition, including forged ZIP64 sizes.
- ClamAV INSTREAM lengths must fit the 64-KiB read buffer before conversion
  or slicing. Invalid reader counts fail closed. Repeated zero-length reads
  without an error terminate with `io.ErrNoProgress`; they cannot produce
  a clean scan result.
- Evidence downloads reject missing, empty, and mixed stream/presigner
  responses before a custody authorization is recorded. Rejected streams
  are closed. Presigner URLs must be bounded absolute HTTPS URLs without
  user information, fragments, malformed query encoding, control characters,
  or backslashes. Rejections do not emit a `Location` header.
- Integrity-verification and uncommitted-object cleanup logs use stable
  failure codes instead of raw provider errors. Regression fixtures inject
  signed-URL and credential-shaped provider text and assert it is absent
  from logs and user-visible failures.

The G710 annotation applies only to the reviewed server-side presigner
redirect after object authorization and these checks. It is not a blanket
scanner exclusion. Endpoint authority still comes from reviewed deployment
configuration, not a hostname allowlist in this handler. Local S3-compatible
presigned redirects therefore need HTTPS too; filesystem development storage
continues to stream through the private authenticated route.

## Executed checks

- `go test -p 2 -count=1 ./internal/pkg/evidence` passed.
- The evidence package passed the same tests with `-race`.
- `go test -p 2 -race -count=1 -run '^TestControlHandler.*(Evidence|Presigner)' ./internal/handler`
  passed, including invalid and mixed response rejection before custody or
  redirect, safe errors, and successful private-stream/presigner paths.
- `go test -p 2 -count=1 -run '^TestEvidenceObjectService' ./internal/service`
  passed, including server-derived metadata, checksum rejection, cleanup,
  bounded signer lifetime, and poisoned provider-log redaction.
- Pinned Gosec 2.28.0 scoped scans for the evidence package and handlers
  passed. The root's full `gosec -quiet ./...` on the frozen W41 source also
  exited 0 with zero findings. This is a point-in-time static-analysis result;
  later source changes require a fresh scan.
- The combined `go test -p 2 ./...` and `go vet -p 2 ./...` passed on that
  frozen source. Subsequent database-login startup hardening is tracked by
  its own combined gates rather than inferred from those earlier results.

Fixtures cover renamed spool files, preserved offsets, mismatched and extreme
signed sizes, unsigned ZIP64 overflow, invalid ClamAV read counts, stalled
readers, invalid presigner variants, nil/empty/mixed delivery, and provider
error text. Unit and race tests are not a live ClamAV deployment/signature
freshness test, bucket IAM review, production upload-capacity measurement,
or evidence reconciliation/retention operating evidence.

See [the object security contract](../evidence-object-security.md) and
[MFA/SCIM acceptance](mfa-scim-security-acceptance.md) for the adjacent controls.
