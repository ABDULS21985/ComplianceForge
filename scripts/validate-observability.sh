#!/bin/sh

set -eu

repo_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
config_dir="$repo_dir/deployments/observability"
validation_dir=$(mktemp -d "${TMPDIR:-/tmp}/complianceforge-observability.XXXXXX")
cleanup() {
  if [ -d "$validation_dir" ]; then
    find "$validation_dir" -mindepth 1 -delete
    rmdir "$validation_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

printf '%s\n' validation-only > "$validation_dir/metrics_token"

jq -e . "$config_dir/grafana/dashboards/complianceforge-overview.json" >/dev/null

docker run --rm --entrypoint /bin/promtool \
  -v "$config_dir/prometheus:/etc/prometheus:ro" \
  -v "$validation_dir/metrics_token:/run/secrets/metrics_token:ro" \
  "prom/prometheus:v3.13.3@sha256:6976aa8a60fec930796ce5772b8d12da7a318a5daa8d40d69c5c7819a05eeed7" \
  check config /etc/prometheus/prometheus.yml

docker run --rm --entrypoint /bin/promtool \
  -v "$config_dir/prometheus:/etc/prometheus:ro" \
  "prom/prometheus:v3.13.3@sha256:6976aa8a60fec930796ce5772b8d12da7a318a5daa8d40d69c5c7819a05eeed7" \
  check config /etc/prometheus/prometheus.dev.yml

docker run --rm \
  -v "$config_dir/otel-collector.dev.yml:/etc/otelcol/config.yml:ro" \
  "otel/opentelemetry-collector-contrib:0.160.0@sha256:799dc6cf12c96192af37b5bdba804da8c10b3bc563b43cb90c3f3c58d9572ad6" \
  validate --config=/etc/otelcol/config.yml

docker run --rm \
  -e OTEL_TRACE_UPSTREAM_ENDPOINT=https://traces.example.invalid \
  -e OTEL_TRACE_UPSTREAM_AUTHORIZATION=validation-only \
  -v "$config_dir/otel-collector.yml:/etc/otelcol/config.yml:ro" \
  "otel/opentelemetry-collector-contrib:0.160.0@sha256:799dc6cf12c96192af37b5bdba804da8c10b3bc563b43cb90c3f3c58d9572ad6" \
  validate --config=/etc/otelcol/config.yml

printf 'Validated Prometheus rules/configuration, OpenTelemetry collectors, and Grafana dashboard JSON.\n'
