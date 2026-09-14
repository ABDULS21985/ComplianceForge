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

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/models"
)

const (
	testUserID      = "10000000-0000-0000-0000-000000000010"
	testOrgID       = "20000000-0000-0000-0000-000000000010"
	testFrameworkID = "30000000-0000-0000-0000-000000000010"
	testControlID   = "40000000-0000-0000-0000-000000000010"
)

type routerAuthService struct {
	user *models.User
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

type routerRiskService struct{ risk *models.Risk }

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
	return RouterDependencies{
		Auth:          handler.NewAuthHandler(&routerAuthService{user: user}),
		Organizations: handler.NewOrganizationHandler(&routerOrganizationService{organization: organization}),
		Frameworks:    handler.NewFrameworkHandler(routerComplianceService{}),
		Controls:      handler.NewControlHandler(routerComplianceService{}),
		Risks:         handler.NewRiskHandler(routerRiskService{risk: risk}),
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
