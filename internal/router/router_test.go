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
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/models"
)

const (
	testUserID = "10000000-0000-0000-0000-000000000010"
	testOrgID  = "20000000-0000-0000-0000-000000000010"
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

func (v routerTokenValidator) ValidateAccessToken(context.Context, string) (*authdomain.Claims, error) {
	return v.claims, nil
}

type routerOrganizationService struct {
	organization *models.Organization
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
		{"organization handler", func(d *RouterDependencies) { d.Organizations = nil }, "organization handler is required"},
		{"token validator", func(d *RouterDependencies) { d.AccessTokenValidator = nil }, "access-token validator is required"},
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

func TestRouterMountsRequiredAuthenticationAndOrganizationRoutes(t *testing.T) {
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
	return RouterDependencies{
		Auth:          handler.NewAuthHandler(&routerAuthService{user: user}),
		Organizations: handler.NewOrganizationHandler(&routerOrganizationService{organization: organization}),
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
