package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequiredRoutePermissionMap(t *testing.T) {
	tests := []struct {
		method, path, resource, action string
	}{
		{http.MethodGet, "/api/v1/organizations", "organizations", "read"},
		{http.MethodPost, "/api/v1/organizations", "organizations", "configure"},
		{http.MethodPut, "/api/v1/organizations/id", "organizations", "update"},
		{http.MethodDelete, "/api/v1/organizations/id", "organizations", "configure"},
		{http.MethodGet, "/api/v1/frameworks", "frameworks", "read"},
		{http.MethodPost, "/api/v1/frameworks/id/adopt", "frameworks", "update"},
		{http.MethodGet, "/api/v1/frameworks/id/controls", "controls", "read"},
		{http.MethodGet, "/api/v1/controls/id", "controls", "read"},
		{http.MethodPatch, "/api/v1/controls/id/implementation", "controls", "update"},
		{http.MethodPost, "/api/v1/controls/id/evidence", "controls", "update"},
		{http.MethodGet, "/api/v1/controls/id/evidence", "controls", "read"},
		{http.MethodGet, "/api/v1/risks", "risks", "read"},
		{http.MethodPost, "/api/v1/risks", "risks", "create"},
		{http.MethodPatch, "/api/v1/risks/id", "risks", "update"},
		{http.MethodPut, "/api/v1/risks/id/assign", "risks", "assign"},
		{http.MethodPost, "/api/v1/risks/id/assessments", "risks", "create"},
		{http.MethodPost, "/api/v1/risks/appetite/id/approve", "risks", "approve"},
		{http.MethodGet, "/api/v1/policies/id", "policies", "read"},
		{http.MethodPost, "/api/v1/policies/id/versions", "policies", "create"},
		{http.MethodPost, "/api/v1/policies/id/submit", "policies", "update"},
		{http.MethodPost, "/api/v1/policies/id/approval/decision", "policies", "approve"},
		{http.MethodPut, "/api/v1/policies/id/publish", "policies", "approve"},
		{http.MethodPut, "/api/v1/policies/id/assign", "policies", "assign"},
		{http.MethodPut, "/api/v1/policies/id/acknowledge", "policies", "read"},
		{http.MethodPut, "/api/v1/notifications/id/read", "users", "read"},
		{http.MethodPut, "/api/v1/notifications/id/acknowledge", "users", "read"},
		{http.MethodPut, "/api/v1/notifications/preferences", "users", "read"},
		{http.MethodGet, "/api/v1/access/my-permissions", "users", "read"},
		{http.MethodPost, "/api/v1/policies/id/exceptions/id/decision", "policies", "approve"},
		{http.MethodPost, "/api/v1/incidents", "incidents", "create"},
		{http.MethodGet, "/api/v1/incidents/statistics", "incidents", "read"},
		{http.MethodPost, "/api/v1/incidents/id/transitions", "incidents", "update"},
		{http.MethodPost, "/api/v1/incidents/id/cancel", "incidents", "update"},
		{http.MethodPost, "/api/v1/incidents/id/reopen", "incidents", "update"},
		{http.MethodPost, "/api/v1/incidents/id/close", "incidents", "approve"},
		{http.MethodPost, "/api/v1/incidents/id/breach-assessment", "incidents", "approve"},
		{http.MethodPost, "/api/v1/incidents/id/notify-dpa", "incidents", "approve"},
		{http.MethodPost, "/api/v1/incidents/id/assignments", "incidents", "assign"},
		{http.MethodPost, "/api/v1/incidents/id/assignments/id/unassign", "incidents", "assign"},
		{http.MethodGet, "/api/v1/assets", "assets", "read"},
		{http.MethodPost, "/api/v1/assets", "assets", "create"},
		{http.MethodPatch, "/api/v1/assets/id", "assets", "update"},
		{http.MethodDelete, "/api/v1/assets/id", "assets", "delete"},
	}
	for _, test := range tests {
		permission, ok := permissionForProtectedRequest(test.method, test.path)
		if !ok || permission.Resource != test.resource || permission.Action != test.action {
			t.Errorf("%s %s => %#v, %v; want %s:%s", test.method, test.path, permission, ok, test.resource, test.action)
		}
	}
}

func TestEveryProtectedNamespaceHasReadablePermission(t *testing.T) {
	for namespace, resource := range protectedResourceAliases {
		permission, ok := permissionForProtectedRequest(http.MethodGet, "/api/v1/"+namespace)
		if !ok || permission.Resource != resource || permission.Action != "read" {
			t.Errorf("namespace %q => %#v, %v", namespace, permission, ok)
		}
	}
	if _, ok := permissionForProtectedRequest(http.MethodGet, "/api/v1/new-unreviewed-module"); ok {
		t.Fatal("unreviewed protected namespace did not fail closed")
	}
}

func TestRouterDistinguishesAuthenticationFromAuthorization(t *testing.T) {
	dependencies := testRouterDependencies()
	denier := &routerAuthorizer{allowed: false}
	dependencies.Authorizer = denier
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/frameworks", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d", unauthenticated.Code)
	}
	if len(denier.requests) != 0 {
		t.Fatal("authorizer was called before authentication")
	}

	authenticatedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/frameworks", nil)
	authenticatedRequest.Header.Set("Authorization", "Bearer access-token")
	forbidden := httptest.NewRecorder()
	router.ServeHTTP(forbidden, authenticatedRequest)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	if len(denier.requests) != 1 || denier.requests[0].Resource != "frameworks" || denier.requests[0].Action != "read" {
		t.Fatalf("authorization requests=%#v", denier.requests)
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.Header.Set("Authorization", "Bearer access-token")
	meResponse := httptest.NewRecorder()
	router.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusForbidden {
		t.Fatalf("denied me status=%d", meResponse.Code)
	}
}

func TestRouterPassesResourceIdentityToAuthorizer(t *testing.T) {
	dependencies := testRouterDependencies()
	authorizer := &routerAuthorizer{allowed: true}
	dependencies.Authorizer = authorizer
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/controls/"+testControlID, nil)
	request.Header.Set("Authorization", "Bearer access-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(authorizer.requests) != 1 || authorizer.requests[0].ResourceID != testControlID {
		t.Fatalf("authorization request=%#v", authorizer.requests)
	}
}

func TestIncidentRoutesFailClosedBeforeHandlerExecution(t *testing.T) {
	dependencies := testRouterDependencies()
	denier := &routerAuthorizer{allowed: false}
	dependencies.Authorizer = denier
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+testIncidentID, nil))
	if anonymous.Code != http.StatusUnauthorized || len(denier.requests) != 0 {
		t.Fatalf("anonymous status=%d authorization=%#v", anonymous.Code, denier.requests)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+testIncidentID+"/notify-dpa", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	forbidden := httptest.NewRecorder()
	router.ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("denied incident status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	if len(denier.requests) != 1 || denier.requests[0].Resource != "incidents" || denier.requests[0].Action != "approve" || denier.requests[0].ResourceID != testIncidentID {
		t.Fatalf("incident authorization request=%#v", denier.requests)
	}
}
