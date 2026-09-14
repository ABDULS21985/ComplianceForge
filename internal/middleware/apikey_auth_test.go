package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authdomain "github.com/complianceforge/platform/internal/auth"
)

type fakeAPIKeyAuthenticator struct {
	principal *authdomain.APIKeyPrincipal
	err       error
	rawKey    string
	clientIP  string
}

func (f *fakeAPIKeyAuthenticator) AuthenticateAPIKey(_ context.Context, rawKey, clientIP string) (*authdomain.APIKeyPrincipal, error) {
	f.rawKey = rawKey
	f.clientIP = clientIP
	return f.principal, f.err
}

type fakeAPIKeyLimiter struct {
	allowed    bool
	retryAfter time.Duration
	err        error
	keyID      string
	limit      int
}

func (f *fakeAPIKeyLimiter) Allow(_ context.Context, keyID string, limit int) (bool, time.Duration, error) {
	f.keyID = keyID
	f.limit = limit
	return f.allowed, f.retryAfter, f.err
}

func TestAPIKeyAuthSuccess(t *testing.T) {
	authenticator := &fakeAPIKeyAuthenticator{principal: &authdomain.APIKeyPrincipal{
		KeyID:              "key-1",
		OrganizationID:     "org-1",
		Permissions:        []string{"read:controls"},
		RateLimitPerMinute: 42,
	}}
	limiter := &fakeAPIKeyLimiter{allowed: true}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := GetOrgIDFromContext(r.Context()); got != "org-1" {
			t.Errorf("organization = %q", got)
		}
		if got := GetAPIKeyIDFromContext(r.Context()); got != "key-1" {
			t.Errorf("key ID = %q", got)
		}
		permissions := GetAPIPermissionsFromContext(r.Context())
		if len(permissions) != 1 || permissions[0] != "read:controls" {
			t.Errorf("permissions = %#v", permissions)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/automation/controls", nil)
	request.RemoteAddr = "192.0.2.25:43120"
	request.Header.Set("X-API-Key", "  cf_live_test-secret  ")
	response := httptest.NewRecorder()
	APIKeyAuth(authenticator, limiter)(next).ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if authenticator.rawKey != "cf_live_test-secret" || authenticator.clientIP != "192.0.2.25" {
		t.Fatalf("authenticator received key %q and IP %q", authenticator.rawKey, authenticator.clientIP)
	}
	if limiter.keyID != "key-1" || limiter.limit != 42 {
		t.Fatalf("limiter received key %q and limit %d", limiter.keyID, limiter.limit)
	}
}

func TestAPIKeyAuthRejectsURLCredential(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/automation/controls?api_key=secret", nil)
	response := httptest.NewRecorder()
	APIKeyAuth(&fakeAPIKeyAuthenticator{}, &fakeAPIKeyLimiter{allowed: true})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler was called")
	})).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestAPIKeyAuthHidesCredentialFailureDetails(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-API-Key", "cf_live_invalid")
	response := httptest.NewRecorder()
	APIKeyAuth(
		&fakeAPIKeyAuthenticator{err: authdomain.ErrInvalidAPIKey},
		&fakeAPIKeyLimiter{allowed: true},
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler was called")
	})).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if got := response.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatal("WWW-Authenticate header is missing")
	}
}

func TestAPIKeyAuthFailsClosedOnDependencyOutage(t *testing.T) {
	tests := []struct {
		name          string
		authenticator authdomain.APIKeyAuthenticator
		limiter       APIKeyRateLimiter
	}{
		{name: "missing authenticator", limiter: &fakeAPIKeyLimiter{allowed: true}},
		{name: "missing limiter", authenticator: &fakeAPIKeyAuthenticator{}},
		{name: "authentication store outage", authenticator: &fakeAPIKeyAuthenticator{err: errors.New("database unavailable")}, limiter: &fakeAPIKeyLimiter{allowed: true}},
		{name: "limiter outage", authenticator: &fakeAPIKeyAuthenticator{principal: validAPIKeyPrincipal()}, limiter: &fakeAPIKeyLimiter{err: errors.New("redis unavailable")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-API-Key", "cf_live_test")
			response := httptest.NewRecorder()
			APIKeyAuth(test.authenticator, test.limiter)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("next handler was called")
			})).ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", response.Code)
			}
		})
	}
}

func TestAPIKeyAuthEnforcesRateLimit(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-API-Key", "cf_live_test")
	response := httptest.NewRecorder()
	APIKeyAuth(
		&fakeAPIKeyAuthenticator{principal: validAPIKeyPrincipal()},
		&fakeAPIKeyLimiter{allowed: false, retryAfter: 2500 * time.Millisecond},
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler was called")
	})).ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "3" {
		t.Fatalf("Retry-After = %q, want 3", got)
	}
}

func TestRequireAPIKeyPermission(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name        string
		keyID       string
		permissions []string
		wantStatus  int
	}{
		{name: "exact grant", keyID: "key-1", permissions: []string{"read:controls"}, wantStatus: http.StatusNoContent},
		{name: "wrong action", keyID: "key-1", permissions: []string{"update:controls"}, wantStatus: http.StatusForbidden},
		{name: "wrong resource", keyID: "key-1", permissions: []string{"read:risks"}, wantStatus: http.StatusForbidden},
		{name: "wildcards are not implicit", keyID: "key-1", permissions: []string{"*:*"}, wantStatus: http.StatusForbidden},
		{name: "missing authentication", permissions: []string{"read:controls"}, wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.keyID != "" {
				ctx = context.WithValue(ctx, ContextKeyAPIKeyID, test.keyID)
			}
			ctx = context.WithValue(ctx, ContextKeyAPIPermissions, test.permissions)
			request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
			response := httptest.NewRecorder()
			RequireAPIKeyPermission("read", "controls")(next).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func validAPIKeyPrincipal() *authdomain.APIKeyPrincipal {
	return &authdomain.APIKeyPrincipal{
		KeyID:              "key-1",
		OrganizationID:     "org-1",
		RateLimitPerMinute: 60,
	}
}
