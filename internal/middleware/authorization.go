package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/authz"
)

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
				writeAuthorizationProblem(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
				return
			}
			if authorizer == nil {
				log.Error().
					Str("request_id", GetRequestIDFromContext(r.Context())).
					Str("resource", resource).
					Str("action", action).
					Msg("authorization policy decision point is unavailable")
				writeAuthorizationProblem(w, http.StatusServiceUnavailable, "authorization_unavailable", "Authorization is temporarily unavailable")
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
				IPAddress:      r.RemoteAddr,
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
				writeAuthorizationProblem(w, http.StatusServiceUnavailable, "authorization_unavailable", "Authorization is temporarily unavailable")
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
					Msg("authorization denied")
				writeAuthorizationProblem(w, http.StatusForbidden, "access_denied", "You do not have permission to perform this action")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthorizationProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "urn:complianceforge:problem:" + code,
		"title":  http.StatusText(status),
		"status": status,
		"code":   code,
		"detail": detail,
	})
}
