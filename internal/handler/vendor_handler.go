package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// VendorService defines the methods required by VendorHandler.
type VendorService interface {
	Create(ctx context.Context, vendor *models.Vendor) error
	GetByID(ctx context.Context, id string) (*models.Vendor, error)
	Update(ctx context.Context, vendor *models.Vendor) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Vendor, int, error)
	GetDueForAssessment(ctx context.Context) ([]models.Vendor, error)
	Assess(ctx context.Context, id string, assessment *VendorAssessmentRequest) error
}

// VendorAssessmentRequest is the payload for POST /vendors/{id}/assess.
type VendorAssessmentRequest struct {
	RiskLevel models.RiskLevel `json:"risk_level" validate:"required"`
	Notes     string           `json:"notes"`
}

// VendorHandler handles vendor management endpoints.
type VendorHandler struct {
	service VendorService
}

// NewVendorHandler creates a new VendorHandler with the given service.
func NewVendorHandler(service VendorService) *VendorHandler {
	return &VendorHandler{service: service}
}

// Create handles POST /vendors.
func (h *VendorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var vendor models.Vendor
	if err := json.NewDecoder(r.Body).Decode(&vendor); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &vendor); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create vendor", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, vendor)
}

// GetByID handles GET /vendors/{id}.
func (h *VendorHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing vendor ID", "")
		return
	}

	vendor, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Vendor not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, vendor)
}

// Update handles PUT /vendors/{id}.
func (h *VendorHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing vendor ID", "")
		return
	}

	var vendor models.Vendor
	if err := json.NewDecoder(r.Body).Decode(&vendor); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	vendor.ID = id

	if err := h.service.Update(r.Context(), &vendor); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update vendor", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, vendor)
}

// Delete handles DELETE /vendors/{id}.
func (h *VendorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing vendor ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete vendor", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /vendors.
func (h *VendorHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	vendors, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list vendors", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": vendors,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetDueForAssessment handles GET /vendors/due-for-assessment.
func (h *VendorHandler) GetDueForAssessment(w http.ResponseWriter, r *http.Request) {
	vendors, err := h.service.GetDueForAssessment(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get vendors due for assessment", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": vendors})
}

// Assess handles POST /vendors/{id}/assess.
func (h *VendorHandler) Assess(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing vendor ID", "")
		return
	}

	var req VendorAssessmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Assess(r.Context(), id, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to assess vendor", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Vendor assessment completed"})
}
