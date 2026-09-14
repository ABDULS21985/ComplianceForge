package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

type contextKeyClientIP string

const ContextKeyClientIP contextKeyClientIP = "client_ip"

// TrustedProxyHeaders resolves the client address from X-Real-IP only when the
// deployment explicitly declares that the API is reachable exclusively via a
// trusted reverse proxy. X-Forwarded-For is intentionally ignored because an
// edge proxy commonly appends to attacker-controlled input.
func TrustedProxyHeaders(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := remoteIP(r.RemoteAddr)
			if enabled {
				realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
				if parsed := net.ParseIP(realIP); parsed != nil && !strings.Contains(realIP, ",") {
					clientIP = parsed.String()
				} else if realIP != "" {
					log.Warn().Str("request_id", GetRequestIDFromContext(r.Context())).Msg("ignored invalid trusted-proxy client address")
				}
			}
			ctx := context.WithValue(r.Context(), ContextKeyClientIP, clientIP)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClientIPFromContext returns the validated client address established by
// TrustedProxyHeaders.
func GetClientIPFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(ContextKeyClientIP).(string); ok {
		return value
	}
	return ""
}

func remoteIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if parsed := net.ParseIP(strings.TrimSpace(host)); parsed != nil {
		return parsed.String()
	}
	return "unknown"
}
