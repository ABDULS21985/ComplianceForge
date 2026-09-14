package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type requestLimiterStub struct {
	allowed bool
	retry   time.Duration
	err     error
	key     string
	limit   int
}

func (s *requestLimiterStub) Allow(_ context.Context, key string, limit int) (bool, time.Duration, error) {
	s.key, s.limit = key, limit
	return s.allowed, s.retry, s.err
}

func TestDistributedRateLimitMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		limiter    *requestLimiterStub
		wantStatus int
	}{
		{name: "allowed", path: "/api/v1/risks", limiter: &requestLimiterStub{allowed: true}, wantStatus: http.StatusNoContent},
		{name: "denied", path: "/api/v1/risks", limiter: &requestLimiterStub{retry: 250 * time.Millisecond}, wantStatus: http.StatusTooManyRequests},
		{name: "store outage fails closed", path: "/api/v1/risks", limiter: &requestLimiterStub{err: errors.New("redis unavailable")}, wantStatus: http.StatusServiceUnavailable},
		{name: "health bypass", path: "/health/live", limiter: &requestLimiterStub{err: errors.New("redis unavailable")}, wantStatus: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.RemoteAddr = "192.0.2.5:1234"
			response := httptest.NewRecorder()
			DistributedRateLimitMiddleware(test.limiter, 10)(next).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s want=%d", response.Code, response.Body.String(), test.wantStatus)
			}
			if test.path != "/health/live" && test.limiter.limit != 10 {
				t.Fatalf("limit=%d want=10", test.limiter.limit)
			}
		})
	}
}
