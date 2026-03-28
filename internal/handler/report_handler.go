package handler

import (
	"context"
	"net/http"
)

// ReportService defines the methods required by ReportHandler.
type ReportService interface {
	GetComplianceReport(ctx context.Context) (interface{}, error)
	GetRiskReport(ctx context.Context) (interface{}, error)
	GetAuditReport(ctx context.Context) (interface{}, error)
	GetExecutiveSummary(ctx context.Context) (interface{}, error)
}

// ReportHandler handles reporting endpoints.
type ReportHandler struct {
	service ReportService
}

// NewReportHandler creates a new ReportHandler with the given service.
func NewReportHandler(service ReportService) *ReportHandler {
	return &ReportHandler{service: service}
}

// GetComplianceReport handles GET /reports/compliance.
func (h *ReportHandler) GetComplianceReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.GetComplianceReport(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate compliance report", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": report})
}

// GetRiskReport handles GET /reports/risk.
func (h *ReportHandler) GetRiskReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.GetRiskReport(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate risk report", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": report})
}

// GetAuditReport handles GET /reports/audit.
func (h *ReportHandler) GetAuditReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.GetAuditReport(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate audit report", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": report})
}

// GetExecutiveSummary handles GET /reports/executive-summary.
func (h *ReportHandler) GetExecutiveSummary(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.GetExecutiveSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate executive summary", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": report})
}
