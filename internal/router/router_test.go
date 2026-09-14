package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/models"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/service"
)

const (
	testUserID      = "10000000-0000-0000-0000-000000000010"
	testOrgID       = "20000000-0000-0000-0000-000000000010"
	testFrameworkID = "30000000-0000-0000-0000-000000000010"
	testControlID   = "40000000-0000-0000-0000-000000000010"
	testPolicyID    = "90000000-0000-0000-0000-000000000010"
	testAuditID     = "a0000000-0000-0000-0000-000000000010"
	testIncidentID  = "b0000000-0000-0000-0000-000000000010"
	testAssetID     = "c1000000-0000-0000-0000-000000000010"
)

type routerAuthService struct {
	user *models.User
}

type routerEmailSender struct{}

func (routerEmailSender) Send(context.Context, emailpkg.Message) error { return nil }

type routerAPIKeyLimiter struct{}

func (routerAPIKeyLimiter) Allow(context.Context, string, int) (bool, time.Duration, error) {
	return true, time.Minute, nil
}

type routerAPIKeyAuthenticator struct {
	permissions []string
}

func (a routerAPIKeyAuthenticator) AuthenticateAPIKey(context.Context, string, string) (*authdomain.APIKeyPrincipal, error) {
	return &authdomain.APIKeyPrincipal{
		KeyID: "api-key-1", OrganizationID: testOrgID,
		Permissions: a.permissions, RateLimitPerMinute: 60,
	}, nil
}

func (s *routerAuthService) Login(context.Context, authdomain.LoginRequest) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

func (s *routerAuthService) Register(context.Context, authdomain.RegisterRequest) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

func (s *routerAuthService) RefreshToken(context.Context, string) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

func (s *routerAuthService) CurrentUser(context.Context, string, string) (*models.User, error) {
	return s.user, nil
}

func (s *routerAuthService) Logout(context.Context, string, string, string) error {
	return nil
}

type routerTokenValidator struct {
	claims *authdomain.Claims
}

type routerAuthorizer struct {
	allowed  bool
	err      error
	requests []authz.Request
}

func (a *routerAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	a.requests = append(a.requests, request)
	return authz.Decision{Allowed: a.allowed}, a.err
}

func (v routerTokenValidator) ValidateAccessToken(context.Context, string) (*authdomain.Claims, error) {
	return v.claims, nil
}

type routerOrganizationService struct {
	organization *models.Organization
}

type routerComplianceService struct{}

type routerPermissionService struct{}

func (routerPermissionService) GetUserPermissions(context.Context, string, string) (map[string][]string, error) {
	return map[string][]string{
		"audits":   {"read"},
		"settings": {"read"},
	}, nil
}

type routerRiskService struct{ risk *models.Risk }

type routerPolicyService struct {
	handler.PolicyService
	policy *models.Policy
}

func (s routerPolicyService) Create(context.Context, string, string, models.PolicyCreateInput) (*models.Policy, error) {
	return s.policy, nil
}

func (s routerPolicyService) GetByID(context.Context, string, string) (*models.Policy, error) {
	return s.policy, nil
}

func (s routerPolicyService) List(context.Context, string, models.PolicyListFilter) ([]models.Policy, int, error) {
	return []models.Policy{*s.policy}, 1, nil
}

func (routerPolicyService) ListCategories(context.Context, string) ([]models.PolicyCategory, error) {
	return []models.PolicyCategory{}, nil
}

type routerAuditService struct {
	handler.AuditService
	audit *models.Audit
}

type routerIncidentService struct {
	handler.IncidentService
	incident *models.Incident
}

type routerAssetService struct{ asset *models.Asset }

func (s routerAssetService) Create(_ context.Context, organizationID, actorID string, input models.AssetCreateInput) (*models.Asset, error) {
	item := *s.asset
	item.OrganizationID, item.CreatedBy, item.Name, item.AssetType = organizationID, actorID, input.Name, input.AssetType
	return &item, nil
}

func (s routerAssetService) GetByID(context.Context, string, string) (*models.Asset, error) {
	return s.asset, nil
}

func (s routerAssetService) Update(context.Context, string, string, string, models.AssetPatch) (*models.Asset, error) {
	return s.asset, nil
}

func (routerAssetService) Delete(context.Context, string, string, string, *int64) error { return nil }

func (s routerAssetService) List(context.Context, string, models.AssetListFilter) ([]models.Asset, int, error) {
	return []models.Asset{*s.asset}, 1, nil
}

func (routerAssetService) Stats(context.Context, string) (*models.AssetStats, error) {
	return &models.AssetStats{Total: 1, Active: 1, ByType: map[string]int{"data": 1}}, nil
}

func (routerAssetService) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error) {
	return []models.AssetLifecycleEvent{}, 0, nil
}

func (s routerIncidentService) Create(context.Context, string, string, models.IncidentCreateInput) (*models.Incident, error) {
	return s.incident, nil
}

func (s routerIncidentService) GetByID(context.Context, string, string) (*models.Incident, error) {
	return s.incident, nil
}

func (s routerIncidentService) List(context.Context, string, models.IncidentListFilter) ([]models.Incident, int, error) {
	return []models.Incident{*s.incident}, 1, nil
}

func (routerIncidentService) ListBreachDue(context.Context, string, int, int) ([]models.Incident, error) {
	return []models.Incident{}, nil
}

func (routerIncidentService) Statistics(context.Context, string) (*models.IncidentStatistics, error) {
	return &models.IncidentStatistics{ByStatus: map[models.IncidentStatus]int{}, BySeverity: map[models.IncidentSeverity]int{}}, nil
}

func (routerIncidentService) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.IncidentEvent, int, error) {
	return []models.IncidentEvent{}, 0, nil
}

func (routerIncidentService) ListAssignments(context.Context, string, string, bool) ([]models.IncidentAssignment, error) {
	return []models.IncidentAssignment{}, nil
}

func (s routerAuditService) Create(context.Context, string, string, models.AuditCreateInput) (*models.Audit, error) {
	return s.audit, nil
}

func (s routerAuditService) GetByID(context.Context, string, string) (*models.Audit, error) {
	return s.audit, nil
}

func (s routerAuditService) List(context.Context, string, models.AuditListFilter) ([]models.Audit, int, error) {
	return []models.Audit{*s.audit}, 1, nil
}

func (s routerRiskService) Create(context.Context, string, models.RiskCreateInput) (*models.Risk, error) {
	return s.risk, nil
}
func (s routerRiskService) GetByID(context.Context, string, string) (*models.Risk, error) {
	return s.risk, nil
}
func (s routerRiskService) Update(context.Context, string, string, models.RiskPatch) (*models.Risk, error) {
	return s.risk, nil
}
func (s routerRiskService) Assign(context.Context, string, string, *string, *string) (*models.Risk, error) {
	return s.risk, nil
}
func (routerRiskService) Delete(context.Context, string, string) error { return nil }
func (s routerRiskService) List(context.Context, string, models.RiskListFilter) ([]models.Risk, int, error) {
	return []models.Risk{*s.risk}, 1, nil
}
func (routerRiskService) GetRiskMatrix(context.Context, string, string) (*models.RiskMatrixView, error) {
	return &models.RiskMatrixView{Dimension: "residual", Cells: []models.RiskMatrixCell{}}, nil
}
func (routerRiskService) ListCategories(context.Context, string) ([]models.RiskCategory, error) {
	return []models.RiskCategory{}, nil
}
func (routerRiskService) GetRiskHeatmap(context.Context, string) ([]models.RiskHeatmapEntry, error) {
	return []models.RiskHeatmapEntry{}, nil
}
func (routerRiskService) CreateAssessment(context.Context, string, string, string, models.RiskAssessmentInput) (*models.RiskAssessment, error) {
	return &models.RiskAssessment{}, nil
}
func (routerRiskService) ListAssessments(context.Context, string, string, models.PaginationRequest) ([]models.RiskAssessment, int, error) {
	return []models.RiskAssessment{}, 0, nil
}
func (routerRiskService) CreateTreatment(context.Context, string, string, string, models.RiskTreatmentInput) (*models.RiskTreatment, error) {
	return &models.RiskTreatment{}, nil
}
func (routerRiskService) GetTreatment(context.Context, string, string, string) (*models.RiskTreatment, error) {
	return &models.RiskTreatment{}, nil
}
func (routerRiskService) ListTreatments(context.Context, string, string, models.PaginationRequest) ([]models.RiskTreatment, int, error) {
	return []models.RiskTreatment{}, 0, nil
}
func (routerRiskService) UpdateTreatment(context.Context, string, string, string, models.RiskTreatmentPatch) (*models.RiskTreatment, error) {
	return &models.RiskTreatment{}, nil
}
func (routerRiskService) ListAppetite(context.Context, string) ([]models.RiskAppetiteStatement, error) {
	return []models.RiskAppetiteStatement{}, nil
}
func (routerRiskService) UpsertAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error) {
	return &models.RiskAppetiteStatement{}, nil
}
func (routerRiskService) ApproveAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error) {
	return &models.RiskAppetiteStatement{}, nil
}
func (routerRiskService) CreateIndicator(context.Context, string, string, string, models.RiskIndicatorInput) (*models.RiskIndicator, error) {
	return &models.RiskIndicator{}, nil
}
func (routerRiskService) ListIndicators(context.Context, string, string) ([]models.RiskIndicator, error) {
	return []models.RiskIndicator{}, nil
}
func (routerRiskService) RecordIndicatorValue(context.Context, string, string, string, string, models.RiskIndicatorValueInput) (*models.RiskIndicatorValue, error) {
	return &models.RiskIndicatorValue{}, nil
}
func (routerRiskService) ListIndicatorValues(context.Context, string, string, string, models.PaginationRequest) ([]models.RiskIndicatorValue, int, error) {
	return []models.RiskIndicatorValue{}, 0, nil
}

func (routerComplianceService) ListFrameworks(context.Context, string, models.PaginationRequest) ([]models.ComplianceFramework, int, error) {
	return []models.ComplianceFramework{{BaseModel: models.BaseModel{ID: testFrameworkID}, Code: "TEST", Name: "Test", Version: "1"}}, 1, nil
}
func (routerComplianceService) GetFramework(context.Context, string, string) (*models.ComplianceFramework, error) {
	return &models.ComplianceFramework{BaseModel: models.BaseModel{ID: testFrameworkID}, Code: "TEST", Name: "Test", Version: "1"}, nil
}
func (routerComplianceService) AdoptFramework(context.Context, string, string, string) (*models.OrganizationFramework, error) {
	return &models.OrganizationFramework{BaseModel: models.BaseModel{ID: "50000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID, FrameworkID: testFrameworkID}, nil
}
func (routerComplianceService) ListFrameworkControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error) {
	return []models.Control{{BaseModel: models.BaseModel{ID: testControlID}, FrameworkID: testFrameworkID, Code: "A.1", Title: "Test"}}, 1, nil
}
func (routerComplianceService) ListControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error) {
	return []models.Control{{BaseModel: models.BaseModel{ID: testControlID}, FrameworkID: testFrameworkID, Code: "A.1", Title: "Test"}}, 1, nil
}
func (routerComplianceService) GetControl(context.Context, string, string) (*models.Control, error) {
	return &models.Control{BaseModel: models.BaseModel{ID: testControlID}, FrameworkID: testFrameworkID, Code: "A.1", Title: "Test"}, nil
}
func (routerComplianceService) UpdateControlImplementation(context.Context, string, string, models.ControlImplementationPatch) (*models.ControlImplementation, error) {
	return &models.ControlImplementation{BaseModel: models.BaseModel{ID: "60000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID, FrameworkControlID: testControlID}, nil
}
func (routerComplianceService) AttachControlEvidence(context.Context, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: "70000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID}, nil
}
func (routerComplianceService) ListControlEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error) {
	return []models.ControlEvidence{}, 0, nil
}

func (s *routerOrganizationService) Create(context.Context, *models.Organization) error {
	return nil
}

func (s *routerOrganizationService) GetByID(context.Context, string) (*models.Organization, error) {
	return s.organization, nil
}

func (s *routerOrganizationService) Update(context.Context, *models.Organization) error {
	return nil
}

func (s *routerOrganizationService) Delete(context.Context, string) error {
	return nil
}

func (s *routerOrganizationService) List(context.Context, models.PaginationRequest) ([]models.Organization, int, error) {
	return []models.Organization{*s.organization}, 1, nil
}

func TestNewRouterWithDependenciesFailsFast(t *testing.T) {
	base := testRouterDependencies()
	tests := []struct {
		name   string
		mutate func(*RouterDependencies)
		want   string
	}{
		{"auth handler", func(d *RouterDependencies) { d.Auth = nil }, "auth handler is required"},
		{"unconfigured auth handler", func(d *RouterDependencies) { d.Auth = handler.NewAuthHandler(nil) }, "auth handler is required"},
		{"organization handler", func(d *RouterDependencies) { d.Organizations = nil }, "organization handler is required"},
		{"unconfigured organization handler", func(d *RouterDependencies) { d.Organizations = handler.NewOrganizationHandler(nil) }, "organization handler is required"},
		{"framework handler", func(d *RouterDependencies) { d.Frameworks = nil }, "framework handler is required"},
		{"unconfigured framework handler", func(d *RouterDependencies) { d.Frameworks = handler.NewFrameworkHandler(nil) }, "framework handler is required"},
		{"control handler", func(d *RouterDependencies) { d.Controls = nil }, "control handler is required"},
		{"unconfigured control handler", func(d *RouterDependencies) { d.Controls = handler.NewControlHandler(nil) }, "control handler is required"},
		{"risk handler", func(d *RouterDependencies) { d.Risks = nil }, "risk handler is required"},
		{"unconfigured risk handler", func(d *RouterDependencies) { d.Risks = handler.NewRiskHandler(nil) }, "risk handler is required"},
		{"policy handler", func(d *RouterDependencies) { d.Policies = nil }, "policy handler is required"},
		{"unconfigured policy handler", func(d *RouterDependencies) { d.Policies = handler.NewPolicyHandler(nil) }, "policy handler is required"},
		{"audit handler", func(d *RouterDependencies) { d.Audits = nil }, "audit handler is required"},
		{"unconfigured audit handler", func(d *RouterDependencies) { d.Audits = handler.NewAuditHandler(nil) }, "audit handler is required"},
		{"incident handler", func(d *RouterDependencies) { d.Incidents = nil }, "incident handler is required"},
		{"unconfigured incident handler", func(d *RouterDependencies) { d.Incidents = handler.NewIncidentHandler(nil) }, "incident handler is required"},
		{"asset handler", func(d *RouterDependencies) { d.Assets = nil }, "asset handler is required"},
		{"unconfigured asset handler", func(d *RouterDependencies) { d.Assets = handler.NewAssetHandler(nil) }, "asset handler is required"},
		{"permission handler", func(d *RouterDependencies) { d.Permissions = nil }, "permission handler is required"},
		{"unconfigured permission handler", func(d *RouterDependencies) { d.Permissions = handler.NewPermissionHandler(nil) }, "permission handler is required"},
		{"notification handler", func(d *RouterDependencies) { d.Notifications = nil }, "notification handler is required"},
		{"unconfigured notification handler", func(d *RouterDependencies) { d.Notifications = handler.NewNotificationHandler(nil, nil) }, "notification handler is required"},
		{"integration handler", func(d *RouterDependencies) { d.Integrations = nil }, "integration handler is required"},
		{"unconfigured integration handler", func(d *RouterDependencies) { d.Integrations = handler.NewIntegrationHandler(nil) }, "integration handler is required"},
		{"API-key authenticator", func(d *RouterDependencies) { d.APIKeyAuthenticator = nil }, "API-key authenticator is required"},
		{"API-key rate limiter", func(d *RouterDependencies) { d.APIKeyRateLimiter = nil }, "API-key rate limiter is required"},
		{"request rate limiter", func(d *RouterDependencies) { d.RequestRateLimiter = nil }, "request rate limiter is required"},
		{"token validator", func(d *RouterDependencies) { d.AccessTokenValidator = nil }, "access-token validator is required"},
		{"authorizer", func(d *RouterDependencies) { d.Authorizer = nil }, "authorizer is required"},
		{"typed nil authorizer", func(d *RouterDependencies) { var authorizer *routerAuthorizer; d.Authorizer = authorizer }, "authorizer is required"},
		{"health check", func(d *RouterDependencies) { d.HealthCheck = nil }, "health check is required"},
		{"tenant middleware", func(d *RouterDependencies) { d.TenantMiddleware = nil }, "tenant middleware is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dependencies := base
			tt.mutate(&dependencies)
			_, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewRouterWithDependencies() error = %v, want %q", err, tt.want)
			}
		})
	}

	if _, err := NewRouterWithDependencies(nil, base); err == nil {
		t.Fatal("NewRouterWithDependencies(nil, ...) returned nil error")
	}
}

func TestNewRouterRejectsMissingProductionDependencies(t *testing.T) {
	if _, err := NewRouter(nil, testRouterConfig()); err == nil || !strings.Contains(err.Error(), "database pool is required") {
		t.Fatalf("NewRouter(nil, cfg) error = %v, want database pool error", err)
	}
	if _, err := BuildDependencies(nil, nil); err == nil || !strings.Contains(err.Error(), "database pool is required") {
		t.Fatalf("BuildDependencies(nil, nil) error = %v, want database pool error", err)
	}
}

func TestRouterMountsRequiredCoreRoutes(t *testing.T) {
	router, err := NewRouterWithDependencies(testRouterConfig(), testRouterDependencies())
	if err != nil {
		t.Fatalf("NewRouterWithDependencies() error = %v", err)
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		authorized bool
		wantStatus int
	}{
		{
			name:       "login",
			method:     http.MethodPost,
			path:       "/api/v1/auth/login",
			body:       `{"email":"user@example.com","password":"correct-password"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "register",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       `{"email":"user@example.com","password":"correct-password","first_name":"A","last_name":"User","organization_id":"` + testOrgID + `"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "refresh",
			method:     http.MethodPost,
			path:       "/api/v1/auth/refresh",
			body:       `{"refresh_token":"refresh-token"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "me",
			method:     http.MethodGet,
			path:       "/api/v1/auth/me",
			authorized: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "me requires authentication",
			method:     http.MethodGet,
			path:       "/api/v1/auth/me",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "logout",
			method:     http.MethodPost,
			path:       "/api/v1/auth/logout",
			authorized: true,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "organization",
			method:     http.MethodGet,
			path:       "/api/v1/organizations/" + testOrgID,
			authorized: true,
			wantStatus: http.StatusOK,
		},
		{name: "list frameworks", method: http.MethodGet, path: "/api/v1/frameworks", authorized: true, wantStatus: http.StatusOK},
		{name: "adopt framework", method: http.MethodPost, path: "/api/v1/frameworks/" + testFrameworkID + "/adopt", authorized: true, wantStatus: http.StatusOK},
		{name: "list framework controls", method: http.MethodGet, path: "/api/v1/frameworks/" + testFrameworkID + "/controls", authorized: true, wantStatus: http.StatusOK},
		{name: "update implementation", method: http.MethodPatch, path: "/api/v1/controls/" + testControlID + "/implementation", body: `{"maturity_level":2}`, authorized: true, wantStatus: http.StatusOK},
		{name: "attach evidence", method: http.MethodPost, path: "/api/v1/controls/" + testControlID + "/evidence", body: `{"title":"Policy","evidence_type":"policy"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "list evidence", method: http.MethodGet, path: "/api/v1/controls/" + testControlID + "/evidence", authorized: true, wantStatus: http.StatusOK},
		{name: "list risks", method: http.MethodGet, path: "/api/v1/risks", authorized: true, wantStatus: http.StatusOK},
		{name: "create risk", method: http.MethodPost, path: "/api/v1/risks", body: `{"title":"Availability","inherent_likelihood":4,"inherent_impact":5}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "risk matrix", method: http.MethodGet, path: "/api/v1/risks/matrix", authorized: true, wantStatus: http.StatusOK},
		{name: "risk categories", method: http.MethodGet, path: "/api/v1/risks/categories", authorized: true, wantStatus: http.StatusOK},
		{name: "create assessment", method: http.MethodPost, path: "/api/v1/risks/80000000-0000-0000-0000-000000000010/assessments", body: `{"assessment_type":"periodic","likelihood_after":3,"impact_after":4}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "list policies", method: http.MethodGet, path: "/api/v1/policies", authorized: true, wantStatus: http.StatusOK},
		{name: "create policy", method: http.MethodPost, path: "/api/v1/policies", body: `{"title":"Security policy","initial_version":{"content_text":"Policy content"}}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "policy categories", method: http.MethodGet, path: "/api/v1/policies/categories", authorized: true, wantStatus: http.StatusOK},
		{name: "effective permissions", method: http.MethodGet, path: "/api/v1/access/my-permissions", authorized: true, wantStatus: http.StatusOK},
		{name: "list incidents", method: http.MethodGet, path: "/api/v1/incidents", authorized: true, wantStatus: http.StatusOK},
		{name: "create incident", method: http.MethodPost, path: "/api/v1/incidents", body: `{"title":"Database exposure","description":"A production snapshot was exposed","category":"privacy","severity":"high"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "incident statistics", method: http.MethodGet, path: "/api/v1/incidents/statistics", authorized: true, wantStatus: http.StatusOK},
		{name: "list assets", method: http.MethodGet, path: "/api/v1/assets", authorized: true, wantStatus: http.StatusOK},
		{name: "create asset", method: http.MethodPost, path: "/api/v1/assets", body: `{"name":"Customer database","asset_type":"data"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "asset statistics", method: http.MethodGet, path: "/api/v1/assets/stats", authorized: true, wantStatus: http.StatusOK},
		{name: "asset history", method: http.MethodGet, path: "/api/v1/assets/" + testAssetID + "/events", authorized: true, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.authorized {
				req.Header.Set("Authorization", "Bearer access-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
		})
	}
}

func TestAutomationRoutesEnforceAPIKeyScopes(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.APIKeyAuthenticator = routerAPIKeyAuthenticator{permissions: []string{"read:controls"}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		path       string
		withKey    bool
		wantStatus int
	}{
		{name: "granted resource", path: "/api/v1/automation/controls", withKey: true, wantStatus: http.StatusOK},
		{name: "audit requires its exact scope", path: "/api/v1/automation/audits", withKey: true, wantStatus: http.StatusForbidden},
		{name: "missing resource scope", path: "/api/v1/automation/risks", withKey: true, wantStatus: http.StatusForbidden},
		{name: "missing credential", path: "/api/v1/automation/controls", wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.withKey {
				request.Header.Set("X-API-Key", "cf_live_test")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
		})
	}
}

func TestIncidentAutomationRoutesAreReadOnly(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.APIKeyAuthenticator = routerAPIKeyAuthenticator{permissions: []string{"read:incidents"}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/automation/incidents", "/api/v1/automation/incidents/statistics", "/api/v1/automation/incidents/" + testIncidentID + "/timeline"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-API-Key", "cf_live_test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/automation/incidents", strings.NewReader(`{}`))
	request.Header.Set("X-API-Key", "cf_live_test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("automation incident mutation status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAssetAutomationRoutesAreReadOnly(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.APIKeyAuthenticator = routerAPIKeyAuthenticator{permissions: []string{"read:assets"}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/automation/assets",
		"/api/v1/automation/assets/stats",
		"/api/v1/automation/assets/" + testAssetID,
		"/api/v1/automation/assets/" + testAssetID + "/events",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-API-Key", "cf_live_test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/automation/assets", strings.NewReader(`{}`))
	request.Header.Set("X-API-Key", "cf_live_test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("automation asset mutation status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRouterHealthEndpoints(t *testing.T) {
	dependencies := testRouterDependencies()
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatalf("NewRouterWithDependencies() error = %v", err)
	}

	for _, path := range []string{"/health", "/health/live", "/health/ready"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}

	dependencies.HealthCheck = func(context.Context) error { return errors.New("database unavailable") }
	unreadyRouter, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatalf("NewRouterWithDependencies() error = %v", err)
	}
	response := httptest.NewRecorder()
	unreadyRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func testRouterDependencies() RouterDependencies {
	dummyPool := new(pgxpool.Pool)
	notificationProtector, err := secretbox.NewHex(strings.Repeat("ab", 32))
	if err != nil {
		panic(err)
	}
	notificationEngine := service.NewNotificationEngineWithProtector(
		dummyPool, service.NewEventBus(), routerEmailSender{}, notificationProtector,
	)
	integrationService, err := service.NewIntegrationService(dummyPool, strings.Repeat("cd", 32))
	if err != nil {
		panic(err)
	}
	user := &models.User{
		TenantModel: models.TenantModel{
			BaseModel:      models.BaseModel{ID: testUserID},
			OrganizationID: testOrgID,
		},
		Email:     "user@example.com",
		FirstName: "A",
		LastName:  "User",
		Status:    models.UserStatusActive,
		Role:      models.UserRoleViewer,
		Language:  "en",
	}
	organization := &models.Organization{
		BaseModel: models.BaseModel{ID: testOrgID},
		Name:      "Example",
		Slug:      "example",
		Status:    "active",
		Tier:      "starter",
	}
	risk := &models.Risk{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: "80000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID},
		RiskRef:     "RSK-0001", Title: "Availability", Status: models.RiskStatusIdentified,
	}
	policy := &models.Policy{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testPolicyID}, OrganizationID: testOrgID},
		PolicyRef:   "POL-0001", Title: "Security policy", Status: models.PolicyStateDraft,
	}
	audit := &models.Audit{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testAuditID}, OrganizationID: testOrgID},
		AuditRef:    "AUD-0001", Title: "Annual audit", Type: models.AuditTypeInternal, Status: models.AuditStatusPlanned,
	}
	incident := &models.Incident{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testIncidentID}, OrganizationID: testOrgID},
		IncidentRef: "INC-000001", Title: "Database exposure", Description: "Production snapshot exposed",
		Category: "privacy", Severity: models.IncidentSeverityHigh, Status: models.IncidentStatusReported,
		ReporterID: testUserID, Version: 1,
	}
	asset := &models.Asset{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testAssetID}, OrganizationID: testOrgID},
		AssetRef:    "AST-000001", Name: "Customer database", AssetType: models.AssetTypeData,
		Criticality: models.AssetCriticalityCritical, Classification: models.AssetClassificationRestricted,
		Status: models.AssetStatusActive, Version: 1, CreatedBy: testUserID, Tags: []string{},
	}
	return RouterDependencies{
		Auth:                handler.NewAuthHandler(&routerAuthService{user: user}),
		Organizations:       handler.NewOrganizationHandler(&routerOrganizationService{organization: organization}),
		Frameworks:          handler.NewFrameworkHandler(routerComplianceService{}),
		Controls:            handler.NewControlHandler(routerComplianceService{}),
		Risks:               handler.NewRiskHandler(routerRiskService{risk: risk}),
		Policies:            handler.NewPolicyHandler(routerPolicyService{policy: policy}),
		Audits:              handler.NewAuditHandler(routerAuditService{audit: audit}),
		Incidents:           handler.NewIncidentHandler(routerIncidentService{incident: incident}),
		Assets:              handler.NewAssetHandler(routerAssetService{asset: asset}),
		Permissions:         handler.NewPermissionHandler(routerPermissionService{}),
		Notifications:       handler.NewNotificationHandler(dummyPool, notificationEngine, notificationProtector),
		Integrations:        handler.NewIntegrationHandler(integrationService),
		APIKeyAuthenticator: routerAPIKeyAuthenticator{permissions: []string{"read:controls"}},
		APIKeyRateLimiter:   routerAPIKeyLimiter{},
		RequestRateLimiter:  routerAPIKeyLimiter{},
		AccessTokenValidator: routerTokenValidator{claims: &authdomain.Claims{
			UserID:         testUserID,
			OrganizationID: testOrgID,
			Role:           string(models.UserRoleViewer),
			Email:          user.Email,
			TokenType:      authdomain.TokenTypeAccess,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: testUserID,
			},
		}},
		Authorizer:  &routerAuthorizer{allowed: true},
		HealthCheck: func(context.Context) error { return nil },
		TenantMiddleware: func(next http.Handler) http.Handler {
			return next
		},
	}
}

func testRouterConfig() *config.Config {
	return &config.Config{
		CORS:      config.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}},
		RateLimit: config.RateLimitConfig{RPS: 10_000},
	}
}

func routerTokenPair(user *models.User) *authdomain.TokenPair {
	return &authdomain.TokenPair{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		User:         user,
	}
}
