package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

const ContextKeyRequestID contextKey = "request_id"

// LoggingMiddleware returns a Chi-compatible middleware that logs each request
// with method, path, status, duration, request_id, remote_addr, and user_agent.
// It generates a UUID request_id for every request and adds it to both the
// context and the X-Request-ID response header.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		requestID := middleware.GetReqID(r.Context())
		if !validRequestID(requestID) {
			requestID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start)

		event := log.Info().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", redactedRequestPath(r.URL.Path)).
			Int("status", ww.Status()).
			Dur("duration", duration).
			Str("client_ip", truncateLogField(GetClientIPFromContext(r.Context()), 64)).
			Str("user_agent", truncateLogField(r.UserAgent(), 256)).
			Int("bytes_written", ww.BytesWritten())
		if spanContext := trace.SpanContextFromContext(r.Context()); spanContext.IsValid() {
			event = event.Str("trace_id", spanContext.TraceID().String()).Str("span_id", spanContext.SpanID().String())
		}
		event.Msg("request completed")
	})
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return true
}

func redactedRequestPath(path string) string {
	for _, prefix := range []string{
		"/api/v1/vendor-portal",
		"/api/v1/board-portal",
		"/api/v1/calendar/ical",
	} {
		if !strings.HasPrefix(path, prefix+"/") {
			continue
		}
		remainder := strings.TrimPrefix(path, prefix+"/")
		_, suffix, found := strings.Cut(remainder, "/")
		if !found {
			return prefix + "/[REDACTED]"
		}
		return prefix + "/[REDACTED]/" + suffix
	}
	return path
}

func truncateLogField(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

// GetRequestIDFromContext extracts the request_id from the request context.
func GetRequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		return v
	}
	return ""
}
