package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
)

const (
	scheduledTenantPageSize   = 1000
	scheduledTenantErrorLimit = 100
)

// runForScheduledTenants prevents a non-owner/NOBYPASS worker from silently
// seeing an empty FORCE-RLS result set. Every callback runs on the same
// acquired connection on which the tenant session setting was established.
// Enumeration is streamed in bounded keyset pages so large installations do
// not accumulate every tenant identifier in memory before work can begin.
func runForScheduledTenants(
	ctx context.Context,
	pool *pgxpool.Pool,
	operation string,
	run func(context.Context, string) error,
) error {
	return runForScheduledTenantsPageSize(ctx, pool, operation, run, scheduledTenantPageSize)
}

// A small page size is injectable for real PostgreSQL contract tests without
// seeding thousands of business tenants. Production uses the bounded default.
func runForScheduledTenantsPageSize(
	ctx context.Context,
	pool *pgxpool.Pool,
	operation string,
	run func(context.Context, string) error,
	pageSize int,
) error {
	if pool == nil || run == nil {
		return fmt.Errorf("%s scheduler is not configured", operation)
	}
	if pageSize < 1 || pageSize > scheduledTenantPageSize {
		return fmt.Errorf("%s scheduler page size must be between 1 and %d", operation, scheduledTenantPageSize)
	}
	var runErrors []error
	failedTenants := 0
	var after *string
	for {
		rows, err := pool.Query(ctx,
			`SELECT organization_id FROM evidence_due_tenants($1,$2::uuid)`,
			pageSize, after,
		)
		if err != nil {
			return errors.Join(errors.Join(runErrors...), fmt.Errorf("enumerate scheduled tenants: %w", err))
		}
		page := make([]string, 0, pageSize)
		for rows.Next() {
			var organizationID string
			if err := rows.Scan(&organizationID); err != nil {
				rows.Close()
				return errors.Join(errors.Join(runErrors...), fmt.Errorf("scan scheduled tenant: %w", err))
			}
			page = append(page, organizationID)
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return errors.Join(errors.Join(runErrors...), fmt.Errorf("iterate scheduled tenants: %w", rowErr))
		}
		for _, organizationID := range page {
			if err := ctx.Err(); err != nil {
				return errors.Join(errors.Join(runErrors...), err)
			}
			if err := database.WithTenantConnection(ctx, pool, organizationID, func(tenantCtx context.Context) error {
				return run(tenantCtx, organizationID)
			}); err != nil {
				failedTenants++
				if len(runErrors) < scheduledTenantErrorLimit {
					runErrors = append(runErrors, fmt.Errorf("%s tenant %s: %w", operation, organizationID, err))
				}
			}
		}
		if len(page) < pageSize {
			if omitted := failedTenants - len(runErrors); omitted > 0 {
				runErrors = append(runErrors, fmt.Errorf("%s failed for %d additional tenants", operation, omitted))
			}
			return errors.Join(runErrors...)
		}
		after = &page[len(page)-1]
	}
}

type workerTransactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func beginWorkerTransaction(ctx context.Context, querier database.Querier) (pgx.Tx, error) {
	beginner, ok := querier.(workerTransactionBeginner)
	if !ok {
		return nil, errors.New("tenant database executor does not support transactions")
	}
	return beginner.Begin(ctx)
}

// scheduledEventID gives recurring deadline checks one durable identity per
// entity, threshold and source deadline. Re-running a scheduler therefore
// reaches the outbox's idempotent conflict path instead of producing a new
// notification every polling interval.
func scheduledEventID(organizationID, eventType, entityID, threshold string, deadline time.Time) string {
	identity := strings.Join([]string{
		"complianceforge", "scheduler", organizationID, eventType, entityID,
		threshold, deadline.UTC().Format(time.RFC3339Nano),
	}, ":")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(identity)).String()
}
