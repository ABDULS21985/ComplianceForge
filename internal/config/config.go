package config

import (
	"fmt"
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

	// Bind environment variables with the CF_ prefix.
	v.SetEnvPrefix("CF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Sensible defaults.
	v.SetDefault("app.name", "ComplianceForge")
	v.SetDefault("app.env", "development")
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.grpc_port", 9090)

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.dbname", "complianceforge")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_conns", 25)
	v.SetDefault("database.min_conns", 5)

	v.SetDefault("redis.host", "localhost")
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

	return &cfg, nil
}
