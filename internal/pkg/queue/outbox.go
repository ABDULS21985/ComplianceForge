package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
)

var (
	ErrOutboxConflict  = errors.New("outbox message ID conflicts with a different message")
	ErrOutboxLeaseLost = errors.New("outbox dispatch lease was lost")
)

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxLeased    OutboxStatus = "leased"
	OutboxPublished OutboxStatus = "published"
	OutboxDead      OutboxStatus = "dead"
)

// OutboxConfig bounds dispatch concurrency, leases, retries, and retention.
type OutboxConfig struct {
	BatchSize       int
	Lease           time.Duration
	PollInterval    time.Duration
	PublishTimeout  time.Duration
	MaxAttempts     int
	RetryBase       time.Duration
	RetryMax        time.Duration
	Retention       time.Duration
	MaxMessageBytes int
}

func DefaultOutboxConfig() OutboxConfig {
	return OutboxConfig{
		BatchSize:       25,
		Lease:           time.Minute,
		PollInterval:    time.Second,
		PublishTimeout:  15 * time.Second,
		MaxAttempts:     10,
		RetryBase:       5 * time.Second,
		RetryMax:        5 * time.Minute,
		Retention:       7 * 24 * time.Hour,
		MaxMessageBytes: 1024 * 1024,
	}
}

func OutboxConfigFromEnvironment() (OutboxConfig, error) {
	config := DefaultOutboxConfig()
	integers := map[string]*int{
		"OUTBOX_BATCH_SIZE":        &config.BatchSize,
		"OUTBOX_MAX_ATTEMPTS":      &config.MaxAttempts,
		"OUTBOX_MAX_MESSAGE_BYTES": &config.MaxMessageBytes,
	}
	for name, destination := range integers {
		if value, exists := os.LookupEnv(name); exists {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return OutboxConfig{}, fmt.Errorf("%s must be an integer: %w", name, err)
			}
			*destination = parsed
		}
	}
	durations := map[string]*time.Duration{
		"OUTBOX_LEASE":           &config.Lease,
		"OUTBOX_POLL_INTERVAL":   &config.PollInterval,
		"OUTBOX_PUBLISH_TIMEOUT": &config.PublishTimeout,
		"OUTBOX_RETRY_BASE":      &config.RetryBase,
		"OUTBOX_RETRY_MAX":       &config.RetryMax,
		"OUTBOX_RETENTION":       &config.Retention,
	}
	for name, destination := range durations {
		if value, exists := os.LookupEnv(name); exists {
			parsed, err := time.ParseDuration(strings.TrimSpace(value))
			if err != nil {
				return OutboxConfig{}, fmt.Errorf("%s must be a Go duration: %w", name, err)
			}
			*destination = parsed
		}
	}
	if err := config.Validate(); err != nil {
		return OutboxConfig{}, err
	}
	return config, nil
}

func (c OutboxConfig) Validate() error {
	if c.BatchSize < 1 || c.BatchSize > 1000 {
		return fmt.Errorf("outbox batch size must be between 1 and 1000")
	}
	if c.Lease < time.Second || c.PollInterval <= 0 || c.PublishTimeout <= 0 {
		return fmt.Errorf("outbox lease must be at least one second and polling/publish timeouts must be positive")
	}
	if c.MaxAttempts < 1 || c.MaxAttempts > 1000 {
		return fmt.Errorf("outbox max attempts must be between 1 and 1000")
	}
	if c.RetryBase <= 0 || c.RetryMax < c.RetryBase {
		return fmt.Errorf("outbox retry bounds are invalid")
	}
	if c.Retention < c.Lease {
		return fmt.Errorf("outbox retention must be at least the lease")
	}
	if c.MaxMessageBytes < 1 {
		return fmt.Errorf("outbox max message bytes must be positive")
	}
	return nil
}

type OutboxMessage struct {
	ID               string
	MessageID        string
	QueueName        string
	Envelope         Envelope
	LeaseToken       string
	DispatchAttempts int
	MaxAttempts      int
}

// OutboxEnqueuer accepts a pool, connection, or caller-owned transaction. A
// domain service should pass its pgx.Tx so the mutation and envelope commit
// atomically.
type OutboxEnqueuer interface {
	Enqueue(context.Context, database.Querier, string, Envelope) error
}

type OutboxStore interface {
	ClaimBatch(context.Context) ([]OutboxMessage, error)
	MarkPublished(context.Context, OutboxMessage) error
	MarkFailed(context.Context, OutboxMessage, error) (OutboxStatus, error)
	ReleaseUnattempted(context.Context, OutboxMessage, error) error
	MarkDead(context.Context, OutboxMessage, error) error
}

type PostgresOutbox struct {
	pool        *pgxpool.Pool
	ownerID     uuid.UUID
	config      OutboxConfig
	leaseMillis int64
}

func NewPostgresOutbox(pool *pgxpool.Pool, ownerID string, config OutboxConfig) (*PostgresOutbox, error) {
	if pool == nil {
		return nil, fmt.Errorf("outbox database pool is required")
	}
	owner, err := uuid.Parse(ownerID)
	if err != nil {
		return nil, fmt.Errorf("outbox owner ID must be a UUID: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &PostgresOutbox{pool: pool, ownerID: owner, config: config, leaseMillis: config.Lease.Milliseconds()}, nil
}

func (o *PostgresOutbox) Enqueue(ctx context.Context, executor database.Querier, queueName string, envelope Envelope) error {
	if executor == nil {
		return fmt.Errorf("outbox enqueue executor is required")
	}
	queueName = strings.TrimSpace(queueName)
	if err := validateBrokerName(queueName, 240); err != nil {
		return fmt.Errorf("invalid outbox queue name: %w", err)
	}
	envelope = InjectTraceContext(ctx, envelope.normalized())
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate outbox envelope: %w", err)
	}
	body, err := encodeEnvelope(envelope)
	if err != nil {
		return err
	}
	if len(body) > o.config.MaxMessageBytes {
		return fmt.Errorf("encoded outbox message is %d bytes; limit is %d", len(body), o.config.MaxMessageBytes)
	}
	command, err := executor.Exec(ctx, `
		INSERT INTO queue_outbox (
			message_id, queue_name, tenant_id, message_type, schema_version,
			correlation_id, causation_id, envelope, max_attempts
		) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9)
		ON CONFLICT (message_id) DO NOTHING`,
		envelope.ID, queueName, optionalUUID(envelope.TenantID), envelope.Type,
		envelope.SchemaVersion, envelope.CorrelationID, envelope.CausationID, body, o.config.MaxAttempts)
	if err != nil {
		return fmt.Errorf("enqueue outbox message: %w", err)
	}
	if command.RowsAffected() == 1 {
		return nil
	}

	var identical bool
	err = executor.QueryRow(ctx, `
		SELECT queue_name = $2 AND envelope = $3::JSONB
		FROM queue_outbox WHERE message_id = $1`, envelope.ID, queueName, body).Scan(&identical)
	if errors.Is(err, pgx.ErrNoRows) {
		// A conflicting identifier may belong to another RLS tenant. Return the
		// same opaque conflict as a visible non-identical row so callers cannot
		// use idempotent enqueue as a cross-tenant existence oracle.
		return ErrOutboxConflict
	}
	if err != nil {
		return fmt.Errorf("verify existing outbox message: %w", err)
	}
	if !identical {
		return ErrOutboxConflict
	}
	return nil
}

func (o *PostgresOutbox) ClaimBatch(ctx context.Context) ([]OutboxMessage, error) {
	tx, err := o.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `
		WITH exhausted AS (
			SELECT id FROM queue_outbox
			WHERE status = 'leased' AND leased_until <= NOW() AND dispatch_attempts >= max_attempts
			ORDER BY leased_until, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE queue_outbox target
		SET status = 'dead', lease_owner = NULL, lease_token = NULL, leased_until = NULL,
			dead_lettered_at = NOW(), last_error = COALESCE(last_error, 'dispatch lease expired after final attempt'),
			updated_at = NOW()
		FROM exhausted WHERE target.id = exhausted.id`, o.config.BatchSize); err != nil {
		return nil, fmt.Errorf("dead-letter exhausted outbox leases: %w", err)
	}

	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM queue_outbox
			WHERE dispatch_attempts < max_attempts AND (
				(status = 'pending' AND available_at <= NOW())
				OR (status = 'leased' AND leased_until <= NOW())
			)
			ORDER BY available_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE queue_outbox target
		SET status = 'leased', lease_owner = $2, lease_token = gen_random_uuid(),
			leased_until = NOW() + ($3 * INTERVAL '1 millisecond'),
			dispatch_attempts = dispatch_attempts + 1, updated_at = NOW()
		FROM candidates
		WHERE target.id = candidates.id
		RETURNING target.id, target.message_id, target.queue_name, target.envelope, target.lease_token,
			target.dispatch_attempts, target.max_attempts`,
		o.config.BatchSize, o.ownerID, o.leaseMillis)
	if err != nil {
		return nil, fmt.Errorf("claim outbox batch: %w", err)
	}
	messages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (OutboxMessage, error) {
		var message OutboxMessage
		var body []byte
		if err := row.Scan(&message.ID, &message.MessageID, &message.QueueName, &body, &message.LeaseToken, &message.DispatchAttempts, &message.MaxAttempts); err != nil {
			return OutboxMessage{}, err
		}
		envelope, err := decodeEnvelope(body)
		if err != nil {
			return OutboxMessage{}, fmt.Errorf("decode claimed outbox envelope %s: %w", message.ID, err)
		}
		if err := envelope.Validate(); err != nil {
			return OutboxMessage{}, fmt.Errorf("validate claimed outbox envelope %s: %w", message.ID, err)
		}
		if envelope.ID != message.MessageID {
			return OutboxMessage{}, fmt.Errorf("claimed outbox envelope ID %s does not match row message ID %s", envelope.ID, message.MessageID)
		}
		message.Envelope = envelope
		return message, nil
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit outbox claim: %w", err)
	}
	return messages, nil
}

func (o *PostgresOutbox) MarkPublished(ctx context.Context, message OutboxMessage) error {
	command, err := o.pool.Exec(ctx, `
		UPDATE queue_outbox
		SET status = 'published', lease_owner = NULL, lease_token = NULL, leased_until = NULL,
			published_at = NOW(), last_error = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'leased' AND lease_owner = $2 AND lease_token = $3`,
		message.ID, o.ownerID, message.LeaseToken)
	if err != nil {
		return fmt.Errorf("mark outbox message published: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrOutboxLeaseLost
	}
	return nil
}

func (o *PostgresOutbox) MarkFailed(ctx context.Context, message OutboxMessage, cause error) (OutboxStatus, error) {
	delay := outboxRetryDelay(o.config.RetryBase, o.config.RetryMax, message.DispatchAttempts)
	var status OutboxStatus
	err := o.pool.QueryRow(ctx, `
		UPDATE queue_outbox
		SET status = CASE WHEN dispatch_attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
			lease_owner = NULL, lease_token = NULL, leased_until = NULL,
			available_at = CASE WHEN dispatch_attempts >= max_attempts THEN available_at
				ELSE NOW() + ($4 * INTERVAL '1 millisecond') END,
			dead_lettered_at = CASE WHEN dispatch_attempts >= max_attempts THEN NOW() ELSE NULL END,
			last_error = $5, updated_at = NOW()
		WHERE id = $1 AND status = 'leased' AND lease_owner = $2 AND lease_token = $3
		RETURNING status`, message.ID, o.ownerID, message.LeaseToken, delay.Milliseconds(), failureReason(cause)).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOutboxLeaseLost
	}
	if err != nil {
		return "", fmt.Errorf("release failed outbox message: %w", err)
	}
	return status, nil
}

// ReleaseUnattempted returns a row claimed behind a transiently failed publish
// to the pending set without consuming an attempt it never actually made.
func (o *PostgresOutbox) ReleaseUnattempted(ctx context.Context, message OutboxMessage, cause error) error {
	command, err := o.pool.Exec(ctx, `
		UPDATE queue_outbox
		SET status = 'pending', lease_owner = NULL, lease_token = NULL, leased_until = NULL,
			dispatch_attempts = GREATEST(dispatch_attempts - 1, 0),
			available_at = NOW() + ($4 * INTERVAL '1 millisecond'),
			last_error = $5, updated_at = NOW()
		WHERE id = $1 AND status = 'leased' AND lease_owner = $2 AND lease_token = $3`,
		message.ID, o.ownerID, message.LeaseToken, o.config.RetryBase.Milliseconds(), failureReason(cause))
	if err != nil {
		return fmt.Errorf("release unattempted outbox message: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrOutboxLeaseLost
	}
	return nil
}

func (o *PostgresOutbox) MarkDead(ctx context.Context, message OutboxMessage, cause error) error {
	command, err := o.pool.Exec(ctx, `
		UPDATE queue_outbox
		SET status = 'dead', lease_owner = NULL, lease_token = NULL, leased_until = NULL,
			dead_lettered_at = NOW(), last_error = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'leased' AND lease_owner = $2 AND lease_token = $3`,
		message.ID, o.ownerID, message.LeaseToken, failureReason(cause))
	if err != nil {
		return fmt.Errorf("dead-letter outbox message: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrOutboxLeaseLost
	}
	return nil
}

func (o *PostgresOutbox) PurgePublishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 10_000 {
		return 0, fmt.Errorf("outbox purge limit must be between 1 and 10000")
	}
	command, err := o.pool.Exec(ctx, `
		WITH expired AS (
			SELECT id FROM queue_outbox
			WHERE status = 'published' AND published_at < $1
			ORDER BY published_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		DELETE FROM queue_outbox target USING expired WHERE target.id = expired.id`, before, limit)
	if err != nil {
		return 0, fmt.Errorf("purge published outbox messages: %w", err)
	}
	return command.RowsAffected(), nil
}

func outboxRetryDelay(base, maximum time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		return base
	}
	delay := base
	for current := 1; current < attempt; current++ {
		if delay >= maximum || delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}
