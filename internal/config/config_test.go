package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadUsesDocumentedEnvironmentContract(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "8181")
	t.Setenv("HTTP_READ_TIMEOUT_SECONDS", "90")
	t.Setenv("HTTP_WRITE_TIMEOUT_SECONDS", "180")
	t.Setenv("DATABASE_URL", "postgres://app:secret@database:5432/grc?sslmode=disable")
	t.Setenv("JWT_SECRET", "test-only-secret")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")
	t.Setenv("EVIDENCE_MAXIMUM_UPLOAD_BYTES", "10485760")
	t.Setenv("EVIDENCE_SCANNER_ADDRESS", "scanner.internal:3310")
	t.Setenv("EVIDENCE_SIGNED_DOWNLOAD_SECONDS", "120")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Port != 8181 {
		t.Fatalf("App.Port = %d, want 8181", cfg.App.Port)
	}
	if cfg.HTTP.ReadTimeoutSeconds != 90 || cfg.HTTP.WriteTimeoutSeconds != 180 {
		t.Fatalf("HTTP config = %#v", cfg.HTTP)
	}
	if got := cfg.DatabaseDSN(); got != "postgres://app:secret@database:5432/grc?sslmode=disable" {
		t.Fatalf("DatabaseDSN() = %q", got)
	}
	if len(cfg.CORS.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins = %#v, want two origins", cfg.CORS.AllowedOrigins)
	}
	if cfg.Evidence.MaximumUploadBytes != 10<<20 || cfg.Evidence.ScannerAddress != "scanner.internal:3310" || cfg.Evidence.SignedDownloadSeconds != 120 {
		t.Fatalf("Evidence config = %#v", cfg.Evidence)
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
	cfg.Encryption.DSRKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cfg.Encryption.IntegrationKey = strings.Repeat("ab", 32)
	cfg.Encryption.IdentityKey = strings.Repeat("ef", 32)
	cfg.SMTP.TLSMode = "starttls"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsCredentialedCORSWildcardAndNonOrigins(t *testing.T) {
	tests := []struct {
		name    string
		origins []string
	}{
		{name: "wildcard", origins: []string{"*"}},
		{name: "path", origins: []string{"https://app.example.com/login"}},
		{name: "query", origins: []string{"https://app.example.com?tenant=a"}},
		{name: "credentials", origins: []string{"https://user:password@app.example.com"}},
		{name: "duplicates", origins: []string{"https://app.example.com", "https://app.example.com/"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.CORS.AllowedOrigins = test.origins
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() accepted origins %#v", test.origins)
			}
		})
	}
}

func TestValidateCanonicalizesCORSOriginTrailingSlash(t *testing.T) {
	cfg := validConfig()
	cfg.CORS.AllowedOrigins = []string{"http://localhost:3000/"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := cfg.CORS.AllowedOrigins[0]; got != "http://localhost:3000" {
		t.Fatalf("origin = %q", got)
	}
}

func TestValidateObservabilityRequiresSeparateBoundedListener(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ObservabilityConfig)
	}{
		{name: "public port reuse", mutate: func(c *ObservabilityConfig) { c.MetricsAddress = "0.0.0.0:8080" }},
		{name: "invalid address", mutate: func(c *ObservabilityConfig) { c.MetricsAddress = "all-interfaces" }},
		{name: "invalid sample ratio", mutate: func(c *ObservabilityConfig) { c.TraceSampleRatio = 1.1 }},
		{name: "missing trace endpoint", mutate: func(c *ObservabilityConfig) { c.TracingEnabled = true; c.OTLPTraceEndpoint = "" }},
		{name: "credentialed trace endpoint", mutate: func(c *ObservabilityConfig) {
			c.TracingEnabled = true
			c.OTLPTraceEndpoint = "https://user:secret@collector.example.com/v1/traces"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg.Observability)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe observability configuration")
			}
		})
	}
}

func TestValidateAcceptsOTLPTraceEndpoint(t *testing.T) {
	cfg := validConfig()
	cfg.Observability.TracingEnabled = true
	cfg.Observability.OTLPTraceEndpoint = "http://otel-collector:4318/v1/traces"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsUnsafeSMTPConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SMTPConfig)
	}{
		{name: "plaintext production SMTP", mutate: func(c *SMTPConfig) { c.TLSMode = "disabled" }},
		{name: "partial credentials", mutate: func(c *SMTPConfig) { c.User = "mailer" }},
		{name: "header injection", mutate: func(c *SMTPConfig) { c.From = "safe@example.com\r\nBcc: stolen@example.com" }},
		{name: "unbounded timeout", mutate: func(c *SMTPConfig) { c.TimeoutSeconds = 121 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.App.Env = "production"
			cfg.Database.URL = "postgres://app:super-secret@database.example.com:5432/grc?sslmode=verify-full"
			cfg.Redis.URL = "rediss://app:super-secret@redis.example.com:6379/0"
			cfg.RabbitMQ.URL = "amqps://app:super-secret@rabbitmq.example.com:5671/grc"
			cfg.JWT.Secret = "0123456789abcdef0123456789abcdef"
			cfg.Encryption.DSRKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
			cfg.Encryption.IntegrationKey = strings.Repeat("ab", 32)
			cfg.Encryption.IdentityKey = strings.Repeat("ef", 32)
			cfg.Storage = StorageConfig{Type: "s3", S3Bucket: "test", S3Region: "eu-west-2"}
			cfg.CORS.AllowedOrigins = []string{"https://app.example.com"}
			test.mutate(&cfg.SMTP)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe SMTP configuration")
			}
		})
	}
}

func TestValidateIdentityWebAuthnBoundary(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		rpID        string
		origins     []string
		valid       bool
	}{
		{name: "https subdomain", environment: "production", rpID: "example.com", origins: []string{"https://app.example.com"}, valid: true},
		{name: "localhost development", environment: "development", rpID: "localhost", origins: []string{"http://localhost:3000"}, valid: true},
		{name: "http production", environment: "production", rpID: "example.com", origins: []string{"http://app.example.com"}},
		{name: "foreign origin", environment: "test", rpID: "example.com", origins: []string{"https://example.net"}},
		{name: "credentialed origin", environment: "test", rpID: "example.com", origins: []string{"https://user:pass@example.com"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.App.Env = test.environment
			cfg.Identity.RPID = test.rpID
			cfg.Identity.RPOrigins = test.origins
			if test.environment == "production" {
				cfg.Database.URL = "postgres://app:super-secret@database.example.com:5432/grc?sslmode=verify-full"
				cfg.Storage = StorageConfig{Type: "s3", S3Bucket: "test", S3Region: "eu-west-2"}
				cfg.Redis.URL = "rediss://app:secret@redis.example.com:6379/0"
				cfg.RabbitMQ.URL = "amqps://app:secret@rabbitmq.example.com:5671/grc"
				cfg.JWT.Secret = strings.Repeat("j", 32)
				cfg.Encryption.DSRKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
				cfg.Encryption.IntegrationKey = strings.Repeat("ab", 32)
				cfg.Encryption.NotificationKey = strings.Repeat("cd", 32)
				cfg.Encryption.IdentityKey = strings.Repeat("ef", 32)
				cfg.CORS.AllowedOrigins = []string{"https://app.example.com"}
			}
			err := cfg.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("Validate() error = %v, valid=%v", err, test.valid)
			}
		})
	}
}

func TestValidateEvidenceSecurityBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvidenceConfig)
	}{
		{name: "tiny upload limit", mutate: func(c *EvidenceConfig) { c.MaximumUploadBytes = 1 }},
		{name: "unbounded upload limit", mutate: func(c *EvidenceConfig) { c.MaximumUploadBytes = 3 << 30 }},
		{name: "unsupported scanner network", mutate: func(c *EvidenceConfig) { c.ScannerNetwork = "udp" }},
		{name: "invalid tcp address", mutate: func(c *EvidenceConfig) { c.ScannerAddress = "scanner" }},
		{name: "relative unix socket", mutate: func(c *EvidenceConfig) { c.ScannerNetwork = "unix"; c.ScannerAddress = "clamd.sock" }},
		{name: "unbounded scan timeout", mutate: func(c *EvidenceConfig) { c.ScannerTimeoutSeconds = 301 }},
		{name: "unbounded signed URL", mutate: func(c *EvidenceConfig) { c.SignedDownloadSeconds = 901 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg.Evidence)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe evidence configuration")
			}
		})
	}

	cfg := validConfig()
	cfg.Evidence.ScannerNetwork = "unix"
	cfg.Evidence.ScannerAddress = "/run/clamav/clamd.sock"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected unix scanner: %v", err)
	}
}

func TestValidateHTTPServerDeadlines(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "zero header deadline", mutate: func(c *Config) { c.HTTP.ReadHeaderTimeoutSeconds = 0 }},
		{name: "unbounded read deadline", mutate: func(c *Config) { c.HTTP.ReadTimeoutSeconds = 3601 }},
		{name: "write cannot cover scan", mutate: func(c *Config) {
			c.HTTP.WriteTimeoutSeconds = c.HTTP.ReadTimeoutSeconds + c.Evidence.ScannerTimeoutSeconds - 1
		}},
		{name: "unbounded idle deadline", mutate: func(c *Config) { c.HTTP.IdleTimeoutSeconds = 601 }},
		{name: "tiny shutdown budget", mutate: func(c *Config) { c.HTTP.ShutdownTimeoutSeconds = 4 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe HTTP deadline configuration")
			}
		})
	}
}

func TestValidateS3SecurityBoundary(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		endpoint    string
		owner       string
	}{
		{name: "plaintext production endpoint", environment: "production", endpoint: "http://minio.internal:9000"},
		{name: "credentialed endpoint", endpoint: "https://user:pass@s3.example.com"},
		{name: "endpoint path", endpoint: "https://s3.example.com/bucket"},
		{name: "invalid expected owner", owner: "1234"},
		{name: "nondigit expected owner", owner: "12345678901x"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.App.Env = test.environment
			cfg.Storage = StorageConfig{Type: "s3", S3Bucket: "evidence", S3Region: "eu-west-2", S3Endpoint: test.endpoint, S3ExpectedBucketOwner: test.owner}
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe S3 configuration")
			}
		})
	}

	cfg := validConfig()
	cfg.Storage = StorageConfig{Type: "s3", S3Bucket: "evidence", S3Region: "eu-west-2", S3Endpoint: "http://127.0.0.1:9000", S3ForcePathStyle: true, S3ExpectedBucketOwner: "123456789012"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected development S3 endpoint: %v", err)
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
		HTTP: HTTPConfig{
			ReadHeaderTimeoutSeconds: 5,
			ReadTimeoutSeconds:       120,
			WriteTimeoutSeconds:      360,
			IdleTimeoutSeconds:       60,
			ShutdownTimeoutSeconds:   30,
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
		Encryption: EncryptionConfig{NotificationKey: strings.Repeat("cd", 32)},
		Identity:   IdentityConfig{RPID: "example.com", RPDisplayName: "ComplianceForge", RPOrigins: []string{"https://app.example.com"}},
		SMTP: SMTPConfig{
			Host:           "localhost",
			Port:           1025,
			From:           "ComplianceForge <noreply@complianceforge.local>",
			TLSMode:        "starttls",
			TimeoutSeconds: 15,
		},
		Storage: StorageConfig{Type: "local", Path: "./storage"},
		Evidence: EvidenceConfig{
			MaximumUploadBytes:    25 << 20,
			ScannerNetwork:        "tcp",
			ScannerAddress:        "127.0.0.1:3310",
			ScannerTimeoutSeconds: 30,
			SignedDownloadSeconds: 300,
		},
		Observability: ObservabilityConfig{
			MetricsEnabled:         true,
			MetricsAddress:         "127.0.0.1:9091",
			MetricsTokenFile:       "/run/secrets/metrics_token",
			TraceSampleRatio:       0.1,
			ServiceVersion:         "test",
			ShutdownTimeoutSeconds: 10,
		},
		CORS: CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}},
		RateLimit: RateLimitConfig{
			RPS: 100,
		},
	}
}
