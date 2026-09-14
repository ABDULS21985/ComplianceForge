package ratelimit

import (
	"context"
	"testing"
)

func TestNewRedisAPIKeyLimiterRequiresClient(t *testing.T) {
	if _, err := NewRedisAPIKeyLimiter(nil); err == nil {
		t.Fatal("NewRedisAPIKeyLimiter() accepted a nil client")
	}
}

func TestRedisAPIKeyLimiterValidatesInputBeforeRedis(t *testing.T) {
	limiter := &RedisAPIKeyLimiter{}
	if _, _, err := limiter.Allow(context.Background(), "", 10); err == nil {
		t.Fatal("Allow() accepted an unconfigured limiter")
	}
}

func TestParseWindowResult(t *testing.T) {
	count, ttl, err := parseWindowResult([]any{int64(3), int64(42_000)})
	if err != nil {
		t.Fatalf("parseWindowResult() error = %v", err)
	}
	if count != 3 || ttl != 42_000 {
		t.Fatalf("parseWindowResult() = (%d, %d)", count, ttl)
	}

	invalid := []any{
		"not a slice",
		[]any{int64(1)},
		[]any{"1", int64(2)},
		[]any{int64(1), "2"},
	}
	for _, result := range invalid {
		if _, _, err := parseWindowResult(result); err == nil {
			t.Fatalf("parseWindowResult(%#v) unexpectedly succeeded", result)
		}
	}
}
