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
	accessAdminHandlerOrg  = "61000000-0000-0000-0000-000000000001"
	accessAdminHandlerUser = "62000000-0000-0000-0000-000000000001"
	accessAdminHandlerRole = "63000000-0000-0000-0000-000000000001"
	accessAdminTargetUser  = "64000000-0000-0000-0000-000000000001"
)

type accessAdminHandlerStub struct {
	AccessAdministrationService
	orgID, actorID string
	createInput    models.ManagedRoleCreateInput
	createErr      error
	listFilter     models.ManagedRoleListFilter
	deleteVersion  int64
	deleteErr      error
}

func (s *accessAdminHandlerStub) CreateRole(_ context.Context, orgID, actorID string, input models.ManagedRoleCreateInput) (*models.ManagedRole, error) {
	s.orgID, s.actorID, s.createInput = orgID, actorID, input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.ManagedRole{ID: accessAdminHandlerRole, OrganizationID: &orgID, Name: input.Name, Slug: input.Slug, IsCustom: true, Version: 1}, nil
}

func (s *accessAdminHandlerStub) ListRoles(_ context.Context, _ string, filter models.ManagedRoleListFilter) ([]models.ManagedRole, int, error) {
	s.listFilter = filter
	return []models.ManagedRole{}, 45, nil
}

func (s *accessAdminHandlerStub) DeleteRole(_ context.Context, _, _, _ string, version int64) error {
	s.deleteVersion = version
	return s.deleteErr
}

func accessAdminRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, accessAdminHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, accessAdminHandlerUser)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", accessAdminHandlerRole)
	routeContext.URLParams.Add("userID", accessAdminTargetUser)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
	return request.WithContext(ctx)
}

func TestAccessAdminHandlerUsesTrustedIdentityAndStrictJSON(t *testing.T) {
	stub := &accessAdminHandlerStub{}
	handler := NewAccessAdministrationHandler(stub)
	response := httptest.NewRecorder()
	handler.CreateRole(response, accessAdminRequest(http.MethodPost, "/access/roles", `{
		"name":"Security reviewer","permissions":[{"resource":"risks","action":"read"}]}`))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.orgID != accessAdminHandlerOrg || stub.actorID != accessAdminHandlerUser {
		t.Fatalf("identity org=%q actor=%q", stub.orgID, stub.actorID)
	}

	response = httptest.NewRecorder()
	handler.CreateRole(response, accessAdminRequest(http.MethodPost, "/access/roles", `{
		"name":"Security reviewer","organization_id":"attacker"}`))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown field") {
		t.Fatalf("unknown-field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAccessAdminHandlerParsesRoleFiltersAndPagination(t *testing.T) {
	stub := &accessAdminHandlerStub{}
	response := httptest.NewRecorder()
	NewAccessAdministrationHandler(stub).ListRoles(response, accessAdminRequest(http.MethodGet,
		"/access/roles?page=2&page_size=20&include_system=false&search=reviewer", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.listFilter.Page != 2 || stub.listFilter.PageSize != 20 || stub.listFilter.IncludeSystem || stub.listFilter.Search != "reviewer" {
		t.Fatalf("filter=%#v", stub.listFilter)
	}
	if !strings.Contains(response.Body.String(), `"total_items":45`) || !strings.Contains(response.Body.String(), `"total_pages":3`) {
		t.Fatalf("pagination=%s", response.Body.String())
	}

	response = httptest.NewRecorder()
	NewAccessAdministrationHandler(stub).ListRoles(response, accessAdminRequest(http.MethodGet, "/access/roles?include_system=maybe", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid boolean status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAccessAdminHandlerRequiresDeleteVersion(t *testing.T) {
	stub := &accessAdminHandlerStub{}
	handler := NewAccessAdministrationHandler(stub)
	for _, target := range []string{"/access/roles/" + accessAdminHandlerRole, "/access/roles/" + accessAdminHandlerRole + "?expected_version=0"} {
		response := httptest.NewRecorder()
		handler.DeleteRole(response, accessAdminRequest(http.MethodDelete, target, ""))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target=%s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	handler.DeleteRole(response, accessAdminRequest(http.MethodDelete, "/access/roles/"+accessAdminHandlerRole+"?expected_version=9", ""))
	if response.Code != http.StatusNoContent || stub.deleteVersion != 9 {
		t.Fatalf("status=%d version=%d body=%s", response.Code, stub.deleteVersion, response.Body.String())
	}
}

func TestAccessAdminHandlerMapsSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid", err: service.ErrManagedRoleInvalid, want: http.StatusBadRequest},
		{name: "not found", err: service.ErrManagedRoleNotFound, want: http.StatusNotFound},
		{name: "conflict", err: service.ErrManagedRoleConflict, want: http.StatusConflict},
		{name: "immutable", err: service.ErrManagedRoleImmutable, want: http.StatusConflict},
		{name: "permission", err: service.ErrManagedRolePermission, want: http.StatusUnprocessableEntity},
		{name: "internal", err: errors.New("pq: password=access-secret"), want: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &accessAdminHandlerStub{createErr: test.err}
			response := httptest.NewRecorder()
			NewAccessAdministrationHandler(stub).CreateRole(response, accessAdminRequest(http.MethodPost, "/access/roles", `{"name":"Reviewer"}`))
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.want == http.StatusInternalServerError && strings.Contains(response.Body.String(), "access-secret") {
				t.Fatalf("internal details leaked: %s", response.Body.String())
			}
		})
	}
}
