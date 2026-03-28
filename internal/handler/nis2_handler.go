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

// NIS2Service defines the methods required by NIS2Handler.
type NIS2Service interface {
	GetAssessment(ctx context.Context, orgID string) (*NIS2Assessment, error)
	CreateAssessment(ctx context.Context, orgID, userID string, assessment *NIS2Assessment) error

	ListIncidentReports(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]NIS2IncidentReport, int, error)
	GetIncidentReport(ctx context.Context, orgID, reportID string) (*NIS2IncidentReport, error)
	SubmitEarlyWarning(ctx context.Context, orgID, userID, reportID string, warning *NIS2EarlyWarning) error
	SubmitNotification(ctx context.Context, orgID, userID, reportID string, notification *NIS2Notification) error
	SubmitFinalReport(ctx context.Context, orgID, userID, reportID string, finalReport *NIS2FinalReport) error

	GetMeasures(ctx context.Context, orgID string) ([]NIS2Measure, error)
	UpdateMeasure(ctx context.Context, orgID string, measure *NIS2Measure) error

	GetManagement(ctx context.Context, orgID string) (*NIS2ManagementOverview, error)
	RecordTraining(ctx context.Context, orgID, userID string, training *NIS2Training) error

	GetDashboard(ctx context.Context, orgID string) (*NIS2Dashboard, error)
}

// ---------- request / response types ----------

// NIS2Assessment represents the NIS2 entity classification and scope assessment.
type NIS2Assessment struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organization_id"`
	EntityType       string `json:"entity_type"`        // essential, important
	Sector           string `json:"sector"`              // energy, transport, banking, health, etc.
	SubSector        string `json:"sub_sector,omitempty"`
	EmployeeCount    int    `json:"employee_count"`
	AnnualTurnover   float64 `json:"annual_turnover"`
	InScope          bool   `json:"in_scope"`
	Justification    string `json:"justification,omitempty"`
	AssessedBy       string `json:"assessed_by"`
	AssessedAt       string `json:"assessed_at"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// NIS2IncidentReport represents an NIS2 incident report with its lifecycle stages.
type NIS2IncidentReport struct {
	ID                string              `json:"id"`
	OrganizationID    string              `json:"organization_id"`
	IncidentID        string              `json:"incident_id,omitempty"`
	Title             string              `json:"title"`
	Description       string              `json:"description"`
	Severity          string              `json:"severity"`
	AffectedServices  []string            `json:"affected_services,omitempty"`
	AffectedCountries []string            `json:"affected_countries,omitempty"`
	Status            string              `json:"status"` // draft, early_warning_sent, notification_sent, final_report_sent
	EarlyWarning      *NIS2EarlyWarning   `json:"early_warning,omitempty"`
	Notification      *NIS2Notification    `json:"notification,omitempty"`
	FinalReport       *NIS2FinalReport     `json:"final_report,omitempty"`
	CreatedBy         string              `json:"created_by"`
	CreatedAt         string              `json:"created_at"`
	UpdatedAt         string              `json:"updated_at"`
}

// NIS2EarlyWarning is the payload for the 24-hour early warning submission.
type NIS2EarlyWarning struct {
	SuspectedCause   string `json:"suspected_cause"`
	CrossBorderImpact bool  `json:"cross_border_impact"`
	InitialImpact    string `json:"initial_impact"`
	SubmittedAt      string `json:"submitted_at,omitempty"`
	SubmittedBy      string `json:"submitted_by,omitempty"`
}

// NIS2Notification is the payload for the 72-hour incident notification.
type NIS2Notification struct {
	InitialAssessment string `json:"initial_assessment"`
	Severity          string `json:"severity"`
	Impact            string `json:"impact"`
	IndicatorsOfCompromise []string `json:"indicators_of_compromise,omitempty"`
	SubmittedAt       string `json:"submitted_at,omitempty"`
	SubmittedBy       string `json:"submitted_by,omitempty"`
}

// NIS2FinalReport is the payload for the final incident report (within 1 month).
type NIS2FinalReport struct {
	DetailedDescription string   `json:"detailed_description"`
	RootCause           string   `json:"root_cause"`
	MitigationMeasures  []string `json:"mitigation_measures"`
	CrossBorderImpact   string   `json:"cross_border_impact,omitempty"`
	LessonsLearned      string   `json:"lessons_learned,omitempty"`
	SubmittedAt         string   `json:"submitted_at,omitempty"`
	SubmittedBy         string   `json:"submitted_by,omitempty"`
}

// NIS2Measure represents a cybersecurity risk management measure under Article 21.
type NIS2Measure struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	Category        string `json:"category"`  // risk_analysis, incident_handling, business_continuity, supply_chain, etc.
	Title           string `json:"title"`
	Description     string `json:"description"`
	Status          string `json:"status"` // not_started, in_progress, implemented, verified
	Evidence        string `json:"evidence,omitempty"`
	ResponsibleID   string `json:"responsible_id,omitempty"`
	DueDate         string `json:"due_date,omitempty"`
	CompletedAt     string `json:"completed_at,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// NIS2ManagementOverview holds management body accountability data.
type NIS2ManagementOverview struct {
	OrganizationID string          `json:"organization_id"`
	TrainingRecords []NIS2Training `json:"training_records"`
	TotalMembers    int            `json:"total_members"`
	TrainedMembers  int            `json:"trained_members"`
	ComplianceRate  float64        `json:"compliance_rate"`
}

// NIS2Training records a management body training session.
type NIS2Training struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
	UserName       string `json:"user_name,omitempty"`
	TrainingType   string `json:"training_type" validate:"required"`
	Title          string `json:"title" validate:"required"`
	Description    string `json:"description,omitempty"`
	CompletedAt    string `json:"completed_at"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	CertificateURL string `json:"certificate_url,omitempty"`
	RecordedBy     string `json:"recorded_by"`
	CreatedAt      string `json:"created_at"`
}

// NIS2Dashboard provides a high-level NIS2 compliance overview.
type NIS2Dashboard struct {
	EntityType        string         `json:"entity_type"`
	InScope           bool           `json:"in_scope"`
	MeasuresTotal     int            `json:"measures_total"`
	MeasuresCompleted int            `json:"measures_completed"`
	MeasuresProgress  float64        `json:"measures_progress"`
	MeasuresByStatus  map[string]int `json:"measures_by_status"`
	OpenIncidents     int            `json:"open_incidents"`
	ManagementTrained float64        `json:"management_trained_pct"`
	OverallReadiness  float64        `json:"overall_readiness"`
}

// ---------- handler ----------

// NIS2Handler handles NIS2 directive compliance endpoints.
type NIS2Handler struct {
	svc NIS2Service
}

// NewNIS2Handler creates a new NIS2Handler with the given service.
func NewNIS2Handler(svc NIS2Service) *NIS2Handler {
	return &NIS2Handler{svc: svc}
}

// GetAssessment handles GET /nis2/assessment.
func (h *NIS2Handler) GetAssessment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	assessment, err := h.svc.GetAssessment(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NIS2 assessment not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": assessment})
}

// CreateAssessment handles POST /nis2/assessment.
func (h *NIS2Handler) CreateAssessment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var assessment NIS2Assessment
	if err := json.NewDecoder(r.Body).Decode(&assessment); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if assessment.EntityType == "" || assessment.Sector == "" {
		writeError(w, http.StatusBadRequest, "entity_type and sector are required", "")
		return
	}

	if err := h.svc.CreateAssessment(r.Context(), orgID, userID, &assessment); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create NIS2 assessment", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, assessment)
}

// ListIncidentReports handles GET /nis2/incidents.
func (h *NIS2Handler) ListIncidentReports(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	reports, total, err := h.svc.ListIncidentReports(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list NIS2 incident reports", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": reports,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetIncidentReport handles GET /nis2/incidents/{id}.
func (h *NIS2Handler) GetIncidentReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	reportID := chi.URLParam(r, "id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "Missing incident report ID", "")
		return
	}

	report, err := h.svc.GetIncidentReport(r.Context(), orgID, reportID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NIS2 incident report not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// SubmitEarlyWarning handles POST /nis2/incidents/{id}/early-warning.
func (h *NIS2Handler) SubmitEarlyWarning(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	reportID := chi.URLParam(r, "id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "Missing incident report ID", "")
		return
	}

	var warning NIS2EarlyWarning
	if err := json.NewDecoder(r.Body).Decode(&warning); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if warning.SuspectedCause == "" {
		writeError(w, http.StatusBadRequest, "suspected_cause is required", "")
		return
	}

	if err := h.svc.SubmitEarlyWarning(r.Context(), orgID, userID, reportID, &warning); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit early warning", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Early warning submitted"})
}

// SubmitNotification handles POST /nis2/incidents/{id}/notification.
func (h *NIS2Handler) SubmitNotification(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	reportID := chi.URLParam(r, "id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "Missing incident report ID", "")
		return
	}

	var notification NIS2Notification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if notification.InitialAssessment == "" || notification.Severity == "" {
		writeError(w, http.StatusBadRequest, "initial_assessment and severity are required", "")
		return
	}

	if err := h.svc.SubmitNotification(r.Context(), orgID, userID, reportID, &notification); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit notification", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Incident notification submitted"})
}

// SubmitFinalReport handles POST /nis2/incidents/{id}/final-report.
func (h *NIS2Handler) SubmitFinalReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	reportID := chi.URLParam(r, "id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "Missing incident report ID", "")
		return
	}

	var finalReport NIS2FinalReport
	if err := json.NewDecoder(r.Body).Decode(&finalReport); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if finalReport.DetailedDescription == "" || finalReport.RootCause == "" {
		writeError(w, http.StatusBadRequest, "detailed_description and root_cause are required", "")
		return
	}

	if err := h.svc.SubmitFinalReport(r.Context(), orgID, userID, reportID, &finalReport); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit final report", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Final report submitted"})
}

// GetMeasures handles GET /nis2/measures.
func (h *NIS2Handler) GetMeasures(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	measures, err := h.svc.GetMeasures(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get NIS2 measures", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": measures})
}

// UpdateMeasure handles PUT /nis2/measures/{id}.
func (h *NIS2Handler) UpdateMeasure(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	measureID := chi.URLParam(r, "id")
	if measureID == "" {
		writeError(w, http.StatusBadRequest, "Missing measure ID", "")
		return
	}

	var measure NIS2Measure
	if err := json.NewDecoder(r.Body).Decode(&measure); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	measure.ID = measureID
	measure.OrganizationID = orgID

	if err := h.svc.UpdateMeasure(r.Context(), orgID, &measure); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update NIS2 measure", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, measure)
}

// GetManagement handles GET /nis2/management.
func (h *NIS2Handler) GetManagement(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	overview, err := h.svc.GetManagement(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get management overview", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": overview})
}

// RecordTraining handles POST /nis2/management.
func (h *NIS2Handler) RecordTraining(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var training NIS2Training
	if err := json.NewDecoder(r.Body).Decode(&training); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if training.TrainingType == "" || training.Title == "" {
		writeError(w, http.StatusBadRequest, "training_type and title are required", "")
		return
	}

	if err := h.svc.RecordTraining(r.Context(), orgID, userID, &training); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to record training", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, training)
}

// GetDashboard handles GET /nis2/dashboard.
func (h *NIS2Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get NIS2 dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": dashboard})
}
