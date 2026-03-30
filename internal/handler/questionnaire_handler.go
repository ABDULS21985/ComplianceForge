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

// QuestionnaireService defines the methods required by QuestionnaireHandler and VendorPortalHandler.
type QuestionnaireService interface {
	// Questionnaire templates
	ListQuestionnaires(ctx context.Context, orgID string, pagination models.PaginationRequest, filters QuestionnaireFilters) ([]Questionnaire, int, error)
	CreateQuestionnaire(ctx context.Context, orgID, userID string, q *Questionnaire) error
	GetQuestionnaire(ctx context.Context, orgID, questionnaireID string) (*QuestionnaireDetail, error)
	UpdateQuestionnaire(ctx context.Context, orgID string, q *Questionnaire) error
	CloneQuestionnaire(ctx context.Context, orgID, userID, questionnaireID string) (*Questionnaire, error)

	// Vendor assessments
	ListVendorAssessments(ctx context.Context, orgID string, pagination models.PaginationRequest, filters VendorAssessmentFilters) ([]VendorAssessment, int, error)
	CreateVendorAssessment(ctx context.Context, orgID, userID string, assessment *VendorAssessment) error
	GetVendorAssessment(ctx context.Context, orgID, assessmentID string) (*VendorAssessmentDetail, error)
	ReviewVendorAssessment(ctx context.Context, orgID, userID, assessmentID string, req *ReviewAssessmentRequest) error
	SendReminder(ctx context.Context, orgID, assessmentID string) error
	CompareAssessments(ctx context.Context, orgID string, assessmentIDs []string) (*AssessmentComparison, error)
	GetAssessmentDashboard(ctx context.Context, orgID string) (*AssessmentDashboard, error)

	// Vendor portal (public, token-authenticated)
	GetPortalQuestionnaire(ctx context.Context, token string) (*PortalQuestionnaire, error)
	UpdatePortalResponses(ctx context.Context, token string, responses *PortalResponses) error
	UploadPortalEvidence(ctx context.Context, token, questionID string, evidence *PortalEvidence) error
	SubmitPortalAssessment(ctx context.Context, token string) error
	GetPortalProgress(ctx context.Context, token string) (*PortalProgress, error)
}

// ---------- request / response types ----------

// QuestionnaireFilters holds filter parameters for listing questionnaires.
type QuestionnaireFilters struct {
	Category string `json:"category"`
	Status   string `json:"status"`
	Search   string `json:"search"`
}

// Questionnaire represents a vendor assessment questionnaire template.
type Questionnaire struct {
	ID             string              `json:"id"`
	OrganizationID string              `json:"organization_id"`
	Title          string              `json:"title" validate:"required"`
	Description    string              `json:"description"`
	Category       string              `json:"category"` // security, privacy, compliance, general
	Status         string              `json:"status"`   // draft, active, archived
	Version        int                 `json:"version"`
	Sections       []QuestionSection   `json:"sections,omitempty"`
	ScoringMethod  string              `json:"scoring_method,omitempty"` // weighted, equal, pass_fail
	CreatedBy      string              `json:"created_by"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

// QuestionnaireDetail extends Questionnaire with usage statistics.
type QuestionnaireDetail struct {
	Questionnaire
	TotalAssessments    int `json:"total_assessments"`
	ActiveAssessments   int `json:"active_assessments"`
	AverageScore        float64 `json:"average_score"`
}

// QuestionSection represents a section within a questionnaire.
type QuestionSection struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Order       int        `json:"order"`
	Weight      float64    `json:"weight,omitempty"`
	Questions   []Question `json:"questions,omitempty"`
}

// Question represents a single question in a questionnaire.
type Question struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Type         string   `json:"type"` // text, single_choice, multi_choice, yes_no, file_upload, scale
	Required     bool     `json:"required"`
	Options      []string `json:"options,omitempty"`
	Weight       float64  `json:"weight,omitempty"`
	HelpText     string   `json:"help_text,omitempty"`
	Order        int      `json:"order"`
	RequiresEvidence bool `json:"requires_evidence"`
}

// VendorAssessmentFilters holds filter parameters for listing vendor assessments.
type VendorAssessmentFilters struct {
	VendorID        string `json:"vendor_id"`
	QuestionnaireID string `json:"questionnaire_id"`
	Status          string `json:"status"`
	Search          string `json:"search"`
}

// VendorAssessment represents a vendor assessment instance.
type VendorAssessment struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	VendorID        string `json:"vendor_id" validate:"required"`
	QuestionnaireID string `json:"questionnaire_id" validate:"required"`
	VendorName      string `json:"vendor_name,omitempty"`
	Status          string `json:"status"` // draft, sent, in_progress, submitted, under_review, completed, expired
	PortalToken     string `json:"portal_token,omitempty"`
	DueDate         string `json:"due_date,omitempty"`
	Score           float64 `json:"score,omitempty"`
	RiskRating      string `json:"risk_rating,omitempty"`
	SentAt          string `json:"sent_at,omitempty"`
	SubmittedAt     string `json:"submitted_at,omitempty"`
	ReviewedAt      string `json:"reviewed_at,omitempty"`
	ReviewedBy      string `json:"reviewed_by,omitempty"`
	CreatedBy       string `json:"created_by"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// VendorAssessmentDetail extends VendorAssessment with responses.
type VendorAssessmentDetail struct {
	VendorAssessment
	Responses []QuestionResponse `json:"responses,omitempty"`
}

// QuestionResponse represents a vendor's response to a question.
type QuestionResponse struct {
	QuestionID  string   `json:"question_id"`
	Answer      string   `json:"answer,omitempty"`
	Answers     []string `json:"answers,omitempty"`
	EvidenceURL string   `json:"evidence_url,omitempty"`
	Score       float64  `json:"score,omitempty"`
	Notes       string   `json:"notes,omitempty"`
}

// ReviewAssessmentRequest is the payload for POST /vendor-assessments/{id}/review.
type ReviewAssessmentRequest struct {
	RiskRating    string              `json:"risk_rating" validate:"required"`
	Comments      string              `json:"comments,omitempty"`
	QuestionNotes []QuestionReviewNote `json:"question_notes,omitempty"`
}

// QuestionReviewNote is a reviewer's note on a specific question response.
type QuestionReviewNote struct {
	QuestionID string `json:"question_id"`
	Score      float64 `json:"score"`
	Note       string  `json:"note,omitempty"`
}

// AssessmentComparison holds comparison data between multiple vendor assessments.
type AssessmentComparison struct {
	Assessments []AssessmentSummary  `json:"assessments"`
	BySection   []SectionComparison  `json:"by_section"`
}

// AssessmentSummary is a summary of a single assessment for comparison.
type AssessmentSummary struct {
	AssessmentID string  `json:"assessment_id"`
	VendorName   string  `json:"vendor_name"`
	OverallScore float64 `json:"overall_score"`
	RiskRating   string  `json:"risk_rating"`
	SubmittedAt  string  `json:"submitted_at"`
}

// SectionComparison holds per-section scores for compared assessments.
type SectionComparison struct {
	SectionTitle string             `json:"section_title"`
	Scores       map[string]float64 `json:"scores"` // assessment_id -> score
}

// AssessmentDashboard provides vendor assessment metrics for an organization.
type AssessmentDashboard struct {
	TotalAssessments   int            `json:"total_assessments"`
	PendingResponses   int            `json:"pending_responses"`
	UnderReview        int            `json:"under_review"`
	CompletedThisMonth int            `json:"completed_this_month"`
	AverageScore       float64        `json:"average_score"`
	ByStatus           map[string]int `json:"by_status"`
	ByRiskRating       map[string]int `json:"by_risk_rating"`
	OverdueCount       int            `json:"overdue_count"`
}

// PortalQuestionnaire is the questionnaire data returned to the vendor portal.
type PortalQuestionnaire struct {
	AssessmentID    string            `json:"assessment_id"`
	VendorName      string            `json:"vendor_name"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	DueDate         string            `json:"due_date,omitempty"`
	Sections        []QuestionSection `json:"sections"`
	ExistingResponses []QuestionResponse `json:"existing_responses,omitempty"`
}

// PortalResponses is the payload for PUT /vendor-portal/{token}/responses.
type PortalResponses struct {
	Responses []QuestionResponse `json:"responses" validate:"required"`
}

// PortalEvidence is the payload for POST /vendor-portal/{token}/responses/{questionId}/evidence.
type PortalEvidence struct {
	FileName    string `json:"file_name" validate:"required"`
	ContentType string `json:"content_type"`
	FileData    string `json:"file_data"` // base64-encoded
	URL         string `json:"url,omitempty"`
}

// PortalProgress provides progress information for the vendor portal.
type PortalProgress struct {
	AssessmentID     string  `json:"assessment_id"`
	TotalQuestions   int     `json:"total_questions"`
	AnsweredQuestions int    `json:"answered_questions"`
	RequiredRemaining int   `json:"required_remaining"`
	CompletionPct    float64 `json:"completion_pct"`
	CanSubmit        bool    `json:"can_submit"`
}

// ---------- handler ----------

// QuestionnaireHandler handles questionnaire and vendor assessment endpoints (authenticated).
type QuestionnaireHandler struct {
	svc QuestionnaireService
}

// NewQuestionnaireHandler creates a new QuestionnaireHandler with the given service.
func NewQuestionnaireHandler(svc QuestionnaireService) *QuestionnaireHandler {
	return &QuestionnaireHandler{svc: svc}
}

// ListQuestionnaires handles GET /questionnaires.
func (h *QuestionnaireHandler) ListQuestionnaires(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := QuestionnaireFilters{
		Category: r.URL.Query().Get("category"),
		Status:   r.URL.Query().Get("status"),
		Search:   r.URL.Query().Get("search"),
	}

	questionnaires, total, err := h.svc.ListQuestionnaires(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list questionnaires", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": questionnaires,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateQuestionnaire handles POST /questionnaires.
func (h *QuestionnaireHandler) CreateQuestionnaire(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var q Questionnaire
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if q.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	}

	if err := h.svc.CreateQuestionnaire(r.Context(), orgID, userID, &q); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create questionnaire", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, q)
}

// GetQuestionnaire handles GET /questionnaires/{id}.
func (h *QuestionnaireHandler) GetQuestionnaire(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	questionnaireID := chi.URLParam(r, "id")
	if questionnaireID == "" {
		writeError(w, http.StatusBadRequest, "Missing questionnaire ID", "")
		return
	}

	detail, err := h.svc.GetQuestionnaire(r.Context(), orgID, questionnaireID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Questionnaire not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// UpdateQuestionnaire handles PUT /questionnaires/{id}.
func (h *QuestionnaireHandler) UpdateQuestionnaire(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	questionnaireID := chi.URLParam(r, "id")
	if questionnaireID == "" {
		writeError(w, http.StatusBadRequest, "Missing questionnaire ID", "")
		return
	}

	var q Questionnaire
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	q.ID = questionnaireID
	q.OrganizationID = orgID

	if err := h.svc.UpdateQuestionnaire(r.Context(), orgID, &q); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update questionnaire", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, q)
}

// CloneQuestionnaire handles POST /questionnaires/{id}/clone.
func (h *QuestionnaireHandler) CloneQuestionnaire(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	questionnaireID := chi.URLParam(r, "id")
	if questionnaireID == "" {
		writeError(w, http.StatusBadRequest, "Missing questionnaire ID", "")
		return
	}

	cloned, err := h.svc.CloneQuestionnaire(r.Context(), orgID, userID, questionnaireID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to clone questionnaire", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, cloned)
}

// ListVendorAssessments handles GET /vendor-assessments.
func (h *QuestionnaireHandler) ListVendorAssessments(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := VendorAssessmentFilters{
		VendorID:        r.URL.Query().Get("vendor_id"),
		QuestionnaireID: r.URL.Query().Get("questionnaire_id"),
		Status:          r.URL.Query().Get("status"),
		Search:          r.URL.Query().Get("search"),
	}

	assessments, total, err := h.svc.ListVendorAssessments(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list vendor assessments", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": assessments,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateVendorAssessment handles POST /vendor-assessments.
func (h *QuestionnaireHandler) CreateVendorAssessment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var assessment VendorAssessment
	if err := json.NewDecoder(r.Body).Decode(&assessment); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if assessment.VendorID == "" || assessment.QuestionnaireID == "" {
		writeError(w, http.StatusBadRequest, "vendor_id and questionnaire_id are required", "")
		return
	}

	if err := h.svc.CreateVendorAssessment(r.Context(), orgID, userID, &assessment); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create vendor assessment", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, assessment)
}

// GetVendorAssessment handles GET /vendor-assessments/{id}.
func (h *QuestionnaireHandler) GetVendorAssessment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	assessmentID := chi.URLParam(r, "id")
	if assessmentID == "" {
		writeError(w, http.StatusBadRequest, "Missing assessment ID", "")
		return
	}

	detail, err := h.svc.GetVendorAssessment(r.Context(), orgID, assessmentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Vendor assessment not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// ReviewVendorAssessment handles POST /vendor-assessments/{id}/review.
func (h *QuestionnaireHandler) ReviewVendorAssessment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	assessmentID := chi.URLParam(r, "id")
	if assessmentID == "" {
		writeError(w, http.StatusBadRequest, "Missing assessment ID", "")
		return
	}

	var req ReviewAssessmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.RiskRating == "" {
		writeError(w, http.StatusBadRequest, "risk_rating is required", "")
		return
	}

	if err := h.svc.ReviewVendorAssessment(r.Context(), orgID, userID, assessmentID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to review vendor assessment", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Assessment reviewed"})
}

// SendReminder handles POST /vendor-assessments/{id}/reminder.
func (h *QuestionnaireHandler) SendReminder(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	assessmentID := chi.URLParam(r, "id")
	if assessmentID == "" {
		writeError(w, http.StatusBadRequest, "Missing assessment ID", "")
		return
	}

	if err := h.svc.SendReminder(r.Context(), orgID, assessmentID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to send reminder", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Reminder sent"})
}

// CompareAssessments handles GET /vendor-assessments/compare.
func (h *QuestionnaireHandler) CompareAssessments(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	ids := r.URL.Query()["ids"]
	if len(ids) < 2 {
		writeError(w, http.StatusBadRequest, "At least 2 assessment IDs required in 'ids' query parameter", "")
		return
	}

	comparison, err := h.svc.CompareAssessments(r.Context(), orgID, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to compare assessments", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, comparison)
}

// GetAssessmentDashboard handles GET /vendor-assessments/dashboard.
func (h *QuestionnaireHandler) GetAssessmentDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetAssessmentDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get assessment dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dashboard)
}

// ---------- vendor portal handler (public, token-authenticated) ----------

// VendorPortalHandler handles vendor-facing portal endpoints (no JWT required).
type VendorPortalHandler struct {
	svc QuestionnaireService
}

// NewVendorPortalHandler creates a new VendorPortalHandler with the given service.
func NewVendorPortalHandler(svc QuestionnaireService) *VendorPortalHandler {
	return &VendorPortalHandler{svc: svc}
}

// GetQuestionnaire handles GET /vendor-portal/{token}.
func (h *VendorPortalHandler) GetQuestionnaire(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	questionnaire, err := h.svc.GetPortalQuestionnaire(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "Questionnaire not found or token expired", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, questionnaire)
}

// UpdateResponses handles PUT /vendor-portal/{token}/responses.
func (h *VendorPortalHandler) UpdateResponses(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	var responses PortalResponses
	if err := json.NewDecoder(r.Body).Decode(&responses); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if len(responses.Responses) == 0 {
		writeError(w, http.StatusBadRequest, "responses is required", "")
		return
	}

	if err := h.svc.UpdatePortalResponses(r.Context(), token, &responses); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update responses", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Responses saved"})
}

// UploadEvidence handles POST /vendor-portal/{token}/responses/{questionId}/evidence.
func (h *VendorPortalHandler) UploadEvidence(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	questionID := chi.URLParam(r, "questionId")
	if questionID == "" {
		writeError(w, http.StatusBadRequest, "Missing question ID", "")
		return
	}

	var evidence PortalEvidence
	if err := json.NewDecoder(r.Body).Decode(&evidence); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if evidence.FileName == "" {
		writeError(w, http.StatusBadRequest, "file_name is required", "")
		return
	}

	if err := h.svc.UploadPortalEvidence(r.Context(), token, questionID, &evidence); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to upload evidence", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "Evidence uploaded"})
}

// Submit handles POST /vendor-portal/{token}/submit.
func (h *VendorPortalHandler) Submit(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	if err := h.svc.SubmitPortalAssessment(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit assessment", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Assessment submitted"})
}

// GetProgress handles GET /vendor-portal/{token}/progress.
func (h *VendorPortalHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	progress, err := h.svc.GetPortalProgress(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "Assessment not found or token expired", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, progress)
}
