package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interface ----------

// ReportEngine defines the methods required by ReportHandler.
type ReportEngine interface {
	GenerateReport(ctx context.Context, orgID, userID string, req *GenerateReportRequest) (*ReportRun, error)
	GetRunStatus(ctx context.Context, orgID, runID string) (*ReportRun, error)
	DownloadReport(ctx context.Context, orgID, runID string) (*ReportFile, error)

	ListDefinitions(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]ReportDefinition, int, error)
	CreateDefinition(ctx context.Context, orgID, userID string, def *ReportDefinition) error
	GetDefinition(ctx context.Context, orgID, defID string) (*ReportDefinition, error)
	UpdateDefinition(ctx context.Context, orgID string, def *ReportDefinition) error
	DeleteDefinition(ctx context.Context, orgID, defID string) error
	GenerateFromDefinition(ctx context.Context, orgID, userID, defID string) (*ReportRun, error)

	ListSchedules(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]ReportSchedule, int, error)
	CreateSchedule(ctx context.Context, orgID, userID string, sched *ReportSchedule) error
	UpdateSchedule(ctx context.Context, orgID string, sched *ReportSchedule) error
	DeleteSchedule(ctx context.Context, orgID, schedID string) error

	ListHistory(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]ReportRun, int, error)
}

// ---------- request / response types ----------

// GenerateReportRequest is the payload for POST /reports/generate.
type GenerateReportRequest struct {
	ReportType string                 `json:"report_type" validate:"required"`
	Title      string                 `json:"title"`
	Format     string                 `json:"format"` // pdf, csv, xlsx
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ReportRun represents the status of a report generation run.
type ReportRun struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	DefinitionID   string `json:"definition_id,omitempty"`
	ReportType     string `json:"report_type"`
	Title          string `json:"title"`
	Format         string `json:"format"`
	Status         string `json:"status"` // pending, running, completed, failed
	FileURL        string `json:"file_url,omitempty"`
	Error          string `json:"error,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
}

// ReportFile holds the downloadable report content.
type ReportFile struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"-"`
}

// ReportDefinition is a saved, reusable report template.
type ReportDefinition struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	Name           string                 `json:"name" validate:"required"`
	ReportType     string                 `json:"report_type" validate:"required"`
	Format         string                 `json:"format"`
	Parameters     map[string]interface{} `json:"parameters,omitempty"`
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// ReportSchedule defines a recurring report generation schedule.
type ReportSchedule struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	DefinitionID   string `json:"definition_id" validate:"required"`
	CronExpr       string `json:"cron_expr" validate:"required"`
	Enabled        bool   `json:"enabled"`
	Recipients     []string `json:"recipients,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	NextRunAt      string `json:"next_run_at,omitempty"`
}

// ---------- handler ----------

// ReportHandler handles reporting endpoints.
type ReportHandler struct {
	engine ReportEngine
}

// NewReportHandler creates a new ReportHandler with the given engine.
func NewReportHandler(engine ReportEngine) *ReportHandler {
	return &ReportHandler{engine: engine}
}

// GenerateReport handles POST /reports/generate.
func (h *ReportHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req GenerateReportRequest
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

	var def ReportDefinition
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

	var def ReportDefinition
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

	var sched ReportSchedule
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

	var sched ReportSchedule
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
