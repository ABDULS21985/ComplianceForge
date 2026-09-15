//go:build integration

package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestValidateAPIDatabasePostureLive exercises the complete catalog query and
// pgx scan contract against PostgreSQL. The role is assumed only inside a
// read-only transaction so a privileged validation URL never becomes the
// runtime identity under test.
func TestValidateAPIDatabasePostureLive(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	roleName := strings.TrimSpace(os.Getenv("API_POSTURE_TEST_ROLE"))
	if databaseURL == "" || roleName == "" {
		t.Skip("TEST_DATABASE_URL and API_POSTURE_TEST_ROLE are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	transaction, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	if _, err := transaction.Exec(ctx, `SET TRANSACTION READ ONLY`); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{roleName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPIDatabasePosture(ctx, transaction); err != nil {
		t.Fatalf("ValidateAPIDatabasePosture() role %q: %v", roleName, err)
	}
}
