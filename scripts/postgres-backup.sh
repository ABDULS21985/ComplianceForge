#!/bin/sh

set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)

database_url=${DATABASE_URL:-}
backup_dir=${POSTGRES_BACKUP_DIR:-$repo_root/backups/postgresql}
prefix=${POSTGRES_BACKUP_PREFIX:-complianceforge-postgres}
retention_days=${POSTGRES_BACKUP_RETENTION_DAYS:-30}
encryption_mode=${POSTGRES_BACKUP_ENCRYPTION:-age}
require_encryption=${POSTGRES_BACKUP_REQUIRE_ENCRYPTION:-true}
lock_stale_seconds=${POSTGRES_BACKUP_LOCK_STALE_SECONDS:-21600}

log() {
  printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

fail() {
  log "backup failed: $*"
  exit 1
}

is_uint() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

[ -n "$database_url" ] || fail 'DATABASE_URL is required'
case "$prefix" in
  ''|*[!A-Za-z0-9_-]*) fail 'POSTGRES_BACKUP_PREFIX may contain only letters, digits, underscore, and hyphen' ;;
esac
is_uint "$retention_days" || fail 'POSTGRES_BACKUP_RETENTION_DAYS must be an integer'
is_uint "$lock_stale_seconds" || fail 'POSTGRES_BACKUP_LOCK_STALE_SECONDS must be an integer'
[ "$retention_days" -ge 1 ] && [ "$retention_days" -le 3650 ] || fail 'retention must be between 1 and 3650 days'
[ "$lock_stale_seconds" -ge 300 ] || fail 'backup lock stale threshold must be at least 300 seconds'
case "$require_encryption" in true|false) ;; *) fail 'POSTGRES_BACKUP_REQUIRE_ENCRYPTION must be true or false' ;; esac
case "$encryption_mode" in
  age|hook) ;;
  none)
    [ "$require_encryption" = false ] || fail 'unencrypted backups require POSTGRES_BACKUP_REQUIRE_ENCRYPTION=false'
    ;;
  *) fail 'POSTGRES_BACKUP_ENCRYPTION must be age, hook, or none' ;;
esac

for command_name in pg_dump pg_restore psql; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done

export PGCONNECT_TIMEOUT=${PGCONNECT_TIMEOUT:-15}

umask 077
mkdir -p "$backup_dir"
backup_dir=$(CDPATH='' cd -- "$backup_dir" && pwd)
case "$backup_dir" in
  /|"${HOME:-__unset__}") fail "unsafe backup directory: $backup_dir" ;;
esac

lock_dir="$backup_dir/.backup.lock"
lock_acquired=false
raw_archive=''
encrypted_tmp=''
cleanup() {
  [ -z "$raw_archive" ] || [ ! -f "$raw_archive" ] || rm -f -- "$raw_archive"
  [ -z "$encrypted_tmp" ] || [ ! -f "$encrypted_tmp" ] || rm -f -- "$encrypted_tmp"
  if [ "$lock_acquired" = true ] && [ -d "$lock_dir" ]; then
    find "$lock_dir" -mindepth 1 -delete
    rmdir "$lock_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

if ! mkdir "$lock_dir" 2>/dev/null; then
  lock_epoch=0
  [ ! -f "$lock_dir/started_at_epoch" ] || lock_epoch=$(sed -n '1p' "$lock_dir/started_at_epoch")
  now_epoch=$(date +%s)
  if is_uint "$lock_epoch" && [ "$lock_epoch" -gt 0 ] && [ $((now_epoch - lock_epoch)) -gt "$lock_stale_seconds" ]; then
    log "recovering stale backup lock older than ${lock_stale_seconds}s"
    find "$lock_dir" -mindepth 1 -delete
    rmdir "$lock_dir"
    mkdir "$lock_dir" || fail 'another backup acquired the recovered lock'
  else
    fail "another backup is running (lock: $lock_dir)"
  fi
fi
lock_acquired=true
backup_started_epoch=$(date +%s)
printf '%s\n' "$backup_started_epoch" > "$lock_dir/started_at_epoch"
printf '%s\n' "$$" > "$lock_dir/pid"

timestamp=$(date -u '+%Y%m%dT%H%M%SZ')
base_name="${prefix}-${timestamp}-$$"
raw_archive="$backup_dir/.${base_name}.dump.tmp"

schema_state=$(psql "$database_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 \
  --command "SELECT version::text || ':' || dirty::text FROM schema_migrations LIMIT 1")
case "$schema_state" in
  *:false) ;;
  *) fail "schema_migrations is absent, empty, or dirty (state: ${schema_state:-empty})" ;;
esac
schema_version=${schema_state%:*}

log "creating PostgreSQL custom-format archive at schema version $schema_version"
PGAPPNAME=complianceforge-backup pg_dump \
  --dbname "$database_url" \
  --format custom \
  --compress 9 \
  --no-owner \
  --no-privileges \
  --serializable-deferrable \
  --file "$raw_archive"
pg_restore --list "$raw_archive" >/dev/null

case "$encryption_mode" in
  age)
    command -v age >/dev/null 2>&1 || fail 'age is required for POSTGRES_BACKUP_ENCRYPTION=age'
    age_recipient=${POSTGRES_BACKUP_AGE_RECIPIENT:-}
    [ -n "$age_recipient" ] || fail 'POSTGRES_BACKUP_AGE_RECIPIENT is required for age encryption'
    final_file="$backup_dir/${base_name}.dump.age"
    encrypted_tmp="$backup_dir/.${base_name}.dump.age.tmp"
    age --encrypt --recipient "$age_recipient" --output "$encrypted_tmp" "$raw_archive"
    [ -s "$encrypted_tmp" ] || fail 'age produced an empty backup'
    mv "$encrypted_tmp" "$final_file"
    encrypted_tmp=''
    rm -f -- "$raw_archive"
    raw_archive=''
    ;;
  hook)
    encryption_hook=${POSTGRES_BACKUP_ENCRYPT_HOOK:-}
    [ -n "$encryption_hook" ] && [ -x "$encryption_hook" ] || fail 'POSTGRES_BACKUP_ENCRYPT_HOOK must be an executable path'
    final_file="$backup_dir/${base_name}.dump.enc"
    encrypted_tmp="$backup_dir/.${base_name}.dump.enc.tmp"
    "$encryption_hook" "$raw_archive" "$encrypted_tmp" >&2
    [ -s "$encrypted_tmp" ] || fail 'encryption hook produced an empty backup'
    mv "$encrypted_tmp" "$final_file"
    encrypted_tmp=''
    rm -f -- "$raw_archive"
    raw_archive=''
    ;;
  none)
    final_file="$backup_dir/${base_name}.dump"
    mv "$raw_archive" "$final_file"
    raw_archive=''
    ;;
esac

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

file_size() {
  if stat -c '%s' "$1" >/dev/null 2>&1; then
    stat -c '%s' "$1"
  else
    stat -f '%z' "$1"
  fi
}

checksum=$(sha256_file "$final_file")
size_bytes=$(file_size "$final_file")
created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
created_at_epoch=$(date +%s)
metadata_file="${final_file}.metadata"
checksum_file="${final_file}.sha256"

{
  printf 'FORMAT_VERSION=1\n'
  printf 'CREATED_AT=%s\n' "$created_at"
  printf 'CREATED_AT_EPOCH=%s\n' "$created_at_epoch"
  printf 'SCHEMA_VERSION=%s\n' "$schema_version"
  printf 'ENCRYPTION=%s\n' "$encryption_mode"
  printf 'SIZE_BYTES=%s\n' "$size_bytes"
  printf 'SHA256=%s\n' "$checksum"
  printf 'PG_DUMP_VERSION=%s\n' "$(pg_dump --version | tr '\n' ' ')"
} > "${metadata_file}.tmp"
printf '%s  %s\n' "$checksum" "$(basename -- "$final_file")" > "${checksum_file}.tmp"
mv "${metadata_file}.tmp" "$metadata_file"
mv "${checksum_file}.tmp" "$checksum_file"

if [ -n "${POSTGRES_BACKUP_UPLOAD_HOOK:-}" ]; then
  [ -x "$POSTGRES_BACKUP_UPLOAD_HOOK" ] || fail 'POSTGRES_BACKUP_UPLOAD_HOOK must be an executable path'
  "$POSTGRES_BACKUP_UPLOAD_HOOK" "$final_file" "$metadata_file" "$checksum_file" >&2
fi

find "$backup_dir" -maxdepth 1 -type f -name "${prefix}-*" -mtime "+$retention_days" -print |
while IFS= read -r expired_file; do
  [ -n "$expired_file" ] || continue
  rm -f -- "$expired_file"
  log "expired local backup artifact: $(basename -- "$expired_file")"
done

log "backup complete: $(basename -- "$final_file") (${size_bytes} bytes, sha256 ${checksum})"
printf '%s\n' "$final_file"
