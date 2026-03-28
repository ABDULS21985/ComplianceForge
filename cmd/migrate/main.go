package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
)

func main() {
	// Accept command-line flags.
	direction := flag.String("direction", "up", "migration direction: up or down")
	steps := flag.Int("steps", 0, "number of migration steps (0 = all)")
	flag.Parse()

	// Load application configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	// Initialize zerolog with console output for CLI usage.
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Build DSN from config.
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	// Create migrate instance.
	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create migrate instance")
	}
	defer m.Close()

	// Run migrations based on direction and steps.
	switch *direction {
	case "up":
		if *steps > 0 {
			err = m.Steps(*steps)
		} else {
			err = m.Up()
		}
	case "down":
		if *steps > 0 {
			err = m.Steps(-*steps)
		} else {
			err = m.Down()
		}
	default:
		log.Fatal().Str("direction", *direction).Msg("invalid direction: must be 'up' or 'down'")
	}

	if err != nil && err != migrate.ErrNoChange {
		log.Fatal().Err(err).Str("direction", *direction).Int("steps", *steps).Msg("migration failed")
	}

	if err == migrate.ErrNoChange {
		log.Info().Msg("no new migrations to apply")
	} else {
		log.Info().Str("direction", *direction).Int("steps", *steps).Msg("migrations applied successfully")
	}

	// Log current version.
	version, dirty, verr := m.Version()
	if verr != nil && verr != migrate.ErrNoChange {
		log.Warn().Err(verr).Msg("could not determine migration version")
	} else {
		log.Info().Uint("version", version).Bool("dirty", dirty).Msg("current migration state")
	}
}
