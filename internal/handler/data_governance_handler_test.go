package handler

import (
	"context"
	"encoding/json"
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
	governanceHandlerOrg      = "74000000-0000-0000-0000-000000000001"
	governanceHandlerActor    = "74000000-0000-0000-0000-000000000002"
	governanceHandlerSchedule = "74000000-0000-0000-0000-000000000003"
	governanceHandlerHold     = "74000000-0000-0000-0000-000000000004"
)

type dataGovernanceHandlerService struct {
	DataGovernanceService
	policyInput       models.DataGovernancePolicyInput
	policyOrg         string
	policyActor       string
	policyRequest     string
	policyErr         error
	scheduleFilter    models.RetentionScheduleFilter
	activeOnly        bool
	listRecordsCalled bool
}

func (s *dataGovernanceHandlerService) GetPolicy(context.Context, string) (*models.DataGovernancePolicy, error) {
	if s.policyErr != nil {
		return nil, s.policyErr
	}
	return &models.DataGovernancePolicy{OrganizationID: governanceHandlerOrg, Version: 1}, nil
}

func (s *dataGovernanceHandlerService) UpsertPolicy(_ context.Context, organizationID, actorID, requestID string, input models.DataGovernancePolicyInput) (*models.DataGovernancePolicy, error) {
	s.policyOrg, s.policyActor, s.policyRequest, s.policyInput = organizationID, actorID, requestID, input
	if s.policyErr != nil {
		return nil, s.policyErr
	}
	return &models.DataGovernancePolicy{OrganizationID: organizationID, PrimaryRegion: input.PrimaryRegion, Version: 1}, nil
}

func (s *dataGovernanceHandlerService) ListSchedules(_ context.Context, _ string, filter models.RetentionScheduleFilter) ([]models.RetentionSchedule, int, error) {
	s.scheduleFilter = filter
	return []models.RetentionSchedule{{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: governanceHandlerSchedule}, OrganizationID: governanceHandlerOrg}, Name: "Incident records"}}, 21, nil
}

func (s *dataGovernanceHandlerService) ListLegalHoldRecords(_ context.Context, _, _ string, activeOnly bool) ([]models.LegalHoldRecord, error) {
	s.listRecordsCalled, s.activeOnly = true, activeOnly
	return []models.LegalHoldRecord{}, nil
}

func dataGovernanceHandlerRequest(method, path, body string, params map[string]string) (*http.Request, *httptest.ResponseRecorder) {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, governanceHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, governanceHandlerActor)
	ctx = context.WithValue(ctx, middleware.ContextKeyRequestID, "governance-request-1")
	ctx = handlerAllowedContext(ctx)
	route := chi.NewRouteContext()
	for key, value := range params {
		route.URLParams.Add(key, value)
	}
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	response := httptest.NewRecorder()
	response.Header().Set("X-Request-ID", "governance-request-1")
	return request.WithContext(ctx), response
}

func TestDataGovernanceHandlerReadinessAndStrictPolicyWrites(t *testing.T) {
	if NewDataGovernanceHandler(nil).Ready() {
		t.Fatal("handler reported ready without its service")
	}
	var typedNilService *service.DataGovernanceService
	if NewDataGovernanceHandler(typedNilService).Ready() {
		t.Fatal("handler reported ready with a typed nil service")
	}
	stub := &dataGovernanceHandlerService{}
	handler := NewDataGovernanceHandler(stub)
	if !handler.Ready() {
		t.Fatal("handler did not report ready with a service")
	}
	body := `{"primary_region":"eu-west-1","allowed_regions":["eu-west-1"],"cross_border_transfer_mode":"prohibited","default_retention_days":365,"deletion_grace_days":7,"disposition_approval_mode":"single","reason":"Establish residency"}`
	request, response := dataGovernanceHandlerRequest(http.MethodPut, "/api/v1/settings/data-governance/policy", body, nil)
	handler.UpsertPolicy(response, request)
	if response.Code != http.StatusCreated || stub.policyOrg != governanceHandlerOrg || stub.policyActor != governanceHandlerActor ||
		stub.policyRequest != "governance-request-1" || stub.policyInput.PrimaryRegion != "eu-west-1" {
		t.Fatalf("status=%d org=%q actor=%q request=%q input=%#v body=%s", response.Code, stub.policyOrg, stub.policyActor, stub.policyRequest, stub.policyInput, response.Body.String())
	}
	request, response = dataGovernanceHandlerRequest(http.MethodPut, "/api/v1/settings/data-governance/policy", strings.TrimSuffix(body, "}")+`,"unknown":true}`, nil)
	handler.UpsertPolicy(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDataGovernanceHandlerReturnsCanonicalPagination(t *testing.T) {
	stub := &dataGovernanceHandlerService{}
	handler := NewDataGovernanceHandler(stub)
	request, response := dataGovernanceHandlerRequest(http.MethodGet, "/api/v1/settings/data-governance/retention-schedules?page=2&page_size=10&record_type=risk&status=active&sort_by=name&sort_direction=asc", "", nil)
	handler.ListSchedules(response, request)
	if response.Code != http.StatusOK || stub.scheduleFilter.Page != 2 || stub.scheduleFilter.PageSize != 10 ||
		stub.scheduleFilter.RecordType != "risk" || stub.scheduleFilter.Status != "active" || stub.scheduleFilter.SortBy != "name" {
		t.Fatalf("status=%d filter=%#v body=%s", response.Code, stub.scheduleFilter, response.Body.String())
	}
	var payload struct {
		Data       []models.RetentionSchedule `json:"data"`
		Pagination models.PaginationResponse  `json:"pagination"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Pagination.Page != 2 || payload.Pagination.PageSize != 10 ||
		payload.Pagination.TotalItems != 21 || payload.Pagination.TotalPages != 3 {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestDataGovernanceHandlerValidatesLegalHoldBoolean(t *testing.T) {
	stub := &dataGovernanceHandlerService{}
	handler := NewDataGovernanceHandler(stub)
	request, response := dataGovernanceHandlerRequest(http.MethodGet, "/api/v1/settings/data-governance/legal-holds/"+governanceHandlerHold+"/records?active_only=sometimes", "", map[string]string{"holdID": governanceHandlerHold})
	handler.ListLegalHoldRecords(response, request)
	if response.Code != http.StatusBadRequest || stub.listRecordsCalled {
		t.Fatalf("invalid status=%d called=%t body=%s", response.Code, stub.listRecordsCalled, response.Body.String())
	}
	request, response = dataGovernanceHandlerRequest(http.MethodGet, "/api/v1/settings/data-governance/legal-holds/"+governanceHandlerHold+"/records?active_only=false", "", map[string]string{"holdID": governanceHandlerHold})
	handler.ListLegalHoldRecords(response, request)
	if response.Code != http.StatusOK || !stub.listRecordsCalled || stub.activeOnly {
		t.Fatalf("valid status=%d called=%t activeOnly=%t body=%s", response.Code, stub.listRecordsCalled, stub.activeOnly, response.Body.String())
	}
}

func TestDataGovernanceHandlerMapsDomainErrorsWithoutLeaks(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid", err: errors.Join(service.ErrDataGovernanceInvalid, errors.New("region is invalid")), status: http.StatusBadRequest, code: "invalid_request"},
		{name: "missing", err: service.ErrDataGovernanceNotFound, status: http.StatusNotFound, code: "resource_not_found"},
		{name: "version", err: service.ErrDataGovernanceConflict, status: http.StatusConflict, code: "state_conflict"},
		{name: "state", err: service.ErrDataGovernanceState, status: http.StatusConflict, code: "state_conflict"},
		{name: "internal", err: errors.New("postgres://user:secret@example.test"), status: http.StatusInternalServerError, code: "internal_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &dataGovernanceHandlerService{policyErr: test.err}
			handler := NewDataGovernanceHandler(stub)
			request, response := dataGovernanceHandlerRequest(http.MethodGet, "/api/v1/settings/data-governance/policy", "", nil)
			handler.GetPolicy(response, request)
			var payload models.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if response.Code != test.status || payload.ErrorCode != test.code || payload.RequestID != "governance-request-1" ||
				strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "postgres") {
				t.Fatalf("status=%d payload=%#v body=%s", response.Code, payload, response.Body.String())
			}
		})
	}
}
