package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
)

// OrganizationService defines the methods required by OrganizationHandler.
type OrganizationService interface {
	Create(ctx context.Context, org *models.Organization) error
	GetByID(ctx context.Context, id string) (*models.Organization, error)
	Update(ctx context.Context, org *models.Organization) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Organization, int, error)
}

// OrganizationHandler handles organization CRUD endpoints.
type OrganizationHandler struct {
	service OrganizationService
}

// NewOrganizationHandler creates a new OrganizationHandler with the given service.
func NewOrganizationHandler(service OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{service: service}
}

// Create handles POST /organizations.
func (h *OrganizationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var org models.Organization
	if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.service.Create(r.Context(), &org); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create organization", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

// GetByID handles GET /organizations/{id}.
func (h *OrganizationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing organization ID", "")
		return
	}

	org, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Organization not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, org)
}

// Update handles PUT /organizations/{id}.
func (h *OrganizationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing organization ID", "")
		return
	}

	var org models.Organization
	if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	org.ID = id

	if err := h.service.Update(r.Context(), &org); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update organization", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, org)
}

// Delete handles DELETE /organizations/{id}.
func (h *OrganizationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing organization ID", "")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete organization", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /organizations.
func (h *OrganizationHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	orgs, total, err := h.service.List(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list organizations", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": orgs,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// parsePagination extracts pagination parameters from query string with defaults.
func parsePagination(r *http.Request) models.PaginationRequest {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	return models.PaginationRequest{
		Page:     page,
		PageSize: pageSize,
	}
}
