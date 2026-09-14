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

type AssetManagementService interface {
	Create(context.Context, string, string, models.AssetCreateInput) (*models.Asset, error)
	GetByID(context.Context, string, string) (*models.Asset, error)
	Update(context.Context, string, string, string, models.AssetPatch) (*models.Asset, error)
	Delete(context.Context, string, string, string, *int64) error
	List(context.Context, string, models.AssetListFilter) ([]models.Asset, int, error)
	Stats(context.Context, string) (*models.AssetStats, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error)
}

type AssetHandler struct{ service AssetManagementService }

func NewAssetHandler(service AssetManagementService) *AssetHandler {
	return &AssetHandler{service: service}
}

func (h *AssetHandler) Ready() bool { return h != nil && h.service != nil }

func (h *AssetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input models.AssetCreateInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Create(r.Context(), assetOrgID(r), assetUserID(r), input)
	if err != nil {
		writeAssetError(w, r, err, "Failed to create asset")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *AssetHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetByID(r.Context(), assetOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeAssetError(w, r, err, "Failed to get asset")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AssetHandler) Update(w http.ResponseWriter, r *http.Request) {
	var patch models.AssetPatch
	if err := decodeAuditJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Update(r.Context(), assetOrgID(r), chi.URLParam(r, "id"), assetUserID(r), patch)
	if err != nil {
		writeAssetError(w, r, err, "Failed to update asset")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AssetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	expectedVersion, err := optionalPositiveInt64(r.URL.Query().Get("expected_version"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid expected_version", err.Error())
		return
	}
	if err := h.service.Delete(r.Context(), assetOrgID(r), chi.URLParam(r, "id"), assetUserID(r), expectedVersion); err != nil {
		writeAssetError(w, r, err, "Failed to delete asset")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)
	filter := models.AssetListFilter{
		PaginationRequest: pagination,
		AssetType:         r.URL.Query().Get("asset_type"), Criticality: r.URL.Query().Get("criticality"),
		Classification: r.URL.Query().Get("classification"), Status: r.URL.Query().Get("status"),
		OwnerUserID: r.URL.Query().Get("owner_user_id"), Tag: r.URL.Query().Get("tag"),
		Search: r.URL.Query().Get("search"), SortBy: r.URL.Query().Get("sort_by"),
		SortDirection: r.URL.Query().Get("sort_dir"),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("processes_personal_data")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid personal-data filter", "processes_personal_data must be true or false")
			return
		}
		filter.ProcessesPersonalData = &value
	}
	items, total, err := h.service.List(r.Context(), assetOrgID(r), filter)
	if err != nil {
		writeAssetError(w, r, err, "Failed to list assets")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *AssetHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Stats(r.Context(), assetOrgID(r))
	if err != nil {
		writeAssetError(w, r, err, "Failed to calculate asset statistics")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *AssetHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, total, err := h.service.ListEvents(
		r.Context(), assetOrgID(r), chi.URLParam(r, "id"), parsePagination(r),
	)
	if err != nil {
		writeAssetError(w, r, err, "Failed to list asset history")
		return
	}
	writePaginated(w, events, total, normalizedHandlerPagination(parsePagination(r)))
}

func assetOrgID(r *http.Request) string  { return middleware.GetOrgIDFromContext(r.Context()) }
func assetUserID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func optionalPositiveInt64(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return nil, errors.New("expected_version must be a positive integer")
	}
	return &value, nil
}

func writeAssetError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrInvalidAsset):
		writeError(w, http.StatusBadRequest, "Invalid asset request", err.Error())
	case errors.Is(err, service.ErrAssetNotFound):
		writeError(w, http.StatusNotFound, "Asset not found", "")
	case errors.Is(err, service.ErrAssetOwnerNotFound):
		writeError(w, http.StatusUnprocessableEntity, "Asset owner is not available", "")
	case errors.Is(err, service.ErrAssetConflict):
		writeError(w, http.StatusConflict, "Asset request conflicts with current state", err.Error())
	default:
		log.Error().Err(err).
			Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Str("organization_id", assetOrgID(r)).
			Msg("asset request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}
