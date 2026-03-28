package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

// CORSMiddleware returns a Chi-compatible middleware that handles CORS using
// the go-chi/cors package. It allows standard methods and headers, supports
// credentials, and sets a preflight max age of 300 seconds.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
