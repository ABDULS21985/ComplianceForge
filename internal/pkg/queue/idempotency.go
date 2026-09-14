package queue

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type DeduplicationStatus uint8

const (
	DeduplicationNew DeduplicationStatus = iota
	DeduplicationInProgress
	DeduplicationComplete
)

// Deduplicator can be replaced by a durable Redis/database implementation
// without changing consumer code. The default memory store protects process-
// local redeliveries and is bounded by both TTL and capacity.
type Deduplicator interface {
	Begin(ctx context.Context, envelope Envelope) (DeduplicationStatus, error)
	Renew(ctx context.Context, envelope Envelope) error
	Complete(ctx context.Context, envelope Envelope) error
	Forget(ctx context.Context, envelope Envelope) error
}

type deduplicationEntry struct {
	status    DeduplicationStatus
	expiresAt time.Time
	createdAt time.Time
}

type MemoryDeduplicator struct {
	mu         sync.Mutex
	entries    map[string]deduplicationEntry
	lease      time.Duration
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func NewMemoryDeduplicator(ttl time.Duration, maxEntries int) (*MemoryDeduplicator, error) {
	return NewMemoryDeduplicatorWithLease(ttl, ttl, maxEntries)
}

func NewMemoryDeduplicatorWithLease(lease, ttl time.Duration, maxEntries int) (*MemoryDeduplicator, error) {
	if lease <= 0 || ttl < lease || maxEntries < 1 {
		return nil, fmt.Errorf("deduplicator TTL and capacity must be greater than zero")
	}
	return &MemoryDeduplicator{
		entries:    make(map[string]deduplicationEntry),
		lease:      lease,
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}, nil
}

func (d *MemoryDeduplicator) Begin(_ context.Context, envelope Envelope) (DeduplicationStatus, error) {
	if envelope.ID == "" || envelope.Attempt < 1 {
		return DeduplicationNew, fmt.Errorf("message ID and attempt are required")
	}
	messageID := deduplicationKey(envelope)
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	d.removeExpired(now)
	if entry, exists := d.entries[messageID]; exists {
		return entry.status, nil
	}
	if len(d.entries) >= d.maxEntries {
		if !d.removeOldestCompleted() {
			return DeduplicationNew, fmt.Errorf("deduplicator is at capacity with in-progress messages")
		}
	}
	d.entries[messageID] = deduplicationEntry{
		status: DeduplicationInProgress, expiresAt: now.Add(d.lease), createdAt: now,
	}
	return DeduplicationNew, nil
}

func (d *MemoryDeduplicator) Renew(_ context.Context, envelope Envelope) error {
	messageID := deduplicationKey(envelope)
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, exists := d.entries[messageID]
	if !exists || entry.status != DeduplicationInProgress {
		return fmt.Errorf("message %s is not in progress", messageID)
	}
	entry.expiresAt = d.now().Add(d.lease)
	d.entries[messageID] = entry
	return nil
}

func (d *MemoryDeduplicator) Complete(_ context.Context, envelope Envelope) error {
	messageID := deduplicationKey(envelope)
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, exists := d.entries[messageID]
	if !exists {
		return fmt.Errorf("message %s was not started", messageID)
	}
	entry.status = DeduplicationComplete
	entry.expiresAt = d.now().Add(d.ttl)
	d.entries[messageID] = entry
	return nil
}

func (d *MemoryDeduplicator) Forget(_ context.Context, envelope Envelope) error {
	messageID := deduplicationKey(envelope)
	d.mu.Lock()
	delete(d.entries, messageID)
	d.mu.Unlock()
	return nil
}

func (d *MemoryDeduplicator) removeExpired(now time.Time) {
	for id, entry := range d.entries {
		if !entry.expiresAt.After(now) {
			delete(d.entries, id)
		}
	}
}

func (d *MemoryDeduplicator) removeOldestCompleted() bool {
	var oldestID string
	var oldestTime time.Time
	for id, entry := range d.entries {
		if entry.status != DeduplicationComplete {
			continue
		}
		if oldestID == "" || entry.createdAt.Before(oldestTime) {
			oldestID, oldestTime = id, entry.createdAt
		}
	}
	if oldestID == "" {
		return false
	}
	delete(d.entries, oldestID)
	return true
}
