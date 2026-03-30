package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interface ----------

// EvidenceTemplateService defines the methods required by EvidenceTemplateHandler.
type EvidenceTemplateService interface {
	// Templates
	ListTemplates(ctx context.Context, orgID string, pagination models.PaginationRequest, filters EvidenceTemplateFilters) ([]EvidenceTemplate, int, error)
	GetTemplate(ctx context.Context, orgID, templateID string) (*EvidenceTemplate, error)
	CreateTemplate(ctx context.Context, orgID, userID string, template *EvidenceTemplate) error

	// Requirements
	ListRequirements(ctx context.Context, orgID string, pagination models.PaginationRequest, filters EvidenceRequirementFilters) ([]EvidenceRequirement, int, error)
	GenerateRequirements(ctx context.Context, orgID, userID string, req *GenerateRequirementsRequest) ([]EvidenceRequirement, error)
	UpdateRequirement(ctx context.Context, orgID string, requirement *EvidenceRequirement) error
	ValidateRequirement(ctx context.Context, orgID, requirementID string, req *ValidateRequirementRequest) (*ValidationResult, error)

	// Gaps
	GetEvidenceGaps(ctx context.Context, orgID string, filters EvidenceGapFilters) ([]EvidenceGap, error)

	// Schedule
	GetEvidenceSchedule(ctx context.Context, orgID string, filters EvidenceScheduleFilters) ([]EvidenceScheduleItem, error)

	// Test suites
	ListTestSuites(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]EvidenceTestSuite, int, error)
	CreateTestSuite(ctx context.Context, orgID, userID string, suite *EvidenceTestSuite) error
	RunTestSuite(ctx context.Context, orgID, userID, suiteID string) (*TestSuiteRun, error)
	GetTestSuiteResults(ctx context.Context, orgID, suiteID string, pagination models.PaginationRequest) ([]TestSuiteRun, int, error)

	// Pre-audit check
	RunPreAuditCheck(ctx context.Context, orgID, userID string, req *PreAuditCheckRequest) (*PreAuditCheck, error)
	GetPreAuditReport(ctx context.Context, orgID, checkID string) (*PreAuditReport, error)
}

// ---------- request / response types ----------

// EvidenceTemplateFilters holds filter parameters for listing evidence templates.
type EvidenceTemplateFilters struct {
	FrameworkID string `json:"framework_id"`
	Category    string `json:"category"`
	Search      string `json:"search"`
}

// EvidenceTemplate represents an evidence collection template.
type EvidenceTemplate struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	Name           string                 `json:"name" validate:"required"`
	Description    string                 `json:"description"`
	Category       string                 `json:"category"`
	FrameworkID    string                 `json:"framework_id,omitempty"`
	ControlID      string                 `json:"control_id,omitempty"`
	Fields         []EvidenceTemplateField `json:"fields,omitempty"`
	Instructions   string                 `json:"instructions,omitempty"`
	Frequency      string                 `json:"frequency,omitempty"` // daily, weekly, monthly, quarterly, annual, on_demand
	IsActive       bool                   `json:"is_active"`
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// EvidenceTemplateField represents a field within an evidence template.
type EvidenceTemplateField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // text, file, date, url, checkbox, select
	Required    bool     `json:"required"`
	Description string   `json:"description,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// EvidenceRequirementFilters holds filter parameters for listing evidence requirements.
type EvidenceRequirementFilters struct {
	FrameworkID string `json:"framework_id"`
	ControlID   string `json:"control_id"`
	Status      string `json:"status"`
	Search      string `json:"search"`
}

// EvidenceRequirement represents an evidence requirement.
type EvidenceRequirement struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	ControlID      string `json:"control_id"`
	FrameworkID    string `json:"framework_id,omitempty"`
	Title          string `json:"title" validate:"required"`
	Description    string `json:"description"`
	Type           string `json:"type"`   // document, screenshot, log, report, configuration, attestation
	Status         string `json:"status"` // pending, collected, validated, expired, rejected
	Frequency      string `json:"frequency,omitempty"`
	LastCollected  string `json:"last_collected,omitempty"`
	NextDue        string `json:"next_due,omitempty"`
	TemplateID     string `json:"template_id,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// GenerateRequirementsRequest is the payload for POST /evidence/requirements/generate.
type GenerateRequirementsRequest struct {
	FrameworkID string   `json:"framework_id" validate:"required"`
	ControlIDs  []string `json:"control_ids,omitempty"`
}

// ValidateRequirementRequest is the payload for POST /evidence/requirements/{id}/validate.
type ValidateRequirementRequest struct {
	EvidenceURL string `json:"evidence_url,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// ValidationResult is the result of evidence validation.
type ValidationResult struct {
	RequirementID string   `json:"requirement_id"`
	Valid         bool     `json:"valid"`
	Score         float64  `json:"score"`
	Issues        []string `json:"issues,omitempty"`
	Suggestions   []string `json:"suggestions,omitempty"`
	ValidatedAt   string   `json:"validated_at"`
}

// EvidenceGapFilters holds filter parameters for evidence gap analysis.
type EvidenceGapFilters struct {
	FrameworkID string `json:"framework_id"`
	Severity    string `json:"severity"`
}

// EvidenceGap represents a gap in evidence collection.
type EvidenceGap struct {
	ControlID      string `json:"control_id"`
	ControlName    string `json:"control_name"`
	FrameworkID    string `json:"framework_id"`
	FrameworkName  string `json:"framework_name"`
	RequiredCount  int    `json:"required_count"`
	CollectedCount int    `json:"collected_count"`
	GapCount       int    `json:"gap_count"`
	Severity       string `json:"severity"`
	MissingTypes   []string `json:"missing_types,omitempty"`
}

// EvidenceScheduleFilters holds filter parameters for evidence schedule.
type EvidenceScheduleFilters struct {
	Month       string `json:"month"`
	FrameworkID string `json:"framework_id"`
}

// EvidenceScheduleItem represents a scheduled evidence collection item.
type EvidenceScheduleItem struct {
	RequirementID   string `json:"requirement_id"`
	RequirementTitle string `json:"requirement_title"`
	ControlID       string `json:"control_id"`
	ControlName     string `json:"control_name"`
	DueDate         string `json:"due_date"`
	Frequency       string `json:"frequency"`
	Status          string `json:"status"`
	AssigneeID      string `json:"assignee_id,omitempty"`
}

// EvidenceTestSuite represents a test suite for evidence validation.
type EvidenceTestSuite struct {
	ID             string              `json:"id"`
	OrganizationID string              `json:"organization_id"`
	Name           string              `json:"name" validate:"required"`
	Description    string              `json:"description"`
	FrameworkID    string              `json:"framework_id,omitempty"`
	TestCases      []EvidenceTestCase  `json:"test_cases,omitempty"`
	IsActive       bool                `json:"is_active"`
	CreatedBy      string              `json:"created_by"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

// EvidenceTestCase represents an individual test case within a suite.
type EvidenceTestCase struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	RequirementID string `json:"requirement_id,omitempty"`
	ExpectedResult string `json:"expected_result"`
	TestType      string `json:"test_type"` // existence, completeness, freshness, accuracy
}

// TestSuiteRun represents a single execution of a test suite.
type TestSuiteRun struct {
	ID          string           `json:"id"`
	SuiteID     string           `json:"suite_id"`
	Status      string           `json:"status"` // running, passed, failed, error
	TotalTests  int              `json:"total_tests"`
	PassedTests int              `json:"passed_tests"`
	FailedTests int              `json:"failed_tests"`
	Results     []TestCaseResult `json:"results,omitempty"`
	StartedAt   string           `json:"started_at"`
	CompletedAt string           `json:"completed_at,omitempty"`
	RunBy       string           `json:"run_by"`
}

// TestCaseResult represents the result of a single test case execution.
type TestCaseResult struct {
	TestCaseID string `json:"test_case_id"`
	Status     string `json:"status"` // passed, failed, error, skipped
	Message    string `json:"message,omitempty"`
	Duration   int    `json:"duration_ms"`
}

// PreAuditCheckRequest is the payload for POST /evidence/pre-audit-check.
type PreAuditCheckRequest struct {
	FrameworkID string   `json:"framework_id" validate:"required"`
	ControlIDs  []string `json:"control_ids,omitempty"`
	AuditDate   string   `json:"audit_date,omitempty"`
}

// PreAuditCheck represents a pre-audit readiness check.
type PreAuditCheck struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	FrameworkID    string `json:"framework_id"`
	Status         string `json:"status"` // running, completed, error
	StartedAt      string `json:"started_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
	RunBy          string `json:"run_by"`
}

// PreAuditReport represents the report from a pre-audit check.
type PreAuditReport struct {
	CheckID             string              `json:"check_id"`
	FrameworkID         string              `json:"framework_id"`
	FrameworkName       string              `json:"framework_name"`
	OverallReadiness    float64             `json:"overall_readiness_pct"`
	TotalControls       int                 `json:"total_controls"`
	ReadyControls       int                 `json:"ready_controls"`
	PartialControls     int                 `json:"partial_controls"`
	NotReadyControls    int                 `json:"not_ready_controls"`
	Findings            []PreAuditFinding   `json:"findings,omitempty"`
	Recommendations     []string            `json:"recommendations,omitempty"`
	GeneratedAt         string              `json:"generated_at"`
}

// PreAuditFinding represents a finding from a pre-audit check.
type PreAuditFinding struct {
	ControlID    string `json:"control_id"`
	ControlName  string `json:"control_name"`
	Severity     string `json:"severity"` // critical, high, medium, low
	Finding      string `json:"finding"`
	Remediation  string `json:"remediation,omitempty"`
}

// ---------- handler ----------

// EvidenceTemplateHandler handles evidence template and collection endpoints.
type EvidenceTemplateHandler struct {
	svc EvidenceTemplateService
}

// NewEvidenceTemplateHandler creates a new EvidenceTemplateHandler with the given service.
func NewEvidenceTemplateHandler(svc EvidenceTemplateService) *EvidenceTemplateHandler {
	return &EvidenceTemplateHandler{svc: svc}
}

// ListTemplates handles GET /evidence/templates.
func (h *EvidenceTemplateHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := EvidenceTemplateFilters{
		FrameworkID: r.URL.Query().Get("framework_id"),
		Category:    r.URL.Query().Get("category"),
		Search:      r.URL.Query().Get("search"),
	}

	templates, total, err := h.svc.ListTemplates(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list evidence templates", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": templates,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetTemplate handles GET /evidence/templates/{id}.
func (h *EvidenceTemplateHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	templateID := chi.URLParam(r, "id")
	if templateID == "" {
		writeError(w, http.StatusBadRequest, "Missing template ID", "")
		return
	}

	template, err := h.svc.GetTemplate(r.Context(), orgID, templateID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Evidence template not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, template)
}

// CreateTemplate handles POST /evidence/templates.
func (h *EvidenceTemplateHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var template EvidenceTemplate
	if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if template.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateTemplate(r.Context(), orgID, userID, &template); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create evidence template", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, template)
}

// ListRequirements handles GET /evidence/requirements.
func (h *EvidenceTemplateHandler) ListRequirements(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := EvidenceRequirementFilters{
		FrameworkID: r.URL.Query().Get("framework_id"),
		ControlID:   r.URL.Query().Get("control_id"),
		Status:      r.URL.Query().Get("status"),
		Search:      r.URL.Query().Get("search"),
	}

	requirements, total, err := h.svc.ListRequirements(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list evidence requirements", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": requirements,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GenerateRequirements handles POST /evidence/requirements/generate.
func (h *EvidenceTemplateHandler) GenerateRequirements(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req GenerateRequirementsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.FrameworkID == "" {
		writeError(w, http.StatusBadRequest, "framework_id is required", "")
		return
	}

	requirements, err := h.svc.GenerateRequirements(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate evidence requirements", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": requirements})
}

// UpdateRequirement handles PUT /evidence/requirements/{id}.
func (h *EvidenceTemplateHandler) UpdateRequirement(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	requirementID := chi.URLParam(r, "id")
	if requirementID == "" {
		writeError(w, http.StatusBadRequest, "Missing requirement ID", "")
		return
	}

	var requirement EvidenceRequirement
	if err := json.NewDecoder(r.Body).Decode(&requirement); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	requirement.ID = requirementID
	requirement.OrganizationID = orgID

	if err := h.svc.UpdateRequirement(r.Context(), orgID, &requirement); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update evidence requirement", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, requirement)
}

// ValidateRequirement handles POST /evidence/requirements/{id}/validate.
func (h *EvidenceTemplateHandler) ValidateRequirement(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	requirementID := chi.URLParam(r, "id")
	if requirementID == "" {
		writeError(w, http.StatusBadRequest, "Missing requirement ID", "")
		return
	}

	var req ValidateRequirementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	result, err := h.svc.ValidateRequirement(r.Context(), orgID, requirementID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to validate evidence requirement", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetEvidenceGaps handles GET /evidence/gaps.
func (h *EvidenceTemplateHandler) GetEvidenceGaps(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	filters := EvidenceGapFilters{
		FrameworkID: r.URL.Query().Get("framework_id"),
		Severity:    r.URL.Query().Get("severity"),
	}

	gaps, err := h.svc.GetEvidenceGaps(r.Context(), orgID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get evidence gaps", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": gaps})
}

// GetEvidenceSchedule handles GET /evidence/schedule.
func (h *EvidenceTemplateHandler) GetEvidenceSchedule(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	filters := EvidenceScheduleFilters{
		Month:       r.URL.Query().Get("month"),
		FrameworkID: r.URL.Query().Get("framework_id"),
	}

	schedule, err := h.svc.GetEvidenceSchedule(r.Context(), orgID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get evidence schedule", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": schedule})
}

// ListTestSuites handles GET /evidence/test-suites.
func (h *EvidenceTemplateHandler) ListTestSuites(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	suites, total, err := h.svc.ListTestSuites(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list evidence test suites", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": suites,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateTestSuite handles POST /evidence/test-suites.
func (h *EvidenceTemplateHandler) CreateTestSuite(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var suite EvidenceTestSuite
	if err := json.NewDecoder(r.Body).Decode(&suite); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if suite.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateTestSuite(r.Context(), orgID, userID, &suite); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create evidence test suite", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, suite)
}

// RunTestSuite handles POST /evidence/test-suites/{id}/run.
func (h *EvidenceTemplateHandler) RunTestSuite(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	suiteID := chi.URLParam(r, "id")
	if suiteID == "" {
		writeError(w, http.StatusBadRequest, "Missing test suite ID", "")
		return
	}

	run, err := h.svc.RunTestSuite(r.Context(), orgID, userID, suiteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to run test suite", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, run)
}

// GetTestSuiteResults handles GET /evidence/test-suites/{id}/results.
func (h *EvidenceTemplateHandler) GetTestSuiteResults(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	suiteID := chi.URLParam(r, "id")
	if suiteID == "" {
		writeError(w, http.StatusBadRequest, "Missing test suite ID", "")
		return
	}

	pagination := parsePagination(r)

	runs, total, err := h.svc.GetTestSuiteResults(r.Context(), orgID, suiteID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get test suite results", err.Error())
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

// RunPreAuditCheck handles POST /evidence/pre-audit-check.
func (h *EvidenceTemplateHandler) RunPreAuditCheck(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req PreAuditCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.FrameworkID == "" {
		writeError(w, http.StatusBadRequest, "framework_id is required", "")
		return
	}

	check, err := h.svc.RunPreAuditCheck(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to run pre-audit check", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, check)
}

// GetPreAuditReport handles GET /evidence/pre-audit-check/{id}/report.
func (h *EvidenceTemplateHandler) GetPreAuditReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	checkID := chi.URLParam(r, "id")
	if checkID == "" {
		writeError(w, http.StatusBadRequest, "Missing check ID", "")
		return
	}

	report, err := h.svc.GetPreAuditReport(r.Context(), orgID, checkID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Pre-audit report not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}
