package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// PolicyService defines the methods required by PolicyHandler.
type PolicyService interface {
	Create(ctx context.Context, policy *models.Policy) error
	GetByID(ctx context.Context, id string) (*models.Policy, error)
	Update(ctx context.Context, policy *models.Policy) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Policy, int, error)
	SubmitForReview(ctx context.Context, id string) error
	Approve(ctx context.Context, id string, approverID string) error
	Publish(ctx context.Context, id string) error
	GetDueForReview(ctx context.Context) ([]models.Policy, error)
}

// PolicyHandler handles policy management endpoints.
type PolicyHandler struct {
	service PolicyService
}

// NewPolicyHandler creates a new PolicyHandler with the given service.
func NewPolicyHandler(service PolicyService) *PolicyHandler {
	return &PolicyHandler{service: service}
}

// Create handles POST /policies.
func (h *PolicyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var policy models.Policy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &policy); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create policy", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, policy)
}

// GetByID handles GET /policies/{id}.
func (h *PolicyHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	policy, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Policy not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, policy)
}

// Update handles PUT /policies/{id}.
func (h *PolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	var policy models.Policy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	policy.ID = id

	if err := h.service.Update(r.Context(), &policy); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update policy", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, policy)
}

// Delete handles DELETE /policies/{id}.
func (h *PolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete policy", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /policies.
func (h *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	policies, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list policies", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": policies,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// SubmitForReview handles PUT /policies/{id}/submit-review.
func (h *PolicyHandler) SubmitForReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	if err := h.service.SubmitForReview(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit policy for review", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Policy submitted for review"})
}

// Approve handles PUT /policies/{id}/approve.
func (h *PolicyHandler) Approve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	approverID := r.Context().Value("user_id")
	approverStr, _ := approverID.(string)

	if err := h.service.Approve(r.Context(), id, approverStr); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to approve policy", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Policy approved"})
}

// Publish handles PUT /policies/{id}/publish.
func (h *PolicyHandler) Publish(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	if err := h.service.Publish(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to publish policy", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Policy published"})
}

// GetDueForReview handles GET /policies/due-for-review.
func (h *PolicyHandler) GetDueForReview(w http.ResponseWriter, r *http.Request) {
	policies, err := h.service.GetDueForReview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get policies due for review", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": policies})
}
