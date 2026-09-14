package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
)

const seedLockID int64 = 0x434653454544 // "CFSEED"

type options struct {
	seedDir      string
	manifestPath string
}

type seedRecord struct {
	name     string
	checksum string
	position int
}

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		log.Fatal().Err(err).Msg("invalid seed arguments")
	}
	seedDir, manifestPath, err := resolveSeedPaths(opts)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to locate seed files")
	}
	seedNames, err := readManifest(manifestPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read seed manifest")
	}
	dsn, err := databaseDSN()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to determine database URL")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			log.Warn().Err(closeErr).Msg("failed to close seed database connection")
		}
	}()

	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", seedLockID); err != nil {
		log.Fatal().Err(err).Msg("failed to acquire bootstrap seed lock")
	}
	defer func() {
		if _, unlockErr := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", seedLockID); unlockErr != nil {
			log.Warn().Err(unlockErr).Msg("failed to release bootstrap seed lock")
		}
	}()

	if err = validateHistory(ctx, conn, seedNames); err != nil {
		log.Fatal().Err(err).Msg("seed history validation failed; run all migrations before seeding")
	}

	applied := 0
	skipped := 0
	for index, seedName := range seedNames {
		seedPath := filepath.Join(seedDir, seedName)
		contents, readErr := os.ReadFile(seedPath)
		if readErr != nil {
			log.Fatal().Err(readErr).Str("seed", seedName).Msg("failed to read seed file")
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(contents))
		position := index + 1

		alreadyApplied, historyErr := checkHistory(ctx, conn, seedRecord{
			name: seedName, checksum: checksum, position: position,
		})
		if historyErr != nil {
			log.Fatal().Err(historyErr).Str("seed", seedName).Msg("seed history mismatch")
		}
		if alreadyApplied {
			skipped++
			log.Info().Str("seed", seedName).Msg("seed already applied")
			continue
		}

		seedSQL, unwrapErr := unwrapTransaction(contents)
		if unwrapErr != nil {
			log.Fatal().Err(unwrapErr).Str("seed", seedName).Msg("invalid seed file")
		}
		if applyErr := applySeed(ctx, conn, seedRecord{
			name: seedName, checksum: checksum, position: position,
		}, seedSQL); applyErr != nil {
			log.Fatal().Err(applyErr).Str("seed", seedName).Msg("failed to apply seed")
		}
		applied++
		log.Info().Str("seed", seedName).Msg("seed applied")
	}

	log.Info().Int("applied", applied).Int("skipped", skipped).Int("total", len(seedNames)).Msg("bootstrap seeds complete")
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	flags.StringVar(&opts.seedDir, "dir", "", "seed directory (default: SEEDS_PATH, sql/seeds, or seeds)")
	flags.StringVar(&opts.manifestPath, "manifest", "", "seed manifest (default: <seed-dir>/manifest.txt)")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if len(flags.Args()) != 0 {
		return options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	return opts, nil
}

func resolveSeedPaths(opts options) (string, string, error) {
	candidates := make([]string, 0, 2)
	if opts.seedDir != "" {
		candidates = append(candidates, opts.seedDir)
	} else if envPath := strings.TrimSpace(os.Getenv("SEEDS_PATH")); envPath != "" {
		candidates = append(candidates, envPath)
	} else {
		candidates = append(candidates, "sql/seeds", "seeds")
	}

	var seedDir string
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			seedDir, err = filepath.Abs(candidate)
			if err != nil {
				return "", "", fmt.Errorf("resolve seed directory %q: %w", candidate, err)
			}
			break
		}
	}
	if seedDir == "" {
		return "", "", fmt.Errorf("no seed directory found (checked %s)", strings.Join(candidates, ", "))
	}

	manifestPath := opts.manifestPath
	if manifestPath == "" {
		manifestPath = filepath.Join(seedDir, "manifest.txt")
	} else if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join(seedDir, manifestPath)
	}
	info, err := os.Stat(manifestPath)
	if err != nil || info.IsDir() {
		return "", "", fmt.Errorf("seed manifest not found: %s", manifestPath)
	}
	return seedDir, manifestPath, nil
}

func readManifest(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var names []string
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		if filepath.Base(name) != name || filepath.Ext(name) != ".sql" {
			return nil, fmt.Errorf("manifest entry must be a plain .sql filename, got %q", name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate manifest entry %q", name)
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errors.New("seed manifest is empty")
	}
	return names, nil
}

func unwrapTransaction(contents []byte) (string, error) {
	lines := strings.Split(string(contents), "\n")
	first, last := -1, -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		if first == -1 {
			first = index
		}
		last = index
	}
	if first == -1 || !strings.EqualFold(strings.TrimSpace(lines[first]), "BEGIN;") {
		return "", errors.New("seed must start with BEGIN;")
	}
	if last == first || !strings.EqualFold(strings.TrimSpace(lines[last]), "COMMIT;") {
		return "", errors.New("seed must end with COMMIT;")
	}
	lines[first] = ""
	lines[last] = ""
	return strings.Join(lines, "\n"), nil
}

func validateHistory(ctx context.Context, conn *pgx.Conn, manifest []string) error {
	rows, err := conn.Query(ctx, `SELECT seed_name, position FROM bootstrap_seed_history ORDER BY position`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var position int
		if err := rows.Scan(&name, &position); err != nil {
			return err
		}
		if position < 1 || position > len(manifest) || manifest[position-1] != name {
			return fmt.Errorf("applied seed %q at position %d does not match the current manifest", name, position)
		}
	}
	return rows.Err()
}

func checkHistory(ctx context.Context, conn *pgx.Conn, expected seedRecord) (bool, error) {
	var checksum string
	var position int
	err := conn.QueryRow(ctx,
		`SELECT BTRIM(checksum), position FROM bootstrap_seed_history WHERE seed_name = $1`,
		expected.name,
	).Scan(&checksum, &position)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if checksum != expected.checksum {
		return false, fmt.Errorf("checksum changed after application (database %s, file %s)", checksum, expected.checksum)
	}
	if position != expected.position {
		return false, fmt.Errorf("manifest position changed after application (database %d, manifest %d)", position, expected.position)
	}
	return true, nil
}

func applySeed(ctx context.Context, conn *pgx.Conn, seed seedRecord, seedSQL string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, seedSQL, pgx.QueryExecModeSimpleProtocol); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO bootstrap_seed_history (seed_name, checksum, position) VALUES ($1, $2, $3)`,
		seed.name, seed.checksum, seed.position,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func databaseDSN() (string, error) {
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
