package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
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
			if _, err := uuid.Parse(orgID); err != nil {
				log.Warn().
					Str("path", r.URL.Path).
					Msg("missing or invalid organization_id in authentication context")
				writeMiddlewareError(w, r, http.StatusUnauthorized, "tenant_context_invalid", "Invalid tenant context", "Authenticate again or contact an administrator if the problem persists.")
				return
			}

			handlerStarted := false
			err := database.WithTenantConnection(r.Context(), pool, orgID, func(ctx context.Context) error {
				// Keep the legacy concrete connection accessor available while
				// repositories migrate to database.QuerierFromContext.
				if conn, ok := database.QuerierFromContext(ctx, nil).(*pgxpool.Conn); ok {
					ctx = context.WithValue(ctx, contextKeyTenantConn, conn)
				}
				handlerStarted = true
				next.ServeHTTP(w, r.WithContext(ctx))
				return nil
			})
			if err != nil {
				log.Error().
					Err(err).
					Str("organization_id", orgID).
					Msg("tenant-scoped request failed")
				if !handlerStarted {
					writeMiddlewareError(w, r, http.StatusInternalServerError, "tenant_context_unavailable", "Tenant context is temporarily unavailable", "")
				}
			}
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
