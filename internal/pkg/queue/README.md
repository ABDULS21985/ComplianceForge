# Worker queue contract

The worker consumes versioned JSON envelopes from `complianceforge.worker` by
default. Producers should construct messages with `queue.NewEnvelope` and use
`PublishEnvelope`; publishing is persistent, mandatory, and acknowledged only
after RabbitMQ sends a positive publisher confirm.

Each logical queue owns four durable queues and four durable direct exchanges:

- the primary quorum queue receives normal jobs;
- `.retry` delays transient failures, then dead-letters them back to primary;
- `.dead` retains permanent failures and jobs that exhausted their attempts;
- `.quarantine` retains malformed, oversized, or contract-invalid deliveries.

The envelope and AMQP properties both carry the message ID, correlation ID,
schema version, attempt, type, and (for tenant jobs) tenant ID. A mismatch is
quarantined. Retry attempts retain the logical message ID but use an
attempt-qualified deduplication key, so an acknowledgement-loss redelivery is
suppressed while the next scheduled attempt can still execute.

The queue library's fallback in-memory deduplicator is bounded and protects
single-process redeliveries. The production worker injects the PostgreSQL inbox
from migration `000043`, which uses renewable owner-bound leases and retained
completion records to suppress duplicates across replicas and restarts.

Producers can enqueue through `PostgresOutbox.Enqueue` using a caller-owned
`pgx.Tx`. The domain mutation and versioned envelope then commit atomically.
Workers claim rows with `FOR UPDATE SKIP LOCKED`, publish with RabbitMQ confirms,
and only then mark them published. Transient failures use bounded exponential
backoff; exhausted or permanent failures become inspectable `dead` rows. A
crash after a broker confirm but before the database update may republish, so
consumer inbox idempotency remains part of the design.

Periodic jobs use renewable PostgreSQL scheduler leases plus transaction-scoped
advisory locks. This makes run-on-boot and cadence triggers safe when multiple
worker replicas start simultaneously, while expired leases remain recoverable.

## Configuration

The standard `RABBITMQ_URL` application setting supplies the broker URL. These
optional environment variables tune worker delivery without changing the core
application configuration:

- `WORKER_QUEUE_NAME`, `WORKER_INSTANCE_ID` (must be unique per live replica), `WORKER_SHUTDOWN_TIMEOUT`,
  `WORKER_SCHEDULER_LEASE`
- `QUEUE_EXCHANGE`, `QUEUE_RETRY_EXCHANGE`, `QUEUE_DEAD_LETTER_EXCHANGE`,
  `QUEUE_QUARANTINE_EXCHANGE`
- `QUEUE_TYPE` (`quorum` by default, or `classic`)
- `QUEUE_CONNECTION_NAME`, `QUEUE_PREFETCH`, `QUEUE_MAX_ATTEMPTS`
- `QUEUE_RETRY_DELAY`, `QUEUE_PUBLISH_TIMEOUT`, `QUEUE_CONNECT_TIMEOUT`,
  `QUEUE_HEARTBEAT`, `QUEUE_RECONNECT_MIN`, `QUEUE_RECONNECT_MAX`
- `QUEUE_MAX_MESSAGE_BYTES`, `QUEUE_IDEMPOTENCY_LEASE`, `QUEUE_IDEMPOTENCY_TTL`,
  `QUEUE_IDEMPOTENCY_MAX_MESSAGES`
- `OUTBOX_BATCH_SIZE`, `OUTBOX_LEASE`, `OUTBOX_POLL_INTERVAL`,
  `OUTBOX_PUBLISH_TIMEOUT`, `OUTBOX_MAX_ATTEMPTS`, `OUTBOX_RETRY_BASE`,
  `OUTBOX_RETRY_MAX`, `OUTBOX_RETENTION`, `OUTBOX_MAX_MESSAGE_BYTES`

Durations use Go duration syntax, for example `30s` or `5m`.

## Worker message types

- `notification.event` carries a `service.Event`; its `org_id` must equal the
  envelope tenant ID.
- `search.index` carries `entity_type`, UUID `entity_id`, and `action`. Supported
  entity types are risk, control, policy, incident, finding, evidence, asset,
  and vendor. Create/update/upsert actions upsert the index; delete actions are
  tenant-scoped deletes.
- `scheduler.*` triggers one of the registered global schedulers and must not
  carry a tenant ID. Scheduled jobs also run on their built-in cadence when the
  worker starts.

## Verification

Unit and race tests do not need a broker:

```sh
go test ./internal/pkg/queue ./internal/pkg/coordination
go test -race ./internal/pkg/queue ./internal/pkg/coordination
```

The opt-in integration tests create uniquely named broker/database records and
verify publisher returns, retry/dead-letter/quarantine behavior, transactional
enqueue, stale-lease recovery, durable inbox deduplication, and cross-replica
scheduler exclusion:

```sh
TEST_DATABASE_URL='postgres://user:password@127.0.0.1:5432/complianceforge?sslmode=disable' \
RABBITMQ_URL='amqp://user:password@127.0.0.1:5672/' \
  go test -tags=integration -v ./internal/pkg/queue ./internal/pkg/coordination
```
