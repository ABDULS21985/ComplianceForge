package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

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

	// Create gRPC server.
	srv := grpc.NewServer()

	// TODO: Register gRPC services here.
	// Example:
	//   pb.RegisterComplianceServiceServer(srv, compliance.NewService(pool, cfg))
	//   pb.RegisterRiskServiceServer(srv, risk.NewService(pool, cfg))
	_ = pool // remove once services are registered

	// Listen on the configured gRPC port.
	grpcAddr := fmt.Sprintf(":%d", cfg.App.GRPCPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatal().Err(err).Int("port", cfg.App.GRPCPort).Msg("failed to listen")
	}

	// Start serving in a goroutine.
	go func() {
		log.Info().Int("port", cfg.App.GRPCPort).Msg("starting gRPC server")
		if err := srv.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("gRPC server failed")
		}
	}()

	// Graceful stop on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("shutting down gRPC server")

	srv.GracefulStop()

	log.Info().Msg("gRPC server stopped gracefully")
}
