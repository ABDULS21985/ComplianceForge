# Administrator diagnostics centre

The authenticated `GET /api/v1/settings/diagnostics` endpoint gives tenant
administrators a bounded operational snapshot without exposing credentials,
endpoints, hostnames, exception text, connector configuration, payloads, or
records belonging to another tenant. Access requires the persisted
`settings:read` permission and the normal authenticated tenant context.

## Signals

- PostgreSQL and Redis are actively probed in production composition with a
  two-second bounded timeout and measured latency. A failed probe exposes only
  a curated unavailable message; the underlying error is not serialized or
  logged by the diagnostics handler.
- The current `schema_migrations` version is compared with the schema version
  compiled into the application. A dirty or newer-than-application database is
  critical; pending application migrations are a warning. Migration validation
  fails CI if that compiled version drifts from the highest migration file.
- Transactional outbox and inbox counts are explicitly filtered by tenant,
  including due backlog, dead messages, expired leases, and oldest ready age.
- Due notification work, terminal delivery failures, and expired notification
  leases are tenant-scoped and contain no recipient or message data.
- Connector totals, health categories, and failed sync count over the previous
  24 hours are tenant-scoped and contain no configuration or provider errors.
- Configuration checks report only a stable key, category, posture, curated
  message, and remediation. They never include the value that was checked.

The overall status is the worst signal in the snapshot. Thresholds are
deliberately conservative: dead or expired queue work, an unhealthy connector,
a dirty/newer schema, a failed critical dependency, or an hour-old due backlog
is critical. Shorter queue/notification lag and degraded or unknown connectors
are warnings.

## Security and tenancy invariants

The repository verifies that `app.current_tenant` exactly matches the
authenticated organization before any query. This protects infrastructure
tables that are not themselves FORCE-RLS tables as well as tenant-owned tables
that are. The handler never accepts an organization identifier from a route,
query, header, or body and marks successful snapshots `Cache-Control: no-store`.

The non-superuser integration test creates two tenants, switches the live
PostgreSQL tenant context, validates isolated queue/notification/connector
counts, and proves a mismatched requested tenant fails closed.

## Operational use

Treat this endpoint as a first diagnostic view, not as a substitute for
metrics, traces, queue tooling, or provider consoles. Operators should use the
response request ID to correlate a failed request with protected server-side
telemetry. Probe failures intentionally do not echo the provider error.

Future dependency adapters should be registered in production composition as
bounded probes with stable keys. Do not return raw errors or configuration
values when extending the response.
