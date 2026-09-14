package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/complianceforge/platform/internal/config"
)

// RedisOptions translates the canonical application configuration into
// production-bounded client options. REDIS_URL takes precedence over split
// host settings, matching Config's validation contract.
func RedisOptions(cfg config.RedisConfig) (*redis.Options, error) {
	var options *redis.Options
	var err error
	if strings.TrimSpace(cfg.URL) != "" {
		options, err = redis.ParseURL(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("parse Redis URL: %w", err)
		}
	} else {
		if strings.TrimSpace(cfg.Host) == "" || cfg.Port < 1 || cfg.Port > 65535 {
			return nil, errors.New("valid Redis host and port are required")
		}
		options = &redis.Options{
			Addr:     net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
			Password: cfg.Password,
			DB:       cfg.DB,
		}
	}

	options.DialTimeout = 5 * time.Second
	options.ReadTimeout = 3 * time.Second
	options.WriteTimeout = 3 * time.Second
	options.PoolTimeout = 4 * time.Second
	options.ConnMaxIdleTime = 5 * time.Minute
	options.ConnMaxLifetime = time.Hour
	options.MaxRetries = 2
	options.MinRetryBackoff = 50 * time.Millisecond
	options.MaxRetryBackoff = 500 * time.Millisecond
	return options, nil
}

// NewRedisClient connects and verifies Redis before returning. Startup fails
// closed rather than deferring dependency errors to the first live request.
func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	options, err := RedisOptions(cfg)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(options)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to Redis: %w", err)
	}
	return client, nil
}

// RedisHealthCheck verifies the shared cache/rate-limit dependency.
func RedisHealthCheck(ctx context.Context, client redis.Cmdable) error {
	if client == nil {
		return errors.New("Redis client is unavailable")
	}
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis health check failed: %w", err)
	}
	return nil
}
