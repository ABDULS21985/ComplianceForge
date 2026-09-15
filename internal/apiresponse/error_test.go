package apiresponse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/models"
)

func TestWriteErrorProducesCanonicalCorrelatedEnvelope(t *testing.T) {
	response := httptest.NewRecorder()
	WriteError(response, http.StatusConflict, "version_conflict", "Version changed", "Reload and retry.", "request-123")

	if response.Code != http.StatusConflict || response.Header().Get("Content-Type") != "application/json" ||
		response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Request-ID") != "request-123" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
	var payload models.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != http.StatusConflict || payload.ErrorCode != "version_conflict" || payload.Message != "Version changed" ||
		payload.Details != "Reload and retry." || payload.RequestID != "request-123" {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestWriteErrorRedactsServerDetailsAndGeneratesRequestID(t *testing.T) {
	response := httptest.NewRecorder()
	WriteError(response, http.StatusServiceUnavailable, "", "Dependency unavailable", "postgres://user:secret@example.test", "invalid request id\n")

	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "postgres") {
		t.Fatalf("server detail leaked: %s", response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" || strings.Contains(response.Header().Get("X-Request-ID"), "\n") {
		t.Fatalf("request ID=%q", response.Header().Get("X-Request-ID"))
	}
	var payload models.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ErrorCode != "service_unavailable" || payload.RequestID != response.Header().Get("X-Request-ID") {
		t.Fatalf("payload=%#v", payload)
	}
}
