package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
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
	err := runForScheduledTenants(ctx, es.pool, "exception lifecycle", func(tenantCtx context.Context, organizationID string) error {
		return errors.Join(
			es.checkExpiringExceptionsForTenant(tenantCtx, organizationID),
			es.autoExpireExceptionsForTenant(tenantCtx, organizationID),
			es.checkOverdueReviewsForTenant(tenantCtx, organizationID),
		)
	})
	log.Info().Msg("exception_scheduler: daily checks completed")
	return err
}

// CheckExpiringExceptions queries approved/active exceptions whose expiry_date
// falls within the notification windows (30d, 14d, 7d, 1d) and emits events
// for each threshold.
func (es *ExceptionScheduler) CheckExpiringExceptions(ctx context.Context) error {
	return runForScheduledTenants(ctx, es.pool, "exception expiry reminders", es.checkExpiringExceptionsForTenant)
}

func (es *ExceptionScheduler) checkExpiringExceptionsForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, es.pool)
	rows, err := querier.Query(ctx, `
		SELECT ce.id, ce.organization_id, ce.exception_ref, ce.title,
		       ce.expiry_date, ce.requested_by, ce.approved_by, ce.priority
		FROM compliance_exceptions ce
		WHERE ce.organization_id = $1::uuid
		  AND ce.deleted_at IS NULL
		  AND ce.status = 'approved'
		  AND ce.expiry_date IS NOT NULL
		  AND ce.expiry_date <= CURRENT_DATE + INTERVAL '30 days'
		  AND ce.expiry_date > CURRENT_DATE
	`, organizationID)
	if err != nil {
		return fmt.Errorf("query expiring exceptions: %w", err)
	}
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
			return fmt.Errorf("scan expiring exception row: %w", err)
		}

		daysUntilExpiry := expiryDate.Sub(now).Hours() / 24

		// Emit the most urgent matching threshold.
		for _, t := range thresholds {
			if daysUntilExpiry <= t.days {
				data := map[string]interface{}{
					"exception_ref":     excRef,
					"exception_title":   title,
					"expiry_date":       expiryDate.Format("2006-01-02"),
					"days_until_expiry": fmt.Sprintf("%.0f", t.days),
					"priority":          priority,
					"requested_by":      requestedBy,
				}
				if approvedBy != nil {
					data["owner_id"] = *approvedBy
				}

				thresholdTime := expiryDate.Add(-time.Duration(t.days * float64(24*time.Hour)))
				es.bus.Publish(service.Event{
					ID:         scheduledEventID(orgID, "exception."+t.eventSuffix, excID, t.eventSuffix, expiryDate),
					Type:       "exception." + t.eventSuffix,
					Severity:   t.severity,
					OrgID:      orgID,
					EntityType: "compliance_exception",
					EntityID:   excID,
					EntityRef:  excRef,
					Data:       data,
					Timestamp:  thresholdTime,
				})
				count++
				break
			}
		}
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	log.Info().Int("notifications_sent", count).Msg("exception_scheduler: expiry notifications complete")
	return nil
}

// AutoExpireExceptions transitions approved exceptions whose expiry_date has
// passed to the 'expired' status. Each transition is recorded in the immutable
// exception_audit_trail.
func (es *ExceptionScheduler) AutoExpireExceptions(ctx context.Context) error {
	return runForScheduledTenants(ctx, es.pool, "exception auto-expiry", es.autoExpireExceptionsForTenant)
}

func (es *ExceptionScheduler) autoExpireExceptionsForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, es.pool)
	// Find all exceptions that should be expired.
	rows, err := querier.Query(ctx, `
		SELECT id, organization_id, exception_ref, title, requested_by
		FROM compliance_exceptions
		WHERE organization_id = $1::uuid
		  AND deleted_at IS NULL
		  AND status = 'approved'
		  AND expiry_date IS NOT NULL
		  AND expiry_date < CURRENT_DATE
	`, organizationID)
	if err != nil {
		return fmt.Errorf("query expired exceptions: %w", err)
	}
	type expirableException struct {
		id, orgID, ref, title, requestedBy string
	}
	var toExpire []expirableException
	for rows.Next() {
		var e expirableException
		if err := rows.Scan(&e.id, &e.orgID, &e.ref, &e.title, &e.requestedBy); err != nil {
			rows.Close()
			return fmt.Errorf("scan expired exception row: %w", err)
		}
		toExpire = append(toExpire, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	now := time.Now().UTC()
	var expired int64

	for _, e := range toExpire {
		tx, err := beginWorkerTransaction(ctx, querier)
		if err != nil {
			return fmt.Errorf("begin exception expiry transaction: %w", err)
		}

		// Update status to expired.
		tag, err := tx.Exec(ctx, `
			UPDATE compliance_exceptions
			SET status = 'expired', updated_at = NOW()
			WHERE id = $1 AND organization_id = $2::uuid
			  AND status = 'approved' AND expiry_date < CURRENT_DATE
		`, e.id, organizationID)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				log.Error().Err(rollbackErr).Str("exception_id", e.id).Msg("exception_scheduler: rollback after status update")
			}
			return fmt.Errorf("update expired exception: %w", err)
		}
		if tag.RowsAffected() == 0 {
			_ = tx.Rollback(ctx)
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
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				log.Error().Err(rollbackErr).Str("exception_id", e.id).Msg("exception_scheduler: rollback after audit insert")
			}
			return fmt.Errorf("append exception expiry audit: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit exception expiry: %w", err)
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
	return runForScheduledTenants(ctx, es.pool, "exception review reminders", es.checkOverdueReviewsForTenant)
}

func (es *ExceptionScheduler) checkOverdueReviewsForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, es.pool)
	rows, err := querier.Query(ctx, `
		SELECT ce.id, ce.organization_id, ce.exception_ref, ce.title,
		       ce.next_review_date, ce.requested_by, ce.approved_by, ce.priority
		FROM compliance_exceptions ce
		WHERE ce.organization_id = $1::uuid
		  AND ce.deleted_at IS NULL
		  AND ce.status = 'approved'
		  AND ce.next_review_date IS NOT NULL
		  AND ce.next_review_date <= CURRENT_DATE + INTERVAL '14 days'
	`, organizationID)
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
			return fmt.Errorf("scan overdue exception review row: %w", err)
		}

		daysUntilReview := nextReview.Sub(now).Hours() / 24

		var severity, eventType string
		var thresholdDays int
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
			thresholdDays = 7
		case daysUntilReview <= 14:
			severity = "low"
			eventType = "exception.review_approaching"
			thresholdDays = 14
		default:
			continue
		}

		data := map[string]interface{}{
			"exception_ref":     excRef,
			"exception_title":   title,
			"next_review_date":  nextReview.Format("2006-01-02"),
			"days_until_review": fmt.Sprintf("%d", thresholdDays),
			"priority":          priority,
			"requested_by":      requestedBy,
		}
		if approvedBy != nil {
			data["owner_id"] = *approvedBy
		}

		thresholdTime := nextReview.AddDate(0, 0, -thresholdDays)
		es.bus.Publish(service.Event{
			ID:         scheduledEventID(orgID, eventType, excID, fmt.Sprintf("%dd", thresholdDays), nextReview),
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "compliance_exception",
			EntityID:   excID,
			EntityRef:  excRef,
			Data:       data,
			Timestamp:  thresholdTime,
		})
		count++
	}

	if err := rows.Err(); err != nil {
		return err
	}

	log.Info().Int("review_notifications_sent", count).Msg("exception_scheduler: overdue review checks complete")
	return nil
}
