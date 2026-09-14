# Production observability and SRE runbook

## Operating contract

ComplianceForge exposes application metrics on a dedicated internal listener, never on the public API router. The API and worker default to `127.0.0.1:9091`; container deployments bind `0.0.0.0:9091` only so the isolated `telemetry` network can scrape them. Production requires `OBSERVABILITY_METRICS_TOKEN_FILE`, mounted as the same Docker secret into the API, worker, and Prometheus. Do not route port 9091 through nginx, a public load balancer, or a public Kubernetes Service.

The internal listener provides:

- `GET /metrics`, protected by a constant-time bearer-token check when a token file is configured.
- `GET /health/live`, which only proves that the process and internal HTTP listener are responsive.
- `GET /health/ready`, which returns 503 while draining or when a required PostgreSQL, Redis, or RabbitMQ check fails.

The public API retains `/health/live` and `/health/ready` for orchestration. Its readiness includes PostgreSQL and Redis. Worker readiness includes PostgreSQL and RabbitMQ. Liveness intentionally does not depend on external services, preventing an outage from causing restart storms.

## Start and validate

For local metrics, rules, dashboards, and an OTLP debug collector:

```sh
docker compose -f deployments/docker/docker-compose.yml --profile observability up --build
```

Prometheus and Grafana bind only to host loopback on ports 9090 and 3001. Application tracing is opt-in locally; set `OBSERVABILITY_TRACING_ENABLED=true` to send sampled traces to the local collector. The development collector uses the basic debug exporter and is not a trace store.

Production requires:

- a random metrics token file of at least 32 bytes (`openssl rand -hex 32 > /secure/path/metrics-token`);
- `OBSERVABILITY_METRICS_TOKEN_SOURCE` pointing to that file;
- `OTEL_TRACE_UPSTREAM_ENDPOINT` pointing to the managed trace backend's OTLP/HTTP base endpoint;
- optional `OTEL_TRACE_UPSTREAM_AUTHORIZATION` containing the backend Authorization header value;
- a unique `IMAGE_TAG`/`SERVICE_VERSION` for every immutable release;
- a strong `GRAFANA_ADMIN_PASSWORD` when the observability profile is enabled.

Validate the checked-in configuration with the exact pinned runtime images:

```sh
make observability-validate
```

This runs `promtool` against both Prometheus configurations and all 15 rules, the Collector's native `validate` command against development and production pipelines, and `jq` against the provisioned dashboard.

## Cardinality and privacy rules

Metrics may label only stable HTTP route patterns, a fixed HTTP method/status class, fixed dependency/queue outcomes, or pre-registered worker task names. Never add organization, tenant, user, object, message, correlation, email, raw URL, remote address, or exception text as a Prometheus label. Unknown values must collapse to `other` or `unmatched`.

Trace sampling defaults to 10% and honors the upstream W3C `traceparent` decision. Trace attributes include stable routes and queue operation metadata, but never payloads, credentials, or tenant/user IDs. Queue envelopes persist only `traceparent`/`tracestate`; arbitrary W3C baggage is intentionally not accepted or propagated. `X-Request-ID` is returned on every API response and logged. When tracing is enabled, `X-Trace-ID` is also returned and `trace_id`/`span_id` are added to the request log.

## Initial service objectives

These are initial objectives to review after 30 days of representative production traffic:

| SLI | Objective | Window | Source |
| --- | --- | --- | --- |
| API availability | 99.9% non-5xx responses | rolling 30 days | `complianceforge_http_requests_total` |
| API latency | 99% of requests at or below 500 ms | rolling 30 days | `complianceforge_http_request_duration_seconds` |
| Outbox freshness | oldest pending message below 5 minutes | continuous | `complianceforge_outbox_oldest_pending_age_seconds` |
| Required dependencies | successful readiness checks | continuous | `complianceforge_dependency_up` |

The checked-in rules page on 14.4x availability/latency burn over 5-minute and 1-hour windows and warn on a persistent 6x availability burn over 30-minute and 6-hour windows. A 99.9% availability objective has a 0.1% error budget; a 99% latency objective has a 1% budget. Tune objectives through an approved SLO review, not by silencing alerts during an incident.

## Alert response

### Availability or latency burn

1. Declare an incident at the critical threshold and record the Grafana time range, release version, and affected stable routes.
2. Compare request/error/latency panels to dependency health and PostgreSQL pool utilization.
3. Query the trace backend by response `X-Trace-ID` or structured-log `trace_id`; do not paste tokens or customer data into the incident channel.
4. Stop or roll back the most recent immutable release if the burn began at deployment and rollback is safer than a forward fix.
5. Confirm both burn windows recover before closing. Record consumed error budget and a prevention action.

### Dependency down or PostgreSQL saturation

1. Check API/worker readiness independently and identify `dependency=postgres`, `redis`, or `rabbitmq`.
2. For PostgreSQL, compare acquired/max connections, acquisition waits, long transactions, and provider capacity before increasing the pool. Keep aggregate maximums across replicas below the server limit.
3. For Redis or RabbitMQ, verify TLS endpoint/DNS/certificate health and provider status. Do not restart every replica simultaneously.
4. Keep liveness stable. Remove a process from readiness only while it cannot safely serve or process work.

### Outbox backlog or terminal queue delivery

1. Check worker readiness, queue reconnect rate, outbox counts, oldest pending age, and queue-storage collection success.
2. Inspect RabbitMQ broker alarms and queue depth in the provider console. Preserve dead-letter/quarantine payloads as restricted incident evidence.
3. Fix the consumer/contract before replay. Replay by message ID only through an audited operational tool; never bulk republish directly from the broker UI.
4. Confirm pending age declines, terminal outcomes stop increasing, and inbox deduplication remains healthy.

## Ownership and change controls

The service team owns instrumentation and SLO definitions. Platform/SRE owns Prometheus, Collector, Grafana, alert routing, retention, and capacity. Security owns telemetry access, token rotation, backend data residency, and retention policy. Every new route or worker type must include an observability review for bounded labels, trace privacy, readiness behavior, and an actionable alert/runbook change.

Rotate the metrics bearer token without downtime by deploying a new secret to Prometheus and application replicas in two coordinated waves. Validate scraping after every wave. Restrict Grafana to SSO/RBAC in the hosting platform; the Compose password is a bootstrap mechanism, not the target enterprise identity model.

## Remaining hosting integrations

This repository supplies collection, rules, and dashboards but cannot supply organization-specific infrastructure. Production still needs a durable HA Prometheus-compatible backend or remote-write target, Alertmanager/on-call routing, a retained trace backend, centralized structured-log ingestion, synthetic probes from outside the cluster, certificate/secret-manager integration, and capacity baselines. RabbitMQ queue depth/consumer utilization require the broker's Prometheus plugin or managed-provider metrics; the application currently reports publish/consume/retry/dead-letter/quarantine outcomes and reconnects. Database spans intentionally stop at aggregate pool/health metrics until parameter-redacted pgx tracing is reviewed.
