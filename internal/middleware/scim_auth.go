package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/models"
)

type contextKeySCIMToken string
type contextKeySCIMScopes string
type contextKeySCIMActor string

const (
	ContextKeySCIMTokenID contextKeySCIMToken  = "scim_token_id" // #nosec G101 -- Typed context key for a token record UUID; no token secret.
	ContextKeySCIMScopes  contextKeySCIMScopes = "scim_scopes"
	ContextKeySCIMActor   contextKeySCIMActor  = "scim_actor_user_id"
	maximumSCIMTokenBytes                      = 256
)

func GetSCIMTokenIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(ContextKeySCIMTokenID).(string)
	return value
}

func GetSCIMScopesFromContext(ctx context.Context) []string {
	value, _ := ctx.Value(ContextKeySCIMScopes).([]string)
	return append([]string(nil), value...)
}

func GetSCIMActorUserIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(ContextKeySCIMActor).(string)
	return value
}

// SCIMAuth accepts only the dedicated Bearer credential namespace. Browser
// access JWTs and general API keys are never delegated to this authenticator.
func SCIMAuth(authenticator authdomain.SCIMTokenAuthenticator, limiter APIKeyRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil || limiter == nil {
				writeSCIMMiddlewareError(w, http.StatusServiceUnavailable, "", "SCIM authentication is temporarily unavailable")
				return
			}
			rawToken, ok := scimBearerToken(r.Header.Get("Authorization"))
			if !ok || len(rawToken) > maximumSCIMTokenBytes {
				w.Header().Set("WWW-Authenticate", `Bearer realm="complianceforge-scim"`)
				writeSCIMMiddlewareError(w, http.StatusUnauthorized, "", "Invalid SCIM bearer token")
				return
			}
			clientIP := GetClientIPFromContext(r.Context())
			if clientIP == "" {
				clientIP = directClientIP(r.RemoteAddr)
			}
			principal, err := authenticator.AuthenticateSCIMToken(r.Context(), rawToken, clientIP)
			if err != nil || principal == nil {
				if err != nil && !errors.Is(err, authdomain.ErrInvalidSCIMToken) {
					log.Error().Err(err).Str("path", r.URL.Path).Msg("SCIM authentication backend failed")
					writeSCIMMiddlewareError(w, http.StatusServiceUnavailable, "", "SCIM authentication is temporarily unavailable")
					return
				}
				w.Header().Set("WWW-Authenticate", `Bearer realm="complianceforge-scim"`)
				writeSCIMMiddlewareError(w, http.StatusUnauthorized, "", "Invalid SCIM bearer token")
				return
			}
			if principal.TokenID == "" || principal.OrganizationID == "" || principal.CreatedByUserID == "" || principal.RateLimitPerMinute < 1 {
				log.Error().Str("scim_token_id", principal.TokenID).Msg("SCIM authenticator returned an invalid principal")
				writeSCIMMiddlewareError(w, http.StatusServiceUnavailable, "", "SCIM authentication is temporarily unavailable")
				return
			}
			allowed, retryAfter, err := limiter.Allow(r.Context(), "scim:"+principal.TokenID, principal.RateLimitPerMinute)
			if err != nil {
				log.Error().Err(err).Str("scim_token_id", principal.TokenID).Msg("SCIM rate limiter failed")
				writeSCIMMiddlewareError(w, http.StatusServiceUnavailable, "", "SCIM rate limiting is temporarily unavailable")
				return
			}
			if !allowed {
				seconds := int(retryAfter.Round(time.Second).Seconds())
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				writeSCIMMiddlewareError(w, http.StatusTooManyRequests, "", "SCIM rate limit exceeded")
				return
			}
			ctx := context.WithValue(r.Context(), ContextKeyOrgID, principal.OrganizationID)
			ctx = context.WithValue(ctx, ContextKeySCIMTokenID, principal.TokenID)
			ctx = context.WithValue(ctx, ContextKeySCIMActor, principal.CreatedByUserID)
			ctx = context.WithValue(ctx, ContextKeySCIMScopes, append([]string(nil), principal.Scopes...))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireSCIMScope(scope string) func(http.Handler) http.Handler {
	scope = strings.TrimSpace(scope)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if GetSCIMTokenIDFromContext(r.Context()) == "" {
				writeSCIMMiddlewareError(w, http.StatusUnauthorized, "", "SCIM authentication required")
				return
			}
			for _, candidate := range GetSCIMScopesFromContext(r.Context()) {
				if candidate == scope {
					next.ServeHTTP(w, r)
					return
				}
			}
			log.Warn().Str("scim_token_id", GetSCIMTokenIDFromContext(r.Context())).Str("required_scope", scope).Msg("SCIM scope denied")
			writeSCIMMiddlewareError(w, http.StatusForbidden, "", "SCIM bearer token lacks the required scope")
		})
	}
}

// RequireSCIMFeature applies subscription/capability enforcement without
// leaking the platform's browser-API error shape into the SCIM protocol.
func RequireSCIMFeature(evaluator FeatureEvaluator, capabilityKey string) func(http.Handler) http.Handler {
	capabilityKey = strings.ToLower(strings.TrimSpace(capabilityKey))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			organizationID := GetOrgIDFromContext(r.Context())
			if organizationID == "" || GetSCIMTokenIDFromContext(r.Context()) == "" {
				writeSCIMMiddlewareError(w, http.StatusUnauthorized, "", "SCIM authentication required")
				return
			}
			if evaluator == nil || capabilityKey == "" {
				writeSCIMMiddlewareError(w, http.StatusServiceUnavailable, "", "SCIM feature evaluation is temporarily unavailable")
				return
			}
			evaluation, err := evaluator.Evaluate(r.Context(), organizationID, capabilityKey)
			if err != nil || evaluation == nil {
				log.Error().Err(err).Str("organization_id", organizationID).Str("capability_key", capabilityKey).
					Msg("SCIM feature evaluation failed closed")
				writeSCIMMiddlewareError(w, http.StatusServiceUnavailable, "", "SCIM feature evaluation is temporarily unavailable")
				return
			}
			w.Header().Set("X-Feature-Flag", capabilityKey)
			if !evaluation.Enabled {
				status := http.StatusForbidden
				if evaluation.Reason == "subscription_denied" {
					status = http.StatusPaymentRequired
				}
				writeSCIMMiddlewareError(w, status, "", "SCIM provisioning is not enabled for this tenant")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func scimBearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return parts[1], true
}

func writeSCIMMiddlewareError(w http.ResponseWriter, status int, scimType, detail string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(models.SCIMError{
		Schemas: []string{models.SCIMErrorSchema}, Status: strconv.Itoa(status), SCIMType: scimType, Detail: detail,
	})
}
