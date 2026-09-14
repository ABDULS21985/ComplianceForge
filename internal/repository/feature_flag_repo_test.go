package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

type featureFlagNoopOutbox struct{}

func (featureFlagNoopOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return nil
}

func TestNewFeatureFlagRepositoryFailsClosed(t *testing.T) {
	pool := new(pgxpool.Pool)
	for _, test := range []struct {
		name   string
		pool   *pgxpool.Pool
		outbox queuepkg.OutboxEnqueuer
		queue  string
	}{
		{name: "database", outbox: featureFlagNoopOutbox{}, queue: "worker"},
		{name: "outbox", pool: pool, queue: "worker"},
		{name: "queue", pool: pool, outbox: featureFlagNoopOutbox{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewFeatureFlagRepository(test.pool, test.outbox, test.queue); err == nil {
				t.Fatal("constructor accepted a missing required dependency")
			}
		})
	}
	if _, err := NewFeatureFlagRepository(pool, featureFlagNoopOutbox{}, " worker "); err != nil {
		t.Fatalf("valid constructor error=%v", err)
	}
}
