package repository

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewAssetRepositoryFailsClosed(t *testing.T) {
	pool := new(pgxpool.Pool)
	for _, test := range []struct {
		name   string
		pool   *pgxpool.Pool
		outbox accessAdminNoopOutbox
		queue  string
	}{
		{name: "database", outbox: accessAdminNoopOutbox{}, queue: "worker"},
		{name: "queue", pool: pool, outbox: accessAdminNoopOutbox{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewAssetRepository(test.pool, test.outbox, test.queue); err == nil {
				t.Fatal("constructor accepted a missing required dependency")
			}
		})
	}
	if _, err := NewAssetRepository(pool, nil, "worker"); err == nil {
		t.Fatal("constructor accepted a nil outbox")
	}
	if _, err := NewAssetRepository(pool, accessAdminNoopOutbox{}, " worker "); err != nil {
		t.Fatalf("valid constructor error=%v", err)
	}
}
