package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.txt")
	if err := os.WriteFile(path, []byte("# comment\nfirst.sql\n\nsecond.sql\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readManifest(path)
	if err != nil {
		t.Fatalf("readManifest() error = %v", err)
	}
	want := []string{"first.sql", "second.sql"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readManifest() = %#v, want %#v", got, want)
	}
}

func TestReadManifestRejectsUnsafeOrDuplicateEntries(t *testing.T) {
	for _, contents := range []string{"../outside.sql\n", "seed.txt\n", "seed.sql\nseed.sql\n"} {
		t.Run(strings.TrimSpace(contents), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.txt")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readManifest(path); err == nil {
				t.Fatal("expected manifest validation error")
			}
		})
	}
}

func TestUnwrapTransaction(t *testing.T) {
	contents := []byte("-- seed\n\nBEGIN;\nINSERT INTO example VALUES (1);\nCOMMIT;\n")
	got, err := unwrapTransaction(contents)
	if err != nil {
		t.Fatalf("unwrapTransaction() error = %v", err)
	}
	if strings.Contains(strings.ToUpper(got), "BEGIN;") || strings.Contains(strings.ToUpper(got), "COMMIT;") {
		t.Fatalf("unwrapTransaction() retained transaction wrapper: %q", got)
	}
	if !strings.Contains(got, "INSERT INTO example") {
		t.Fatalf("unwrapTransaction() lost seed SQL: %q", got)
	}
}

func TestUnwrapTransactionRejectsMissingWrapper(t *testing.T) {
	if _, err := unwrapTransaction([]byte("INSERT INTO example VALUES (1);")); err == nil {
		t.Fatal("expected transaction wrapper validation error")
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
