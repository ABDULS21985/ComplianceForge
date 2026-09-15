package main

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
)

type options struct {
	direction      string
	steps          int
	migrationsPath string
}

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		log.Fatal().Err(err).Msg("invalid migration arguments")
	}

	dsn, err := databaseDSN()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to determine database URL")
	}

	migrationSource, err := resolveMigrationSource(opts.migrationsPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to locate migrations")
	}

	m, err := migrate.New(migrationSource, dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create migrate instance")
	}
	defer func() {
		if sourceErr, databaseErr := m.Close(); sourceErr != nil || databaseErr != nil {
			log.Warn().Err(sourceErr).AnErr("database_error", databaseErr).Msg("failed to close migrator cleanly")
		}
	}()

	if opts.direction == "up" {
		if opts.steps > 0 {
			err = m.Steps(opts.steps)
		} else {
			err = m.Up()
		}
	} else {
		if opts.steps > 0 {
			err = m.Steps(-opts.steps)
		} else {
			err = m.Down()
		}
	}

	if err != nil && err != migrate.ErrNoChange {
		log.Fatal().Err(err).Str("direction", opts.direction).Int("steps", opts.steps).Msg("migration failed")
	}
	if err == migrate.ErrNoChange {
		log.Info().Msg("no migration changes to apply")
	} else {
		log.Info().Str("direction", opts.direction).Int("steps", opts.steps).Msg("migrations applied successfully")
	}

	version, dirty, versionErr := m.Version()
	if versionErr != nil && versionErr != migrate.ErrNilVersion {
		log.Warn().Err(versionErr).Msg("could not determine migration version")
		return
	}
	if versionErr == migrate.ErrNilVersion {
		log.Info().Msg("database is at the initial migration state")
		return
	}
	log.Info().Uint("version", version).Bool("dirty", dirty).Msg("current migration state")
}

// parseOptions accepts both the documented subcommand form (`migrate up`) and
// the legacy flag form (`migrate -direction=up`). A positional step count is
// also supported, for example `migrate down 1`.
func parseOptions(args []string) (options, error) {
	opts := options{direction: "up"}
	if len(args) > 0 && (args[0] == "up" || args[0] == "down") {
		opts.direction = args[0]
		args = args[1:]
	}

	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.StringVar(&opts.direction, "direction", opts.direction, "migration direction: up or down")
	flags.IntVar(&opts.steps, "steps", 0, "number of migration steps (0 = all)")
	flags.StringVar(&opts.migrationsPath, "path", "", "migration directory (default: MIGRATIONS_PATH, sql/migrations, or migrations)")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}

	remaining := flags.Args()
	if len(remaining) > 1 {
		return options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(remaining, " "))
	}
	if len(remaining) == 1 {
		if opts.steps != 0 {
			return options{}, fmt.Errorf("step count was supplied both positionally and with -steps")
		}
		steps, err := strconv.Atoi(remaining[0])
		if err != nil {
			return options{}, fmt.Errorf("invalid step count %q: %w", remaining[0], err)
		}
		opts.steps = steps
	}

	if opts.direction != "up" && opts.direction != "down" {
		return options{}, fmt.Errorf("direction must be up or down, got %q", opts.direction)
	}
	if opts.steps < 0 {
		return options{}, fmt.Errorf("steps must be zero or greater")
	}
	return opts, nil
}

func databaseDSN() (string, error) {
	if dsn := strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URL")); dsn != "" {
		return dsn, nil
	}
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		return dsn, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.Database.User, cfg.Database.Password),
		Host:   net.JoinHostPort(cfg.Database.Host, strconv.Itoa(cfg.Database.Port)),
		Path:   cfg.Database.DBName,
	}
	query := dsn.Query()
	query.Set("sslmode", cfg.Database.SSLMode)
	dsn.RawQuery = query.Encode()
	return dsn.String(), nil
}

func resolveMigrationSource(configuredPath string) (string, error) {
	candidates := make([]string, 0, 3)
	if configuredPath != "" {
		candidates = append(candidates, configuredPath)
	} else if envPath := strings.TrimSpace(os.Getenv("MIGRATIONS_PATH")); envPath != "" {
		candidates = append(candidates, envPath)
	} else {
		candidates = append(candidates, "sql/migrations", "migrations")
	}

	for _, candidate := range candidates {
		absolutePath, err := filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve migration directory %q: %w", candidate, err)
		}
		canonicalPath, err := filepath.EvalSymlinks(absolutePath)
		if err != nil {
			continue
		}
		// OpenRoot verifies that the canonical source is a directory and binds the
		// validation to a directory handle instead of a traversal-prone Stat call.
		root, err := os.OpenRoot(canonicalPath)
		if err != nil {
			continue
		}
		if closeErr := root.Close(); closeErr != nil {
			return "", fmt.Errorf("close migration directory %q: %w", candidate, closeErr)
		}
		return (&url.URL{Scheme: "file", Path: filepath.ToSlash(canonicalPath)}).String(), nil
	}

	return "", fmt.Errorf("no migration directory found (checked %s)", strings.Join(candidates, ", "))
}
