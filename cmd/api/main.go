package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/observability"
	"github.com/complianceforge/platform/internal/pkg/ratelimit"
	"github.com/complianceforge/platform/internal/router"
)

func main() {
	// Load application configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	// Initialize zerolog.
	level, err := zerolog.ParseLevel(cfg.Log.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	if cfg.App.Env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	if err := run(ctx, cfg); err != nil {
		log.Fatal().Err(err).Msg("API stopped with an error")
	}
}

func run(ctx context.Context, cfg *config.Config) error {
	// Create PostgreSQL connection pool.
	pool, err := database.NewPostgresPool(cfg)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()

	// Shared Redis is required for API-key quotas and participates in readiness.
	redisClient, err := database.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("create Redis client: %w", err)
	}
	defer redisClient.Close()
	apiKeyLimiter, err := ratelimit.NewRedisAPIKeyLimiter(redisClient)
	if err != nil {
		return fmt.Errorf("create API-key rate limiter: %w", err)
	}
	requestLimiter, err := ratelimit.NewRedisRequestLimiter(redisClient)
	if err != nil {
		return fmt.Errorf("create request rate limiter: %w", err)
	}

	instanceID := uuid.NewString()
	telemetry, err := observability.New(ctx, cfg.Observability, "complianceforge-api", cfg.App.Env, instanceID)
	if err != nil {
		return fmt.Errorf("initialize observability: %w", err)
	}
	telemetry.Metrics().RegisterPostgresPool(pool)
	telemetry.Metrics().RegisterRedis(redisClient)
	postgresHealth := telemetry.Metrics().WrapDependencyCheck("postgres", func(ctx context.Context) error {
		return database.HealthCheck(ctx, pool)
	})
	redisHealth := telemetry.Metrics().WrapDependencyCheck("redis", func(ctx context.Context) error {
		return database.RedisHealthCheck(ctx, redisClient)
	})
	readiness := func(ctx context.Context) error {
		return errors.Join(postgresHealth(ctx), redisHealth(ctx))
	}
	if err := telemetry.StartMetricsServer(readiness); err != nil {
		shutdownTelemetry(telemetry, cfg.Observability.ShutdownTimeoutSeconds)
		return err
	}
	defer shutdownTelemetry(telemetry, cfg.Observability.ShutdownTimeoutSeconds)

	dependencies, err := router.BuildDependencies(pool, cfg, apiKeyLimiter, requestLimiter)
	if err != nil {
		return fmt.Errorf("compose API dependencies: %w", err)
	}
	dependencies.HealthCheck = readiness
	r, err := router.NewRouterWithDependencies(cfg, dependencies)
	if err != nil {
		return fmt.Errorf("compose API router: %w", err)
	}

	// Create HTTP server.
	addr := fmt.Sprintf(":%d", cfg.App.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           telemetry.Metrics().HTTPMiddleware(r),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on public API address %s: %w", addr, err)
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Info().Int("port", cfg.App.Port).Str("instance_id", instanceID).Msg("starting REST API server")
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serverErrors <- serveErr
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		log.Info().Msg("API shutdown requested")
	case runErr = <-serverErrors:
		runErr = fmt.Errorf("public API server: %w", runErr)
	case runErr = <-telemetry.Errors():
	}
	telemetry.SetDraining()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutdown public API: %w", err))
	}
	log.Info().Msg("server stopped gracefully")
	return runErr
}

func shutdownTelemetry(runtime *observability.Runtime, timeoutSeconds int) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("observability shutdown failed")
	}
}
