package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type FrameworkService interface {
	ListFrameworks(context.Context, string, models.PaginationRequest) ([]models.ComplianceFramework, int, error)
	GetFramework(context.Context, string, string) (*models.ComplianceFramework, error)
	AdoptFramework(context.Context, string, string, string) (*models.OrganizationFramework, error)
	ListFrameworkControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error)
}

type FrameworkHandler struct{ service FrameworkService }

func NewFrameworkHandler(service FrameworkService) *FrameworkHandler {
	return &FrameworkHandler{service: service}
}
func (h *FrameworkHandler) Ready() bool { return h != nil && h.service != nil }

func (h *FrameworkHandler) List(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListFrameworks(r.Context(), middleware.GetOrgIDFromContext(r.Context()), p)
	if err != nil {
		writeComplianceError(w, err, "Failed to list frameworks")
		return
	}
	writePaginated(w, items, total, p)
}

func (h *FrameworkHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetFramework(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeComplianceError(w, err, "Failed to get framework")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *FrameworkHandler) Adopt(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.AdoptFramework(r.Context(), middleware.GetOrgIDFromContext(r.Context()), middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeComplianceError(w, err, "Failed to adopt framework")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *FrameworkHandler) GetControls(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListFrameworkControls(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"), p)
	if err != nil {
		writeComplianceError(w, err, "Failed to list framework controls")
		return
	}
	writePaginated(w, items, total, p)
}

func writePaginated(w http.ResponseWriter, data any, total int, p models.PaginationRequest) {
	totalPages := 0
	if p.PageSize > 0 {
		totalPages = (total + p.PageSize - 1) / p.PageSize
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "pagination": models.PaginationResponse{Page: p.Page, PageSize: p.PageSize, TotalItems: total, TotalPages: totalPages}})
}

func writeComplianceError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrSubscriptionLimitExceeded):
		writeError(w, http.StatusPaymentRequired, "Subscription framework limit reached", "Upgrade the subscription or remove an existing framework before adopting another")
	case errors.Is(err, service.ErrInvalidComplianceID), errors.Is(err, service.ErrInvalidControlPatch), errors.Is(err, service.ErrInvalidEvidence):
		writeError(w, http.StatusBadRequest, "Invalid compliance request", err.Error())
	case errors.Is(err, service.ErrFrameworkNotFound), errors.Is(err, service.ErrControlNotFound):
		writeError(w, http.StatusNotFound, "Compliance resource not found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback, err.Error())
	}
}
