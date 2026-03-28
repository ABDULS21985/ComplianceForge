package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// AnalyticsEngine provides compliance analytics, trend analysis, predictions, and benchmarking.
type AnalyticsEngine struct {
	pool *pgxpool.Pool
}

func NewAnalyticsEngine(pool *pgxpool.Pool) *AnalyticsEngine {
	return &AnalyticsEngine{pool: pool}
}

// --- Types ---

type MetricSnapshot struct {
	ID           string                 `json:"id"`
	OrgID        string                 `json:"organization_id"`
	SnapshotType string                 `json:"snapshot_type"`
	SnapshotDate string                 `json:"snapshot_date"`
	Metrics      map[string]interface{} `json:"metrics"`
	CreatedAt    string                 `json:"created_at"`
}

type ComplianceTrend struct {
	FrameworkCode   string  `json:"framework_code"`
	FrameworkName   string  `json:"framework_name"`
	MeasurementDate string  `json:"measurement_date"`
	ComplianceScore float64 `json:"compliance_score"`
	ControlsImpl    int     `json:"controls_implemented"`
	ControlsTotal   int     `json:"controls_total"`
	MaturityAvg     float64 `json:"maturity_avg"`
	ScoreChange7d   float64 `json:"score_change_7d"`
	ScoreChange30d  float64 `json:"score_change_30d"`
	ScoreChange90d  float64 `json:"score_change_90d"`
	TrendDirection  string  `json:"trend_direction"`
}

type RiskPrediction struct {
	RiskID                 string  `json:"risk_id"`
	PredictionDate         string  `json:"prediction_date"`
	PredictionType         string  `json:"prediction_type"`
	PredictedValue         float64 `json:"predicted_value"`
	ConfidenceIntervalLow  float64 `json:"confidence_interval_low"`
	ConfidenceIntervalHigh float64 `json:"confidence_interval_high"`
	ConfidenceLevel        float64 `json:"confidence_level"`
	ModelVersion           string  `json:"model_version"`
}

type BenchmarkEntry struct {
	BenchmarkType string  `json:"benchmark_type"`
	Category      string  `json:"category"`
	MetricName    string  `json:"metric_name"`
	Period        string  `json:"period"`
	Percentile25  float64 `json:"percentile_25"`
	Percentile50  float64 `json:"percentile_50"`
	Percentile75  float64 `json:"percentile_75"`
	Percentile90  float64 `json:"percentile_90"`
	SampleSize    int     `json:"sample_size"`
	OrgValue      float64 `json:"org_value"`
	OrgPercentile float64 `json:"org_percentile"`
}

type TimeSeriesPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type MetricComparison struct {
	MetricName     string  `json:"metric_name"`
	CurrentValue   float64 `json:"current_value"`
	PreviousValue  float64 `json:"previous_value"`
	ChangeAbsolute float64 `json:"change_absolute"`
	ChangePercent  float64 `json:"change_percent"`
	Direction      string  `json:"direction"`
}

type TopMover struct {
	EntityID   string  `json:"entity_id"`
	EntityRef  string  `json:"entity_ref"`
	EntityName string  `json:"entity_name"`
	MetricName string  `json:"metric_name"`
	OldValue   float64 `json:"old_value"`
	NewValue   float64 `json:"new_value"`
	Change     float64 `json:"change"`
	Direction  string  `json:"direction"`
}

type DistributionEntry struct {
	Label      string  `json:"label"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

type CustomDashboard struct {
	ID        string                   `json:"id"`
	OrgID     string                   `json:"organization_id"`
	Name      string                   `json:"name"`
	Layout    []map[string]interface{} `json:"layout"`
	IsDefault bool                     `json:"is_default"`
	IsShared  bool                     `json:"is_shared"`
	OwnerID   string                   `json:"owner_user_id"`
}

type WidgetType struct {
	WidgetType       string   `json:"widget_type"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	AvailableMetrics []string `json:"available_metrics"`
	MinWidth         int      `json:"min_width"`
	MinHeight        int      `json:"min_height"`
}

// --- Snapshot Collection ---

// TakeSnapshot captures all current metrics for an organization.
func (ae *AnalyticsEngine) TakeSnapshot(ctx context.Context, orgID, snapshotType string) (*MetricSnapshot, error) {
	metrics := make(map[string]interface{})

	// Compliance metrics
	var overallScore float64
	var totalControls, implControls, gapsTotal int
	err := ae.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(compliance_score), 0),
		       COALESCE(SUM(total_controls), 0)::INT,
		       COALESCE(SUM(effective_count + implemented_count), 0)::INT,
		       COALESCE(SUM(not_implemented_count + partial_count), 0)::INT
		FROM v_compliance_score_by_framework
		WHERE organization_id = $1
	`, orgID).Scan(&overallScore, &totalControls, &implControls, &gapsTotal)
	if err != nil {
		// Views may not exist yet, use fallback
		overallScore = 0
	}
	metrics["compliance"] = map[string]interface{}{
		"overall_score":        math.Round(overallScore*100) / 100,
		"controls_total":       totalControls,
		"controls_implemented": implControls,
		"gaps_total":           gapsTotal,
	}

	// Risk metrics
	var totalRisks, criticalRisks, highRisks int
	var avgResidual float64
	_ = ae.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE residual_risk_level = 'critical'),
		       COUNT(*) FILTER (WHERE residual_risk_level = 'high'),
		       COALESCE(AVG(residual_risk_score), 0)
		FROM risks WHERE organization_id = $1 AND deleted_at IS NULL AND status != 'closed'
	`, orgID).Scan(&totalRisks, &criticalRisks, &highRisks, &avgResidual)
	metrics["risks"] = map[string]interface{}{
		"total":              totalRisks,
		"critical":           criticalRisks,
		"high":               highRisks,
		"avg_residual_score": math.Round(avgResidual*100) / 100,
	}

	// Incident metrics
	var openIncidents, breaches int
	_ = ae.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status NOT IN ('closed', 'resolved')),
		       COUNT(*) FILTER (WHERE is_data_breach = true)
		FROM incidents WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&openIncidents, &breaches)
	metrics["incidents"] = map[string]interface{}{
		"total_open": openIncidents,
		"breaches":   breaches,
	}

	// Policy metrics
	var totalPolicies, published, overdueReview int
	var attestRate float64
	_ = ae.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'published'),
		       COUNT(*) FILTER (WHERE review_status = 'overdue'),
		       0
		FROM policies WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&totalPolicies, &published, &overdueReview, &attestRate)
	metrics["policies"] = map[string]interface{}{
		"total":            totalPolicies,
		"published":        published,
		"overdue_review":   overdueReview,
		"attestation_rate": attestRate,
	}

	// Vendor metrics
	var totalVendors, highRiskVendors, missingDPA int
	_ = ae.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE risk_tier IN ('critical', 'high')),
		       COUNT(*) FILTER (WHERE data_processing = true AND dpa_in_place = false)
		FROM vendors WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&totalVendors, &highRiskVendors, &missingDPA)
	metrics["vendors"] = map[string]interface{}{
		"total":       totalVendors,
		"high_risk":   highRiskVendors,
		"missing_dpa": missingDPA,
	}

	metricsJSON, _ := json.Marshal(metrics)

	var id string
	err = ae.pool.QueryRow(ctx, `
		INSERT INTO analytics_snapshots (organization_id, snapshot_type, snapshot_date, metrics)
		VALUES ($1, $2, CURRENT_DATE, $3)
		ON CONFLICT (organization_id, snapshot_type, snapshot_date)
		DO UPDATE SET metrics = $3
		RETURNING id
	`, orgID, snapshotType, metricsJSON).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("inserting snapshot: %w", err)
	}

	log.Info().Str("org_id", orgID).Str("type", snapshotType).Msg("analytics: snapshot captured")

	return &MetricSnapshot{
		ID: id, OrgID: orgID, SnapshotType: snapshotType,
		SnapshotDate: time.Now().Format("2006-01-02"), Metrics: metrics,
	}, nil
}

// ListSnapshots returns historical snapshots for an org.
func (ae *AnalyticsEngine) ListSnapshots(ctx context.Context, orgID string, snapshotType string, limit int) ([]MetricSnapshot, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := ae.pool.Query(ctx, `
		SELECT id, snapshot_type, snapshot_date, metrics, created_at
		FROM analytics_snapshots
		WHERE organization_id = $1 AND ($2 = '' OR snapshot_type = $2)
		ORDER BY snapshot_date DESC
		LIMIT $3
	`, orgID, snapshotType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MetricSnapshot
	for rows.Next() {
		var s MetricSnapshot
		var metricsJSON []byte
		var createdAt time.Time
		if err := rows.Scan(&s.ID, &s.SnapshotType, &s.SnapshotDate, &metricsJSON, &createdAt); err != nil {
			continue
		}
		_ = json.Unmarshal(metricsJSON, &s.Metrics)
		s.OrgID = orgID
		s.CreatedAt = createdAt.Format(time.RFC3339)
		results = append(results, s)
	}
	return results, nil
}

// --- Trend Analysis ---

// CalculateComplianceTrends computes trends per framework.
func (ae *AnalyticsEngine) CalculateComplianceTrends(ctx context.Context, orgID string) ([]ComplianceTrend, error) {
	rows, err := ae.pool.Query(ctx, `
		SELECT framework_code, framework_name, measurement_date,
		       compliance_score, controls_implemented, controls_total,
		       maturity_avg, score_change_7d, score_change_30d, score_change_90d, trend_direction
		FROM analytics_compliance_trends
		WHERE organization_id = $1
		ORDER BY measurement_date DESC, framework_code
		LIMIT 100
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []ComplianceTrend
	for rows.Next() {
		var t ComplianceTrend
		if err := rows.Scan(&t.FrameworkCode, &t.FrameworkName, &t.MeasurementDate,
			&t.ComplianceScore, &t.ControlsImpl, &t.ControlsTotal,
			&t.MaturityAvg, &t.ScoreChange7d, &t.ScoreChange30d, &t.ScoreChange90d, &t.TrendDirection,
		); err != nil {
			continue
		}
		trends = append(trends, t)
	}
	return trends, nil
}

// --- Predictions ---

// PredictRiskScoreTrajectory uses exponential smoothing on historical scores.
func (ae *AnalyticsEngine) PredictRiskScoreTrajectory(ctx context.Context, orgID, riskID string) (*RiskPrediction, error) {
	// Fetch last 10 assessment scores for this risk
	rows, err := ae.pool.Query(ctx, `
		SELECT score_after, assessment_date
		FROM risk_assessments
		WHERE organization_id = $1 AND risk_id = $2 AND score_after IS NOT NULL
		ORDER BY assessment_date DESC LIMIT 10
	`, orgID, riskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []float64
	for rows.Next() {
		var score float64
		var date time.Time
		if err := rows.Scan(&score, &date); err != nil {
			continue
		}
		scores = append(scores, score)
	}

	if len(scores) < 2 {
		// Not enough data -- return current score as prediction
		var currentScore float64
		_ = ae.pool.QueryRow(ctx, `SELECT COALESCE(residual_risk_score, 0) FROM risks WHERE id = $1 AND organization_id = $2`, riskID, orgID).Scan(&currentScore)
		return &RiskPrediction{
			RiskID:                 riskID,
			PredictionDate:         time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
			PredictionType:         "score_forecast",
			PredictedValue:         currentScore,
			ConfidenceIntervalLow:  currentScore * 0.8,
			ConfidenceIntervalHigh: currentScore * 1.2,
			ConfidenceLevel:        0.50,
			ModelVersion:           "simple_v1",
		}, nil
	}

	// Simple exponential smoothing (alpha=0.3)
	alpha := 0.3
	smoothed := scores[len(scores)-1]
	for i := len(scores) - 2; i >= 0; i-- {
		smoothed = alpha*scores[i] + (1-alpha)*smoothed
	}

	// Calculate trend from last 3 points
	trend := 0.0
	if len(scores) >= 3 {
		trend = (scores[0] - scores[2]) / 2.0
	}

	predicted30d := smoothed + trend
	if predicted30d < 0 {
		predicted30d = 0
	}
	if predicted30d > 25 {
		predicted30d = 25
	}

	// Confidence interval: wider with fewer data points
	spread := 3.0 / math.Sqrt(float64(len(scores)))

	return &RiskPrediction{
		RiskID:                 riskID,
		PredictionDate:         time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
		PredictionType:         "score_forecast",
		PredictedValue:         math.Round(predicted30d*100) / 100,
		ConfidenceIntervalLow:  math.Max(0, math.Round((predicted30d-spread)*100)/100),
		ConfidenceIntervalHigh: math.Min(25, math.Round((predicted30d+spread)*100)/100),
		ConfidenceLevel:        0.80,
		ModelVersion:           "exponential_smoothing_v1",
	}, nil
}

// PredictBreachProbability estimates breach likelihood based on risk posture.
func (ae *AnalyticsEngine) PredictBreachProbability(ctx context.Context, orgID string) (map[string]interface{}, error) {
	var criticalRisks, highRisks, totalRisks int
	var avgScore float64
	var incidentsLast12mo int

	_ = ae.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE residual_risk_level = 'critical'),
		       COUNT(*) FILTER (WHERE residual_risk_level = 'high'),
		       COALESCE(AVG(residual_risk_score), 0)
		FROM risks WHERE organization_id = $1 AND deleted_at IS NULL AND status != 'closed'
	`, orgID).Scan(&totalRisks, &criticalRisks, &highRisks, &avgScore)

	_ = ae.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM incidents
		WHERE organization_id = $1 AND is_data_breach = true AND created_at > NOW() - INTERVAL '12 months'
	`, orgID).Scan(&incidentsLast12mo)

	// Simple logistic model: probability increases with risk exposure
	riskFactor := float64(criticalRisks)*0.15 + float64(highRisks)*0.08 + avgScore*0.02
	historyFactor := float64(incidentsLast12mo) * 0.10
	baseRate := 0.05 // 5% base rate

	prob30d := math.Min(0.95, baseRate+riskFactor*0.3+historyFactor*0.5)
	prob90d := math.Min(0.95, baseRate+riskFactor*0.7+historyFactor*0.8)
	prob365d := math.Min(0.95, baseRate+riskFactor+historyFactor)

	return map[string]interface{}{
		"probability_30d":  math.Round(prob30d*10000) / 10000,
		"probability_90d":  math.Round(prob90d*10000) / 10000,
		"probability_365d": math.Round(prob365d*10000) / 10000,
		"model_version":    "logistic_v1",
		"confidence_level": 0.70,
		"input_factors": map[string]interface{}{
			"critical_risks":     criticalRisks,
			"high_risks":         highRisks,
			"avg_residual_score": avgScore,
			"incidents_last_12mo": incidentsLast12mo,
		},
		"note": "This is a statistical estimate based on current risk posture. It is not a prediction of a specific event.",
	}, nil
}

// --- Benchmarking ---

// GetBenchmarks returns peer comparison data for an org.
func (ae *AnalyticsEngine) GetBenchmarks(ctx context.Context, orgID string) ([]BenchmarkEntry, error) {
	// Fetch org's industry for comparison
	var industry string
	_ = ae.pool.QueryRow(ctx, `SELECT COALESCE(industry, '') FROM organizations WHERE id = $1`, orgID).Scan(&industry)

	rows, err := ae.pool.Query(ctx, `
		SELECT benchmark_type, category, metric_name, period,
		       percentile_25, percentile_50, percentile_75, percentile_90, sample_size
		FROM analytics_benchmarks
		WHERE (benchmark_type = 'industry' AND category = $1)
		   OR benchmark_type = 'overall'
		ORDER BY metric_name, benchmark_type
	`, industry)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []BenchmarkEntry
	for rows.Next() {
		var b BenchmarkEntry
		if err := rows.Scan(&b.BenchmarkType, &b.Category, &b.MetricName, &b.Period,
			&b.Percentile25, &b.Percentile50, &b.Percentile75, &b.Percentile90, &b.SampleSize,
		); err != nil {
			continue
		}
		entries = append(entries, b)
	}
	return entries, nil
}

// --- Time Series Queries ---

// GetMetricTimeSeries returns historical data points for charting.
func (ae *AnalyticsEngine) GetMetricTimeSeries(ctx context.Context, orgID, metric, period string) ([]TimeSeriesPoint, error) {
	interval := "30 days"
	switch period {
	case "7d":
		interval = "7 days"
	case "30d":
		interval = "30 days"
	case "90d":
		interval = "90 days"
	case "12m":
		interval = "365 days"
	case "24m":
		interval = "730 days"
	}

	rows, err := ae.pool.Query(ctx, `
		SELECT snapshot_date, metrics->$2 AS metric_value
		FROM analytics_snapshots
		WHERE organization_id = $1
		  AND snapshot_date >= CURRENT_DATE - $3::interval
		ORDER BY snapshot_date ASC
	`, orgID, metric, interval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TimeSeriesPoint
	for rows.Next() {
		var date string
		var valueJSON json.RawMessage
		if err := rows.Scan(&date, &valueJSON); err != nil {
			continue
		}
		var value float64
		_ = json.Unmarshal(valueJSON, &value)
		points = append(points, TimeSeriesPoint{Date: date, Value: value})
	}
	return points, nil
}

// GetMetricComparison compares a metric between two periods.
func (ae *AnalyticsEngine) GetMetricComparison(ctx context.Context, orgID, metric, currentPeriod, previousPeriod string) (*MetricComparison, error) {
	daysDiff := 30
	switch currentPeriod {
	case "7d":
		daysDiff = 7
	case "30d":
		daysDiff = 30
	case "90d":
		daysDiff = 90
	}

	var currentVal, previousVal float64
	_ = ae.pool.QueryRow(ctx, `
		SELECT COALESCE((metrics->>$2)::DECIMAL, 0)
		FROM analytics_snapshots
		WHERE organization_id = $1 ORDER BY snapshot_date DESC LIMIT 1
	`, orgID, metric).Scan(&currentVal)

	_ = ae.pool.QueryRow(ctx, `
		SELECT COALESCE((metrics->>$2)::DECIMAL, 0)
		FROM analytics_snapshots
		WHERE organization_id = $1 AND snapshot_date <= CURRENT_DATE - $3 * INTERVAL '1 day'
		ORDER BY snapshot_date DESC LIMIT 1
	`, orgID, metric, daysDiff).Scan(&previousVal)

	change := currentVal - previousVal
	changePct := 0.0
	if previousVal != 0 {
		changePct = (change / previousVal) * 100
	}
	direction := "stable"
	if change > 0.5 {
		direction = "improving"
	} else if change < -0.5 {
		direction = "declining"
	}

	return &MetricComparison{
		MetricName:     metric,
		CurrentValue:   currentVal,
		PreviousValue:  previousVal,
		ChangeAbsolute: math.Round(change*100) / 100,
		ChangePercent:  math.Round(changePct*100) / 100,
		Direction:      direction,
	}, nil
}

// GetTopMovers returns entities with the biggest metric changes.
func (ae *AnalyticsEngine) GetTopMovers(ctx context.Context, orgID, metric, period, direction string, limit int) ([]TopMover, error) {
	if limit <= 0 {
		limit = 10
	}
	// For compliance_score top movers, compare across frameworks
	rows, err := ae.pool.Query(ctx, `
		SELECT framework_code, framework_name, compliance_score, score_change_30d, trend_direction
		FROM analytics_compliance_trends
		WHERE organization_id = $1 AND measurement_date = (
			SELECT MAX(measurement_date) FROM analytics_compliance_trends WHERE organization_id = $1
		)
		ORDER BY ABS(score_change_30d) DESC
		LIMIT $2
	`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movers []TopMover
	for rows.Next() {
		var m TopMover
		var score, change float64
		if err := rows.Scan(&m.EntityRef, &m.EntityName, &score, &change, &m.Direction); err != nil {
			continue
		}
		m.MetricName = "compliance_score"
		m.NewValue = score
		m.OldValue = score - change
		m.Change = change
		movers = append(movers, m)
	}
	return movers, nil
}

// GetDistribution returns entity counts grouped by a dimension.
func (ae *AnalyticsEngine) GetDistribution(ctx context.Context, orgID, entity, groupBy string) ([]DistributionEntry, error) {
	var query string
	switch entity {
	case "risks":
		query = `SELECT COALESCE(residual_risk_level, 'unassessed') AS label, COUNT(*) AS cnt
		         FROM risks WHERE organization_id = $1 AND deleted_at IS NULL AND status != 'closed'
		         GROUP BY label ORDER BY cnt DESC`
	case "controls":
		query = `SELECT COALESCE(status, 'unknown') AS label, COUNT(*) AS cnt
		         FROM control_implementations WHERE organization_id = $1 AND deleted_at IS NULL
		         GROUP BY label ORDER BY cnt DESC`
	case "incidents":
		query = `SELECT COALESCE(severity, 'unknown') AS label, COUNT(*) AS cnt
		         FROM incidents WHERE organization_id = $1 AND deleted_at IS NULL
		         GROUP BY label ORDER BY cnt DESC`
	case "findings":
		query = `SELECT COALESCE(severity, 'unknown') AS label, COUNT(*) AS cnt
		         FROM audit_findings WHERE organization_id = $1 AND deleted_at IS NULL
		         GROUP BY label ORDER BY cnt DESC`
	default:
		return nil, fmt.Errorf("unsupported entity type: %s", entity)
	}

	rows, err := ae.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []DistributionEntry
	total := 0
	for rows.Next() {
		var e DistributionEntry
		if err := rows.Scan(&e.Label, &e.Count); err != nil {
			continue
		}
		total += e.Count
		entries = append(entries, e)
	}
	for i := range entries {
		if total > 0 {
			entries[i].Percentage = math.Round(float64(entries[i].Count)/float64(total)*10000) / 100
		}
	}
	return entries, nil
}

// ExportAnalyticsData exports raw data as JSON for external BI tools.
func (ae *AnalyticsEngine) ExportAnalyticsData(ctx context.Context, orgID string, config map[string]interface{}) ([]byte, error) {
	entity, _ := config["entity"].(string)
	exportData := map[string]interface{}{
		"organization_id": orgID,
		"exported_at":     time.Now().UTC().Format(time.RFC3339),
		"entity":          entity,
	}

	switch entity {
	case "snapshots":
		snapshots, _ := ae.ListSnapshots(ctx, orgID, "", 365)
		exportData["data"] = snapshots
	case "compliance_trends":
		trends, _ := ae.CalculateComplianceTrends(ctx, orgID)
		exportData["data"] = trends
	default:
		exportData["data"] = []interface{}{}
	}

	return json.MarshalIndent(exportData, "", "  ")
}

// --- Custom Dashboards ---

func (ae *AnalyticsEngine) ListDashboards(ctx context.Context, orgID, userID string) ([]CustomDashboard, error) {
	rows, err := ae.pool.Query(ctx, `
		SELECT id, name, layout, is_default, is_shared, owner_user_id
		FROM analytics_custom_dashboards
		WHERE organization_id = $1 AND (owner_user_id = $2 OR is_shared = true)
		ORDER BY is_default DESC, name ASC
	`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dashboards []CustomDashboard
	for rows.Next() {
		var d CustomDashboard
		var layoutJSON []byte
		if err := rows.Scan(&d.ID, &d.Name, &layoutJSON, &d.IsDefault, &d.IsShared, &d.OwnerID); err != nil {
			continue
		}
		_ = json.Unmarshal(layoutJSON, &d.Layout)
		d.OrgID = orgID
		dashboards = append(dashboards, d)
	}
	return dashboards, nil
}

func (ae *AnalyticsEngine) CreateDashboard(ctx context.Context, orgID, userID string, dash CustomDashboard) (*CustomDashboard, error) {
	layoutJSON, _ := json.Marshal(dash.Layout)
	var id string
	err := ae.pool.QueryRow(ctx, `
		INSERT INTO analytics_custom_dashboards (organization_id, name, description, layout, is_default, is_shared, owner_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id
	`, orgID, dash.Name, "", layoutJSON, dash.IsDefault, dash.IsShared, userID).Scan(&id)
	if err != nil {
		return nil, err
	}
	dash.ID = id
	dash.OrgID = orgID
	dash.OwnerID = userID
	return &dash, nil
}

func (ae *AnalyticsEngine) UpdateDashboard(ctx context.Context, orgID, dashID string, dash CustomDashboard) error {
	layoutJSON, _ := json.Marshal(dash.Layout)
	_, err := ae.pool.Exec(ctx, `
		UPDATE analytics_custom_dashboards
		SET name = $3, layout = $4, is_default = $5, is_shared = $6, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
	`, dashID, orgID, dash.Name, layoutJSON, dash.IsDefault, dash.IsShared)
	return err
}

func (ae *AnalyticsEngine) DeleteDashboard(ctx context.Context, orgID, dashID string) error {
	_, err := ae.pool.Exec(ctx, `
		DELETE FROM analytics_custom_dashboards WHERE id = $1 AND organization_id = $2
	`, dashID, orgID)
	return err
}

func (ae *AnalyticsEngine) GetWidgetTypes(ctx context.Context) ([]WidgetType, error) {
	rows, err := ae.pool.Query(ctx, `
		SELECT widget_type, name, description, available_metrics, min_width, min_height
		FROM analytics_widget_types ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []WidgetType
	for rows.Next() {
		var w WidgetType
		if err := rows.Scan(&w.WidgetType, &w.Name, &w.Description, &w.AvailableMetrics, &w.MinWidth, &w.MinHeight); err != nil {
			continue
		}
		types = append(types, w)
	}
	return types, nil
}
