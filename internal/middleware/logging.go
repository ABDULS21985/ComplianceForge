package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const ContextKeyRequestID contextKey = "request_id"

// LoggingMiddleware returns a Chi-compatible middleware that logs each request
// with method, path, status, duration, request_id, remote_addr, and user_agent.
// It generates a UUID request_id for every request and adds it to both the
// context and the X-Request-ID response header.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID)

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start)

		log.Info().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Dur("duration", duration).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Int("bytes_written", ww.BytesWritten()).
			Msg("request completed")
	})
}

// GetRequestIDFromContext extracts the request_id from the request context.
func GetRequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		return v
	}
	return ""
}
