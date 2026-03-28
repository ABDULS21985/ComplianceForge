package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ComplianceEngine defines the methods required by DashboardHandler.
type ComplianceEngine interface {
	GetDashboard(ctx context.Context) (interface{}, error)
	GetComplianceScore(ctx context.Context, frameworkID string) (interface{}, error)
}

// DashboardHandler handles dashboard and compliance score endpoints.
type DashboardHandler struct {
	engine ComplianceEngine
}

// NewDashboardHandler creates a new DashboardHandler with the given compliance engine.
func NewDashboardHandler(engine ComplianceEngine) *DashboardHandler {
	return &DashboardHandler{engine: engine}
}

// GetDashboard handles GET /dashboard.
func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	dashboard, err := h.engine.GetDashboard(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": dashboard})
}

// GetComplianceScore handles GET /dashboard/compliance-score/{frameworkID}.
func (h *DashboardHandler) GetComplianceScore(w http.ResponseWriter, r *http.Request) {
	frameworkID := chi.URLParam(r, "frameworkID")
	if frameworkID == "" {
		writeError(w, http.StatusBadRequest, "Missing framework ID", "")
		return
	}

	score, err := h.engine.GetComplianceScore(r.Context(), frameworkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get compliance score", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": score})
}
