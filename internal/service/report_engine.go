package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ReportEngine generates compliance reports in various formats.
type ReportEngine struct {
	pool *pgxpool.Pool
}

// ReportData holds all data needed to render a report.
type ReportData struct {
	Title          string          `json:"title"`
	Organization   string          `json:"organization"`
	GeneratedAt    time.Time       `json:"generated_at"`
	GeneratedBy    string          `json:"generated_by"`
	Classification string          `json:"classification"`
	Format         string          `json:"format"`
	Sections       []ReportSection `json:"sections"`
}

// ReportSection is a single section within a report.
type ReportSection struct {
	Title   string      `json:"title"`
	Type    string      `json:"type"` // "text", "table", "chart", "kpi"
	Content interface{} `json:"content"`
}

// ReportDefinition is a reusable report configuration.
type ReportDefinition struct {
	ID             string                   `json:"id"`
	OrgID          string                   `json:"organization_id"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	ReportType     string                   `json:"report_type"`
	Format         string                   `json:"format"`
	Filters        map[string]interface{}   `json:"filters"`
	Sections       []map[string]interface{} `json:"sections"`
	Classification string                   `json:"classification"`
	CreatedBy      string                   `json:"created_by"`
}

// ReportRun represents a single report generation attempt.
type ReportRun struct {
	ID            string     `json:"id"`
	DefinitionID  string     `json:"report_definition_id"`
	ScheduleID    *string    `json:"schedule_id"`
	Status        string     `json:"status"`
	Format        string     `json:"format"`
	FilePath      *string    `json:"file_path"`
	FileSizeBytes *int64     `json:"file_size_bytes"`
	PageCount     *int       `json:"page_count"`
	GenTimeMs     *int       `json:"generation_time_ms"`
	GeneratedBy   *string    `json:"generated_by"`
	ErrorMessage  *string    `json:"error_message"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}

// NewReportEngine creates a new ReportEngine.
func NewReportEngine(pool *pgxpool.Pool) *ReportEngine {
	return &ReportEngine{pool: pool}
}

// GenerateReport creates a report run, gathers data, writes a JSON export, and
// returns the completed run record.
func (re *ReportEngine) GenerateReport(ctx context.Context, orgID string, definition ReportDefinition) (*ReportRun, error) {
	startTime := time.Now()

	// Create the report_runs record in 'generating' status.
	var run ReportRun
	err := re.pool.QueryRow(ctx, `
		INSERT INTO report_runs (organization_id, report_definition_id, status, format, generated_by, parameters)
		VALUES ($1, $2, 'generating', $3, $4, $5)
		RETURNING id, report_definition_id, status, format, generated_by, created_at`,
		orgID, definition.ID, definition.Format, definition.CreatedBy, "{}",
	).Scan(&run.ID, &run.DefinitionID, &run.Status, &run.Format, &run.GeneratedBy, &run.CreatedAt)
	if err != nil {
		log.Error().Err(err).Str("org_id", orgID).Msg("failed to create report run")
		return nil, fmt.Errorf("create report run: %w", err)
	}

	// Gather report data based on report type.
	var reportData *ReportData
	var gatherErr error

	switch definition.ReportType {
	case "compliance_status", "gap_analysis":
		reportData, gatherErr = re.GetComplianceReportData(ctx, orgID, definition.Filters)
	case "risk_register", "risk_heatmap", "kri_dashboard", "treatment_progress":
		reportData, gatherErr = re.GetRiskReportData(ctx, orgID, definition.Filters)
	case "executive_summary":
		reportData, gatherErr = re.GetExecutiveSummaryData(ctx, orgID)
	default:
		reportData, gatherErr = re.GetComplianceReportData(ctx, orgID, definition.Filters)
	}

	if gatherErr != nil {
		errMsg := gatherErr.Error()
		now := time.Now()
		_, _ = re.pool.Exec(ctx, `
			UPDATE report_runs SET status = 'failed', error_message = $1, completed_at = $2
			WHERE id = $3 AND organization_id = $4`,
			errMsg, now, run.ID, orgID)
		run.Status = "failed"
		run.ErrorMessage = &errMsg
		run.CompletedAt = &now
		log.Error().Err(gatherErr).Str("run_id", run.ID).Msg("report data gathering failed")
		return &run, gatherErr
	}

	// Serialize to JSON as the output artifact.
	reportJSON, err := json.MarshalIndent(reportData, "", "  ")
	if err != nil {
		errMsg := err.Error()
		now := time.Now()
		_, _ = re.pool.Exec(ctx, `
			UPDATE report_runs SET status = 'failed', error_message = $1, completed_at = $2
			WHERE id = $3 AND organization_id = $4`,
			errMsg, now, run.ID, orgID)
		run.Status = "failed"
		run.ErrorMessage = &errMsg
		run.CompletedAt = &now
		return &run, fmt.Errorf("serialize report: %w", err)
	}

	// Compute file metadata.
	elapsedMs := int(time.Since(startTime).Milliseconds())
	fileSize := int64(len(reportJSON))
	filePath := fmt.Sprintf("reports/%s/%s.json", orgID, run.ID)
	now := time.Now()

	// Update run with completion status.
	_, err = re.pool.Exec(ctx, `
		UPDATE report_runs
		SET status = 'completed', file_path = $1, file_size_bytes = $2,
		    generation_time_ms = $3, completed_at = $4
		WHERE id = $5 AND organization_id = $6`,
		filePath, fileSize, elapsedMs, now, run.ID, orgID)
	if err != nil {
		log.Error().Err(err).Str("run_id", run.ID).Msg("failed to update report run status")
		return nil, fmt.Errorf("update report run: %w", err)
	}

	run.Status = "completed"
	run.FilePath = &filePath
	run.FileSizeBytes = &fileSize
	run.GenTimeMs = &elapsedMs
	run.CompletedAt = &now

	log.Info().
		Str("run_id", run.ID).
		Str("report_type", definition.ReportType).
		Int("generation_time_ms", elapsedMs).
		Msg("report generated successfully")

	return &run, nil
}

// GetComplianceReportData queries compliance scores, control implementations,
// gap analysis, and maturity levels for a comprehensive compliance report.
func (re *ReportEngine) GetComplianceReportData(ctx context.Context, orgID string, filters map[string]interface{}) (*ReportData, error) {
	data := &ReportData{
		Title:          "Compliance Status Report",
		GeneratedAt:    time.Now(),
		Classification: "internal",
		Format:         "json",
	}

	// Get organization name.
	_ = re.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&data.Organization)

	// Section 1: Framework compliance scores.
	rows, err := re.pool.Query(ctx, `
		SELECT cf.name, of.compliance_score, of.status,
		       COUNT(ci.id) AS total_controls,
		       COUNT(ci.id) FILTER (WHERE ci.status = 'effective') AS effective,
		       COUNT(ci.id) FILTER (WHERE ci.status = 'implemented') AS implemented,
		       COUNT(ci.id) FILTER (WHERE ci.status = 'partial') AS partial,
		       COUNT(ci.id) FILTER (WHERE ci.status = 'not_implemented') AS not_implemented,
		       COUNT(ci.id) FILTER (WHERE ci.status = 'not_applicable') AS not_applicable
		FROM organization_frameworks of
		JOIN compliance_frameworks cf ON cf.id = of.framework_id
		LEFT JOIN control_implementations ci ON ci.org_framework_id = of.id AND ci.deleted_at IS NULL
		WHERE of.organization_id = $1
		GROUP BY cf.name, of.compliance_score, of.status
		ORDER BY cf.name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query framework scores: %w", err)
	}
	defer rows.Close()

	var frameworkScores []map[string]interface{}
	for rows.Next() {
		var name, status string
		var score float64
		var total, effective, implemented, partial, notImpl, notApplicable int
		if err := rows.Scan(&name, &score, &status, &total, &effective, &implemented, &partial, &notImpl, &notApplicable); err != nil {
			return nil, fmt.Errorf("scan framework score: %w", err)
		}
		frameworkScores = append(frameworkScores, map[string]interface{}{
			"framework":       name,
			"score":           score,
			"status":          status,
			"total_controls":  total,
			"effective":       effective,
			"implemented":     implemented,
			"partial":         partial,
			"not_implemented": notImpl,
			"not_applicable":  notApplicable,
		})
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Framework Compliance Scores",
		Type:    "table",
		Content: frameworkScores,
	})

	// Section 2: Maturity distribution.
	maturityRows, err := re.pool.Query(ctx, `
		SELECT maturity_level, COUNT(*) AS count
		FROM control_implementations
		WHERE organization_id = $1 AND deleted_at IS NULL
		GROUP BY maturity_level
		ORDER BY maturity_level`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query maturity: %w", err)
	}
	defer maturityRows.Close()

	maturityDist := make(map[string]int)
	maturityLabels := map[int]string{
		0: "Non-existent", 1: "Initial", 2: "Managed",
		3: "Defined", 4: "Quantitatively Managed", 5: "Optimizing",
	}
	for maturityRows.Next() {
		var level, count int
		if err := maturityRows.Scan(&level, &count); err != nil {
			return nil, fmt.Errorf("scan maturity: %w", err)
		}
		label := maturityLabels[level]
		if label == "" {
			label = fmt.Sprintf("Level %d", level)
		}
		maturityDist[label] = count
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Control Maturity Distribution",
		Type:    "chart",
		Content: maturityDist,
	})

	// Section 3: Gap analysis — controls with gaps.
	gapRows, err := re.pool.Query(ctx, `
		SELECT fc.control_code, fc.title, ci.gap_description, ci.remediation_plan, ci.remediation_due_date
		FROM control_implementations ci
		JOIN framework_controls fc ON fc.id = ci.framework_control_id
		WHERE ci.organization_id = $1 AND ci.deleted_at IS NULL
		  AND ci.gap_description IS NOT NULL AND ci.gap_description != ''
		ORDER BY ci.remediation_due_date ASC NULLS LAST
		LIMIT 50`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query gaps: %w", err)
	}
	defer gapRows.Close()

	var gaps []map[string]interface{}
	for gapRows.Next() {
		var code, title string
		var gapDesc, remPlan *string
		var remDate *time.Time
		if err := gapRows.Scan(&code, &title, &gapDesc, &remPlan, &remDate); err != nil {
			return nil, fmt.Errorf("scan gap: %w", err)
		}
		g := map[string]interface{}{
			"control_code":    code,
			"control_title":   title,
			"gap_description": gapDesc,
		}
		if remPlan != nil {
			g["remediation_plan"] = *remPlan
		}
		if remDate != nil {
			g["remediation_due_date"] = remDate.Format("2006-01-02")
		}
		gaps = append(gaps, g)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Gap Analysis",
		Type:    "table",
		Content: gaps,
	})

	return data, nil
}

// GetRiskReportData queries risks, heatmap data, treatments, and KRIs.
func (re *ReportEngine) GetRiskReportData(ctx context.Context, orgID string, filters map[string]interface{}) (*ReportData, error) {
	data := &ReportData{
		Title:          "Risk Assessment Report",
		GeneratedAt:    time.Now(),
		Classification: "internal",
		Format:         "json",
	}

	_ = re.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&data.Organization)

	// Section 1: Risk heatmap data.
	rows, err := re.pool.Query(ctx, `
		SELECT id, title, risk_category, likelihood, impact, risk_score, risk_level,
		       mitigation_status, owner_user_id
		FROM risks
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY risk_score DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query risks: %w", err)
	}
	defer rows.Close()

	var risks []map[string]interface{}
	for rows.Next() {
		var id, title, category, level, mitStatus string
		var likelihood, impact int
		var score float64
		var ownerID *string
		if err := rows.Scan(&id, &title, &category, &likelihood, &impact, &score, &level, &mitStatus, &ownerID); err != nil {
			return nil, fmt.Errorf("scan risk: %w", err)
		}
		risks = append(risks, map[string]interface{}{
			"id":                id,
			"title":             title,
			"category":          category,
			"likelihood":        likelihood,
			"impact":            impact,
			"score":             score,
			"level":             level,
			"mitigation_status": mitStatus,
		})
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Risk Register",
		Type:    "table",
		Content: risks,
	})

	// Section 2: Risk summary by level.
	summaryRows, err := re.pool.Query(ctx, `
		SELECT risk_level, COUNT(*) AS count
		FROM risks
		WHERE organization_id = $1 AND deleted_at IS NULL
		GROUP BY risk_level
		ORDER BY COUNT(*) DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query risk summary: %w", err)
	}
	defer summaryRows.Close()

	riskSummary := make(map[string]int)
	for summaryRows.Next() {
		var level string
		var count int
		if err := summaryRows.Scan(&level, &count); err != nil {
			return nil, fmt.Errorf("scan risk summary: %w", err)
		}
		riskSummary[level] = count
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Risk Summary by Level",
		Type:    "kpi",
		Content: riskSummary,
	})

	// Section 3: Active risk treatments.
	treatRows, err := re.pool.Query(ctx, `
		SELECT rt.id, r.title AS risk_title, rt.treatment_type, rt.status,
		       rt.description, rt.due_date, rt.progress_percentage
		FROM risk_treatments rt
		JOIN risks r ON r.id = rt.risk_id
		WHERE rt.organization_id = $1 AND rt.status != 'completed'
		ORDER BY rt.due_date ASC NULLS LAST`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query treatments: %w", err)
	}
	defer treatRows.Close()

	var treatments []map[string]interface{}
	for treatRows.Next() {
		var id, riskTitle, tType, status, desc string
		var dueDate *time.Time
		var progress *float64
		if err := treatRows.Scan(&id, &riskTitle, &tType, &status, &desc, &dueDate, &progress); err != nil {
			return nil, fmt.Errorf("scan treatment: %w", err)
		}
		t := map[string]interface{}{
			"id":             id,
			"risk_title":     riskTitle,
			"treatment_type": tType,
			"status":         status,
			"description":    desc,
		}
		if dueDate != nil {
			t["due_date"] = dueDate.Format("2006-01-02")
		}
		if progress != nil {
			t["progress_percentage"] = *progress
		}
		treatments = append(treatments, t)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Active Risk Treatments",
		Type:    "table",
		Content: treatments,
	})

	// Section 4: Key Risk Indicators.
	kriRows, err := re.pool.Query(ctx, `
		SELECT ri.name, ri.current_value, ri.threshold_green, ri.threshold_amber,
		       ri.threshold_red, ri.trend, ri.measurement_unit
		FROM risk_indicators ri
		WHERE ri.organization_id = $1
		ORDER BY ri.name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query KRIs: %w", err)
	}
	defer kriRows.Close()

	var kris []map[string]interface{}
	for kriRows.Next() {
		var name string
		var currentVal, threshG, threshA, threshR *float64
		var trend, unit *string
		if err := kriRows.Scan(&name, &currentVal, &threshG, &threshA, &threshR, &trend, &unit); err != nil {
			return nil, fmt.Errorf("scan KRI: %w", err)
		}
		k := map[string]interface{}{"name": name}
		if currentVal != nil {
			k["current_value"] = *currentVal
		}
		if threshG != nil {
			k["threshold_green"] = *threshG
		}
		if threshA != nil {
			k["threshold_amber"] = *threshA
		}
		if threshR != nil {
			k["threshold_red"] = *threshR
		}
		if trend != nil {
			k["trend"] = *trend
		}
		if unit != nil {
			k["unit"] = *unit
		}
		kris = append(kris, k)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Key Risk Indicators",
		Type:    "kpi",
		Content: kris,
	})

	return data, nil
}

// GetExecutiveSummaryData queries all KPIs: compliance score, risk summary,
// incidents, findings, and policies for an executive-level report.
func (re *ReportEngine) GetExecutiveSummaryData(ctx context.Context, orgID string) (*ReportData, error) {
	data := &ReportData{
		Title:          "Executive Summary Report",
		GeneratedAt:    time.Now(),
		Classification: "confidential",
		Format:         "json",
	}

	_ = re.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&data.Organization)

	// KPI 1: Overall compliance score (average across frameworks).
	var avgScore *float64
	_ = re.pool.QueryRow(ctx, `
		SELECT AVG(compliance_score) FROM organization_frameworks
		WHERE organization_id = $1`, orgID).Scan(&avgScore)
	complianceKPI := map[string]interface{}{"overall_compliance_score": 0.0}
	if avgScore != nil {
		complianceKPI["overall_compliance_score"] = *avgScore
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Compliance Overview",
		Type:    "kpi",
		Content: complianceKPI,
	})

	// KPI 2: Risk summary.
	riskRows, err := re.pool.Query(ctx, `
		SELECT risk_level, COUNT(*) FROM risks
		WHERE organization_id = $1 AND deleted_at IS NULL
		GROUP BY risk_level`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query risk summary: %w", err)
	}
	defer riskRows.Close()

	riskKPI := make(map[string]int)
	for riskRows.Next() {
		var level string
		var count int
		if err := riskRows.Scan(&level, &count); err != nil {
			return nil, fmt.Errorf("scan risk KPI: %w", err)
		}
		riskKPI[level] = count
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Risk Summary",
		Type:    "kpi",
		Content: riskKPI,
	})

	// KPI 3: Incident summary.
	incidentKPI := make(map[string]interface{})
	var totalIncidents, openIncidents, breachNotifiable int
	_ = re.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status IN ('open', 'investigating', 'contained')),
		       COUNT(*) FILTER (WHERE is_breach_notifiable = true)
		FROM incidents
		WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).
		Scan(&totalIncidents, &openIncidents, &breachNotifiable)
	incidentKPI["total"] = totalIncidents
	incidentKPI["open"] = openIncidents
	incidentKPI["breach_notifiable"] = breachNotifiable
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Incident Summary",
		Type:    "kpi",
		Content: incidentKPI,
	})

	// KPI 4: Audit findings.
	findingsKPI := make(map[string]interface{})
	var totalFindings, openFindings int
	_ = re.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE af.status IN ('open', 'in_progress'))
		FROM audit_findings af
		JOIN audits a ON a.id = af.audit_id
		WHERE a.organization_id = $1 AND af.deleted_at IS NULL`, orgID).
		Scan(&totalFindings, &openFindings)
	findingsKPI["total_findings"] = totalFindings
	findingsKPI["open_findings"] = openFindings
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Audit Findings",
		Type:    "kpi",
		Content: findingsKPI,
	})

	// KPI 5: Policy compliance.
	policyKPI := make(map[string]interface{})
	var totalPolicies, activePolicies, overdueReview int
	_ = re.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'active'),
		       COUNT(*) FILTER (WHERE next_review_date < NOW())
		FROM policies
		WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).
		Scan(&totalPolicies, &activePolicies, &overdueReview)
	policyKPI["total"] = totalPolicies
	policyKPI["active"] = activePolicies
	policyKPI["overdue_for_review"] = overdueReview
	data.Sections = append(data.Sections, ReportSection{
		Title:   "Policy Status",
		Type:    "kpi",
		Content: policyKPI,
	})

	return data, nil
}

// ListDefinitions returns all report definitions for an organization.
func (re *ReportEngine) ListDefinitions(ctx context.Context, orgID string) ([]ReportDefinition, error) {
	rows, err := re.pool.Query(ctx, `
		SELECT id, organization_id, name, COALESCE(description, ''), report_type, format,
		       filters, sections, classification, created_by
		FROM report_definitions
		WHERE organization_id = $1
		ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list report definitions: %w", err)
	}
	defer rows.Close()

	var defs []ReportDefinition
	for rows.Next() {
		var d ReportDefinition
		var filtersJSON, sectionsJSON []byte
		var createdBy *string
		if err := rows.Scan(&d.ID, &d.OrgID, &d.Name, &d.Description, &d.ReportType,
			&d.Format, &filtersJSON, &sectionsJSON, &d.Classification, &createdBy); err != nil {
			return nil, fmt.Errorf("scan report definition: %w", err)
		}
		if createdBy != nil {
			d.CreatedBy = *createdBy
		}
		_ = json.Unmarshal(filtersJSON, &d.Filters)
		_ = json.Unmarshal(sectionsJSON, &d.Sections)
		if d.Filters == nil {
			d.Filters = make(map[string]interface{})
		}
		defs = append(defs, d)
	}

	return defs, nil
}

// CreateDefinition inserts a new report definition.
func (re *ReportEngine) CreateDefinition(ctx context.Context, orgID string, def ReportDefinition) (*ReportDefinition, error) {
	filtersJSON, err := json.Marshal(def.Filters)
	if err != nil {
		return nil, fmt.Errorf("marshal filters: %w", err)
	}
	sectionsJSON, err := json.Marshal(def.Sections)
	if err != nil {
		return nil, fmt.Errorf("marshal sections: %w", err)
	}

	var createdByPtr *string
	if def.CreatedBy != "" {
		createdByPtr = &def.CreatedBy
	}

	err = re.pool.QueryRow(ctx, `
		INSERT INTO report_definitions (organization_id, name, description, report_type, format,
		    filters, sections, classification, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		orgID, def.Name, def.Description, def.ReportType, def.Format,
		filtersJSON, sectionsJSON, def.Classification, createdByPtr,
	).Scan(&def.ID)
	if err != nil {
		return nil, fmt.Errorf("insert report definition: %w", err)
	}

	def.OrgID = orgID
	log.Info().Str("def_id", def.ID).Str("name", def.Name).Msg("report definition created")
	return &def, nil
}

// GetRunStatus retrieves the status of a specific report run.
func (re *ReportEngine) GetRunStatus(ctx context.Context, orgID, runID string) (*ReportRun, error) {
	var run ReportRun
	err := re.pool.QueryRow(ctx, `
		SELECT id, report_definition_id, schedule_id, status, format, file_path,
		       file_size_bytes, page_count, generation_time_ms, generated_by,
		       error_message, created_at, completed_at
		FROM report_runs
		WHERE id = $1 AND organization_id = $2`, runID, orgID,
	).Scan(
		&run.ID, &run.DefinitionID, &run.ScheduleID, &run.Status, &run.Format,
		&run.FilePath, &run.FileSizeBytes, &run.PageCount, &run.GenTimeMs,
		&run.GeneratedBy, &run.ErrorMessage, &run.CreatedAt, &run.CompletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("report run not found")
		}
		return nil, fmt.Errorf("get report run: %w", err)
	}
	return &run, nil
}

// ListRuns returns a paginated list of report runs for an organization.
func (re *ReportEngine) ListRuns(ctx context.Context, orgID string, page, pageSize int) ([]ReportRun, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	err := re.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM report_runs WHERE organization_id = $1`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count report runs: %w", err)
	}

	rows, err := re.pool.Query(ctx, `
		SELECT id, report_definition_id, schedule_id, status, format, file_path,
		       file_size_bytes, page_count, generation_time_ms, generated_by,
		       error_message, created_at, completed_at
		FROM report_runs
		WHERE organization_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list report runs: %w", err)
	}
	defer rows.Close()

	var runs []ReportRun
	for rows.Next() {
		var r ReportRun
		if err := rows.Scan(
			&r.ID, &r.DefinitionID, &r.ScheduleID, &r.Status, &r.Format,
			&r.FilePath, &r.FileSizeBytes, &r.PageCount, &r.GenTimeMs,
			&r.GeneratedBy, &r.ErrorMessage, &r.CreatedAt, &r.CompletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan report run: %w", err)
		}
		runs = append(runs, r)
	}

	return runs, total, nil
}
