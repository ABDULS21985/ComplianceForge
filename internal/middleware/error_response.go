package middleware

import (
	"net/http"

	"github.com/complianceforge/platform/internal/apiresponse"
)

func writeMiddlewareError(w http.ResponseWriter, r *http.Request, status int, code, message, details string) {
	requestID := ""
	if r != nil {
		requestID = GetRequestIDFromContext(r.Context())
	}
	apiresponse.WriteError(w, status, code, message, details, requestID)
}
