package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// IncidentService defines the methods required by IncidentHandler.
type IncidentService interface {
	Create(ctx context.Context, incident *models.Incident) error
	GetByID(ctx context.Context, id string) (*models.Incident, error)
	Update(ctx context.Context, incident *models.Incident) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Incident, int, error)
	UpdateStatus(ctx context.Context, id string, status models.IncidentStatus) error
	GetBreachNotifiable(ctx context.Context) ([]models.Incident, error)
	Escalate(ctx context.Context, id string) error
}

// UpdateIncidentStatusRequest is the payload for PUT /incidents/{id}/status.
type UpdateIncidentStatusRequest struct {
	Status models.IncidentStatus `json:"status" validate:"required"`
}

// IncidentHandler handles incident management endpoints.
type IncidentHandler struct {
	service IncidentService
}

// NewIncidentHandler creates a new IncidentHandler with the given service.
func NewIncidentHandler(service IncidentService) *IncidentHandler {
	return &IncidentHandler{service: service}
}

// Create handles POST /incidents.
func (h *IncidentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var incident models.Incident
	if err := json.NewDecoder(r.Body).Decode(&incident); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &incident); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create incident", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, incident)
}

// GetByID handles GET /incidents/{id}.
func (h *IncidentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing incident ID", "")
		return
	}

	incident, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Incident not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, incident)
}

// Update handles PUT /incidents/{id}.
func (h *IncidentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing incident ID", "")
		return
	}

	var incident models.Incident
	if err := json.NewDecoder(r.Body).Decode(&incident); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	incident.ID = id

	if err := h.service.Update(r.Context(), &incident); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update incident", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, incident)
}

// Delete handles DELETE /incidents/{id}.
func (h *IncidentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing incident ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete incident", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /incidents.
func (h *IncidentHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	incidents, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list incidents", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": incidents,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// UpdateStatus handles PUT /incidents/{id}/status.
func (h *IncidentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing incident ID", "")
		return
	}

	var req UpdateIncidentStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.UpdateStatus(r.Context(), id, req.Status); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update incident status", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Incident status updated"})
}

// GetBreachNotifiable handles GET /incidents/breach-notifiable.
func (h *IncidentHandler) GetBreachNotifiable(w http.ResponseWriter, r *http.Request) {
	incidents, err := h.service.GetBreachNotifiable(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get breach-notifiable incidents", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": incidents})
}

// Escalate handles PUT /incidents/{id}/escalate.
func (h *IncidentHandler) Escalate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing incident ID", "")
		return
	}

	if err := h.service.Escalate(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to escalate incident", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Incident escalated"})
}
