package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/models"
)

func TestMiddlewareErrorWritersUseCanonicalEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.Handler
		request    *http.Request
		wantStatus int
		wantCode   string
	}{
		{
			name: "tenant context",
			handler: TenantMiddleware(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("invalid tenant reached downstream handler")
			})),
			request:    httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "tenant_context_invalid",
		},
		{
			name: "plan tenant context",
			handler: PlanLimits(nil, "risks")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("missing tenant reached downstream handler")
			})),
			request:    httptest.NewRequest(http.MethodPost, "/api/v1/risks", nil),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "tenant_context_required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestID := "request-middleware-contract"
			ctx := context.WithValue(test.request.Context(), ContextKeyRequestID, requestID)
			response := httptest.NewRecorder()
			test.handler.ServeHTTP(response, test.request.WithContext(ctx))
			if response.Code != test.wantStatus || response.Header().Get("Content-Type") != "application/json" ||
				response.Header().Get("X-Request-ID") != requestID {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
			var payload models.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code != test.wantStatus || payload.ErrorCode != test.wantCode || payload.Message == "" || payload.RequestID != requestID {
				t.Fatalf("payload=%#v", payload)
			}
			if strings.Contains(response.Body.String(), `"error":`) || strings.Contains(response.Body.String(), `"type":`) {
				t.Fatalf("legacy error envelope=%s", response.Body.String())
			}
		})
	}
}
