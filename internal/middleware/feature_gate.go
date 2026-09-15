package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/models"
)

type FeatureEvaluator interface {
	Evaluate(context.Context, string, string) (*models.FeatureFlagEvaluation, error)
}

type EntitlementLimitChecker interface {
	CheckLimit(context.Context, string, string, int64) (*models.EntitlementLimitDecision, error)
}

// RequireFeature evaluates a capability after authentication and tenant
// connection setup. Evaluation failures deny access rather than accidentally
// exposing a premium or operationally disabled capability.
func RequireFeature(evaluator FeatureEvaluator, capabilityKey string) func(http.Handler) http.Handler {
	capabilityKey = strings.TrimSpace(strings.ToLower(capabilityKey))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			organizationID := GetOrgIDFromContext(r.Context())
			if organizationID == "" {
				writeFeatureGateError(w, r, http.StatusUnauthorized, "TENANT_CONTEXT_REQUIRED", "Tenant context is required")
				return
			}
			if evaluator == nil || capabilityKey == "" {
				log.Error().Str("capability_key", capabilityKey).Msg("feature evaluator is unavailable")
				writeFeatureGateError(w, r, http.StatusServiceUnavailable, "FEATURE_EVALUATION_UNAVAILABLE", "Feature availability could not be verified")
				return
			}
			evaluation, err := evaluator.Evaluate(r.Context(), organizationID, capabilityKey)
			if err != nil || evaluation == nil {
				log.Error().Err(err).Str("organization_id", organizationID).
					Str("capability_key", capabilityKey).Msg("feature evaluation failed closed")
				writeFeatureGateError(w, r, http.StatusServiceUnavailable, "FEATURE_EVALUATION_UNAVAILABLE", "Feature availability could not be verified")
				return
			}
			w.Header().Set("X-Feature-Flag", capabilityKey)
			if !evaluation.Enabled {
				if evaluation.Reason == "subscription_denied" {
					writeFeatureGateError(w, r, http.StatusPaymentRequired, "ENTITLEMENT_REQUIRED", "This capability is not included in the current subscription")
					return
				}
				writeFeatureGateError(w, r, http.StatusForbidden, "FEATURE_DISABLED", "This capability is currently disabled")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireEntitlementCapacity provides an actionable preflight response before
// a bounded create route runs. Repositories repeat the check under an advisory
// transaction lock, so this middleware improves UX without becoming the sole
// concurrency boundary.
func RequireEntitlementCapacity(checker EntitlementLimitChecker, metric string, requested int64) func(http.Handler) http.Handler {
	metric = strings.TrimSpace(strings.ToLower(metric))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			organizationID := GetOrgIDFromContext(r.Context())
			if organizationID == "" {
				writeFeatureGateError(w, r, http.StatusUnauthorized, "TENANT_CONTEXT_REQUIRED", "Tenant context is required")
				return
			}
			if checker == nil || metric == "" || requested < 1 {
				writeFeatureGateError(w, r, http.StatusServiceUnavailable, "ENTITLEMENT_EVALUATION_UNAVAILABLE", "Subscription capacity could not be verified")
				return
			}
			decision, err := checker.CheckLimit(r.Context(), organizationID, metric, requested)
			if err != nil || decision == nil {
				log.Error().Err(err).Str("organization_id", organizationID).Str("metric", metric).
					Msg("entitlement capacity evaluation failed closed")
				writeFeatureGateError(w, r, http.StatusServiceUnavailable, "ENTITLEMENT_EVALUATION_UNAVAILABLE", "Subscription capacity could not be verified")
				return
			}
			if !decision.Allowed {
				w.Header().Set("X-Entitlement-Metric", metric)
				writeFeatureGateError(w, r, http.StatusPaymentRequired, "ENTITLEMENT_LIMIT_EXCEEDED", "The subscription limit for this resource has been reached")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeFeatureGateError(w http.ResponseWriter, r *http.Request, status int, errorCode, message string) {
	writeMiddlewareError(w, r, status, errorCode, message, "")
}
