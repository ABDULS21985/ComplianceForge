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
)

const (
	handlerIncidentOrg  = "10000000-0000-0000-0000-000000000001"
	handlerIncidentUser = "20000000-0000-0000-0000-000000000001"
	handlerIncidentID   = "30000000-0000-0000-0000-000000000001"
)

type incidentHandlerService struct {
	IncidentService
	orgID, actorID string
	createInput    models.IncidentCreateInput
	createErr      error
}

func (s *incidentHandlerService) Create(_ context.Context, orgID, actorID string, input models.IncidentCreateInput) (*models.Incident, error) {
	s.orgID, s.actorID, s.createInput = orgID, actorID, input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.Incident{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: handlerIncidentID}, OrganizationID: orgID},
		IncidentRef: "INC-000001", Title: input.Title, Description: input.Description,
		Category: input.Category, Severity: input.Severity, Status: models.IncidentStatusReported,
		ReporterID: actorID, Version: 1,
	}, nil
}

func (s *incidentHandlerService) Transition(_ context.Context, _, _, _ string, _ models.IncidentTransitionInput) (*models.Incident, error) {
	return &models.Incident{}, nil
}

func incidentHandlerRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, handlerIncidentOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, handlerIncidentUser)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", handlerIncidentID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
	return request.WithContext(ctx)
}

func TestIncidentHandlerUsesTrustedContextAndStrictJSON(t *testing.T) {
	service := &incidentHandlerService{}
	handler := NewIncidentHandler(service)
	response := httptest.NewRecorder()
	handler.Create(response, incidentHandlerRequest(http.MethodPost, "/incidents", `{
		"title":"Database exposure","description":"Snapshot was public",
		"category":"privacy","severity":"high"}`))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.orgID != handlerIncidentOrg || service.actorID != handlerIncidentUser {
		t.Fatalf("trusted identities org=%q actor=%q", service.orgID, service.actorID)
	}

	response = httptest.NewRecorder()
	handler.Create(response, incidentHandlerRequest(http.MethodPost, "/incidents", `{
		"title":"Database exposure","description":"Snapshot was public",
		"category":"privacy","severity":"high","organization_id":"attacker"}`))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown field") {
		t.Fatalf("unknown-field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestIncidentHandlerRejectsCloseThroughGenericTransition(t *testing.T) {
	handler := NewIncidentHandler(&incidentHandlerService{})
	response := httptest.NewRecorder()
	handler.Transition(response, incidentHandlerRequest(http.MethodPost, "/incidents/"+handlerIncidentID+"/transitions", `{"status":"closed","version":1}`))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "approved close action") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestIncidentHandlerDoesNotLeakInternalErrors(t *testing.T) {
	service := &incidentHandlerService{createErr: errors.New("pq: password=super-secret table=incidents")}
	handler := NewIncidentHandler(service)
	response := httptest.NewRecorder()
	handler.Create(response, incidentHandlerRequest(http.MethodPost, "/incidents", `{
		"title":"Database exposure","description":"Snapshot was public",
		"category":"privacy","severity":"high"}`))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "super-secret") || strings.Contains(response.Body.String(), "pq:") {
		t.Fatalf("internal error leaked: %s", response.Body.String())
	}
}

func TestIncidentHandlerBoundsRequestBody(t *testing.T) {
	handler := NewIncidentHandler(&incidentHandlerService{})
	response := httptest.NewRecorder()
	oversized := `{"title":"` + strings.Repeat("x", 1<<20) + `","description":"x","category":"privacy","severity":"high"}`
	handler.Create(response, incidentHandlerRequest(http.MethodPost, "/incidents", oversized))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
