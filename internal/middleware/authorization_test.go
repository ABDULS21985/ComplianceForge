package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/models"
)

type fakeAuthorizer struct {
	decision authz.Decision
	err      error
	request  authz.Request
}

func (a *fakeAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	a.request = request
	return a.decision, a.err
}

func TestRequireAuthorizationAllowsGrantedRequest(t *testing.T) {
	authorizer := &fakeAuthorizer{decision: authz.Decision{Allowed: true, PolicyID: "policy-1"}}
	called := false
	handler := RequireAuthorization(authorizer, "risks", "update", func(*http.Request) string {
		return "risk-1"
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	request := requestWithAuthorizationIdentity(httptest.NewRequest(http.MethodPatch, "/risks/risk-1", nil))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("status = %d, called = %v", response.Code, called)
	}
	if authorizer.request.SubjectID != "user-1" || authorizer.request.OrganizationID != "org-1" ||
		authorizer.request.Role != "risk_manager" || authorizer.request.ResourceID != "risk-1" {
		t.Fatalf("authorization request = %#v", authorizer.request)
	}
}

func TestRequireAuthorizationDeniesUnmatchedPolicy(t *testing.T) {
	authorizer := &fakeAuthorizer{decision: authz.Decision{Allowed: false, Reason: "default deny"}}
	handler := RequireAuthorization(authorizer, "risks", "delete", nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached downstream handler")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithAuthorizationIdentity(httptest.NewRequest(http.MethodDelete, "/risks/risk-1", nil)))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	var payload models.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != http.StatusForbidden || payload.ErrorCode != "access_denied" || payload.RequestID == "" {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestRequireAuthorizationFailsClosedOnDecisionError(t *testing.T) {
	authorizer := &fakeAuthorizer{err: errors.New("policy store unavailable")}
	handler := RequireAuthorization(authorizer, "controls", "approve", nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("failed decision reached downstream handler")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithAuthorizationIdentity(httptest.NewRequest(http.MethodPost, "/controls/one/approve", nil)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestRequireAuthorizationRejectsMissingPolicyDecisionPoint(t *testing.T) {
	handler := RequireAuthorization(nil, "reports", "export", nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request reached downstream handler")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithAuthorizationIdentity(httptest.NewRequest(http.MethodPost, "/reports/export", nil)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestRequireAuthorizationRejectsMissingIdentity(t *testing.T) {
	authorizer := &fakeAuthorizer{decision: authz.Decision{Allowed: true}}
	handler := RequireAuthorization(authorizer, "reports", "read", nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("anonymous request reached downstream handler")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/reports", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func requestWithAuthorizationIdentity(request *http.Request) *http.Request {
	ctx := context.WithValue(request.Context(), ContextKeyUserID, "user-1")
	ctx = context.WithValue(ctx, ContextKeyOrgID, "org-1")
	ctx = context.WithValue(ctx, ContextKeyRole, "risk_manager")
	return request.WithContext(ctx)
}
