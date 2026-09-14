package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

type accessAdminNoopOutbox struct{}

func (accessAdminNoopOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return nil
}

func TestNewAccessAdministrationRepositoryFailsClosed(t *testing.T) {
	pool := new(pgxpool.Pool)
	for _, test := range []struct {
		name   string
		pool   *pgxpool.Pool
		outbox queuepkg.OutboxEnqueuer
		queue  string
	}{
		{name: "database", outbox: accessAdminNoopOutbox{}, queue: "worker"},
		{name: "outbox", pool: pool, queue: "worker"},
		{name: "queue", pool: pool, outbox: accessAdminNoopOutbox{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewAccessAdministrationRepository(test.pool, test.outbox, test.queue); err == nil {
				t.Fatal("constructor accepted a missing required dependency")
			}
		})
	}
	if _, err := NewAccessAdministrationRepository(pool, accessAdminNoopOutbox{}, " worker "); err != nil {
		t.Fatalf("valid constructor error=%v", err)
	}
}

func TestSortPermissionGrantsIsDeterministic(t *testing.T) {
	values := []models.PermissionGrant{
		{Resource: "risks", Action: "update"},
		{Resource: "policies", Action: "read"},
		{Resource: "risks", Action: "read"},
	}
	sortPermissionGrants(values)
	want := []string{"policies:read", "risks:read", "risks:update"}
	for index, value := range values {
		if got := value.Resource + ":" + value.Action; got != want[index] {
			t.Fatalf("index=%d got=%q want=%q values=%#v", index, got, want[index], values)
		}
	}
}
