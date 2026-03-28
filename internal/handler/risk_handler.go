package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// RiskService defines the methods required by RiskHandler.
type RiskService interface {
	Create(ctx context.Context, risk *models.Risk) error
	GetByID(ctx context.Context, id string) (*models.Risk, error)
	Update(ctx context.Context, risk *models.Risk) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Risk, int, error)
	GetRiskMatrix(ctx context.Context) (interface{}, error)
	GetHeatmap(ctx context.Context) (interface{}, error)
}

// RiskHandler handles risk management endpoints.
type RiskHandler struct {
	service RiskService
}

// NewRiskHandler creates a new RiskHandler with the given service.
func NewRiskHandler(service RiskService) *RiskHandler {
	return &RiskHandler{service: service}
}

// Create handles POST /risks.
func (h *RiskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var risk models.Risk
	if err := json.NewDecoder(r.Body).Decode(&risk); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &risk); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create risk", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, risk)
}

// GetByID handles GET /risks/{id}.
func (h *RiskHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing risk ID", "")
		return
	}

	risk, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Risk not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, risk)
}

// Update handles PUT /risks/{id}.
func (h *RiskHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing risk ID", "")
		return
	}

	var risk models.Risk
	if err := json.NewDecoder(r.Body).Decode(&risk); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	risk.ID = id

	if err := h.service.Update(r.Context(), &risk); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update risk", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, risk)
}

// Delete handles DELETE /risks/{id}.
func (h *RiskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing risk ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete risk", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /risks.
func (h *RiskHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	risks, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list risks", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": risks,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetMatrix handles GET /risks/matrix.
func (h *RiskHandler) GetMatrix(w http.ResponseWriter, r *http.Request) {
	matrix, err := h.service.GetRiskMatrix(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get risk matrix", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": matrix})
}

// GetHeatmap handles GET /risks/heatmap.
func (h *RiskHandler) GetHeatmap(w http.ResponseWriter, r *http.Request) {
	heatmap, err := h.service.GetHeatmap(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get risk heatmap", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": heatmap})
}
