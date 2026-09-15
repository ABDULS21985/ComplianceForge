package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const (
	featureFlagHandlerOrg   = "71000000-0000-0000-0000-000000000001"
	featureFlagHandlerActor = "72000000-0000-0000-0000-000000000001"
)

type featureFlagHandlerStub struct {
	FeatureFlagService
	orgID, actorID, key, requestID string
	input                          models.FeatureFlagOverrideInput
	reset                          models.FeatureFlagResetInput
	pagination                     models.PaginationRequest
	err                            error
}

func (s *featureFlagHandlerStub) ListEvaluations(context.Context, string) ([]models.FeatureFlagEvaluation, error) {
	return []models.FeatureFlagEvaluation{}, s.err
}

func (s *featureFlagHandlerStub) UpsertOverride(
	_ context.Context, orgID, key, actorID, requestID string, input models.FeatureFlagOverrideInput,
) (*models.TenantFeatureFlagOverride, error) {
	s.orgID, s.key, s.actorID, s.requestID, s.input = orgID, key, actorID, requestID, input
	if s.err != nil {
		return nil, s.err
	}
	return &models.TenantFeatureFlagOverride{
		OrganizationID: orgID, CapabilityKey: key, Enabled: input.Enabled,
		Reason: input.Reason, Version: 1, Variant: map[string]any{},
	}, nil
}

func (s *featureFlagHandlerStub) ResetOverride(
	_ context.Context, orgID, key, actorID, requestID string, input models.FeatureFlagResetInput,
) error {
	s.orgID, s.key, s.actorID, s.requestID, s.reset = orgID, key, actorID, requestID, input
	return s.err
}

func (s *featureFlagHandlerStub) CheckLimit(context.Context, string, string, int64) (*models.EntitlementLimitDecision, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &models.EntitlementLimitDecision{Metric: "users", Allowed: false, Limit: 5, Usage: 5, Requested: 1}, nil
}

func (s *featureFlagHandlerStub) ListEvents(
	_ context.Context, _, _ string, pagination models.PaginationRequest,
) ([]models.FeatureFlagChangeEvent, int, error) {
	s.pagination = pagination
	return []models.FeatureFlagChangeEvent{}, 41, s.err
}

func featureFlagRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, featureFlagHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, featureFlagHandlerActor)
	ctx = context.WithValue(ctx, middleware.ContextKeyRequestID, "request-flag-1")
	ctx = handlerAllowedContext(ctx)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("key", "advanced_reporting")
	routeContext.URLParams.Add("metric", "users")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
	return request.WithContext(ctx)
}

func TestFeatureFlagHandlerUsesTrustedIdentityStrictJSONAndCreateUpdateStatus(t *testing.T) {
	stub := &featureFlagHandlerStub{}
	handler := NewFeatureFlagHandler(stub)
	response := httptest.NewRecorder()
	handler.UpsertOverride(response, featureFlagRequest(http.MethodPut, "/settings/feature-flags/advanced_reporting", `{
		"enabled":true,"reason":"Staged tenant rollout","variant":{"layout":"new"}}`))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.orgID != featureFlagHandlerOrg || stub.actorID != featureFlagHandlerActor || stub.key != "advanced_reporting" || stub.requestID != "request-flag-1" {
		t.Fatalf("identity org=%q actor=%q key=%q request=%q", stub.orgID, stub.actorID, stub.key, stub.requestID)
	}

	response = httptest.NewRecorder()
	handler.UpsertOverride(response, featureFlagRequest(http.MethodPut, "/settings/feature-flags/advanced_reporting", `{
		"enabled":false,"reason":"Pause rollout","expected_version":1}`))
	if response.Code != http.StatusOK || stub.input.ExpectedVersion == nil || *stub.input.ExpectedVersion != 1 {
		t.Fatalf("update status=%d input=%+v body=%s", response.Code, stub.input, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.UpsertOverride(response, featureFlagRequest(http.MethodPut, "/settings/feature-flags/advanced_reporting", `{
		"enabled":true,"reason":"valid reason","organization_id":"attacker"}`))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown field") {
		t.Fatalf("strict status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFeatureFlagHandlerResetLimitAndHistoryContracts(t *testing.T) {
	stub := &featureFlagHandlerStub{}
	handler := NewFeatureFlagHandler(stub)
	response := httptest.NewRecorder()
	handler.ResetOverride(response, featureFlagRequest(http.MethodPost, "/settings/feature-flags/advanced_reporting/reset", `{
		"expected_version":3,"reason":"Return to subscription default"}`))
	if response.Code != http.StatusNoContent || stub.reset.ExpectedVersion != 3 || stub.reset.Reason != "Return to subscription default" {
		t.Fatalf("reset status=%d input=%+v body=%s", response.Code, stub.reset, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.CheckLimit(response, featureFlagRequest(http.MethodGet, "/settings/entitlements/limits/users/check?requested=1", ""))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"allowed":false`) {
		t.Fatalf("limit status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.CheckLimit(response, featureFlagRequest(http.MethodGet, "/settings/entitlements/limits/users/check?requested=bad", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ListEvents(response, featureFlagRequest(http.MethodGet, "/settings/feature-flags/advanced_reporting/history?page=2&page_size=20", ""))
	if response.Code != http.StatusOK || stub.pagination.Page != 2 || stub.pagination.PageSize != 20 || !strings.Contains(response.Body.String(), `"total_items":41`) {
		t.Fatalf("history status=%d pagination=%+v body=%s", response.Code, stub.pagination, response.Body.String())
	}
}

func TestFeatureFlagHandlerMapsSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid", err: service.ErrFeatureFlagInvalid, want: http.StatusBadRequest},
		{name: "not found", err: service.ErrFeatureFlagNotFound, want: http.StatusNotFound},
		{name: "conflict", err: service.ErrFeatureFlagConflict, want: http.StatusConflict},
		{name: "internal", err: errors.New("pq: password=feature-secret"), want: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &featureFlagHandlerStub{err: test.err}
			response := httptest.NewRecorder()
			NewFeatureFlagHandler(stub).UpsertOverride(response, featureFlagRequest(http.MethodPut, "/settings/feature-flags/advanced_reporting", `{
				"enabled":true,"reason":"valid reason"}`))
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.want == http.StatusInternalServerError && strings.Contains(response.Body.String(), "feature-secret") {
				t.Fatalf("internal details leaked: %s", response.Body.String())
			}
		})
	}
}
