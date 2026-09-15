package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestDeprecatedRouteEmitsStandardsBasedHeaders(t *testing.T) {
	router := chi.NewRouter()
	router.With(DeprecatedRoute("/api/v1/things/{id}/replacement")).Get("/api/v1/things/{id}/old", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/things/item-1/old", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got, want := response.Header().Get("Deprecation"), "@"+strconv.FormatInt(aliasDeprecationDate.Unix(), 10); got != want {
		t.Fatalf("Deprecation=%q want=%q", got, want)
	}
	if got, want := response.Header().Get("Sunset"), aliasSunsetDate.Format(http.TimeFormat); got != want {
		t.Fatalf("Sunset=%q want=%q", got, want)
	}
	if got, want := response.Header().Get("Link"), `</api/v1/things/item-1/replacement>; rel="successor-version"`; got != want {
		t.Fatalf("Link=%q want=%q", got, want)
	}
	if aliasSunsetDate.Sub(aliasDeprecationDate) < 180*24*time.Hour {
		t.Fatalf("deprecation window=%s, want at least 180 days", aliasSunsetDate.Sub(aliasDeprecationDate))
	}
}

func TestDeprecatedRouteOmitsUnsafeOrUnresolvedSuccessor(t *testing.T) {
	for _, successor := range []string{"https://example.test/other", "/api/v1/other\r\nX-Evil: true", "/api/v1/things/{missing}"} {
		t.Run(successor, func(t *testing.T) {
			response := httptest.NewRecorder()
			DeprecatedRoute(successor)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/old", nil))
			if response.Header().Get("Link") != "" {
				t.Fatalf("unsafe successor produced Link=%q", response.Header().Get("Link"))
			}
		})
	}
}
