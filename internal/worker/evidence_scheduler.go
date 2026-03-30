package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/service"
)

// EvidenceScheduler runs daily to manage evidence collection lifecycle:
//   - Remind stakeholders when evidence collection is approaching due
//   - Mark expired evidence (valid_until has passed) as no longer current
//   - Detect evidence collection configs with overdue next_collection_at
//   - Flag controls that have no current evidence
type EvidenceScheduler struct {
	pool *pgxpool.Pool
	bus  *service.EventBus
}

func NewEvidenceScheduler(pool *pgxpool.Pool, bus *service.EventBus) *EvidenceScheduler {
	return &EvidenceScheduler{
		pool: pool,
		bus:  bus,
	}
}

// Run executes all evidence lifecycle checks. Called once per day by the
// background worker.
func (es *EvidenceScheduler) Run(ctx context.Context) error {
	log.Info().Msg("evidence_scheduler: starting daily checks")

	checks := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"collection reminders", es.CheckUpcomingCollections},
		{"expire stale evidence", es.ExpireStaleEvidence},
		{"overdue collections", es.CheckOverdueCollections},
		{"controls missing evidence", es.CheckControlsMissingEvidence},
	}

	var firstErr error
	for _, check := range checks {
		if err := check.fn(ctx); err != nil {
			log.Error().Err(err).Str("check", check.name).Msg("evidence_scheduler: check failed")
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", check.name, err)
			}
		}
	}

	log.Info().Msg("evidence_scheduler: daily checks completed")
	return firstErr
}

// CheckUpcomingCollections queries active evidence_collection_configs whose
// next_collection_at falls within the upcoming notification windows (7d, 3d, 1d)
// and emits reminder events.
func (es *EvidenceScheduler) CheckUpcomingCollections(ctx context.Context) error {
	rows, err := es.pool.Query(ctx, `
		SELECT ecc.id, ecc.organization_id, ecc.control_implementation_id,
		       ecc.name, ecc.next_collection_at, ecc.collection_method,
		       ecc.consecutive_failures,
		       ci.control_code
		FROM evidence_collection_configs ecc
		JOIN control_implementations ci ON ci.id = ecc.control_implementation_id
		WHERE ecc.is_active = true
		  AND ecc.next_collection_at IS NOT NULL
		  AND ecc.next_collection_at <= NOW() + INTERVAL '7 days'
		  AND ecc.next_collection_at > NOW()
	`)
	if err != nil {
		return fmt.Errorf("query upcoming collections: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	type threshold struct {
		days        float64
		severity    string
		eventSuffix string
	}
	thresholds := []threshold{
		{1, "high", "collection_due_1d"},
		{3, "medium", "collection_due_3d"},
		{7, "low", "collection_due_7d"},
	}

	var count int
	for rows.Next() {
		var configID, orgID, controlImplID, name, collectionMethod string
		var nextCollectionAt time.Time
		var consecutiveFailures int
		var controlCode *string

		if err := rows.Scan(&configID, &orgID, &controlImplID, &name,
			&nextCollectionAt, &collectionMethod, &consecutiveFailures, &controlCode); err != nil {
			log.Error().Err(err).Msg("evidence_scheduler: scan upcoming collection row")
			continue
		}

		daysUntilCollection := nextCollectionAt.Sub(now).Hours() / 24

		for _, t := range thresholds {
			if daysUntilCollection <= t.days {
				data := map[string]interface{}{
					"config_name":            name,
					"collection_method":      collectionMethod,
					"next_collection_at":     nextCollectionAt.Format(time.RFC3339),
					"days_until_collection":  fmt.Sprintf("%.0f", daysUntilCollection),
					"consecutive_failures":   consecutiveFailures,
					"control_implementation": controlImplID,
				}
				if controlCode != nil {
					data["control_code"] = *controlCode
				}

				es.bus.Publish(service.Event{
					Type:       "evidence." + t.eventSuffix,
					Severity:   t.severity,
					OrgID:      orgID,
					EntityType: "evidence_collection_config",
					EntityID:   configID,
					EntityRef:  name,
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

	log.Info().Int("reminders_sent", count).Msg("evidence_scheduler: collection reminders complete")
	return nil
}

// ExpireStaleEvidence marks evidence whose valid_until date has passed as no
// longer current. This ensures that controls relying on time-bound evidence
// (certificates, audit reports, etc.) are flagged for re-collection.
func (es *EvidenceScheduler) ExpireStaleEvidence(ctx context.Context) error {
	// Update is_current and review_status for evidence past its validity.
	result, err := es.pool.Exec(ctx, `
		UPDATE control_evidence
		SET is_current = false,
		    review_status = 'expired',
		    updated_at = NOW()
		WHERE deleted_at IS NULL
		  AND is_current = true
		  AND valid_until IS NOT NULL
		  AND valid_until < CURRENT_DATE
	`)
	if err != nil {
		return fmt.Errorf("expire stale evidence: %w", err)
	}

	expired := result.RowsAffected()
	if expired > 0 {
		log.Info().Int64("expired_count", expired).Msg("evidence_scheduler: marked stale evidence as expired")
	}

	// Emit events for each newly expired evidence item so control owners are
	// notified. We query evidence that was just expired (review_status=expired,
	// updated today) to avoid re-notifying.
	rows, err := es.pool.Query(ctx, `
		SELECT ce.id, ce.organization_id, ce.control_implementation_id,
		       ce.title, ce.valid_until, ce.collected_by,
		       ci.control_code
		FROM control_evidence ce
		JOIN control_implementations ci ON ci.id = ce.control_implementation_id
		WHERE ce.deleted_at IS NULL
		  AND ce.review_status = 'expired'
		  AND ce.is_current = false
		  AND ce.updated_at::date = CURRENT_DATE
		  AND ce.valid_until IS NOT NULL
		  AND ce.valid_until < CURRENT_DATE
	`)
	if err != nil {
		return fmt.Errorf("query newly expired evidence: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	for rows.Next() {
		var evidenceID, orgID, controlImplID, title string
		var validUntil time.Time
		var collectedBy *string
		var controlCode *string

		if err := rows.Scan(&evidenceID, &orgID, &controlImplID, &title,
			&validUntil, &collectedBy, &controlCode); err != nil {
			log.Error().Err(err).Msg("evidence_scheduler: scan expired evidence row")
			continue
		}

		data := map[string]interface{}{
			"evidence_title":       title,
			"valid_until":          validUntil.Format("2006-01-02"),
			"control_implementation": controlImplID,
		}
		if collectedBy != nil {
			data["owner_id"] = *collectedBy
		}
		if controlCode != nil {
			data["control_code"] = *controlCode
		}

		es.bus.Publish(service.Event{
			Type:       "evidence.expired",
			Severity:   "high",
			OrgID:      orgID,
			EntityType: "control_evidence",
			EntityID:   evidenceID,
			EntityRef:  title,
			Data:       data,
			Timestamp:  now,
		})
	}

	return rows.Err()
}

// CheckOverdueCollections queries active evidence collection configs whose
// next_collection_at has passed (i.e. the automated or manual collection did
// not happen on schedule) and emits overdue notifications.
func (es *EvidenceScheduler) CheckOverdueCollections(ctx context.Context) error {
	rows, err := es.pool.Query(ctx, `
		SELECT ecc.id, ecc.organization_id, ecc.control_implementation_id,
		       ecc.name, ecc.next_collection_at, ecc.collection_method,
		       ecc.consecutive_failures, ecc.failure_threshold,
		       ci.control_code
		FROM evidence_collection_configs ecc
		JOIN control_implementations ci ON ci.id = ecc.control_implementation_id
		WHERE ecc.is_active = true
		  AND ecc.next_collection_at IS NOT NULL
		  AND ecc.next_collection_at < NOW()
	`)
	if err != nil {
		return fmt.Errorf("query overdue collections: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var count int

	for rows.Next() {
		var configID, orgID, controlImplID, name, collectionMethod string
		var nextCollectionAt time.Time
		var consecutiveFailures, failureThreshold int
		var controlCode *string

		if err := rows.Scan(&configID, &orgID, &controlImplID, &name,
			&nextCollectionAt, &collectionMethod, &consecutiveFailures,
			&failureThreshold, &controlCode); err != nil {
			log.Error().Err(err).Msg("evidence_scheduler: scan overdue collection row")
			continue
		}

		daysOverdue := now.Sub(nextCollectionAt).Hours() / 24

		var severity, eventType string
		switch {
		case consecutiveFailures >= failureThreshold:
			severity = "critical"
			eventType = "evidence.collection_circuit_breaker"
		case daysOverdue > 7:
			severity = "high"
			eventType = "evidence.collection_severely_overdue"
		case daysOverdue > 1:
			severity = "high"
			eventType = "evidence.collection_overdue"
		default:
			severity = "medium"
			eventType = "evidence.collection_missed"
		}

		data := map[string]interface{}{
			"config_name":           name,
			"collection_method":     collectionMethod,
			"next_collection_at":    nextCollectionAt.Format(time.RFC3339),
			"days_overdue":          fmt.Sprintf("%.0f", daysOverdue),
			"consecutive_failures":  consecutiveFailures,
			"failure_threshold":     failureThreshold,
			"control_implementation": controlImplID,
		}
		if controlCode != nil {
			data["control_code"] = *controlCode
		}

		es.bus.Publish(service.Event{
			Type:       eventType,
			Severity:   severity,
			OrgID:      orgID,
			EntityType: "evidence_collection_config",
			EntityID:   configID,
			EntityRef:  name,
			Data:       data,
			Timestamp:  now,
		})
		count++
	}

	if err := rows.Err(); err != nil {
		return err
	}

	log.Info().Int("overdue_notifications", count).Msg("evidence_scheduler: overdue collection checks complete")
	return nil
}

// CheckControlsMissingEvidence finds control implementations that have no
// current evidence at all (is_current=true) and emits a warning so that
// control owners can upload or configure automated collection.
func (es *EvidenceScheduler) CheckControlsMissingEvidence(ctx context.Context) error {
	rows, err := es.pool.Query(ctx, `
		SELECT ci.id, ci.organization_id, ci.control_code, ci.owner_id
		FROM control_implementations ci
		WHERE ci.deleted_at IS NULL
		  AND ci.status IN ('implemented', 'partially_implemented')
		  AND NOT EXISTS (
		      SELECT 1 FROM control_evidence ce
		      WHERE ce.control_implementation_id = ci.id
		        AND ce.is_current = true
		        AND ce.deleted_at IS NULL
		  )
	`)
	if err != nil {
		return fmt.Errorf("query controls missing evidence: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var count int

	for rows.Next() {
		var implID, orgID string
		var controlCode *string
		var ownerID *string

		if err := rows.Scan(&implID, &orgID, &controlCode, &ownerID); err != nil {
			log.Error().Err(err).Msg("evidence_scheduler: scan missing evidence row")
			continue
		}

		data := map[string]interface{}{
			"control_implementation": implID,
		}
		if controlCode != nil {
			data["control_code"] = *controlCode
		}
		if ownerID != nil {
			data["owner_id"] = *ownerID
		}

		ref := implID
		if controlCode != nil {
			ref = *controlCode
		}

		es.bus.Publish(service.Event{
			Type:       "evidence.missing_for_control",
			Severity:   "medium",
			OrgID:      orgID,
			EntityType: "control_implementation",
			EntityID:   implID,
			EntityRef:  ref,
			Data:       data,
			Timestamp:  now,
		})
		count++
	}

	if err := rows.Err(); err != nil {
		return err
	}

	log.Info().Int("controls_missing_evidence", count).Msg("evidence_scheduler: missing evidence checks complete")
	return nil
}
