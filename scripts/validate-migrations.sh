#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
migrations_dir=${MIGRATIONS_DIR:-$repo_root/sql/migrations}
seeds_dir=${SEEDS_DIR:-$repo_root/sql/seeds}
manifest=${SEED_MANIFEST:-$seeds_dir/manifest.txt}

fail() {
  printf 'migration validation failed: %s\n' "$*" >&2
  exit 1
}

[ -d "$migrations_dir" ] || fail "directory not found: $migrations_dir"
[ -d "$seeds_dir" ] || fail "seed directory not found: $seeds_dir"
[ -f "$manifest" ] || fail "seed manifest not found: $manifest"

expected=1
seen_versions=' '
up_count=0

for up_file in "$migrations_dir"/*.up.sql; do
  [ -f "$up_file" ] || fail "no up migrations found in $migrations_dir"

  filename=$(basename -- "$up_file")
  stem=${filename%.up.sql}
  version=${stem%%_*}

  printf '%s\n' "$version" | grep -Eq '^[0-9]{6}$' || fail "invalid migration filename: $filename"
  case "$stem" in
    "${version}_"*) ;;
    *) fail "migration must include a descriptive suffix: $filename" ;;
  esac

  expected_version=$(printf '%06d' "$expected")
  [ "$version" = "$expected_version" ] || fail "expected version $expected_version, found $version ($filename)"
  case "$seen_versions" in
    *" $version "*) fail "duplicate migration version: $version" ;;
  esac
  seen_versions="$seen_versions$version "

  down_file="$migrations_dir/$stem.down.sql"
  [ -f "$down_file" ] || fail "missing down migration for $filename"
  [ -s "$up_file" ] || fail "empty migration: $filename"
  [ -s "$down_file" ] || fail "empty migration: $(basename -- "$down_file")"

  up_begin=$(grep -Eic '^[[:space:]]*BEGIN[[:space:]]*;' "$up_file" || true)
  up_commit=$(grep -Eic '^[[:space:]]*COMMIT[[:space:]]*;' "$up_file" || true)
  down_begin=$(grep -Eic '^[[:space:]]*BEGIN[[:space:]]*;' "$down_file" || true)
  down_commit=$(grep -Eic '^[[:space:]]*COMMIT[[:space:]]*;' "$down_file" || true)
  [ "$up_begin" -eq "$up_commit" ] || fail "unbalanced top-level BEGIN/COMMIT in $filename"
  [ "$down_begin" -eq "$down_commit" ] || fail "unbalanced top-level BEGIN/COMMIT in $(basename -- "$down_file")"

  expected=$((expected + 1))
  up_count=$((up_count + 1))
done

for down_file in "$migrations_dir"/*.down.sql; do
  [ -f "$down_file" ] || fail "no down migrations found in $migrations_dir"
  stem=$(basename -- "$down_file" .down.sql)
  [ -f "$migrations_dir/$stem.up.sql" ] || fail "orphan down migration: $(basename -- "$down_file")"
done

seed_count=0
seen_seeds='|'
while IFS= read -r raw_entry || [ -n "$raw_entry" ]; do
  entry=$(printf '%s' "$raw_entry" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
  case "$entry" in
    ''|'#'*) continue ;;
  esac
  case "$entry" in
    /*|*'..'*|*/*) fail "unsafe seed manifest entry: $entry" ;;
    *.sql) ;;
    *) fail "seed manifest entry must end in .sql: $entry" ;;
  esac
  case "$seen_seeds" in
    *"|$entry|"*) fail "duplicate seed manifest entry: $entry" ;;
  esac
  [ -s "$seeds_dir/$entry" ] || fail "missing or empty seed file: $entry"
  seen_seeds="$seen_seeds$entry|"
  seed_count=$((seed_count + 1))
done < "$manifest"

[ "$seed_count" -gt 0 ] || fail "seed manifest contains no seed files"

for seed_file in "$seeds_dir"/*.sql; do
  [ -f "$seed_file" ] || continue
  seed_name=$(basename -- "$seed_file")
  case "$seen_seeds" in
    *"|$seed_name|"*) ;;
    *) fail "seed file is not ordered in manifest.txt: $seed_name" ;;
  esac
done

printf 'Validated %s reversible, contiguous migration pairs and %s ordered seed files.\n' "$up_count" "$seed_count"
