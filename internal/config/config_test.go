package config

import (
	"strings"
	"testing"
)

func TestLoadUsesDocumentedEnvironmentContract(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "8181")
	t.Setenv("DATABASE_URL", "postgres://app:secret@database:5432/grc?sslmode=disable")
	t.Setenv("JWT_SECRET", "test-only-secret")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Port != 8181 {
		t.Fatalf("App.Port = %d, want 8181", cfg.App.Port)
	}
	if got := cfg.DatabaseDSN(); got != "postgres://app:secret@database:5432/grc?sslmode=disable" {
		t.Fatalf("DatabaseDSN() = %q", got)
	}
	if len(cfg.CORS.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins = %#v, want two origins", cfg.CORS.AllowedOrigins)
	}
}

func TestLoadPrefersCanonicalNameOverCompatibilityAlias(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "8181")
	t.Setenv("CF_APP_PORT", "8282")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.App.Port != 8181 {
		t.Fatalf("App.Port = %d, want canonical APP_PORT value 8181", cfg.App.Port)
	}
}

func TestValidateRejectsInsecureProductionDefaults(t *testing.T) {
	cfg := validConfig()
	cfg.App.Env = "production"
	cfg.JWT.Secret = "change-me-in-production"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("Validate() error = %v, want JWT_SECRET error", err)
	}
}

func TestValidateAcceptsHardenedProductionConfig(t *testing.T) {
	cfg := validConfig()
	cfg.App.Env = "production"
	cfg.Database.URL = "postgres://app:super-secret@database.example.com:5432/grc?sslmode=verify-full"
	cfg.Storage.Type = "s3"
	cfg.Storage.S3Bucket = "grc-production"
	cfg.Storage.S3Region = "eu-west-2"
	cfg.Redis.URL = "rediss://app:super-secret@redis.example.com:6379/0"
	cfg.RabbitMQ.URL = "amqps://app:super-secret@rabbitmq.example.com:5671/grc"
	cfg.CORS.AllowedOrigins = []string{"https://app.example.com"}
	cfg.JWT.Secret = "0123456789abcdef0123456789abcdef"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func validConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:     "ComplianceForge",
			Env:      "test",
			Port:     8080,
			GRPCPort: 9090,
		},
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "postgres",
			DBName:   "complianceforge",
			SSLMode:  "disable",
			MaxConns: 25,
			MinConns: 5,
		},
		Redis:    RedisConfig{Host: "localhost", Port: 6379},
		RabbitMQ: RabbitMQConfig{URL: "amqp://guest:guest@localhost:5672/"},
		JWT: JWTConfig{
			Secret:      "test-only-secret",
			Issuer:      "complianceforge",
			ExpiryHours: 24,
		},
		Storage: StorageConfig{Type: "local", Path: "./storage"},
		CORS:    CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}},
		RateLimit: RateLimitConfig{
			RPS: 100,
		},
	}
}
