package queue

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func deduplicationTestEnvelope() Envelope {
	return Envelope{ID: uuid.NewString(), Attempt: 1}
}

func TestMemoryDeduplicatorLifecycle(t *testing.T) {
	store, err := NewMemoryDeduplicator(time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	envelope := deduplicationTestEnvelope()
	status, err := store.Begin(ctx, envelope)
	if err != nil || status != DeduplicationNew {
		t.Fatalf("first Begin() = %v, %v", status, err)
	}
	status, _ = store.Begin(ctx, envelope)
	if status != DeduplicationInProgress {
		t.Fatalf("second Begin() = %v", status)
	}
	if err := store.Complete(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	status, _ = store.Begin(ctx, envelope)
	if status != DeduplicationComplete {
		t.Fatalf("completed Begin() = %v", status)
	}
	if err := store.Forget(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	status, _ = store.Begin(ctx, envelope)
	if status != DeduplicationNew {
		t.Fatalf("forgotten Begin() = %v", status)
	}
}

func TestMemoryDeduplicatorExpiresAndBoundsEntries(t *testing.T) {
	store, err := NewMemoryDeduplicator(time.Hour, 2)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	ctx := context.Background()
	oldest := deduplicationTestEnvelope()
	middle := deduplicationTestEnvelope()
	newest := deduplicationTestEnvelope()
	_, _ = store.Begin(ctx, oldest)
	if err := store.Complete(ctx, oldest); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	_, _ = store.Begin(ctx, middle)
	if err := store.Complete(ctx, middle); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	_, _ = store.Begin(ctx, newest)
	if len(store.entries) != 2 {
		t.Fatalf("entry count = %d", len(store.entries))
	}
	if _, exists := store.entries[deduplicationKey(oldest)]; exists {
		t.Fatal("oldest entry was not evicted")
	}

	now = now.Add(2 * time.Hour)
	status, err := store.Begin(ctx, middle)
	if err != nil || status != DeduplicationNew {
		t.Fatalf("expired Begin() = %v, %v", status, err)
	}
}

func TestMemoryDeduplicatorDoesNotEvictInProgressMessages(t *testing.T) {
	store, err := NewMemoryDeduplicator(time.Hour, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	running := deduplicationTestEnvelope()
	another := deduplicationTestEnvelope()
	if _, err := store.Begin(ctx, running); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(ctx, another); err == nil {
		t.Fatal("expected capacity error while every entry is in progress")
	}
	status, err := store.Begin(ctx, running)
	if err != nil || status != DeduplicationInProgress {
		t.Fatalf("original in-progress entry was lost: status=%v err=%v", status, err)
	}
}

func TestMemoryDeduplicatorRenewsProcessingLease(t *testing.T) {
	store, err := NewMemoryDeduplicatorWithLease(time.Minute, time.Hour, 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	envelope := deduplicationTestEnvelope()
	if _, err := store.Begin(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	want := now.Add(90 * time.Second)
	now = now.Add(30 * time.Second)
	if err := store.Renew(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	if got := store.entries[deduplicationKey(envelope)].expiresAt; !got.Equal(want) {
		t.Fatalf("renewed lease = %s, want %s", got, want)
	}
}

func TestMemoryDeduplicatorRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewMemoryDeduplicator(0, 1); err == nil {
		t.Fatal("expected TTL validation error")
	}
	if _, err := NewMemoryDeduplicator(time.Hour, 0); err == nil {
		t.Fatal("expected capacity validation error")
	}
}
