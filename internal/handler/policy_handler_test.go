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

type policyHandlerServiceStub struct {
	PolicyService
	createOrg, createUser string
	createInput           models.PolicyCreateInput
	decisionErr           error
	ackIP                 string
}

func (s *policyHandlerServiceStub) Create(_ context.Context, orgID, userID string, input models.PolicyCreateInput) (*models.Policy, error) {
	s.createOrg, s.createUser, s.createInput = orgID, userID, input
	return &models.Policy{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: policyTestHandlerID}, OrganizationID: orgID}, Title: input.Title}, nil
}

func (s *policyHandlerServiceStub) DecideApproval(context.Context, string, string, string, string, models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error) {
	return nil, s.decisionErr
}

func (s *policyHandlerServiceStub) Acknowledge(_ context.Context, _, _, _, ip string, _ models.PolicyAttestationInput) (*models.PolicyAttestation, error) {
	s.ackIP = ip
	return &models.PolicyAttestation{ID: "99000000-0000-0000-0000-000000000009", Status: "attested"}, nil
}

const (
	policyTestHandlerOrgID  = "11000000-0000-0000-0000-000000000001"
	policyTestHandlerUserID = "22000000-0000-0000-0000-000000000002"
	policyTestHandlerID     = "44000000-0000-0000-0000-000000000004"
)

func policyHandlerRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, policyTestHandlerOrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, policyTestHandlerUserID)
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, "compliance_manager")
	return request.WithContext(ctx)
}

func TestPolicyHandlerCreateIsStrictAndPropagatesTenantIdentity(t *testing.T) {
	stub := &policyHandlerServiceStub{}
	h := NewPolicyHandler(stub)
	router := chi.NewRouter()
	router.Post("/policies", h.Create)

	bad := httptest.NewRecorder()
	router.ServeHTTP(bad, policyHandlerRequest(http.MethodPost, "/policies", `{"title":"Policy","unknown":true}`))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", bad.Code, bad.Body.String())
	}

	good := httptest.NewRecorder()
	router.ServeHTTP(good, policyHandlerRequest(http.MethodPost, "/policies", `{"title":"Policy","initial_version":{"content_text":"content"}}`))
	if good.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", good.Code, good.Body.String())
	}
	if stub.createOrg != policyTestHandlerOrgID || stub.createUser != policyTestHandlerUserID || stub.createInput.Title != "Policy" {
		t.Fatalf("identity/input not propagated: %#v", stub)
	}
}

func TestPolicyHandlerMapsApprovalAssignmentToForbidden(t *testing.T) {
	stub := &policyHandlerServiceStub{decisionErr: service.ErrPolicyApprovalForbidden}
	h := NewPolicyHandler(stub)
	router := chi.NewRouter()
	router.Post("/policies/{id}/approval/decision", h.DecideApproval)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, policyHandlerRequest(http.MethodPost, "/policies/"+policyTestHandlerID+"/approval/decision", `{"decision":"approve"}`))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPolicyHandlerRecordsNormalizedClientIP(t *testing.T) {
	stub := &policyHandlerServiceStub{}
	h := NewPolicyHandler(stub)
	router := chi.NewRouter()
	router.Put("/policies/{id}/acknowledge", h.Acknowledge)
	request := policyHandlerRequest(http.MethodPut, "/policies/"+policyTestHandlerID+"/acknowledge", `{"decision":"attest"}`)
	request.RemoteAddr = "192.0.2.10:443"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || stub.ackIP != "192.0.2.10" {
		t.Fatalf("status=%d ip=%q body=%s", response.Code, stub.ackIP, response.Body.String())
	}
}

func TestWritePolicyErrorContract(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{service.ErrPolicyInvalid, http.StatusBadRequest},
		{service.ErrPolicyNotFound, http.StatusNotFound},
		{service.ErrPolicyApprovalForbidden, http.StatusForbidden},
		{service.ErrPolicyInvalidTransition, http.StatusConflict},
		{errors.New("database unavailable"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		writePolicyError(response, test.err, "failed")
		if response.Code != test.want {
			t.Errorf("error %v status=%d want=%d", test.err, response.Code, test.want)
		}
	}
}
