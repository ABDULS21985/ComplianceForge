package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

type diagnosticsHandlerServiceStub struct {
	result         *models.DiagnosticsSnapshot
	err            error
	organizationID string
}

func (s *diagnosticsHandlerServiceStub) GetSnapshot(_ context.Context, organizationID string) (*models.DiagnosticsSnapshot, error) {
	s.organizationID = organizationID
	return s.result, s.err
}

func diagnosticsHandlerRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/settings/diagnostics", nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, "20000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "10000000-0000-0000-0000-000000000001")
	ctx = handlerAllowedContext(ctx)
	return request.WithContext(ctx)
}

func TestDiagnosticsHandlerReturnsNoStoreSnapshot(t *testing.T) {
	service := &diagnosticsHandlerServiceStub{result: &models.DiagnosticsSnapshot{
		OrganizationID: "20000000-0000-0000-0000-000000000001",
		GeneratedAt:    time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC),
		OverallStatus:  models.DiagnosticStatusHealthy,
	}}
	response := httptest.NewRecorder()

	NewDiagnosticsHandler(service).GetSnapshot(response, diagnosticsHandlerRequest())

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	if !strings.Contains(response.Body.String(), `"overall_status":"healthy"`) {
		t.Fatalf("body = %s", response.Body.String())
	}
	if service.organizationID == "" {
		t.Fatal("handler did not pass authenticated organization")
	}
}

func TestDiagnosticsHandlerRejectsMissingAuthenticationContext(t *testing.T) {
	response := httptest.NewRecorder()
	NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}).GetSnapshot(
		response, httptest.NewRequest(http.MethodGet, "/api/v1/settings/diagnostics", nil),
	)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestDiagnosticsHandlerRedactsServiceErrors(t *testing.T) {
	response := httptest.NewRecorder()
	NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{err: errors.New("postgres://secret@internal")}).GetSnapshot(
		response, diagnosticsHandlerRequest(),
	)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "internal") {
		t.Fatalf("response leaked internal error: %s", response.Body.String())
	}
}

func TestDiagnosticsHandlerReadyRejectsTypedNilService(t *testing.T) {
	var service *diagnosticsHandlerServiceStub
	if NewDiagnosticsHandler(service).Ready() {
		t.Fatal("handler reported ready with typed nil service")
	}
	if !NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}).Ready() {
		t.Fatal("handler did not report ready")
	}
}
