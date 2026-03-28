package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// AuditService defines the methods required by AuditHandler.
type AuditService interface {
	Create(ctx context.Context, audit *models.Audit) error
	GetByID(ctx context.Context, id string) (*models.Audit, error)
	Update(ctx context.Context, audit *models.Audit) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Audit, int, error)
	CreateFinding(ctx context.Context, finding *models.AuditFinding) error
	GetFindings(ctx context.Context, auditID string) ([]models.AuditFinding, error)
	Start(ctx context.Context, id string) error
	Complete(ctx context.Context, id string) error
}

// AuditHandler handles audit management endpoints.
type AuditHandler struct {
	service AuditService
}

// NewAuditHandler creates a new AuditHandler with the given service.
func NewAuditHandler(service AuditService) *AuditHandler {
	return &AuditHandler{service: service}
}

// Create handles POST /audits.
func (h *AuditHandler) Create(w http.ResponseWriter, r *http.Request) {
	var audit models.Audit
	if err := json.NewDecoder(r.Body).Decode(&audit); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &audit); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create audit", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, audit)
}

// GetByID handles GET /audits/{id}.
func (h *AuditHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing audit ID", "")
		return
	}

	audit, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Audit not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, audit)
}

// Update handles PUT /audits/{id}.
func (h *AuditHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing audit ID", "")
		return
	}

	var audit models.Audit
	if err := json.NewDecoder(r.Body).Decode(&audit); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	audit.ID = id

	if err := h.service.Update(r.Context(), &audit); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update audit", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, audit)
}

// Delete handles DELETE /audits/{id}.
func (h *AuditHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing audit ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete audit", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /audits.
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	audits, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list audits", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": audits,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateFinding handles POST /audits/{id}/findings.
func (h *AuditHandler) CreateFinding(w http.ResponseWriter, r *http.Request) {
	auditID := chi.URLParam(r, "id")
	if auditID == "" {
		writeError(w, http.StatusBadRequest, "Missing audit ID", "")
		return
	}

	var finding models.AuditFinding
	if err := json.NewDecoder(r.Body).Decode(&finding); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	finding.AuditID = auditID

	if err := h.service.CreateFinding(r.Context(), &finding); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create audit finding", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, finding)
}

// GetFindings handles GET /audits/{id}/findings.
func (h *AuditHandler) GetFindings(w http.ResponseWriter, r *http.Request) {
	auditID := chi.URLParam(r, "id")
	if auditID == "" {
		writeError(w, http.StatusBadRequest, "Missing audit ID", "")
		return
	}

	findings, err := h.service.GetFindings(r.Context(), auditID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get audit findings", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": findings})
}

// Start handles PUT /audits/{id}/start.
func (h *AuditHandler) Start(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing audit ID", "")
		return
	}

	if err := h.service.Start(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start audit", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Audit started"})
}

// Complete handles PUT /audits/{id}/complete.
func (h *AuditHandler) Complete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing audit ID", "")
		return
	}

	if err := h.service.Complete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to complete audit", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Audit completed"})
}
