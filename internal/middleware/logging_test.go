package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestRedactedRequestPath(t *testing.T) {
	tests := map[string]string{
		"/api/v1/vendor-portal/secret-token":                   "/api/v1/vendor-portal/[REDACTED]",
		"/api/v1/vendor-portal/secret-token/responses":         "/api/v1/vendor-portal/[REDACTED]/responses",
		"/api/v1/board-portal/secret-token/meetings/meeting-1": "/api/v1/board-portal/[REDACTED]/meetings/meeting-1",
		"/api/v1/calendar/ical/secret-token":                   "/api/v1/calendar/ical/[REDACTED]",
		"/api/v1/vendor-portal":                                "/api/v1/vendor-portal",
		"/api/v1/risks/risk-1":                                 "/api/v1/risks/risk-1",
	}

	for input, want := range tests {
		if got := redactedRequestPath(input); got != want {
			t.Errorf("redactedRequestPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidRequestID(t *testing.T) {
	for _, valid := range []string{"request-123", "trace.id:span_1", strings.Repeat("a", 64)} {
		if !validRequestID(valid) {
			t.Errorf("validRequestID(%q) = false", valid)
		}
	}
	for _, invalid := range []string{"", "contains spaces", "contains\nnewline", strings.Repeat("a", 65)} {
		if validRequestID(invalid) {
			t.Errorf("validRequestID(%q) = true", invalid)
		}
	}
}

func TestLoggingMiddlewareReplacesInvalidRequestIDAndRedactsToken(t *testing.T) {
	previousLogger := log.Logger
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previousLogger })

	var downstreamRequestID string
	handler := chimiddleware.RequestID(LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstreamRequestID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusNoContent)
	})))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/vendor-portal/sensitive-value/progress", nil)
	request.Header.Set("X-Request-ID", "invalid request id")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	responseRequestID := response.Header().Get("X-Request-ID")
	if !validRequestID(responseRequestID) || responseRequestID == "invalid request id" {
		t.Fatalf("response request ID = %q, want generated valid ID", responseRequestID)
	}
	if downstreamRequestID != responseRequestID {
		t.Fatalf("downstream request ID = %q, response ID = %q", downstreamRequestID, responseRequestID)
	}
	if strings.Contains(output.String(), "sensitive-value") {
		t.Fatalf("request log leaked portal token: %s", output.String())
	}
	if !strings.Contains(output.String(), "/api/v1/vendor-portal/[REDACTED]/progress") {
		t.Fatalf("request log did not contain redacted path: %s", output.String())
	}
}
