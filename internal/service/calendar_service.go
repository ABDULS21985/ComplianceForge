package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrCalendarEventNotFound = fmt.Errorf("calendar event not found")
	ErrInvalidCalendarToken  = fmt.Errorf("invalid or expired calendar feed token")
	ErrEventAlreadyComplete  = fmt.Errorf("event is already completed")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// CalendarEvent represents a unified calendar entry sourced from any GRC module.
type CalendarEvent struct {
	ID            string  `json:"id"`
	OrgID         string  `json:"organization_id"`
	EventType     string  `json:"event_type"`     // policy_review, risk_review, audit_task, evidence_collection, vendor_review, exception_expiry, dsr_deadline, incident_followup, regulatory_deadline, bc_test, board_meeting
	Category      string  `json:"category"`        // policy, risk, audit, evidence, vendor, exception, dsr, incident, regulatory, bc, board
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	SourceType    string  `json:"source_type"`
	SourceID      string  `json:"source_id"`
	SourceRef     string  `json:"source_ref"`
	DueDate       string  `json:"due_date"`
	Priority      string  `json:"priority"`        // critical, high, medium, low
	Status        string  `json:"status"`          // pending, completed, overdue, rescheduled
	AssignedTo    *string `json:"assigned_to"`
	CompletedAt   *string `json:"completed_at"`
	CompletedBy   *string `json:"completed_by"`
	CompletedNote *string `json:"completed_note"`
	Recurrence    *string `json:"recurrence"`      // daily, weekly, monthly, quarterly, annually
	AllDay        bool    `json:"all_day"`
	Color         string  `json:"color"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// CalendarFilter defines query filters for calendar views.
type CalendarFilter struct {
	Categories []string `json:"categories"`
	Priorities []string `json:"priorities"`
	Statuses   []string `json:"statuses"`
	AssignedTo *string  `json:"assigned_to"`
}

// CalendarSummaryDay holds the event count for a single day (heatmap).
type CalendarSummaryDay struct {
	Date       string `json:"date"`
	EventCount int    `json:"event_count"`
}

// OverdueGroup groups overdue items by category.
type OverdueGroup struct {
	Category string          `json:"category"`
	Count    int             `json:"count"`
	Items    []CalendarEvent `json:"items"`
}

// SyncStatus records the last sync time for a module.
type SyncStatus struct {
	Module       string `json:"module"`
	LastSyncedAt string `json:"last_synced_at"`
	EventCount   int    `json:"event_count"`
}

// CalendarSubscription holds a user's calendar subscription preferences.
type CalendarSubscription struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	OrgID        string   `json:"organization_id"`
	Categories   []string `json:"categories"`
	Priorities   []string `json:"priorities"`
	EmailDigest  bool     `json:"email_digest"`
	DigestFreq   string   `json:"digest_frequency"` // daily, weekly
	ICalEnabled  bool     `json:"ical_enabled"`
	ICalTokenHash *string `json:"-"`
	UpdatedAt    string   `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// CalendarService manages the unified compliance calendar.
type CalendarService struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// NewCalendarService creates a CalendarService.
func NewCalendarService(pool *pgxpool.Pool, bus *EventBus) *CalendarService {
	return &CalendarService{pool: pool, bus: bus}
}

// ---------------------------------------------------------------------------
// Sync orchestration
// ---------------------------------------------------------------------------

// SyncAllEvents runs a full calendar sync across every GRC module.
func (s *CalendarService) SyncAllEvents(ctx context.Context, orgID string) error {
	start := time.Now()
	log.Info().Str("org_id", orgID).Msg("calendar: starting full sync")

	syncs := []struct {
		name string
		fn   func(context.Context, string) (int, error)
	}{
		{"policy", s.SyncPolicyEvents},
		{"risk", s.SyncRiskEvents},
		{"vendor", s.SyncVendorEvents},
		{"audit", s.SyncAuditEvents},
		{"evidence", s.SyncEvidenceEvents},
		{"exception", s.SyncExceptionEvents},
		{"dsr", s.SyncDSREvents},
		{"incident", s.SyncIncidentEvents},
		{"regulatory", s.SyncRegulatoryEvents},
		{"bc", s.SyncBCEvents},
		{"board", s.SyncBoardEvents},
	}

	for _, sy := range syncs {
		count, err := sy.fn(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Str("module", sy.name).Msg("calendar: sync failed")
			continue
		}
		if _, err2 := s.pool.Exec(ctx, `
			INSERT INTO calendar_sync_status (organization_id, module, last_synced_at, event_count)
			VALUES ($1, $2, NOW(), $3)
			ON CONFLICT (organization_id, module) DO UPDATE
			  SET last_synced_at = NOW(), event_count = $3`,
			orgID, sy.name, count); err2 != nil {
			log.Error().Err(err2).Str("module", sy.name).Msg("calendar: failed to record sync status")
		}
	}

	log.Info().Str("org_id", orgID).Dur("elapsed", time.Since(start)).Msg("calendar: full sync complete")
	return nil
}

// upsertCalendarEvent inserts or updates a calendar event using the dedup constraint.
func (s *CalendarService) upsertCalendarEvent(ctx context.Context, orgID, eventType, category, title, description, sourceType, sourceID, sourceRef, dueDate, priority string, assignedTo *string, recurrence *string) error {
	color := categoryColor(category)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO calendar_events
			(id, organization_id, event_type, category, title, description,
			 source_type, source_id, source_ref, due_date, priority, status,
			 assigned_to, recurrence, all_day, color, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
				CASE WHEN $9::date < CURRENT_DATE THEN 'overdue' ELSE 'pending' END,
				$11, $12, true, $13, NOW(), NOW())
		ON CONFLICT (organization_id, source_type, source_id) DO UPDATE
		  SET title       = EXCLUDED.title,
			  description = EXCLUDED.description,
			  due_date    = EXCLUDED.due_date,
			  priority    = EXCLUDED.priority,
			  assigned_to = EXCLUDED.assigned_to,
			  status      = CASE WHEN calendar_events.status = 'completed' THEN 'completed'
							     WHEN EXCLUDED.due_date::date < CURRENT_DATE THEN 'overdue'
							     ELSE 'pending' END,
			  updated_at  = NOW()`,
		orgID, eventType, category, title, description, sourceType, sourceID,
		sourceRef, dueDate, priority, assignedTo, recurrence, color)
	return err
}

func categoryColor(c string) string {
	colors := map[string]string{
		"policy": "#3B82F6", "risk": "#EF4444", "audit": "#8B5CF6",
		"evidence": "#10B981", "vendor": "#F59E0B", "exception": "#EC4899",
		"dsr": "#6366F1", "incident": "#DC2626", "regulatory": "#0EA5E9",
		"bc": "#14B8A6", "board": "#7C3AED",
	}
	if v, ok := colors[c]; ok {
		return v
	}
	return "#6B7280"
}

// ---------------------------------------------------------------------------
// Per-module sync methods
// ---------------------------------------------------------------------------

func (s *CalendarService) SyncPolicyEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, policy_ref, title, next_review_date, owner
		FROM policies
		WHERE organization_id = $1 AND status != 'retired' AND next_review_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		var owner *string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &owner); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "policy_review", "policy",
			fmt.Sprintf("Policy Review: %s", title), fmt.Sprintf("Scheduled review for policy %s", ref),
			"policy", id, ref, dueDate, "medium", owner, strPtr("annually"))
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncRiskEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, risk_ref, title, next_review_date, owner
		FROM risks
		WHERE organization_id = $1 AND status = 'open' AND next_review_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		var owner *string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &owner); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "risk_review", "risk",
			fmt.Sprintf("Risk Review: %s", title), fmt.Sprintf("Scheduled review for risk %s", ref),
			"risk", id, ref, dueDate, "high", owner, strPtr("quarterly"))
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncVendorEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, vendor_ref, name, next_review_date
		FROM vendors
		WHERE organization_id = $1 AND status = 'active' AND next_review_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, name, dueDate string
		if err := rows.Scan(&id, &ref, &name, &dueDate); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "vendor_review", "vendor",
			fmt.Sprintf("Vendor Review: %s", name), fmt.Sprintf("Scheduled review for vendor %s", ref),
			"vendor", id, ref, dueDate, "medium", nil, strPtr("annually"))
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncAuditEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, audit_ref, title, due_date, assigned_to
		FROM audit_findings
		WHERE organization_id = $1 AND status NOT IN ('closed','cancelled') AND due_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		var assignee *string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &assignee); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "audit_task", "audit",
			fmt.Sprintf("Audit Finding: %s", title), fmt.Sprintf("Remediation due for %s", ref),
			"audit_finding", id, ref, dueDate, "high", assignee, nil)
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncEvidenceEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, evidence_ref, title, next_collection_date, assigned_to
		FROM evidence_items
		WHERE organization_id = $1 AND status = 'active' AND next_collection_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		var assignee *string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &assignee); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "evidence_collection", "evidence",
			fmt.Sprintf("Evidence Collection: %s", title), fmt.Sprintf("Collect evidence for %s", ref),
			"evidence", id, ref, dueDate, "medium", assignee, strPtr("monthly"))
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncExceptionEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, exception_ref, title, expiry_date, requested_by
		FROM compliance_exceptions
		WHERE organization_id = $1 AND status = 'approved' AND expiry_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		var requestedBy *string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &requestedBy); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "exception_expiry", "exception",
			fmt.Sprintf("Exception Expiry: %s", title), fmt.Sprintf("Exception %s expires", ref),
			"exception", id, ref, dueDate, "high", requestedBy, nil)
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncDSREvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, request_ref, request_type, regulatory_deadline, assigned_to
		FROM data_subject_requests
		WHERE organization_id = $1 AND status NOT IN ('completed','cancelled') AND regulatory_deadline IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, reqType, dueDate string
		var assignee *string
		if err := rows.Scan(&id, &ref, &reqType, &dueDate, &assignee); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "dsr_deadline", "dsr",
			fmt.Sprintf("DSR Deadline: %s (%s)", ref, reqType), fmt.Sprintf("Regulatory deadline for DSR %s", ref),
			"dsr", id, ref, dueDate, "critical", assignee, nil)
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncIncidentEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, incident_ref, title, followup_date, assigned_to
		FROM incidents
		WHERE organization_id = $1 AND status NOT IN ('closed','cancelled') AND followup_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		var assignee *string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &assignee); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "incident_followup", "incident",
			fmt.Sprintf("Incident Follow-up: %s", title), fmt.Sprintf("Follow-up due for incident %s", ref),
			"incident", id, ref, dueDate, "high", assignee, nil)
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncRegulatoryEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, change_ref, title, effective_date
		FROM regulatory_changes
		WHERE organization_id = $1 AND status IN ('pending','in_progress') AND effective_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		if err := rows.Scan(&id, &ref, &title, &dueDate); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "regulatory_deadline", "regulatory",
			fmt.Sprintf("Regulatory Change: %s", title), fmt.Sprintf("Effective date for %s", ref),
			"regulatory_change", id, ref, dueDate, "critical", nil, nil)
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncBCEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, test_ref, title, scheduled_date, coordinator
		FROM bc_tests
		WHERE organization_id = $1 AND status IN ('scheduled','in_progress') AND scheduled_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate string
		var coordinator *string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &coordinator); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "bc_test", "bc",
			fmt.Sprintf("BC Test: %s", title), fmt.Sprintf("Business continuity test %s", ref),
			"bc_test", id, ref, dueDate, "medium", coordinator, nil)
		count++
	}
	return count, nil
}

func (s *CalendarService) SyncBoardEvents(ctx context.Context, orgID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, meeting_ref, title, scheduled_date, created_by
		FROM board_meetings
		WHERE organization_id = $1 AND status IN ('scheduled','in_progress') AND scheduled_date IS NOT NULL`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ref, title, dueDate, createdBy string
		if err := rows.Scan(&id, &ref, &title, &dueDate, &createdBy); err != nil {
			continue
		}
		_ = s.upsertCalendarEvent(ctx, orgID, "board_meeting", "board",
			fmt.Sprintf("Board Meeting: %s", title), fmt.Sprintf("Governance board meeting %s", ref),
			"board_meeting", id, ref, dueDate, "high", &createdBy, nil)
		count++
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// GetCalendarView returns calendar events within a date range with optional filters.
func (s *CalendarService) GetCalendarView(ctx context.Context, orgID, userID, startDate, endDate string, filters CalendarFilter) ([]CalendarEvent, error) {
	query := `
		SELECT id, organization_id, event_type, category, title, description,
			   source_type, source_id, source_ref, due_date, priority, status,
			   assigned_to, completed_at, completed_by, completed_note,
			   recurrence, all_day, color, created_at, updated_at
		FROM calendar_events
		WHERE organization_id = $1
		  AND due_date >= $2 AND due_date <= $3`
	args := []interface{}{orgID, startDate, endDate}
	idx := 4

	if len(filters.Categories) > 0 {
		query += fmt.Sprintf(" AND category = ANY($%d)", idx)
		args = append(args, filters.Categories)
		idx++
	}
	if len(filters.Priorities) > 0 {
		query += fmt.Sprintf(" AND priority = ANY($%d)", idx)
		args = append(args, filters.Priorities)
		idx++
	}
	if len(filters.Statuses) > 0 {
		query += fmt.Sprintf(" AND status = ANY($%d)", idx)
		args = append(args, filters.Statuses)
		idx++
	}
	if filters.AssignedTo != nil {
		query += fmt.Sprintf(" AND assigned_to = $%d", idx)
		args = append(args, *filters.AssignedTo)
		idx++
	}
	query += " ORDER BY due_date ASC, priority_ord(priority) ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("calendar: query view: %w", err)
	}
	defer rows.Close()
	return scanCalendarEvents(rows)
}

// GetUpcomingDeadlines returns the most critical upcoming items within a number of days.
func (s *CalendarService) GetUpcomingDeadlines(ctx context.Context, orgID, userID string, withinDays, limit int) ([]CalendarEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, event_type, category, title, description,
			   source_type, source_id, source_ref, due_date, priority, status,
			   assigned_to, completed_at, completed_by, completed_note,
			   recurrence, all_day, color, created_at, updated_at
		FROM calendar_events
		WHERE organization_id = $1
		  AND status IN ('pending','overdue')
		  AND due_date BETWEEN CURRENT_DATE AND CURRENT_DATE + ($2 || ' days')::interval
		  AND (assigned_to = $3 OR assigned_to IS NULL)
		ORDER BY due_date ASC, priority_ord(priority) ASC
		LIMIT $4`, orgID, withinDays, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("calendar: upcoming deadlines: %w", err)
	}
	defer rows.Close()
	return scanCalendarEvents(rows)
}

// GetOverdueItems returns overdue items grouped by category.
func (s *CalendarService) GetOverdueItems(ctx context.Context, orgID string) ([]OverdueGroup, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, event_type, category, title, description,
			   source_type, source_id, source_ref, due_date, priority, status,
			   assigned_to, completed_at, completed_by, completed_note,
			   recurrence, all_day, color, created_at, updated_at
		FROM calendar_events
		WHERE organization_id = $1 AND status = 'overdue'
		ORDER BY category, due_date ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("calendar: overdue items: %w", err)
	}
	defer rows.Close()

	events, err := scanCalendarEvents(rows)
	if err != nil {
		return nil, err
	}

	grouped := map[string][]CalendarEvent{}
	for _, e := range events {
		grouped[e.Category] = append(grouped[e.Category], e)
	}
	var result []OverdueGroup
	for cat, items := range grouped {
		result = append(result, OverdueGroup{Category: cat, Count: len(items), Items: items})
	}
	return result, nil
}

// GetCalendarSummary returns event counts per day for a given month (heatmap).
func (s *CalendarService) GetCalendarSummary(ctx context.Context, orgID, month string) ([]CalendarSummaryDay, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT due_date::date AS day, COUNT(*) AS cnt
		FROM calendar_events
		WHERE organization_id = $1
		  AND to_char(due_date, 'YYYY-MM') = $2
		GROUP BY day
		ORDER BY day`, orgID, month)
	if err != nil {
		return nil, fmt.Errorf("calendar: summary: %w", err)
	}
	defer rows.Close()

	var result []CalendarSummaryDay
	for rows.Next() {
		var d CalendarSummaryDay
		if err := rows.Scan(&d.Date, &d.EventCount); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

// CompleteEvent marks a calendar event as completed.
func (s *CalendarService) CompleteEvent(ctx context.Context, orgID, eventID, userID, notes string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET status = 'completed', completed_at = NOW(), completed_by = $1, completed_note = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND status != 'completed'`,
		userID, notes, eventID, orgID)
	if err != nil {
		return fmt.Errorf("calendar: complete event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventAlreadyComplete
	}
	log.Info().Str("event_id", eventID).Str("user_id", userID).Msg("calendar: event completed")
	s.bus.Publish(Event{
		Type: "calendar.event_completed", Severity: "low", OrgID: orgID,
		EntityType: "calendar_event", EntityID: eventID,
		Data: map[string]interface{}{"completed_by": userID}, Timestamp: time.Now(),
	})
	return nil
}

// RescheduleEvent changes the due date of a calendar event with an audit reason.
func (s *CalendarService) RescheduleEvent(ctx context.Context, orgID, eventID, newDate, reason string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET due_date = $1,
			status = CASE WHEN $1::date < CURRENT_DATE THEN 'overdue' ELSE 'pending' END,
			updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND status != 'completed'`,
		newDate, eventID, orgID)
	if err != nil {
		return fmt.Errorf("calendar: reschedule event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCalendarEventNotFound
	}
	// Audit log
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO calendar_reschedule_log (id, event_id, organization_id, new_date, reason, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW())`, eventID, orgID, newDate, reason)
	log.Info().Str("event_id", eventID).Str("new_date", newDate).Msg("calendar: event rescheduled")
	return nil
}

// ---------------------------------------------------------------------------
// iCal export
// ---------------------------------------------------------------------------

// ExportICalFeed generates an iCal .ics format feed for a user.
func (s *CalendarService) ExportICalFeed(ctx context.Context, orgID, userID, token string) (string, error) {
	// Validate token
	var tokenUserID string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM calendar_subscriptions
		WHERE organization_id = $1 AND ical_token_hash = digest($2, 'sha256')
		  AND ical_enabled = true`, orgID, token).Scan(&tokenUserID)
	if err != nil {
		return "", ErrInvalidCalendarToken
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, description, due_date, category, priority, status
		FROM calendar_events
		WHERE organization_id = $1
		  AND (assigned_to = $2 OR assigned_to IS NULL)
		  AND due_date >= CURRENT_DATE - INTERVAL '30 days'
		  AND due_date <= CURRENT_DATE + INTERVAL '365 days'
		ORDER BY due_date`, orgID, tokenUserID)
	if err != nil {
		return "", fmt.Errorf("calendar: ical query: %w", err)
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ComplianceForge//Calendar//EN\r\nCALSCALE:GREGORIAN\r\nMETHOD:PUBLISH\r\n")
	for rows.Next() {
		var id, title, desc, dueDate, category, priority, status string
		if err := rows.Scan(&id, &title, &desc, &dueDate, &category, &priority, &status); err != nil {
			continue
		}
		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString(fmt.Sprintf("UID:%s@complianceforge\r\n", id))
		dt := strings.ReplaceAll(dueDate[:10], "-", "")
		b.WriteString(fmt.Sprintf("DTSTART;VALUE=DATE:%s\r\n", dt))
		b.WriteString(fmt.Sprintf("DTEND;VALUE=DATE:%s\r\n", dt))
		b.WriteString(fmt.Sprintf("SUMMARY:[%s] %s\r\n", strings.ToUpper(priority), title))
		b.WriteString(fmt.Sprintf("DESCRIPTION:%s (Status: %s)\r\n", desc, status))
		b.WriteString(fmt.Sprintf("CATEGORIES:%s\r\n", category))
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String(), nil
}

// ---------------------------------------------------------------------------
// Sync status & trigger
// ---------------------------------------------------------------------------

// GetSyncStatus returns the last sync time per module.
func (s *CalendarService) GetSyncStatus(ctx context.Context, orgID string) ([]SyncStatus, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT module, last_synced_at, event_count
		FROM calendar_sync_status
		WHERE organization_id = $1
		ORDER BY module`, orgID)
	if err != nil {
		return nil, fmt.Errorf("calendar: sync status: %w", err)
	}
	defer rows.Close()

	var result []SyncStatus
	for rows.Next() {
		var ss SyncStatus
		if err := rows.Scan(&ss.Module, &ss.LastSyncedAt, &ss.EventCount); err != nil {
			return nil, err
		}
		result = append(result, ss)
	}
	return result, nil
}

// TriggerSync manually starts a full calendar sync.
func (s *CalendarService) TriggerSync(ctx context.Context, orgID string) error {
	log.Info().Str("org_id", orgID).Msg("calendar: manual sync triggered")
	return s.SyncAllEvents(ctx, orgID)
}

// ---------------------------------------------------------------------------
// Subscription management
// ---------------------------------------------------------------------------

// ManageSubscriptions returns or creates the user's calendar subscription preferences.
func (s *CalendarService) ManageSubscriptions(ctx context.Context, orgID, userID string) (*CalendarSubscription, error) {
	var sub CalendarSubscription
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, organization_id, categories, priorities,
			   email_digest, digest_frequency, ical_enabled, updated_at
		FROM calendar_subscriptions
		WHERE organization_id = $1 AND user_id = $2`, orgID, userID).Scan(
		&sub.ID, &sub.UserID, &sub.OrgID, &sub.Categories, &sub.Priorities,
		&sub.EmailDigest, &sub.DigestFreq, &sub.ICalEnabled, &sub.UpdatedAt)
	if err == pgx.ErrNoRows {
		err = s.pool.QueryRow(ctx, `
			INSERT INTO calendar_subscriptions
				(id, user_id, organization_id, categories, priorities, email_digest, digest_frequency, ical_enabled, updated_at)
			VALUES (gen_random_uuid(), $1, $2,
				ARRAY['policy','risk','audit','evidence','vendor','exception','dsr','incident','regulatory','bc','board'],
				ARRAY['critical','high','medium','low'], true, 'daily', false, NOW())
			RETURNING id, user_id, organization_id, categories, priorities, email_digest, digest_frequency, ical_enabled, updated_at`,
			userID, orgID).Scan(
			&sub.ID, &sub.UserID, &sub.OrgID, &sub.Categories, &sub.Priorities,
			&sub.EmailDigest, &sub.DigestFreq, &sub.ICalEnabled, &sub.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("calendar: create subscription: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("calendar: get subscription: %w", err)
	}
	return &sub, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

func scanCalendarEvents(rows pgx.Rows) ([]CalendarEvent, error) {
	var events []CalendarEvent
	for rows.Next() {
		var e CalendarEvent
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.EventType, &e.Category, &e.Title, &e.Description,
			&e.SourceType, &e.SourceID, &e.SourceRef, &e.DueDate, &e.Priority, &e.Status,
			&e.AssignedTo, &e.CompletedAt, &e.CompletedBy, &e.CompletedNote,
			&e.Recurrence, &e.AllDay, &e.Color, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("calendar: scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}
