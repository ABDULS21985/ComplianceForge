# PostgreSQL backup and disaster-recovery runbook

Owner: Platform Engineering  
Approver for a production restore: Incident Commander and Database Owner  
Evidence owner: Security/Compliance

## Service objectives

The repository automation enforces a baseline recovery point objective (RPO) of 24 hours and recovery time objective (RTO) of 4 hours:

- `DR_RPO_TARGET_SECONDS=86400`
- `DR_RTO_TARGET_SECONDS=14400`

These are baseline logical-backup objectives, not a claim that the hosting platform already meets them. Production must schedule at least one successful encrypted backup inside every RPO window, copy it to immutable cross-account or cross-region storage, alert on missed jobs, and measure restores regularly. Use provider-managed point-in-time recovery with continuous WAL archival when the product requires an RPO below the backup interval.

The scheduled `PostgreSQL Restore Drill` GitHub workflow proves that the current migrations, seeds, backup archive, checksum, and restore path work together. It creates a database whose name is restricted to `cf_restore_drill_*`, verifies critical table row counts and migration state, records measured RPO/RTO, and drops the drill database. Run the drill at least quarterly in a production-like isolated account in addition to CI.

## Prerequisites

- PostgreSQL client tools (`psql`, `pg_dump`, and `pg_restore`) must be the same major version as the server or newer. Test version upgrades before changing the scheduled backup image.
- The backup principal needs read access to every application schema and sequence. The restore principal must own a newly created target database.
- Production encryption is mandatory by default. Prefer an `age` recipient backed by an offline-controlled identity, or use the hook interface to call the organization's KMS/HSM envelope-encryption client.
- Keep database URLs, age identities, and KMS credentials in the deployment secret manager. Never place them in workflow YAML, shell history, logs, or backup metadata.
- Store the archive, `.metadata`, and `.sha256` sidecars together. Apply object lock, retention controls, separate-account replication, and access logging at the storage layer.

## Create and retain a backup

Using `age` encryption:

```sh
export DATABASE_URL='postgres://backup-role@db.example:5432/complianceforge?sslmode=verify-full'
export POSTGRES_BACKUP_DIR=/var/lib/complianceforge-backups
export POSTGRES_BACKUP_AGE_RECIPIENT='age1...'
export POSTGRES_BACKUP_RETENTION_DAYS=30
./scripts/postgres-backup.sh
```

The final archive path is the only stdout value, which makes it safe to capture in a scheduler. Operational logs go to stderr. A backup is accepted only when the database migration state is clean and `pg_restore --list` can read the archive. Local artifacts are written with mode-restricting `umask 077`, checksum metadata, schema version, timestamps, and an atomic final rename. RPO age is conservatively measured from backup/snapshot start rather than dump completion; an unsuccessful run removes its incomplete local archive and sidecars.

For KMS or HSM encryption, set `POSTGRES_BACKUP_ENCRYPTION=hook` and `POSTGRES_BACKUP_ENCRYPT_HOOK` to an executable. The hook is invoked as:

```text
encrypt-hook <plaintext-custom-archive> <encrypted-output-path>
```

It must return zero and create a non-empty output. The plaintext temporary archive is removed after successful encryption. `POSTGRES_BACKUP_UPLOAD_HOOK`, when set, is invoked with the final archive, metadata sidecar, and checksum sidecar. The upload hook must return non-zero unless all objects have reached durable storage.

Unencrypted output is blocked unless an operator explicitly sets both `POSTGRES_BACKUP_ENCRYPTION=none` and `POSTGRES_BACKUP_REQUIRE_ENCRYPTION=false`. That exception is intended only for ephemeral restore drills.

## Restore drill

Use an isolated PostgreSQL instance or account. The drill user needs permission to create and drop only the named drill database.

```sh
export DRILL_SOURCE_DATABASE_URL='postgres://drill-role@db:5432/complianceforge?sslmode=verify-full'
export DRILL_ADMIN_DATABASE_URL='postgres://drill-admin@db:5432/postgres?sslmode=verify-full'
export DRILL_TARGET_DATABASE_NAME=cf_restore_drill_2026q3
export DRILL_TARGET_DATABASE_URL='postgres://drill-admin@db:5432/cf_restore_drill_2026q3?sslmode=verify-full'
export POSTGRES_RESTORE_DRILL_CONFIRM='CREATE_AND_DROP:cf_restore_drill_2026q3'
./scripts/postgres-restore-drill.sh
```

Archive the JSON evidence report with the change or incident record. Investigate a failed drill immediately; do not reset the objective clock by deleting failed evidence.

## Production recovery

1. Open an incident, record `DR_INCIDENT_STARTED_AT_EPOCH`, appoint the Incident Commander, and stop writes or fence the failed primary.
2. Select the newest independently replicated backup and its two sidecars. Verify object-store version, retention status, checksum, creation time, schema version, and encryption key availability.
3. Provision a **new, empty database** in an isolated recovery environment. Do not restore over the failed primary. Validate network policy, TLS verification, extensions, capacity, and PostgreSQL major-version compatibility.
4. Run the restore with a target-specific confirmation token:

```sh
export POSTGRES_BACKUP_FILE=/secure-recovery/complianceforge-postgres-....dump.age
export TARGET_DATABASE_URL='postgres://restore-owner@recovery-db:5432/complianceforge_recovered?sslmode=verify-full'
export POSTGRES_RESTORE_AGE_IDENTITY_FILE=/run/secrets/backup-age-identity
export POSTGRES_RESTORE_MODE=disaster-recovery
export POSTGRES_RESTORE_CONFIRM='RESTORE:complianceforge_recovered'
export DR_INCIDENT_STARTED_AT_EPOCH=1789320000
export DR_ENFORCE_OBJECTIVES=true
./scripts/postgres-restore.sh
```

5. The script verifies the encrypted-object checksum before any restore, rejects a non-empty target by default, runs `pg_restore` with `--exit-on-error` and a single transaction, verifies a clean migration version, and checks critical relations. If an approved emergency requires an existing target, set `POSTGRES_RESTORE_ALLOW_NONEMPTY=true` only after recording that destructive exception.
6. Run application-level smoke tests and tenant-isolation checks against the recovery database. Reconcile object storage and message delivery separately; this logical PostgreSQL backup does not contain S3 objects, Redis data, or RabbitMQ messages.
7. Obtain Incident Commander approval, update the application secret or database endpoint, deploy by immutable image digest, then monitor readiness, error rate, queue lag, and audit events before reopening writes.
8. Record actual RPO and RTO, backup object version, image digest, database versions, approvers, validation evidence, and any objective breach in the incident record.

`DR_ENFORCE_OBJECTIVES=false` permits a stale backup only as an explicit incident decision. It produces a warning; it does not make the objective compliant.

## Hooks and operational controls

- `POSTGRES_RESTORE_PRE_HOOK` can fence application traffic or record a change ticket immediately before `pg_restore`.
- `POSTGRES_RESTORE_POST_HOOK` can run organization-specific consistency checks. It runs only after built-in verification succeeds.
- Hooks are executable paths, not shell command strings. The scripts never evaluate hook text.
- Alert on backup age, backup/upload failures, checksum mismatch, dirty migration state, restore-drill failure, and RPO/RTO breach.
- Test key recovery independently. A valid encrypted archive without a recoverable key is a failed backup.
- Rotate backup credentials and encryption recipients under dual control, retaining old decrypt-only keys for the full backup retention period.

## Known boundary

These scripts provide portable logical backup and verified recovery. Infrastructure still must supply scheduling, secret injection, KMS/HSM or `age` identities, immutable replicated object storage, monitoring/alerting, managed snapshots or WAL/PITR, and a production-like drill environment. Those external controls are required before the stated RPO/RTO can be treated as an SLO.
