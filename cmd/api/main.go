package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	"github.com/complianceforge/platform/internal/service"
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
	if err := enforceProductionAPIDatabasePosture(ctx, cfg.App.Env, func(checkCtx context.Context) error {
		if err := database.ValidateRuntimeDatabaseLoginIdentity(checkCtx, pool, pool.Config().ConnConfig.User); err != nil {
			return err
		}
		return database.ValidateAPIDatabasePosture(checkCtx, pool)
	}); err != nil {
		return err
	}

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
	diagnosticsHandler, err := router.BuildDiagnosticsHandler(pool, cfg,
		service.DependencyProbe{Key: "postgres", Name: "PostgreSQL", Critical: true, Check: postgresHealth},
		service.DependencyProbe{Key: "redis", Name: "Redis", Critical: true, Check: redisHealth},
		service.DependencyProbe{Key: "evidence_scanner", Name: "Evidence malware scanner", Critical: false, Check: dependencies.EvidenceScannerCheck},
	)
	if err != nil {
		return fmt.Errorf("compose administrator diagnostics: %w", err)
	}
	dependencies.Diagnostics = diagnosticsHandler
	dependencies.HealthCheck = readiness
	r, err := router.NewRouterWithDependencies(cfg, dependencies)
	if err != nil {
		return fmt.Errorf("compose API router: %w", err)
	}

	// Create HTTP server.
	addr := fmt.Sprintf(":%d", cfg.App.Port)
	srv := newPublicHTTPServer(addr, telemetry.Metrics().HTTPMiddleware(r), cfg.HTTP)

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.HTTP.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutdown public API: %w", err))
	}
	log.Info().Msg("server stopped gracefully")
	return runErr
}

func enforceProductionAPIDatabasePosture(
	ctx context.Context,
	environment string,
	check func(context.Context) error,
) error {
	if strings.ToLower(strings.TrimSpace(environment)) != "production" {
		return nil
	}
	if check == nil {
		return fmt.Errorf("validate production API database identity: posture check is not configured")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := check(checkCtx); err != nil {
		return fmt.Errorf("validate production API database identity: %w", err)
	}
	return nil
}

func newPublicHTTPServer(addr string, next http.Handler, configured config.HTTPConfig) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           next,
		ReadHeaderTimeout: time.Duration(configured.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(configured.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(configured.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(configured.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func shutdownTelemetry(runtime *observability.Runtime, timeoutSeconds int) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("observability shutdown failed")
	}
}
