package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/middleware"
)

type permissionServiceStub struct {
	permissions map[string][]string
	err         error
	orgID       string
	userID      string
}

func (s *permissionServiceStub) GetUserPermissions(_ context.Context, organizationID, userID string) (map[string][]string, error) {
	s.orgID, s.userID = organizationID, userID
	return s.permissions, s.err
}

func permissionRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/access/my-permissions", nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, "20000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "10000000-0000-0000-0000-000000000001")
	return request.WithContext(ctx)
}

func TestPermissionHandlerReturnsEffectivePermissions(t *testing.T) {
	service := &permissionServiceStub{permissions: map[string][]string{"audits": {"read", "update"}}}
	handler := NewPermissionHandler(service)
	response := httptest.NewRecorder()

	handler.GetMyPermissions(response, permissionRequest())

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"audits":["read","update"]`) {
		t.Fatalf("body = %s", response.Body.String())
	}
	if service.orgID == "" || service.userID == "" {
		t.Fatal("handler did not pass authenticated tenant and user")
	}
}

func TestPermissionHandlerRequiresAuthenticationContext(t *testing.T) {
	response := httptest.NewRecorder()
	NewPermissionHandler(&permissionServiceStub{}).GetMyPermissions(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/access/my-permissions", nil),
	)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestPermissionHandlerDoesNotExposeStoreErrors(t *testing.T) {
	response := httptest.NewRecorder()
	NewPermissionHandler(&permissionServiceStub{err: errors.New("secret database detail")}).GetMyPermissions(response, permissionRequest())
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "secret database detail") {
		t.Fatalf("response leaked internal error: %s", response.Body.String())
	}
}
