#!/bin/sh

set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
threat_model="$repo_root/docs/security/threat-model.md"
risk_register="$repo_root/docs/security/architecture-risk-register.md"
sdlc="$repo_root/docs/runbooks/secure-sdlc.md"
pull_request_template="$repo_root/.github/pull_request_template.md"

fail() {
  printf 'security governance validation failed: %s\n' "$*" >&2
  exit 1
}

for required_file in "$threat_model" "$risk_register" "$sdlc" "$pull_request_template"; do
  [ -s "$required_file" ] || fail "missing or empty required file: $required_file"
done

for heading in \
  '## Scope and security objectives' \
  '## Assets and trust boundaries' \
  '## Threat scenarios and required verification' \
  '## Data-flow review rules'; do
  grep -Fqx "$heading" "$threat_model" || fail "threat model is missing heading: $heading"
done

threat_ids=$(sed -n 's/^| \(TM-[0-9][0-9]\) |.*/\1/p' "$threat_model")
[ "$(printf '%s\n' "$threat_ids" | sed '/^$/d' | wc -l | tr -d ' ')" -ge 1 ] || fail "threat model has no scenario rows"
duplicate_threat_ids=$(printf '%s\n' "$threat_ids" | sort | uniq -d)
[ -z "$duplicate_threat_ids" ] || fail "duplicate threat IDs: $duplicate_threat_ids"

for threat_id in $threat_ids; do
  grep -Fq "$threat_id" "$risk_register" || fail "$threat_id is not linked from the architecture risk register"
done

risk_ids=$(sed -n 's/^| \(AR-[0-9][0-9][0-9]\) |.*/\1/p' "$risk_register")
[ "$(printf '%s\n' "$risk_ids" | sed '/^$/d' | wc -l | tr -d ' ')" -ge 1 ] || fail "architecture risk register has no risk rows"
duplicate_risk_ids=$(printf '%s\n' "$risk_ids" | sort | uniq -d)
[ -z "$duplicate_risk_ids" ] || fail "duplicate architecture risk IDs: $duplicate_risk_ids"

if ! awk -F '|' '
  /^\| AR-[0-9][0-9][0-9] / {
    status=$9
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", status)
    if (status != "open" && status != "mitigating" && status != "accepted" && status != "closed") {
      print "invalid status for " $2 ": " status > "/dev/stderr"
      invalid=1
    }
    owner=$7
    target=$8
    gsub(/[[:space:]]/, "", owner)
    gsub(/[[:space:]]/, "", target)
    if (owner == "" || target !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/) {
      print "missing owner or ISO target date for " $2 > "/dev/stderr"
      invalid=1
    }
  }
  END { exit invalid }
' "$risk_register"; then
  fail "architecture risk rows must have controlled status, owner, and target review date"
fi

grep -Fq 'Change class: `standard` / `security-sensitive` / `architecture`' "$pull_request_template" ||
  fail "pull-request template is missing security change classification"
grep -Fq 'Affected threat-model IDs' "$pull_request_template" ||
  fail "pull-request template is missing threat-model traceability"
grep -Fq 'An author cannot provide the sole security approval' "$sdlc" ||
  fail "secure SDLC is missing independent-review policy"
grep -Fq 'independent penetration test' "$sdlc" ||
  fail "secure SDLC is missing penetration-test cadence"

printf 'Validated %s threat scenarios and %s architecture risks with SDLC review gates.\n' \
  "$(printf '%s\n' "$threat_ids" | wc -l | tr -d ' ')" \
  "$(printf '%s\n' "$risk_ids" | wc -l | tr -d ' ')"
