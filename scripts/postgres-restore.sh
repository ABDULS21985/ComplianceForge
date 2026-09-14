#!/bin/sh

set -eu

backup_file=${POSTGRES_BACKUP_FILE:-}
target_database_url=${TARGET_DATABASE_URL:-}
restore_mode=${POSTGRES_RESTORE_MODE:-drill}
require_metadata=${POSTGRES_RESTORE_REQUIRE_METADATA:-true}
allow_nonempty=${POSTGRES_RESTORE_ALLOW_NONEMPTY:-false}
enforce_objectives=${DR_ENFORCE_OBJECTIVES:-true}
rpo_target_seconds=${DR_RPO_TARGET_SECONDS:-86400}
rto_target_seconds=${DR_RTO_TARGET_SECONDS:-14400}
incident_started_epoch=${DR_INCIDENT_STARTED_AT_EPOCH:-$(date +%s)}

log() {
  printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

fail() {
  log "restore failed: $*"
  exit 1
}

is_uint() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

metadata_value() {
  key=$1
  file=$2
  awk -F= -v wanted="$key" '$1 == wanted { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

[ -n "$backup_file" ] || fail 'POSTGRES_BACKUP_FILE is required'
[ -f "$backup_file" ] || fail "backup file not found: $backup_file"
[ -n "$target_database_url" ] || fail 'TARGET_DATABASE_URL is required'
case "$restore_mode" in drill|disaster-recovery) ;; *) fail 'POSTGRES_RESTORE_MODE must be drill or disaster-recovery' ;; esac
case "$require_metadata" in true|false) ;; *) fail 'POSTGRES_RESTORE_REQUIRE_METADATA must be true or false' ;; esac
case "$allow_nonempty" in true|false) ;; *) fail 'POSTGRES_RESTORE_ALLOW_NONEMPTY must be true or false' ;; esac
case "$enforce_objectives" in true|false) ;; *) fail 'DR_ENFORCE_OBJECTIVES must be true or false' ;; esac
is_uint "$rpo_target_seconds" || fail 'DR_RPO_TARGET_SECONDS must be an integer'
is_uint "$rto_target_seconds" || fail 'DR_RTO_TARGET_SECONDS must be an integer'
is_uint "$incident_started_epoch" || fail 'DR_INCIDENT_STARTED_AT_EPOCH must be an integer'

for command_name in pg_restore psql; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done

metadata_file="${backup_file}.metadata"
checksum_file="${backup_file}.sha256"
if [ "$require_metadata" = true ]; then
  [ -f "$metadata_file" ] || fail "metadata sidecar not found: $metadata_file"
  [ -f "$checksum_file" ] || fail "checksum sidecar not found: $checksum_file"
fi

expected_checksum=''
backup_created_epoch=''
expected_schema_version=''
encryption_mode=''
if [ -f "$metadata_file" ]; then
  expected_checksum=$(metadata_value SHA256 "$metadata_file")
  backup_created_epoch=$(metadata_value CREATED_AT_EPOCH "$metadata_file")
  expected_schema_version=$(metadata_value SCHEMA_VERSION "$metadata_file")
  encryption_mode=$(metadata_value ENCRYPTION "$metadata_file")
fi
if [ -z "$expected_checksum" ] && [ -f "$checksum_file" ]; then
  expected_checksum=$(awk 'NR == 1 { print $1 }' "$checksum_file")
fi
[ -n "$expected_checksum" ] || fail 'no trusted checksum was supplied'
actual_checksum=$(sha256_file "$backup_file")
[ "$actual_checksum" = "$expected_checksum" ] || fail 'backup checksum does not match its metadata'

now_epoch=$(date +%s)
if is_uint "$backup_created_epoch" && [ "$backup_created_epoch" -le "$now_epoch" ]; then
  rpo_seconds=$((now_epoch - backup_created_epoch))
else
  [ "$enforce_objectives" = false ] || fail 'backup metadata has no valid creation epoch for the RPO check'
  rpo_seconds=-1
fi
if [ "$rpo_seconds" -ge 0 ] && [ "$rpo_seconds" -gt "$rpo_target_seconds" ]; then
  if [ "$enforce_objectives" = true ]; then
    fail "RPO objective exceeded: backup age ${rpo_seconds}s > ${rpo_target_seconds}s"
  fi
  log "WARNING: RPO objective exceeded: backup age ${rpo_seconds}s > ${rpo_target_seconds}s"
fi

export PGAPPNAME=complianceforge-restore
export PGCONNECT_TIMEOUT="${PGCONNECT_TIMEOUT:-15}"
target_database=$(psql "$target_database_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command 'SELECT current_database()')
[ -n "$target_database" ] || fail 'could not resolve the target database name'
expected_confirmation="RESTORE:$target_database"
[ "${POSTGRES_RESTORE_CONFIRM:-}" = "$expected_confirmation" ] || \
  fail "set POSTGRES_RESTORE_CONFIRM=$expected_confirmation to authorize this target"

existing_tables=$(psql "$target_database_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 \
  --command "SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog', 'information_schema')")
if [ "$existing_tables" -gt 0 ] && [ "$allow_nonempty" != true ]; then
  fail "target contains $existing_tables application tables; restore into a new database or explicitly set POSTGRES_RESTORE_ALLOW_NONEMPTY=true"
fi

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/complianceforge-restore.XXXXXX")
archive="$work_dir/restore.dump"
cleanup() {
  if [ -d "$work_dir" ]; then
    find "$work_dir" -mindepth 1 -delete
    rmdir "$work_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

case "$encryption_mode" in
  age)
    command -v age >/dev/null 2>&1 || fail 'age is required to decrypt this backup'
    identity_file=${POSTGRES_RESTORE_AGE_IDENTITY_FILE:-}
    [ -f "$identity_file" ] || fail 'POSTGRES_RESTORE_AGE_IDENTITY_FILE must reference an age identity file'
    age --decrypt --identity "$identity_file" --output "$archive" "$backup_file"
    ;;
  hook)
    decryption_hook=${POSTGRES_RESTORE_DECRYPT_HOOK:-}
    [ -n "$decryption_hook" ] && [ -x "$decryption_hook" ] || fail 'POSTGRES_RESTORE_DECRYPT_HOOK must be an executable path'
    "$decryption_hook" "$backup_file" "$archive" >&2
    ;;
  none|'')
    [ "$require_metadata" = false ] || [ "$encryption_mode" = none ] || fail 'backup metadata has no supported ENCRYPTION value'
    cp "$backup_file" "$archive"
    ;;
  *) fail "unsupported backup encryption mode: $encryption_mode" ;;
esac
[ -s "$archive" ] || fail 'decryption produced an empty archive'
pg_restore --list "$archive" >/dev/null

if [ -n "${POSTGRES_RESTORE_PRE_HOOK:-}" ]; then
  [ -x "$POSTGRES_RESTORE_PRE_HOOK" ] || fail 'POSTGRES_RESTORE_PRE_HOOK must be an executable path'
  "$POSTGRES_RESTORE_PRE_HOOK" >&2
fi

log "restoring schema version ${expected_schema_version:-unknown} into $target_database ($restore_mode)"
pg_restore \
  --clean \
  --if-exists \
  --exit-on-error \
  --single-transaction \
  --no-owner \
  --no-privileges \
  --dbname "$target_database_url" \
  "$archive"

schema_state=$(psql "$target_database_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 \
  --command "SELECT version::text || ':' || dirty::text FROM schema_migrations LIMIT 1")
case "$schema_state" in
  *:false) ;;
  *) fail "restored schema_migrations is absent, empty, or dirty (state: ${schema_state:-empty})" ;;
esac
actual_schema_version=${schema_state%:*}
if [ -n "$expected_schema_version" ] && [ "$actual_schema_version" != "$expected_schema_version" ]; then
  fail "restored schema version $actual_schema_version does not match backup version $expected_schema_version"
fi

required_relations=$(psql "$target_database_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command \
  "SELECT count(*) FROM unnest(ARRAY['organizations','users','audit_logs','bootstrap_seed_history']) AS name WHERE to_regclass('public.' || name) IS NOT NULL")
[ "$required_relations" -eq 4 ] || fail "only $required_relations of 4 critical relations were restored"

if [ -n "${POSTGRES_RESTORE_POST_HOOK:-}" ]; then
  [ -x "$POSTGRES_RESTORE_POST_HOOK" ] || fail 'POSTGRES_RESTORE_POST_HOOK must be an executable path'
  "$POSTGRES_RESTORE_POST_HOOK" >&2
fi

finished_epoch=$(date +%s)
[ "$incident_started_epoch" -le "$finished_epoch" ] || fail 'DR_INCIDENT_STARTED_AT_EPOCH is in the future'
rto_seconds=$((finished_epoch - incident_started_epoch))
if [ "$rto_seconds" -gt "$rto_target_seconds" ]; then
  if [ "$enforce_objectives" = true ]; then
    fail "RTO objective exceeded: ${rto_seconds}s > ${rto_target_seconds}s"
  fi
  log "WARNING: RTO objective exceeded: ${rto_seconds}s > ${rto_target_seconds}s"
fi

log "restore verified: database=$target_database schema=$actual_schema_version rpo=${rpo_seconds}s rto=${rto_seconds}s"
printf 'RESTORED_DATABASE=%s\n' "$target_database"
printf 'RESTORED_SCHEMA_VERSION=%s\n' "$actual_schema_version"
printf 'RESTORE_RPO_SECONDS=%s\n' "$rpo_seconds"
printf 'RESTORE_RTO_SECONDS=%s\n' "$rto_seconds"
