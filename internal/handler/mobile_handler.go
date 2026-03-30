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

// PushService defines the methods required by MobileHandler.
type PushService interface {
	// Mobile dashboard
	GetMobileDashboard(ctx context.Context, orgID, userID string) (*MobileDashboard, error)

	// Approvals
	ListPendingApprovals(ctx context.Context, orgID, userID string, pagination models.PaginationRequest) ([]MobileApproval, int, error)
	ApproveItem(ctx context.Context, orgID, userID, approvalID string, req *ApprovalActionRequest) error
	RejectItem(ctx context.Context, orgID, userID, approvalID string, req *ApprovalActionRequest) error

	// Quick views
	GetActiveIncidents(ctx context.Context, orgID string) ([]MobileIncident, error)
	GetUpcomingDeadlines(ctx context.Context, orgID, userID string) ([]MobileDeadline, error)
	GetRecentActivity(ctx context.Context, orgID, userID string, pagination models.PaginationRequest) ([]MobileActivityItem, int, error)

	// Push notifications
	RegisterDevice(ctx context.Context, orgID, userID string, req *PushRegistration) error
	UnregisterDevice(ctx context.Context, orgID, userID string) error
	GetPushPreferences(ctx context.Context, orgID, userID string) (*PushPreferences, error)
	UpdatePushPreferences(ctx context.Context, orgID, userID string, prefs *PushPreferences) error
}

// ---------- request / response types ----------

// MobileDashboard provides a compact dashboard for mobile clients.
type MobileDashboard struct {
	ComplianceScore    float64            `json:"compliance_score"`
	OpenRisks          int                `json:"open_risks"`
	CriticalRisks      int                `json:"critical_risks"`
	PendingApprovals   int                `json:"pending_approvals"`
	ActiveIncidents    int                `json:"active_incidents"`
	UpcomingDeadlines  int                `json:"upcoming_deadlines"`
	OverdueItems       int                `json:"overdue_items"`
	RecentAlerts       []MobileAlert      `json:"recent_alerts,omitempty"`
	QuickStats         map[string]int     `json:"quick_stats,omitempty"`
}

// MobileAlert represents a compact alert for mobile display.
type MobileAlert struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // incident, risk, deadline, approval
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	CreatedAt string `json:"created_at"`
}

// MobileApproval represents a pending approval item for mobile.
type MobileApproval struct {
	ID           string `json:"id"`
	Type         string `json:"type"` // policy, exception, vendor, workflow
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	RequestedBy  string `json:"requested_by"`
	RequestedAt  string `json:"requested_at"`
	DueDate      string `json:"due_date,omitempty"`
	Priority     string `json:"priority,omitempty"`
	EntityType   string `json:"entity_type,omitempty"`
	EntityID     string `json:"entity_id,omitempty"`
}

// ApprovalActionRequest is the payload for approve/reject actions.
type ApprovalActionRequest struct {
	Comments string `json:"comments,omitempty"`
}

// MobileIncident represents a compact incident for mobile display.
type MobileIncident struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	ReportedAt  string `json:"reported_at"`
	AssigneeName string `json:"assignee_name,omitempty"`
}

// MobileDeadline represents an upcoming deadline for mobile display.
type MobileDeadline struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	DueDate    string `json:"due_date"`
	DaysLeft   int    `json:"days_left"`
	Priority   string `json:"priority"`
	EntityType string `json:"entity_type,omitempty"`
	EntityID   string `json:"entity_id,omitempty"`
}

// MobileActivityItem represents a compact activity item for mobile.
type MobileActivityItem struct {
	ID          string `json:"id"`
	ActorName   string `json:"actor_name"`
	Action      string `json:"action"`
	EntityType  string `json:"entity_type"`
	EntityTitle string `json:"entity_title"`
	CreatedAt   string `json:"created_at"`
}

// PushRegistration is the payload for POST /mobile/push/register.
type PushRegistration struct {
	DeviceToken string `json:"device_token" validate:"required"`
	Platform    string `json:"platform" validate:"required"` // ios, android
	DeviceName  string `json:"device_name,omitempty"`
}

// PushPreferences holds push notification preferences.
type PushPreferences struct {
	Enabled          bool     `json:"enabled"`
	IncidentAlerts   bool     `json:"incident_alerts"`
	ApprovalRequests bool     `json:"approval_requests"`
	DeadlineReminders bool    `json:"deadline_reminders"`
	RiskAlerts       bool     `json:"risk_alerts"`
	CommentMentions  bool     `json:"comment_mentions"`
	QuietHoursStart  string   `json:"quiet_hours_start,omitempty"` // e.g. "22:00"
	QuietHoursEnd    string   `json:"quiet_hours_end,omitempty"`   // e.g. "07:00"
	MutedEntities    []string `json:"muted_entities,omitempty"`
}

// ---------- handler ----------

// MobileHandler handles mobile-specific endpoints.
type MobileHandler struct {
	svc PushService
}

// NewMobileHandler creates a new MobileHandler with the given service.
func NewMobileHandler(svc PushService) *MobileHandler {
	return &MobileHandler{svc: svc}
}

// GetDashboard handles GET /mobile/dashboard.
func (h *MobileHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	dashboard, err := h.svc.GetMobileDashboard(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get mobile dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dashboard)
}

// ListApprovals handles GET /mobile/approvals.
func (h *MobileHandler) ListApprovals(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	pagination := parsePagination(r)

	approvals, total, err := h.svc.ListPendingApprovals(r.Context(), orgID, userID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list approvals", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": approvals,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// ApproveItem handles POST /mobile/approvals/{id}/approve.
func (h *MobileHandler) ApproveItem(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	approvalID := chi.URLParam(r, "id")
	if approvalID == "" {
		writeError(w, http.StatusBadRequest, "Missing approval ID", "")
		return
	}

	var req ApprovalActionRequest
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	if err := h.svc.ApproveItem(r.Context(), orgID, userID, approvalID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to approve item", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Item approved"})
}

// RejectItem handles POST /mobile/approvals/{id}/reject.
func (h *MobileHandler) RejectItem(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	approvalID := chi.URLParam(r, "id")
	if approvalID == "" {
		writeError(w, http.StatusBadRequest, "Missing approval ID", "")
		return
	}

	var req ApprovalActionRequest
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	if err := h.svc.RejectItem(r.Context(), orgID, userID, approvalID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reject item", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Item rejected"})
}

// GetActiveIncidents handles GET /mobile/incidents/active.
func (h *MobileHandler) GetActiveIncidents(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	incidents, err := h.svc.GetActiveIncidents(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get active incidents", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": incidents})
}

// GetDeadlines handles GET /mobile/deadlines.
func (h *MobileHandler) GetDeadlines(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	deadlines, err := h.svc.GetUpcomingDeadlines(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get deadlines", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": deadlines})
}

// GetActivity handles GET /mobile/activity.
func (h *MobileHandler) GetActivity(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	pagination := parsePagination(r)

	activity, total, err := h.svc.GetRecentActivity(r.Context(), orgID, userID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get activity", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": activity,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// RegisterDevice handles POST /mobile/push/register.
func (h *MobileHandler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req PushRegistration
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.DeviceToken == "" || req.Platform == "" {
		writeError(w, http.StatusBadRequest, "device_token and platform are required", "")
		return
	}

	if err := h.svc.RegisterDevice(r.Context(), orgID, userID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to register device", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "Device registered"})
}

// UnregisterDevice handles DELETE /mobile/push/unregister.
func (h *MobileHandler) UnregisterDevice(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	if err := h.svc.UnregisterDevice(r.Context(), orgID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to unregister device", err.Error())
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// GetPushPreferences handles GET /mobile/push/preferences.
func (h *MobileHandler) GetPushPreferences(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	prefs, err := h.svc.GetPushPreferences(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get push preferences", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, prefs)
}

// UpdatePushPreferences handles PUT /mobile/push/preferences.
func (h *MobileHandler) UpdatePushPreferences(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var prefs PushPreferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.UpdatePushPreferences(r.Context(), orgID, userID, &prefs); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update push preferences", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Push preferences updated"})
}
