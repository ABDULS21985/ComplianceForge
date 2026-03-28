package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// ControlService defines the methods required by ControlHandler.
type ControlService interface {
	Create(ctx context.Context, control *models.Control) error
	GetByID(ctx context.Context, id string) (*models.Control, error)
	Update(ctx context.Context, control *models.Control) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Control, int, error)
	UpdateStatus(ctx context.Context, id string, status models.ComplianceStatus) error
	BulkCreate(ctx context.Context, controls []models.Control) error
}

// UpdateControlStatusRequest is the payload for PUT /controls/{id}/status.
type UpdateControlStatusRequest struct {
	Status models.ComplianceStatus `json:"status" validate:"required"`
}

// ControlHandler handles control endpoints.
type ControlHandler struct {
	service ControlService
}

// NewControlHandler creates a new ControlHandler with the given service.
func NewControlHandler(service ControlService) *ControlHandler {
	return &ControlHandler{service: service}
}

// Create handles POST /controls.
func (h *ControlHandler) Create(w http.ResponseWriter, r *http.Request) {
	var control models.Control
	if err := json.NewDecoder(r.Body).Decode(&control); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &control); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create control", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, control)
}

// GetByID handles GET /controls/{id}.
func (h *ControlHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing control ID", "")
		return
	}

	control, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Control not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, control)
}

// Update handles PUT /controls/{id}.
func (h *ControlHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing control ID", "")
		return
	}

	var control models.Control
	if err := json.NewDecoder(r.Body).Decode(&control); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	control.ID = id

	if err := h.service.Update(r.Context(), &control); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update control", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, control)
}

// Delete handles DELETE /controls/{id}.
func (h *ControlHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing control ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete control", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /controls.
func (h *ControlHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	controls, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list controls", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": controls,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// UpdateStatus handles PUT /controls/{id}/status.
func (h *ControlHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing control ID", "")
		return
	}

	var req UpdateControlStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.UpdateStatus(r.Context(), id, req.Status); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update control status", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Control status updated"})
}

// BulkCreate handles POST /controls/bulk.
func (h *ControlHandler) BulkCreate(w http.ResponseWriter, r *http.Request) {
	var controls []models.Control
	if err := json.NewDecoder(r.Body).Decode(&controls); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if len(controls) == 0 {
		writeError(w, http.StatusBadRequest, "Empty controls list", "At least one control is required")
		return
	}

	if err := h.service.BulkCreate(r.Context(), controls); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to bulk create controls", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Controls created successfully",
		"count":   len(controls),
	})
}
