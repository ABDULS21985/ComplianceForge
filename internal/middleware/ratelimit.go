package middleware

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = math.Min(b.maxTokens, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RateLimitMiddleware returns a Chi-compatible middleware that enforces a
// per-IP token bucket rate limit. When the limit is exceeded, it responds
// with 429 Too Many Requests and includes a Retry-After header.
func RateLimitMiddleware(rps int) func(http.Handler) http.Handler {
	buckets := &sync.Map{}

	// Background cleanup of stale buckets every 5 minutes.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-10 * time.Minute)
			buckets.Range(func(key, value interface{}) bool {
				b := value.(*tokenBucket)
				b.mu.Lock()
				if b.lastRefill.Before(cutoff) {
					buckets.Delete(key)
				}
				b.mu.Unlock()
				return true
			})
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}

			val, _ := buckets.LoadOrStore(ip, &tokenBucket{
				tokens:     float64(rps),
				maxTokens:  float64(rps),
				refillRate: float64(rps),
				lastRefill: time.Now(),
			})
			bucket := val.(*tokenBucket)

			if !bucket.allow() {
				log.Warn().
					Str("ip", ip).
					Str("path", r.URL.Path).
					Msg("rate limit exceeded")

				w.Header().Set("Retry-After", fmt.Sprintf("%d", 1))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, `{"error":"rate limit exceeded"}`)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
