package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- handler ----------

// ReportHandler handles reporting endpoints.
type ReportHandler struct {
	engine models.ReportEngine
}

// NewReportHandler creates a new ReportHandler with the given engine.
func NewReportHandler(engine models.ReportEngine) *ReportHandler {
	return &ReportHandler{engine: engine}
}

// GenerateReport handles POST /reports/generate.
func (h *ReportHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req models.GenerateReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.ReportType == "" {
		writeError(w, http.StatusBadRequest, "report_type is required", "")
		return
	}

	run, err := h.engine.GenerateReport(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate report", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, run)
}

// GetRunStatus handles GET /reports/status/{id}.
func (h *ReportHandler) GetRunStatus(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	runID := chi.URLParam(r, "id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "Missing run ID", "")
		return
	}

	run, err := h.engine.GetRunStatus(r.Context(), orgID, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Report run not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, run)
}

// DownloadReport handles GET /reports/download/{id}.
func (h *ReportHandler) DownloadReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	runID := chi.URLParam(r, "id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "Missing run ID", "")
		return
	}

	file, err := h.engine.DownloadReport(r.Context(), orgID, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Report file not found", err.Error())
		return
	}

	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+file.FileName+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
	w.WriteHeader(http.StatusOK)
	w.Write(file.Data)
}

// ListDefinitions handles GET /reports/definitions.
func (h *ReportHandler) ListDefinitions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	defs, total, err := h.engine.ListDefinitions(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list report definitions", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": defs,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateDefinition handles POST /reports/definitions.
func (h *ReportHandler) CreateDefinition(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var def models.ReportDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if def.Name == "" || def.ReportType == "" {
		writeError(w, http.StatusBadRequest, "name and report_type are required", "")
		return
	}

	if err := h.engine.CreateDefinition(r.Context(), orgID, userID, &def); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create report definition", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, def)
}

// UpdateDefinition handles PUT /reports/definitions/{id}.
func (h *ReportHandler) UpdateDefinition(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	defID := chi.URLParam(r, "id")
	if defID == "" {
		writeError(w, http.StatusBadRequest, "Missing definition ID", "")
		return
	}

	var def models.ReportDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	def.ID = defID
	def.OrganizationID = orgID

	if err := h.engine.UpdateDefinition(r.Context(), orgID, &def); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update report definition", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, def)
}

// DeleteDefinition handles DELETE /reports/definitions/{id}.
func (h *ReportHandler) DeleteDefinition(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	defID := chi.URLParam(r, "id")
	if defID == "" {
		writeError(w, http.StatusBadRequest, "Missing definition ID", "")
		return
	}

	if err := h.engine.DeleteDefinition(r.Context(), orgID, defID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete report definition", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GenerateFromDefinition handles POST /reports/definitions/{id}/generate.
func (h *ReportHandler) GenerateFromDefinition(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	defID := chi.URLParam(r, "id")
	if defID == "" {
		writeError(w, http.StatusBadRequest, "Missing definition ID", "")
		return
	}

	run, err := h.engine.GenerateFromDefinition(r.Context(), orgID, userID, defID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate report from definition", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, run)
}

// ListSchedules handles GET /reports/schedules.
func (h *ReportHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	schedules, total, err := h.engine.ListSchedules(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list report schedules", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": schedules,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateSchedule handles POST /reports/schedules.
func (h *ReportHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var sched models.ReportSchedule
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if sched.DefinitionID == "" || sched.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "definition_id and cron_expr are required", "")
		return
	}

	if err := h.engine.CreateSchedule(r.Context(), orgID, userID, &sched); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create report schedule", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sched)
}

// UpdateSchedule handles PUT /reports/schedules/{id}.
func (h *ReportHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	schedID := chi.URLParam(r, "id")
	if schedID == "" {
		writeError(w, http.StatusBadRequest, "Missing schedule ID", "")
		return
	}

	var sched models.ReportSchedule
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	sched.ID = schedID
	sched.OrganizationID = orgID

	if err := h.engine.UpdateSchedule(r.Context(), orgID, &sched); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update report schedule", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sched)
}

// DeleteSchedule handles DELETE /reports/schedules/{id}.
func (h *ReportHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	schedID := chi.URLParam(r, "id")
	if schedID == "" {
		writeError(w, http.StatusBadRequest, "Missing schedule ID", "")
		return
	}

	if err := h.engine.DeleteSchedule(r.Context(), orgID, schedID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete report schedule", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListHistory handles GET /reports/history.
func (h *ReportHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	runs, total, err := h.engine.ListHistory(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list report history", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": runs,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}
