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

// AnalyticsService defines the methods required by AnalyticsHandler.
type AnalyticsService interface {
	ListSnapshots(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]AnalyticsSnapshot, int, error)
	GetComplianceTrends(ctx context.Context, orgID string, filters TrendFilters) (*TrendData, error)
	GetRiskTrends(ctx context.Context, orgID string, filters TrendFilters) (*TrendData, error)

	GetRiskPrediction(ctx context.Context, orgID, riskID string) (*RiskPrediction, error)
	GetBreachProbability(ctx context.Context, orgID string, params BreachProbabilityParams) (*BreachProbability, error)
	GetBenchmarks(ctx context.Context, orgID string, params BenchmarkParams) (*BenchmarkData, error)

	GetMetricTimeSeries(ctx context.Context, orgID, metric string, params TimeSeriesParams) (*TimeSeriesData, error)
	CompareMetricPeriods(ctx context.Context, orgID, metric string, params PeriodCompareParams) (*PeriodComparison, error)
	GetTopMovers(ctx context.Context, orgID string, params TopMoversParams) ([]TopMover, error)
	GetDistribution(ctx context.Context, orgID, entity string, params DistributionParams) (*DistributionData, error)
	ExportData(ctx context.Context, orgID, userID string, req *ExportRequest) (*ExportResult, error)

	ListDashboards(ctx context.Context, orgID string) ([]CustomDashboard, error)
	CreateDashboard(ctx context.Context, orgID, userID string, dashboard *CustomDashboard) error
	UpdateDashboard(ctx context.Context, orgID string, dashboard *CustomDashboard) error
	DeleteDashboard(ctx context.Context, orgID, dashboardID string) error
	GetWidgetTypes(ctx context.Context) ([]WidgetType, error)
}

// ---------- request / response types ----------

// AnalyticsSnapshot represents a point-in-time snapshot of analytics data.
type AnalyticsSnapshot struct {
	ID              string                 `json:"id"`
	OrganizationID  string                 `json:"organization_id"`
	SnapshotDate    string                 `json:"snapshot_date"`
	ComplianceScore float64                `json:"compliance_score"`
	RiskScore       float64                `json:"risk_score"`
	ControlsTotal   int                    `json:"controls_total"`
	ControlsMet     int                    `json:"controls_met"`
	OpenRisks       int                    `json:"open_risks"`
	OpenIncidents   int                    `json:"open_incidents"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       string                 `json:"created_at"`
}

// TrendFilters holds filter parameters for trend data.
type TrendFilters struct {
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Granularity string `json:"granularity"` // daily, weekly, monthly
	FrameworkID string `json:"framework_id,omitempty"`
}

// TrendData represents trend data over a period.
type TrendData struct {
	Metric      string           `json:"metric"`
	Granularity string           `json:"granularity"`
	DataPoints  []TrendDataPoint `json:"data_points"`
	Summary     TrendSummary     `json:"summary"`
}

// TrendDataPoint is a single data point in a trend.
type TrendDataPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// TrendSummary provides aggregate statistics for a trend.
type TrendSummary struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Average float64 `json:"average"`
	Change  float64 `json:"change"`
	ChangePct float64 `json:"change_pct"`
}

// RiskPrediction represents a predicted risk trajectory.
type RiskPrediction struct {
	RiskID          string           `json:"risk_id"`
	CurrentScore    float64          `json:"current_score"`
	PredictedScore  float64          `json:"predicted_score"`
	Confidence      float64          `json:"confidence"`
	Horizon         string           `json:"horizon"` // 30d, 60d, 90d
	Trend           string           `json:"trend"`   // increasing, stable, decreasing
	Factors         []PredictionFactor `json:"factors"`
	DataPoints      []TrendDataPoint `json:"data_points"`
}

// PredictionFactor is a contributing factor to a risk prediction.
type PredictionFactor struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Impact string  `json:"impact"` // positive, negative, neutral
}

// BreachProbabilityParams holds parameters for breach probability forecast.
type BreachProbabilityParams struct {
	Horizon string `json:"horizon"` // 30d, 90d, 1y
}

// BreachProbability represents a breach probability forecast.
type BreachProbability struct {
	Probability     float64          `json:"probability"`
	Confidence      float64          `json:"confidence"`
	Horizon         string           `json:"horizon"`
	TopRiskFactors  []PredictionFactor `json:"top_risk_factors"`
	HistoricalComparison float64     `json:"historical_comparison"`
	Recommendations []string         `json:"recommendations,omitempty"`
}

// BenchmarkParams holds parameters for peer benchmarking.
type BenchmarkParams struct {
	Industry string `json:"industry"`
	Size     string `json:"size"` // small, medium, large, enterprise
}

// BenchmarkData represents peer comparison data.
type BenchmarkData struct {
	OrganizationScore float64          `json:"organization_score"`
	PeerAverage       float64          `json:"peer_average"`
	PeerMedian        float64          `json:"peer_median"`
	Percentile        float64          `json:"percentile"`
	SampleSize        int              `json:"sample_size"`
	Categories        []BenchmarkCategory `json:"categories"`
}

// BenchmarkCategory is a single category in benchmark comparison.
type BenchmarkCategory struct {
	Name       string  `json:"name"`
	OrgScore   float64 `json:"org_score"`
	PeerAvg    float64 `json:"peer_avg"`
	Percentile float64 `json:"percentile"`
}

// TimeSeriesParams holds parameters for time series queries.
type TimeSeriesParams struct {
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Granularity string `json:"granularity"`
}

// TimeSeriesData represents a metric time series.
type TimeSeriesData struct {
	Metric      string           `json:"metric"`
	Unit        string           `json:"unit"`
	Granularity string           `json:"granularity"`
	DataPoints  []TrendDataPoint `json:"data_points"`
}

// PeriodCompareParams holds parameters for period-over-period comparison.
type PeriodCompareParams struct {
	CurrentStart   string `json:"current_start"`
	CurrentEnd     string `json:"current_end"`
	PreviousStart  string `json:"previous_start"`
	PreviousEnd    string `json:"previous_end"`
}

// PeriodComparison represents a period-over-period comparison.
type PeriodComparison struct {
	Metric         string  `json:"metric"`
	CurrentValue   float64 `json:"current_value"`
	PreviousValue  float64 `json:"previous_value"`
	Change         float64 `json:"change"`
	ChangePct      float64 `json:"change_pct"`
	Trend          string  `json:"trend"`
}

// TopMoversParams holds parameters for top movers query.
type TopMoversParams struct {
	Period    string `json:"period"` // 7d, 30d, 90d
	Direction string `json:"direction"` // up, down, both
	Limit     int    `json:"limit"`
}

// TopMover represents an entity with the largest change.
type TopMover struct {
	EntityID   string  `json:"entity_id"`
	EntityType string  `json:"entity_type"`
	EntityName string  `json:"entity_name"`
	OldValue   float64 `json:"old_value"`
	NewValue   float64 `json:"new_value"`
	Change     float64 `json:"change"`
	ChangePct  float64 `json:"change_pct"`
}

// DistributionParams holds parameters for distribution queries.
type DistributionParams struct {
	GroupBy string `json:"group_by"`
}

// DistributionData represents the distribution of an entity.
type DistributionData struct {
	Entity   string             `json:"entity"`
	GroupBy  string             `json:"group_by"`
	Buckets  []DistributionBucket `json:"buckets"`
	Total    int                `json:"total"`
}

// DistributionBucket is a single bucket in a distribution.
type DistributionBucket struct {
	Label string  `json:"label"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
}

// ExportRequest is the payload for POST /analytics/export.
type ExportRequest struct {
	DataType  string `json:"data_type" validate:"required"` // compliance, risks, controls, incidents
	Format    string `json:"format" validate:"required"`    // csv, xlsx, json
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	Filters   map[string]string `json:"filters,omitempty"`
}

// ExportResult is the result of an analytics data export.
type ExportResult struct {
	ID          string `json:"id"`
	Status      string `json:"status"` // processing, completed, failed
	DownloadURL string `json:"download_url,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// CustomDashboard represents a user-defined analytics dashboard.
type CustomDashboard struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	Name           string          `json:"name" validate:"required"`
	Description    string          `json:"description,omitempty"`
	IsDefault      bool            `json:"is_default"`
	Widgets        []DashboardWidget `json:"widgets,omitempty"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

// DashboardWidget is a widget configuration within a custom dashboard.
type DashboardWidget struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
	Position WidgetPosition         `json:"position"`
	Config   map[string]interface{} `json:"config,omitempty"`
}

// WidgetPosition defines the layout position of a widget.
type WidgetPosition struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// WidgetType describes an available widget type.
type WidgetType struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	ConfigSchema map[string]interface{} `json:"config_schema,omitempty"`
}

// ---------- handler ----------

// AnalyticsHandler handles analytics and reporting endpoints.
type AnalyticsHandler struct {
	svc AnalyticsService
}

// NewAnalyticsHandler creates a new AnalyticsHandler with the given service.
func NewAnalyticsHandler(svc AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc}
}

// ListSnapshots handles GET /analytics/snapshots.
func (h *AnalyticsHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	snapshots, total, err := h.svc.ListSnapshots(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list analytics snapshots", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": snapshots,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetComplianceTrends handles GET /analytics/trends/compliance.
func (h *AnalyticsHandler) GetComplianceTrends(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	filters := TrendFilters{
		StartDate:   r.URL.Query().Get("start_date"),
		EndDate:     r.URL.Query().Get("end_date"),
		Granularity: r.URL.Query().Get("granularity"),
		FrameworkID: r.URL.Query().Get("framework_id"),
	}

	trends, err := h.svc.GetComplianceTrends(r.Context(), orgID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get compliance trends", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, trends)
}

// GetRiskTrends handles GET /analytics/trends/risks.
func (h *AnalyticsHandler) GetRiskTrends(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	filters := TrendFilters{
		StartDate:   r.URL.Query().Get("start_date"),
		EndDate:     r.URL.Query().Get("end_date"),
		Granularity: r.URL.Query().Get("granularity"),
	}

	trends, err := h.svc.GetRiskTrends(r.Context(), orgID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get risk trends", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, trends)
}

// GetRiskPrediction handles GET /analytics/predictions/risks/{riskId}.
func (h *AnalyticsHandler) GetRiskPrediction(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	riskID := chi.URLParam(r, "riskId")
	if riskID == "" {
		writeError(w, http.StatusBadRequest, "Missing risk ID", "")
		return
	}

	prediction, err := h.svc.GetRiskPrediction(r.Context(), orgID, riskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Risk prediction not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, prediction)
}

// GetBreachProbability handles GET /analytics/predictions/breach-probability.
func (h *AnalyticsHandler) GetBreachProbability(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	params := BreachProbabilityParams{
		Horizon: r.URL.Query().Get("horizon"),
	}

	probability, err := h.svc.GetBreachProbability(r.Context(), orgID, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get breach probability", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, probability)
}

// GetBenchmarks handles GET /analytics/benchmarks.
func (h *AnalyticsHandler) GetBenchmarks(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	params := BenchmarkParams{
		Industry: r.URL.Query().Get("industry"),
		Size:     r.URL.Query().Get("size"),
	}

	benchmarks, err := h.svc.GetBenchmarks(r.Context(), orgID, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get benchmarks", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, benchmarks)
}

// GetMetricTimeSeries handles GET /analytics/metrics/{metric}.
func (h *AnalyticsHandler) GetMetricTimeSeries(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	metric := chi.URLParam(r, "metric")
	if metric == "" {
		writeError(w, http.StatusBadRequest, "Missing metric name", "")
		return
	}

	params := TimeSeriesParams{
		StartDate:   r.URL.Query().Get("start_date"),
		EndDate:     r.URL.Query().Get("end_date"),
		Granularity: r.URL.Query().Get("granularity"),
	}

	data, err := h.svc.GetMetricTimeSeries(r.Context(), orgID, metric, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get metric time series", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, data)
}

// CompareMetricPeriods handles GET /analytics/metrics/{metric}/compare.
func (h *AnalyticsHandler) CompareMetricPeriods(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	metric := chi.URLParam(r, "metric")
	if metric == "" {
		writeError(w, http.StatusBadRequest, "Missing metric name", "")
		return
	}

	params := PeriodCompareParams{
		CurrentStart:  r.URL.Query().Get("current_start"),
		CurrentEnd:    r.URL.Query().Get("current_end"),
		PreviousStart: r.URL.Query().Get("previous_start"),
		PreviousEnd:   r.URL.Query().Get("previous_end"),
	}

	comparison, err := h.svc.CompareMetricPeriods(r.Context(), orgID, metric, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to compare metric periods", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, comparison)
}

// GetTopMovers handles GET /analytics/top-movers.
func (h *AnalyticsHandler) GetTopMovers(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := parseIntParam(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	params := TopMoversParams{
		Period:    r.URL.Query().Get("period"),
		Direction: r.URL.Query().Get("direction"),
		Limit:     limit,
	}

	movers, err := h.svc.GetTopMovers(r.Context(), orgID, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get top movers", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": movers})
}

// GetDistribution handles GET /analytics/distribution/{entity}.
func (h *AnalyticsHandler) GetDistribution(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	entity := chi.URLParam(r, "entity")
	if entity == "" {
		writeError(w, http.StatusBadRequest, "Missing entity", "")
		return
	}

	params := DistributionParams{
		GroupBy: r.URL.Query().Get("group_by"),
	}

	data, err := h.svc.GetDistribution(r.Context(), orgID, entity, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get distribution", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, data)
}

// ExportData handles POST /analytics/export.
func (h *AnalyticsHandler) ExportData(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.DataType == "" || req.Format == "" {
		writeError(w, http.StatusBadRequest, "data_type and format are required", "")
		return
	}

	result, err := h.svc.ExportData(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to export data", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// ListDashboards handles GET /analytics/dashboards.
func (h *AnalyticsHandler) ListDashboards(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboards, err := h.svc.ListDashboards(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list dashboards", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": dashboards})
}

// CreateDashboard handles POST /analytics/dashboards.
func (h *AnalyticsHandler) CreateDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var dashboard CustomDashboard
	if err := json.NewDecoder(r.Body).Decode(&dashboard); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if dashboard.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateDashboard(r.Context(), orgID, userID, &dashboard); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, dashboard)
}

// UpdateDashboard handles PUT /analytics/dashboards/{id}.
func (h *AnalyticsHandler) UpdateDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	dashboardID := chi.URLParam(r, "id")
	if dashboardID == "" {
		writeError(w, http.StatusBadRequest, "Missing dashboard ID", "")
		return
	}

	var dashboard CustomDashboard
	if err := json.NewDecoder(r.Body).Decode(&dashboard); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	dashboard.ID = dashboardID
	dashboard.OrganizationID = orgID

	if err := h.svc.UpdateDashboard(r.Context(), orgID, &dashboard); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dashboard)
}

// DeleteDashboard handles DELETE /analytics/dashboards/{id}.
func (h *AnalyticsHandler) DeleteDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	dashboardID := chi.URLParam(r, "id")
	if dashboardID == "" {
		writeError(w, http.StatusBadRequest, "Missing dashboard ID", "")
		return
	}

	if err := h.svc.DeleteDashboard(r.Context(), orgID, dashboardID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete dashboard", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetWidgetTypes handles GET /analytics/widget-types.
func (h *AnalyticsHandler) GetWidgetTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.svc.GetWidgetTypes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get widget types", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": types})
}

// parseIntParam is a helper that parses a string to int.
func parseIntParam(s string) (int, error) {
	return strconv.Atoi(s)
}
