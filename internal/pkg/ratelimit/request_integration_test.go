//go:build integration

package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRedisRequestLimiterSharesTokenBucket(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL is not set")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	first, err := NewRedisRequestLimiter(client)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRedisRequestLimiter(client)
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	defer client.Del(context.Background(), requestPrefix+key)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	allowed, _, err := first.Allow(ctx, key, 1)
	if err != nil || !allowed {
		t.Fatalf("first request allowed=%v error=%v", allowed, err)
	}
	allowed, retry, err := second.Allow(ctx, key, 1)
	if err != nil {
		t.Fatal(err)
	}
	if allowed || retry <= 0 || retry > time.Second {
		t.Fatalf("second request allowed=%v retry=%s", allowed, retry)
	}
}
