package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	authdomain "github.com/complianceforge/platform/internal/auth"
)

type contextKeyAPIPerms string
type contextKeyAPIKeyID string

const (
	ContextKeyAPIPermissions contextKeyAPIPerms = "api_permissions"
	ContextKeyAPIKeyID       contextKeyAPIKeyID = "api_key_id"
	maxAPIKeyLength                             = 256
)

// APIKeyRateLimiter is implemented by a shared, atomic rate-limit store (for
// example Redis). Implementations return how long the client should wait when
// a request is denied. API-key authentication fails closed if it is absent.
type APIKeyRateLimiter interface {
	Allow(ctx context.Context, keyID string, limitPerMinute int) (allowed bool, retryAfter time.Duration, err error)
}

// GetAPIPermissionsFromContext extracts the API key permissions from context.
func GetAPIPermissionsFromContext(ctx context.Context) []string {
	if permissions, ok := ctx.Value(ContextKeyAPIPermissions).([]string); ok {
		return append([]string(nil), permissions...)
	}
	return nil
}

// GetAPIKeyIDFromContext extracts the authenticated API key identifier.
func GetAPIKeyIDFromContext(ctx context.Context) string {
	if keyID, ok := ctx.Value(ContextKeyAPIKeyID).(string); ok {
		return keyID
	}
	return ""
}

// APIKeyAuth authenticates X-API-Key credentials, establishes the tenant
// identity, and enforces the key's own distributed rate limit. Query-string
// credentials are deliberately rejected because URLs are commonly retained
// in browser history, proxies, analytics, and access logs.
func APIKeyAuth(authenticator authdomain.APIKeyAuthenticator, limiter APIKeyRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil || limiter == nil {
				log.Error().Msg("API key authentication dependencies are unavailable")
				writeAPIKeyProblem(w, http.StatusServiceUnavailable, "API key authentication unavailable")
				return
			}

			rawKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if rawKey == "" || len(rawKey) > maxAPIKeyLength {
				writeAPIKeyProblem(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			principal, err := authenticator.AuthenticateAPIKey(r.Context(), rawKey, directClientIP(r.RemoteAddr))
			if err != nil || principal == nil {
				if err != nil && !errors.Is(err, authdomain.ErrInvalidAPIKey) {
					log.Error().Err(err).Str("path", r.URL.Path).Msg("API key authentication backend failed")
					writeAPIKeyProblem(w, http.StatusServiceUnavailable, "API key authentication unavailable")
					return
				}
				log.Warn().Str("path", r.URL.Path).Msg("invalid API key")
				writeAPIKeyProblem(w, http.StatusUnauthorized, "Invalid API key")
				return
			}
			if principal.KeyID == "" || principal.OrganizationID == "" || principal.RateLimitPerMinute < 1 {
				log.Error().Str("key_id", principal.KeyID).Msg("API key authenticator returned an invalid principal")
				writeAPIKeyProblem(w, http.StatusServiceUnavailable, "API key authentication unavailable")
				return
			}

			allowed, retryAfter, err := limiter.Allow(r.Context(), principal.KeyID, principal.RateLimitPerMinute)
			if err != nil {
				log.Error().Err(err).Str("key_id", principal.KeyID).Msg("API key rate limiter failed")
				writeAPIKeyProblem(w, http.StatusServiceUnavailable, "API key rate limiting unavailable")
				return
			}
			if !allowed {
				seconds := int(retryAfter.Round(time.Second).Seconds())
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				writeAPIKeyProblem(w, http.StatusTooManyRequests, "API key rate limit exceeded")
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyOrgID, principal.OrganizationID)
			ctx = context.WithValue(ctx, ContextKeyAPIKeyID, principal.KeyID)
			ctx = context.WithValue(ctx, ContextKeyAPIPermissions, append([]string(nil), principal.Permissions...))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func directClientIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return ip.String()
	}
	return ""
}

func writeAPIKeyProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `ApiKey realm="complianceforge"`)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "urn:complianceforge:problem:api-key-authentication",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}
