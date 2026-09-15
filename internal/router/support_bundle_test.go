package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type routerSupportBundleService struct{ calls int }

type routerSupportBundleAudit struct{}

func (routerSupportBundleAudit) RecordSupportBundleGeneration(context.Context, models.SupportBundleAudit) error {
	return nil
}

func (s *routerSupportBundleService) Generate(ctx context.Context, org, actor, requestID string, request models.SupportBundleRequest) (*models.SupportBundleArtifact, error) {
	s.calls++
	provider, err := service.NewSupportBundleService(routerDiagnosticsService{}, routerSupportBundleAudit{}, testRouterConfig())
	if err != nil {
		return nil, err
	}
	return provider.Generate(ctx, org, actor, requestID, request)
}

func TestSupportBundleRouteRequiresSettingsConfigureAndExplicitConsent(t *testing.T) {
	for _, test := range []struct {
		name   string
		auth   bool
		allow  bool
		body   string
		status int
		calls  int
	}{
		{"unauthenticated", false, true, `{"consent":true,"scope":"health_and_posture"}`, http.StatusUnauthorized, 0},
		{"permission denied", true, false, `{"consent":true,"scope":"health_and_posture"}`, http.StatusForbidden, 0},
		{"no consent", true, true, `{"consent":false,"scope":"health_and_posture"}`, http.StatusBadRequest, 0},
		{"consented", true, true, `{"consent":true,"scope":"health_and_posture"}`, http.StatusOK, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies := testRouterDependencies()
			provider := &routerSupportBundleService{}
			dependencies.Diagnostics = handler.NewDiagnosticsHandler(routerDiagnosticsService{}, handler.WithSupportBundleService(provider))
			dependencies.Authorizer = &routerAuthorizer{allowed: test.allow}
			router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/diagnostics/support-bundle", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.auth {
				request.Header.Set("Authorization", "Bearer test-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status || provider.calls != test.calls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.calls, response.Body.String())
			}
			if test.status != http.StatusOK && (response.Header().Get("Content-Disposition") != "" || response.Header().Get("X-Support-Bundle-SHA256") != "") {
				t.Fatal("failed support route leaked artifact metadata")
			}
		})
	}
}
