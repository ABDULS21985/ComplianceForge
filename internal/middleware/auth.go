package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
)

type contextKey string

const (
	ContextKeyUserID contextKey = "user_id"
	ContextKeyOrgID  contextKey = "organization_id"
	ContextKeyRole   contextKey = "role"
	ContextKeyEmail  contextKey = "email"
)

// Claims represents the JWT claims extracted from the token.
type Claims struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
	Email          string `json:"email"`
	jwt.RegisteredClaims
}

// AuthMiddleware returns a Chi-compatible middleware that validates JWT tokens
// from the Authorization header and injects claims into the request context.
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Warn().
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("missing authorization header")
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				log.Warn().
					Str("path", r.URL.Path).
					Msg("invalid authorization header format")
				http.Error(w, `{"error":"invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})
			if err != nil || !token.Valid {
				log.Warn().
					Err(err).
					Str("path", r.URL.Path).
					Msg("invalid or expired token")
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
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
