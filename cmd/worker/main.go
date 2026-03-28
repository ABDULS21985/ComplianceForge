// Package main starts the background worker process.
//
// The worker handles asynchronous jobs including:
//   - Email notifications (compliance alerts, report delivery)
//   - Report generation (PDF/CSV compliance reports)
//   - Compliance scoring (periodic recalculation of risk and compliance scores)
package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
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

	// TODO: Connect to RabbitMQ message queue.
	// Example:
	//   conn, err := amqp.Dial(cfg.RabbitMQURL)
	//   if err != nil {
	//       log.Fatal().Err(err).Msg("failed to connect to RabbitMQ")
	//   }
	//   defer conn.Close()
	//
	//   ch, err := conn.Channel()
	//   if err != nil {
	//       log.Fatal().Err(err).Msg("failed to open RabbitMQ channel")
	//   }
	//   defer ch.Close()

	// TODO: Start consuming from job queues.
	// Example:
	//   go worker.ConsumeEmailJobs(ch, pool, cfg)
	//   go worker.ConsumeReportJobs(ch, pool, cfg)
	//   go worker.ConsumeComplianceScoringJobs(ch, pool, cfg)
	_ = pool // remove once workers are wired up

	log.Info().Msg("starting background worker")

	// Signal handling for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("shutting down worker")
	log.Info().Msg("worker stopped gracefully")
}
