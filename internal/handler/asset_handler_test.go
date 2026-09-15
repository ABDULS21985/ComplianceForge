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
	assetHandlerOrgID  = "41000000-0000-0000-0000-000000000001"
	assetHandlerUserID = "42000000-0000-0000-0000-000000000001"
	assetHandlerID     = "43000000-0000-0000-0000-000000000001"
)

type assetHandlerServiceStub struct {
	createOrgID  string
	createActor  string
	createInput  models.AssetCreateInput
	createErr    error
	listFilter   models.AssetListFilter
	deleteID     string
	deleteActor  string
	deleteVer    *int64
	listResponse []models.Asset
	listTotal    int
}

func (s *assetHandlerServiceStub) Create(_ context.Context, orgID, actorID string, input models.AssetCreateInput) (*models.Asset, error) {
	s.createOrgID, s.createActor, s.createInput = orgID, actorID, input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.Asset{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: assetHandlerID}, OrganizationID: orgID},
		AssetRef:    "AST-000001", Name: input.Name, AssetType: input.AssetType,
		Criticality: input.Criticality, Classification: input.Classification,
		Status: models.AssetStatusActive, Version: 1, CreatedBy: actorID,
	}, nil
}

func (s *assetHandlerServiceStub) GetByID(_ context.Context, orgID, id string) (*models.Asset, error) {
	return &models.Asset{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: id}, OrganizationID: orgID}}, nil
}

func (s *assetHandlerServiceStub) Update(_ context.Context, orgID, id, actorID string, patch models.AssetPatch) (*models.Asset, error) {
	return &models.Asset{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: id}, OrganizationID: orgID}, Version: 2, CreatedBy: actorID}, nil
}

func (s *assetHandlerServiceStub) Delete(_ context.Context, _ string, id, actorID string, version *int64) error {
	s.deleteID, s.deleteActor, s.deleteVer = id, actorID, version
	return nil
}

func (s *assetHandlerServiceStub) List(_ context.Context, _ string, filter models.AssetListFilter) ([]models.Asset, int, error) {
	s.listFilter = filter
	return s.listResponse, s.listTotal, nil
}

func (s *assetHandlerServiceStub) Stats(context.Context, string) (*models.AssetStats, error) {
	return &models.AssetStats{Total: 2, Active: 2}, nil
}

func (s *assetHandlerServiceStub) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error) {
	return []models.AssetLifecycleEvent{}, 0, nil
}

func assetHandlerRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, assetHandlerOrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, assetHandlerUserID)
	ctx = handlerAllowedContext(ctx)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", assetHandlerID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
	return request.WithContext(ctx)
}

func TestAssetHandlerCreateUsesTrustedIdentityAndStrictJSON(t *testing.T) {
	stub := &assetHandlerServiceStub{}
	handler := NewAssetHandler(stub)
	response := httptest.NewRecorder()
	handler.Create(response, assetHandlerRequest(http.MethodPost, "/assets", `{
		"name":"Customer database","asset_type":"data","criticality":"critical",
		"classification":"restricted"}`))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.createOrgID != assetHandlerOrgID || stub.createActor != assetHandlerUserID {
		t.Fatalf("untrusted identity propagation org=%q actor=%q", stub.createOrgID, stub.createActor)
	}

	response = httptest.NewRecorder()
	handler.Create(response, assetHandlerRequest(http.MethodPost, "/assets", `{
		"name":"Customer database","asset_type":"data","organization_id":"attacker"}`))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown field") {
		t.Fatalf("unknown-field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAssetHandlerParsesBoundedFiltersAndPagination(t *testing.T) {
	stub := &assetHandlerServiceStub{listResponse: []models.Asset{}, listTotal: 41}
	handler := NewAssetHandler(stub)
	response := httptest.NewRecorder()
	handler.List(response, assetHandlerRequest(http.MethodGet,
		"/assets?page=2&page_size=20&asset_type=data&criticality=high&classification=restricted&status=active&processes_personal_data=true&sort_by=name&sort_dir=asc&tag=pii&search=customer", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.listFilter.Page != 2 || stub.listFilter.PageSize != 20 || stub.listFilter.ProcessesPersonalData == nil || !*stub.listFilter.ProcessesPersonalData {
		t.Fatalf("parsed filter=%#v", stub.listFilter)
	}
	if !strings.Contains(response.Body.String(), `"total_items":41`) || !strings.Contains(response.Body.String(), `"total_pages":3`) {
		t.Fatalf("pagination response=%s", response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.List(response, assetHandlerRequest(http.MethodGet, "/assets?processes_personal_data=sometimes", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid boolean status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAssetHandlerDeleteRequiresPositiveVersion(t *testing.T) {
	stub := &assetHandlerServiceStub{}
	handler := NewAssetHandler(stub)
	response := httptest.NewRecorder()
	handler.Delete(response, assetHandlerRequest(http.MethodDelete, "/assets/"+assetHandlerID+"?expected_version=7", ""))
	if response.Code != http.StatusNoContent || stub.deleteVer == nil || *stub.deleteVer != 7 || stub.deleteActor != assetHandlerUserID {
		t.Fatalf("status=%d version=%v actor=%q", response.Code, stub.deleteVer, stub.deleteActor)
	}

	response = httptest.NewRecorder()
	handler.Delete(response, assetHandlerRequest(http.MethodDelete, "/assets/"+assetHandlerID+"?expected_version=0", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid version status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAssetHandlerMapsConflictsAndRedactsInternalErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid", err: service.ErrInvalidAsset, want: http.StatusBadRequest},
		{name: "owner", err: service.ErrAssetOwnerNotFound, want: http.StatusUnprocessableEntity},
		{name: "conflict", err: service.ErrAssetConflict, want: http.StatusConflict},
		{name: "internal", err: errors.New("pq: password=asset-secret"), want: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &assetHandlerServiceStub{createErr: test.err}
			response := httptest.NewRecorder()
			NewAssetHandler(stub).Create(response, assetHandlerRequest(http.MethodPost, "/assets", `{
				"name":"Customer database","asset_type":"data"}`))
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.want == http.StatusInternalServerError && strings.Contains(response.Body.String(), "asset-secret") {
				t.Fatalf("internal details leaked: %s", response.Body.String())
			}
		})
	}
}
