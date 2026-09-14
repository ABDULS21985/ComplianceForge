#!/bin/sh

set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)

source_url=${DRILL_SOURCE_DATABASE_URL:-}
admin_url=${DRILL_ADMIN_DATABASE_URL:-}
target_url=${DRILL_TARGET_DATABASE_URL:-}
target_name=${DRILL_TARGET_DATABASE_NAME:-}
confirm=${POSTGRES_RESTORE_DRILL_CONFIRM:-}
rpo_target=${DR_RPO_TARGET_SECONDS:-86400}
rto_target=${DR_RTO_TARGET_SECONDS:-14400}
report_path=${DRILL_REPORT_PATH:-$repo_root/artifacts/postgres-restore-drill.json}

log() {
  printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

fail() {
  log "restore drill failed: $*"
  exit 1
}

[ -n "$source_url" ] || fail 'DRILL_SOURCE_DATABASE_URL is required'
[ -n "$admin_url" ] || fail 'DRILL_ADMIN_DATABASE_URL is required'
[ -n "$target_url" ] || fail 'DRILL_TARGET_DATABASE_URL is required'
case "$target_name" in cf_restore_drill_*) ;; *) fail 'DRILL_TARGET_DATABASE_NAME must start with cf_restore_drill_' ;; esac
case "$target_name" in *[!A-Za-z0-9_]*) fail 'DRILL_TARGET_DATABASE_NAME may contain only letters, digits, and underscore' ;; esac
[ "$confirm" = "CREATE_AND_DROP:$target_name" ] || fail "set POSTGRES_RESTORE_DRILL_CONFIRM=CREATE_AND_DROP:$target_name"

for command_name in pg_dump pg_restore psql; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done

source_name=$(psql "$source_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command 'SELECT current_database()')
case "$source_name" in
  ''|*[!A-Za-z0-9_.-]*) fail 'source database name contains characters that cannot be safely recorded in drill evidence' ;;
esac
[ "$source_name" != "$target_name" ] || fail 'drill target must not be the source database'

drill_started_epoch=$(date +%s)
drill_started_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/complianceforge-drill.XXXXXX")
target_created=false
cleanup() {
  if [ "$target_created" = true ]; then
    psql "$admin_url" --no-psqlrc --set ON_ERROR_STOP=1 \
      --command "DROP DATABASE IF EXISTS \"$target_name\" WITH (FORCE)" >/dev/null || true
  fi
  if [ -d "$work_dir" ]; then
    find "$work_dir" -mindepth 1 -delete
    rmdir "$work_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

log "creating isolated restore-drill database $target_name"
psql "$admin_url" --no-psqlrc --set ON_ERROR_STOP=1 \
  --command "DROP DATABASE IF EXISTS \"$target_name\" WITH (FORCE)" >/dev/null
psql "$admin_url" --no-psqlrc --set ON_ERROR_STOP=1 \
  --command "CREATE DATABASE \"$target_name\" TEMPLATE template0" >/dev/null
target_created=true

resolved_target_name=$(psql "$target_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command 'SELECT current_database()')
[ "$resolved_target_name" = "$target_name" ] || fail "DRILL_TARGET_DATABASE_URL connects to $resolved_target_name, expected $target_name"

source_schema_state=$(psql "$source_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 \
  --command "SELECT version::text || ':' || dirty::text FROM schema_migrations LIMIT 1")
case "$source_schema_state" in *:false) ;; *) fail "source migration state is not clean: ${source_schema_state:-empty}" ;; esac
source_schema_version=${source_schema_state%:*}
case "$source_schema_version" in ''|*[!0-9]*) fail "invalid source schema version: $source_schema_version" ;; esac

count_query="SELECT (SELECT count(*) FROM organizations)::text || ':' || (SELECT count(*) FROM users)::text || ':' || (SELECT count(*) FROM audit_logs)::text || ':' || (SELECT count(*) FROM bootstrap_seed_history)::text"
source_counts=$(psql "$source_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command "$count_query")

backup_file=$(
  DATABASE_URL="$source_url" \
  POSTGRES_BACKUP_DIR="$work_dir/backups" \
  POSTGRES_BACKUP_PREFIX=restore-drill \
  POSTGRES_BACKUP_RETENTION_DAYS=1 \
  POSTGRES_BACKUP_ENCRYPTION=none \
  POSTGRES_BACKUP_REQUIRE_ENCRYPTION=false \
  "$script_dir/postgres-backup.sh"
)
[ -f "$backup_file" ] || fail 'backup phase did not produce an archive'

restore_output="$work_dir/restore-output.txt"
POSTGRES_BACKUP_FILE="$backup_file" \
TARGET_DATABASE_URL="$target_url" \
POSTGRES_RESTORE_MODE=drill \
POSTGRES_RESTORE_CONFIRM="RESTORE:$target_name" \
DR_ENFORCE_OBJECTIVES=true \
DR_RPO_TARGET_SECONDS="$rpo_target" \
DR_RTO_TARGET_SECONDS="$rto_target" \
DR_INCIDENT_STARTED_AT_EPOCH="$drill_started_epoch" \
"$script_dir/postgres-restore.sh" > "$restore_output"

target_schema_state=$(psql "$target_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 \
  --command "SELECT version::text || ':' || dirty::text FROM schema_migrations LIMIT 1")
[ "$target_schema_state" = "$source_schema_state" ] || fail "migration state differs: source=$source_schema_state target=$target_schema_state"
target_counts=$(psql "$target_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command "$count_query")
[ "$target_counts" = "$source_counts" ] || fail "critical row counts differ: source=$source_counts target=$target_counts"

rpo_seconds=$(sed -n 's/^RESTORE_RPO_SECONDS=//p' "$restore_output")
rto_seconds=$(sed -n 's/^RESTORE_RTO_SECONDS=//p' "$restore_output")
case "$rpo_seconds" in ''|*[!0-9]*) fail 'restore did not emit a valid RPO measurement' ;; esac
case "$rto_seconds" in ''|*[!0-9]*) fail 'restore did not emit a valid RTO measurement' ;; esac

report_dir=$(dirname -- "$report_path")
mkdir -p "$report_dir"
report_tmp="${report_path}.tmp"
finished_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
cat > "$report_tmp" <<EOF
{
  "status": "passed",
  "started_at": "$drill_started_at",
  "finished_at": "$finished_at",
  "source_database": "$source_name",
  "ephemeral_target_database": "$target_name",
  "schema_version": $source_schema_version,
  "critical_row_counts": "$source_counts",
  "measured_rpo_seconds": $rpo_seconds,
  "rpo_target_seconds": $rpo_target,
  "measured_rto_seconds": $rto_seconds,
  "rto_target_seconds": $rto_target
}
EOF
mv "$report_tmp" "$report_path"

log "restore drill passed: schema=$source_schema_version counts=$source_counts rpo=${rpo_seconds}s rto=${rto_seconds}s"
printf '%s\n' "$report_path"
