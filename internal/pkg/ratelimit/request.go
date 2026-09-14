package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const requestPrefix = "complianceforge:rate-limit:http:"

var tokenBucketScript = redis.NewScript(`
local timestamp = redis.call('TIME')
local now_ms = (timestamp[1] * 1000) + math.floor(timestamp[2] / 1000)
local capacity = tonumber(ARGV[1])
local refill_per_ms = capacity / 1000
local current = redis.call('HMGET', KEYS[1], 'tokens', 'updated_ms')
local tokens = tonumber(current[1])
local updated_ms = tonumber(current[2])
if tokens == nil or updated_ms == nil then
  tokens = capacity
  updated_ms = now_ms
end
local elapsed = math.max(0, now_ms - updated_ms)
tokens = math.min(capacity, tokens + (elapsed * refill_per_ms))
local allowed = 0
local retry_ms = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  retry_ms = math.ceil((1 - tokens) / refill_per_ms)
end
redis.call('HSET', KEYS[1], 'tokens', tokens, 'updated_ms', now_ms)
local full_refill_ms = capacity / refill_per_ms
local ttl_ms = math.max(2000, math.min(60000, math.ceil(full_refill_ms * 2)))
redis.call('PEXPIRE', KEYS[1], ttl_ms)
return {allowed, retry_ms}
`)

// RedisRequestLimiter is a shared per-client token bucket. Redis server time
// prevents clock skew between API replicas from changing quota decisions.
type RedisRequestLimiter struct {
	client redis.Scripter
	prefix string
}

func NewRedisRequestLimiter(client redis.Scripter) (*RedisRequestLimiter, error) {
	if client == nil {
		return nil, errors.New("Redis request-limit client is required")
	}
	return &RedisRequestLimiter{client: client, prefix: requestPrefix}, nil
}

// Allow consumes one token from the client's requests-per-second bucket.
func (l *RedisRequestLimiter) Allow(ctx context.Context, clientKey string, requestsPerSecond int) (bool, time.Duration, error) {
	if l == nil || l.client == nil {
		return false, 0, errors.New("Redis request limiter is not configured")
	}
	if clientKey == "" || len(clientKey) > 128 {
		return false, 0, errors.New("request limiter client key is invalid")
	}
	if requestsPerSecond < 1 || requestsPerSecond > 1_000_000 {
		return false, 0, errors.New("requests-per-second limit is invalid")
	}
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	result, err := tokenBucketScript.Run(ctx, l.client, []string{l.prefix + clientKey}, requestsPerSecond).Result()
	if err != nil {
		return false, 0, fmt.Errorf("consume request-rate token: %w", err)
	}
	values, ok := result.([]any)
	if !ok || len(values) != 2 {
		return false, 0, fmt.Errorf("unexpected Redis request-limit response %T", result)
	}
	allowed, ok := values[0].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected Redis request-limit decision %T", values[0])
	}
	retryMilliseconds, ok := values[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected Redis request-limit retry value %T", values[1])
	}
	retryAfter := time.Duration(retryMilliseconds) * time.Millisecond
	if allowed == 1 {
		return true, 0, nil
	}
	if retryAfter < time.Millisecond {
		retryAfter = time.Millisecond
	}
	if retryAfter > time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter, nil
}
