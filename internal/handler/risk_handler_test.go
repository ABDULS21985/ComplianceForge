package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type riskHandlerServiceStub struct {
	RiskService
	orgID string
	input models.RiskCreateInput
	err   error
}

func (s *riskHandlerServiceStub) Create(_ context.Context, orgID string, input models.RiskCreateInput) (*models.Risk, error) {
	s.orgID, s.input = orgID, input
	if s.err != nil {
		return nil, s.err
	}
	return &models.Risk{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: "30000000-0000-0000-0000-000000000099"}, OrganizationID: orgID},
		RiskRef:     "RSK-0001", Title: input.Title, Status: models.RiskStatusIdentified,
	}, nil
}

func TestRiskHandlerUsesAuthenticatedTenantAndRejectsUnknownFields(t *testing.T) {
	stub := &riskHandlerServiceStub{}
	h := NewRiskHandler(stub)
	request := httptest.NewRequest(http.MethodPost, "/risks", strings.NewReader(`{"title":"Availability","inherent_likelihood":4,"inherent_impact":5}`))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, riskTestHandlerOrgID)
	ctx = handlerAllowedContext(ctx)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	h.Create(response, request)
	if response.Code != http.StatusCreated || stub.orgID != riskTestHandlerOrgID || stub.input.Title != "Availability" {
		t.Fatalf("status=%d org=%q input=%#v body=%s", response.Code, stub.orgID, stub.input, response.Body.String())
	}

	bad := httptest.NewRequest(http.MethodPost, "/risks", strings.NewReader(`{"title":"Spoof","organization_id":"00000000-0000-0000-0000-000000000000"}`))
	bad = bad.WithContext(ctx)
	badResponse := httptest.NewRecorder()
	h.Create(badResponse, bad)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("unknown tenant field status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}
}

func TestRiskHandlerMapsLifecycleConflict(t *testing.T) {
	stub := &riskHandlerServiceStub{err: service.ErrInvalidRiskTransition}
	h := NewRiskHandler(stub)
	request := httptest.NewRequest(http.MethodPost, "/risks", strings.NewReader(`{"title":"Risk"}`))
	request = request.WithContext(handlerAllowedContext(context.WithValue(request.Context(), middleware.ContextKeyOrgID, riskTestHandlerOrgID)))
	response := httptest.NewRecorder()
	h.Create(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

const riskTestHandlerOrgID = "10000000-0000-0000-0000-000000000099"
