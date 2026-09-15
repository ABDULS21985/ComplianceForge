package handler

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/apiresponse"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type DataQualityService interface {
	GetSnapshot(context.Context, string, string) (*models.DataQualitySnapshot, error)
}

type DataQualityHandler struct{ service DataQualityService }

func NewDataQualityHandler(provider DataQualityService) *DataQualityHandler {
	return &DataQualityHandler{service: provider}
}

func (h *DataQualityHandler) Ready() bool {
	if h == nil || h.service == nil {
		return false
	}
	value := reflect.ValueOf(h.service)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

// GetSnapshot is a bodyless GET /api/v1/settings/data-quality, protected by
// settings:read. Aggregate data is withheld entirely when field obligations
// hide/mask anything: an unmasked derived status must never reveal a count.
func (h *DataQualityHandler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	organizationID := middleware.GetOrgIDFromContext(r.Context())
	actorID := middleware.GetUserIDFromContext(r.Context())
	if organizationID == "" || actorID == "" {
		writeDataQualityError(w, r, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	if err := preflightDataQualityDecision(r); err != nil {
		writeClassifiedFailure(w, r, "settings", err)
		return
	}
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		writeDataQualityError(w, r, http.StatusBadRequest, "data_quality_invalid_scope", "Data quality accepts only a bodyless request for the fixed tenant ruleset")
		return
	}
	if !h.Ready() {
		writeDataQualityError(w, r, http.StatusServiceUnavailable, "data_quality_unavailable", "Data quality is temporarily unavailable")
		return
	}
	snapshot, err := h.service.GetSnapshot(r.Context(), organizationID, actorID)
	if errors.Is(err, service.ErrDataQualityScope) {
		writeDataQualityError(w, r, http.StatusForbidden, "data_quality_scope_denied", "Data quality requires an active tenant-bound principal")
		return
	}
	if err != nil || !service.ValidateDataQualitySnapshot(snapshot) || r.Context().Err() != nil {
		log.Error().Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Msg("data quality snapshot unavailable")
		writeDataQualityError(w, r, http.StatusServiceUnavailable, "data_quality_unavailable", "Data quality is temporarily unavailable")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": snapshot})
}

func preflightDataQualityDecision(r *http.Request) error {
	decision, exists := middleware.GetAuthorizationDecision(r.Context())
	if !exists || !decision.Allowed {
		return errors.New("data quality has no valid authorization allow decision")
	}
	for _, obligation := range decision.Obligations {
		if obligation.Kind != service.AccessFieldVisibilityObligation {
			return errors.New("data quality cannot enforce the required non-field obligation")
		}
		for key := range obligation.Parameters {
			switch key {
			case "id", "policy_id", "policy_priority", "resource_type", "field_path", "classification", "visibility", "mask_strategy", "mask_pattern":
			default:
				return errors.New("data quality field obligation has unsupported parameters")
			}
		}
	}
	permissions, err := service.AccessFieldsFromObligations(decision.Obligations, "settings")
	if err != nil {
		return errors.New("data quality field obligations are invalid")
	}
	for _, permission := range permissions {
		if permission.Visibility != models.AccessFieldVisible {
			return errors.New("data quality aggregate and derived status cannot be safely partially disclosed")
		}
	}
	return nil
}

func writeDataQualityError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	apiresponse.WriteError(w, status, code, message, "", middleware.GetRequestIDFromContext(r.Context()))
}
