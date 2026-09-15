package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/models"
)

type routerAccessGovernanceService struct {
	handler.AccessGovernanceHandlerService
}

func TestAccessGovernanceAuthenticationVsAuthorization(t *testing.T) {
	for _, tt := range []struct {
		name    string
		token   string
		allowed bool
		status  int
	}{{"missing JWT", "", true, 401}, {"API key cannot substitute for JWT", "", true, 401}, {"authenticated denied", "access-token", false, 403}, {"authenticated allowed", "access-token", true, 200}} {
		t.Run(tt.name, func(t *testing.T) {
			dependencies := testRouterDependencies()
			dependencies.Authorizer = &routerAuthorizer{allowed: tt.allowed}
			router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/access/governance/campaigns", nil)
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			request.Header.Set("X-API-Key", "cf_live_test")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status=%d expected=%d body=%s", response.Code, tt.status, response.Body.String())
			}
		})
	}
}

func (routerAccessGovernanceService) ListCampaigns(context.Context, string, models.PaginationRequest) ([]models.AccessReviewCampaign, int, error) {
	return []models.AccessReviewCampaign{}, 0, nil
}
func TestAccessGovernanceRequiredAndPermissionMap(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.AccessGovernance = nil
	if err := dependencies.Validate(); err == nil || !strings.Contains(err.Error(), "access governance handler is required") {
		t.Fatalf("missing governance accepted: %v", err)
	}
	for _, tt := range []struct{ method, path, action string }{
		{"GET", "/api/v1/access/governance/campaigns", "read"},
		{"POST", "/api/v1/access/governance/campaigns", "configure"},
		{"POST", "/api/v1/access/governance/campaigns/" + testRoleID + "/items/" + testGroupID + "/decision", "configure"},
		{"PUT", "/api/v1/access/roles/" + testRoleID + "/assignments/" + testUserID + "/window", "configure"},
		{"POST", "/api/v1/access/governance/sod-exceptions/" + testRoleID + "/approve", "configure"},
	} {
		permission, ok := permissionForProtectedRequest(tt.method, tt.path)
		if !ok || permission.Resource != "settings" || permission.Action != tt.action {
			t.Fatalf("%s %s permission=%+v ok=%v", tt.method, tt.path, permission, ok)
		}
	}
}
