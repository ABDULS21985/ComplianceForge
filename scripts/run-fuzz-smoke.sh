#!/bin/sh

set -eu

fuzz_time=${FUZZ_TIME:-2s}

run_fuzz() {
  package=$1
  target=$2
  go test "$package" -run='^$' -fuzz="^${target}$" -fuzztime="$fuzz_time"
}

run_fuzz ./internal/pkg/queue FuzzEnvelopeDecodeValidateRoundTrip
run_fuzz ./internal/pkg/safehttp FuzzValidateURL
run_fuzz ./internal/pkg/storage FuzzLocalStoragePathContainment
run_fuzz ./internal/handler FuzzAttachmentResponseHeaders
run_fuzz ./internal/handler FuzzStylesheetResponseRejectsHTMLSentinels
run_fuzz ./internal/service FuzzDirectoryCSVParser
run_fuzz ./internal/service FuzzDynamicDirectoryGroupRule
run_fuzz ./internal/service FuzzFindingAndIncidentStateMachines
run_fuzz ./internal/service FuzzPaginationAndAuditDateParsers
run_fuzz ./internal/router FuzzProtectedRoutePermissionMapping
