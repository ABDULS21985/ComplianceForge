// Package apiresponse contains transport-level response primitives shared by
// handlers and middleware. Keeping error serialization in one package prevents
// authentication, authorization, and quota failures from drifting into
// incompatible wire formats.
package apiresponse

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
)

// WriteError writes the canonical API error envelope. Details from server-side
// failures are deliberately discarded because callers must never receive
// database, dependency, credential, or stack information.
func WriteError(w http.ResponseWriter, status int, errorCode, message, details, requestID string) {
	if strings.TrimSpace(errorCode) == "" {
		errorCode = StableErrorCode(status)
	}
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(status)
	}
	if status >= http.StatusInternalServerError {
		details = ""
	}
	requestID = responseRequestID(w, requestID)

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(models.ErrorResponse{
		Code:      status,
		ErrorCode: errorCode,
		Message:   message,
		Details:   details,
		RequestID: requestID,
	})
}

// StableErrorCode maps transport statuses to the default stable machine code.
// Domain middleware may supply a more specific code to WriteError.
func StableErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "authentication_required"
	case http.StatusForbidden:
		return "permission_denied"
	case http.StatusPaymentRequired:
		return "entitlement_required"
	case http.StatusNotFound:
		return "resource_not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "state_conflict"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusUnprocessableEntity:
		return "validation_failed"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		if status >= http.StatusInternalServerError {
			return "internal_error"
		}
		return "request_failed"
	}
}

func responseRequestID(w http.ResponseWriter, requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = strings.TrimSpace(w.Header().Get("X-Request-ID"))
	}
	if !validRequestID(requestID) {
		requestID = uuid.NewString()
	}
	return requestID
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
