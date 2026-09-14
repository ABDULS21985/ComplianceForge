package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// RequestRateLimiter is implemented by a shared atomic quota store.
type RequestRateLimiter interface {
	Allow(context.Context, string, int) (bool, time.Duration, error)
}

// DistributedRateLimitMiddleware enforces a per-client requests-per-second
// token bucket across API replicas. Health probes bypass quotas so liveness is
// still observable during a Redis outage; readiness checks Redis separately.
func DistributedRateLimitMiddleware(limiter RequestRateLimiter, requestsPerSecond int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
				next.ServeHTTP(w, r)
				return
			}
			if limiter == nil || requestsPerSecond < 1 {
				writeRequestLimitProblem(w, http.StatusServiceUnavailable, "Request rate limiting unavailable", 0)
				return
			}
			clientIP := GetClientIPFromContext(r.Context())
			if clientIP == "" {
				clientIP = remoteIP(r.RemoteAddr)
			}
			digest := sha256.Sum256([]byte(clientIP))
			allowed, retryAfter, err := limiter.Allow(r.Context(), hex.EncodeToString(digest[:]), requestsPerSecond)
			if err != nil {
				log.Error().Err(err).Str("request_id", GetRequestIDFromContext(r.Context())).Msg("distributed request rate limiter failed")
				writeRequestLimitProblem(w, http.StatusServiceUnavailable, "Request rate limiting unavailable", 0)
				return
			}
			if !allowed {
				writeRequestLimitProblem(w, http.StatusTooManyRequests, "Request rate limit exceeded", retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeRequestLimitProblem(w http.ResponseWriter, status int, detail string, retryAfter time.Duration) {
	w.Header().Set("Content-Type", "application/problem+json")
	if retryAfter > 0 {
		seconds := int(retryAfter.Round(time.Second).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "urn:complianceforge:problem:request-rate-limit", "title": http.StatusText(status),
		"status": status, "detail": detail,
	})
}
