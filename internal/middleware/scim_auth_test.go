package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/models"
)

type scimAuthenticatorStub struct {
	principal *authdomain.SCIMPrincipal
	err       error
	raw       string
	ip        string
}

func (s *scimAuthenticatorStub) AuthenticateSCIMToken(_ context.Context, raw, ip string) (*authdomain.SCIMPrincipal, error) {
	s.raw, s.ip = raw, ip
	return s.principal, s.err
}

func TestSCIMAuthEstablishesDedicatedTenantPrincipal(t *testing.T) {
	authenticator := &scimAuthenticatorStub{principal: &authdomain.SCIMPrincipal{
		TokenID: "token-id", OrganizationID: "organization-id", CreatedByUserID: "creator-id",
		Scopes: []string{models.SCIMTokenScopeUsersRead}, RateLimitPerMinute: 80,
	}}
	limiter := &fakeAPIKeyLimiter{allowed: true}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetOrgIDFromContext(r.Context()) != "organization-id" || GetSCIMTokenIDFromContext(r.Context()) != "token-id" ||
			GetSCIMActorUserIDFromContext(r.Context()) != "creator-id" || len(GetSCIMScopesFromContext(r.Context())) != 1 {
			t.Fatalf("SCIM context was not established")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/scim/v2/Users", nil)
	request.RemoteAddr = "192.0.2.45:1234"
	request.Header.Set("Authorization", "Bearer cfs_test-token")
	response := httptest.NewRecorder()
	SCIMAuth(authenticator, limiter)(next).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || authenticator.raw != "cfs_test-token" || authenticator.ip != "192.0.2.45" {
		t.Fatalf("status=%d raw=%q ip=%q body=%s", response.Code, authenticator.raw, authenticator.ip, response.Body.String())
	}
	if limiter.keyID != "scim:token-id" || limiter.limit != 80 {
		t.Fatalf("limiter key=%q limit=%d", limiter.keyID, limiter.limit)
	}
}

func TestSCIMAuthUsesRFC7644ErrorsAndFailsClosed(t *testing.T) {
	tests := []struct {
		name          string
		authenticator authdomain.SCIMTokenAuthenticator
		limiter       APIKeyRateLimiter
		header        string
		want          int
	}{
		{name: "missing", authenticator: &scimAuthenticatorStub{}, limiter: &fakeAPIKeyLimiter{allowed: true}, want: http.StatusUnauthorized},
		{name: "wrong scheme", authenticator: &scimAuthenticatorStub{}, limiter: &fakeAPIKeyLimiter{allowed: true}, header: "Basic abc", want: http.StatusUnauthorized},
		{name: "invalid", authenticator: &scimAuthenticatorStub{err: authdomain.ErrInvalidSCIMToken}, limiter: &fakeAPIKeyLimiter{allowed: true}, header: "Bearer invalid", want: http.StatusUnauthorized},
		{name: "store unavailable", authenticator: &scimAuthenticatorStub{err: errors.New("database unavailable")}, limiter: &fakeAPIKeyLimiter{allowed: true}, header: "Bearer token", want: http.StatusServiceUnavailable},
		{name: "limiter unavailable", authenticator: &scimAuthenticatorStub{principal: validSCIMPrincipal()}, limiter: &fakeAPIKeyLimiter{err: errors.New("redis unavailable")}, header: "Bearer token", want: http.StatusServiceUnavailable},
		{name: "limited", authenticator: &scimAuthenticatorStub{principal: validSCIMPrincipal()}, limiter: &fakeAPIKeyLimiter{allowed: false, retryAfter: 1500 * time.Millisecond}, header: "Bearer token", want: http.StatusTooManyRequests},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/scim/v2/Users", nil)
			request.Header.Set("Authorization", test.header)
			response := httptest.NewRecorder()
			SCIMAuth(test.authenticator, test.limiter)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("next handler called")
			})).ServeHTTP(response, request)
			if response.Code != test.want || response.Header().Get("Content-Type") != "application/scim+json" {
				t.Fatalf("status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
			}
			var problem models.SCIMError
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || len(problem.Schemas) != 1 || problem.Schemas[0] != models.SCIMErrorSchema {
				t.Fatalf("problem=%#v error=%v", problem, err)
			}
		})
	}
}

func TestRequireSCIMScopeUsesExactMatch(t *testing.T) {
	for _, test := range []struct {
		name, scope string
		scopes      []string
		want        int
	}{
		{name: "allowed", scope: models.SCIMTokenScopeUsersRead, scopes: []string{models.SCIMTokenScopeUsersRead}, want: http.StatusNoContent},
		{name: "write does not imply read", scope: models.SCIMTokenScopeUsersRead, scopes: []string{models.SCIMTokenScopeUsersWrite}, want: http.StatusForbidden},
		{name: "wildcard rejected", scope: models.SCIMTokenScopeUsersRead, scopes: []string{"*"}, want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), ContextKeySCIMTokenID, "token")
			ctx = context.WithValue(ctx, ContextKeySCIMScopes, test.scopes)
			request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
			response := httptest.NewRecorder()
			RequireSCIMScope(test.scope)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type scimFeatureEvaluatorStub struct {
	evaluation *models.FeatureFlagEvaluation
	err        error
}

func (s scimFeatureEvaluatorStub) Evaluate(context.Context, string, string) (*models.FeatureFlagEvaluation, error) {
	return s.evaluation, s.err
}

func TestRequireSCIMFeaturePreservesProtocolErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		evaluator  FeatureEvaluator
		principal  bool
		wantStatus int
	}{
		{name: "enabled", principal: true, evaluator: scimFeatureEvaluatorStub{evaluation: &models.FeatureFlagEvaluation{Enabled: true}}, wantStatus: http.StatusNoContent},
		{name: "disabled", principal: true, evaluator: scimFeatureEvaluatorStub{evaluation: &models.FeatureFlagEvaluation{}}, wantStatus: http.StatusForbidden},
		{name: "subscription", principal: true, evaluator: scimFeatureEvaluatorStub{evaluation: &models.FeatureFlagEvaluation{Reason: "subscription_denied"}}, wantStatus: http.StatusPaymentRequired},
		{name: "unavailable", principal: true, evaluator: scimFeatureEvaluatorStub{err: errors.New("database unavailable")}, wantStatus: http.StatusServiceUnavailable},
		{name: "unauthenticated", evaluator: scimFeatureEvaluatorStub{evaluation: &models.FeatureFlagEvaluation{Enabled: true}}, wantStatus: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.principal {
				ctx = context.WithValue(ctx, ContextKeyOrgID, "organization-id")
				ctx = context.WithValue(ctx, ContextKeySCIMTokenID, "token-id")
			}
			request := httptest.NewRequest(http.MethodGet, "/api/scim/v2/Users", nil).WithContext(ctx)
			response := httptest.NewRecorder()
			RequireSCIMFeature(test.evaluator, "api_access")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.wantStatus != http.StatusNoContent && response.Header().Get("Content-Type") != "application/scim+json" {
				t.Fatalf("content type=%q", response.Header().Get("Content-Type"))
			}
		})
	}
}

func validSCIMPrincipal() *authdomain.SCIMPrincipal {
	return &authdomain.SCIMPrincipal{TokenID: "token", OrganizationID: "org", CreatedByUserID: "creator", RateLimitPerMinute: 60}
}
