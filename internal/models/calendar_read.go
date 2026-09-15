package models

import "time"

const (
	CalendarMaximumWindowDays = 366
	CalendarMaximumCandidates = 2000
)

// CalendarEventQuery uses date-only values; it never requests a calendar mutation.
type CalendarEventQuery struct {
	StartDate  string    `json:"start_date,omitempty"`
	EndDate    string    `json:"end_date,omitempty"`
	EventType  string    `json:"event_type,omitempty"`
	Category   string    `json:"category,omitempty"`
	Priority   string    `json:"priority,omitempty"`
	Status     string    `json:"status,omitempty"`
	AssignedTo string    `json:"assigned_to,omitempty"`
	Search     string    `json:"search,omitempty"`
	SortBy     string    `json:"sort_by,omitempty"`
	SortOrder  string    `json:"sort_order,omitempty"`
	Page       int       `json:"page,omitempty"`
	PageSize   int       `json:"page_size,omitempty"`
	DaysAhead  int       `json:"days_ahead,omitempty"`
	AsOf       time.Time `json:"-"`
}

// CalendarAccessContext must come from verified middleware, not query parameters.
type CalendarAccessContext struct {
	Role        string
	IPAddress   string
	MFAVerified bool
	RequestID   string
}

// CalendarEventView is the bounded projection of migration035 calendar_events.
// Internal authorization anchors are deliberately not serialized.
type CalendarEventView struct {
	ID                      string    `json:"id"`
	OrganizationID          string    `json:"organization_id"`
	EventRef                string    `json:"event_ref"`
	Title                   string    `json:"title"`
	Description             string    `json:"description,omitempty"`
	EventType               string    `json:"event_type"`
	Category                string    `json:"category,omitempty"`
	Priority                string    `json:"priority,omitempty"`
	Status                  string    `json:"status"`
	SourceEntityType        string    `json:"source_entity_type"`
	SourceEntityID          string    `json:"source_entity_id"`
	SourceEntityRef         string    `json:"source_entity_ref,omitempty"`
	StartDate               string    `json:"start_date"`
	EndDate                 *string   `json:"end_date,omitempty"`
	StartTime               *string   `json:"start_time,omitempty"`
	EndTime                 *string   `json:"end_time,omitempty"`
	IsAllDay                bool      `json:"is_all_day"`
	Timezone                string    `json:"timezone"`
	IsRecurring             bool      `json:"is_recurring"`
	AssignedTo              *string   `json:"assigned_to,omitempty"`
	AssignmentAvailable     bool      `json:"assignment_available"`
	IsDueToday              bool      `json:"is_due_today"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	AuthorizationResource   string    `json:"-"`
	AuthorizationResourceID string    `json:"-"`
}

type CalendarViewMeta struct {
	StartDate      string    `json:"start_date"`
	EndDate        string    `json:"end_date"`
	AsOf           time.Time `json:"as_of"`
	DateTimezone   string    `json:"date_timezone"`
	SourceTable    string    `json:"source_table"`
	SyncVerified   bool      `json:"sync_verified"`
	CandidateLimit int       `json:"candidate_limit"`
}

type CalendarEventPage struct {
	Data       []CalendarEventView `json:"data"`
	Pagination PaginationResponse  `json:"pagination"`
	Meta       CalendarViewMeta    `json:"meta"`
}

type CalendarDayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type CalendarSummaryView struct {
	TotalEvents     int                `json:"total_events"`
	CompletedEvents int                `json:"completed_events"`
	OverdueEvents   int                `json:"overdue_events"`
	UpcomingEvents  int                `json:"upcoming_events"`
	DueTodayEvents  int                `json:"due_today_events"`
	ByEventType     map[string]int     `json:"by_event_type"`
	ByCategory      map[string]int     `json:"by_category"`
	ByPriority      map[string]int     `json:"by_priority"`
	ByStatus        map[string]int     `json:"by_status"`
	Days            []CalendarDayCount `json:"days"`
}

type CalendarSummaryResponse struct {
	Data CalendarSummaryView `json:"data"`
	Meta CalendarViewMeta    `json:"meta"`
}
