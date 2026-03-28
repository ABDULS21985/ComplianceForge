package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// FrameworkService defines the methods required by FrameworkHandler.
type FrameworkService interface {
	Create(ctx context.Context, framework *models.ComplianceFramework) error
	GetByID(ctx context.Context, id string) (*models.ComplianceFramework, error)
	Update(ctx context.Context, framework *models.ComplianceFramework) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.ComplianceFramework, int, error)
	GetControls(ctx context.Context, frameworkID string) ([]models.Control, error)
	Import(ctx context.Context, framework *models.ComplianceFramework) error
}

// FrameworkHandler handles compliance framework endpoints.
type FrameworkHandler struct {
	service FrameworkService
}

// NewFrameworkHandler creates a new FrameworkHandler with the given service.
func NewFrameworkHandler(service FrameworkService) *FrameworkHandler {
	return &FrameworkHandler{service: service}
}

// Create handles POST /frameworks.
func (h *FrameworkHandler) Create(w http.ResponseWriter, r *http.Request) {
	var framework models.ComplianceFramework
	if err := json.NewDecoder(r.Body).Decode(&framework); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &framework); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create framework", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, framework)
}

// GetByID handles GET /frameworks/{id}.
func (h *FrameworkHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing framework ID", "")
		return
	}

	framework, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Framework not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, framework)
}

// Update handles PUT /frameworks/{id}.
func (h *FrameworkHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing framework ID", "")
		return
	}

	var framework models.ComplianceFramework
	if err := json.NewDecoder(r.Body).Decode(&framework); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	framework.ID = id

	if err := h.service.Update(r.Context(), &framework); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update framework", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, framework)
}

// Delete handles DELETE /frameworks/{id}.
func (h *FrameworkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing framework ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete framework", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /frameworks.
func (h *FrameworkHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	frameworks, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list frameworks", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": frameworks,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetControls handles GET /frameworks/{id}/controls.
func (h *FrameworkHandler) GetControls(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing framework ID", "")
		return
	}

	controls, err := h.service.GetControls(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get framework controls", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": controls})
}

// Import handles POST /frameworks/import.
func (h *FrameworkHandler) Import(w http.ResponseWriter, r *http.Request) {
	var framework models.ComplianceFramework
	if err := json.NewDecoder(r.Body).Decode(&framework); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Import(r.Context(), &framework); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to import framework", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, framework)
}
