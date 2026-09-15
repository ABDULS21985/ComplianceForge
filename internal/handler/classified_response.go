package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/apiresponse"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

var errClassifiedResponseDenied = errors.New("classified response has no valid allow decision")
var errClassifiedAttachmentMaskingUnsupported = errors.New("classified attachment requires field masking")

// writeClassifiedJSON is the final serialization boundary for resource models
// governed by ABAC field permissions. A missing/denied decision, malformed
// obligation, or cloning failure stops serialization before any domain value is
// written. Production protected routes provide the decision through
// RequireAuthorization.
func writeClassifiedJSON(w http.ResponseWriter, r *http.Request, status int, resourceType string, value any) {
	writeClassifiedJSONWithHeaders(w, r, status, resourceType, value, nil)
}

// writeClassifiedJSONWithHeaders delays data-derived response headers until
// after masking succeeds. Callers must pass only headers whose values were
// derived from the same authorized response; failure paths emit neither those
// values nor the domain payload.
func writeClassifiedJSONWithHeaders(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	resourceType string,
	value any,
	headers map[string]string,
) {
	masked, err := classifiedResponseValue(r, resourceType, value)
	if err != nil {
		writeClassifiedFailure(w, r, resourceType, err)
		return
	}
	for name, value := range headers {
		w.Header().Set(name, value)
	}
	writeJSON(w, status, masked)
}

// writeClassifiedAttachment permits a binary response only when the trusted
// authorization decision contains no hidden/masked field obligation. Arbitrary
// report formats cannot be safely rewritten by the JSON masker, so the
// restrictive case fails closed before any attachment bytes or headers are
// emitted.
func writeClassifiedAttachment(
	w http.ResponseWriter,
	r *http.Request,
	resourceType, fileName, contentType string,
	data []byte,
) {
	permissions, err := classifiedResponsePermissions(r, resourceType)
	if err == nil {
		for _, permission := range permissions {
			if permission.Visibility != models.AccessFieldVisible {
				err = errClassifiedAttachmentMaskingUnsupported
				break
			}
		}
	}
	if err != nil {
		writeClassifiedFailure(w, r, resourceType, err)
		return
	}
	writeAttachment(w, fileName, contentType, data)
}

func writeClassifiedPaginated(
	w http.ResponseWriter,
	r *http.Request,
	resourceType string,
	data any,
	total int,
	pagination models.PaginationRequest,
) {
	writeClassifiedPaginatedWithHeaders(w, r, resourceType, data, total, pagination, nil)
}

func writeClassifiedPaginatedWithHeaders(
	w http.ResponseWriter,
	r *http.Request,
	resourceType string,
	data any,
	total int,
	pagination models.PaginationRequest,
	headers map[string]string,
) {
	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}
	writeClassifiedJSONWithHeaders(w, r, http.StatusOK, resourceType, map[string]any{
		"data": data,
		"pagination": models.PaginationResponse{
			Page: pagination.Page, PageSize: pagination.PageSize,
			TotalItems: total, TotalPages: totalPages,
		},
	}, headers)
}

func classifiedResponseValue(r *http.Request, resourceType string, value any) (any, error) {
	permissions, err := classifiedResponsePermissions(r, resourceType)
	if err != nil {
		return nil, err
	}
	return service.MaskAccessResponse(value, permissions)
}

func classifiedResponsePermissions(r *http.Request, resourceType string) ([]models.AccessFieldPermission, error) {
	decision, exists := middleware.GetAuthorizationDecision(r.Context())
	if !exists || !decision.Allowed {
		return nil, errClassifiedResponseDenied
	}
	return service.AccessFieldsFromObligations(decision.Obligations, strings.TrimSpace(resourceType))
}

func writeClassifiedFailure(w http.ResponseWriter, r *http.Request, resourceType string, err error) {
	requestID := middleware.GetRequestIDFromContext(r.Context())
	log.Error().Err(err).
		Str("request_id", requestID).
		Str("resource", resourceType).
		Msg("classified response serialization failed closed")
	apiresponse.WriteError(
		w, http.StatusServiceUnavailable, "response_masking_unavailable",
		"The authorized response could not be safely prepared", "", requestID,
	)
}
