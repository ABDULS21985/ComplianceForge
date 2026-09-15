package handler

import (
	"bytes"
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
	directoryHandlerOrg   = "72000000-0000-0000-0000-000000000001"
	directoryHandlerActor = "72000000-0000-0000-0000-000000000002"
	directoryHandlerUser  = "72000000-0000-0000-0000-000000000003"
	directoryHandlerGroup = "72000000-0000-0000-0000-000000000004"
)

type directoryHandlerService struct {
	UserAdministrationService
	createInput    models.DirectoryUserCreateInput
	createOrg      string
	createActor    string
	createErr      error
	deleteVersion  int64
	deleteReason   string
	previewContent []byte
	applyKey       string
	applyReason    string
}

func (s *directoryHandlerService) CreateUser(_ context.Context, orgID, actorID string, input models.DirectoryUserCreateInput) (*models.DirectoryUser, error) {
	s.createOrg, s.createActor, s.createInput = orgID, actorID, input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.DirectoryUser{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: directoryHandlerUser}, OrganizationID: orgID}, Email: input.Email, Version: 1}, nil
}

func (s *directoryHandlerService) ListUsers(context.Context, string, models.DirectoryUserListFilter) ([]models.DirectoryUser, int, error) {
	return []models.DirectoryUser{}, 0, nil
}

func (s *directoryHandlerService) DeleteGroup(_ context.Context, _, _, _ string, version int64, reason string) error {
	s.deleteVersion, s.deleteReason = version, reason
	return nil
}

func (s *directoryHandlerService) PreviewImport(_ context.Context, _ string, content []byte) (*models.DirectoryImportPreview, error) {
	s.previewContent = content
	return &models.DirectoryImportPreview{RowCount: 1, ValidCount: 1}, nil
}

func (s *directoryHandlerService) ApplyImport(_ context.Context, _, _, key, reason string, _ []byte) (*models.DirectoryImportResult, error) {
	s.applyKey, s.applyReason = key, reason
	return &models.DirectoryImportResult{ID: directoryHandlerGroup, IdempotencyKey: key, RowCount: 1}, nil
}

func directoryHandlerRequest(method, path, body string, params map[string]string) (*http.Request, *httptest.ResponseRecorder) {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, directoryHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, directoryHandlerActor)
	ctx = handlerAllowedContext(ctx)
	route := chi.NewRouteContext()
	for key, value := range params {
		route.URLParams.Add(key, value)
	}
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	return request.WithContext(ctx), httptest.NewRecorder()
}

func TestUserAdministrationHandlerCreateIsStrictAndTrustedContextScoped(t *testing.T) {
	stub := &directoryHandlerService{}
	handler := NewUserAdministrationHandler(stub)
	request, response := directoryHandlerRequest(http.MethodPost, "/api/v1/directory/users", `{"email":"ada@example.test","first_name":"Ada","reason":"Create user"}`, nil)
	handler.CreateUser(response, request)
	if response.Code != http.StatusCreated || stub.createOrg != directoryHandlerOrg || stub.createActor != directoryHandlerActor || stub.createInput.Email != "ada@example.test" {
		t.Fatalf("status=%d org=%q actor=%q input=%#v body=%s", response.Code, stub.createOrg, stub.createActor, stub.createInput, response.Body.String())
	}
	request, response = directoryHandlerRequest(http.MethodPost, "/api/v1/directory/users", `{"email":"ada@example.test","reason":"Create user","unknown":true}`, nil)
	handler.CreateUser(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUserAdministrationHandlerMapsCapacityWithoutLeakingDetails(t *testing.T) {
	stub := &directoryHandlerService{createErr: fmtDirectoryCapacityError()}
	handler := NewUserAdministrationHandler(stub)
	request, response := directoryHandlerRequest(http.MethodPost, "/api/v1/directory/users", `{"email":"ada@example.test","first_name":"Ada","reason":"Create user"}`, nil)
	handler.CreateUser(response, request)
	if response.Code != http.StatusPaymentRequired || strings.Contains(response.Body.String(), "metric=") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func fmtDirectoryCapacityError() error {
	return errors.Join(service.ErrSubscriptionLimitExceeded, errors.New("metric=users limit=5 usage=5"))
}

func TestUserAdministrationHandlerDeleteGroupRequiresVersionAndReason(t *testing.T) {
	stub := &directoryHandlerService{}
	handler := NewUserAdministrationHandler(stub)
	request, response := directoryHandlerRequest(http.MethodDelete, "/api/v1/directory/groups/"+directoryHandlerGroup+"?expected_version=4", "", map[string]string{"groupID": directoryHandlerGroup})
	handler.DeleteGroup(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing reason status=%d body=%s", response.Code, response.Body.String())
	}
	request, response = directoryHandlerRequest(http.MethodDelete, "/api/v1/directory/groups/"+directoryHandlerGroup+"?expected_version=4", "", map[string]string{"groupID": directoryHandlerGroup})
	request.Header.Set("X-Change-Reason", "Group is no longer needed")
	handler.DeleteGroup(response, request)
	if response.Code != http.StatusNoContent || stub.deleteVersion != 4 || stub.deleteReason != "Group is no longer needed" {
		t.Fatalf("status=%d version=%d reason=%q", response.Code, stub.deleteVersion, stub.deleteReason)
	}
}

func TestUserAdministrationHandlerCSVPreviewAndApplyHeaders(t *testing.T) {
	stub := &directoryHandlerService{}
	handler := NewUserAdministrationHandler(stub)
	csvBody := "email,first_name\nada@example.test,Ada\n"
	request, response := directoryHandlerRequest(http.MethodPost, "/api/v1/directory/users/import/preview", csvBody, nil)
	handler.PreviewImport(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(stub.previewContent, []byte(csvBody)) {
		t.Fatalf("preview status=%d content=%q body=%s", response.Code, stub.previewContent, response.Body.String())
	}
	request, response = directoryHandlerRequest(http.MethodPost, "/api/v1/directory/users/import", csvBody, nil)
	request.Header.Set("Idempotency-Key", "import-request-001")
	request.Header.Set("X-Change-Reason", "Quarterly identity synchronization")
	handler.ApplyImport(response, request)
	if response.Code != http.StatusCreated || stub.applyKey != "import-request-001" || stub.applyReason != "Quarterly identity synchronization" {
		t.Fatalf("apply status=%d key=%q reason=%q body=%s", response.Code, stub.applyKey, stub.applyReason, response.Body.String())
	}
}
