// Package ratelimit provides distributed request-limiting primitives.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultPrefix = "complianceforge:rate-limit:api-key:"

var fixedWindowScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
return {current, ttl}
`)

// RedisAPIKeyLimiter implements an atomic, shared fixed-window limiter. Redis
// is required so limits remain consistent across API replicas and restarts.
type RedisAPIKeyLimiter struct {
	client redis.Scripter
	window time.Duration
	prefix string
}

// NewRedisAPIKeyLimiter constructs a one-minute API-key limiter.
func NewRedisAPIKeyLimiter(client redis.Scripter) (*RedisAPIKeyLimiter, error) {
	if client == nil {
		return nil, errors.New("Redis rate-limit client is required")
	}
	return &RedisAPIKeyLimiter{client: client, window: time.Minute, prefix: defaultPrefix}, nil
}

// Allow atomically increments a key's current window and reports whether it is
// still within its configured per-minute limit.
func (l *RedisAPIKeyLimiter) Allow(ctx context.Context, keyID string, limitPerMinute int) (bool, time.Duration, error) {
	if l == nil || l.client == nil {
		return false, 0, errors.New("Redis rate limiter is not configured")
	}
	if keyID == "" {
		return false, 0, errors.New("API key ID is required")
	}
	if limitPerMinute < 1 {
		return false, 0, errors.New("API key rate limit must be greater than zero")
	}
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}

	result, err := fixedWindowScript.Run(
		ctx,
		l.client,
		[]string{l.prefix + keyID},
		l.window.Milliseconds(),
	).Result()
	if err != nil {
		return false, 0, fmt.Errorf("increment API key rate limit: %w", err)
	}
	count, ttl, err := parseWindowResult(result)
	if err != nil {
		return false, 0, err
	}

	retryAfter := time.Duration(ttl) * time.Millisecond
	if retryAfter <= 0 || retryAfter > l.window {
		retryAfter = l.window
	}
	return count <= int64(limitPerMinute), retryAfter, nil
}

func parseWindowResult(result any) (count int64, ttlMilliseconds int64, err error) {
	values, ok := result.([]any)
	if !ok || len(values) != 2 {
		return 0, 0, fmt.Errorf("unexpected Redis rate-limit response %T", result)
	}
	count, ok = values[0].(int64)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected Redis rate-limit count %T", values[0])
	}
	ttlMilliseconds, ok = values[1].(int64)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected Redis rate-limit TTL %T", values[1])
	}
	return count, ttlMilliseconds, nil
}
