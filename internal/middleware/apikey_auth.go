package middleware

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/authz"
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

// RequireAPIKeyPermission enforces one exact action:resource grant after
// APIKeyAuth. API keys deliberately have no implicit role permissions or
// wildcard escalation path.
func RequireAPIKeyPermission(action, resource string) func(http.Handler) http.Handler {
	required := strings.TrimSpace(action) + ":" + strings.TrimSpace(resource)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if GetAPIKeyIDFromContext(r.Context()) == "" {
				writeAPIKeyProblem(w, r, http.StatusUnauthorized, "api_key_authentication_required", "API key authentication required", "Provide a valid X-API-Key header.")
				return
			}
			for _, permission := range GetAPIPermissionsFromContext(r.Context()) {
				if permission == required {
					// The exact API-key scope is this non-user entry point's
					// authorization decision. Persist it in the same trusted
					// context slot used by the interactive policy engine so
					// classified serializers never infer an allow merely from
					// the absence of an ABAC decision.
					ctx := ContextWithAuthorizationDecision(r.Context(), authz.Decision{
						Allowed:     true,
						Reason:      "Exact API key scope granted",
						ReasonCode:  "api_key_scope_granted",
						Obligations: []authz.Obligation{},
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			log.Warn().
				Str("key_id", GetAPIKeyIDFromContext(r.Context())).
				Str("required_permission", required).
				Msg("API key permission denied")
			writeAPIKeyProblem(w, r, http.StatusForbidden, "api_key_permission_denied", "API key permission denied", "Use an API key that grants "+required+".")
		})
	}
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
				writeAPIKeyProblem(w, r, http.StatusServiceUnavailable, "api_key_authentication_unavailable", "API key authentication is temporarily unavailable", "")
				return
			}

			rawKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if rawKey == "" || len(rawKey) > maxAPIKeyLength {
				writeAPIKeyProblem(w, r, http.StatusUnauthorized, "invalid_api_key", "Invalid API key", "Provide a current API key in the X-API-Key header.")
				return
			}

			clientIP := GetClientIPFromContext(r.Context())
			if clientIP == "" {
				clientIP = directClientIP(r.RemoteAddr)
			}
			principal, err := authenticator.AuthenticateAPIKey(r.Context(), rawKey, clientIP)
			if err != nil || principal == nil {
				if err != nil && !errors.Is(err, authdomain.ErrInvalidAPIKey) {
					log.Error().Err(err).Str("path", r.URL.Path).Msg("API key authentication backend failed")
					writeAPIKeyProblem(w, r, http.StatusServiceUnavailable, "api_key_authentication_unavailable", "API key authentication is temporarily unavailable", "")
					return
				}
				log.Warn().Str("path", r.URL.Path).Msg("invalid API key")
				writeAPIKeyProblem(w, r, http.StatusUnauthorized, "invalid_api_key", "Invalid API key", "Provide a current API key in the X-API-Key header.")
				return
			}
			if principal.KeyID == "" || principal.OrganizationID == "" || principal.RateLimitPerMinute < 1 {
				log.Error().Str("key_id", principal.KeyID).Msg("API key authenticator returned an invalid principal")
				writeAPIKeyProblem(w, r, http.StatusServiceUnavailable, "api_key_authentication_unavailable", "API key authentication is temporarily unavailable", "")
				return
			}

			allowed, retryAfter, err := limiter.Allow(r.Context(), principal.KeyID, principal.RateLimitPerMinute)
			if err != nil {
				log.Error().Err(err).Str("key_id", principal.KeyID).Msg("API key rate limiter failed")
				writeAPIKeyProblem(w, r, http.StatusServiceUnavailable, "api_key_rate_limiting_unavailable", "API key rate limiting is temporarily unavailable", "")
				return
			}
			if !allowed {
				seconds := int(retryAfter.Round(time.Second).Seconds())
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				writeAPIKeyProblem(w, r, http.StatusTooManyRequests, "api_key_rate_limit_exceeded", "API key rate limit exceeded", "Wait for Retry-After seconds before retrying.")
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

func writeAPIKeyProblem(w http.ResponseWriter, r *http.Request, status int, code, message, details string) {
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `ApiKey realm="complianceforge"`)
	}
	writeMiddlewareError(w, r, status, code, message, details)
}
