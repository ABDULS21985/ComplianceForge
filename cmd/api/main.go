package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
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

	// Create PostgreSQL connection pool.
	pool, err := database.NewPostgresPool(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create database pool")
	}
	defer pool.Close()

	// Shared Redis is required for API-key quotas and participates in readiness.
	redisClient, err := database.NewRedisClient(context.Background(), cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Redis client")
	}
	defer redisClient.Close()
	apiKeyLimiter, err := ratelimit.NewRedisAPIKeyLimiter(redisClient)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create API-key rate limiter")
	}
	requestLimiter, err := ratelimit.NewRedisRequestLimiter(redisClient)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create request rate limiter")
	}

	dependencies, err := router.BuildDependencies(pool, cfg, apiKeyLimiter, requestLimiter)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to compose API dependencies")
	}
	dependencies.HealthCheck = func(ctx context.Context) error {
		return errors.Join(
			database.HealthCheck(ctx, pool),
			database.RedisHealthCheck(ctx, redisClient),
		)
	}
	r, err := router.NewRouterWithDependencies(cfg, dependencies)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to compose API router")
	}

	// Create HTTP server.
	addr := fmt.Sprintf(":%d", cfg.App.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// Start server in a goroutine.
	go func() {
		log.Info().Int("port", cfg.App.Port).Msg("starting REST API server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	// Graceful shutdown: listen for SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server stopped gracefully")
}
