package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is implemented by pgx pools, acquired connections, and
// transactions. Repositories use the request-scoped implementation when one
// is available so RLS settings are applied to the connection executing SQL.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type querierContextKey struct{}

// WithQuerier stores a request-scoped database executor in ctx.
func WithQuerier(ctx context.Context, querier Querier) context.Context {
	return context.WithValue(ctx, querierContextKey{}, querier)
}

// QuerierFromContext returns the request-scoped executor, or fallback when no
// executor has been attached.
func QuerierFromContext(ctx context.Context, fallback Querier) Querier {
	if querier, ok := ctx.Value(querierContextKey{}).(Querier); ok && querier != nil {
		return querier
	}
	return fallback
}

// WithTenantConnection runs fn on one acquired connection with a parameterized
// RLS tenant setting. The setting is always cleared before the connection is
// returned to the pool. An existing request-scoped executor is reused.
func WithTenantConnection(
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID string,
	fn func(context.Context) error,
) (returnErr error) {
	if QuerierFromContext(ctx, nil) != nil {
		return fn(ctx)
	}
	if pool == nil {
		return fmt.Errorf("tenant database pool is nil")
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring tenant database connection: %w", err)
	}
	release := true
	tenantSet := false
	defer func() {
		if tenantSet {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := conn.Exec(cleanupCtx, `SELECT set_config('app.current_tenant', '', false)`); err != nil {
				// A connection whose tenant state could not be cleared must never
				// be returned to the shared pool.
				release = false
				underlying := conn.Hijack()
				_ = underlying.Close(cleanupCtx)
				if returnErr == nil {
					returnErr = fmt.Errorf("clearing tenant context: %w", err)
				}
			}
		}
		if release {
			conn.Release()
		}
	}()

	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant', $1, false)`, tenantID); err != nil {
		return fmt.Errorf("setting tenant context: %w", err)
	}
	tenantSet = true

	return fn(WithQuerier(ctx, conn))
}
