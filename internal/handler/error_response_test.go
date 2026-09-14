package handler

import (
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
