# Evidence object security

ComplianceForge treats every uploaded evidence file as untrusted. The object
pipeline derives the filename, byte count, media type, and SHA-256 digest on the
server; browser-supplied file metadata is never written directly to the evidence
record.

## Upload state machine

1. The HTTP layer caps the complete multipart request and accepts exactly one
   `file` part plus the documented metadata fields.
2. The pipeline streams the file into an owner-only temporary spool while
   enforcing the configured byte limit and calculating SHA-256.
3. Extension, declared media type, magic bytes, and format-specific structure
   are compared. Active formats such as HTML and SVG, executables, macro-enabled
   Office files, encrypted Office containers, archive traversal, embedded
   objects, and archive bombs are rejected.
4. The object is written under a tenant-specific `quarantine/` key.
5. ClamAV scans the same bytes with its bounded `INSTREAM` protocol. Scanner
   errors and indeterminate verdicts fail closed; the object is never promoted.
6. A clean object is copied under a random tenant-specific `evidence/` key and
   the quarantine copy is removed. The database transaction atomically checks
   the tenant storage entitlement before recording the server-derived object
   metadata. A failed transaction triggers best-effort object cleanup.

Malware signatures and internal object keys are operational data. They must not
be returned to end users or written to high-cardinality metrics. Quarantined
objects must never be reachable through a download route.

## Required configuration

```dotenv
EVIDENCE_MAXIMUM_UPLOAD_BYTES=26214400
EVIDENCE_SCANNER_NETWORK=tcp
EVIDENCE_SCANNER_ADDRESS=clamav.internal:3310
EVIDENCE_SCANNER_TIMEOUT_SECONDS=30
EVIDENCE_SIGNED_DOWNLOAD_SECONDS=300
HTTP_READ_TIMEOUT_SECONDS=120
HTTP_WRITE_TIMEOUT_SECONDS=360
```

`EVIDENCE_SCANNER_NETWORK` may be `tcp` or `unix`; Unix socket paths must be
absolute. Configure clamd's `StreamMaxLength` at least as high as
`EVIDENCE_MAXIMUM_UPLOAD_BYTES`. Network policy should permit the API workload
to reach only the scanner endpoint, and the scanner should run without access
to application secrets.

The public HTTP write deadline must be at least the read deadline plus the
scanner deadline. Configuration validation enforces that relationship so an
accepted multipart upload has enough time to finish ingestion and scanning.

Production and staging require S3-compatible private object storage. AWS
credentials are obtained from the standard workload-identity credential chain,
not from application configuration.

```dotenv
STORAGE_TYPE=s3
S3_BUCKET=complianceforge-evidence
S3_REGION=eu-west-2
S3_KMS_KEY_ID=arn:aws:kms:eu-west-2:123456789012:key/...
S3_EXPECTED_BUCKET_OWNER=123456789012
```

`S3_ENDPOINT` and `S3_FORCE_PATH_STYLE` are intended for compatible development
services. A custom endpoint must use HTTPS in staging and production.

The adapter requests SHA-256 transport checksums, private/no-store response
semantics, and server-side encryption. When a KMS key is configured it uses
SSE-KMS with S3 Bucket Keys; otherwise it explicitly requests SSE-S3. The
expected-owner pin prevents a valid credential from silently addressing a
same-named bucket in another AWS account.

## Bucket controls

Apply these controls outside the application as infrastructure policy:

- enable all S3 Block Public Access settings and Bucket Owner Enforced object
  ownership;
- deny requests made without TLS, public ACLs, and unencrypted writes;
- grant the API role only `GetObject`, `PutObject`, and `DeleteObject` on the
  application prefixes in the named bucket;
- enable versioning and access logging; consider Object Lock governance mode
  where the organisation's retention policy requires immutable evidence;
- use a lifecycle rule for quarantined-object expiry only after the security
  investigation and legal-hold requirements are defined;
- alert on direct object changes made by identities other than the application
  role.

Signed downloads are generated only after an authorised, tenant-scoped database
lookup. Their lifetime is capped at 15 minutes, content is forced to an
attachment with `application/octet-stream`, and quarantine keys are rejected.
The API accepts only a bounded absolute HTTPS URL emitted by the deployment's
configured object-store presigner. Relative/non-HTTPS URLs, URL user information,
fragments, control characters, malformed query strings and mixed stream/redirect
responses fail closed before custody authorization is recorded. The deployment
owner must review the storage endpoint; this is not a customer-selected redirect
parameter or a general outbound-destination allowlist. Local S3-compatible
presigned redirects require TLS too; filesystem development storage streams
privately instead.
Local development streams the same private object through the authenticated API
with `nosniff`, `no-store`, and a restrictive document sandbox.

Before issuing either form of download, the application privately reads the
object and compares its exact byte count and SHA-256 digest with the immutable
server-derived database values. A mismatch fails closed before a stream or
signed URL is exposed. S3 reads additionally request provider checksum
validation and pin the expected bucket owner.

JSON and Office structure inspection reads the already-open private spool,
without reopening its pathname or moving its stream offset. JSON inspection
remains capped at 25 MiB. Upload configuration is capped at 2 GiB, and Office
expanded-size accounting rejects signed-size conversion and unsigned-addition
overflow before any expansion. ClamAV chunk lengths are checked against the
64-KiB buffer before conversion/slicing; invalid and non-progressing readers
cannot emit a chunk or receive a clean verdict.

## Failure handling

- Scanner unavailable or unknown verdict: return a retriable service-unavailable
  response and retain the quarantine copy.
- Malware found or content mismatch: return a non-retriable rejection without
  creating an evidence record.
- Storage quota exceeded or database transaction failed: delete the promoted
  object with a cancellation-independent bounded cleanup context.
- Quarantine cleanup failed after promotion: preserve both keys and record a
  reconciliation marker; never discard the clean object after a successful
  scan.
- Download signing or object read failed: return a generic unavailable response;
  provider errors and signed URLs must not be logged.

Integrity-verification and uncommitted-object cleanup logs retain only stable
failure codes, never raw storage-provider errors, URLs or credential text.

Operations should monitor scanner reachability and latency, rejected and
quarantined counts, cleanup backlog, upload byte totals, quota denials, and
signed-download failures. Metrics must use bounded labels only.
