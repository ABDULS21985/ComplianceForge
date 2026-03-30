package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/pkg/pdf"
	"github.com/complianceforge/platform/internal/pkg/xlsx"
)

// ReportEngineService generates compliance reports in PDF, XLSX, CSV, and JSON
// formats. It implements models.ReportEngine.
type ReportEngineService struct {
	pool *pgxpool.Pool
	pdf  *pdf.ReportPDFGenerator
	xlsx *xlsx.ExcelizeGenerator

	// In-memory file cache keyed by run ID. In production this would be backed
	// by object storage (S3/GCS). Entries are evicted after fileRetention.
	fileMu    sync.RWMutex
	fileCache map[string]cachedFile
}

type cachedFile struct {
	Data        []byte
	ContentType string
	FileName    string
	CreatedAt   time.Time
}

// Verify interface satisfaction at compile time.
var _ models.ReportEngine = (*ReportEngineService)(nil)

// NewReportEngineService creates a new report engine with PDF and XLSX renderers.
func NewReportEngineService(pool *pgxpool.Pool, companyName, logoPath, classification string) *ReportEngineService {
	return &ReportEngineService{
		pool:      pool,
		pdf:       pdf.NewReportPDFGenerator(companyName, logoPath, classification),
		xlsx:      xlsx.NewExcelizeGenerator(companyName, classification),
		fileCache: make(map[string]cachedFile),
	}
}

// ---------------------------------------------------------------------------
// Report Generation
// ---------------------------------------------------------------------------

// GenerateReport handles ad-hoc report generation. It gathers data, renders to
// the requested format (pdf, xlsx, csv, json), and stores the result.
func (re *ReportEngineService) GenerateReport(ctx context.Context, orgID, userID string, req *models.GenerateReportRequest) (*models.ReportRun, error) {
	startTime := time.Now()
	format := req.Format
	if format == "" {
		format = "pdf"
	}

	// Create the report_runs record in 'generating' status.
	var runID string
	var createdAt time.Time
	err := re.pool.QueryRow(ctx, `
		INSERT INTO report_runs (organization_id, report_definition_id, status, format, generated_by, parameters)
		VALUES ($1, NULL, 'generating', $2, $3, $4)
		RETURNING id, created_at`,
		orgID, format, userID, "{}",
	).Scan(&runID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("create report run: %w", err)
	}

	run := &models.ReportRun{
		ID:             runID,
		OrganizationID: orgID,
		ReportType:     req.ReportType,
		Title:          req.Title,
		Format:         format,
		Status:         "generating",
		CreatedBy:      userID,
		CreatedAt:      createdAt.Format(time.RFC3339),
	}

	// Gather data based on report type.
	reportData, gatherErr := re.gatherData(ctx, orgID, req.ReportType, req.Parameters)
	if gatherErr != nil {
		re.failRun(ctx, orgID, runID, run, gatherErr)
		return run, gatherErr
	}

	if req.Title != "" {
		reportData.Title = req.Title
	}

	// Render to the requested format.
	fileBytes, contentType, ext, renderErr := re.renderReport(reportData, req.ReportType, format)
	if renderErr != nil {
		re.failRun(ctx, orgID, runID, run, renderErr)
		return run, renderErr
	}

	// Store the file.
	fileName := fmt.Sprintf("%s_%s.%s", req.ReportType, time.Now().Format("20060102_150405"), ext)
	re.storeFile(runID, fileBytes, contentType, fileName)

	// Update run record.
	elapsed := int(time.Since(startTime).Milliseconds())
	fileSize := int64(len(fileBytes))
	filePath := fmt.Sprintf("reports/%s/%s.%s", orgID, runID, ext)
	now := time.Now()
	_, _ = re.pool.Exec(ctx, `
		UPDATE report_runs
		SET status = 'completed', file_path = $1, file_size_bytes = $2,
		    generation_time_ms = $3, completed_at = $4
		WHERE id = $5 AND organization_id = $6`,
		filePath, fileSize, elapsed, now, runID, orgID)

	run.Status = "completed"
	run.FileURL = fmt.Sprintf("/api/v1/reports/download/%s", runID)
	run.CompletedAt = now.Format(time.RFC3339)

	log.Info().
		Str("run_id", runID).
		Str("report_type", req.ReportType).
		Str("format", format).
		Int("generation_time_ms", elapsed).
		Int64("file_size", fileSize).
		Msg("report generated successfully")

	return run, nil
}

// GetRunStatus retrieves the current status of a report run.
func (re *ReportEngineService) GetRunStatus(ctx context.Context, orgID, runID string) (*models.ReportRun, error) {
	var run models.ReportRun
	var createdAt time.Time
	var completedAt *time.Time
	var fileURL, errMsg *string
	err := re.pool.QueryRow(ctx, `
		SELECT id, organization_id, COALESCE(report_definition_id::text, ''), status, format,
		       file_path, error_message, COALESCE(generated_by::text, ''), created_at, completed_at
		FROM report_runs
		WHERE id = $1 AND organization_id = $2`, runID, orgID,
	).Scan(&run.ID, &run.OrganizationID, &run.DefinitionID, &run.Status, &run.Format,
		&fileURL, &errMsg, &run.CreatedBy, &createdAt, &completedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("report run not found")
		}
		return nil, fmt.Errorf("get report run: %w", err)
	}
	run.CreatedAt = createdAt.Format(time.RFC3339)
	if completedAt != nil {
		run.CompletedAt = completedAt.Format(time.RFC3339)
	}
	if fileURL != nil {
		run.FileURL = fmt.Sprintf("/api/v1/reports/download/%s", runID)
	}
	if errMsg != nil {
		run.Error = *errMsg
	}
	return &run, nil
}

// DownloadReport returns the generated report file for download.
func (re *ReportEngineService) DownloadReport(ctx context.Context, orgID, runID string) (*models.ReportFile, error) {
	// Verify the run belongs to this org and is completed.
	var status string
	err := re.pool.QueryRow(ctx, `
		SELECT status FROM report_runs
		WHERE id = $1 AND organization_id = $2`, runID, orgID,
	).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("report run not found")
		}
		return nil, fmt.Errorf("check report run: %w", err)
	}
	if status != "completed" {
		return nil, fmt.Errorf("report is not yet completed (status: %s)", status)
	}

	re.fileMu.RLock()
	f, ok := re.fileCache[runID]
	re.fileMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("report file not found in cache (may have expired)")
	}

	return &models.ReportFile{
		FileName:    f.FileName,
		ContentType: f.ContentType,
		Data:        f.Data,
	}, nil
}

// ---------------------------------------------------------------------------
// Definitions CRUD
// ---------------------------------------------------------------------------

func (re *ReportEngineService) ListDefinitions(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.ReportDefinition, int, error) {
	page, pageSize := normalizePagination(pagination.Page, pagination.PageSize)
	offset := (page - 1) * pageSize

	var total int
	if err := re.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM report_definitions WHERE organization_id = $1`, orgID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count definitions: %w", err)
	}

	rows, err := re.pool.Query(ctx, `
		SELECT id, organization_id, name, report_type, COALESCE(format, 'pdf'),
		       COALESCE(created_by::text, ''), created_at, updated_at
		FROM report_definitions
		WHERE organization_id = $1
		ORDER BY name
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list definitions: %w", err)
	}
	defer rows.Close()

	var defs []models.ReportDefinition
	for rows.Next() {
		var d models.ReportDefinition
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.Name, &d.ReportType, &d.Format,
			&d.CreatedBy, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan definition: %w", err)
		}
		d.CreatedAt = createdAt.Format(time.RFC3339)
		d.UpdatedAt = updatedAt.Format(time.RFC3339)
		defs = append(defs, d)
	}
	return defs, total, nil
}

func (re *ReportEngineService) CreateDefinition(ctx context.Context, orgID, userID string, def *models.ReportDefinition) error {
	params, _ := json.Marshal(def.Parameters)
	if def.Format == "" {
		def.Format = "pdf"
	}
	var id string
	var createdAt time.Time
	err := re.pool.QueryRow(ctx, `
		INSERT INTO report_definitions (organization_id, name, report_type, format, filters, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		orgID, def.Name, def.ReportType, def.Format, params, userID,
	).Scan(&id, &createdAt)
	if err != nil {
		return fmt.Errorf("create definition: %w", err)
	}
	def.ID = id
	def.OrganizationID = orgID
	def.CreatedBy = userID
	def.CreatedAt = createdAt.Format(time.RFC3339)
	return nil
}

func (re *ReportEngineService) GetDefinition(ctx context.Context, orgID, defID string) (*models.ReportDefinition, error) {
	var d models.ReportDefinition
	var paramsJSON []byte
	var createdAt, updatedAt time.Time
	err := re.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, report_type, COALESCE(format, 'pdf'),
		       filters, COALESCE(created_by::text, ''), created_at, updated_at
		FROM report_definitions
		WHERE id = $1 AND organization_id = $2`, defID, orgID,
	).Scan(&d.ID, &d.OrganizationID, &d.Name, &d.ReportType, &d.Format,
		&paramsJSON, &d.CreatedBy, &createdAt, &updatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("definition not found")
		}
		return nil, fmt.Errorf("get definition: %w", err)
	}
	_ = json.Unmarshal(paramsJSON, &d.Parameters)
	d.CreatedAt = createdAt.Format(time.RFC3339)
	d.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &d, nil
}

func (re *ReportEngineService) UpdateDefinition(ctx context.Context, orgID string, def *models.ReportDefinition) error {
	params, _ := json.Marshal(def.Parameters)
	result, err := re.pool.Exec(ctx, `
		UPDATE report_definitions
		SET name = $1, report_type = $2, format = $3, filters = $4, updated_at = NOW()
		WHERE id = $5 AND organization_id = $6`,
		def.Name, def.ReportType, def.Format, params, def.ID, orgID)
	if err != nil {
		return fmt.Errorf("update definition: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("definition not found")
	}
	return nil
}

func (re *ReportEngineService) DeleteDefinition(ctx context.Context, orgID, defID string) error {
	result, err := re.pool.Exec(ctx, `
		DELETE FROM report_definitions WHERE id = $1 AND organization_id = $2`,
		defID, orgID)
	if err != nil {
		return fmt.Errorf("delete definition: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("definition not found")
	}
	return nil
}

func (re *ReportEngineService) GenerateFromDefinition(ctx context.Context, orgID, userID, defID string) (*models.ReportRun, error) {
	def, err := re.GetDefinition(ctx, orgID, defID)
	if err != nil {
		return nil, err
	}
	return re.GenerateReport(ctx, orgID, userID, &models.GenerateReportRequest{
		ReportType: def.ReportType,
		Title:      def.Name,
		Format:     def.Format,
		Parameters: def.Parameters,
	})
}

// ---------------------------------------------------------------------------
// Schedules CRUD
// ---------------------------------------------------------------------------

func (re *ReportEngineService) ListSchedules(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.ReportSchedule, int, error) {
	page, pageSize := normalizePagination(pagination.Page, pagination.PageSize)
	offset := (page - 1) * pageSize

	var total int
	if err := re.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM report_schedules WHERE organization_id = $1`, orgID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count schedules: %w", err)
	}

	rows, err := re.pool.Query(ctx, `
		SELECT id, organization_id, report_definition_id,
		       COALESCE(frequency::text, ''), is_active,
		       COALESCE(created_by::text, ''), created_at, updated_at, next_run_at
		FROM report_schedules
		WHERE organization_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()

	var schedules []models.ReportSchedule
	for rows.Next() {
		var s models.ReportSchedule
		var createdAt, updatedAt time.Time
		var nextRunAt *time.Time
		if err := rows.Scan(&s.ID, &s.OrganizationID, &s.DefinitionID,
			&s.CronExpr, &s.Enabled,
			&s.CreatedBy, &createdAt, &updatedAt, &nextRunAt); err != nil {
			return nil, 0, fmt.Errorf("scan schedule: %w", err)
		}
		s.CreatedAt = createdAt.Format(time.RFC3339)
		s.UpdatedAt = updatedAt.Format(time.RFC3339)
		if nextRunAt != nil {
			s.NextRunAt = nextRunAt.Format(time.RFC3339)
		}
		schedules = append(schedules, s)
	}
	return schedules, total, nil
}

func (re *ReportEngineService) CreateSchedule(ctx context.Context, orgID, userID string, sched *models.ReportSchedule) error {
	recipients, _ := json.Marshal(sched.Recipients)
	var id string
	var createdAt time.Time
	err := re.pool.QueryRow(ctx, `
		INSERT INTO report_schedules (organization_id, report_definition_id, frequency,
		    is_active, recipient_user_ids, created_by, next_run_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW() + INTERVAL '1 hour')
		RETURNING id, created_at`,
		orgID, sched.DefinitionID, sched.CronExpr, sched.Enabled, recipients, userID,
	).Scan(&id, &createdAt)
	if err != nil {
		return fmt.Errorf("create schedule: %w", err)
	}
	sched.ID = id
	sched.OrganizationID = orgID
	sched.CreatedBy = userID
	sched.CreatedAt = createdAt.Format(time.RFC3339)
	return nil
}

func (re *ReportEngineService) UpdateSchedule(ctx context.Context, orgID string, sched *models.ReportSchedule) error {
	recipients, _ := json.Marshal(sched.Recipients)
	result, err := re.pool.Exec(ctx, `
		UPDATE report_schedules
		SET report_definition_id = $1, frequency = $2, is_active = $3,
		    recipient_user_ids = $4, updated_at = NOW()
		WHERE id = $5 AND organization_id = $6`,
		sched.DefinitionID, sched.CronExpr, sched.Enabled, recipients, sched.ID, orgID)
	if err != nil {
		return fmt.Errorf("update schedule: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

func (re *ReportEngineService) DeleteSchedule(ctx context.Context, orgID, schedID string) error {
	result, err := re.pool.Exec(ctx, `
		DELETE FROM report_schedules WHERE id = $1 AND organization_id = $2`,
		schedID, orgID)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

// ---------------------------------------------------------------------------
// History
// ---------------------------------------------------------------------------

func (re *ReportEngineService) ListHistory(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.ReportRun, int, error) {
	page, pageSize := normalizePagination(pagination.Page, pagination.PageSize)
	offset := (page - 1) * pageSize

	var total int
	if err := re.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM report_runs WHERE organization_id = $1`, orgID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count runs: %w", err)
	}

	rows, err := re.pool.Query(ctx, `
		SELECT id, organization_id, COALESCE(report_definition_id::text, ''),
		       status, format, file_path, error_message,
		       COALESCE(generated_by::text, ''), created_at, completed_at
		FROM report_runs
		WHERE organization_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var runs []models.ReportRun
	for rows.Next() {
		var r models.ReportRun
		var fileURL, errMsg *string
		var createdAt time.Time
		var completedAt *time.Time
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.DefinitionID,
			&r.Status, &r.Format, &fileURL, &errMsg,
			&r.CreatedBy, &createdAt, &completedAt); err != nil {
			return nil, 0, fmt.Errorf("scan run: %w", err)
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		if completedAt != nil {
			r.CompletedAt = completedAt.Format(time.RFC3339)
		}
		if fileURL != nil {
			r.FileURL = fmt.Sprintf("/api/v1/reports/download/%s", r.ID)
		}
		if errMsg != nil {
			r.Error = *errMsg
		}
		runs = append(runs, r)
	}
	return runs, total, nil
}

// ---------------------------------------------------------------------------
// Internal: Data Gathering
// ---------------------------------------------------------------------------

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

func (re *ReportEngineService) gatherData(ctx context.Context, orgID, reportType string, params map[string]interface{}) (*ReportData, error) {
	switch reportType {
	case "compliance_status", "gap_analysis", "cross_framework_mapping":
		return re.getComplianceReportData(ctx, orgID, params)
	case "risk_register", "risk_heatmap", "kri_dashboard", "treatment_progress":
		return re.getRiskReportData(ctx, orgID, params)
	case "audit_summary", "audit_findings":
		return re.getAuditReportData(ctx, orgID, params)
	case "executive_summary":
		return re.getExecutiveSummaryData(ctx, orgID)
	default:
		return re.getComplianceReportData(ctx, orgID, params)
	}
}

// ---------------------------------------------------------------------------
// Internal: Format Rendering
// ---------------------------------------------------------------------------

// renderReport converts gathered ReportData into the requested output format.
func (re *ReportEngineService) renderReport(data *ReportData, reportType, format string) ([]byte, string, string, error) {
	renderData := re.buildRenderData(data)

	switch format {
	case "pdf":
		return re.renderPDF(renderData, reportType)
	case "xlsx":
		return re.renderXLSX(renderData, reportType)
	case "csv":
		// XLSX generator output is also suitable; for true CSV the caller
		// can use JSON export. Re-use XLSX for structured output.
		return re.renderXLSX(renderData, reportType)
	case "json":
		return re.renderJSON(data)
	default:
		return re.renderPDF(renderData, reportType)
	}
}

func (re *ReportEngineService) renderPDF(data map[string]interface{}, reportType string) ([]byte, string, string, error) {
	var bytes []byte
	var err error

	switch reportType {
	case "risk_register", "risk_heatmap", "kri_dashboard", "treatment_progress":
		bytes, err = re.pdf.GenerateRiskReport(data)
	case "audit_summary", "audit_findings":
		bytes, err = re.pdf.GenerateAuditReport(data)
	default:
		bytes, err = re.pdf.GenerateComplianceReport(data)
	}

	if err != nil {
		return nil, "", "", fmt.Errorf("render PDF: %w", err)
	}
	return bytes, "application/pdf", "pdf", nil
}

func (re *ReportEngineService) renderXLSX(data map[string]interface{}, reportType string) ([]byte, string, string, error) {
	var bytes []byte
	var err error

	switch reportType {
	case "risk_register", "risk_heatmap", "kri_dashboard", "treatment_progress":
		bytes, err = re.xlsx.GenerateRiskReport(data)
	case "audit_summary", "audit_findings":
		bytes, err = re.xlsx.GenerateAuditReport(data)
	default:
		bytes, err = re.xlsx.GenerateComplianceReport(data)
	}

	if err != nil {
		return nil, "", "", fmt.Errorf("render XLSX: %w", err)
	}
	return bytes, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx", nil
}

func (re *ReportEngineService) renderJSON(data *ReportData) ([]byte, string, string, error) {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, "", "", fmt.Errorf("render JSON: %w", err)
	}
	return bytes, "application/json", "json", nil
}

// buildRenderData converts the internal ReportData into the flat
// map[string]interface{} format that the PDF and XLSX generators expect.
func (re *ReportEngineService) buildRenderData(data *ReportData) map[string]interface{} {
	result := map[string]interface{}{
		"title":          data.Title,
		"organization":   data.Organization,
		"generated_at":   data.GeneratedAt,
		"classification": data.Classification,
	}

	for _, section := range data.Sections {
		switch section.Title {
		case "Framework Compliance Scores":
			result["framework_scores"] = section.Content
			if scores, ok := section.Content.([]map[string]interface{}); ok {
				result["frameworks_count"] = len(scores)
				totalControls := 0
				var totalScore float64
				for _, s := range scores {
					if tc, ok := s["total_controls"].(int); ok {
						totalControls += tc
					}
					if sc, ok := s["compliance_score"].(float64); ok {
						totalScore += sc
					}
				}
				result["total_controls"] = totalControls
				if len(scores) > 0 {
					result["overall_score"] = totalScore / float64(len(scores))
				}
			}
		case "Gap Analysis":
			result["gaps"] = section.Content
		case "Risk Register":
			result["top_risks"] = section.Content
			if risks, ok := section.Content.([]map[string]interface{}); ok {
				result["total_risks"] = len(risks)
				critical := 0
				for _, r := range risks {
					if l, ok := r["level"].(string); ok && l == "critical" {
						critical++
					}
				}
				result["critical_count"] = critical
			}
		case "Risk Summary by Level":
			result["risk_summary"] = section.Content
		case "Active Risk Treatments":
			result["treatment_summary"] = section.Content
			if treatments, ok := section.Content.([]map[string]interface{}); ok {
				completed := 0
				total := len(treatments)
				for _, t := range treatments {
					if s, ok := t["status"].(string); ok && s == "completed" {
						completed++
					}
				}
				if total > 0 {
					result["treatment_completion_rate"] = float64(completed) / float64(total) * 100
				} else {
					result["treatment_completion_rate"] = 0.0
				}
			}
		case "Key Risk Indicators":
			result["kris"] = section.Content
		case "Audit Findings":
			if items, ok := section.Content.([]map[string]interface{}); ok {
				result["findings"] = items
				result["total_findings"] = len(items)
				critical, high, open := 0, 0, 0
				for _, item := range items {
					if s, ok := item["severity"].(string); ok {
						if s == "critical" {
							critical++
						}
						if s == "high" {
							high++
						}
					}
					if s, ok := item["status"].(string); ok && (s == "open" || s == "in_progress") {
						open++
					}
				}
				result["critical_findings"] = critical
				result["high_findings"] = high
				result["open_findings"] = open
			} else {
				// KPI-style findings from executive summary
				result["findings_kpi"] = section.Content
			}
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Internal: File Storage & Helpers
// ---------------------------------------------------------------------------

func (re *ReportEngineService) storeFile(runID string, data []byte, contentType, fileName string) {
	re.fileMu.Lock()
	defer re.fileMu.Unlock()
	re.fileCache[runID] = cachedFile{
		Data:        data,
		ContentType: contentType,
		FileName:    fileName,
		CreatedAt:   time.Now(),
	}
}

func (re *ReportEngineService) failRun(ctx context.Context, orgID, runID string, run *models.ReportRun, err error) {
	errMsg := err.Error()
	now := time.Now()
	_, _ = re.pool.Exec(ctx, `
		UPDATE report_runs SET status = 'failed', error_message = $1, completed_at = $2
		WHERE id = $3 AND organization_id = $4`,
		errMsg, now, runID, orgID)
	run.Status = "failed"
	run.Error = errMsg
	run.CompletedAt = now.Format(time.RFC3339)
}

func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

// ---------------------------------------------------------------------------
// Internal: Data Queries
// ---------------------------------------------------------------------------

func (re *ReportEngineService) getComplianceReportData(ctx context.Context, orgID string, filters map[string]interface{}) (*ReportData, error) {
	data := &ReportData{
		Title:          "Compliance Status Report",
		GeneratedAt:    time.Now(),
		Classification: "internal",
	}
	_ = re.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&data.Organization)

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
			"framework_name":        name,
			"framework_version":     "",
			"compliance_score":      score,
			"status":                status,
			"total_controls":        total,
			"implemented_count":     effective + implemented,
			"partial_count":         partial,
			"not_implemented_count": notImpl,
			"not_applicable_count":  notApplicable,
			"avg_maturity_level":    0.0,
		})
	}
	data.Sections = append(data.Sections, ReportSection{
		Title: "Framework Compliance Scores", Type: "table", Content: frameworkScores,
	})

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
			"control_code":          code,
			"control_title":         title,
			"framework_name":        "",
			"status":                "gap",
			"risk_if_not_implemented": "high",
			"owner_name":            "",
		}
		if gapDesc != nil {
			g["gap_description"] = *gapDesc
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
		Title: "Gap Analysis", Type: "table", Content: gaps,
	})

	return data, nil
}

func (re *ReportEngineService) getRiskReportData(ctx context.Context, orgID string, filters map[string]interface{}) (*ReportData, error) {
	data := &ReportData{
		Title:          "Risk Assessment Report",
		GeneratedAt:    time.Now(),
		Classification: "internal",
	}
	_ = re.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&data.Organization)

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
		ref := id
		if len(ref) > 8 {
			ref = ref[:8]
		}
		risks = append(risks, map[string]interface{}{
			"risk_ref":             ref,
			"title":                title,
			"category_name":        category,
			"risk_source":          "",
			"inherent_risk_score":  score,
			"residual_risk_score":  score,
			"residual_risk_level":  level,
			"financial_impact_eur": 0,
			"status":               mitStatus,
			"owner_name":           "",
			"level":                level,
		})
	}
	data.Sections = append(data.Sections, ReportSection{
		Title: "Risk Register", Type: "table", Content: risks,
	})

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
		Title: "Risk Summary by Level", Type: "kpi", Content: riskSummary,
	})

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
			"id": id, "risk_title": riskTitle, "treatment_type": tType,
			"status": status, "description": desc,
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
		Title: "Active Risk Treatments", Type: "table", Content: treatments,
	})

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
		Title: "Key Risk Indicators", Type: "kpi", Content: kris,
	})

	return data, nil
}

func (re *ReportEngineService) getAuditReportData(ctx context.Context, orgID string, filters map[string]interface{}) (*ReportData, error) {
	data := &ReportData{
		Title:          "Audit Findings Report",
		GeneratedAt:    time.Now(),
		Classification: "internal",
	}
	_ = re.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&data.Organization)

	rows, err := re.pool.Query(ctx, `
		SELECT af.id, af.title, af.severity, af.status, af.finding_type,
		       af.due_date, a.title AS audit_title
		FROM audit_findings af
		JOIN audits a ON a.id = af.audit_id
		WHERE a.organization_id = $1 AND af.deleted_at IS NULL
		ORDER BY
		  CASE af.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1
		       WHEN 'medium' THEN 2 ELSE 3 END,
		  af.due_date ASC NULLS LAST`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query findings: %w", err)
	}
	defer rows.Close()

	var findings []map[string]interface{}
	for rows.Next() {
		var id, title, severity, status, findingType, auditTitle string
		var dueDate *time.Time
		if err := rows.Scan(&id, &title, &severity, &status, &findingType, &dueDate, &auditTitle); err != nil {
			return nil, fmt.Errorf("scan finding: %w", err)
		}
		ref := id
		if len(ref) > 8 {
			ref = ref[:8]
		}
		f := map[string]interface{}{
			"finding_ref":      ref,
			"title":            title,
			"audit_title":      auditTitle,
			"severity":         severity,
			"status":           status,
			"finding_type":     findingType,
			"responsible_name": "",
			"root_cause":       "",
		}
		if dueDate != nil {
			f["due_date"] = dueDate.Format("2006-01-02")
		}
		findings = append(findings, f)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title: "Audit Findings", Type: "table", Content: findings,
	})

	return data, nil
}

func (re *ReportEngineService) getExecutiveSummaryData(ctx context.Context, orgID string) (*ReportData, error) {
	data := &ReportData{
		Title:          "Executive Summary Report",
		GeneratedAt:    time.Now(),
		Classification: "confidential",
	}
	_ = re.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&data.Organization)

	var avgScore *float64
	_ = re.pool.QueryRow(ctx, `
		SELECT AVG(compliance_score) FROM organization_frameworks
		WHERE organization_id = $1`, orgID).Scan(&avgScore)
	complianceKPI := map[string]interface{}{"overall_compliance_score": 0.0}
	if avgScore != nil {
		complianceKPI["overall_compliance_score"] = *avgScore
	}
	data.Sections = append(data.Sections, ReportSection{
		Title: "Compliance Overview", Type: "kpi", Content: complianceKPI,
	})

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
		Title: "Risk Summary", Type: "kpi", Content: riskKPI,
	})

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
		Title: "Incident Summary", Type: "kpi", Content: incidentKPI,
	})

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
		Title: "Audit Findings", Type: "kpi", Content: findingsKPI,
	})

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
		Title: "Policy Status", Type: "kpi", Content: policyKPI,
	})

	return data, nil
}
