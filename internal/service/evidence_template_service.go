package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// EvidenceTemplate defines the evidence expected for a given control.
type EvidenceTemplate struct {
	ID                string                 `json:"id"`
	OrgID             string                 `json:"organization_id"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	ControlCode       string                 `json:"control_code"`
	FrameworkCode     string                 `json:"framework_code"`
	EvidenceType      string                 `json:"evidence_type"` // document, screenshot, log, report, certificate, attestation
	CollectionFreq    string                 `json:"collection_frequency"` // daily, weekly, monthly, quarterly, annually, on_demand
	RetentionDays     int                    `json:"retention_days"`
	ValidationRules   []ValidationRule       `json:"validation_rules"`
	RequiredFields    map[string]interface{} `json:"required_fields"`
	IsSystem          bool                   `json:"is_system"`
	CreatedAt         string                 `json:"created_at"`
}

// ValidationRule defines a single validation check applied to evidence.
type ValidationRule struct {
	RuleType  string `json:"rule_type"`  // file_not_empty, date_within, contains_text, file_type, file_size
	Parameter string `json:"parameter"`  // e.g., "90" for days, "pdf,docx" for types, "1048576" for bytes
	Message   string `json:"message"`
}

// EvidenceRequirement is a specific evidence need generated from a template.
type EvidenceRequirement struct {
	ID                  string  `json:"id"`
	OrgID               string  `json:"organization_id"`
	TemplateID          string  `json:"template_id"`
	ControlImplID       string  `json:"control_implementation_id"`
	FrameworkID         string  `json:"framework_id"`
	Title               string  `json:"title"`
	Description         string  `json:"description"`
	EvidenceType        string  `json:"evidence_type"`
	Status              string  `json:"status"` // pending, collected, validated, failed, expired
	CollectionFreq      string  `json:"collection_frequency"`
	DueDate             *string `json:"due_date"`
	LastCollectedAt     *string `json:"last_collected_at"`
	NextCollectionDate  *string `json:"next_collection_date"`
	AssignedTo          *string `json:"assigned_to"`
	CreatedAt           string  `json:"created_at"`
}

// EvidenceValidationResult holds the result of validating a piece of evidence.
type EvidenceValidationResult struct {
	EvidenceID    string             `json:"evidence_id"`
	RequirementID string            `json:"requirement_id"`
	IsValid       bool              `json:"is_valid"`
	RuleResults   []RuleResult      `json:"rule_results"`
	ValidatedAt   string            `json:"validated_at"`
}

// RuleResult is the outcome of a single validation rule.
type RuleResult struct {
	RuleType string `json:"rule_type"`
	Passed   bool   `json:"passed"`
	Message  string `json:"message"`
}

// EvidenceGap represents a control with missing, expired, or failed evidence.
type EvidenceGap struct {
	FrameworkCode string `json:"framework_code"`
	FrameworkName string `json:"framework_name"`
	ControlCode   string `json:"control_code"`
	ControlTitle  string `json:"control_title"`
	GapType       string `json:"gap_type"` // missing, expired, failed
	RequirementID string `json:"requirement_id"`
	DueDate       string `json:"due_date"`
}

// CollectionTask represents an upcoming evidence collection task.
type CollectionTask struct {
	RequirementID   string  `json:"requirement_id"`
	Title           string  `json:"title"`
	ControlCode     string  `json:"control_code"`
	EvidenceType    string  `json:"evidence_type"`
	DueDate         string  `json:"due_date"`
	AssignedTo      *string `json:"assigned_to"`
	DaysUntilDue    int     `json:"days_until_due"`
	Priority        string  `json:"priority"`
}

// TestSuite groups related evidence test cases.
type TestSuite struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"organization_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	TestCases   []TestCase `json:"test_cases"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   string     `json:"created_at"`
}

// TestCase is a single evidence validation test.
type TestCase struct {
	ID            string `json:"id"`
	SuiteID       string `json:"suite_id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	RequirementID string `json:"requirement_id"`
	TestType      string `json:"test_type"` // existence, completeness, accuracy, timeliness
	ExpectedResult string `json:"expected_result"`
	SortOrder     int    `json:"sort_order"`
}

// TestSuiteRun represents a completed test suite execution.
type TestSuiteRun struct {
	ID           string           `json:"id"`
	SuiteID      string           `json:"suite_id"`
	Status       string           `json:"status"` // running, passed, failed, error
	TotalTests   int              `json:"total_tests"`
	Passed       int              `json:"passed"`
	Failed       int              `json:"failed"`
	Errors       int              `json:"errors"`
	Results      []TestCaseResult `json:"results"`
	TriggeredBy  string           `json:"triggered_by"`
	StartedAt    string           `json:"started_at"`
	CompletedAt  *string          `json:"completed_at"`
}

// TestCaseResult is the outcome of a single test case execution.
type TestCaseResult struct {
	TestCaseID string  `json:"test_case_id"`
	TestName   string  `json:"test_name"`
	Status     string  `json:"status"` // passed, failed, error
	Details    string  `json:"details"`
	Duration   float64 `json:"duration_ms"`
}

// PreAuditCheck is a comprehensive pre-audit verification result.
type PreAuditCheck struct {
	FrameworkID      string            `json:"framework_id"`
	FrameworkName    string            `json:"framework_name"`
	OverallStatus    string            `json:"overall_status"` // ready, needs_attention, not_ready
	TotalControls    int               `json:"total_controls"`
	ControlsWithEvidence int           `json:"controls_with_evidence"`
	ControlsMissing  int               `json:"controls_missing_evidence"`
	ExpiredEvidence  int               `json:"expired_evidence"`
	FailedValidation int               `json:"failed_validation"`
	ReadinessScore   float64           `json:"readiness_score"`
	Gaps             []EvidenceGap     `json:"gaps"`
	Recommendations  []string          `json:"recommendations"`
}

// CreateEvidenceTemplateRequest holds input for creating a template.
type CreateEvidenceTemplateRequest struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	ControlCode      string                 `json:"control_code"`
	FrameworkCode    string                 `json:"framework_code"`
	EvidenceType     string                 `json:"evidence_type"`
	CollectionFreq   string                 `json:"collection_frequency"`
	RetentionDays    int                    `json:"retention_days"`
	ValidationRules  []ValidationRule       `json:"validation_rules"`
	RequiredFields   map[string]interface{} `json:"required_fields"`
}

// CreateTestSuiteRequest holds input for creating a test suite.
type CreateTestSuiteRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	TestCases   []TestCase `json:"test_cases"`
	CreatedBy   string     `json:"created_by"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// EvidenceTemplateService manages evidence templates, requirements, and validation.
type EvidenceTemplateService struct {
	pool *pgxpool.Pool
}

// NewEvidenceTemplateService creates a new EvidenceTemplateService.
func NewEvidenceTemplateService(pool *pgxpool.Pool) *EvidenceTemplateService {
	return &EvidenceTemplateService{pool: pool}
}

// GetTemplatesForControl returns all evidence templates matching a control code and framework.
func (s *EvidenceTemplateService) GetTemplatesForControl(ctx context.Context, controlCode, frameworkCode string) ([]EvidenceTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, description, control_code, framework_code,
			evidence_type, collection_frequency, retention_days,
			validation_rules, required_fields, is_system, created_at
		FROM evidence_templates
		WHERE control_code = $1 AND framework_code = $2
		ORDER BY name`, controlCode, frameworkCode)
	if err != nil {
		return nil, fmt.Errorf("query templates for control: %w", err)
	}
	defer rows.Close()
	return s.scanTemplates(rows)
}

// GetTemplatesForFramework returns all templates for a given framework within an org.
func (s *EvidenceTemplateService) GetTemplatesForFramework(ctx context.Context, orgID, frameworkCode string) ([]EvidenceTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, description, control_code, framework_code,
			evidence_type, collection_frequency, retention_days,
			validation_rules, required_fields, is_system, created_at
		FROM evidence_templates
		WHERE (organization_id = $1 OR is_system = true) AND framework_code = $2
		ORDER BY control_code, name`, orgID, frameworkCode)
	if err != nil {
		return nil, fmt.Errorf("query templates for framework: %w", err)
	}
	defer rows.Close()
	return s.scanTemplates(rows)
}

// GenerateEvidenceRequirements creates evidence requirements from templates for all
// control implementations linked to a framework.
func (s *EvidenceTemplateService) GenerateEvidenceRequirements(ctx context.Context, orgID, frameworkID string) (int, error) {
	result, err := s.pool.Exec(ctx, `
		INSERT INTO evidence_requirements (
			organization_id, template_id, control_implementation_id, framework_id,
			title, description, evidence_type, status, collection_frequency,
			due_date, next_collection_date
		)
		SELECT
			ci.organization_id, et.id, ci.id, $2,
			et.name, et.description, et.evidence_type, 'pending', et.collection_frequency,
			CURRENT_DATE + (CASE et.collection_frequency
				WHEN 'daily' THEN INTERVAL '1 day'
				WHEN 'weekly' THEN INTERVAL '7 days'
				WHEN 'monthly' THEN INTERVAL '30 days'
				WHEN 'quarterly' THEN INTERVAL '90 days'
				WHEN 'annually' THEN INTERVAL '365 days'
				ELSE INTERVAL '30 days'
			END),
			CURRENT_DATE + (CASE et.collection_frequency
				WHEN 'daily' THEN INTERVAL '1 day'
				WHEN 'weekly' THEN INTERVAL '7 days'
				WHEN 'monthly' THEN INTERVAL '30 days'
				WHEN 'quarterly' THEN INTERVAL '90 days'
				WHEN 'annually' THEN INTERVAL '365 days'
				ELSE INTERVAL '30 days'
			END)
		FROM control_implementations ci
		JOIN framework_controls fc ON ci.control_id = fc.id
		JOIN evidence_templates et ON et.control_code = fc.control_code AND et.framework_code = fc.framework_code
		WHERE ci.organization_id = $1 AND fc.framework_id = $2
			AND NOT EXISTS (
				SELECT 1 FROM evidence_requirements er
				WHERE er.control_implementation_id = ci.id AND er.template_id = et.id
			)`,
		orgID, frameworkID)
	if err != nil {
		return 0, fmt.Errorf("generate evidence requirements: %w", err)
	}

	count := int(result.RowsAffected())
	log.Info().
		Str("org_id", orgID).
		Str("framework_id", frameworkID).
		Int("generated", count).
		Msg("evidence requirements generated")
	return count, nil
}

// ValidateEvidence runs validation rules against a piece of evidence.
func (s *EvidenceTemplateService) ValidateEvidence(ctx context.Context, orgID, requirementID, evidenceID string) (*EvidenceValidationResult, error) {
	// Get the requirement's template validation rules.
	var rulesJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT et.validation_rules
		FROM evidence_requirements er
		JOIN evidence_templates et ON er.template_id = et.id
		WHERE er.id = $1 AND er.organization_id = $2`,
		requirementID, orgID).Scan(&rulesJSON)
	if err != nil {
		return nil, fmt.Errorf("get validation rules: %w", err)
	}

	var rules []ValidationRule
	if err := json.Unmarshal(rulesJSON, &rules); err != nil {
		return nil, fmt.Errorf("parse validation rules: %w", err)
	}

	// Get evidence metadata.
	var fileName string
	var fileSizeBytes int64
	var fileType string
	var contentText *string
	var collectedAt time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT file_name, file_size_bytes, file_type, content_text, collected_at
		FROM evidence_items
		WHERE id = $1 AND organization_id = $2`,
		evidenceID, orgID).Scan(&fileName, &fileSizeBytes, &fileType, &contentText, &collectedAt)
	if err != nil {
		return nil, fmt.Errorf("get evidence: %w", err)
	}

	result := &EvidenceValidationResult{
		EvidenceID:    evidenceID,
		RequirementID: requirementID,
		IsValid:       true,
		ValidatedAt:   time.Now().Format(time.RFC3339),
	}

	for _, rule := range rules {
		rr := RuleResult{RuleType: rule.RuleType, Passed: true}

		switch rule.RuleType {
		case "file_not_empty":
			if fileSizeBytes == 0 {
				rr.Passed = false
				rr.Message = "File is empty"
			} else {
				rr.Message = fmt.Sprintf("File size: %d bytes", fileSizeBytes)
			}

		case "date_within":
			days := 90
			fmt.Sscanf(rule.Parameter, "%d", &days)
			cutoff := time.Now().AddDate(0, 0, -days)
			if collectedAt.Before(cutoff) {
				rr.Passed = false
				rr.Message = fmt.Sprintf("Evidence collected %s, older than %d days", collectedAt.Format("2006-01-02"), days)
			} else {
				rr.Message = fmt.Sprintf("Evidence collected %s, within %d day window", collectedAt.Format("2006-01-02"), days)
			}

		case "contains_text":
			if contentText == nil || !strings.Contains(*contentText, rule.Parameter) {
				rr.Passed = false
				rr.Message = fmt.Sprintf("Required text '%s' not found", rule.Parameter)
			} else {
				rr.Message = fmt.Sprintf("Required text '%s' found", rule.Parameter)
			}

		case "file_type":
			allowedTypes := strings.Split(rule.Parameter, ",")
			found := false
			for _, t := range allowedTypes {
				if strings.TrimSpace(t) == fileType {
					found = true
					break
				}
			}
			if !found {
				rr.Passed = false
				rr.Message = fmt.Sprintf("File type '%s' not in allowed types: %s", fileType, rule.Parameter)
			} else {
				rr.Message = fmt.Sprintf("File type '%s' is allowed", fileType)
			}

		case "file_size":
			var maxBytes int64
			fmt.Sscanf(rule.Parameter, "%d", &maxBytes)
			if fileSizeBytes > maxBytes {
				rr.Passed = false
				rr.Message = fmt.Sprintf("File size %d exceeds maximum %d bytes", fileSizeBytes, maxBytes)
			} else {
				rr.Message = fmt.Sprintf("File size %d within limit", fileSizeBytes)
			}
		}

		if !rr.Passed {
			result.IsValid = false
		}
		result.RuleResults = append(result.RuleResults, rr)
	}

	// Update requirement status based on validation.
	newStatus := "validated"
	if !result.IsValid {
		newStatus = "failed"
	}
	_, _ = s.pool.Exec(ctx, `
		UPDATE evidence_requirements SET status = $1, last_collected_at = NOW(), updated_at = NOW()
		WHERE id = $2`, newStatus, requirementID)

	// Store validation result.
	resultJSON, _ := json.Marshal(result.RuleResults)
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO evidence_validation_results (
			organization_id, evidence_id, requirement_id, is_valid, rule_results, validated_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		orgID, evidenceID, requirementID, result.IsValid, resultJSON)

	log.Info().
		Str("evidence_id", evidenceID).
		Str("requirement_id", requirementID).
		Bool("valid", result.IsValid).
		Msg("evidence validated")

	return result, nil
}

// GetEvidenceGaps returns controls missing, expired, or failed evidence grouped by framework.
func (s *EvidenceTemplateService) GetEvidenceGaps(ctx context.Context, orgID string) ([]EvidenceGap, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			f.code AS framework_code,
			f.name AS framework_name,
			fc.control_code,
			fc.title AS control_title,
			CASE
				WHEN er.id IS NULL THEN 'missing'
				WHEN er.status = 'failed' THEN 'failed'
				WHEN er.status = 'expired' OR (er.due_date IS NOT NULL AND er.due_date < CURRENT_DATE) THEN 'expired'
				ELSE 'missing'
			END AS gap_type,
			COALESCE(er.id, ''),
			COALESCE(TO_CHAR(er.due_date, 'YYYY-MM-DD'), '')
		FROM control_implementations ci
		JOIN framework_controls fc ON ci.control_id = fc.id
		JOIN frameworks f ON fc.framework_id = f.id
		LEFT JOIN evidence_requirements er ON er.control_implementation_id = ci.id
			AND er.status IN ('pending', 'failed', 'expired')
		WHERE ci.organization_id = $1
			AND (er.id IS NULL OR er.status IN ('pending', 'failed', 'expired'))
		ORDER BY f.code, fc.control_code`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query evidence gaps: %w", err)
	}
	defer rows.Close()

	var gaps []EvidenceGap
	for rows.Next() {
		var g EvidenceGap
		if err := rows.Scan(&g.FrameworkCode, &g.FrameworkName, &g.ControlCode, &g.ControlTitle,
			&g.GapType, &g.RequirementID, &g.DueDate); err != nil {
			return nil, fmt.Errorf("scan gap: %w", err)
		}
		gaps = append(gaps, g)
	}
	return gaps, nil
}

// GetCollectionSchedule returns upcoming evidence collection tasks.
func (s *EvidenceTemplateService) GetCollectionSchedule(ctx context.Context, orgID string) ([]CollectionTask, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			er.id, er.title,
			fc.control_code, er.evidence_type,
			COALESCE(TO_CHAR(er.next_collection_date, 'YYYY-MM-DD'), TO_CHAR(er.due_date, 'YYYY-MM-DD')),
			er.assigned_to,
			COALESCE(er.next_collection_date, er.due_date)::DATE - CURRENT_DATE AS days_until_due,
			CASE
				WHEN COALESCE(er.next_collection_date, er.due_date) < CURRENT_DATE THEN 'overdue'
				WHEN COALESCE(er.next_collection_date, er.due_date) <= CURRENT_DATE + INTERVAL '7 days' THEN 'urgent'
				WHEN COALESCE(er.next_collection_date, er.due_date) <= CURRENT_DATE + INTERVAL '30 days' THEN 'upcoming'
				ELSE 'scheduled'
			END AS priority
		FROM evidence_requirements er
		JOIN control_implementations ci ON er.control_implementation_id = ci.id
		JOIN framework_controls fc ON ci.control_id = fc.id
		WHERE er.organization_id = $1 AND er.status IN ('pending', 'failed', 'expired')
		ORDER BY COALESCE(er.next_collection_date, er.due_date) ASC
		LIMIT 100`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query collection schedule: %w", err)
	}
	defer rows.Close()

	var tasks []CollectionTask
	for rows.Next() {
		var t CollectionTask
		if err := rows.Scan(&t.RequirementID, &t.Title, &t.ControlCode, &t.EvidenceType,
			&t.DueDate, &t.AssignedTo, &t.DaysUntilDue, &t.Priority); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// RunTestSuite executes all test cases in a suite.
func (s *EvidenceTemplateService) RunTestSuite(ctx context.Context, orgID, suiteID, triggeredBy string) (*TestSuiteRun, error) {
	// Get test cases.
	rows, err := s.pool.Query(ctx, `
		SELECT id, suite_id, name, description, requirement_id, test_type, expected_result, sort_order
		FROM evidence_test_cases
		WHERE suite_id = $1
		ORDER BY sort_order`, suiteID)
	if err != nil {
		return nil, fmt.Errorf("query test cases: %w", err)
	}
	defer rows.Close()

	var cases []TestCase
	for rows.Next() {
		var tc TestCase
		if err := rows.Scan(&tc.ID, &tc.SuiteID, &tc.Name, &tc.Description,
			&tc.RequirementID, &tc.TestType, &tc.ExpectedResult, &tc.SortOrder); err != nil {
			return nil, fmt.Errorf("scan test case: %w", err)
		}
		cases = append(cases, tc)
	}

	// Create run record.
	var runID string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO evidence_test_runs (organization_id, suite_id, status, total_tests, triggered_by, started_at)
		VALUES ($1, $2, 'running', $3, $4, NOW())
		RETURNING id`,
		orgID, suiteID, len(cases), triggeredBy).Scan(&runID)
	if err != nil {
		return nil, fmt.Errorf("create test run: %w", err)
	}

	run := &TestSuiteRun{
		ID:          runID,
		SuiteID:     suiteID,
		Status:      "running",
		TotalTests:  len(cases),
		TriggeredBy: triggeredBy,
		StartedAt:   time.Now().Format(time.RFC3339),
	}

	// Execute each test case.
	for _, tc := range cases {
		startTime := time.Now()
		tcr := TestCaseResult{TestCaseID: tc.ID, TestName: tc.Name, Status: "passed"}

		switch tc.TestType {
		case "existence":
			var count int
			err := s.pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM evidence_items ei
				JOIN evidence_requirements er ON ei.requirement_id = er.id
				WHERE er.id = $1 AND er.organization_id = $2`,
				tc.RequirementID, orgID).Scan(&count)
			if err != nil || count == 0 {
				tcr.Status = "failed"
				tcr.Details = "No evidence found for requirement"
			} else {
				tcr.Details = fmt.Sprintf("Found %d evidence items", count)
			}

		case "completeness":
			var status string
			err := s.pool.QueryRow(ctx, `
				SELECT status FROM evidence_requirements WHERE id = $1 AND organization_id = $2`,
				tc.RequirementID, orgID).Scan(&status)
			if err != nil {
				tcr.Status = "error"
				tcr.Details = "Requirement not found"
			} else if status != "validated" {
				tcr.Status = "failed"
				tcr.Details = fmt.Sprintf("Requirement status is '%s', expected 'validated'", status)
			} else {
				tcr.Details = "Requirement fully validated"
			}

		case "accuracy":
			var validCount int
			err := s.pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM evidence_validation_results
				WHERE requirement_id = $1 AND organization_id = $2 AND is_valid = true`,
				tc.RequirementID, orgID).Scan(&validCount)
			if err != nil || validCount == 0 {
				tcr.Status = "failed"
				tcr.Details = "No valid validation results found"
			} else {
				tcr.Details = fmt.Sprintf("Found %d valid validation results", validCount)
			}

		case "timeliness":
			var daysOld int
			err := s.pool.QueryRow(ctx, `
				SELECT COALESCE(CURRENT_DATE - er.last_collected_at::DATE, 999)
				FROM evidence_requirements er
				WHERE er.id = $1 AND er.organization_id = $2`,
				tc.RequirementID, orgID).Scan(&daysOld)
			if err != nil || daysOld > 90 {
				tcr.Status = "failed"
				tcr.Details = fmt.Sprintf("Evidence is %d days old", daysOld)
			} else {
				tcr.Details = fmt.Sprintf("Evidence is %d days old, within limits", daysOld)
			}
		}

		tcr.Duration = float64(time.Since(startTime).Milliseconds())

		switch tcr.Status {
		case "passed":
			run.Passed++
		case "failed":
			run.Failed++
		case "error":
			run.Errors++
		}
		run.Results = append(run.Results, tcr)

		// Store result.
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO evidence_test_results (organization_id, run_id, test_case_id, status, details, duration_ms)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			orgID, runID, tc.ID, tcr.Status, tcr.Details, tcr.Duration)
	}

	// Update run status.
	run.Status = "passed"
	if run.Failed > 0 {
		run.Status = "failed"
	}
	if run.Errors > 0 {
		run.Status = "error"
	}
	completedAt := time.Now().Format(time.RFC3339)
	run.CompletedAt = &completedAt

	_, _ = s.pool.Exec(ctx, `
		UPDATE evidence_test_runs
		SET status = $1, passed = $2, failed = $3, errors = $4, completed_at = NOW()
		WHERE id = $5`, run.Status, run.Passed, run.Failed, run.Errors, runID)

	log.Info().
		Str("suite_id", suiteID).
		Str("run_id", runID).
		Str("status", run.Status).
		Int("total", run.TotalTests).
		Int("passed", run.Passed).
		Int("failed", run.Failed).
		Msg("test suite completed")

	return run, nil
}

// RunPreAuditChecks performs comprehensive pre-audit verification for a framework.
func (s *EvidenceTemplateService) RunPreAuditChecks(ctx context.Context, orgID, frameworkID string) (*PreAuditCheck, error) {
	check := &PreAuditCheck{FrameworkID: frameworkID}

	// Get framework name.
	_ = s.pool.QueryRow(ctx, `SELECT name FROM frameworks WHERE id = $1`, frameworkID).Scan(&check.FrameworkName)

	// Total controls.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT ci.id)
		FROM control_implementations ci
		JOIN framework_controls fc ON ci.control_id = fc.id
		WHERE ci.organization_id = $1 AND fc.framework_id = $2`,
		orgID, frameworkID).Scan(&check.TotalControls)

	// Controls with validated evidence.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT er.control_implementation_id)
		FROM evidence_requirements er
		JOIN control_implementations ci ON er.control_implementation_id = ci.id
		JOIN framework_controls fc ON ci.control_id = fc.id
		WHERE er.organization_id = $1 AND fc.framework_id = $2 AND er.status = 'validated'`,
		orgID, frameworkID).Scan(&check.ControlsWithEvidence)

	check.ControlsMissing = check.TotalControls - check.ControlsWithEvidence

	// Expired evidence.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM evidence_requirements er
		JOIN control_implementations ci ON er.control_implementation_id = ci.id
		JOIN framework_controls fc ON ci.control_id = fc.id
		WHERE er.organization_id = $1 AND fc.framework_id = $2
			AND (er.status = 'expired' OR (er.due_date IS NOT NULL AND er.due_date < CURRENT_DATE))`,
		orgID, frameworkID).Scan(&check.ExpiredEvidence)

	// Failed validation.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM evidence_requirements er
		JOIN control_implementations ci ON er.control_implementation_id = ci.id
		JOIN framework_controls fc ON ci.control_id = fc.id
		WHERE er.organization_id = $1 AND fc.framework_id = $2 AND er.status = 'failed'`,
		orgID, frameworkID).Scan(&check.FailedValidation)

	// Readiness score.
	if check.TotalControls > 0 {
		check.ReadinessScore = float64(check.ControlsWithEvidence) * 100.0 / float64(check.TotalControls)
	}

	// Determine overall status.
	switch {
	case check.ReadinessScore >= 95 && check.ExpiredEvidence == 0 && check.FailedValidation == 0:
		check.OverallStatus = "ready"
	case check.ReadinessScore >= 70:
		check.OverallStatus = "needs_attention"
	default:
		check.OverallStatus = "not_ready"
	}

	// Gaps.
	gaps, err := s.GetEvidenceGaps(ctx, orgID)
	if err == nil {
		check.Gaps = gaps
	}

	// Recommendations.
	if check.ControlsMissing > 0 {
		check.Recommendations = append(check.Recommendations,
			fmt.Sprintf("Collect evidence for %d controls missing documentation", check.ControlsMissing))
	}
	if check.ExpiredEvidence > 0 {
		check.Recommendations = append(check.Recommendations,
			fmt.Sprintf("Renew %d expired evidence items", check.ExpiredEvidence))
	}
	if check.FailedValidation > 0 {
		check.Recommendations = append(check.Recommendations,
			fmt.Sprintf("Remediate %d evidence items that failed validation", check.FailedValidation))
	}

	log.Info().
		Str("framework_id", frameworkID).
		Str("status", check.OverallStatus).
		Float64("readiness", check.ReadinessScore).
		Msg("pre-audit check completed")

	return check, nil
}

// ListTemplates returns paginated evidence templates for an organization.
func (s *EvidenceTemplateService) ListTemplates(ctx context.Context, orgID string, page, pageSize int) ([]EvidenceTemplate, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence_templates WHERE organization_id = $1 OR is_system = true`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count templates: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, description, control_code, framework_code,
			evidence_type, collection_frequency, retention_days,
			validation_rules, required_fields, is_system, created_at
		FROM evidence_templates
		WHERE organization_id = $1 OR is_system = true
		ORDER BY framework_code, control_code
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	templates, err := s.scanTemplates(rows)
	if err != nil {
		return nil, 0, err
	}
	return templates, total, nil
}

// CreateTemplate creates a new evidence template.
func (s *EvidenceTemplateService) CreateTemplate(ctx context.Context, orgID string, req CreateEvidenceTemplateRequest) (*EvidenceTemplate, error) {
	rulesJSON, _ := json.Marshal(req.ValidationRules)
	fieldsJSON, _ := json.Marshal(req.RequiredFields)
	if fieldsJSON == nil {
		fieldsJSON = []byte("{}")
	}

	var t EvidenceTemplate
	var vr, rf []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO evidence_templates (
			organization_id, name, description, control_code, framework_code,
			evidence_type, collection_frequency, retention_days,
			validation_rules, required_fields
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, organization_id, name, description, control_code, framework_code,
			evidence_type, collection_frequency, retention_days,
			validation_rules, required_fields, is_system, created_at`,
		orgID, req.Name, req.Description, req.ControlCode, req.FrameworkCode,
		req.EvidenceType, req.CollectionFreq, req.RetentionDays, rulesJSON, fieldsJSON,
	).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Description, &t.ControlCode, &t.FrameworkCode,
		&t.EvidenceType, &t.CollectionFreq, &t.RetentionDays,
		&vr, &rf, &t.IsSystem, &t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}
	_ = json.Unmarshal(vr, &t.ValidationRules)
	_ = json.Unmarshal(rf, &t.RequiredFields)

	log.Info().Str("template_id", t.ID).Str("name", t.Name).Msg("evidence template created")
	return &t, nil
}

// ListRequirements returns paginated evidence requirements for an organization.
func (s *EvidenceTemplateService) ListRequirements(ctx context.Context, orgID string, page, pageSize int) ([]EvidenceRequirement, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence_requirements WHERE organization_id = $1`, orgID).Scan(&total)

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, template_id, control_implementation_id, framework_id,
			title, description, evidence_type, status, collection_frequency,
			due_date, last_collected_at, next_collection_date, assigned_to, created_at
		FROM evidence_requirements
		WHERE organization_id = $1
		ORDER BY due_date ASC NULLS LAST
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list requirements: %w", err)
	}
	defer rows.Close()

	var reqs []EvidenceRequirement
	for rows.Next() {
		var r EvidenceRequirement
		if err := rows.Scan(&r.ID, &r.OrgID, &r.TemplateID, &r.ControlImplID, &r.FrameworkID,
			&r.Title, &r.Description, &r.EvidenceType, &r.Status, &r.CollectionFreq,
			&r.DueDate, &r.LastCollectedAt, &r.NextCollectionDate, &r.AssignedTo, &r.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan requirement: %w", err)
		}
		reqs = append(reqs, r)
	}
	return reqs, total, nil
}

// ListTestSuites returns all test suites for an organization.
func (s *EvidenceTemplateService) ListTestSuites(ctx context.Context, orgID string) ([]TestSuite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, description, status, created_by, created_at
		FROM evidence_test_suites
		WHERE organization_id = $1
		ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list test suites: %w", err)
	}
	defer rows.Close()

	var suites []TestSuite
	for rows.Next() {
		var ts TestSuite
		if err := rows.Scan(&ts.ID, &ts.OrgID, &ts.Name, &ts.Description,
			&ts.Status, &ts.CreatedBy, &ts.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan suite: %w", err)
		}
		suites = append(suites, ts)
	}
	return suites, nil
}

// CreateTestSuite creates a new test suite with its test cases.
func (s *EvidenceTemplateService) CreateTestSuite(ctx context.Context, orgID string, req CreateTestSuiteRequest) (*TestSuite, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var suite TestSuite
	err = tx.QueryRow(ctx, `
		INSERT INTO evidence_test_suites (organization_id, name, description, status, created_by)
		VALUES ($1, $2, $3, 'active', $4)
		RETURNING id, organization_id, name, description, status, created_by, created_at`,
		orgID, req.Name, req.Description, req.CreatedBy,
	).Scan(&suite.ID, &suite.OrgID, &suite.Name, &suite.Description,
		&suite.Status, &suite.CreatedBy, &suite.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create suite: %w", err)
	}

	for i, tc := range req.TestCases {
		var caseID string
		err = tx.QueryRow(ctx, `
			INSERT INTO evidence_test_cases (
				suite_id, name, description, requirement_id, test_type, expected_result, sort_order
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			suite.ID, tc.Name, tc.Description, tc.RequirementID,
			tc.TestType, tc.ExpectedResult, i+1,
		).Scan(&caseID)
		if err != nil {
			return nil, fmt.Errorf("create test case: %w", err)
		}
		tc.ID = caseID
		tc.SuiteID = suite.ID
		tc.SortOrder = i + 1
		suite.TestCases = append(suite.TestCases, tc)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	log.Info().Str("suite_id", suite.ID).Str("name", suite.Name).Msg("test suite created")
	return &suite, nil
}

// scanTemplates scans rows into EvidenceTemplate slices.
func (s *EvidenceTemplateService) scanTemplates(rows pgx.Rows) ([]EvidenceTemplate, error) {
	var result []EvidenceTemplate
	for rows.Next() {
		var t EvidenceTemplate
		var vr, rf []byte
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Description, &t.ControlCode, &t.FrameworkCode,
			&t.EvidenceType, &t.CollectionFreq, &t.RetentionDays,
			&vr, &rf, &t.IsSystem, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		_ = json.Unmarshal(vr, &t.ValidationRules)
		_ = json.Unmarshal(rf, &t.RequiredFields)
		result = append(result, t)
	}
	return result, nil
}
