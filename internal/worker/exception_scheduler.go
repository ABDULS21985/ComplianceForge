package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/service"
)

// ExceptionScheduler runs daily to manage the compliance exception lifecycle:
//   - Notify stakeholders when exceptions approach expiry (30d, 14d, 7d, 1d)
//   - Auto-expire exceptions whose expiry_date has passed
//   - Flag exceptions with overdue periodic reviews
//   - Record status transitions in the immutable audit trail
type ExceptionScheduler struct {
	pool *pgxpool.Pool
	bus  *service.EventBus
}

func NewExceptionScheduler(pool *pgxpool.Pool, bus *service.EventBus) *ExceptionScheduler {
	return &ExceptionScheduler{
		pool: pool,
		bus:  bus,
	}
}

// Run executes all exception lifecycle checks. Called once per day by the
// background worker.
func (es *ExceptionScheduler) Run(ctx context.Context) error {
	log.Info().Msg("exception_scheduler: starting daily checks")

	checks := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"expiry notifications", es.CheckExpiringExceptions},
		{"auto-expire", es.AutoExpireExceptions},
		{"overdue reviews", es.CheckOverdueReviews},
	}

	var firstErr error
	for _, check := range checks {
		if err := check.fn(ctx); err != nil {
			log.Error().Err(err).Str("check", check.name).Msg("exception_scheduler: check failed")
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", check.name, err)
			}
		}
	}

	log.Info().Msg("exception_scheduler: daily checks completed")
	return firstErr
}

// CheckExpiringExceptions queries approved/active exceptions whose expiry_date
// falls within the notification windows (30d, 14d, 7d, 1d) and emits events
// for each threshold.
func (es *ExceptionScheduler) CheckExpiringExceptions(ctx context.Context) error {
	rows, err := es.pool.Query(ctx, `
		SELECT ce.id, ce.organization_id, ce.exception_ref, ce.title,
		       ce.expiry_date, ce.requested_by, ce.approved_by, ce.priority
		FROM compliance_exceptions ce
		WHERE ce.deleted_at IS NULL
		  AND ce.status = 'approved'
		  AND ce.expiry_date IS NOT NULL
		  AND ce.expiry_date <= CURRENT_DATE + INTERVAL '30 days'
		  AND ce.expiry_date > CURRENT_DATE
	`)
	if err != nil {
		return fmt.Errorf("query expiring exceptions: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	type threshold struct {
		days        float64
		severity    string
		eventSuffix string
	}
	thresholds := []threshold{
		{1, "critical", "expiry_1d"},
		{7, "high", "expiry_7d"},
		{14, "medium", "expiry_14d"},
		{30, "low", "expiry_30d"},
	}

	var count int
	for rows.Next() {
		var excID, orgID, excRef, title, priority string
		var expiryDate time.Time
		var requestedBy string
		var approvedBy *string

		if err := rows.Scan(&excID, &orgID, &excRef, &title, &expiryDate, &requestedBy, &approvedBy, &priority); err != nil {
			log.Error().Err(err).Msg("exception_scheduler: scan expiring exception row")
			continue
		}

		daysUntilExpiry := expiryDate.Sub(now).Hours() / 24

		// Emit the most urgent matching threshold.
		for _, t := range thresholds {
			if daysUntilExpiry <= t.days {
				data := map[string]interface{}{
					"exception_ref":     excRef,
					"exception_title":   title,
					"expiry_date":       expiryDate.Format("2006-01-02"),
					"days_until_expiry": fmt.Sprintf("%.0f", daysUntilExpiry),
					"priority":          priority,
					"requested_by":      requestedBy,
				}
				if approvedBy != nil {
					data["owner_id"] = *approvedBy
				}

				es.bus.Publish(service.Event{
					Type:       "exception." + t.eventSuffix,
					Severity:   t.severity,
					OrgID:      orgID,
					EntityType: "compliance_exception",
					EntityID:   excID,
					EntityRef:  excRef,
					Data:       data,
					Timestamp:  now,
				})
				count++
				break
			}
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	log.Info().Int("notifications_sent", count).Msg("exception_scheduler: expiry notifications complete")
	return nil
}

// AutoExpireExceptions transitions approved exceptions whose expiry_date has
// passed to the 'expired' status. Each transition is recorded in the immutable
// exception_audit_trail.
func (es *ExceptionScheduler) AutoExpireExceptions(ctx context.Context) error {
	// Find all exceptions that should be expired.
	rows, err := es.pool.Query(ctx, `
		SELECT id, organization_id, exception_ref, title, requested_by
		FROM compliance_exceptions
		WHERE deleted_at IS NULL
		  AND status = 'approved'
		  AND expiry_date IS NOT NULL
		  AND expiry_date < CURRENT_DATE
	`)
	if err != nil {
		return fmt.Errorf("query expired exceptions: %w", err)
	}
	defer rows.Close()

	type expirableException struct {
		id, orgID, ref, title, requestedBy string
	}
	var toExpire []expirableException
	for rows.Next() {
		var e expirableException
		if err := rows.Scan(&e.id, &e.orgID, &e.ref, &e.title, &e.requestedBy); err != nil {
			log.Error().Err(err).Msg("exception_scheduler: scan expired exception row")
			continue
		}
		toExpire = append(toExpire, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().UTC()
	var expired int64

	for _, e := range toExpire {
		tx, err := es.pool.Begin(ctx)
		if err != nil {
			log.Error().Err(err).Str("exception_id", e.id).Msg("exception_scheduler: begin tx")
			continue
		}

		// Update status to expired.
		_, err = tx.Exec(ctx, `
			UPDATE compliance_exceptions
			SET status = 'expired', updated_at = NOW()
			WHERE id = $1
		`, e.id)
		if err != nil {
			tx.Rollback(ctx)
			log.Error().Err(err).Str("exception_id", e.id).Msg("exception_scheduler: update status")
			continue
		}

		// Record in the immutable audit trail.
		_, err = tx.Exec(ctx, `
			INSERT INTO exception_audit_trail
				(organization_id, exception_id, action, previous_status, new_status, details, metadata)
			VALUES ($1, $2, 'auto_expired', 'approved', 'expired',
			        'Exception automatically expired by scheduler — expiry date has passed.',
			        '{"triggered_by": "exception_scheduler"}'::jsonb)
		`, e.orgID, e.id)
		if err != nil {
			tx.Rollback(ctx)
			log.Error().Err(err).Str("exception_id", e.id).Msg("exception_scheduler: audit trail insert")
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			log.Error().Err(err).Str("exception_id", e.id).Msg("exception_scheduler: commit tx")
			continue
		}

		// Emit notification event.
		es.bus.Publish(service.Event{
			Type:       "exception.expired",
			Severity:   "high",
			OrgID:      e.orgID,
			EntityType: "compliance_exception",
			EntityID:   e.id,
			EntityRef:  e.ref,
			Data: map[string]interface{}{
				"exception_ref":   e.ref,
				"exception_title": e.title,
				"owner_id":        e.requestedBy,
			},
			Timestamp: now,
		})
		expired++
	}

	log.Info().Int64("auto_expired", expired).Msg("exception_scheduler: auto-expire complete")
	return nil
}

// CheckOverdueReviews finds approved exceptions whose next_review_date has
// passed without a corresponding review, and emits reminder events at
// escalating severity levels.
func (es *ExceptionScheduler) CheckOverdueReviews(ctx context.Context) error {
	rows, err := es.pool.Query(ctx, `
		SELECT ce.id, ce.organization_id, ce.exception_ref, ce.title,
		       ce.next_review_date, ce.requested_by, ce.approved_by, ce.priority
		FROM compliance_exceptions ce
		WHERE ce.deleted_at IS NULL
		  AND ce.status = 'approved'
		  AND ce.next_review_date IS NOT NULL
		  AND ce.next_review_date <= CURRENT_DATE + INTERVAL '14 days'
	`)
	if err != nil {
		return fmt.Errorf("query overdue exception reviews: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var count int

	for rows.Next() {
		var excID, orgID, excRef, title, priority string
		var nextReview time.Time
		var requestedBy string
		var approvedBy *string

		if err := rows.Scan(&excID, &orgID, &excRef, &title, &nextReview, &requestedBy, &approvedBy, &priority); err != nil {
			log.Error().Err(err).Msg("exception_scheduler: scan overdue review row")
			continue
		}

		daysUntilReview := nextReview.Sub(now).Hours() / 24

		var severity, eventType string
		switch {
		case daysUntilReview < -14:
			severity = "critical"
			eventType = "exception.review_severely_overdue"
		case daysUntilReview < 0:
			severity = "high"
			eventType = "exception.review_overdue"
		case daysUntilReview <= 7:
			severity = "medium"
			eventType = "exception.review_due_soon"
		case daysUntilReview <= 14:
			severity = "low"
			eventType = "exception.review_approaching"
		default:
			continue
		}

		data := map[string]interface{}{
			"exception_ref":     excRef,
			"exception_title":   title,
			"next_review_date":  nextReview.Format("2006-01-02"),
			"days_until_review": fmt.Sprintf("%.0f", daysUntilReview),
			"priority":          priority,
			"requested_by":      requestedBy,
		}
		if approvedBy != nil {
			data["owner_id"] = *approvedBy
		}

		es.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "compliance_exception",
			EntityID:   excID,
			EntityRef:  excRef,
			Data:       data,
			Timestamp:  now,
		})
		count++
	}

	if err := rows.Err(); err != nil {
		return err
	}

	log.Info().Int("review_notifications_sent", count).Msg("exception_scheduler: overdue review checks complete")
	return nil
}
