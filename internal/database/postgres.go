package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
)

// NewPostgresPool creates a new pgxpool.Pool configured from the application
// config. It retries the connection up to 3 times with exponential backoff.
func NewPostgresPool(cfg *config.Config) (*pgxpool.Pool, error) {
	dsn := cfg.DatabaseDSN()

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}

	poolCfg.MaxConns = cfg.Database.MaxConns
	poolCfg.MinConns = cfg.Database.MinConns
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnLifetimeJitter = 5 * time.Minute
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second
	poolCfg.ConnConfig.ConnectTimeout = 5 * time.Second
	poolCfg.ConnConfig.RuntimeParams["application_name"] = cfg.App.Name
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = "30s"
	poolCfg.ConnConfig.RuntimeParams["lock_timeout"] = "5s"
	poolCfg.AfterRelease = clearTenantContext

	const maxRetries = 3
	var pool *pgxpool.Pool

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		pool, err = pgxpool.NewWithConfig(ctx, poolCfg)
		if err == nil {
			// Verify the connection is actually usable.
			err = pool.Ping(ctx)
		}
		cancel()

		if err == nil {
			log.Info().
				Str("host", poolCfg.ConnConfig.Host).
				Uint16("port", poolCfg.ConnConfig.Port).
				Str("database", poolCfg.ConnConfig.Database).
				Msg("connected to PostgreSQL")
			return pool, nil
		}
		if pool != nil {
			pool.Close()
			pool = nil
		}

		if attempt < maxRetries {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			log.Warn().
				Err(err).
				Int("attempt", attempt).
				Dur("backoff", backoff).
				Msg("failed to connect to PostgreSQL, retrying")
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("connecting to PostgreSQL after %d attempts: %w", maxRetries, err)
}

// clearTenantContext is a defense-in-depth guard against tenant state leaking
// between callers if a code path accidentally uses session-scoped set_config.
// Request queries must still use SET LOCAL inside their transaction.
func clearTenantContext(conn *pgx.Conn) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := conn.Exec(ctx, "SELECT set_config('app.current_tenant', '', false)"); err != nil {
		log.Error().Err(err).Msg("discarding PostgreSQL connection that could not clear tenant context")
		return false
	}
	return true
}

// HealthCheck pings the database pool and returns an error if it is unreachable.
func HealthCheck(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}
	return nil
}

// EnableRLS enables Row-Level Security on the database and creates the
// set_tenant helper function. The function stores the tenant ID in
// current_setting('app.current_tenant') so that RLS policies can reference it.
func EnableRLS(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
		CREATE OR REPLACE FUNCTION set_tenant(tenant_id TEXT)
		RETURNS VOID
		LANGUAGE plpgsql
		AS $$
		BEGIN
			PERFORM set_config('app.current_tenant', tenant_id, false);
		END;
		$$;
	`

	if _, err := pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("creating set_tenant function: %w", err)
	}

	log.Info().Msg("RLS set_tenant function created successfully")
	return nil
}
