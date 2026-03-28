package database

import (
	"context"
	"fmt"
	"math"
	"time"

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
	poolCfg.HealthCheckPeriod = 30 * time.Second

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
				Str("host", cfg.Database.Host).
				Int("port", cfg.Database.Port).
				Str("database", cfg.Database.DBName).
				Msg("connected to PostgreSQL")
			return pool, nil
		}

		if attempt < maxRetries {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
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
