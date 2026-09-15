package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type CalendarReadService interface {
	ListEvents(context.Context, string, string, models.CalendarEventQuery, models.CalendarAccessContext) (*models.CalendarEventPage, error)
	Upcoming(context.Context, string, string, models.CalendarEventQuery, models.CalendarAccessContext) (*models.CalendarEventPage, error)
	Overdue(context.Context, string, string, models.CalendarEventQuery, models.CalendarAccessContext) (*models.CalendarEventPage, error)
	Summary(context.Context, string, string, models.CalendarEventQuery, models.CalendarAccessContext) (*models.CalendarSummaryResponse, error)
}

var _ CalendarReadService = (*service.ComplianceCalendarService)(nil)

type CalendarReadHandler struct{ svc CalendarReadService }

func NewCalendarReadHandler(svc CalendarReadService) *CalendarReadHandler {
	return &CalendarReadHandler{svc: svc}
}
func (h *CalendarReadHandler) Ready() bool {
	if h == nil || h.svc == nil {
		return false
	}
	value := reflect.ValueOf(h.svc)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func (h *CalendarReadHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "events")
}
func (h *CalendarReadHandler) Upcoming(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "upcoming")
}
func (h *CalendarReadHandler) Overdue(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "overdue")
}
func (h *CalendarReadHandler) Summary(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "summary")
}

func (h *CalendarReadHandler) read(w http.ResponseWriter, r *http.Request, mode string) {
	orgID, userID := middleware.GetOrgIDFromContext(r.Context()), middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication is required", "")
		return
	}
	if !h.Ready() {
		writeError(w, http.StatusServiceUnavailable, "Calendar view is temporarily unavailable", "")
		return
	}
	query, err := parseCalendarReadQuery(r, mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid calendar query", "")
		return
	}
	decision, exists := middleware.GetAuthorizationDecision(r.Context())
	if !exists || !decision.Allowed {
		writeError(w, http.StatusForbidden, "Calendar access is not permitted", "")
		return
	}
	for _, obligation := range decision.Obligations {
		if obligation.Kind != service.AccessFieldVisibilityObligation {
			writeError(w, http.StatusServiceUnavailable, "Calendar access obligations cannot be safely applied", "")
			return
		}
	}
	access := models.CalendarAccessContext{Role: middleware.GetRoleFromContext(r.Context()),
		IPAddress: middleware.GetClientIPFromContext(r.Context()), MFAVerified: middleware.GetMFAVerifiedFromContext(r.Context()),
		RequestID: middleware.GetRequestIDFromContext(r.Context())}
	var result any
	switch mode {
	case "upcoming":
		result, err = h.svc.Upcoming(r.Context(), orgID, userID, query, access)
	case "overdue":
		result, err = h.svc.Overdue(r.Context(), orgID, userID, query, access)
	case "summary":
		result, err = h.svc.Summary(r.Context(), orgID, userID, query, access)
	default:
		result, err = h.svc.ListEvents(r.Context(), orgID, userID, query, access)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, service.ErrComplianceCalendarInvalid) {
			writeError(w, http.StatusBadRequest, "Invalid calendar query", "")
			return
		}
		if errors.Is(err, service.ErrComplianceCalendarScope) {
			writeError(w, http.StatusForbidden, "Calendar access is not permitted", "")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "Calendar view is temporarily unavailable", "")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "audits", result)
}

func parseCalendarReadQuery(r *http.Request, mode string) (models.CalendarEventQuery, error) {
	query := models.CalendarEventQuery{}
	if len(r.URL.RawQuery) > 8192 {
		return query, service.ErrComplianceCalendarInvalid
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return query, service.ErrComplianceCalendarInvalid
	}
	allowed := map[string]bool{"start_date": true, "end_date": true, "from_date": true, "to_date": true,
		"event_type": true, "category": true, "priority": true, "status": true, "assigned_to": true, "search": true,
		"sort_by": true, "sort_order": true, "page": mode != "summary", "page_size": mode != "summary", "days_ahead": mode == "upcoming"}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 || entries[0] == "" {
			return query, service.ErrComplianceCalendarInvalid
		}
	}
	query.StartDate, query.EndDate = values.Get("start_date"), values.Get("end_date")
	if alias := values.Get("from_date"); alias != "" {
		if query.StartDate != "" && query.StartDate != alias {
			return query, service.ErrComplianceCalendarInvalid
		}
		query.StartDate = alias
	}
	if alias := values.Get("to_date"); alias != "" {
		if query.EndDate != "" && query.EndDate != alias {
			return query, service.ErrComplianceCalendarInvalid
		}
		query.EndDate = alias
	}
	query.EventType, query.Category, query.Priority, query.Status = values.Get("event_type"), values.Get("category"), values.Get("priority"), values.Get("status")
	query.AssignedTo, query.Search, query.SortBy, query.SortOrder = values.Get("assigned_to"), values.Get("search"), values.Get("sort_by"), values.Get("sort_order")
	for key, target := range map[string]*int{"page": &query.Page, "page_size": &query.PageSize, "days_ahead": &query.DaysAhead} {
		if value := values.Get(key); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				return query, service.ErrComplianceCalendarInvalid
			}
			*target = parsed
		}
	}
	return query, nil
}
