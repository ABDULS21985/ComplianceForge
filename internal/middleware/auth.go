package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	authdomain "github.com/complianceforge/platform/internal/auth"
)

type contextKey string

const (
	ContextKeyUserID contextKey = "user_id"
	ContextKeyOrgID  contextKey = "organization_id"
	ContextKeyRole   contextKey = "role"
	ContextKeyEmail  contextKey = "email"
)

// AccessTokenValidator validates a signed access token and its persisted
// session before the request is allowed to proceed.
type AccessTokenValidator interface {
	ValidateAccessToken(ctx context.Context, token string) (*authdomain.Claims, error)
}

// Claims is retained as an alias for callers that previously referenced the
// middleware-local claim type.
type Claims = authdomain.Claims

// AuthMiddleware returns a Chi-compatible middleware that validates JWT tokens
// from the Authorization header and injects claims into the request context.
func AuthMiddleware(validator AccessTokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validator == nil {
				log.Error().Msg("authentication middleware has no token validator")
				writeAuthError(w, "authentication unavailable")
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Warn().
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("missing authorization header")
				writeAuthError(w, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				log.Warn().
					Str("path", r.URL.Path).
					Msg("invalid authorization header format")
				writeAuthError(w, "invalid authorization header format")
				return
			}

			tokenString := parts[1]
			claims, err := validator.ValidateAccessToken(r.Context(), tokenString)
			if err != nil || claims == nil {
				log.Warn().
					Err(err).
					Str("path", r.URL.Path).
					Msg("invalid or expired token")
				writeAuthError(w, "invalid or expired token")
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyOrgID, claims.OrganizationID)
			ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
			ctx = context.WithValue(ctx, ContextKeyEmail, claims.Email)

			log.Debug().
				Str("user_id", claims.UserID).
				Str("organization_id", claims.OrganizationID).
				Str("role", claims.Role).
				Msg("authenticated request")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}

// GetUserIDFromContext extracts the user_id from the request context.
func GetUserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return v
	}
	return ""
}

// GetOrgIDFromContext extracts the organization_id from the request context.
func GetOrgIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyOrgID).(string); ok {
		return v
	}
	return ""
}

// GetRoleFromContext extracts the role from the request context.
func GetRoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyRole).(string); ok {
		return v
	}
	return ""
}

// GetEmailFromContext extracts the email from the request context.
func GetEmailFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyEmail).(string); ok {
		return v
	}
	return ""
}
