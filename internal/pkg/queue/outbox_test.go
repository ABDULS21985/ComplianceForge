package queue

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type outboxStoreRecorder struct {
	messages    []OutboxMessage
	published   []string
	failed      []string
	unattempted []string
	dead        []string
	claimErr    error
}

func (s *outboxStoreRecorder) ClaimBatch(context.Context) ([]OutboxMessage, error) {
	return append([]OutboxMessage(nil), s.messages...), s.claimErr
}

func (s *outboxStoreRecorder) MarkPublished(_ context.Context, message OutboxMessage) error {
	s.published = append(s.published, message.ID)
	return nil
}

func (s *outboxStoreRecorder) MarkFailed(_ context.Context, message OutboxMessage, _ error) (OutboxStatus, error) {
	s.failed = append(s.failed, message.ID)
	return OutboxPending, nil
}

func (s *outboxStoreRecorder) ReleaseUnattempted(_ context.Context, message OutboxMessage, _ error) error {
	s.unattempted = append(s.unattempted, message.ID)
	return nil
}

func (s *outboxStoreRecorder) MarkDead(_ context.Context, message OutboxMessage, _ error) error {
	s.dead = append(s.dead, message.ID)
	return nil
}

type outboxPublisherRecorder struct {
	published []string
	errors    map[string]error
}

func (p *outboxPublisherRecorder) PublishEnvelope(_ context.Context, _ string, envelope Envelope) error {
	p.published = append(p.published, envelope.ID)
	return p.errors[envelope.ID]
}

func outboxTestMessage(t *testing.T, id string) OutboxMessage {
	t.Helper()
	envelope, err := NewEnvelope("test.event", "", map[string]string{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	return OutboxMessage{ID: id, MessageID: envelope.ID, QueueName: "worker.test", Envelope: envelope, LeaseToken: envelope.ID, DispatchAttempts: 1, MaxAttempts: 3}
}

func TestOutboxDispatcherMarksConfirmedMessagesPublished(t *testing.T) {
	first := outboxTestMessage(t, "row-1")
	second := outboxTestMessage(t, "row-2")
	store := &outboxStoreRecorder{messages: []OutboxMessage{first, second}}
	publisher := &outboxPublisherRecorder{errors: map[string]error{}}
	dispatcher, err := NewOutboxDispatcher(store, publisher, DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	processed, err := dispatcher.DispatchOnce(context.Background())
	if err != nil || processed != 2 {
		t.Fatalf("DispatchOnce() = %d, %v", processed, err)
	}
	if !reflect.DeepEqual(store.published, []string{"row-1", "row-2"}) || len(store.failed) != 0 || len(store.dead) != 0 {
		t.Fatalf("store results: published=%v failed=%v dead=%v", store.published, store.failed, store.dead)
	}
}

func TestOutboxDispatcherReleasesBatchAfterTransientFailure(t *testing.T) {
	first := outboxTestMessage(t, "row-1")
	second := outboxTestMessage(t, "row-2")
	store := &outboxStoreRecorder{messages: []OutboxMessage{first, second}}
	publisher := &outboxPublisherRecorder{errors: map[string]error{first.Envelope.ID: errors.New("broker offline")}}
	dispatcher, err := NewOutboxDispatcher(store, publisher, DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	processed, err := dispatcher.DispatchOnce(context.Background())
	if err == nil || processed != 1 {
		t.Fatalf("DispatchOnce() = %d, %v", processed, err)
	}
	if !reflect.DeepEqual(store.failed, []string{"row-1"}) || !reflect.DeepEqual(store.unattempted, []string{"row-2"}) || len(store.published) != 0 {
		t.Fatalf("store results: published=%v failed=%v unattempted=%v", store.published, store.failed, store.unattempted)
	}
}

func TestOutboxDispatcherDeadLettersPermanentPublishFailure(t *testing.T) {
	message := outboxTestMessage(t, "row-1")
	store := &outboxStoreRecorder{messages: []OutboxMessage{message}}
	publisher := &outboxPublisherRecorder{errors: map[string]error{
		message.Envelope.ID: &UnroutableError{Exchange: "jobs", RoutingKey: "missing", ReplyCode: 312, ReplyText: "NO_ROUTE"},
	}}
	dispatcher, err := NewOutboxDispatcher(store, publisher, DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	processed, err := dispatcher.DispatchOnce(context.Background())
	if err != nil || processed != 1 {
		t.Fatalf("DispatchOnce() = %d, %v", processed, err)
	}
	if !reflect.DeepEqual(store.dead, []string{"row-1"}) || len(store.failed) != 0 {
		t.Fatalf("store results: dead=%v failed=%v", store.dead, store.failed)
	}
}

func TestOutboxRetryDelayIsExponentialAndBounded(t *testing.T) {
	want := []time.Duration{time.Second, time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second}
	for attempt, expected := range want {
		if got := outboxRetryDelay(time.Second, 5*time.Second, attempt); got != expected {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, got, expected)
		}
	}
}

func TestOutboxConfigEnvironmentAndValidation(t *testing.T) {
	t.Setenv("OUTBOX_BATCH_SIZE", "7")
	t.Setenv("OUTBOX_LEASE", "45s")
	t.Setenv("OUTBOX_RETRY_MAX", "2m")
	config, err := OutboxConfigFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if config.BatchSize != 7 || config.Lease != 45*time.Second || config.RetryMax != 2*time.Minute {
		t.Fatalf("outbox environment config = %+v", config)
	}
	config.BatchSize = 0
	if err := config.Validate(); err == nil {
		t.Fatal("expected invalid batch size")
	}
}
