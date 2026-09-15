package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

type dataGovernanceNoopOutbox struct{}

func (dataGovernanceNoopOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return nil
}

type dataGovernancePointerOutbox struct{}

func (*dataGovernancePointerOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return nil
}

func TestNewDataGovernanceRepositoryFailsClosed(t *testing.T) {
	pool := new(pgxpool.Pool)
	var typedNilOutbox *dataGovernancePointerOutbox
	for _, test := range []struct {
		name   string
		pool   *pgxpool.Pool
		outbox queuepkg.OutboxEnqueuer
		queue  string
	}{
		{name: "database", outbox: dataGovernanceNoopOutbox{}, queue: "worker"},
		{name: "outbox", pool: pool, queue: "worker"},
		{name: "typed nil outbox", pool: pool, outbox: typedNilOutbox, queue: "worker"},
		{name: "queue", pool: pool, outbox: dataGovernanceNoopOutbox{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewDataGovernanceRepository(test.pool, test.outbox, test.queue); err == nil {
				t.Fatal("constructor accepted a missing required dependency")
			}
		})
	}
	if _, err := NewDataGovernanceRepository(pool, dataGovernanceNoopOutbox{}, " worker "); err != nil {
		t.Fatalf("valid constructor error=%v", err)
	}
}
