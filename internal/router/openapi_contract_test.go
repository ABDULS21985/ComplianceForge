package router

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	openapirouter "github.com/getkin/kin-openapi/routers"
	openapilegacy "github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	contract "github.com/complianceforge/platform/internal/openapi"
)

var registerSCIMBodyDecoder sync.Once

func TestOpenAPIContractCoversRequiredProductionRouter(t *testing.T) {
	document, err := contract.Load(context.Background(), openAPIContractPath(t))
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}

	mounted := requiredProductionRoutes(t)
	documented := make(map[string]struct{}, len(document.Operations))
	for route := range document.Operations {
		documented[route.String()] = struct{}{}
	}

	missing := difference(mounted, documented)
	stale := difference(documented, mounted)
	if len(missing) != 0 || len(stale) != 0 {
		t.Fatalf("OpenAPI/router drift:\nundocumented required routes:\n  %s\nstale or optional documented routes:\n  %s",
			strings.Join(missing, "\n  "), strings.Join(stale, "\n  "))
	}
}

func TestOpenAPIContractValidatesRepresentativeHandlerResponses(t *testing.T) {
	loaded, err := contract.Load(context.Background(), openAPIContractPath(t))
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	validator, err := openapilegacy.NewRouter(loaded.Document)
	if err != nil {
		t.Fatalf("index OpenAPI routes: %v", err)
	}
	handler, err := NewRouterWithDependencies(testRouterConfig(), testRouterDependencies())
	if err != nil {
		t.Fatalf("build production router: %v", err)
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		authorized bool
		wantStatus int
	}{
		{name: "liveness", method: http.MethodGet, path: "/health/live", wantStatus: http.StatusOK},
		{name: "login", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"email":"user@example.com","password":"correct-password"}`, wantStatus: http.StatusOK},
		{name: "password recovery anti-enumeration response", method: http.MethodPost, path: "/api/v1/auth/password/forgot", body: `{"organization_id":"` + testOrgID + `","email":"user@example.com"}`, wantStatus: http.StatusAccepted},
		{name: "passkey authentication ceremony", method: http.MethodPost, path: "/api/v1/auth/passkeys/authentication/options", body: `{"organization_id":"` + testOrgID + `","email":"user@example.com"}`, wantStatus: http.StatusOK},
		{name: "MFA token exchange", method: http.MethodPost, path: "/api/v1/auth/mfa/verify", body: `{"challenge_token":"opaque-token","method":"totp","code":"123456"}`, wantStatus: http.StatusOK},
		{name: "authentication error envelope", method: http.MethodGet, path: "/api/v1/auth/me", wantStatus: http.StatusUnauthorized},
		{name: "identity policy", method: http.MethodGet, path: "/api/v1/identity/policy", authorized: true, wantStatus: http.StatusOK},
		{name: "identity session inventory", method: http.MethodGet, path: "/api/v1/identity/sessions", authorized: true, wantStatus: http.StatusOK},
		{name: "risk", method: http.MethodGet, path: "/api/v1/risks/80000000-0000-0000-0000-000000000010", authorized: true, wantStatus: http.StatusOK},
		{name: "managed role", method: http.MethodGet, path: "/api/v1/access/roles/" + testRoleID, authorized: true, wantStatus: http.StatusOK},
		{name: "policy access inventory", method: http.MethodGet, path: "/api/v1/access/policies", authorized: true, wantStatus: http.StatusOK},
		{name: "policy access record", method: http.MethodGet, path: "/api/v1/access/policies/" + testPolicyID, authorized: true, wantStatus: http.StatusOK},
		{name: "policy access decision", method: http.MethodPost, path: "/api/v1/access/evaluate", body: `{"resource_type":"risks","resource_id":"80000000-0000-0000-0000-000000000010","action":"read"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "policy decision evidence", method: http.MethodGet, path: "/api/v1/access/decision-evidence", authorized: true, wantStatus: http.StatusOK},
		{name: "directory user", method: http.MethodGet, path: "/api/v1/directory/users/" + testUserID, authorized: true, wantStatus: http.StatusOK},
		{name: "directory invitation", method: http.MethodPost, path: "/api/v1/directory/users/" + testUserID + "/invitation", body: `{"reason":"Approved onboarding invitation"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "directory group", method: http.MethodGet, path: "/api/v1/directory/groups/" + testGroupID, authorized: true, wantStatus: http.StatusOK},
		{name: "feature evaluation", method: http.MethodGet, path: "/api/v1/settings/capabilities/advanced_reporting/evaluation", authorized: true, wantStatus: http.StatusOK},
		{name: "data governance policy", method: http.MethodGet, path: "/api/v1/settings/data-governance/policy", authorized: true, wantStatus: http.StatusOK},
		{name: "administrator diagnostics", method: http.MethodGet, path: "/api/v1/settings/diagnostics", authorized: true, wantStatus: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			if test.authorized {
				request.Header.Set("Authorization", "Bearer access-token")
			}
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
			validateOpenAPIResponse(t, validator, test.method, test.path, response)
		})
	}
}

func TestOpenAPIContractValidatesSCIMMediaAndDiscoveryResponse(t *testing.T) {
	loaded, err := contract.Load(context.Background(), openAPIContractPath(t))
	if err != nil {
		t.Fatal(err)
	}
	validator, err := openapilegacy.NewRouter(loaded.Document)
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewRouterWithDependencies(testRouterConfig(), testRouterDependencies())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/scim/v2/ServiceProviderConfig", "/api/scim/v2/Users", "/api/scim/v2/Users/" + testUserID} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer cfs_contract-token")
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/scim+json" {
			t.Fatalf("GET %s status=%d content-type=%q body=%s", path, response.Code,
				response.Header().Get("Content-Type"), response.Body.String())
		}
		validateOpenAPIResponse(t, validator, http.MethodGet, path, response)
	}
}

func TestDeprecatedAliasesEmitLifecycleHeaders(t *testing.T) {
	handler, err := NewRouterWithDependencies(testRouterConfig(), testRouterDependencies())
	if err != nil {
		t.Fatalf("build production router: %v", err)
	}
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		successor  string
		authorized bool
	}{
		{name: "health", method: http.MethodGet, path: "/health", successor: "/health/live"},
		{name: "incident deadline", method: http.MethodGet, path: "/api/v1/incidents/breach-notifiable", successor: "/api/v1/incidents/breaches/upcoming", authorized: true},
		{name: "vendor statistics", method: http.MethodGet, path: "/api/v1/vendors/stats", successor: "/api/v1/vendors/statistics", authorized: true},
		{name: "vendor assessment", method: http.MethodPost, path: "/api/v1/vendors/" + testVendorID + "/assess", body: `{}`, successor: "/api/v1/vendors/" + testVendorID + "/assessments", authorized: true},
		{name: "policy approval", method: http.MethodPut, path: "/api/v1/policies/" + testPolicyID + "/approve", body: `{}`, successor: "/api/v1/policies/" + testPolicyID + "/approval/decision", authorized: true},
		{name: "policy submission", method: http.MethodPut, path: "/api/v1/policies/" + testPolicyID + "/submit-review", body: `{"approvers":[]}`, successor: "/api/v1/policies/" + testPolicyID + "/submit", authorized: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.authorized {
				request.Header.Set("Authorization", "Bearer access-token")
			}
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			deprecation := response.Header().Get("Deprecation")
			if !strings.HasPrefix(deprecation, "@") || len(deprecation) < 2 {
				t.Fatalf("Deprecation=%q status=%d body=%s", deprecation, response.Code, response.Body.String())
			}
			sunset, parseErr := http.ParseTime(response.Header().Get("Sunset"))
			if parseErr != nil || !sunset.After(time.Now()) {
				t.Fatalf("Sunset=%q parsed=%v error=%v", response.Header().Get("Sunset"), sunset, parseErr)
			}
			if got, want := response.Header().Get("Link"), "<"+test.successor+">; rel=\"successor-version\""; got != want {
				t.Fatalf("Link=%q want=%q status=%d body=%s", got, want, response.Code, response.Body.String())
			}
		})
	}
}

func TestPrintRequiredProductionRoutes(t *testing.T) {
	if os.Getenv("PRINT_REQUIRED_ROUTES") != "1" {
		t.Skip("set PRINT_REQUIRED_ROUTES=1 to print the required route inventory")
	}

	routes := make([]string, 0)
	for route := range requiredProductionRoutes(t) {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		fmt.Println(route)
	}
}

func requiredProductionRoutes(t *testing.T) map[string]struct{} {
	t.Helper()
	handler, err := NewRouterWithDependencies(testRouterConfig(), testRouterDependencies())
	if err != nil {
		t.Fatal(err)
	}
	routes, ok := handler.(chi.Routes)
	if !ok {
		t.Fatalf("router %T does not expose Chi routes", handler)
	}

	mounted := make(map[string]struct{})
	if err := chi.Walk(routes, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if contract.IsHTTPMethod(method) {
			mounted[method+" "+path] = struct{}{}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return mounted
}

func openAPIContractPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve OpenAPI contract test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "api", "openapi", "openapi.json"))
}

func difference(left, right map[string]struct{}) []string {
	items := make([]string, 0)
	for item := range left {
		if _, exists := right[item]; !exists {
			items = append(items, item)
		}
	}
	sort.Strings(items)
	return items
}

func validateOpenAPIResponse(t *testing.T, validator openapirouter.Router, method, path string, response *httptest.ResponseRecorder) {
	t.Helper()
	registerSCIMBodyDecoder.Do(func() {
		openapi3filter.RegisterBodyDecoder("application/scim+json", openapi3filter.JSONBodyDecoder)
	})
	request := httptest.NewRequest(method, "https://api.example.com"+path, nil)
	route, pathParameters, err := validator.FindRoute(request)
	if err != nil {
		t.Fatalf("find OpenAPI route %s %s: %v", method, path, err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: request, PathParams: pathParameters, Route: route,
		},
		Status: response.Code,
		Header: response.Header(),
		Body:   io.NopCloser(bytes.NewReader(response.Body.Bytes())),
	}
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		t.Fatalf("response violates OpenAPI contract: %v\nbody: %s", err, response.Body.String())
	}
}
