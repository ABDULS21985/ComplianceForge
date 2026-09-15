package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrComplianceCalendarInvalid     = errors.New("invalid calendar query")
	ErrComplianceCalendarScope       = errors.New("calendar subject is unavailable")
	ErrComplianceCalendarUnavailable = errors.New("calendar view is temporarily unavailable")
)

type ComplianceCalendarOption func(*ComplianceCalendarService)

func WithComplianceCalendarClock(now func() time.Time) ComplianceCalendarOption {
	return func(s *ComplianceCalendarService) { s.now = now }
}

type ComplianceCalendarService struct {
	store      repository.CalendarReadRepository
	authorizer authz.Authorizer
	now        func() time.Time
}

func NewComplianceCalendarService(store repository.CalendarReadRepository, authorizer authz.Authorizer, options ...ComplianceCalendarOption) (*ComplianceCalendarService, error) {
	if interfaceValueIsNil(store) || interfaceValueIsNil(authorizer) {
		return nil, errors.New("calendar repository and composite authorizer are required")
	}
	s := &ComplianceCalendarService{store: store, authorizer: authorizer, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	if s.now == nil {
		return nil, errors.New("calendar clock is required")
	}
	return s, nil
}

func (s *ComplianceCalendarService) ListEvents(ctx context.Context, orgID, userID string, query models.CalendarEventQuery, access models.CalendarAccessContext) (*models.CalendarEventPage, error) {
	return s.list(ctx, orgID, userID, query, access, "events")
}

func (s *ComplianceCalendarService) Upcoming(ctx context.Context, orgID, userID string, query models.CalendarEventQuery, access models.CalendarAccessContext) (*models.CalendarEventPage, error) {
	return s.list(ctx, orgID, userID, query, access, "upcoming")
}

func (s *ComplianceCalendarService) Overdue(ctx context.Context, orgID, userID string, query models.CalendarEventQuery, access models.CalendarAccessContext) (*models.CalendarEventPage, error) {
	return s.list(ctx, orgID, userID, query, access, "overdue")
}

func (s *ComplianceCalendarService) list(ctx context.Context, orgID, userID string, query models.CalendarEventQuery, access models.CalendarAccessContext, mode string) (*models.CalendarEventPage, error) {
	items, normalized, err := s.visible(ctx, orgID, userID, query, access, mode)
	if err != nil {
		return nil, err
	}
	sortCalendarEvents(items, normalized.SortBy, normalized.SortOrder)
	total := len(items)
	start := (normalized.Page - 1) * normalized.PageSize
	if start > total {
		start = total
	}
	end := start + normalized.PageSize
	if end > total {
		end = total
	}
	page := append(make([]models.CalendarEventView, 0, end-start), items[start:end]...)
	return &models.CalendarEventPage{Data: page,
		Pagination: models.PaginationResponse{Page: normalized.Page, PageSize: normalized.PageSize,
			TotalItems: total, TotalPages: (total + normalized.PageSize - 1) / normalized.PageSize},
		Meta: calendarViewMeta(normalized)}, nil
}

func (s *ComplianceCalendarService) Summary(ctx context.Context, orgID, userID string, query models.CalendarEventQuery, access models.CalendarAccessContext) (*models.CalendarSummaryResponse, error) {
	items, normalized, err := s.visible(ctx, orgID, userID, query, access, "events")
	if err != nil {
		return nil, err
	}
	result := models.CalendarSummaryView{TotalEvents: len(items), ByEventType: map[string]int{},
		ByCategory: map[string]int{}, ByPriority: map[string]int{}, ByStatus: map[string]int{},
		Days: make([]models.CalendarDayCount, 0)}
	dayCounts := map[string]int{}
	for _, event := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result.ByEventType[event.EventType]++
		result.ByCategory[event.Category]++
		result.ByPriority[event.Priority]++
		result.ByStatus[event.Status]++
		dayCounts[event.StartDate]++
		if event.Status == "completed" {
			result.CompletedEvents++
		}
		if event.Status == "overdue" {
			result.OverdueEvents++
		}
		if event.IsDueToday {
			result.DueTodayEvents++
		}
		if calendarOpen(event.Status) && event.StartDate > normalized.AsOf.Format("2006-01-02") {
			result.UpcomingEvents++
		}
	}
	first, _ := time.Parse("2006-01-02", normalized.StartDate)
	last, _ := time.Parse("2006-01-02", normalized.EndDate)
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		result.Days = append(result.Days, models.CalendarDayCount{Date: date, Count: dayCounts[date]})
	}
	return &models.CalendarSummaryResponse{Data: result, Meta: calendarViewMeta(normalized)}, nil
}

func (s *ComplianceCalendarService) visible(ctx context.Context, orgID, userID string, query models.CalendarEventQuery, access models.CalendarAccessContext, mode string) ([]models.CalendarEventView, models.CalendarEventQuery, error) {
	if err := ctx.Err(); err != nil {
		return nil, query, err
	}
	if s == nil || interfaceValueIsNil(s.store) || interfaceValueIsNil(s.authorizer) || s.now == nil {
		return nil, query, ErrComplianceCalendarUnavailable
	}
	if !calendarCanonicalUUID(orgID) || !calendarCanonicalUUID(userID) {
		return nil, query, ErrComplianceCalendarInvalid
	}
	normalized, err := normalizeCalendarQuery(query, s.now().UTC(), mode)
	if err != nil {
		return nil, query, err
	}
	items, err := s.store.LoadCandidates(ctx, orgID, userID, normalized)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, query, err
		}
		if errors.Is(err, repository.ErrCalendarReadScope) {
			return nil, query, ErrComplianceCalendarScope
		}
		return nil, query, ErrComplianceCalendarUnavailable
	}
	if len(items) > models.CalendarMaximumCandidates {
		return nil, query, ErrComplianceCalendarUnavailable
	}
	visible := make([]models.CalendarEventView, 0, len(items))
	decisions := map[string]authz.Decision{}
	today := normalized.AsOf.Format("2006-01-02")
	for _, event := range items {
		if err := ctx.Err(); err != nil {
			return nil, query, err
		}
		if event.OrganizationID != orgID || !calendarCanonicalUUID(event.ID) ||
			!calendarCanonicalUUID(event.SourceEntityID) || !calendarCanonicalUUID(event.AuthorizationResourceID) ||
			!calendarAuthorizationResource(event.AuthorizationResource) || !calendarDate(event.StartDate) ||
			!calendarOneOf(event.Status, "upcoming", "in_progress", "completed", "overdue", "cancelled", "snoozed") {
			return nil, query, ErrComplianceCalendarUnavailable
		}
		key := event.AuthorizationResource + ":" + event.AuthorizationResourceID
		decision, cached := decisions[key]
		if !cached {
			decision, err = s.authorizer.Authorize(ctx, authz.Request{SubjectID: userID, OrganizationID: orgID,
				Role: access.Role, Resource: event.AuthorizationResource, ResourceID: event.AuthorizationResourceID,
				Action: "read", IPAddress: access.IPAddress, MFAVerified: access.MFAVerified,
				Attributes: map[string]any{"request_id": access.RequestID, "calendar_projection": true}})
			if err != nil {
				if ctx.Err() != nil {
					return nil, query, ctx.Err()
				}
				return nil, query, ErrComplianceCalendarUnavailable
			}
			decisions[key] = decision
		}
		if !calendarProjectionAllowed(decision, event.AuthorizationResource) {
			continue
		}
		// Keep the migrated state vocabulary. due_today is a boolean view flag,
		// not a fabricated persisted state or a mutation performed by GET.
		if calendarOpen(event.Status) {
			if event.StartDate < today {
				event.Status = "overdue"
			}
			if event.StartDate >= today && event.Status == "overdue" {
				event.Status = "upcoming"
			}
		}
		event.IsDueToday = calendarOpen(event.Status) && event.StartDate == today
		if mode == "upcoming" && !calendarOpen(event.Status) {
			continue
		}
		if mode == "overdue" && event.Status != "overdue" {
			continue
		}
		if normalized.Status != "" && normalized.Status != event.Status {
			continue
		}
		visible = append(visible, event)
	}
	return visible, normalized, nil
}

func calendarProjectionAllowed(decision authz.Decision, resource string) bool {
	if !decision.Allowed {
		return false
	}
	for _, obligation := range decision.Obligations {
		if obligation.Kind != AccessFieldVisibilityObligation {
			return false
		}
	}
	fields, err := AccessFieldsFromObligations(decision.Obligations, resource)
	if err != nil {
		return false
	}
	// Calendar text is denormalized from source records. We cannot prove that a
	// masked/hidden source field did not contribute to title, description or
	// source_ref. Exclude that source entirely rather than reuse unmasked text.
	for _, field := range fields {
		if field.Visibility != models.AccessFieldVisible {
			return false
		}
	}
	return true
}

func normalizeCalendarQuery(query models.CalendarEventQuery, now time.Time, mode string) (models.CalendarEventQuery, error) {
	query.AsOf = now
	if query.DaysAhead != 0 && (query.StartDate != "" || query.EndDate != "") {
		return query, ErrComplianceCalendarInvalid
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.Page < 1 || query.Page > 100000 || query.PageSize < 1 || query.PageSize > 100 {
		return query, ErrComplianceCalendarInvalid
	}
	if query.DaysAhead == 0 && mode == "upcoming" {
		query.DaysAhead = 30
	}
	if query.DaysAhead < 0 || query.DaysAhead > 365 || query.DaysAhead != 0 && mode != "upcoming" {
		return query, ErrComplianceCalendarInvalid
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if (query.StartDate == "") != (query.EndDate == "") {
		return query, ErrComplianceCalendarInvalid
	}
	if query.StartDate == "" {
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1)
		if mode == "upcoming" {
			first, last = today, today.AddDate(0, 0, query.DaysAhead)
		}
		if mode == "overdue" {
			first, last = today.AddDate(0, 0, -90), today.AddDate(0, 0, -1)
		}
		query.StartDate, query.EndDate = first.Format("2006-01-02"), last.Format("2006-01-02")
	}
	if !calendarDate(query.StartDate) || !calendarDate(query.EndDate) {
		return query, ErrComplianceCalendarInvalid
	}
	first, _ := time.Parse("2006-01-02", query.StartDate)
	last, _ := time.Parse("2006-01-02", query.EndDate)
	if last.Before(first) || last.Sub(first)/(24*time.Hour) >= models.CalendarMaximumWindowDays {
		return query, ErrComplianceCalendarInvalid
	}
	if mode == "overdue" && !last.Before(today) || mode == "upcoming" && first.Before(today) {
		return query, ErrComplianceCalendarInvalid
	}
	if query.AssignedTo != "" && !calendarCanonicalUUID(query.AssignedTo) {
		return query, ErrComplianceCalendarInvalid
	}
	if query.SortBy == "" {
		query.SortBy = "start_date"
	}
	if query.SortOrder == "" {
		query.SortOrder = "asc"
	}
	if !calendarOneOf(query.SortBy, "start_date", "title", "priority", "updated_at") || !calendarOneOf(query.SortOrder, "asc", "desc") {
		return query, ErrComplianceCalendarInvalid
	}
	if query.EventType != "" && !calendarOneOf(query.EventType,
		"policy_review", "policy_expiry", "policy_approval_due", "control_assessment", "control_testing", "control_review",
		"risk_review", "risk_reassessment", "risk_treatment_due", "audit_start", "audit_fieldwork", "audit_report_due", "audit_followup",
		"certification_renewal", "certification_audit", "regulatory_filing", "regulatory_deadline", "regulatory_effective_date",
		"training_due", "training_renewal", "vendor_review", "vendor_contract_renewal", "vendor_assessment_due", "incident_followup", "incident_review", "board_meeting", "committee_meeting", "custom") {
		return query, ErrComplianceCalendarInvalid
	}
	if query.Priority != "" && !calendarOneOf(query.Priority, "critical", "high", "medium", "low") ||
		query.Status != "" && !calendarOneOf(query.Status, "upcoming", "in_progress", "completed", "overdue", "cancelled", "snoozed") {
		return query, ErrComplianceCalendarInvalid
	}
	if query.Category != "" && !calendarOneOf(query.Category, "policy", "risk", "audit", "control", "evidence", "vendor", "incident", "asset", "custom") {
		return query, ErrComplianceCalendarInvalid
	}
	if !utf8.ValidString(query.Search) || utf8.RuneCountInString(query.Search) > 200 || query.Search != strings.TrimSpace(query.Search) {
		return query, ErrComplianceCalendarInvalid
	}
	for _, character := range query.Search {
		if unicode.IsControl(character) {
			return query, ErrComplianceCalendarInvalid
		}
	}
	return query, nil
}

func calendarDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && len(value) == 10 && parsed.Format("2006-01-02") == value && parsed.Year() >= 1900 && parsed.Year() <= 2100
}

func calendarCanonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}

func calendarOneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func calendarAuthorizationResource(resource string) bool {
	return calendarOneOf(resource, "risks", "policies", "audits", "vendors", "incidents", "controls", "assets")
}
func calendarOpen(status string) bool {
	return calendarOneOf(status, "upcoming", "in_progress", "overdue")
}

func calendarViewMeta(query models.CalendarEventQuery) models.CalendarViewMeta {
	return models.CalendarViewMeta{StartDate: query.StartDate, EndDate: query.EndDate, AsOf: query.AsOf,
		DateTimezone: "UTC", SourceTable: "calendar_events", SyncVerified: false, CandidateLimit: models.CalendarMaximumCandidates}
}

func sortCalendarEvents(items []models.CalendarEventView, column, order string) {
	rank := func(priority string) int {
		for index, value := range []string{"critical", "high", "medium", "low"} {
			if priority == value {
				return index
			}
		}
		return 4
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		comparison := 0
		switch column {
		case "title":
			comparison = strings.Compare(left.Title, right.Title)
		case "priority":
			comparison = rank(left.Priority) - rank(right.Priority)
		case "updated_at":
			if left.UpdatedAt.Before(right.UpdatedAt) {
				comparison = -1
			}
			if left.UpdatedAt.After(right.UpdatedAt) {
				comparison = 1
			}
		default:
			comparison = strings.Compare(left.StartDate, right.StartDate)
		}
		if comparison == 0 {
			return left.ID < right.ID
		}
		if order == "desc" {
			return comparison > 0
		}
		return comparison < 0
	})
}
