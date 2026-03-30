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

// CalendarService defines the methods required by CalendarHandler.
type CalendarService interface {
	// Events
	ListEvents(ctx context.Context, orgID string, pagination models.PaginationRequest, filters CalendarEventFilters) ([]CalendarEvent, int, error)
	GetEvent(ctx context.Context, orgID, eventID string) (*CalendarEvent, error)
	CompleteEvent(ctx context.Context, orgID, userID, eventID string) error
	RescheduleEvent(ctx context.Context, orgID, userID, eventID string, req *RescheduleEventRequest) error
	AssignEvent(ctx context.Context, orgID, userID, eventID string, req *AssignEventRequest) error
	CreateEvent(ctx context.Context, orgID, userID string, event *CalendarEvent) error

	// Views
	GetDeadlines(ctx context.Context, orgID string, filters CalendarDeadlineFilters) ([]CalendarDeadline, error)
	GetOverdueItems(ctx context.Context, orgID string) ([]CalendarOverdueItem, error)
	GetSummary(ctx context.Context, orgID string, period string) (*CalendarSummary, error)

	// Subscriptions
	GetSubscriptions(ctx context.Context, orgID, userID string) (*CalendarSubscriptions, error)
	UpdateSubscriptions(ctx context.Context, orgID, userID string, subs *CalendarSubscriptions) error

	// iCal feed (public, token-authenticated)
	GetICalFeed(ctx context.Context, token string) ([]byte, error)

	// Sync
	GetSyncStatus(ctx context.Context, orgID, userID string) (*CalendarSyncStatus, error)
	TriggerSync(ctx context.Context, orgID, userID string) (*CalendarSyncStatus, error)
}

// ---------- request / response types ----------

// CalendarEventFilters holds filter parameters for listing calendar events.
type CalendarEventFilters struct {
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	AssigneeID string `json:"assignee_id"`
	Search     string `json:"search"`
}

// CalendarEvent represents a calendar event.
type CalendarEvent struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Title          string   `json:"title" validate:"required"`
	Description    string   `json:"description,omitempty"`
	Type           string   `json:"type"` // audit, review, assessment, deadline, meeting, custom
	Status         string   `json:"status"` // scheduled, in_progress, completed, overdue, cancelled
	StartDate      string   `json:"start_date"`
	EndDate        string   `json:"end_date,omitempty"`
	DueDate        string   `json:"due_date,omitempty"`
	AllDay         bool     `json:"all_day"`
	AssigneeID     string   `json:"assignee_id,omitempty"`
	AssigneeName   string   `json:"assignee_name,omitempty"`
	EntityType     string   `json:"entity_type,omitempty"` // risk, control, policy, audit, etc.
	EntityID       string   `json:"entity_id,omitempty"`
	Recurrence     string   `json:"recurrence,omitempty"` // none, daily, weekly, monthly, quarterly, annual
	Tags           []string `json:"tags,omitempty"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// RescheduleEventRequest is the payload for PUT /calendar/events/{id}/reschedule.
type RescheduleEventRequest struct {
	StartDate string `json:"start_date" validate:"required"`
	EndDate   string `json:"end_date,omitempty"`
	DueDate   string `json:"due_date,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// AssignEventRequest is the payload for PUT /calendar/events/{id}/assign.
type AssignEventRequest struct {
	AssigneeID string `json:"assignee_id" validate:"required"`
}

// CalendarDeadlineFilters holds filter parameters for listing deadlines.
type CalendarDeadlineFilters struct {
	DaysAhead int    `json:"days_ahead"`
	Type      string `json:"type"`
	Priority  string `json:"priority"`
}

// CalendarDeadline represents an upcoming deadline.
type CalendarDeadline struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	DueDate      string `json:"due_date"`
	DaysLeft     int    `json:"days_left"`
	Priority     string `json:"priority"`
	AssigneeID   string `json:"assignee_id,omitempty"`
	AssigneeName string `json:"assignee_name,omitempty"`
	EntityType   string `json:"entity_type,omitempty"`
	EntityID     string `json:"entity_id,omitempty"`
	Status       string `json:"status"`
}

// CalendarOverdueItem represents an overdue calendar item.
type CalendarOverdueItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	DueDate      string `json:"due_date"`
	DaysOverdue  int    `json:"days_overdue"`
	AssigneeID   string `json:"assignee_id,omitempty"`
	AssigneeName string `json:"assignee_name,omitempty"`
	EntityType   string `json:"entity_type,omitempty"`
	EntityID     string `json:"entity_id,omitempty"`
}

// CalendarSummary provides a summary of calendar activity.
type CalendarSummary struct {
	Period           string         `json:"period"`
	TotalEvents      int            `json:"total_events"`
	CompletedEvents  int            `json:"completed_events"`
	OverdueEvents    int            `json:"overdue_events"`
	UpcomingDeadlines int           `json:"upcoming_deadlines"`
	EventsByType     map[string]int `json:"events_by_type"`
	EventsByStatus   map[string]int `json:"events_by_status"`
}

// CalendarSubscriptions holds user calendar subscription preferences.
type CalendarSubscriptions struct {
	EventTypes     []string `json:"event_types"`
	EntityTypes    []string `json:"entity_types"`
	Reminders      bool     `json:"reminders"`
	ReminderBefore string   `json:"reminder_before,omitempty"` // e.g. "1d", "2h"
	ICalEnabled    bool     `json:"ical_enabled"`
	ICalToken      string   `json:"ical_token,omitempty"`
}

// CalendarSyncStatus holds the status of external calendar sync.
type CalendarSyncStatus struct {
	Provider     string `json:"provider,omitempty"` // google, outlook, ical
	Status       string `json:"status"` // connected, syncing, error, disconnected
	LastSyncAt   string `json:"last_sync_at,omitempty"`
	NextSyncAt   string `json:"next_sync_at,omitempty"`
	EventsSynced int    `json:"events_synced"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ---------- handler ----------

// CalendarHandler handles calendar and scheduling endpoints.
type CalendarHandler struct {
	svc CalendarService
}

// NewCalendarHandler creates a new CalendarHandler with the given service.
func NewCalendarHandler(svc CalendarService) *CalendarHandler {
	return &CalendarHandler{svc: svc}
}

// ListEvents handles GET /calendar/events.
func (h *CalendarHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := CalendarEventFilters{
		StartDate:  r.URL.Query().Get("start_date"),
		EndDate:    r.URL.Query().Get("end_date"),
		Type:       r.URL.Query().Get("type"),
		Status:     r.URL.Query().Get("status"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
		Search:     r.URL.Query().Get("search"),
	}

	events, total, err := h.svc.ListEvents(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list calendar events", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": events,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetEvent handles GET /calendar/events/{id}.
func (h *CalendarHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "Missing event ID", "")
		return
	}

	event, err := h.svc.GetEvent(r.Context(), orgID, eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Event not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, event)
}

// CompleteEvent handles PUT /calendar/events/{id}/complete.
func (h *CalendarHandler) CompleteEvent(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "Missing event ID", "")
		return
	}

	if err := h.svc.CompleteEvent(r.Context(), orgID, userID, eventID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to complete event", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Event completed"})
}

// RescheduleEvent handles PUT /calendar/events/{id}/reschedule.
func (h *CalendarHandler) RescheduleEvent(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "Missing event ID", "")
		return
	}

	var req RescheduleEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.StartDate == "" {
		writeError(w, http.StatusBadRequest, "start_date is required", "")
		return
	}

	if err := h.svc.RescheduleEvent(r.Context(), orgID, userID, eventID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reschedule event", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Event rescheduled"})
}

// AssignEvent handles PUT /calendar/events/{id}/assign.
func (h *CalendarHandler) AssignEvent(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "Missing event ID", "")
		return
	}

	var req AssignEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.AssigneeID == "" {
		writeError(w, http.StatusBadRequest, "assignee_id is required", "")
		return
	}

	if err := h.svc.AssignEvent(r.Context(), orgID, userID, eventID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to assign event", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Event assigned"})
}

// CreateEvent handles POST /calendar/events.
func (h *CalendarHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var event CalendarEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if event.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	}

	event.OrganizationID = orgID
	event.CreatedBy = userID

	if err := h.svc.CreateEvent(r.Context(), orgID, userID, &event); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create event", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

// GetDeadlines handles GET /calendar/deadlines.
func (h *CalendarHandler) GetDeadlines(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	daysAhead := 30
	if d := r.URL.Query().Get("days_ahead"); d != "" {
		// best-effort parse; keep default on failure
		var parsed int
		if _, err := json.Number(d).Int64(); err == nil {
			parsed = int(json.Number(d).String()[0] - '0')
		}
		if parsed > 0 {
			daysAhead = parsed
		}
	}

	filters := CalendarDeadlineFilters{
		DaysAhead: daysAhead,
		Type:      r.URL.Query().Get("type"),
		Priority:  r.URL.Query().Get("priority"),
	}

	deadlines, err := h.svc.GetDeadlines(r.Context(), orgID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get deadlines", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": deadlines})
}

// GetOverdue handles GET /calendar/overdue.
func (h *CalendarHandler) GetOverdue(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	items, err := h.svc.GetOverdueItems(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get overdue items", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": items})
}

// GetSummary handles GET /calendar/summary.
func (h *CalendarHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	summary, err := h.svc.GetSummary(r.Context(), orgID, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get calendar summary", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// GetSubscriptions handles GET /calendar/subscriptions.
func (h *CalendarHandler) GetSubscriptions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	subs, err := h.svc.GetSubscriptions(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get subscriptions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, subs)
}

// UpdateSubscriptions handles PUT /calendar/subscriptions.
func (h *CalendarHandler) UpdateSubscriptions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var subs CalendarSubscriptions
	if err := json.NewDecoder(r.Body).Decode(&subs); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.UpdateSubscriptions(r.Context(), orgID, userID, &subs); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update subscriptions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Subscriptions updated"})
}

// GetICalFeed handles GET /calendar/ical/{token} (public, no JWT).
func (h *CalendarHandler) GetICalFeed(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Missing iCal token", "")
		return
	}

	data, err := h.svc.GetICalFeed(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "Calendar feed not found or token expired", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"calendar.ics\"")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GetSyncStatus handles GET /calendar/sync/status.
func (h *CalendarHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	status, err := h.svc.GetSyncStatus(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get sync status", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// TriggerSync handles POST /calendar/sync/trigger.
func (h *CalendarHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	status, err := h.svc.TriggerSync(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to trigger calendar sync", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, status)
}
