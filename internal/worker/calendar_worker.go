package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
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

	today := time.Now().UTC().Truncate(24 * time.Hour)

	rows, err := cw.pool.Query(ctx, `
		SELECT id, organization_id, title, start_date, reminder_days_before, reminders_sent, assignee_id
		FROM calendar_events
		WHERE status NOT IN ('completed', 'cancelled')
		  AND deleted_at IS NULL
		  AND reminder_days_before IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("querying calendar events for reminders: %w", err)
	}
	defer rows.Close()

	var processed, sent int
	for rows.Next() {
		var (
			id, orgID, title, assigneeID string
			startDate                     time.Time
			reminderDaysRaw               []byte
			remindersSentRaw              []byte
		)
		if err := rows.Scan(&id, &orgID, &title, &startDate, &reminderDaysRaw, &remindersSentRaw, &assigneeID); err != nil {
			log.Error().Err(err).Msg("calendar_worker: scanning event row")
			continue
		}
		processed++

		var reminderDays []int
		if err := json.Unmarshal(reminderDaysRaw, &reminderDays); err != nil {
			log.Error().Err(err).Str("event_id", id).Msg("calendar_worker: parsing reminder_days_before")
			continue
		}

		remindersSent := make(map[string]bool)
		if len(remindersSentRaw) > 0 {
			_ = json.Unmarshal(remindersSentRaw, &remindersSent)
		}

		eventDate := startDate.Truncate(24 * time.Hour)
		daysUntil := int(eventDate.Sub(today).Hours() / 24)

		for _, rd := range reminderDays {
			if daysUntil != rd {
				continue
			}
			key := fmt.Sprintf("%d", rd)
			if remindersSent[key] {
				continue
			}

			// Emit notification
			if err := cw.emitNotification(ctx, orgID, assigneeID, "calendar.reminder", map[string]interface{}{
				"event_id":   id,
				"event_title": title,
				"days_until":  rd,
				"start_date":  startDate.Format("2006-01-02"),
			}); err != nil {
				log.Error().Err(err).Str("event_id", id).Int("days", rd).Msg("calendar_worker: emitting reminder")
				continue
			}

			// Record in reminders_sent
			remindersSent[key] = true
			sentJSON, _ := json.Marshal(remindersSent)
			if _, err := cw.pool.Exec(ctx, `
				UPDATE calendar_events SET reminders_sent = $1, updated_at = NOW() WHERE id = $2
			`, sentJSON, id); err != nil {
				log.Error().Err(err).Str("event_id", id).Msg("calendar_worker: updating reminders_sent")
			}
			sent++
		}
	}

	log.Info().Int("processed", processed).Int("sent", sent).Msg("calendar_worker: reminder scheduler complete")
	return nil
}

// OverdueEscalator runs hourly. For events where status='overdue' and
// days_overdue >= escalation_days, it emits an escalation notification
// and marks escalation_sent=true.
func (cw *CalendarWorker) OverdueEscalator(ctx context.Context) error {
	log.Info().Msg("calendar_worker: running overdue escalator")

	today := time.Now().UTC().Truncate(24 * time.Hour)

	rows, err := cw.pool.Query(ctx, `
		SELECT id, organization_id, title, start_date, escalation_days, assignee_id
		FROM calendar_events
		WHERE status = 'overdue'
		  AND deleted_at IS NULL
		  AND escalation_sent = false
		  AND escalation_days IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("querying overdue events for escalation: %w", err)
	}
	defer rows.Close()

	var escalated int
	for rows.Next() {
		var (
			id, orgID, title, assigneeID string
			startDate                     time.Time
			escalationDays                int
		)
		if err := rows.Scan(&id, &orgID, &title, &startDate, &escalationDays, &assigneeID); err != nil {
			log.Error().Err(err).Msg("calendar_worker: scanning overdue event")
			continue
		}

		eventDate := startDate.Truncate(24 * time.Hour)
		daysOverdue := int(today.Sub(eventDate).Hours() / 24)

		if daysOverdue < escalationDays {
			continue
		}

		if err := cw.emitNotification(ctx, orgID, assigneeID, "calendar.escalation", map[string]interface{}{
			"event_id":     id,
			"event_title":  title,
			"days_overdue": daysOverdue,
		}); err != nil {
			log.Error().Err(err).Str("event_id", id).Msg("calendar_worker: emitting escalation")
			continue
		}

		if _, err := cw.pool.Exec(ctx, `
			UPDATE calendar_events SET escalation_sent = true, updated_at = NOW() WHERE id = $1
		`, id); err != nil {
			log.Error().Err(err).Str("event_id", id).Msg("calendar_worker: marking escalation_sent")
		}
		escalated++
	}

	log.Info().Int("escalated", escalated).Msg("calendar_worker: overdue escalator complete")
	return nil
}

// StatusUpdater runs every 30 minutes. It transitions event statuses:
//   - 'upcoming' -> 'due_today' when start_date = today
//   - 'due_today' -> 'overdue' when start_date is past and event is not completed
func (cw *CalendarWorker) StatusUpdater(ctx context.Context) error {
	log.Info().Msg("calendar_worker: running status updater")

	// Upcoming -> due_today
	res, err := cw.pool.Exec(ctx, `
		UPDATE calendar_events
		SET status = 'due_today', updated_at = NOW()
		WHERE status = 'upcoming'
		  AND deleted_at IS NULL
		  AND start_date::date = CURRENT_DATE
	`)
	if err != nil {
		return fmt.Errorf("transitioning upcoming to due_today: %w", err)
	}
	dueTodayCount := res.RowsAffected()

	// Due_today -> overdue
	res, err = cw.pool.Exec(ctx, `
		UPDATE calendar_events
		SET status = 'overdue', updated_at = NOW()
		WHERE status = 'due_today'
		  AND deleted_at IS NULL
		  AND start_date::date < CURRENT_DATE
	`)
	if err != nil {
		return fmt.Errorf("transitioning due_today to overdue: %w", err)
	}
	overdueCount := res.RowsAffected()

	log.Info().
		Int64("due_today", dueTodayCount).
		Int64("overdue", overdueCount).
		Msg("calendar_worker: status updater complete")
	return nil
}

func (cw *CalendarWorker) emitNotification(ctx context.Context, orgID, userID, eventType string, payload map[string]interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling notification payload: %w", err)
	}

	_, err = cw.pool.Exec(ctx, `
		INSERT INTO notifications (organization_id, user_id, type, payload, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, orgID, userID, eventType, payloadJSON)
	return err
}
