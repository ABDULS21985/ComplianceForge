//go:build integration

package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestPostgresOutboxInboxAndRabbitMQContract(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	amqpURL := os.Getenv("RABBITMQ_URL")
	if databaseURL == "" || amqpURL == "" {
		t.Skip("TEST_DATABASE_URL (or DATABASE_URL) and RABBITMQ_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	token := uuid.NewString()
	queueName := "cf.durable." + token
	ownerOne := uuid.NewString()
	ownerTwo := uuid.NewString()
	queueConfig := DefaultConfig(amqpURL)
	queueConfig.Exchange = "cf.durable." + token
	queueConfig.RetryExchange = queueConfig.Exchange + ".retry"
	queueConfig.DeadLetterExchange = queueConfig.Exchange + ".dead"
	queueConfig.QuarantineExchange = queueConfig.Exchange + ".quarantine"
	queueConfig.ConnectionName = "cf-durable-contract"
	queueConfig.IdempotencyLease = 3 * time.Second
	queueConfig.IdempotencyTTL = time.Hour
	queueConfig.ReconnectMin = 100 * time.Millisecond
	queueConfig.ReconnectMax = time.Second
	queueConfig.PublishTimeout = 5 * time.Second
	queueConfig.ConnectTimeout = 5 * time.Second

	outboxConfig := DefaultOutboxConfig()
	outboxConfig.BatchSize = 10
	outboxConfig.Lease = 2 * time.Second
	outboxConfig.MaxAttempts = 2
	outboxConfig.RetryBase = time.Second
	outboxConfig.RetryMax = 2 * time.Second
	outboxOne, err := NewPostgresOutbox(pool, ownerOne, outboxConfig)
	if err != nil {
		t.Fatal(err)
	}
	outboxTwo, err := NewPostgresOutbox(pool, ownerTwo, outboxConfig)
	if err != nil {
		t.Fatal(err)
	}
	deduplicatorOne, err := NewPostgresDeduplicator(pool, PostgresDeduplicatorConfig{
		ConsumerName: queueName, OwnerID: ownerOne, Lease: queueConfig.IdempotencyLease, Retention: queueConfig.IdempotencyTTL,
	})
	if err != nil {
		t.Fatal(err)
	}
	deduplicatorTwo, err := NewPostgresDeduplicator(pool, PostgresDeduplicatorConfig{
		ConsumerName: queueName, OwnerID: ownerTwo, Lease: queueConfig.IdempotencyLease, Retention: queueConfig.IdempotencyTTL,
	})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := NewRabbitMQServiceWithConfig(queueConfig, deduplicatorOne)
	if err != nil {
		t.Fatal(err)
	}

	var messageIDs []string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM queue_inbox WHERE consumer_name = $1`, queueName)
		if len(messageIDs) > 0 {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM queue_outbox WHERE message_id = ANY($1::UUID[])`, messageIDs)
		}
		_ = broker.Close()
		deleteRabbitTopology(cleanupCtx, amqpURL, queueName, queueConfig)
	})

	envelope, err := NewEnvelope("integration.durable", uuid.NewString(), map[string]bool{"committed": true})
	if err != nil {
		t.Fatal(err)
	}
	messageIDs = append(messageIDs, envelope.ID)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := outboxOne.Enqueue(ctx, tx, queueName, envelope); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertOutboxCount(t, ctx, pool, envelope.ID, 0)

	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := outboxOne.Enqueue(ctx, tx, queueName, envelope); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := outboxOne.Enqueue(ctx, pool, queueName, envelope); err != nil {
		t.Fatalf("idempotent enqueue failed: %v", err)
	}
	conflict := envelope
	conflict.Payload = []byte(`{"committed":false}`)
	if err := outboxOne.Enqueue(ctx, pool, queueName, conflict); !errors.Is(err, ErrOutboxConflict) {
		t.Fatalf("conflicting enqueue error = %v", err)
	}

	dispatcher, err := NewOutboxDispatcher(outboxOne, broker, outboxConfig)
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := dispatcher.DispatchOnce(ctx); err != nil || processed != 1 {
		t.Fatalf("outbox DispatchOnce() = %d, %v", processed, err)
	}
	var outboxStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM queue_outbox WHERE message_id = $1`, envelope.ID).Scan(&outboxStatus); err != nil || outboxStatus != string(OutboxPublished) {
		t.Fatalf("published outbox status = %q, %v", outboxStatus, err)
	}

	consumerCtx, cancelConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)
	firstDelivery := make(chan struct{}, 1)
	var handlerCalls atomic.Int32
	go func() {
		consumerDone <- broker.SubscribeEnvelope(consumerCtx, queueName, func(_ context.Context, delivered Envelope) error {
			if delivered.ID != envelope.ID {
				return Permanent(fmt.Errorf("unexpected message %s", delivered.ID))
			}
			handlerCalls.Add(1)
			firstDelivery <- struct{}{}
			return nil
		})
	}()
	select {
	case <-firstDelivery:
	case <-ctx.Done():
		t.Fatal("timed out waiting for outbox delivery")
	}
	waitForInboxStatus(t, ctx, pool, queueName, envelope.ID, "completed")
	if err := broker.PublishEnvelope(ctx, queueName, envelope); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if calls := handlerCalls.Load(); calls != 1 {
		t.Fatalf("durable duplicate invoked handler %d times", calls)
	}
	cancelConsumer()
	select {
	case err := <-consumerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("durable consumer did not drain")
	}

	staleEnvelope, err := NewEnvelope("integration.stale", "", map[string]bool{"recover": true})
	if err != nil {
		t.Fatal(err)
	}
	messageIDs = append(messageIDs, staleEnvelope.ID)
	if err := outboxOne.Enqueue(ctx, pool, queueName, staleEnvelope); err != nil {
		t.Fatal(err)
	}
	firstLease := claimOnlyMessage(t, ctx, outboxOne)
	if competing, err := outboxTwo.ClaimBatch(ctx); err != nil || len(competing) != 0 {
		t.Fatalf("competing claim = %d, %v", len(competing), err)
	}
	if _, err := pool.Exec(ctx, `UPDATE queue_outbox SET leased_until = NOW() - INTERVAL '1 second' WHERE id = $1`, firstLease.ID); err != nil {
		t.Fatal(err)
	}
	recovered := claimOnlyMessage(t, ctx, outboxTwo)
	if recovered.DispatchAttempts != 2 {
		t.Fatalf("recovered dispatch attempts = %d", recovered.DispatchAttempts)
	}
	if err := outboxOne.MarkPublished(ctx, firstLease); !errors.Is(err, ErrOutboxLeaseLost) {
		t.Fatalf("stale outbox owner error = %v", err)
	}
	status, err := outboxTwo.MarkFailed(ctx, recovered, errors.New("broker unavailable"))
	if err != nil || status != OutboxDead {
		t.Fatalf("final outbox failure status = %q, %v", status, err)
	}

	inboxEnvelope, err := NewEnvelope("integration.inbox-stale", "", map[string]bool{"recover": true})
	if err != nil {
		t.Fatal(err)
	}
	if status, err := deduplicatorOne.Begin(ctx, inboxEnvelope); err != nil || status != DeduplicationNew {
		t.Fatalf("first inbox Begin() = %v, %v", status, err)
	}
	if status, err := deduplicatorTwo.Begin(ctx, inboxEnvelope); err != nil || status != DeduplicationInProgress {
		t.Fatalf("competing inbox Begin() = %v, %v", status, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE queue_inbox SET leased_until = NOW() - INTERVAL '1 second'
		WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = $3`, queueName, inboxEnvelope.ID, inboxEnvelope.Attempt); err != nil {
		t.Fatal(err)
	}
	if status, err := deduplicatorTwo.Begin(ctx, inboxEnvelope); err != nil || status != DeduplicationNew {
		t.Fatalf("recovered inbox Begin() = %v, %v", status, err)
	}
	if err := deduplicatorOne.Complete(ctx, inboxEnvelope); !errors.Is(err, ErrDeduplicationLeaseLost) {
		t.Fatalf("stale inbox owner error = %v", err)
	}
	if err := deduplicatorTwo.Complete(ctx, inboxEnvelope); err != nil {
		t.Fatal(err)
	}
	if status, err := deduplicatorOne.Begin(ctx, inboxEnvelope); err != nil || status != DeduplicationComplete {
		t.Fatalf("completed inbox Begin() = %v, %v", status, err)
	}
	inboxConflict := inboxEnvelope
	inboxConflict.Payload = []byte(`{"recover":false}`)
	if _, err := deduplicatorOne.Begin(ctx, inboxConflict); !errors.Is(err, ErrDeduplicationConflict) {
		t.Fatalf("conflicting inbox envelope error = %v", err)
	}

	concurrentEnvelope, err := NewEnvelope("integration.inbox-concurrent", "", map[string]bool{"race": true})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type beginResult struct {
		status DeduplicationStatus
		err    error
	}
	results := make(chan beginResult, 2)
	for _, deduplicator := range []*PostgresDeduplicator{deduplicatorOne, deduplicatorTwo} {
		deduplicator := deduplicator
		go func() {
			<-start
			status, err := deduplicator.Begin(ctx, concurrentEnvelope)
			results <- beginResult{status: status, err: err}
		}()
	}
	close(start)
	seen := map[DeduplicationStatus]int{}
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent inbox Begin() error = %v", result.err)
		}
		seen[result.status]++
	}
	if seen[DeduplicationNew] != 1 || seen[DeduplicationInProgress] != 1 {
		t.Fatalf("concurrent inbox statuses = %v", seen)
	}
}

func claimOnlyMessage(t *testing.T, ctx context.Context, outbox *PostgresOutbox) OutboxMessage {
	t.Helper()
	messages, err := outbox.ClaimBatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("claimed %d outbox messages, want 1", len(messages))
	}
	return messages[0]
}

func assertOutboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_outbox WHERE message_id = $1`, messageID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("outbox count = %d, want %d", count, want)
	}
}

func waitForInboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerName, messageID, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := pool.QueryRow(ctx, `
			SELECT status FROM queue_inbox
			WHERE consumer_name = $1 AND message_id = $2 AND delivery_attempt = 1`, consumerName, messageID).Scan(&status)
		if err == nil && status == want {
			return
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("inbox message %s did not reach status %s", messageID, want)
}

func deleteRabbitTopology(ctx context.Context, amqpURL, queueName string, config Config) {
	connection, err := amqp.Dial(amqpURL)
	if err != nil {
		return
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		return
	}
	defer channel.Close()
	topology, err := TopologyFor(queueName, config)
	if err != nil {
		return
	}
	_, _ = channel.QueueDelete(topology.Queue, false, false, false)
	_, _ = channel.QueueDelete(topology.RetryQueue, false, false, false)
	_, _ = channel.QueueDelete(topology.DeadLetterQueue, false, false, false)
	_, _ = channel.QueueDelete(topology.QuarantineQueue, false, false, false)
	_ = channel.ExchangeDelete(config.Exchange, false, false)
	_ = channel.ExchangeDelete(config.RetryExchange, false, false)
	_ = channel.ExchangeDelete(config.DeadLetterExchange, false, false)
	_ = channel.ExchangeDelete(config.QuarantineExchange, false, false)
}
