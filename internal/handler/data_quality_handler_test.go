package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type dataQualityHandlerProvider struct {
	value *models.DataQualitySnapshot
	err   error
	calls int
}

func (p *dataQualityHandlerProvider) GetSnapshot(_ context.Context, organizationID, actorID string) (*models.DataQualitySnapshot, error) {
	p.calls++
	if organizationID != classifiedOrgID || actorID != classifiedUserID {
		return nil, errors.New("unexpected principal binding")
	}
	return p.value, p.err
}

func dataQualityHandlerSnapshot() *models.DataQualitySnapshot {
	result := &models.DataQualitySnapshot{
		RulesetVersion: models.DataQualityRulesetVersion, Scope: models.DataQualityScope,
		SchemaVersion: database.SupportedSchemaVersion, AsOf: time.Date(2026, 9, 15, 1, 2, 3, 456000000, time.UTC),
		Status: models.DataQualityCritical,
	}
	for index, definition := range models.DataQualityCheckDefinitions() {
		check := models.DataQualityCheck{DataQualityCheckDefinition: definition, Status: models.DataQualityHealthy}
		if index == 0 {
			check.Count, check.Status = 917213, models.DataQualityCritical
		}
		result.Checks = append(result.Checks, check)
	}
	return result
}

func dataQualityHandlerRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/settings/data-quality", nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, classifiedOrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, classifiedUserID)
	ctx = context.WithValue(ctx, middleware.ContextKeyRequestID, "data-quality-request-1")
	return request.WithContext(handlerAllowedContext(ctx))
}

func assertDataQualitySafeError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var envelope models.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Code != status || envelope.ErrorCode != code || envelope.RequestID != "data-quality-request-1" {
		t.Fatalf("canonical safe envelope drift: %#v error=%v body=%s", envelope, err, response.Body.String())
	}
	for _, forbidden := range []string{"917213", `"checks"`, `"data"`, `"status"`, "credential", "secret", classifiedOrgID, classifiedUserID} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("aggregate, principal, or internal failure leaked %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestDataQualityHandlerSerializesOnlyFixedSafeCountsWithoutMutation(t *testing.T) {
	for _, visibility := range []bool{false, true} {
		t.Run(map[bool]string{false: "unconstrained allow", true: "visible obligations"}[visibility], func(t *testing.T) {
			value := dataQualityHandlerSnapshot()
			before := *value
			before.Checks = append([]models.DataQualityCheck(nil), value.Checks...)
			provider := &dataQualityHandlerProvider{value: value}
			request := dataQualityHandlerRequest()
			if visibility {
				rule := classifiedRule("settings", "checks.count", models.AccessFieldVisible, "")
				request = request.WithContext(middleware.ContextWithAuthorizationDecision(request.Context(), authz.Decision{
					Allowed: true, Obligations: service.AccessFieldObligations([]models.AccessFieldPermission{rule}),
				}))
			}
			response := httptest.NewRecorder()
			NewDataQualityHandler(provider).GetSnapshot(response, request)
			if response.Code != http.StatusOK || provider.calls != 1 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.calls, response.Body.String())
			}
			var body struct {
				Data models.DataQualitySnapshot `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || !service.ValidateDataQualitySnapshot(&body.Data) {
				t.Fatalf("real handler JSON failed typed contract: %#v error=%v", body, err)
			}
			if !reflect.DeepEqual(value, &before) || !reflect.DeepEqual(body.Data, before) {
				t.Fatal("handler mutated or changed the typed provider snapshot")
			}
			for _, forbidden := range []string{classifiedOrgID, classifiedUserID, "organization_id", "actor_id", "record_id", "metadata", "entity_id"} {
				if strings.Contains(response.Body.String(), forbidden) {
					t.Fatalf("unsafe response field leaked %q", forbidden)
				}
			}
		})
	}
}

func TestDataQualityHandlerWithholdsAllDerivedDataBeforeHiddenMaskedOrUnsupportedObligations(t *testing.T) {
	hidden := service.AccessFieldObligations([]models.AccessFieldPermission{
		classifiedRule("settings", "checks.count", models.AccessFieldHidden, ""),
	})
	masked := service.AccessFieldObligations([]models.AccessFieldPermission{
		classifiedRule("settings", "data.checks.count", models.AccessFieldMasked, models.AccessMaskRedact),
	})
	foreign := service.AccessFieldObligations([]models.AccessFieldPermission{
		classifiedRule("risks", "checks.count", models.AccessFieldVisible, ""),
	})
	unknown := service.AccessFieldObligations([]models.AccessFieldPermission{
		classifiedRule("settings", "checks.count", models.AccessFieldVisible, ""),
	})
	unknown[0].Parameters["future_derived_status"] = "credential-secret"
	for _, test := range []struct {
		name     string
		decision *authz.Decision
	}{
		{"missing decision", nil},
		{"denied decision", &authz.Decision{Allowed: false}},
		{"hidden nested count", &authz.Decision{Allowed: true, Obligations: hidden}},
		{"masked array count", &authz.Decision{Allowed: true, Obligations: masked}},
		{"hidden overall status", &authz.Decision{Allowed: true, Obligations: service.AccessFieldObligations([]models.AccessFieldPermission{classifiedRule("settings", "status", models.AccessFieldHidden, "")})}},
		{"unrelated hidden settings field conservative denial", &authz.Decision{Allowed: true, Obligations: service.AccessFieldObligations([]models.AccessFieldPermission{classifiedRule("settings", "metadata", models.AccessFieldHidden, "")})}},
		{"foreign resource", &authz.Decision{Allowed: true, Obligations: foreign}},
		{"nonfield watermark", &authz.Decision{Allowed: true, Obligations: []authz.Obligation{{Kind: "watermark", Parameters: map[string]string{"text": "credential-secret"}}}}},
		{"malformed parameters", &authz.Decision{Allowed: true, Obligations: []authz.Obligation{{Kind: service.AccessFieldVisibilityObligation}}}},
		{"unsupported field parameter", &authz.Decision{Allowed: true, Obligations: unknown}},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &dataQualityHandlerProvider{value: dataQualityHandlerSnapshot()}
			request := dataQualityHandlerRequest()
			if test.decision == nil {
				ctx := context.WithValue(context.Background(), middleware.ContextKeyOrgID, classifiedOrgID)
				ctx = context.WithValue(ctx, middleware.ContextKeyUserID, classifiedUserID)
				ctx = context.WithValue(ctx, middleware.ContextKeyRequestID, "data-quality-request-1")
				request = request.WithContext(ctx)
			} else {
				request = request.WithContext(middleware.ContextWithAuthorizationDecision(request.Context(), *test.decision))
			}
			response := httptest.NewRecorder()
			NewDataQualityHandler(provider).GetSnapshot(response, request)
			assertDataQualitySafeError(t, response, http.StatusServiceUnavailable, "response_masking_unavailable")
			if provider.calls != 0 {
				t.Fatal("restricted derived data was queried before preflight")
			}
		})
	}
}

func TestDataQualityHandlerRejectsArbitraryRuleScopeOrBodyBeforeQuery(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"unknown check", func(request *http.Request) { request.URL.RawQuery = "check=unknown" }},
		{"pagination sampling", func(request *http.Request) { request.URL.RawQuery = "page_size=1" }},
		{"alternate tenant", func(request *http.Request) { request.URL.RawQuery = "organization_id=" + classifiedItemID }},
		{"fixed scope override", func(request *http.Request) { request.URL.RawQuery = "scope=all" }},
		{"request body", func(request *http.Request) { request.ContentLength = 1 }},
		{"unknown body length", func(request *http.Request) { request.ContentLength = -1 }},
		{"chunked body", func(request *http.Request) { request.TransferEncoding = []string{"chunked"} }},
		{"write method", func(request *http.Request) { request.Method = http.MethodPost }},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &dataQualityHandlerProvider{value: dataQualityHandlerSnapshot()}
			request := dataQualityHandlerRequest()
			test.mutate(request)
			response := httptest.NewRecorder()
			NewDataQualityHandler(provider).GetSnapshot(response, request)
			assertDataQualitySafeError(t, response, http.StatusBadRequest, "data_quality_invalid_scope")
			if provider.calls != 0 {
				t.Fatal("unknown rules or non-read-only request reached provider")
			}
		})
	}
}

func TestDataQualityHandlerSafeErrorsRejectInactiveScopeAndUnsafeProviderResults(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*dataQualityHandlerProvider)
		status int
		code   string
	}{
		{"inactive or unbound actor", func(provider *dataQualityHandlerProvider) { provider.err = service.ErrDataQualityScope }, http.StatusForbidden, "data_quality_scope_denied"},
		{"query error redacted", func(provider *dataQualityHandlerProvider) { provider.err = errors.New("credential secret record") }, http.StatusServiceUnavailable, "data_quality_unavailable"},
		{"nil provider result", func(provider *dataQualityHandlerProvider) { provider.value = nil }, http.StatusServiceUnavailable, "data_quality_unavailable"},
		{"missing check", func(provider *dataQualityHandlerProvider) { provider.value.Checks = provider.value.Checks[:17] }, http.StatusServiceUnavailable, "data_quality_unavailable"},
		{"injected business definition", func(provider *dataQualityHandlerProvider) {
			provider.value.Checks[0].Definition = "credential secret record"
		}, http.StatusServiceUnavailable, "data_quality_unavailable"},
		{"false healthy status", func(provider *dataQualityHandlerProvider) { provider.value.Status = models.DataQualityHealthy }, http.StatusServiceUnavailable, "data_quality_unavailable"},
		{"unsafe JS count", func(provider *dataQualityHandlerProvider) {
			provider.value.Checks[0].Count = models.MaximumDataQualityCount + 1
		}, http.StatusServiceUnavailable, "data_quality_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &dataQualityHandlerProvider{value: dataQualityHandlerSnapshot()}
			test.mutate(provider)
			response := httptest.NewRecorder()
			NewDataQualityHandler(provider).GetSnapshot(response, dataQualityHandlerRequest())
			assertDataQualitySafeError(t, response, test.status, test.code)
		})
	}
	for _, provider := range []DataQualityService{nil, (*dataQualityHandlerProvider)(nil)} {
		handler := NewDataQualityHandler(provider)
		if handler.Ready() {
			t.Fatal("nil or typed-nil provider is ready")
		}
		response := httptest.NewRecorder()
		handler.GetSnapshot(response, dataQualityHandlerRequest())
		assertDataQualitySafeError(t, response, http.StatusServiceUnavailable, "data_quality_unavailable")
	}
}

func TestDataQualityHandlerRequiresBothAuthenticatedPrincipals(t *testing.T) {
	for _, missing := range []string{"organization", "actor"} {
		t.Run(missing, func(t *testing.T) {
			request := dataQualityHandlerRequest()
			ctx := request.Context()
			if missing == "organization" {
				ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, "")
			} else {
				ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "")
			}
			provider := &dataQualityHandlerProvider{value: dataQualityHandlerSnapshot()}
			response := httptest.NewRecorder()
			NewDataQualityHandler(provider).GetSnapshot(response, request.WithContext(ctx))
			assertDataQualitySafeError(t, response, http.StatusUnauthorized, "authentication_required")
			if provider.calls != 0 {
				t.Fatal("unauthenticated diagnostic queried provider")
			}
		})
	}
}
