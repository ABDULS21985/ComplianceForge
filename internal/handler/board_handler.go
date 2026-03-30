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

// BoardService defines the methods required by BoardHandler for board governance management.
type BoardService interface {
	// Board members
	ListMembers(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]BoardMember, int, error)
	CreateMember(ctx context.Context, orgID, userID string, member *BoardMember) error
	UpdateMember(ctx context.Context, orgID string, member *BoardMember) error

	// Board meetings
	ListMeetings(ctx context.Context, orgID string, pagination models.PaginationRequest, filters BoardMeetingFilters) ([]BoardMeeting, int, error)
	CreateMeeting(ctx context.Context, orgID, userID string, meeting *BoardMeeting) error
	UpdateMeeting(ctx context.Context, orgID string, meeting *BoardMeeting) error
	GenerateMeetingPack(ctx context.Context, orgID, userID, meetingID string) (*MeetingPack, error)
	DownloadMeetingPack(ctx context.Context, orgID, meetingID string) (*MeetingPackFile, error)

	// Board decisions
	CreateDecision(ctx context.Context, orgID, userID string, decision *BoardDecision) error
	ListDecisions(ctx context.Context, orgID string, pagination models.PaginationRequest, filters BoardDecisionFilters) ([]BoardDecision, int, error)
	UpdateDecisionAction(ctx context.Context, orgID, userID, decisionID string, req *DecisionActionUpdate) error

	// Reports & dashboard
	ListReports(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]BoardReport, int, error)
	GenerateReport(ctx context.Context, orgID, userID string, req *GenerateBoardReportRequest) (*BoardReport, error)
	GetDashboard(ctx context.Context, orgID string) (*BoardDashboard, error)
	GetNIS2Governance(ctx context.Context, orgID string) (*NIS2GovernanceReport, error)

	// Board portal (public, token-authenticated)
	GetPortalOverview(ctx context.Context, token string) (*BoardPortalOverview, error)
	GetPortalMeetings(ctx context.Context, token string) ([]BoardMeeting, error)
	GetPortalMeetingPack(ctx context.Context, token, meetingID string) (*MeetingPackFile, error)
	GetPortalDecisions(ctx context.Context, token string) ([]BoardDecision, error)
}

// ---------- request / response types ----------

// BoardMember represents a board member.
type BoardMember struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Name           string   `json:"name" validate:"required"`
	Email          string   `json:"email" validate:"required"`
	Role           string   `json:"role"` // chairperson, vice_chair, member, secretary, observer
	Title          string   `json:"title,omitempty"`
	Department     string   `json:"department,omitempty"`
	Committees     []string `json:"committees,omitempty"`
	IsActive       bool     `json:"is_active"`
	JoinedAt       string   `json:"joined_at,omitempty"`
	PortalToken    string   `json:"portal_token,omitempty"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// BoardMeetingFilters holds filter parameters for listing board meetings.
type BoardMeetingFilters struct {
	Status string `json:"status"`
	Type   string `json:"type"`
	Search string `json:"search"`
}

// BoardMeeting represents a board meeting.
type BoardMeeting struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organization_id"`
	Title          string            `json:"title" validate:"required"`
	Description    string            `json:"description"`
	Type           string            `json:"type"` // regular, extraordinary, committee
	Status         string            `json:"status"` // scheduled, in_progress, completed, cancelled
	ScheduledAt    string            `json:"scheduled_at"`
	Location       string            `json:"location,omitempty"`
	Attendees      []string          `json:"attendees,omitempty"`
	AgendaItems    []AgendaItem      `json:"agenda_items,omitempty"`
	Minutes        string            `json:"minutes,omitempty"`
	PackGenerated  bool              `json:"pack_generated"`
	PackURL        string            `json:"pack_url,omitempty"`
	CreatedBy      string            `json:"created_by"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
}

// AgendaItem represents an item on the meeting agenda.
type AgendaItem struct {
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Presenter   string `json:"presenter,omitempty"`
	DurationMin int    `json:"duration_minutes,omitempty"`
	Type        string `json:"type,omitempty"` // information, discussion, decision
}

// MeetingPack represents a generated meeting pack.
type MeetingPack struct {
	MeetingID   string `json:"meeting_id"`
	Status      string `json:"status"` // generating, completed, error
	GeneratedAt string `json:"generated_at,omitempty"`
	URL         string `json:"url,omitempty"`
}

// MeetingPackFile holds the downloadable file data for a meeting pack.
type MeetingPackFile struct {
	MeetingID   string `json:"meeting_id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	FileData    []byte `json:"-"`
	FileURL     string `json:"file_url"`
}

// BoardDecisionFilters holds filter parameters for listing board decisions.
type BoardDecisionFilters struct {
	Status   string `json:"status"`
	Category string `json:"category"`
	Search   string `json:"search"`
}

// BoardDecision represents a board decision.
type BoardDecision struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	MeetingID      string          `json:"meeting_id,omitempty"`
	Title          string          `json:"title" validate:"required"`
	Description    string          `json:"description"`
	Category       string          `json:"category,omitempty"` // risk, compliance, budget, strategy, policy
	Status         string          `json:"status"` // proposed, approved, rejected, deferred, implemented
	DecidedAt      string          `json:"decided_at,omitempty"`
	DueDate        string          `json:"due_date,omitempty"`
	AssigneeID     string          `json:"assignee_id,omitempty"`
	Actions        []DecisionAction `json:"actions,omitempty"`
	Votes          []BoardVote     `json:"votes,omitempty"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

// DecisionAction represents an action item from a board decision.
type DecisionAction struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	AssigneeID  string `json:"assignee_id,omitempty"`
	Status      string `json:"status"` // pending, in_progress, completed, overdue
	DueDate     string `json:"due_date,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// BoardVote represents a vote on a board decision.
type BoardVote struct {
	MemberID string `json:"member_id"`
	Vote     string `json:"vote"` // approve, reject, abstain
	Comments string `json:"comments,omitempty"`
}

// DecisionActionUpdate is the payload for PUT /board/decisions/{id}/action.
type DecisionActionUpdate struct {
	ActionID    string `json:"action_id" validate:"required"`
	Status      string `json:"status,omitempty"`
	Notes       string `json:"notes,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// BoardReport represents a board report.
type BoardReport struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Title          string `json:"title"`
	Type           string `json:"type"` // compliance_summary, risk_overview, incident_report, quarterly_review
	Status         string `json:"status"` // generating, completed, error
	FileURL        string `json:"file_url,omitempty"`
	GeneratedBy    string `json:"generated_by"`
	GeneratedAt    string `json:"generated_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// GenerateBoardReportRequest is the payload for POST /board/reports/generate.
type GenerateBoardReportRequest struct {
	Type      string `json:"type" validate:"required"`
	Period    string `json:"period,omitempty"` // monthly, quarterly, annual
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

// BoardDashboard provides board governance metrics for an organization.
type BoardDashboard struct {
	TotalMembers         int            `json:"total_members"`
	ActiveMembers        int            `json:"active_members"`
	UpcomingMeetings     int            `json:"upcoming_meetings"`
	PendingDecisions     int            `json:"pending_decisions"`
	OverdueActions       int            `json:"overdue_actions"`
	MeetingsThisQuarter  int            `json:"meetings_this_quarter"`
	DecisionsByCategory  map[string]int `json:"decisions_by_category"`
	DecisionsByStatus    map[string]int `json:"decisions_by_status"`
	RecentReports        []BoardReport  `json:"recent_reports,omitempty"`
	ComplianceHighlights []string       `json:"compliance_highlights,omitempty"`
}

// NIS2GovernanceReport provides NIS2-specific governance data.
type NIS2GovernanceReport struct {
	ManagementBodyTraining   bool           `json:"management_body_training"`
	LastTrainingDate         string         `json:"last_training_date,omitempty"`
	RiskOversightInPlace     bool           `json:"risk_oversight_in_place"`
	CyberSecurityReviews     int            `json:"cyber_security_reviews_count"`
	IncidentReportingProcess bool           `json:"incident_reporting_process"`
	SupplyChainOversight     bool           `json:"supply_chain_oversight"`
	GovernanceScore          float64        `json:"governance_score"`
	Gaps                     []string       `json:"gaps,omitempty"`
	Recommendations          []string       `json:"recommendations,omitempty"`
	ComplianceStatus         map[string]string `json:"compliance_status,omitempty"`
}

// BoardPortalOverview is the overview data for the board portal.
type BoardPortalOverview struct {
	MemberName        string `json:"member_name"`
	Role              string `json:"role"`
	UpcomingMeetings  int    `json:"upcoming_meetings"`
	PendingDecisions  int    `json:"pending_decisions"`
	UnreadReports     int    `json:"unread_reports"`
	LastLoginAt       string `json:"last_login_at,omitempty"`
}

// ---------- handler ----------

// BoardHandler handles board governance management endpoints.
type BoardHandler struct {
	svc BoardService
}

// NewBoardHandler creates a new BoardHandler with the given service.
func NewBoardHandler(svc BoardService) *BoardHandler {
	return &BoardHandler{svc: svc}
}

// ListMembers handles GET /board/members.
func (h *BoardHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	members, total, err := h.svc.ListMembers(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list board members", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": members,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateMember handles POST /board/members.
func (h *BoardHandler) CreateMember(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var member BoardMember
	if err := json.NewDecoder(r.Body).Decode(&member); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if member.Name == "" || member.Email == "" {
		writeError(w, http.StatusBadRequest, "name and email are required", "")
		return
	}

	if err := h.svc.CreateMember(r.Context(), orgID, userID, &member); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create board member", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, member)
}

// UpdateMember handles PUT /board/members/{id}.
func (h *BoardHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	memberID := chi.URLParam(r, "id")
	if memberID == "" {
		writeError(w, http.StatusBadRequest, "Missing member ID", "")
		return
	}

	var member BoardMember
	if err := json.NewDecoder(r.Body).Decode(&member); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	member.ID = memberID
	member.OrganizationID = orgID

	if err := h.svc.UpdateMember(r.Context(), orgID, &member); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update board member", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, member)
}

// ListMeetings handles GET /board/meetings.
func (h *BoardHandler) ListMeetings(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := BoardMeetingFilters{
		Status: r.URL.Query().Get("status"),
		Type:   r.URL.Query().Get("type"),
		Search: r.URL.Query().Get("search"),
	}

	meetings, total, err := h.svc.ListMeetings(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list board meetings", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": meetings,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateMeeting handles POST /board/meetings.
func (h *BoardHandler) CreateMeeting(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var meeting BoardMeeting
	if err := json.NewDecoder(r.Body).Decode(&meeting); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if meeting.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	}

	if err := h.svc.CreateMeeting(r.Context(), orgID, userID, &meeting); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create board meeting", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, meeting)
}

// UpdateMeeting handles PUT /board/meetings/{id}.
func (h *BoardHandler) UpdateMeeting(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	meetingID := chi.URLParam(r, "id")
	if meetingID == "" {
		writeError(w, http.StatusBadRequest, "Missing meeting ID", "")
		return
	}

	var meeting BoardMeeting
	if err := json.NewDecoder(r.Body).Decode(&meeting); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	meeting.ID = meetingID
	meeting.OrganizationID = orgID

	if err := h.svc.UpdateMeeting(r.Context(), orgID, &meeting); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update board meeting", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, meeting)
}

// GenerateMeetingPack handles POST /board/meetings/{id}/generate-pack.
func (h *BoardHandler) GenerateMeetingPack(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	meetingID := chi.URLParam(r, "id")
	if meetingID == "" {
		writeError(w, http.StatusBadRequest, "Missing meeting ID", "")
		return
	}

	pack, err := h.svc.GenerateMeetingPack(r.Context(), orgID, userID, meetingID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate meeting pack", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, pack)
}

// DownloadMeetingPack handles GET /board/meetings/{id}/download-pack.
func (h *BoardHandler) DownloadMeetingPack(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	meetingID := chi.URLParam(r, "id")
	if meetingID == "" {
		writeError(w, http.StatusBadRequest, "Missing meeting ID", "")
		return
	}

	file, err := h.svc.DownloadMeetingPack(r.Context(), orgID, meetingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Meeting pack not found", err.Error())
		return
	}

	if len(file.FileData) > 0 {
		w.Header().Set("Content-Type", file.ContentType)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+file.FileName+"\"")
		w.WriteHeader(http.StatusOK)
		w.Write(file.FileData)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"file_url":  file.FileURL,
		"file_name": file.FileName,
	})
}

// CreateDecision handles POST /board/decisions.
func (h *BoardHandler) CreateDecision(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var decision BoardDecision
	if err := json.NewDecoder(r.Body).Decode(&decision); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if decision.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	}

	if err := h.svc.CreateDecision(r.Context(), orgID, userID, &decision); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create board decision", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, decision)
}

// ListDecisions handles GET /board/decisions.
func (h *BoardHandler) ListDecisions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := BoardDecisionFilters{
		Status:   r.URL.Query().Get("status"),
		Category: r.URL.Query().Get("category"),
		Search:   r.URL.Query().Get("search"),
	}

	decisions, total, err := h.svc.ListDecisions(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list board decisions", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": decisions,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// UpdateDecisionAction handles PUT /board/decisions/{id}/action.
func (h *BoardHandler) UpdateDecisionAction(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	decisionID := chi.URLParam(r, "id")
	if decisionID == "" {
		writeError(w, http.StatusBadRequest, "Missing decision ID", "")
		return
	}

	var req DecisionActionUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.ActionID == "" {
		writeError(w, http.StatusBadRequest, "action_id is required", "")
		return
	}

	if err := h.svc.UpdateDecisionAction(r.Context(), orgID, userID, decisionID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update decision action", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Decision action updated"})
}

// ListReports handles GET /board/reports.
func (h *BoardHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	reports, total, err := h.svc.ListReports(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list board reports", err.Error())
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

// GenerateReport handles POST /board/reports/generate.
func (h *BoardHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req GenerateBoardReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required", "")
		return
	}

	report, err := h.svc.GenerateReport(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate board report", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, report)
}

// GetBoardDashboard handles GET /board/dashboard.
func (h *BoardHandler) GetBoardDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get board dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dashboard)
}

// GetNIS2Governance handles GET /board/nis2-governance.
func (h *BoardHandler) GetNIS2Governance(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	report, err := h.svc.GetNIS2Governance(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get NIS2 governance report", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// ---------- board portal handler (public, token-authenticated) ----------

// BoardPortalHandler handles board member portal endpoints (no JWT required).
type BoardPortalHandler struct {
	svc BoardService
}

// NewBoardPortalHandler creates a new BoardPortalHandler with the given service.
func NewBoardPortalHandler(svc BoardService) *BoardPortalHandler {
	return &BoardPortalHandler{svc: svc}
}

// GetOverview handles GET /board-portal/{token}.
func (h *BoardPortalHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	overview, err := h.svc.GetPortalOverview(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "Portal not found or token expired", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, overview)
}

// GetMeetings handles GET /board-portal/{token}/meetings.
func (h *BoardPortalHandler) GetMeetings(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	meetings, err := h.svc.GetPortalMeetings(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "Portal not found or token expired", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": meetings})
}

// GetMeetingPack handles GET /board-portal/{token}/meetings/{id}/pack.
func (h *BoardPortalHandler) GetMeetingPack(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	meetingID := chi.URLParam(r, "id")
	if meetingID == "" {
		writeError(w, http.StatusBadRequest, "Missing meeting ID", "")
		return
	}

	file, err := h.svc.GetPortalMeetingPack(r.Context(), token, meetingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Meeting pack not found", err.Error())
		return
	}

	if len(file.FileData) > 0 {
		w.Header().Set("Content-Type", file.ContentType)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+file.FileName+"\"")
		w.WriteHeader(http.StatusOK)
		w.Write(file.FileData)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"file_url":  file.FileURL,
		"file_name": file.FileName,
	})
}

// GetDecisions handles GET /board-portal/{token}/decisions.
func (h *BoardPortalHandler) GetDecisions(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing portal token", "")
		return
	}

	decisions, err := h.svc.GetPortalDecisions(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "Portal not found or token expired", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": decisions})
}
