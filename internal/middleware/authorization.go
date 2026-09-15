package middleware

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/authz"
)

type authorizationDecisionContextKey struct{}

// GetAuthorizationDecision returns the persisted decision and obligations for
// the current protected request. Downstream serializers/exporters can enforce
// field masking and watermarking only after authorization has succeeded.
func GetAuthorizationDecision(ctx context.Context) (authz.Decision, bool) {
	decision, ok := ctx.Value(authorizationDecisionContextKey{}).(authz.Decision)
	return decision, ok
}

// ContextWithAuthorizationDecision attaches a decision produced by a trusted
// policy decision point. It exists so non-HTTP adapters and focused tests can
// reuse the same fail-closed serialization boundary; callers must never derive
// the decision from request payloads or headers.
func ContextWithAuthorizationDecision(ctx context.Context, decision authz.Decision) context.Context {
	return context.WithValue(ctx, authorizationDecisionContextKey{}, decision)
}

// ResourceIDResolver extracts a resource identifier after the router has
// populated path parameters. It may return an empty string for collection-level
// authorization decisions.
type ResourceIDResolver func(*http.Request) string

// RequireAuthorization asks the configured policy decision point to authorize
// every request. Missing identity, decision errors, and unmatched policies all
// fail closed.
func RequireAuthorization(
	authorizer authz.Authorizer,
	resource string,
	action string,
	resolveResourceID ResourceIDResolver,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserIDFromContext(r.Context())
			orgID := GetOrgIDFromContext(r.Context())
			if userID == "" || orgID == "" {
				writeAuthorizationProblem(w, r, http.StatusUnauthorized, "authentication_required", "Authentication is required")
				return
			}
			if authorizer == nil {
				log.Error().
					Str("request_id", GetRequestIDFromContext(r.Context())).
					Str("resource", resource).
					Str("action", action).
					Msg("authorization policy decision point is unavailable")
				writeAuthorizationProblem(w, r, http.StatusServiceUnavailable, "authorization_unavailable", "Authorization is temporarily unavailable")
				return
			}

			resourceID := ""
			if resolveResourceID != nil {
				resourceID = resolveResourceID(r)
			}
			decision, err := authorizer.Authorize(r.Context(), authz.Request{
				SubjectID:      userID,
				OrganizationID: orgID,
				Role:           GetRoleFromContext(r.Context()),
				Resource:       resource,
				ResourceID:     resourceID,
				Action:         action,
				IPAddress:      GetClientIPFromContext(r.Context()),
				MFAVerified:    GetMFAVerifiedFromContext(r.Context()),
				Attributes: map[string]any{
					"request_id": GetRequestIDFromContext(r.Context()),
				},
			})
			if err != nil {
				log.Error().
					Err(err).
					Str("request_id", GetRequestIDFromContext(r.Context())).
					Str("user_id", userID).
					Str("organization_id", orgID).
					Str("resource", resource).
					Str("action", action).
					Msg("authorization decision failed closed")
				writeAuthorizationProblem(w, r, http.StatusServiceUnavailable, "authorization_unavailable", "Authorization is temporarily unavailable")
				return
			}
			if !decision.Allowed {
				log.Warn().
					Str("request_id", GetRequestIDFromContext(r.Context())).
					Str("user_id", userID).
					Str("organization_id", orgID).
					Str("resource", resource).
					Str("resource_id", resourceID).
					Str("action", action).
					Str("policy_id", decision.PolicyID).
					Str("reason_code", decision.ReasonCode).
					Msg("authorization denied")
				writeAuthorizationProblem(w, r, http.StatusForbidden, "access_denied", "You do not have permission to perform this action")
				return
			}

			ctx := ContextWithAuthorizationDecision(r.Context(), decision)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthorizationProblem(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeMiddlewareError(w, r, status, code, message, "")
}
