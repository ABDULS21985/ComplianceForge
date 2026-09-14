package observability

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware records RED metrics using Chi route patterns, never raw URLs,
// and creates a server span from an incoming W3C trace context.
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	tracer := otel.Tracer("github.com/complianceforge/platform/http")
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		started := time.Now()
		method := normalizeMethod(request.Method)
		parent := otel.GetTextMapPropagator().Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		ctx, span := tracer.Start(parent, method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		if chi.RouteContext(ctx) == nil {
			ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
		}
		request = request.WithContext(ctx)

		if spanContext := span.SpanContext(); spanContext.IsValid() {
			w.Header().Set("X-Trace-ID", spanContext.TraceID().String())
		}
		m.httpInFlight.Inc()
		defer m.httpInFlight.Dec()
		wrapped := chimiddleware.NewWrapResponseWriter(w, request.ProtoMajor)
		next.ServeHTTP(wrapped, request)

		route := "unmatched"
		if routeContext := chi.RouteContext(request.Context()); routeContext != nil {
			if pattern := routeContext.RoutePattern(); pattern != "" && len(pattern) <= 256 {
				route = pattern
			}
		}
		status := wrapped.Status()
		if status == 0 {
			status = http.StatusOK
		}
		statusClass := normalizeStatusClass(status)
		m.httpRequests.WithLabelValues(method, route, statusClass).Inc()
		m.httpDuration.WithLabelValues(method, route).Observe(time.Since(started).Seconds())
		span.SetName(method + " " + route)
		span.SetAttributes(
			attribute.String("http.request.method", method),
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", status),
		)
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	})
}
