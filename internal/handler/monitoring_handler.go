package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interfaces ----------

// EvidenceCollector defines evidence collection methods required by MonitoringHandler.
type EvidenceCollector interface {
	ListConfigs(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]CollectionConfig, int, error)
	CreateConfig(ctx context.Context, orgID, userID string, config *CollectionConfig) error
	UpdateConfig(ctx context.Context, orgID string, config *CollectionConfig) error
	RunNow(ctx context.Context, orgID, userID, configID string) (*CollectionRun, error)
	GetHistory(ctx context.Context, orgID, configID string, pagination models.PaginationRequest) ([]CollectionRun, int, error)
}

// ComplianceMonitor defines compliance monitoring methods required by MonitoringHandler.
type ComplianceMonitor interface {
	ListMonitors(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]Monitor, int, error)
	CreateMonitor(ctx context.Context, orgID, userID string, monitor *Monitor) error
	UpdateMonitor(ctx context.Context, orgID string, monitor *Monitor) error
	GetMonitorResults(ctx context.Context, orgID, monitorID string, pagination models.PaginationRequest) ([]MonitorResult, int, error)

	ListDriftEvents(ctx context.Context, orgID string, pagination models.PaginationRequest, filters DriftFilters) ([]DriftEvent, int, error)
	AcknowledgeDrift(ctx context.Context, orgID, userID, driftID string, note string) error
	ResolveDrift(ctx context.Context, orgID, userID, driftID string, resolution *DriftResolution) error

	GetDashboard(ctx context.Context, orgID string) (*MonitoringDashboard, error)
}

// ---------- request / response types ----------

// CollectionConfig defines an automated evidence collection configuration.
type CollectionConfig struct {
	ID              string                 `json:"id"`
	OrganizationID  string                 `json:"organization_id"`
	Name            string                 `json:"name" validate:"required"`
	SourceType      string                 `json:"source_type" validate:"required"` // aws, azure, gcp, api, script
	ConnectionInfo  map[string]interface{} `json:"connection_info"`
	Schedule        string                 `json:"schedule,omitempty"` // cron expression
	Enabled         bool                   `json:"enabled"`
	ControlIDs      []string               `json:"control_ids,omitempty"`
	LastRunAt       string                 `json:"last_run_at,omitempty"`
	LastRunStatus   string                 `json:"last_run_status,omitempty"`
	CreatedBy       string                 `json:"created_by"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
}

// CollectionRun represents a single evidence collection execution.
type CollectionRun struct {
	ID             string `json:"id"`
	ConfigID       string `json:"config_id"`
	Status         string `json:"status"` // pending, running, completed, failed
	EvidenceCount  int    `json:"evidence_count"`
	ErrorMessage   string `json:"error_message,omitempty"`
	StartedAt      string `json:"started_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
	TriggeredBy    string `json:"triggered_by"` // schedule, manual
}

// Monitor defines a compliance monitoring rule.
type Monitor struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	Name           string                 `json:"name" validate:"required"`
	Description    string                 `json:"description,omitempty"`
	MonitorType    string                 `json:"monitor_type" validate:"required"` // threshold, drift, policy, custom
	ControlID      string                 `json:"control_id,omitempty"`
	Configuration  map[string]interface{} `json:"configuration"`
	Severity       string                 `json:"severity"` // critical, high, medium, low
	Enabled        bool                   `json:"enabled"`
	Schedule       string                 `json:"schedule,omitempty"`
	LastCheckAt    string                 `json:"last_check_at,omitempty"`
	LastStatus     string                 `json:"last_status,omitempty"` // passing, failing, error
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// MonitorResult records the outcome of a single monitor check.
type MonitorResult struct {
	ID         string                 `json:"id"`
	MonitorID  string                 `json:"monitor_id"`
	Status     string                 `json:"status"` // passing, failing, error
	Details    map[string]interface{} `json:"details,omitempty"`
	CheckedAt  string                 `json:"checked_at"`
}

// DriftFilters holds filter parameters for listing drift events.
type DriftFilters struct {
	Status   string `json:"status"`
	Severity string `json:"severity"`
}

// DriftEvent represents a detected configuration or compliance drift.
type DriftEvent struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	MonitorID       string `json:"monitor_id"`
	MonitorName     string `json:"monitor_name,omitempty"`
	ControlID       string `json:"control_id,omitempty"`
	Severity        string `json:"severity"`
	Status          string `json:"status"` // open, acknowledged, resolved
	Description     string `json:"description"`
	ExpectedValue   string `json:"expected_value,omitempty"`
	ActualValue     string `json:"actual_value,omitempty"`
	DetectedAt      string `json:"detected_at"`
	AcknowledgedBy  string `json:"acknowledged_by,omitempty"`
	AcknowledgedAt  string `json:"acknowledged_at,omitempty"`
	AcknowledgeNote string `json:"acknowledge_note,omitempty"`
	ResolvedBy      string `json:"resolved_by,omitempty"`
	ResolvedAt      string `json:"resolved_at,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
}

// DriftResolution is the payload for PUT /monitoring/drift/{id}/resolve.
type DriftResolution struct {
	Resolution string `json:"resolution" validate:"required"`
	Notes      string `json:"notes,omitempty"`
}

// MonitoringDashboard provides aggregate monitoring metrics.
type MonitoringDashboard struct {
	TotalMonitors     int            `json:"total_monitors"`
	ActiveMonitors    int            `json:"active_monitors"`
	PassingMonitors   int            `json:"passing_monitors"`
	FailingMonitors   int            `json:"failing_monitors"`
	ErrorMonitors     int            `json:"error_monitors"`
	OpenDriftEvents   int            `json:"open_drift_events"`
	DriftBySeverity   map[string]int `json:"drift_by_severity"`
	CollectionConfigs int            `json:"collection_configs"`
	RecentCollections []CollectionRun `json:"recent_collections,omitempty"`
	OverallHealth     float64        `json:"overall_health"`
}

// ---------- handler ----------

// MonitoringHandler handles continuous monitoring and evidence collection endpoints.
type MonitoringHandler struct {
	collector EvidenceCollector
	monitor   ComplianceMonitor
}

// NewMonitoringHandler creates a new MonitoringHandler with the given services.
func NewMonitoringHandler(collector EvidenceCollector, monitor ComplianceMonitor) *MonitoringHandler {
	return &MonitoringHandler{collector: collector, monitor: monitor}
}

// ---------- Evidence Collection ----------

// ListCollectionConfigs handles GET /monitoring/configs.
func (h *MonitoringHandler) ListCollectionConfigs(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	configs, total, err := h.collector.ListConfigs(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list collection configs", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": configs,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateCollectionConfig handles POST /monitoring/configs.
func (h *MonitoringHandler) CreateCollectionConfig(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var config CollectionConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if config.Name == "" || config.SourceType == "" {
		writeError(w, http.StatusBadRequest, "name and source_type are required", "")
		return
	}

	if err := h.collector.CreateConfig(r.Context(), orgID, userID, &config); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create collection config", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, config)
}

// UpdateCollectionConfig handles PUT /monitoring/configs/{id}.
func (h *MonitoringHandler) UpdateCollectionConfig(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	configID := chi.URLParam(r, "id")
	if configID == "" {
		writeError(w, http.StatusBadRequest, "Missing config ID", "")
		return
	}

	var config CollectionConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	config.ID = configID
	config.OrganizationID = orgID

	if err := h.collector.UpdateConfig(r.Context(), orgID, &config); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update collection config", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, config)
}

// RunCollectionNow handles POST /monitoring/configs/{id}/run-now.
func (h *MonitoringHandler) RunCollectionNow(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	configID := chi.URLParam(r, "id")
	if configID == "" {
		writeError(w, http.StatusBadRequest, "Missing config ID", "")
		return
	}

	run, err := h.collector.RunNow(r.Context(), orgID, userID, configID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to trigger collection run", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, run)
}

// GetCollectionHistory handles GET /monitoring/configs/{id}/history.
func (h *MonitoringHandler) GetCollectionHistory(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	configID := chi.URLParam(r, "id")
	if configID == "" {
		writeError(w, http.StatusBadRequest, "Missing config ID", "")
		return
	}

	pagination := parsePagination(r)

	runs, total, err := h.collector.GetHistory(r.Context(), orgID, configID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get collection history", err.Error())
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

// ---------- Monitors ----------

// ListMonitors handles GET /monitoring/monitors.
func (h *MonitoringHandler) ListMonitors(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	monitors, total, err := h.monitor.ListMonitors(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list monitors", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": monitors,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateMonitor handles POST /monitoring/monitors.
func (h *MonitoringHandler) CreateMonitor(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var monitor Monitor
	if err := json.NewDecoder(r.Body).Decode(&monitor); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if monitor.Name == "" || monitor.MonitorType == "" {
		writeError(w, http.StatusBadRequest, "name and monitor_type are required", "")
		return
	}

	if err := h.monitor.CreateMonitor(r.Context(), orgID, userID, &monitor); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create monitor", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, monitor)
}

// UpdateMonitor handles PUT /monitoring/monitors/{id}.
func (h *MonitoringHandler) UpdateMonitor(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	monitorID := chi.URLParam(r, "id")
	if monitorID == "" {
		writeError(w, http.StatusBadRequest, "Missing monitor ID", "")
		return
	}

	var monitor Monitor
	if err := json.NewDecoder(r.Body).Decode(&monitor); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	monitor.ID = monitorID
	monitor.OrganizationID = orgID

	if err := h.monitor.UpdateMonitor(r.Context(), orgID, &monitor); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update monitor", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, monitor)
}

// GetMonitorResults handles GET /monitoring/monitors/{id}/results.
func (h *MonitoringHandler) GetMonitorResults(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	monitorID := chi.URLParam(r, "id")
	if monitorID == "" {
		writeError(w, http.StatusBadRequest, "Missing monitor ID", "")
		return
	}

	pagination := parsePagination(r)

	results, total, err := h.monitor.GetMonitorResults(r.Context(), orgID, monitorID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get monitor results", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": results,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// ---------- Drift Events ----------

// ListDriftEvents handles GET /monitoring/drift.
func (h *MonitoringHandler) ListDriftEvents(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := DriftFilters{
		Status:   r.URL.Query().Get("status"),
		Severity: r.URL.Query().Get("severity"),
	}

	events, total, err := h.monitor.ListDriftEvents(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list drift events", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": events,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// AcknowledgeDrift handles PUT /monitoring/drift/{id}/acknowledge.
func (h *MonitoringHandler) AcknowledgeDrift(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	driftID := chi.URLParam(r, "id")
	if driftID == "" {
		writeError(w, http.StatusBadRequest, "Missing drift event ID", "")
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.monitor.AcknowledgeDrift(r.Context(), orgID, userID, driftID, body.Note); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to acknowledge drift event", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Drift event acknowledged"})
}

// ResolveDrift handles PUT /monitoring/drift/{id}/resolve.
func (h *MonitoringHandler) ResolveDrift(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	driftID := chi.URLParam(r, "id")
	if driftID == "" {
		writeError(w, http.StatusBadRequest, "Missing drift event ID", "")
		return
	}

	var resolution DriftResolution
	if err := json.NewDecoder(r.Body).Decode(&resolution); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if resolution.Resolution == "" {
		writeError(w, http.StatusBadRequest, "resolution is required", "")
		return
	}

	if err := h.monitor.ResolveDrift(r.Context(), orgID, userID, driftID, &resolution); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to resolve drift event", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Drift event resolved"})
}

// ---------- Dashboard ----------

// GetDashboard handles GET /monitoring/dashboard.
func (h *MonitoringHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.monitor.GetDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get monitoring dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": dashboard})
}
