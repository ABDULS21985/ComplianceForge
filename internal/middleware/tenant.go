package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// TenantMiddleware returns a Chi-compatible middleware that extracts the
// organization_id from the request context (set by AuthMiddleware) and
// configures the PostgreSQL session variable app.current_tenant. This enables
// Row-Level Security policies to filter data automatically per tenant.
//
// This middleware MUST run after AuthMiddleware in the middleware chain.
func TenantMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID := GetOrgIDFromContext(r.Context())
			if orgID == "" {
				log.Warn().
					Str("path", r.URL.Path).
					Msg("missing organization_id in context; ensure auth middleware runs first")
				http.Error(w, `{"error":"missing tenant context"}`, http.StatusUnauthorized)
				return
			}

			conn, err := pool.Acquire(r.Context())
			if err != nil {
				log.Error().
					Err(err).
					Str("organization_id", orgID).
					Msg("failed to acquire database connection")
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				return
			}
			defer conn.Release()

			// Use a parameterized format to safely set the tenant variable.
			// SET does not support $1 placeholders, so we use fmt.Sprintf with
			// strict validation (orgID comes from a verified JWT claim).
			query := fmt.Sprintf("SET app.current_tenant = '%s'", orgID)
			_, err = conn.Exec(r.Context(), query)
			if err != nil {
				log.Error().
					Err(err).
					Str("organization_id", orgID).
					Msg("failed to set tenant context in database session")
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				return
			}

			// Store the tenant-scoped connection in context so downstream
			// handlers can use it for RLS-filtered queries.
			ctx := context.WithValue(r.Context(), contextKeyTenantConn, conn)

			log.Debug().
				Str("organization_id", orgID).
				Msg("tenant context set")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

const contextKeyTenantConn contextKey = "tenant_conn"

// GetTenantConnFromContext retrieves the tenant-scoped database connection
// from the request context. Returns nil if not available.
func GetTenantConnFromContext(ctx context.Context) *pgxpool.Conn {
	if v, ok := ctx.Value(contextKeyTenantConn).(*pgxpool.Conn); ok {
		return v
	}
	return nil
}
