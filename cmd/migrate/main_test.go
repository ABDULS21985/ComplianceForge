package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		direction string
		steps     int
		wantErr   bool
	}{
		{name: "defaults to all up", direction: "up"},
		{name: "up subcommand", args: []string{"up"}, direction: "up"},
		{name: "down subcommand with positional step", args: []string{"down", "1"}, direction: "down", steps: 1},
		{name: "subcommand with step flag", args: []string{"up", "-steps", "3"}, direction: "up", steps: 3},
		{name: "legacy flags", args: []string{"-direction", "down", "-steps", "2"}, direction: "down", steps: 2},
		{name: "reject invalid direction", args: []string{"-direction", "sideways"}, wantErr: true},
		{name: "reject negative steps", args: []string{"down", "-1"}, wantErr: true},
		{name: "reject duplicate step forms", args: []string{"down", "-steps", "1", "2"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseOptions(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOptions() error = %v", err)
			}
			if opts.direction != tt.direction || opts.steps != tt.steps {
				t.Fatalf("parseOptions() = direction %q, steps %d; want %q, %d", opts.direction, opts.steps, tt.direction, tt.steps)
			}
		})
	}
}

func TestResolveMigrationSource(t *testing.T) {
	dir := t.TempDir()
	source, err := resolveMigrationSource(dir)
	if err != nil {
		t.Fatalf("resolveMigrationSource() error = %v", err)
	}
	absolutePath, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(source, "file://") || !strings.Contains(source, filepath.ToSlash(absolutePath)) {
		t.Fatalf("resolveMigrationSource() = %q, want a file URL containing %q", source, absolutePath)
	}
}

func TestDatabaseDSNPrefersEnvironment(t *testing.T) {
	const want = "postgres://test:test@localhost:5432/test?sslmode=disable"
	t.Setenv("DATABASE_URL", want)
	got, err := databaseDSN()
	if err != nil {
		t.Fatalf("databaseDSN() error = %v", err)
	}
	if got != want {
		t.Fatalf("databaseDSN() = %q, want %q", got, want)
	}
}

func TestResolveMigrationSourceUsesEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MIGRATIONS_PATH", dir)
	if _, err := resolveMigrationSource(""); err != nil {
		t.Fatalf("resolveMigrationSource() error = %v", err)
	}
}

func TestResolveMigrationSourceCanonicalizesSymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "migrations")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	source, err := resolveMigrationSource(link)
	if err != nil {
		t.Fatalf("resolveMigrationSource() error = %v", err)
	}
	if strings.Contains(source, filepath.ToSlash(link)) || !strings.Contains(source, filepath.ToSlash(target)) {
		t.Fatalf("resolveMigrationSource() = %q, want canonical target %q", source, target)
	}
}
