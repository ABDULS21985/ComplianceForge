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

// RemediationService defines the methods required by RemediationHandler.
type RemediationService interface {
	// Remediation plans
	ListPlans(ctx context.Context, orgID string, pagination models.PaginationRequest, filters RemediationPlanFilters) ([]RemediationPlan, int, error)
	CreatePlan(ctx context.Context, orgID, userID string, plan *RemediationPlan) error
	GeneratePlan(ctx context.Context, orgID, userID string, req *GeneratePlanRequest) (*RemediationPlan, error)
	GetPlan(ctx context.Context, orgID, planID string) (*RemediationPlanDetail, error)
	UpdatePlan(ctx context.Context, orgID string, plan *RemediationPlan) error
	ApprovePlan(ctx context.Context, orgID, userID, planID string, req *ApprovePlanRequest) error
	GetPlanProgress(ctx context.Context, orgID, planID string) (*PlanProgress, error)

	// Remediation actions
	UpdateAction(ctx context.Context, orgID string, actionID string, update *ActionUpdate) error
	CompleteAction(ctx context.Context, orgID, userID, actionID string, req *CompleteActionRequest) error

	// AI assistance
	GetControlGuidance(ctx context.Context, orgID string, req *ControlGuidanceRequest) (*ControlGuidanceResponse, error)
	GetEvidenceSuggestion(ctx context.Context, orgID string, req *EvidenceSuggestionRequest) (*EvidenceSuggestionResponse, error)
	GetPolicyDraft(ctx context.Context, orgID string, req *PolicyDraftRequest) (*PolicyDraftResponse, error)
	GetRiskNarrative(ctx context.Context, orgID string, req *RiskNarrativeRequest) (*RiskNarrativeResponse, error)
	GetAIUsage(ctx context.Context, orgID string) (*AIUsageStats, error)
	SubmitAIFeedback(ctx context.Context, orgID, userID string, feedback *AIFeedback) error
}

// ---------- request / response types ----------

// RemediationPlanFilters holds filter parameters for listing remediation plans.
type RemediationPlanFilters struct {
	Status     string `json:"status"`
	Priority   string `json:"priority"`
	AssigneeID string `json:"assignee_id"`
	Search     string `json:"search"`
}

// RemediationPlan represents a remediation plan.
type RemediationPlan struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Title          string   `json:"title" validate:"required"`
	Description    string   `json:"description"`
	Status         string   `json:"status"`   // draft, pending_approval, approved, in_progress, completed, cancelled
	Priority       string   `json:"priority"` // critical, high, medium, low
	Source         string   `json:"source"`   // manual, ai_generated
	GapIDs         []string `json:"gap_ids,omitempty"`
	AssigneeID     string   `json:"assignee_id,omitempty"`
	DueDate        string   `json:"due_date,omitempty"`
	ApprovedBy     string   `json:"approved_by,omitempty"`
	ApprovedAt     string   `json:"approved_at,omitempty"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	CompletedAt    string   `json:"completed_at,omitempty"`
}

// RemediationPlanDetail extends RemediationPlan with actions.
type RemediationPlanDetail struct {
	RemediationPlan
	Actions []RemediationAction `json:"actions"`
}

// RemediationAction represents a single action within a remediation plan.
type RemediationAction struct {
	ID          string `json:"id"`
	PlanID      string `json:"plan_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"` // pending, in_progress, completed, skipped
	AssigneeID  string `json:"assignee_id,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	EvidenceURL string `json:"evidence_url,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Order       int    `json:"order"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// GeneratePlanRequest is the payload for POST /remediation/plans/generate.
type GeneratePlanRequest struct {
	GapIDs      []string `json:"gap_ids" validate:"required"`
	FrameworkID string   `json:"framework_id,omitempty"`
	Priority    string   `json:"priority,omitempty"`
}

// ApprovePlanRequest is the payload for POST /remediation/plans/{id}/approve.
type ApprovePlanRequest struct {
	Comments string `json:"comments,omitempty"`
}

// PlanProgress holds progress metrics for a remediation plan.
type PlanProgress struct {
	PlanID           string  `json:"plan_id"`
	TotalActions     int     `json:"total_actions"`
	CompletedActions int     `json:"completed_actions"`
	InProgressActions int    `json:"in_progress_actions"`
	OverdueActions   int     `json:"overdue_actions"`
	CompletionPct    float64 `json:"completion_pct"`
	EstimatedDays    int     `json:"estimated_days_remaining"`
}

// ActionUpdate is the payload for PUT /remediation/actions/{id}.
type ActionUpdate struct {
	Status     string `json:"status"`
	AssigneeID string `json:"assignee_id,omitempty"`
	DueDate    string `json:"due_date,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// CompleteActionRequest is the payload for POST /remediation/actions/{id}/complete.
type CompleteActionRequest struct {
	EvidenceURL string `json:"evidence_url,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// ControlGuidanceRequest is the payload for POST /ai/control-guidance.
type ControlGuidanceRequest struct {
	ControlID   string `json:"control_id" validate:"required"`
	FrameworkID string `json:"framework_id,omitempty"`
	Context     string `json:"context,omitempty"`
}

// ControlGuidanceResponse is the AI-generated guidance for a control.
type ControlGuidanceResponse struct {
	ControlID       string   `json:"control_id"`
	Guidance        string   `json:"guidance"`
	Steps           []string `json:"steps"`
	References      []string `json:"references,omitempty"`
	EffortEstimate  string   `json:"effort_estimate,omitempty"`
	TokensUsed      int      `json:"tokens_used"`
}

// EvidenceSuggestionRequest is the payload for POST /ai/evidence-suggestion.
type EvidenceSuggestionRequest struct {
	ControlID   string `json:"control_id" validate:"required"`
	FrameworkID string `json:"framework_id,omitempty"`
}

// EvidenceSuggestionResponse is the AI-generated evidence suggestions.
type EvidenceSuggestionResponse struct {
	ControlID   string               `json:"control_id"`
	Suggestions []EvidenceSuggestion `json:"suggestions"`
	TokensUsed  int                  `json:"tokens_used"`
}

// EvidenceSuggestion is a single evidence suggestion.
type EvidenceSuggestion struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

// PolicyDraftRequest is the payload for POST /ai/policy-draft.
type PolicyDraftRequest struct {
	PolicyType  string   `json:"policy_type" validate:"required"`
	Frameworks  []string `json:"frameworks,omitempty"`
	Industry    string   `json:"industry,omitempty"`
	OrgContext  string   `json:"org_context,omitempty"`
}

// PolicyDraftResponse is the AI-generated policy draft.
type PolicyDraftResponse struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Sections   []PolicySection `json:"sections"`
	TokensUsed int    `json:"tokens_used"`
}

// PolicySection is a section within a generated policy.
type PolicySection struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// RiskNarrativeRequest is the payload for POST /ai/risk-narrative.
type RiskNarrativeRequest struct {
	RiskID  string `json:"risk_id" validate:"required"`
	Format  string `json:"format,omitempty"` // executive, technical, board
}

// RiskNarrativeResponse is the AI-generated risk narrative.
type RiskNarrativeResponse struct {
	RiskID     string `json:"risk_id"`
	Narrative  string `json:"narrative"`
	TokensUsed int    `json:"tokens_used"`
}

// AIUsageStats provides AI usage metrics for an organization.
type AIUsageStats struct {
	TotalRequests    int            `json:"total_requests"`
	TokensUsed       int            `json:"tokens_used"`
	TokensRemaining  int            `json:"tokens_remaining"`
	RequestsByType   map[string]int `json:"requests_by_type"`
	AverageFeedback  float64        `json:"average_feedback"`
	PeriodStart      string         `json:"period_start"`
	PeriodEnd        string         `json:"period_end"`
}

// AIFeedback is the payload for POST /ai/feedback.
type AIFeedback struct {
	RequestID  string `json:"request_id" validate:"required"`
	Rating     int    `json:"rating" validate:"required"` // 1-5
	Comment    string `json:"comment,omitempty"`
	Useful     bool   `json:"useful"`
}

// ---------- handler ----------

// RemediationHandler handles AI remediation and assistance endpoints.
type RemediationHandler struct {
	svc RemediationService
}

// NewRemediationHandler creates a new RemediationHandler with the given service.
func NewRemediationHandler(svc RemediationService) *RemediationHandler {
	return &RemediationHandler{svc: svc}
}

// ListPlans handles GET /remediation/plans.
func (h *RemediationHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := RemediationPlanFilters{
		Status:     r.URL.Query().Get("status"),
		Priority:   r.URL.Query().Get("priority"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
		Search:     r.URL.Query().Get("search"),
	}

	plans, total, err := h.svc.ListPlans(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list remediation plans", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": plans,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreatePlan handles POST /remediation/plans.
func (h *RemediationHandler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var plan RemediationPlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if plan.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	}

	if err := h.svc.CreatePlan(r.Context(), orgID, userID, &plan); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create remediation plan", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, plan)
}

// GeneratePlan handles POST /remediation/plans/generate.
func (h *RemediationHandler) GeneratePlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req GeneratePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if len(req.GapIDs) == 0 {
		writeError(w, http.StatusBadRequest, "gap_ids is required", "")
		return
	}

	plan, err := h.svc.GeneratePlan(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate remediation plan", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, plan)
}

// GetPlan handles GET /remediation/plans/{id}.
func (h *RemediationHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	planID := chi.URLParam(r, "id")
	if planID == "" {
		writeError(w, http.StatusBadRequest, "Missing plan ID", "")
		return
	}

	detail, err := h.svc.GetPlan(r.Context(), orgID, planID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Remediation plan not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// UpdatePlan handles PUT /remediation/plans/{id}.
func (h *RemediationHandler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	planID := chi.URLParam(r, "id")
	if planID == "" {
		writeError(w, http.StatusBadRequest, "Missing plan ID", "")
		return
	}

	var plan RemediationPlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	plan.ID = planID
	plan.OrganizationID = orgID

	if err := h.svc.UpdatePlan(r.Context(), orgID, &plan); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update remediation plan", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

// ApprovePlan handles POST /remediation/plans/{id}/approve.
func (h *RemediationHandler) ApprovePlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	planID := chi.URLParam(r, "id")
	if planID == "" {
		writeError(w, http.StatusBadRequest, "Missing plan ID", "")
		return
	}

	var req ApprovePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.ApprovePlan(r.Context(), orgID, userID, planID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to approve remediation plan", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Plan approved"})
}

// GetPlanProgress handles GET /remediation/plans/{id}/progress.
func (h *RemediationHandler) GetPlanProgress(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	planID := chi.URLParam(r, "id")
	if planID == "" {
		writeError(w, http.StatusBadRequest, "Missing plan ID", "")
		return
	}

	progress, err := h.svc.GetPlanProgress(r.Context(), orgID, planID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Plan progress not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, progress)
}

// UpdateAction handles PUT /remediation/actions/{id}.
func (h *RemediationHandler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	actionID := chi.URLParam(r, "id")
	if actionID == "" {
		writeError(w, http.StatusBadRequest, "Missing action ID", "")
		return
	}

	var update ActionUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.UpdateAction(r.Context(), orgID, actionID, &update); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update action", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Action updated"})
}

// CompleteAction handles POST /remediation/actions/{id}/complete.
func (h *RemediationHandler) CompleteAction(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	actionID := chi.URLParam(r, "id")
	if actionID == "" {
		writeError(w, http.StatusBadRequest, "Missing action ID", "")
		return
	}

	var req CompleteActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.CompleteAction(r.Context(), orgID, userID, actionID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to complete action", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Action completed"})
}

// GetControlGuidance handles POST /ai/control-guidance.
func (h *RemediationHandler) GetControlGuidance(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	var req ControlGuidanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.ControlID == "" {
		writeError(w, http.StatusBadRequest, "control_id is required", "")
		return
	}

	resp, err := h.svc.GetControlGuidance(r.Context(), orgID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate control guidance", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetEvidenceSuggestion handles POST /ai/evidence-suggestion.
func (h *RemediationHandler) GetEvidenceSuggestion(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	var req EvidenceSuggestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.ControlID == "" {
		writeError(w, http.StatusBadRequest, "control_id is required", "")
		return
	}

	resp, err := h.svc.GetEvidenceSuggestion(r.Context(), orgID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate evidence suggestions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetPolicyDraft handles POST /ai/policy-draft.
func (h *RemediationHandler) GetPolicyDraft(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	var req PolicyDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.PolicyType == "" {
		writeError(w, http.StatusBadRequest, "policy_type is required", "")
		return
	}

	resp, err := h.svc.GetPolicyDraft(r.Context(), orgID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate policy draft", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetRiskNarrative handles POST /ai/risk-narrative.
func (h *RemediationHandler) GetRiskNarrative(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	var req RiskNarrativeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.RiskID == "" {
		writeError(w, http.StatusBadRequest, "risk_id is required", "")
		return
	}

	resp, err := h.svc.GetRiskNarrative(r.Context(), orgID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate risk narrative", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetAIUsage handles GET /ai/usage.
func (h *RemediationHandler) GetAIUsage(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	stats, err := h.svc.GetAIUsage(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get AI usage stats", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// SubmitAIFeedback handles POST /ai/feedback.
func (h *RemediationHandler) SubmitAIFeedback(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var feedback AIFeedback
	if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if feedback.RequestID == "" || feedback.Rating == 0 {
		writeError(w, http.StatusBadRequest, "request_id and rating are required", "")
		return
	}

	if feedback.Rating < 1 || feedback.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating must be between 1 and 5", "")
		return
	}

	if err := h.svc.SubmitAIFeedback(r.Context(), orgID, userID, &feedback); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit AI feedback", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "Feedback submitted"})
}
