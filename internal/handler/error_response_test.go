package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorRedactsInternalDetails(t *testing.T) {
	response := httptest.NewRecorder()
	writeError(response, http.StatusInternalServerError, "Request failed", "postgres://user:secret@example.test")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "postgres") {
		t.Fatalf("internal details leaked: %s", response.Body.String())
	}
}

func TestWriteErrorKeepsActionableClientDetails(t *testing.T) {
	response := httptest.NewRecorder()
	writeError(response, http.StatusBadRequest, "Invalid request", "title is required")

	if !strings.Contains(response.Body.String(), "title is required") {
		t.Fatalf("validation detail missing: %s", response.Body.String())
	}
}

func TestWriteErrorIncludesStableCodeAndCorrelationID(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set("X-Request-ID", "request-123")
	writeError(response, http.StatusConflict, "Version changed", "reload before retrying")

	var payload struct {
		Code      int    `json:"code"`
		ErrorCode string `json:"error_code"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != http.StatusConflict || payload.ErrorCode != "state_conflict" || payload.RequestID != "request-123" {
		t.Fatalf("error payload=%#v", payload)
	}
}

func TestStableHTTPErrorCodesCoverEnterpriseFailureClasses(t *testing.T) {
	tests := map[int]string{
		http.StatusBadRequest:          "invalid_request",
		http.StatusUnauthorized:        "authentication_required",
		http.StatusForbidden:           "permission_denied",
		http.StatusPaymentRequired:     "entitlement_required",
		http.StatusNotFound:            "resource_not_found",
		http.StatusConflict:            "state_conflict",
		http.StatusUnprocessableEntity: "validation_failed",
		http.StatusTooManyRequests:     "rate_limit_exceeded",
		http.StatusInternalServerError: "internal_error",
		http.StatusServiceUnavailable:  "service_unavailable",
	}
	for status, want := range tests {
		if got := stableHTTPErrorCode(status); got != want {
			t.Errorf("status=%d got=%q want=%q", status, got, want)
		}
	}
}
