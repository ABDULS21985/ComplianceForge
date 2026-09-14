package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type FeatureFlagService interface {
	ListEvaluations(context.Context, string) ([]models.FeatureFlagEvaluation, error)
	Evaluate(context.Context, string, string) (*models.FeatureFlagEvaluation, error)
	GetEntitlements(context.Context, string) (*models.EntitlementSnapshot, error)
	CheckLimit(context.Context, string, string, int64) (*models.EntitlementLimitDecision, error)
	UpsertOverride(context.Context, string, string, string, string, models.FeatureFlagOverrideInput) (*models.TenantFeatureFlagOverride, error)
	ResetOverride(context.Context, string, string, string, string, models.FeatureFlagResetInput) error
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.FeatureFlagChangeEvent, int, error)
}

type FeatureFlagHandler struct{ service FeatureFlagService }

func NewFeatureFlagHandler(service FeatureFlagService) *FeatureFlagHandler {
	return &FeatureFlagHandler{service: service}
}

func (h *FeatureFlagHandler) Ready() bool { return h != nil && h.service != nil }

func (h *FeatureFlagHandler) ListCapabilities(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListEvaluations(r.Context(), featureFlagOrgID(r))
	if err != nil {
		writeFeatureFlagError(w, r, err, "Failed to list product capabilities")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *FeatureFlagHandler) Evaluate(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Evaluate(r.Context(), featureFlagOrgID(r), chi.URLParam(r, "key"))
	if err != nil {
		writeFeatureFlagError(w, r, err, "Failed to evaluate feature flag")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *FeatureFlagHandler) GetEntitlements(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.service.GetEntitlements(r.Context(), featureFlagOrgID(r))
	if err != nil {
		writeFeatureFlagError(w, r, err, "Failed to load subscription entitlements")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *FeatureFlagHandler) CheckLimit(w http.ResponseWriter, r *http.Request) {
	requested, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("requested")), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid requested usage", "requested must be a positive integer")
		return
	}
	decision, err := h.service.CheckLimit(r.Context(), featureFlagOrgID(r), chi.URLParam(r, "metric"), requested)
	if err != nil {
		writeFeatureFlagError(w, r, err, "Failed to check subscription limit")
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

func (h *FeatureFlagHandler) UpsertOverride(w http.ResponseWriter, r *http.Request) {
	var input models.FeatureFlagOverrideInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpsertOverride(
		r.Context(), featureFlagOrgID(r), chi.URLParam(r, "key"), featureFlagActorID(r),
		middleware.GetRequestIDFromContext(r.Context()), input,
	)
	if err != nil {
		writeFeatureFlagError(w, r, err, "Failed to save feature flag override")
		return
	}
	status := http.StatusOK
	if input.ExpectedVersion == nil {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
}

func (h *FeatureFlagHandler) ResetOverride(w http.ResponseWriter, r *http.Request) {
	var input models.FeatureFlagResetInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.service.ResetOverride(
		r.Context(), featureFlagOrgID(r), chi.URLParam(r, "key"), featureFlagActorID(r),
		middleware.GetRequestIDFromContext(r.Context()), input,
	); err != nil {
		writeFeatureFlagError(w, r, err, "Failed to reset feature flag override")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FeatureFlagHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)
	items, total, err := h.service.ListEvents(
		r.Context(), featureFlagOrgID(r), chi.URLParam(r, "key"), pagination,
	)
	if err != nil {
		writeFeatureFlagError(w, r, err, "Failed to list feature flag history")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(pagination))
}

func featureFlagOrgID(r *http.Request) string   { return middleware.GetOrgIDFromContext(r.Context()) }
func featureFlagActorID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func writeFeatureFlagError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrFeatureFlagInvalid):
		writeError(w, http.StatusBadRequest, "Invalid feature flag request", err.Error())
	case errors.Is(err, service.ErrFeatureFlagNotFound):
		writeError(w, http.StatusNotFound, "Product capability or override not found", "")
	case errors.Is(err, service.ErrFeatureFlagConflict):
		writeError(w, http.StatusConflict, "Feature flag override changed", "Reload the current override and retry with its version")
	default:
		log.Error().Err(err).
			Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Str("organization_id", featureFlagOrgID(r)).Msg("feature flag request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}
