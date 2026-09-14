// Package coordination provides renewable PostgreSQL leases for work that
// must execute on at most one replica at a time.
package coordination

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLeaseLost = errors.New("distributed scheduler lease was lost")

type Lease struct {
	TaskName  string
	OwnerID   string
	Token     string
	ExpiresAt time.Time
}

type LeaseStore interface {
	TryAcquire(context.Context, string, time.Duration) (Lease, bool, error)
	Renew(context.Context, Lease, time.Duration) (Lease, error)
	Release(context.Context, Lease, error) error
}

// PostgresLeaseStore combines a transaction-scoped advisory lock with a
// renewable row lease. The advisory lock serializes competing acquisitions;
// the row lease survives process failure and can be recovered after expiry.
type PostgresLeaseStore struct {
	pool    *pgxpool.Pool
	ownerID uuid.UUID
}

func NewPostgresLeaseStore(pool *pgxpool.Pool, ownerID string) (*PostgresLeaseStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("scheduler lease database pool is required")
	}
	owner, err := uuid.Parse(ownerID)
	if err != nil {
		return nil, fmt.Errorf("scheduler owner ID must be a UUID: %w", err)
	}
	return &PostgresLeaseStore{pool: pool, ownerID: owner}, nil
}

func (s *PostgresLeaseStore) TryAcquire(ctx context.Context, taskName string, duration time.Duration) (Lease, bool, error) {
	if err := validateLeaseInput(taskName, duration); err != nil {
		return Lease{}, false, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Lease{}, false, fmt.Errorf("begin scheduler lease acquisition: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var advisoryAcquired bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, taskName).Scan(&advisoryAcquired); err != nil {
		return Lease{}, false, fmt.Errorf("acquire scheduler advisory lock: %w", err)
	}
	if !advisoryAcquired {
		return Lease{}, false, nil
	}

	token := uuid.New()
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO scheduler_leases (
			task_name, lease_owner, lease_token, leased_until, acquired_at,
			heartbeat_at, last_started_at, run_count
		) VALUES (
			$1, $2, $3, NOW() + ($4 * INTERVAL '1 millisecond'),
			NOW(), NOW(), NOW(), 1
		)
		ON CONFLICT (task_name) DO UPDATE
		SET last_owner = scheduler_leases.lease_owner,
			last_error = CASE WHEN scheduler_leases.lease_owner IS NOT NULL
				THEN 'previous worker lease expired' ELSE NULL END,
			lease_owner = EXCLUDED.lease_owner,
			lease_token = EXCLUDED.lease_token,
			leased_until = EXCLUDED.leased_until,
			acquired_at = NOW(), heartbeat_at = NOW(), last_started_at = NOW(),
			run_count = scheduler_leases.run_count + 1, updated_at = NOW()
		WHERE scheduler_leases.lease_owner IS NULL OR scheduler_leases.leased_until <= NOW()
		RETURNING leased_until`, taskName, s.ownerID, token, duration.Milliseconds()).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return Lease{}, false, fmt.Errorf("commit skipped scheduler lease: %w", err)
		}
		return Lease{}, false, nil
	}
	if err != nil {
		return Lease{}, false, fmt.Errorf("acquire scheduler row lease: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Lease{}, false, fmt.Errorf("commit scheduler lease: %w", err)
	}
	return Lease{TaskName: taskName, OwnerID: s.ownerID.String(), Token: token.String(), ExpiresAt: expiresAt}, true, nil
}

func (s *PostgresLeaseStore) Renew(ctx context.Context, lease Lease, duration time.Duration) (Lease, error) {
	if err := validateLease(lease, duration); err != nil {
		return Lease{}, err
	}
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `
		UPDATE scheduler_leases
		SET leased_until = NOW() + ($4 * INTERVAL '1 millisecond'),
			heartbeat_at = NOW(), updated_at = NOW()
		WHERE task_name = $1 AND lease_owner = $2 AND lease_token = $3
			AND leased_until > NOW()
		RETURNING leased_until`, lease.TaskName, s.ownerID, lease.Token, duration.Milliseconds()).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, ErrLeaseLost
	}
	if err != nil {
		return Lease{}, fmt.Errorf("renew scheduler lease: %w", err)
	}
	lease.ExpiresAt = expiresAt
	return lease, nil
}

func (s *PostgresLeaseStore) Release(ctx context.Context, lease Lease, runErr error) error {
	if err := validateLease(lease, time.Nanosecond); err != nil {
		return err
	}
	errorText := ""
	if runErr != nil {
		errorText = boundedError(runErr, 2048)
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE scheduler_leases
		SET last_owner = lease_owner, last_completed_at = NOW(), last_error = NULLIF($4, ''),
			lease_owner = NULL, lease_token = NULL, leased_until = NULL,
			acquired_at = NULL, heartbeat_at = NULL, updated_at = NOW()
		WHERE task_name = $1 AND lease_owner = $2 AND lease_token = $3`,
		lease.TaskName, s.ownerID, lease.Token, errorText)
	if err != nil {
		return fmt.Errorf("release scheduler lease: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

type CoordinatorConfig struct {
	Lease          time.Duration
	ReleaseTimeout time.Duration
}

func (c CoordinatorConfig) Validate() error {
	if c.Lease <= 0 || c.ReleaseTimeout <= 0 {
		return fmt.Errorf("scheduler lease and release timeout must be positive")
	}
	return nil
}

type Coordinator struct {
	store  LeaseStore
	config CoordinatorConfig
}

func NewCoordinator(store LeaseStore, config CoordinatorConfig) (*Coordinator, error) {
	if store == nil {
		return nil, fmt.Errorf("scheduler lease store is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Coordinator{store: store, config: config}, nil
}

// Run executes work only when this replica acquired the task lease. It renews
// the lease while work is active and cancels work immediately if ownership is
// lost. acquired=false is an expected outcome when another replica is active.
func (c *Coordinator) Run(ctx context.Context, taskName string, work func(context.Context) error) (acquired bool, runErr error) {
	if work == nil {
		return false, fmt.Errorf("scheduled work is required")
	}
	lease, acquired, err := c.store.TryAcquire(ctx, taskName, c.config.Lease)
	if err != nil || !acquired {
		return acquired, err
	}

	workCtx, cancelWork := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() {
		interval := c.config.Lease / 3
		if interval <= 0 {
			interval = time.Nanosecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		current := lease
		for {
			select {
			case <-workCtx.Done():
				heartbeatDone <- nil
				return
			case <-ticker.C:
				renewed, renewErr := c.store.Renew(workCtx, current, c.config.Lease)
				if renewErr != nil {
					if workCtx.Err() != nil {
						heartbeatDone <- nil
						return
					}
					cancelWork()
					heartbeatDone <- fmt.Errorf("renew task %s lease: %w", taskName, renewErr)
					return
				}
				current = renewed
			}
		}
	}()

	runErr = invokeWork(workCtx, work)
	cancelWork()
	heartbeatErr := <-heartbeatDone
	runErr = errors.Join(runErr, heartbeatErr)

	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), c.config.ReleaseTimeout)
	releaseErr := c.store.Release(releaseCtx, lease, runErr)
	releaseCancel()
	return true, errors.Join(runErr, releaseErr)
}

func validateLeaseInput(taskName string, duration time.Duration) error {
	if strings.TrimSpace(taskName) == "" || taskName != strings.TrimSpace(taskName) || len(taskName) > 255 || !utf8.ValidString(taskName) {
		return fmt.Errorf("scheduler task name is required, trimmed UTF-8, and at most 255 bytes")
	}
	if duration <= 0 {
		return fmt.Errorf("scheduler lease duration must be positive")
	}
	return nil
}

func validateLease(lease Lease, duration time.Duration) error {
	if err := validateLeaseInput(lease.TaskName, duration); err != nil {
		return err
	}
	if lease.OwnerID == "" || lease.Token == "" {
		return fmt.Errorf("scheduler lease owner and token are required")
	}
	if _, err := uuid.Parse(lease.OwnerID); err != nil {
		return fmt.Errorf("scheduler lease owner must be a UUID: %w", err)
	}
	if _, err := uuid.Parse(lease.Token); err != nil {
		return fmt.Errorf("scheduler lease token must be a UUID: %w", err)
	}
	return nil
}

func invokeWork(ctx context.Context, work func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("scheduled work panic: %v", recovered)
		}
	}()
	return work(ctx)
}

func boundedError(err error, limit int) string {
	if err == nil {
		return ""
	}
	value := strings.ToValidUTF8(err.Error(), "?")
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
