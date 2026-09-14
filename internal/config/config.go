package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

// AppConfig holds application-level settings.
type AppConfig struct {
	Name     string `mapstructure:"name"`
	Env      string `mapstructure:"env"`
	Port     int    `mapstructure:"port"`
	GRPCPort int    `mapstructure:"grpc_port"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	URL      string `mapstructure:"url"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxConns int32  `mapstructure:"max_conns"`
	MinConns int32  `mapstructure:"min_conns"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL      string `mapstructure:"url"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RabbitMQConfig holds RabbitMQ connection settings.
type RabbitMQConfig struct {
	URL string `mapstructure:"url"`
}

// JWTConfig holds JWT authentication settings.
type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	Issuer      string `mapstructure:"issuer"`
	ExpiryHours int    `mapstructure:"expiry_hours"`
}

// OAuthConfig holds OAuth2 client settings.
type OAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
}

// SMTPConfig holds email/SMTP settings.
type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
}

// StorageConfig holds file storage settings.
type StorageConfig struct {
	Type     string `mapstructure:"type"`
	Path     string `mapstructure:"path"`
	S3Bucket string `mapstructure:"s3_bucket"`
	S3Region string `mapstructure:"s3_region"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// RateLimitConfig holds rate-limiting settings.
type RateLimitConfig struct {
	RPS int `mapstructure:"rps"`
}

// Config is the root configuration struct for ComplianceForge.
type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	RabbitMQ  RabbitMQConfig  `mapstructure:"rabbitmq"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	OAuth     OAuthConfig     `mapstructure:"oauth"`
	SMTP      SMTPConfig      `mapstructure:"smtp"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Log       LogConfig       `mapstructure:"log"`
	CORS      CORSConfig      `mapstructure:"cors"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
}

// DatabaseDSN returns the PostgreSQL connection string derived from the database config.
func (c *Config) DatabaseDSN() string {
	if strings.TrimSpace(c.Database.URL) != "" {
		return c.Database.URL
	}

	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Database.Host,
		c.Database.Port,
		c.Database.User,
		c.Database.Password,
		c.Database.DBName,
		c.Database.SSLMode,
	)
}

// Load reads configuration from file, environment variables, and defaults.
// It returns the populated Config or an error.
func Load() (*Config, error) {
	v := viper.New()

	// Config file settings.
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")
	v.AddConfigPath("/etc/complianceforge")

	// Environment variables are explicitly bound so the public contract in
	// .env.example, containers, CI, and the application cannot silently drift.
	// CF_* aliases are retained for compatibility with earlier deployments.
	if err := bindEnvironment(v); err != nil {
		return nil, err
	}
	v.AutomaticEnv()

	// Sensible defaults.
	v.SetDefault("app.name", "ComplianceForge")
	v.SetDefault("app.env", "development")
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.grpc_port", 9090)

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.url", "")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.dbname", "complianceforge")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_conns", 25)
	v.SetDefault("database.min_conns", 5)

	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.url", "")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	v.SetDefault("rabbitmq.url", "amqp://guest:guest@localhost:5672/")

	v.SetDefault("jwt.secret", "change-me-in-production")
	v.SetDefault("jwt.issuer", "complianceforge")
	v.SetDefault("jwt.expiry_hours", 24)

	v.SetDefault("oauth.client_id", "")
	v.SetDefault("oauth.client_secret", "")
	v.SetDefault("oauth.redirect_url", "http://localhost:8080/auth/callback")

	v.SetDefault("smtp.host", "localhost")
	v.SetDefault("smtp.port", 587)
	v.SetDefault("smtp.user", "")
	v.SetDefault("smtp.password", "")
	v.SetDefault("smtp.from", "noreply@complianceforge.io")

	v.SetDefault("storage.type", "local")
	v.SetDefault("storage.path", "./uploads")
	v.SetDefault("storage.s3_bucket", "")
	v.SetDefault("storage.s3_region", "us-east-1")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("cors.allowed_origins", []string{"http://localhost:3000"})

	v.SetDefault("rate_limit.rps", 100)

	// Read config file (optional — env vars and defaults still work without it).
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating configuration: %w", err)
	}

	return &cfg, nil
}

// bindEnvironment defines the canonical environment contract. The first name
// for each key is the documented name; later names are backwards-compatible
// aliases and should not be introduced into new deployments.
func bindEnvironment(v *viper.Viper) error {
	bindings := map[string][]string{
		"app.name":             {"APP_NAME", "CF_APP_NAME"},
		"app.env":              {"APP_ENV", "CF_APP_ENV"},
		"app.port":             {"APP_PORT", "PORT", "CF_APP_PORT"},
		"app.grpc_port":        {"APP_GRPC_PORT", "CF_APP_GRPC_PORT"},
		"database.url":         {"DATABASE_URL", "CF_DATABASE_URL"},
		"database.host":        {"DB_HOST", "CF_DATABASE_HOST"},
		"database.port":        {"DB_PORT", "CF_DATABASE_PORT"},
		"database.user":        {"DB_USER", "CF_DATABASE_USER"},
		"database.password":    {"DB_PASSWORD", "CF_DATABASE_PASSWORD"},
		"database.dbname":      {"DB_NAME", "CF_DATABASE_DBNAME"},
		"database.sslmode":     {"DB_SSL_MODE", "CF_DATABASE_SSLMODE"},
		"database.max_conns":   {"DB_MAX_CONNS", "CF_DATABASE_MAX_CONNS"},
		"database.min_conns":   {"DB_MIN_CONNS", "CF_DATABASE_MIN_CONNS"},
		"redis.url":            {"REDIS_URL", "CF_REDIS_URL"},
		"redis.host":           {"REDIS_HOST", "CF_REDIS_HOST"},
		"redis.port":           {"REDIS_PORT", "CF_REDIS_PORT"},
		"redis.password":       {"REDIS_PASSWORD", "CF_REDIS_PASSWORD"},
		"redis.db":             {"REDIS_DB", "CF_REDIS_DB"},
		"rabbitmq.url":         {"RABBITMQ_URL", "CF_RABBITMQ_URL"},
		"jwt.secret":           {"JWT_SECRET", "CF_JWT_SECRET"},
		"jwt.issuer":           {"JWT_ISSUER", "CF_JWT_ISSUER"},
		"jwt.expiry_hours":     {"JWT_EXPIRY_HOURS", "CF_JWT_EXPIRY_HOURS"},
		"oauth.client_id":      {"OAUTH_CLIENT_ID", "CF_OAUTH_CLIENT_ID"},
		"oauth.client_secret":  {"OAUTH_CLIENT_SECRET", "CF_OAUTH_CLIENT_SECRET"},
		"oauth.redirect_url":   {"OAUTH_REDIRECT_URL", "CF_OAUTH_REDIRECT_URL"},
		"smtp.host":            {"SMTP_HOST", "CF_SMTP_HOST"},
		"smtp.port":            {"SMTP_PORT", "CF_SMTP_PORT"},
		"smtp.user":            {"SMTP_USER", "CF_SMTP_USER"},
		"smtp.password":        {"SMTP_PASSWORD", "CF_SMTP_PASSWORD"},
		"smtp.from":            {"SMTP_FROM", "CF_SMTP_FROM"},
		"storage.type":         {"STORAGE_TYPE", "CF_STORAGE_TYPE"},
		"storage.path":         {"STORAGE_PATH", "CF_STORAGE_PATH"},
		"storage.s3_bucket":    {"S3_BUCKET", "CF_STORAGE_S3_BUCKET"},
		"storage.s3_region":    {"S3_REGION", "CF_STORAGE_S3_REGION"},
		"log.level":            {"LOG_LEVEL", "CF_LOG_LEVEL"},
		"log.format":           {"LOG_FORMAT", "CF_LOG_FORMAT"},
		"cors.allowed_origins": {"CORS_ALLOWED_ORIGINS", "CF_CORS_ALLOWED_ORIGINS"},
		"rate_limit.rps":       {"RATE_LIMIT_RPS", "CF_RATE_LIMIT_RPS"},
	}

	for key, names := range bindings {
		args := append([]string{key}, names...)
		if err := v.BindEnv(args...); err != nil {
			return fmt.Errorf("binding environment variable for %s: %w", key, err)
		}
	}

	return nil
}

// Validate rejects internally inconsistent configuration in every environment
// and insecure defaults in staging and production.
func (c *Config) Validate() error {
	c.App.Env = strings.ToLower(strings.TrimSpace(c.App.Env))
	if c.App.Env == "" {
		return fmt.Errorf("APP_ENV is required")
	}
	switch c.App.Env {
	case "development", "test", "staging", "production":
	default:
		return fmt.Errorf("APP_ENV must be one of development, test, staging, or production")
	}

	if c.App.Port < 1 || c.App.Port > 65535 {
		return fmt.Errorf("APP_PORT must be between 1 and 65535")
	}
	if c.App.GRPCPort < 1 || c.App.GRPCPort > 65535 {
		return fmt.Errorf("APP_GRPC_PORT must be between 1 and 65535")
	}
	if c.App.Port == c.App.GRPCPort {
		return fmt.Errorf("APP_PORT and APP_GRPC_PORT must be different")
	}

	if c.Database.URL != "" {
		parsed, err := url.Parse(c.Database.URL)
		if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
			return fmt.Errorf("DATABASE_URL must be a valid postgres or postgresql URL")
		}
	} else {
		if strings.TrimSpace(c.Database.Host) == "" || strings.TrimSpace(c.Database.User) == "" || strings.TrimSpace(c.Database.DBName) == "" {
			return fmt.Errorf("DB_HOST, DB_USER, and DB_NAME are required when DATABASE_URL is unset")
		}
		if c.Database.Port < 1 || c.Database.Port > 65535 {
			return fmt.Errorf("DB_PORT must be between 1 and 65535")
		}
	}
	if c.Database.MaxConns < 1 {
		return fmt.Errorf("DB_MAX_CONNS must be greater than zero")
	}
	if c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("DB_MIN_CONNS must be between zero and DB_MAX_CONNS")
	}
	if c.Redis.URL != "" {
		parsed, err := url.Parse(c.Redis.URL)
		if err != nil || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.Host == "" {
			return fmt.Errorf("REDIS_URL must be a valid redis or rediss URL")
		}
	} else if strings.TrimSpace(c.Redis.Host) == "" || c.Redis.Port < 1 || c.Redis.Port > 65535 {
		return fmt.Errorf("REDIS_HOST and a valid REDIS_PORT are required when REDIS_URL is unset")
	}
	rabbitURL, err := url.Parse(c.RabbitMQ.URL)
	if err != nil || (rabbitURL.Scheme != "amqp" && rabbitURL.Scheme != "amqps") || rabbitURL.Host == "" {
		return fmt.Errorf("RABBITMQ_URL must be a valid amqp or amqps URL")
	}
	if strings.TrimSpace(c.JWT.Secret) == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if strings.TrimSpace(c.JWT.Issuer) == "" {
		return fmt.Errorf("JWT_ISSUER is required")
	}
	if c.JWT.ExpiryHours < 1 {
		return fmt.Errorf("JWT_EXPIRY_HOURS must be greater than zero")
	}
	if c.RateLimit.RPS < 1 {
		return fmt.Errorf("RATE_LIMIT_RPS must be greater than zero")
	}
	if len(c.CORS.AllowedOrigins) == 0 {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS must contain at least one origin")
	}

	for i, configuredOrigin := range c.CORS.AllowedOrigins {
		origin := strings.TrimSpace(configuredOrigin)
		c.CORS.AllowedOrigins[i] = origin
		parsed, err := url.Parse(origin)
		if origin == "*" {
			continue
		}
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("invalid CORS origin %q", origin)
		}
	}

	storageType := strings.ToLower(strings.TrimSpace(c.Storage.Type))
	switch storageType {
	case "local":
		if strings.TrimSpace(c.Storage.Path) == "" {
			return fmt.Errorf("STORAGE_PATH is required for local storage")
		}
	case "s3":
		if strings.TrimSpace(c.Storage.S3Bucket) == "" || strings.TrimSpace(c.Storage.S3Region) == "" {
			return fmt.Errorf("S3_BUCKET and S3_REGION are required for S3 storage")
		}
	default:
		return fmt.Errorf("STORAGE_TYPE must be local or s3")
	}
	c.Storage.Type = storageType

	if c.App.Env == "staging" || c.App.Env == "production" {
		if len(c.JWT.Secret) < 32 || c.JWT.Secret == "change-me-in-production" || c.JWT.Secret == "replace-with-a-strong-secret" {
			return fmt.Errorf("JWT_SECRET must be a non-default secret of at least 32 characters")
		}
		if c.Database.URL == "" {
			password := strings.ToLower(strings.TrimSpace(c.Database.Password))
			if password == "" || password == "postgres" || password == "changeme" {
				return fmt.Errorf("DB_PASSWORD must be a non-default secret")
			}
			if !isTLSDatabaseMode(c.Database.SSLMode) {
				return fmt.Errorf("DB_SSL_MODE must require TLS in staging and production")
			}
		} else {
			parsedDatabaseURL, _ := url.Parse(c.Database.URL)
			hasCredentials := false
			if parsedDatabaseURL.User != nil && parsedDatabaseURL.User.Username() != "" {
				password, hasPassword := parsedDatabaseURL.User.Password()
				hasCredentials = hasPassword && password != ""
			}
			if !hasCredentials {
				return fmt.Errorf("DATABASE_URL must include non-empty credentials in staging and production")
			}
			if !databaseURLUsesTLS(c.Database.URL) {
				return fmt.Errorf("DATABASE_URL must require TLS in staging and production")
			}
		}
		redisURL, err := url.Parse(c.Redis.URL)
		if err != nil || redisURL.Scheme != "rediss" {
			return fmt.Errorf("REDIS_URL must use TLS (rediss) in staging and production")
		}
		if rabbitURL.Scheme != "amqps" || rabbitURL.User == nil || rabbitURL.User.Username() == "guest" {
			return fmt.Errorf("RABBITMQ_URL must use TLS and non-guest credentials in staging and production")
		}
		if c.Storage.Type == "local" {
			return fmt.Errorf("STORAGE_TYPE=local is not supported in staging or production")
		}
		for _, origin := range c.CORS.AllowedOrigins {
			parsed, _ := url.Parse(strings.TrimSpace(origin))
			if origin == "*" || parsed.Scheme != "https" {
				return fmt.Errorf("CORS origins must use HTTPS and cannot be wildcarded in staging or production")
			}
		}
	}

	return nil
}

func isTLSDatabaseMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

func databaseURLUsesTLS(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return isTLSDatabaseMode(parsed.Query().Get("sslmode"))
}
