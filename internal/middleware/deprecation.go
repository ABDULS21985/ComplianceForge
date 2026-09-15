package middleware

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	aliasDeprecationDate = time.Date(2026, time.September, 14, 0, 0, 0, 0, time.UTC)
	aliasSunsetDate      = time.Date(2027, time.April, 1, 0, 0, 0, 0, time.UTC)
)

// DeprecatedRoute advertises the lifecycle of a compatibility alias without
// changing its behavior. Deprecation uses the RFC 9745 structured date form;
// Sunset uses the RFC 8594 HTTP-date form. Template path parameters are
// replaced from Chi's vetted route context before producing the successor link.
func DeprecatedRoute(successorTemplate string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Deprecation", "@"+strconv.FormatInt(aliasDeprecationDate.Unix(), 10))
			w.Header().Set("Sunset", aliasSunsetDate.Format(http.TimeFormat))
			if successor := deprecatedSuccessor(successorTemplate, r); successor != "" {
				w.Header().Add("Link", "<"+successor+">; rel=\"successor-version\"")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func deprecatedSuccessor(template string, r *http.Request) string {
	template = strings.TrimSpace(template)
	if !strings.HasPrefix(template, "/") || strings.ContainsAny(template, "\r\n<>\"") {
		return ""
	}
	if r != nil {
		routeContext := chi.RouteContext(r.Context())
		if routeContext != nil {
			for index, key := range routeContext.URLParams.Keys {
				if index < len(routeContext.URLParams.Values) {
					template = strings.ReplaceAll(template, "{"+key+"}", url.PathEscape(routeContext.URLParams.Values[index]))
				}
			}
		}
	}
	if strings.ContainsAny(template, "{}") {
		return ""
	}
	return template
}
