package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheService defines the interface for key-value caching operations.
type CacheService interface {
	// Get retrieves the value associated with the given key.
	// Returns an error if the key does not exist.
	Get(ctx context.Context, key string) (string, error)

	// Set stores a key-value pair with an optional time-to-live.
	// A zero TTL means the key does not expire.
	Set(ctx context.Context, key, value string, ttl time.Duration) error

	// Delete removes the given key from the cache.
	Delete(ctx context.Context, key string) error

	// Exists checks whether the given key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)
}

// RedisCacheService implements CacheService using Redis via go-redis/v9.
type RedisCacheService struct {
	client *redis.Client
}

// NewRedisCacheService creates a new RedisCacheService connected to the
// specified Redis instance.
func NewRedisCacheService(host, port, password string, db int) (*RedisCacheService, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis at %s:%s: %w", host, port, err)
	}

	return &RedisCacheService{client: client}, nil
}

// Get retrieves the value for the given key from Redis.
func (r *RedisCacheService) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("cache get %q: %w", key, err)
	}
	return val, nil
}

// Set stores a key-value pair in Redis with the specified TTL.
func (r *RedisCacheService) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %q: %w", key, err)
	}
	return nil
}

// Delete removes the given key from Redis.
func (r *RedisCacheService) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache delete %q: %w", key, err)
	}
	return nil
}

// Exists checks whether the given key exists in Redis.
func (r *RedisCacheService) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("cache exists %q: %w", key, err)
	}
	return n > 0, nil
}
