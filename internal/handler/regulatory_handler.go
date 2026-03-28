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

// RegulatoryService defines the methods required by RegulatoryHandler.
type RegulatoryService interface {
	ListChanges(ctx context.Context, orgID string, pagination models.PaginationRequest, filters RegulatoryChangeFilters) ([]RegulatoryChange, int, error)
	GetChange(ctx context.Context, orgID, changeID string) (*RegulatoryChangeDetail, error)
	AssessImpact(ctx context.Context, orgID, userID, changeID string, req *ImpactAssessmentRequest) (*ImpactAssessment, error)
	GetAssessment(ctx context.Context, orgID, changeID string) (*ImpactAssessment, error)
	CreateResponsePlan(ctx context.Context, orgID, userID, changeID string, req *RegulatoryResponsePlan) error

	ListSources(ctx context.Context, orgID string) ([]RegulatorySource, error)
	AddSource(ctx context.Context, orgID, userID string, source *RegulatorySource) error

	ListSubscriptions(ctx context.Context, orgID string) ([]RegulatorySubscription, error)
	Subscribe(ctx context.Context, orgID, userID string, sub *RegulatorySubscription) error

	GetDashboard(ctx context.Context, orgID string) (*RegulatoryDashboard, error)
	GetTimeline(ctx context.Context, orgID string, filters TimelineFilters) ([]TimelineEvent, error)
}

// ---------- request / response types ----------

// RegulatoryChangeFilters holds filter parameters for listing regulatory changes.
type RegulatoryChangeFilters struct {
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	Region     string `json:"region"`
	Category   string `json:"category"`
	Search     string `json:"search"`
}

// RegulatoryChange represents a regulatory change event.
type RegulatoryChange struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	SourceID        string   `json:"source_id"`
	SourceName      string   `json:"source_name"`
	Region          string   `json:"region"`
	Category        string   `json:"category"` // data_privacy, cybersecurity, financial, environmental, health
	Severity        string   `json:"severity"` // critical, high, medium, low, informational
	Status          string   `json:"status"`   // new, assessed, responded, archived
	EffectiveDate   string   `json:"effective_date,omitempty"`
	PublishedDate   string   `json:"published_date"`
	AffectedFrameworks []string `json:"affected_frameworks,omitempty"`
	URL             string   `json:"url,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

// RegulatoryChangeDetail extends RegulatoryChange with full text and assessment.
type RegulatoryChangeDetail struct {
	RegulatoryChange
	FullText       string            `json:"full_text,omitempty"`
	Assessment     *ImpactAssessment `json:"assessment,omitempty"`
	ResponsePlan   *RegulatoryResponsePlan `json:"response_plan,omitempty"`
}

// ImpactAssessmentRequest is the payload for POST /regulatory/changes/{id}/assess.
type ImpactAssessmentRequest struct {
	Notes string `json:"notes,omitempty"`
}

// ImpactAssessment represents the organization-specific impact assessment of a regulatory change.
type ImpactAssessment struct {
	ID               string   `json:"id"`
	ChangeID         string   `json:"change_id"`
	OrganizationID   string   `json:"organization_id"`
	ImpactLevel      string   `json:"impact_level"` // critical, high, medium, low, none
	AffectedControls []string `json:"affected_controls,omitempty"`
	AffectedPolicies []string `json:"affected_policies,omitempty"`
	GapCount         int      `json:"gap_count"`
	Summary          string   `json:"summary"`
	Notes            string   `json:"notes,omitempty"`
	AssessedBy       string   `json:"assessed_by"`
	AssessedAt       string   `json:"assessed_at"`
}

// RegulatoryResponsePlan is the response plan for a regulatory change.
type RegulatoryResponsePlan struct {
	ID             string                 `json:"id"`
	ChangeID       string                 `json:"change_id"`
	OrganizationID string                 `json:"organization_id"`
	Title          string                 `json:"title" validate:"required"`
	Status         string                 `json:"status"` // draft, in_progress, completed
	Actions        []RegulatoryAction     `json:"actions,omitempty"`
	DueDate        string                 `json:"due_date,omitempty"`
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// RegulatoryAction is an action item within a regulatory response plan.
type RegulatoryAction struct {
	Title      string `json:"title"`
	AssigneeID string `json:"assignee_id,omitempty"`
	DueDate    string `json:"due_date,omitempty"`
	Status     string `json:"status"`
}

// RegulatorySource represents a regulatory intelligence source.
type RegulatorySource struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id,omitempty"`
	Name           string `json:"name" validate:"required"`
	Type           string `json:"type"` // official, third_party, custom
	URL            string `json:"url,omitempty"`
	Region         string `json:"region,omitempty"`
	IsBuiltIn      bool   `json:"is_built_in"`
	IsActive       bool   `json:"is_active"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// RegulatorySubscription represents an organization's subscription to regulatory topics.
type RegulatorySubscription struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Regions        []string `json:"regions,omitempty"`
	Categories     []string `json:"categories,omitempty"`
	Severities     []string `json:"severities,omitempty"`
	Keywords       []string `json:"keywords,omitempty"`
	NotifyEmail    bool     `json:"notify_email"`
	NotifyInApp    bool     `json:"notify_in_app"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// RegulatoryDashboard provides aggregate regulatory intelligence metrics.
type RegulatoryDashboard struct {
	TotalChanges      int            `json:"total_changes"`
	NewChanges        int            `json:"new_changes"`
	PendingAssessment int            `json:"pending_assessment"`
	CriticalChanges   int            `json:"critical_changes"`
	ByRegion          map[string]int `json:"by_region"`
	ByCategory        map[string]int `json:"by_category"`
	BySeverity        map[string]int `json:"by_severity"`
	UpcomingDeadlines []RegulatoryChange `json:"upcoming_deadlines"`
}

// TimelineFilters holds filter parameters for the regulatory timeline.
type TimelineFilters struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Region    string `json:"region"`
	Category  string `json:"category"`
}

// TimelineEvent is a single event in the regulatory timeline.
type TimelineEvent struct {
	Date        string `json:"date"`
	Type        string `json:"type"` // effective_date, deadline, published
	ChangeID    string `json:"change_id"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Description string `json:"description,omitempty"`
}

// ---------- handler ----------

// RegulatoryHandler handles regulatory change intelligence endpoints.
type RegulatoryHandler struct {
	svc RegulatoryService
}

// NewRegulatoryHandler creates a new RegulatoryHandler with the given service.
func NewRegulatoryHandler(svc RegulatoryService) *RegulatoryHandler {
	return &RegulatoryHandler{svc: svc}
}

// ListChanges handles GET /regulatory/changes.
func (h *RegulatoryHandler) ListChanges(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := RegulatoryChangeFilters{
		Status:   r.URL.Query().Get("status"),
		Severity: r.URL.Query().Get("severity"),
		Region:   r.URL.Query().Get("region"),
		Category: r.URL.Query().Get("category"),
		Search:   r.URL.Query().Get("search"),
	}

	changes, total, err := h.svc.ListChanges(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list regulatory changes", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": changes,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetChange handles GET /regulatory/changes/{id}.
func (h *RegulatoryHandler) GetChange(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	changeID := chi.URLParam(r, "id")
	if changeID == "" {
		writeError(w, http.StatusBadRequest, "Missing change ID", "")
		return
	}

	detail, err := h.svc.GetChange(r.Context(), orgID, changeID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Regulatory change not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// AssessImpact handles POST /regulatory/changes/{id}/assess.
func (h *RegulatoryHandler) AssessImpact(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	changeID := chi.URLParam(r, "id")
	if changeID == "" {
		writeError(w, http.StatusBadRequest, "Missing change ID", "")
		return
	}

	var req ImpactAssessmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	assessment, err := h.svc.AssessImpact(r.Context(), orgID, userID, changeID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to assess impact", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, assessment)
}

// GetAssessment handles GET /regulatory/changes/{id}/assessment.
func (h *RegulatoryHandler) GetAssessment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	changeID := chi.URLParam(r, "id")
	if changeID == "" {
		writeError(w, http.StatusBadRequest, "Missing change ID", "")
		return
	}

	assessment, err := h.svc.GetAssessment(r.Context(), orgID, changeID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Assessment not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, assessment)
}

// CreateResponsePlan handles POST /regulatory/changes/{id}/respond.
func (h *RegulatoryHandler) CreateResponsePlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	changeID := chi.URLParam(r, "id")
	if changeID == "" {
		writeError(w, http.StatusBadRequest, "Missing change ID", "")
		return
	}

	var plan RegulatoryResponsePlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if plan.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	}

	if err := h.svc.CreateResponsePlan(r.Context(), orgID, userID, changeID, &plan); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create response plan", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, plan)
}

// ListSources handles GET /regulatory/sources.
func (h *RegulatoryHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	sources, err := h.svc.ListSources(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list regulatory sources", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": sources})
}

// AddSource handles POST /regulatory/sources.
func (h *RegulatoryHandler) AddSource(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var source RegulatorySource
	if err := json.NewDecoder(r.Body).Decode(&source); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if source.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.AddSource(r.Context(), orgID, userID, &source); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to add regulatory source", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, source)
}

// ListSubscriptions handles GET /regulatory/subscriptions.
func (h *RegulatoryHandler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	subs, err := h.svc.ListSubscriptions(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list regulatory subscriptions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": subs})
}

// Subscribe handles POST /regulatory/subscriptions.
func (h *RegulatoryHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var sub RegulatorySubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.Subscribe(r.Context(), orgID, userID, &sub); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create subscription", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sub)
}

// GetDashboard handles GET /regulatory/dashboard.
func (h *RegulatoryHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get regulatory dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": dashboard})
}

// GetTimeline handles GET /regulatory/timeline.
func (h *RegulatoryHandler) GetTimeline(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	filters := TimelineFilters{
		StartDate: r.URL.Query().Get("start_date"),
		EndDate:   r.URL.Query().Get("end_date"),
		Region:    r.URL.Query().Get("region"),
		Category:  r.URL.Query().Get("category"),
	}

	events, err := h.svc.GetTimeline(r.Context(), orgID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get regulatory timeline", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": events})
}
