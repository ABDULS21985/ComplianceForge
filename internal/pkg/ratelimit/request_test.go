package ratelimit

import (
	"context"
	"testing"
)

func TestNewRedisRequestLimiterRequiresClient(t *testing.T) {
	if _, err := NewRedisRequestLimiter(nil); err == nil {
		t.Fatal("NewRedisRequestLimiter() accepted nil client")
	}
}

func TestRedisRequestLimiterValidatesBeforeRedis(t *testing.T) {
	limiter := &RedisRequestLimiter{}
	if _, _, err := limiter.Allow(context.Background(), "client", 10); err == nil {
		t.Fatal("unconfigured request limiter was accepted")
	}
}
