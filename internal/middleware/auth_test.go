package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/models"
)

type authValidatorStub struct {
	claims *authdomain.Claims
	err    error
}

func (stub authValidatorStub) ValidateAccessToken(context.Context, string) (*authdomain.Claims, error) {
	return stub.claims, stub.err
}

func TestAuthMiddlewareUsesStandardCorrelatedErrorEnvelope(t *testing.T) {
	tests := []struct {
		name      string
		validator AccessTokenValidator
		header    string
		detail    string
	}{
		{name: "unavailable", detail: "authentication unavailable"},
		{name: "missing", validator: authValidatorStub{}, detail: "missing authorization header"},
		{name: "malformed", validator: authValidatorStub{}, header: "Token abc", detail: "invalid authorization header format"},
		{name: "invalid", validator: authValidatorStub{err: errors.New("signature mismatch")}, header: "Bearer abc", detail: "invalid or expired token"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := LoggingMiddleware(AuthMiddleware(test.validator)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("protected handler executed")
			})))
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", test.header)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", response.Code)
			}
			var payload models.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload.Code != http.StatusUnauthorized || payload.ErrorCode != "authentication_required" || payload.Message != "Authentication required" || payload.Details != test.detail {
				t.Fatalf("payload = %#v", payload)
			}
			if payload.RequestID == "" || payload.RequestID != response.Header().Get("X-Request-ID") {
				t.Fatalf("request ID body=%q header=%q", payload.RequestID, response.Header().Get("X-Request-ID"))
			}
		})
	}
}

func TestAuthMiddlewarePropagatesValidatedIdentity(t *testing.T) {
	claims := &authdomain.Claims{UserID: "user-1", OrganizationID: "org-1", Role: "viewer", Email: "user@example.com"}
	handler := AuthMiddleware(authValidatorStub{claims: claims})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserIDFromContext(r.Context()) != claims.UserID || GetOrgIDFromContext(r.Context()) != claims.OrganizationID ||
			GetRoleFromContext(r.Context()) != claims.Role || GetEmailFromContext(r.Context()) != claims.Email {
			t.Fatal("validated identity was not propagated")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}
