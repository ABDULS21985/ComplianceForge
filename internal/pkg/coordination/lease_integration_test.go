//go:build integration

package coordination

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresSchedulerLeaseContentionAndStaleRecovery(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	taskName := "integration.lease." + uuid.NewString()
	first, err := NewPostgresLeaseStore(pool, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPostgresLeaseStore(pool, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM scheduler_leases WHERE task_name = $1`, taskName)
	})

	lease, acquired, err := first.TryAcquire(ctx, taskName, 2*time.Second)
	if err != nil || !acquired {
		t.Fatalf("first TryAcquire() = %t, %v", acquired, err)
	}
	if _, acquired, err := second.TryAcquire(ctx, taskName, 2*time.Second); err != nil || acquired {
		t.Fatalf("competing TryAcquire() = %t, %v", acquired, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE scheduler_leases SET leased_until = NOW() - INTERVAL '1 second' WHERE task_name = $1`, taskName); err != nil {
		t.Fatal(err)
	}
	recovered, acquired, err := second.TryAcquire(ctx, taskName, 2*time.Second)
	if err != nil || !acquired {
		t.Fatalf("stale TryAcquire() = %t, %v", acquired, err)
	}
	if err := first.Release(ctx, lease, nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale owner release error = %v", err)
	}
	renewed, err := second.Renew(ctx, recovered, 2*time.Second)
	if err != nil || !renewed.ExpiresAt.After(recovered.ExpiresAt) {
		t.Fatalf("Renew() expiry=%s, err=%v", renewed.ExpiresAt, err)
	}
	if err := second.Release(ctx, renewed, errors.New("task failed")); err != nil {
		t.Fatal(err)
	}
	var owner *string
	var lastError string
	var runCount int
	if err := pool.QueryRow(ctx, `
		SELECT lease_owner::TEXT, COALESCE(last_error, ''), run_count
		FROM scheduler_leases WHERE task_name = $1`, taskName).Scan(&owner, &lastError, &runCount); err != nil {
		t.Fatal(err)
	}
	if owner != nil || lastError != "task failed" || runCount != 2 {
		t.Fatalf("released lease owner=%v last_error=%q run_count=%d", owner, lastError, runCount)
	}
}
