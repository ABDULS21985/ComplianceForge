package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/models"
)

type featureEvaluatorStub struct {
	evaluation *models.FeatureFlagEvaluation
	err        error
	orgID      string
	key        string
}

type entitlementCheckerStub struct {
	decision  *models.EntitlementLimitDecision
	err       error
	metric    string
	requested int64
}

func (s *entitlementCheckerStub) CheckLimit(_ context.Context, _ string, metric string, requested int64) (*models.EntitlementLimitDecision, error) {
	s.metric, s.requested = metric, requested
	return s.decision, s.err
}

func (s *featureEvaluatorStub) Evaluate(_ context.Context, orgID, key string) (*models.FeatureFlagEvaluation, error) {
	s.orgID, s.key = orgID, key
	return s.evaluation, s.err
}

func featureGateRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/vendors", nil)
	ctx := context.WithValue(request.Context(), ContextKeyOrgID, "10000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, ContextKeyRequestID, "request-feature-gate")
	return request.WithContext(ctx)
}

func TestRequireFeatureAllowsOnlyEnabledEvaluations(t *testing.T) {
	evaluator := &featureEvaluatorStub{evaluation: &models.FeatureFlagEvaluation{Enabled: true, Reason: "enabled"}}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	RequireFeature(evaluator, " Vendor_Management ")(next).ServeHTTP(response, featureGateRequest())
	if response.Code != http.StatusNoContent || !called || evaluator.key != "vendor_management" || evaluator.orgID == "" {
		t.Fatalf("status=%d called=%v org=%q key=%q", response.Code, called, evaluator.orgID, evaluator.key)
	}
	if response.Header().Get("X-Feature-Flag") != "vendor_management" {
		t.Fatalf("feature header=%q", response.Header().Get("X-Feature-Flag"))
	}
}

func TestRequireFeatureFailsClosedWithActionableSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		evaluator  FeatureEvaluator
		request    *http.Request
		wantStatus int
		wantCode   string
	}{
		{name: "subscription", evaluator: &featureEvaluatorStub{evaluation: &models.FeatureFlagEvaluation{Reason: "subscription_denied"}}, request: featureGateRequest(), wantStatus: http.StatusPaymentRequired, wantCode: "ENTITLEMENT_REQUIRED"},
		{name: "disabled", evaluator: &featureEvaluatorStub{evaluation: &models.FeatureFlagEvaluation{Reason: "tenant_disabled"}}, request: featureGateRequest(), wantStatus: http.StatusForbidden, wantCode: "FEATURE_DISABLED"},
		{name: "evaluation error", evaluator: &featureEvaluatorStub{err: errors.New("database secret")}, request: featureGateRequest(), wantStatus: http.StatusServiceUnavailable, wantCode: "FEATURE_EVALUATION_UNAVAILABLE"},
		{name: "missing evaluator", request: featureGateRequest(), wantStatus: http.StatusServiceUnavailable, wantCode: "FEATURE_EVALUATION_UNAVAILABLE"},
		{name: "missing tenant", evaluator: &featureEvaluatorStub{}, request: httptest.NewRequest(http.MethodGet, "/api/v1/vendors", nil), wantStatus: http.StatusUnauthorized, wantCode: "TENANT_CONTEXT_REQUIRED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			response := httptest.NewRecorder()
			RequireFeature(test.evaluator, "vendor_management")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			})).ServeHTTP(response, test.request)
			if response.Code != test.wantStatus || called || !strings.Contains(response.Body.String(), `"error_code":"`+test.wantCode+`"`) {
				t.Fatalf("status=%d called=%v body=%s", response.Code, called, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "database secret") {
				t.Fatalf("internal error leaked: %s", response.Body.String())
			}
			if test.request.Context().Value(ContextKeyRequestID) != nil && !strings.Contains(response.Body.String(), "request-feature-gate") {
				t.Fatalf("request id missing: %s", response.Body.String())
			}
		})
	}
}

func TestRequireEntitlementCapacityIsActionableAndFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name       string
		checker    EntitlementLimitChecker
		wantStatus int
		wantCode   string
		called     bool
	}{
		{name: "available", checker: &entitlementCheckerStub{decision: &models.EntitlementLimitDecision{Allowed: true}}, wantStatus: http.StatusNoContent, called: true},
		{name: "exhausted", checker: &entitlementCheckerStub{decision: &models.EntitlementLimitDecision{Allowed: false}}, wantStatus: http.StatusPaymentRequired, wantCode: "ENTITLEMENT_LIMIT_EXCEEDED"},
		{name: "error", checker: &entitlementCheckerStub{err: errors.New("private database error")}, wantStatus: http.StatusServiceUnavailable, wantCode: "ENTITLEMENT_EVALUATION_UNAVAILABLE"},
		{name: "missing", wantStatus: http.StatusServiceUnavailable, wantCode: "ENTITLEMENT_EVALUATION_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			response := httptest.NewRecorder()
			RequireEntitlementCapacity(test.checker, " USERS ", 2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(response, featureGateRequest())
			if response.Code != test.wantStatus || called != test.called {
				t.Fatalf("status=%d called=%v body=%s", response.Code, called, response.Body.String())
			}
			if test.wantCode != "" && !strings.Contains(response.Body.String(), `"error_code":"`+test.wantCode+`"`) {
				t.Fatalf("missing code %q: %s", test.wantCode, response.Body.String())
			}
			if checker, ok := test.checker.(*entitlementCheckerStub); ok && checker.decision != nil && checker.metric != "users" {
				t.Fatalf("metric=%q requested=%d", checker.metric, checker.requested)
			}
			if strings.Contains(response.Body.String(), "private database") {
				t.Fatalf("internal error leaked: %s", response.Body.String())
			}
		})
	}
}
