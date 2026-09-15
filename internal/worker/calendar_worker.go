package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
)

// CalendarWorker handles background tasks for calendar events including
// reminder scheduling, overdue escalation, and status transitions.
type CalendarWorker struct {
	pool *pgxpool.Pool
}

func NewCalendarWorker(pool *pgxpool.Pool) *CalendarWorker {
	return &CalendarWorker{pool: pool}
}

// ReminderScheduler runs every 15 minutes. For events where reminder_days_before
// includes today's offset and the reminder has not yet been sent, it emits a
// 'calendar.reminder' notification and records the sent reminder in the
// reminders_sent JSONB column.
func (cw *CalendarWorker) ReminderScheduler(ctx context.Context) error {
	log.Info().Msg("calendar_worker: running reminder scheduler")
	return runForScheduledTenants(ctx, cw.pool, "calendar reminders", cw.remindersForTenant)
}

func (cw *CalendarWorker) remindersForTenant(ctx context.Context, organizationID string) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	querier := database.QuerierFromContext(ctx, cw.pool)

	rows, err := querier.Query(ctx, `
		SELECT id, organization_id, title, start_date, reminder_days_before, reminders_sent, assigned_to
		FROM calendar_events
		WHERE organization_id = $1::uuid
		  AND status NOT IN ('completed', 'cancelled')
		  AND reminder_days_before IS NOT NULL
	`, organizationID)
	if err != nil {
		return fmt.Errorf("querying calendar events for reminders: %w", err)
	}
	type reminderEvent struct {
		id, orgID, title string
		assigneeID       *string
		startDate        time.Time
		reminderDays     []int32
		remindersSent    map[string]any
	}
	events := make([]reminderEvent, 0)
	for rows.Next() {
		event := reminderEvent{remindersSent: make(map[string]any)}
		var remindersSentRaw []byte
		if err := rows.Scan(&event.id, &event.orgID, &event.title, &event.startDate,
			&event.reminderDays, &remindersSentRaw, &event.assigneeID); err != nil {
			rows.Close()
			return fmt.Errorf("scanning reminder event: %w", err)
		}
		if len(remindersSentRaw) > 0 {
			if err := json.Unmarshal(remindersSentRaw, &event.remindersSent); err != nil {
				rows.Close()
				return fmt.Errorf("parse reminders sent for event %s: %w", event.id, err)
			}
		}
		events = append(events, event)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return fmt.Errorf("iterate reminder events: %w", rowErr)
	}

	var sent int
	for _, event := range events {
		eventDate := event.startDate.Truncate(24 * time.Hour)
		daysUntil := int(eventDate.Sub(today).Hours() / 24)

		for _, reminderDay := range event.reminderDays {
			rd := int(reminderDay)
			if daysUntil != rd {
				continue
			}
			key := fmt.Sprintf("%d", rd)
			if _, alreadySent := event.remindersSent[key]; alreadySent {
				continue
			}

			// Emit notification
			if event.assigneeID == nil {
				continue
			}
			if err := cw.emitNotification(ctx, querier, event.orgID, *event.assigneeID, "calendar.reminder", map[string]interface{}{
				"event_id":    event.id,
				"event_title": event.title,
				"days_until":  rd,
				"start_date":  event.startDate.Format("2006-01-02"),
			}); err != nil {
				return fmt.Errorf("emit reminder for event %s: %w", event.id, err)
			}

			// Record in reminders_sent
			event.remindersSent[key] = time.Now().UTC().Format(time.RFC3339)
			sentJSON, err := json.Marshal(event.remindersSent)
			if err != nil {
				return fmt.Errorf("encode reminders sent for event %s: %w", event.id, err)
			}
			if _, err := querier.Exec(ctx, `
				UPDATE calendar_events SET reminders_sent = $1, updated_at = NOW()
				WHERE id = $2 AND organization_id = $3::uuid
			`, sentJSON, event.id, organizationID); err != nil {
				return fmt.Errorf("update reminders sent for event %s: %w", event.id, err)
			}
			sent++
		}
	}

	log.Info().Int("processed", len(events)).Int("sent", sent).Msg("calendar_worker: reminder scheduler complete")
	return nil
}

// OverdueEscalator runs hourly. For events where status='overdue' and
// days_overdue >= escalation_days_overdue, it emits an escalation notification
// and marks escalation_sent=true.
func (cw *CalendarWorker) OverdueEscalator(ctx context.Context) error {
	log.Info().Msg("calendar_worker: running overdue escalator")
	return runForScheduledTenants(ctx, cw.pool, "calendar escalations", cw.escalationsForTenant)
}

func (cw *CalendarWorker) escalationsForTenant(ctx context.Context, organizationID string) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	querier := database.QuerierFromContext(ctx, cw.pool)

	rows, err := querier.Query(ctx, `
		SELECT id, organization_id, title, start_date, escalation_days_overdue, assigned_to
		FROM calendar_events
		WHERE organization_id = $1::uuid
		  AND status = 'overdue'
		  AND escalation_sent = false
		  AND escalation_days_overdue IS NOT NULL
	`, organizationID)
	if err != nil {
		return fmt.Errorf("querying overdue events for escalation: %w", err)
	}
	type escalationEvent struct {
		id, orgID, title string
		assigneeID       *string
		startDate        time.Time
		escalationDays   int
	}
	events := make([]escalationEvent, 0)
	for rows.Next() {
		event := escalationEvent{}
		if err := rows.Scan(&event.id, &event.orgID, &event.title, &event.startDate,
			&event.escalationDays, &event.assigneeID); err != nil {
			rows.Close()
			return fmt.Errorf("scanning overdue escalation event: %w", err)
		}
		events = append(events, event)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return fmt.Errorf("iterate overdue escalation events: %w", rowErr)
	}

	var escalated int
	for _, event := range events {
		eventDate := event.startDate.Truncate(24 * time.Hour)
		daysOverdue := int(today.Sub(eventDate).Hours() / 24)

		if daysOverdue < event.escalationDays {
			continue
		}

		if event.assigneeID == nil {
			continue
		}
		if err := cw.emitNotification(ctx, querier, event.orgID, *event.assigneeID, "calendar.escalation", map[string]interface{}{
			"event_id":     event.id,
			"event_title":  event.title,
			"days_overdue": daysOverdue,
		}); err != nil {
			return fmt.Errorf("emit escalation for event %s: %w", event.id, err)
		}

		if _, err := querier.Exec(ctx, `
			UPDATE calendar_events SET escalation_sent = true, updated_at = NOW()
			WHERE id = $1 AND organization_id = $2::uuid
		`, event.id, organizationID); err != nil {
			return fmt.Errorf("mark escalation sent for event %s: %w", event.id, err)
		}
		escalated++
	}

	log.Info().Int("escalated", escalated).Msg("calendar_worker: overdue escalator complete")
	return nil
}

// StatusUpdater runs every 30 minutes. It transitions event statuses:
// Due-today is derived from start_date by readers because it is not a persisted
// calendar status. This task transitions incomplete past events to overdue.
func (cw *CalendarWorker) StatusUpdater(ctx context.Context) error {
	log.Info().Msg("calendar_worker: running status updater")
	return runForScheduledTenants(ctx, cw.pool, "calendar status", cw.updateStatusForTenant)
}

func (cw *CalendarWorker) updateStatusForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, cw.pool)
	res, err := querier.Exec(ctx, `
		UPDATE calendar_events
		SET status = 'overdue', updated_at = NOW()
		WHERE organization_id = $1::uuid
		  AND status IN ('upcoming', 'in_progress')
		  AND start_date::date < CURRENT_DATE
	`, organizationID)
	if err != nil {
		return fmt.Errorf("transitioning past events to overdue: %w", err)
	}
	overdueCount := res.RowsAffected()

	log.Info().
		Int64("overdue", overdueCount).
		Msg("calendar_worker: status updater complete")
	return nil
}

func (cw *CalendarWorker) emitNotification(ctx context.Context, querier database.Querier, orgID, userID, eventType string, payload map[string]interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling notification payload: %w", err)
	}

	_, err = querier.Exec(ctx, `
		INSERT INTO notifications (
			organization_id, recipient_user_id, event_type, event_payload,
			channel_type, delivery_key, body_text, created_at
		)
		VALUES (
			$1::uuid, $2::uuid, $3::text, $4::jsonb, 'in_app',
			encode(digest(concat_ws('|',$1::text,$2::text,$3::text,$4::text),'sha256'),'hex'),
			$4::jsonb->>'event_title', NOW()
		)
		ON CONFLICT (organization_id, delivery_key) DO NOTHING
	`, orgID, userID, eventType, string(payloadJSON))
	return err
}
