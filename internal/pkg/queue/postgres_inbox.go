package queue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresDeduplicatorConfig identifies one logical consumer and one running
// replica. A consumer name is stable across deployments; OwnerID is unique to
// a process so stale replicas cannot complete work after losing their lease.
type PostgresDeduplicatorConfig struct {
	ConsumerName string
	OwnerID      string
	Lease        time.Duration
	Retention    time.Duration
}

// PostgresDeduplicator provides durable inbox idempotency across worker
// replicas and restarts. Processing leases make abandoned work recoverable.
type PostgresDeduplicator struct {
	pool            *pgxpool.Pool
	consumerName    string
	ownerID         uuid.UUID
	leaseMillis     int64
	retentionMillis int64
}

func NewPostgresDeduplicator(pool *pgxpool.Pool, config PostgresDeduplicatorConfig) (*PostgresDeduplicator, error) {
	if pool == nil {
		return nil, fmt.Errorf("inbox database pool is required")
	}
	if err := validateBrokerName(strings.TrimSpace(config.ConsumerName), 255); err != nil {
		return nil, fmt.Errorf("invalid inbox consumer name: %w", err)
	}
	ownerID, err := uuid.Parse(config.OwnerID)
	if err != nil {
		return nil, fmt.Errorf("inbox owner ID must be a UUID: %w", err)
	}
	if config.Lease < time.Millisecond || config.Retention < config.Lease {
		return nil, fmt.Errorf("inbox lease must be at least one millisecond and retention must be at least the lease")
	}
	return &PostgresDeduplicator{
		pool:            pool,
		consumerName:    strings.TrimSpace(config.ConsumerName),
		ownerID:         ownerID,
		leaseMillis:     config.Lease.Milliseconds(),
		retentionMillis: config.Retention.Milliseconds(),
	}, nil
}

func (d *PostgresDeduplicator) Begin(ctx context.Context, envelope Envelope) (DeduplicationStatus, error) {
	fingerprint, err := inboxEnvelopeFingerprint(envelope)
	if err != nil {
		return DeduplicationNew, err
	}
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DeduplicationNew, fmt.Errorf("begin inbox transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	command, err := tx.Exec(ctx, `
		INSERT INTO queue_inbox (
			consumer_name, message_id, delivery_attempt, tenant_id,
			message_type, envelope_hash, status, lease_owner, leased_until
		) VALUES ($1, $2, $3, $4, $5, $6, 'processing', $7,
			NOW() + ($8 * INTERVAL '1 millisecond'))
		ON CONFLICT (consumer_name, message_id, delivery_attempt) DO NOTHING`,
		d.consumerName, envelope.ID, envelope.Attempt, optionalUUID(envelope.TenantID),
		envelope.Type, fingerprint, d.ownerID, d.leaseMillis)
	if err != nil {
		return DeduplicationNew, fmt.Errorf("insert inbox lease: %w", err)
	}
	if command.RowsAffected() == 1 {
		if err := tx.Commit(ctx); err != nil {
			return DeduplicationNew, fmt.Errorf("commit inbox lease: %w", err)
		}
		return DeduplicationNew, nil
	}

	var status, tenantID, messageType string
	var storedFingerprint []byte
	var active bool
	err = tx.QueryRow(ctx, `
		SELECT status, COALESCE(tenant_id::TEXT, ''), message_type, envelope_hash,
			CASE
				WHEN status = 'processing' THEN leased_until > NOW()
				ELSE expires_at > NOW()
			END
		FROM queue_inbox
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3
		FOR UPDATE`, d.consumerName, envelope.ID, envelope.Attempt,
	).Scan(&status, &tenantID, &messageType, &storedFingerprint, &active)
	if err != nil {
		return DeduplicationNew, fmt.Errorf("read inbox entry: %w", err)
	}
	if tenantID != envelope.TenantID || messageType != envelope.Type || !bytes.Equal(storedFingerprint, fingerprint) {
		return DeduplicationNew, fmt.Errorf("%w: message %s attempt %d metadata changed", ErrDeduplicationConflict, envelope.ID, envelope.Attempt)
	}

	if active {
		if _, err := tx.Exec(ctx, `
			UPDATE queue_inbox
			SET received_count = received_count + 1, last_received_at = NOW()
			WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3`,
			d.consumerName, envelope.ID, envelope.Attempt); err != nil {
			return DeduplicationNew, fmt.Errorf("record duplicate inbox delivery: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return DeduplicationNew, fmt.Errorf("commit duplicate inbox delivery: %w", err)
		}
		if status == "completed" {
			return DeduplicationComplete, nil
		}
		return DeduplicationInProgress, nil
	}

	command, err = tx.Exec(ctx, `
		UPDATE queue_inbox
		SET status = 'processing', lease_owner = $4,
			leased_until = NOW() + ($5 * INTERVAL '1 millisecond'),
			received_count = received_count + 1, last_received_at = NOW(),
			completed_at = NULL, expires_at = NULL, last_error = NULL
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3`,
		d.consumerName, envelope.ID, envelope.Attempt, d.ownerID, d.leaseMillis)
	if err != nil {
		return DeduplicationNew, fmt.Errorf("recover stale inbox lease: %w", err)
	}
	if command.RowsAffected() != 1 {
		return DeduplicationNew, ErrDeduplicationLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return DeduplicationNew, fmt.Errorf("commit recovered inbox lease: %w", err)
	}
	return DeduplicationNew, nil
}

func (d *PostgresDeduplicator) Renew(ctx context.Context, envelope Envelope) error {
	fingerprint, err := inboxEnvelopeFingerprint(envelope)
	if err != nil {
		return err
	}
	command, err := d.pool.Exec(ctx, `
		UPDATE queue_inbox
		SET leased_until = NOW() + ($6 * INTERVAL '1 millisecond'),
			last_received_at = GREATEST(last_received_at, NOW())
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3
			AND status = 'processing' AND lease_owner = $4 AND envelope_hash = $5 AND leased_until > NOW()`,
		d.consumerName, envelope.ID, envelope.Attempt, d.ownerID, fingerprint, d.leaseMillis)
	if err != nil {
		return fmt.Errorf("renew inbox lease: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrDeduplicationLeaseLost
	}
	return nil
}

func (d *PostgresDeduplicator) Complete(ctx context.Context, envelope Envelope) error {
	fingerprint, err := inboxEnvelopeFingerprint(envelope)
	if err != nil {
		return err
	}
	command, err := d.pool.Exec(ctx, `
		UPDATE queue_inbox
		SET status = 'completed', lease_owner = NULL, leased_until = NULL,
			completed_at = NOW(),
			expires_at = NOW() + ($6 * INTERVAL '1 millisecond'),
			last_error = NULL
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3
			AND status = 'processing' AND lease_owner = $4 AND envelope_hash = $5 AND leased_until > NOW()`,
		d.consumerName, envelope.ID, envelope.Attempt, d.ownerID, fingerprint, d.retentionMillis)
	if err != nil {
		return fmt.Errorf("complete inbox entry: %w", err)
	}
	if command.RowsAffected() == 1 {
		return nil
	}

	var complete bool
	err = d.pool.QueryRow(ctx, `
		SELECT status = 'completed' AND expires_at > NOW()
		FROM queue_inbox
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3 AND envelope_hash = $4`,
		d.consumerName, envelope.ID, envelope.Attempt, fingerprint).Scan(&complete)
	if err == nil && complete {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("verify completed inbox entry: %w", err)
	}
	return ErrDeduplicationLeaseLost
}

func (d *PostgresDeduplicator) Forget(ctx context.Context, envelope Envelope) error {
	fingerprint, err := inboxEnvelopeFingerprint(envelope)
	if err != nil {
		return err
	}
	command, err := d.pool.Exec(ctx, `
		DELETE FROM queue_inbox
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3
			AND status = 'processing' AND lease_owner = $4 AND envelope_hash = $5`,
		d.consumerName, envelope.ID, envelope.Attempt, d.ownerID, fingerprint)
	if err != nil {
		return fmt.Errorf("forget inbox entry: %w", err)
	}
	if command.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	err = d.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM queue_inbox
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3
	)`, d.consumerName, envelope.ID, envelope.Attempt).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify forgotten inbox entry: %w", err)
	}
	if exists {
		return ErrDeduplicationLeaseLost
	}
	return nil
}

// PurgeExpired deletes a bounded batch of retained completed entries without
// blocking active consumers. It is safe to call concurrently on every replica.
func (d *PostgresDeduplicator) PurgeExpired(ctx context.Context, limit int) (int64, error) {
	if limit < 1 || limit > 10_000 {
		return 0, fmt.Errorf("inbox purge limit must be between 1 and 10000")
	}
	command, err := d.pool.Exec(ctx, `
		WITH expired AS (
			SELECT consumer_name, message_id, delivery_attempt
			FROM queue_inbox
			WHERE status = 'completed' AND expires_at <= NOW()
			ORDER BY expires_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		DELETE FROM queue_inbox target
		USING expired
		WHERE target.consumer_name = expired.consumer_name
			AND target.message_id = expired.message_id
			AND target.delivery_attempt = expired.delivery_attempt`, limit)
	if err != nil {
		return 0, fmt.Errorf("purge expired inbox entries: %w", err)
	}
	return command.RowsAffected(), nil
}

func inboxEnvelopeFingerprint(envelope Envelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, fmt.Errorf("validate inbox envelope: %w", err)
	}
	body, err := encodeEnvelope(envelope)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(body)
	return digest[:], nil
}

func optionalUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}
