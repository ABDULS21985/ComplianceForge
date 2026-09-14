package router

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/config"
)

func TestRequiredDependenciesComposeAgainstMigratedPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "router-integration-test-secret-with-sufficient-entropy",
			Issuer: "complianceforge-test", ExpiryHours: 1,
		},
		Encryption: config.EncryptionConfig{
			IntegrationKey:  strings.Repeat("ab", 32),
			NotificationKey: strings.Repeat("cd", 32),
		},
		SMTP: config.SMTPConfig{
			Host: "127.0.0.1", Port: 2525, From: "noreply@example.test",
			TLSMode: "disabled", TimeoutSeconds: 1,
		},
		CORS:      config.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}},
		RateLimit: config.RateLimitConfig{RPS: 100},
	}
	dependencies, err := BuildDependencies(pool, cfg)
	if err != nil {
		t.Fatalf("BuildDependencies() error = %v", err)
	}
	if err := dependencies.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if dependencies.Policies == nil || dependencies.Notifications == nil || dependencies.Integrations == nil {
		t.Fatal("required enterprise handlers were not composed")
	}
	if _, err := NewRouterWithDependencies(cfg, dependencies); err != nil {
		t.Fatalf("NewRouterWithDependencies() error = %v", err)
	}
}
