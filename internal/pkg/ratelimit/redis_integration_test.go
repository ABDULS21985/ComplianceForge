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

func TestRedisAPIKeyLimiterIsSharedAndAtomic(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL is not set")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("redis.ParseURL() error = %v", err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, err := NewRedisAPIKeyLimiter(client)
	if err != nil {
		t.Fatalf("NewRedisAPIKeyLimiter() error = %v", err)
	}
	second, err := NewRedisAPIKeyLimiter(client)
	if err != nil {
		t.Fatalf("NewRedisAPIKeyLimiter() error = %v", err)
	}
	keyID := uuid.NewString()
	defer client.Del(context.Background(), defaultPrefix+keyID)

	for request := 1; request <= 2; request++ {
		allowed, _, err := first.Allow(ctx, keyID, 2)
		if err != nil || !allowed {
			t.Fatalf("request %d allowed = %v, error = %v", request, allowed, err)
		}
	}
	allowed, retryAfter, err := second.Allow(ctx, keyID, 2)
	if err != nil {
		t.Fatalf("third request error = %v", err)
	}
	if allowed || retryAfter <= 0 || retryAfter > time.Minute {
		t.Fatalf("third request allowed = %v, retryAfter = %s", allowed, retryAfter)
	}
}
