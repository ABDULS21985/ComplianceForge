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
	vendorHandlerOrg  = "61000000-0000-0000-0000-000000000001"
	vendorHandlerUser = "62000000-0000-0000-0000-000000000001"
	vendorHandlerID   = "63000000-0000-0000-0000-000000000001"
)

type vendorHandlerServiceStub struct {
	VendorService
	createOrg, createActor string
	createInput            models.VendorCreateInput
	createErr              error
	listFilter             models.VendorListFilter
	deleteVersion          int64
	contactInput           models.VendorContactInput
}

func (s *vendorHandlerServiceStub) Create(_ context.Context, orgID, actorID string, input models.VendorCreateInput) (*models.Vendor, error) {
	s.createOrg, s.createActor, s.createInput = orgID, actorID, input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.Vendor{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: vendorHandlerID}, OrganizationID: orgID}, VendorRef: "VND-000001", Name: input.Name, Status: models.VendorStatusProspective, Version: 1}, nil
}
func (s *vendorHandlerServiceStub) GetByID(_ context.Context, orgID, id string) (*models.Vendor, error) {
	return &models.Vendor{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: id}, OrganizationID: orgID}, Contacts: []models.VendorContact{}, Version: 3}, nil
}
func (s *vendorHandlerServiceStub) List(_ context.Context, _ string, filter models.VendorListFilter) ([]models.Vendor, int, error) {
	s.listFilter = filter
	return []models.Vendor{}, 41, nil
}
func (s *vendorHandlerServiceStub) Delete(_ context.Context, _, _, _ string, version int64) error {
	s.deleteVersion = version
	return nil
}
func (s *vendorHandlerServiceStub) SaveContact(_ context.Context, orgID, _, vendorID, contactID string, input models.VendorContactInput) (*models.VendorContact, *models.Vendor, error) {
	s.contactInput = input
	return &models.VendorContact{ID: contactID, OrganizationID: orgID, VendorID: vendorID, Name: input.Name}, &models.Vendor{Version: input.Version + 1}, nil
}

func vendorHandlerRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(r.Context(), middleware.ContextKeyOrgID, vendorHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, vendorHandlerUser)
	ctx = handlerAllowedContext(ctx)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", vendorHandlerID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	return r.WithContext(ctx)
}

func TestVendorHandlerCreateUsesTrustedIdentityAndStrictJSON(t *testing.T) {
	stub := &vendorHandlerServiceStub{}
	h := NewVendorHandler(stub)
	response := httptest.NewRecorder()
	h.Create(response, vendorHandlerRequest(http.MethodPost, "/vendors", `{"name":"Nimbus","risk_tier":"high","contact_name":"Ada","contact_email":"ada@example.test"}`))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.createOrg != vendorHandlerOrg || stub.createActor != vendorHandlerUser {
		t.Fatalf("identity org=%q actor=%q", stub.createOrg, stub.createActor)
	}
	response = httptest.NewRecorder()
	h.Create(response, vendorHandlerRequest(http.MethodPost, "/vendors", `{"name":"Nimbus","organization_id":"attacker"}`))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown field") {
		t.Fatalf("strict JSON status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestVendorHandlerParsesFiltersAndPagination(t *testing.T) {
	stub := &vendorHandlerServiceStub{}
	h := NewVendorHandler(stub)
	response := httptest.NewRecorder()
	h.List(response, vendorHandlerRequest(http.MethodGet, "/vendors?page=2&page_size=20&risk_tier=high&status=active&data_processing=true&due_assessment=false&sort_by=name&sort_dir=asc", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.listFilter.Page != 2 || stub.listFilter.PageSize != 20 || stub.listFilter.DataProcessing == nil || !*stub.listFilter.DataProcessing || stub.listFilter.DueAssessment == nil || *stub.listFilter.DueAssessment {
		t.Fatalf("filter=%#v", stub.listFilter)
	}
	if !strings.Contains(response.Body.String(), `"total_items":41`) || !strings.Contains(response.Body.String(), `"total_pages":3`) {
		t.Fatalf("body=%s", response.Body.String())
	}
	response = httptest.NewRecorder()
	h.List(response, vendorHandlerRequest(http.MethodGet, "/vendors?data_processing=perhaps", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid filter status=%d", response.Code)
	}
}

func TestVendorHandlerRequiresVersionAndStrictRelatedPayload(t *testing.T) {
	stub := &vendorHandlerServiceStub{}
	h := NewVendorHandler(stub)
	response := httptest.NewRecorder()
	h.Delete(response, vendorHandlerRequest(http.MethodDelete, "/vendors/"+vendorHandlerID, ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing version status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	h.Delete(response, vendorHandlerRequest(http.MethodDelete, "/vendors/"+vendorHandlerID+"?version=7", ""))
	if response.Code != http.StatusNoContent || stub.deleteVersion != 7 {
		t.Fatalf("delete status=%d version=%d", response.Code, stub.deleteVersion)
	}
	response = httptest.NewRecorder()
	h.SaveContact(response, vendorHandlerRequest(http.MethodPost, "/vendors/"+vendorHandlerID+"/contacts", `{"version":3,"name":"Ada","email":"ada@example.test","unexpected":true}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("related strict status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestVendorHandlerMapsErrorsAndRedactsInternalDetails(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want int
	}{{"invalid", service.ErrVendorInvalid, http.StatusBadRequest}, {"not_found", service.ErrVendorNotFound, http.StatusNotFound}, {"owner", service.ErrVendorUserNotFound, http.StatusUnprocessableEntity}, {"conflict", service.ErrVendorVersionConflict, http.StatusConflict}, {"internal", errors.New("pq: password=vendor-secret"), http.StatusInternalServerError}} {
		t.Run(test.name, func(t *testing.T) {
			stub := &vendorHandlerServiceStub{createErr: test.err}
			response := httptest.NewRecorder()
			NewVendorHandler(stub).Create(response, vendorHandlerRequest(http.MethodPost, "/vendors", `{"name":"Nimbus"}`))
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.want == http.StatusInternalServerError && strings.Contains(response.Body.String(), "vendor-secret") {
				t.Fatalf("leaked body=%s", response.Body.String())
			}
		})
	}
}
