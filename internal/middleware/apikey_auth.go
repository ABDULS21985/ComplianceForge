package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type contextKeyAPIPerms string

const ContextKeyAPIPermissions contextKeyAPIPerms = "api_permissions"

// GetAPIPermissionsFromContext extracts the API key permissions from the request context.
func GetAPIPermissionsFromContext(ctx context.Context) []string {
	if v, ok := ctx.Value(ContextKeyAPIPermissions).([]string); ok {
		return v
	}
	return nil
}

// APIKeyAuth middleware authenticates requests using API keys.
// It checks the X-API-Key header first, then falls back to the api_key query parameter.
// On success it injects org_id and permissions into the request context.
func APIKeyAuth(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Extract the raw API key from header or query param.
			rawKey := r.Header.Get("X-API-Key")
			if rawKey == "" {
				rawKey = r.URL.Query().Get("api_key")
			}
			if rawKey == "" {
				log.Warn().
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("missing API key")
				writeAPIError(w, http.StatusUnauthorized, "Missing API key")
				return
			}

			// 2. Hash the key with SHA-256 for database lookup.
			hash := sha256.Sum256([]byte(rawKey))
			keyHash := hex.EncodeToString(hash[:])

			// 3. Look up the key in the api_keys table.
			var (
				keyID           string
				orgID           string
				permissionsJSON []byte
				isActive        bool
				expiresAt       *time.Time
				rateLimit       int
			)

			err := pool.QueryRow(r.Context(),
				`SELECT id, organization_id, permissions, is_active, expires_at, rate_limit
				 FROM api_keys
				 WHERE key_hash = $1`,
				keyHash,
			).Scan(&keyID, &orgID, &permissionsJSON, &isActive, &expiresAt, &rateLimit)

			if err != nil {
				log.Warn().
					Err(err).
					Str("path", r.URL.Path).
					Msg("API key not found")
				writeAPIError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			// 4. Verify the key is active.
			if !isActive {
				log.Warn().
					Str("key_id", keyID).
					Msg("API key is inactive")
				writeAPIError(w, http.StatusUnauthorized, "API key is inactive")
				return
			}

			// 5. Verify the key is not expired.
			if expiresAt != nil && expiresAt.Before(time.Now()) {
				log.Warn().
					Str("key_id", keyID).
					Time("expires_at", *expiresAt).
					Msg("API key has expired")
				writeAPIError(w, http.StatusUnauthorized, "API key has expired")
				return
			}

			// 6. Parse permissions.
			var permissions []string
			if len(permissionsJSON) > 0 {
				if err := json.Unmarshal(permissionsJSON, &permissions); err != nil {
					log.Error().
						Err(err).
						Str("key_id", keyID).
						Msg("failed to parse API key permissions")
					writeAPIError(w, http.StatusInternalServerError, "Internal error")
					return
				}
			}

			// 7. Rate limit check: use a simple per-minute counter via database.
			if rateLimit > 0 {
				var requestCount int
				err := pool.QueryRow(r.Context(),
					`SELECT COUNT(*) FROM api_key_usage
					 WHERE key_id = $1 AND created_at > NOW() - INTERVAL '1 minute'`,
					keyID,
				).Scan(&requestCount)

				if err == nil && requestCount >= rateLimit {
					log.Warn().
						Str("key_id", keyID).
						Int("rate_limit", rateLimit).
						Int("current", requestCount).
						Msg("API key rate limit exceeded")
					w.Header().Set("Retry-After", "60")
					writeAPIError(w, http.StatusTooManyRequests, "Rate limit exceeded")
					return
				}
			}

			// 8. Update last_used_at and last_used_ip asynchronously.
			clientIP := extractClientIP(r)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := pool.Exec(ctx,
					`UPDATE api_keys SET last_used_at = NOW(), last_used_ip = $1 WHERE id = $2`,
					clientIP, keyID,
				)
				if err != nil {
					log.Error().
						Err(err).
						Str("key_id", keyID).
						Msg("failed to update API key last_used")
				}
			}()

			// 9. Set org_id and permissions in context.
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyOrgID, orgID)
			ctx = context.WithValue(ctx, ContextKeyAPIPermissions, permissions)

			log.Debug().
				Str("key_id", keyID).
				Str("org_id", orgID).
				Int("permissions_count", len(permissions)).
				Msg("authenticated via API key")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractClientIP returns the client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr.
func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Strip port from RemoteAddr.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// writeAPIError writes a JSON error response for the API key middleware.
func writeAPIError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}
