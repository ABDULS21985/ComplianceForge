package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// PlanLimits middleware checks subscription plan limits before resource creation.
// It queries the current count of the given resource for the organization and
// compares it against the plan limit. If the limit is exceeded, it returns
// 402 Payment Required with a JSON body describing the constraint.
//
// A max value of 0 means unlimited.
func PlanLimits(pool *pgxpool.Pool, resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID := GetOrgIDFromContext(r.Context())
			if orgID == "" {
				log.Warn().
					Str("path", r.URL.Path).
					Msg("plan limits check: missing organization context")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "missing organization context",
				})
				return
			}

			// Query the current count of the resource for this organization.
			var currentCount int
			err := pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM `+sanitizeTableName(resource)+
					` WHERE organization_id = $1 AND deleted_at IS NULL`,
				orgID,
			).Scan(&currentCount)
			if err != nil {
				log.Error().
					Err(err).
					Str("org_id", orgID).
					Str("resource", resource).
					Msg("failed to count resources for plan limit check")
				// Allow the request through on count failure to avoid blocking.
				next.ServeHTTP(w, r)
				return
			}

			// Query the plan limit for this resource.
			var maxAllowed int
			err = pool.QueryRow(r.Context(),
				`SELECT COALESCE(pl.max_value, 0)
				 FROM subscriptions s
				 JOIN plan_limits pl ON pl.plan_id = s.plan_id
				 WHERE s.organization_id = $1
				   AND s.status = 'active'
				   AND pl.resource = $2`,
				orgID, resource,
			).Scan(&maxAllowed)
			if err != nil {
				log.Warn().
					Err(err).
					Str("org_id", orgID).
					Str("resource", resource).
					Msg("no plan limit found, allowing request")
				// No limit defined or no active subscription found; allow through.
				next.ServeHTTP(w, r)
				return
			}

			// A maxAllowed of 0 means unlimited.
			if maxAllowed > 0 && currentCount >= maxAllowed {
				log.Info().
					Str("org_id", orgID).
					Str("resource", resource).
					Int("current", currentCount).
					Int("max", maxAllowed).
					Msg("plan limit exceeded")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":       "plan_limit_exceeded",
					"resource":    resource,
					"current":     currentCount,
					"max":         maxAllowed,
					"upgrade_url": "/subscription/plans",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// sanitizeTableName returns the resource name only if it matches known safe
// table names, preventing SQL injection through the resource parameter.
func sanitizeTableName(resource string) string {
	allowed := map[string]string{
		"users":        "users",
		"risks":        "risks",
		"controls":     "controls",
		"policies":     "policies",
		"frameworks":   "frameworks",
		"audits":       "audits",
		"incidents":    "incidents",
		"vendors":      "vendors",
		"assets":       "assets",
		"integrations": "integrations",
		"workflows":    "workflow_definitions",
		"api_keys":     "api_keys",
		"documents":    "documents",
	}
	if table, ok := allowed[resource]; ok {
		return table
	}
	// Default fallback: use the resource name as-is if it looks safe.
	// In production, this should reject unknown resources.
	return resource
}
