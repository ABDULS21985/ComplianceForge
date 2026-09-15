package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

const (
	defaultEvidenceExpiryBatch = 1000
)

type EvidenceLifecycleSchedulerRepository interface {
	ExpireDueEvidence(context.Context, string, int) ([]models.ExpiredEvidenceNotice, error)
}

// EvidenceScheduler runs daily to manage evidence collection lifecycle:
//   - Remind stakeholders when evidence collection is approaching due
//   - Mark expired evidence (valid_until has passed) as no longer current
//   - Detect evidence collection configs with overdue next_collection_at
//   - Flag controls that have no current evidence
type EvidenceScheduler struct {
	pool      *pgxpool.Pool
	bus       *service.EventBus
	lifecycle EvidenceLifecycleSchedulerRepository
}

func NewEvidenceScheduler(
	pool *pgxpool.Pool,
	bus *service.EventBus,
	configured ...EvidenceLifecycleSchedulerRepository,
) *EvidenceScheduler {
	var lifecycle EvidenceLifecycleSchedulerRepository
	lifecycle, _ = repository.NewEvidenceLifecycleRepository(pool)
	if len(configured) > 0 {
		lifecycle = configured[0]
	}
	return &EvidenceScheduler{
		pool:      pool,
		bus:       bus,
		lifecycle: lifecycle,
	}
}

type evidenceTenantCheck struct {
	name string
	run  func(context.Context, string) error
}

// Run executes all evidence lifecycle checks. Called once per day by the
// background worker.
func (es *EvidenceScheduler) Run(ctx context.Context) error {
	log.Info().Msg("evidence_scheduler: starting daily checks")

	checks := []evidenceTenantCheck{
		{"collection reminders", es.checkUpcomingCollectionsForTenant},
		{"expire stale evidence", es.expireStaleEvidenceForTenant},
		{"overdue collections", es.checkOverdueCollectionsForTenant},
		{"controls missing evidence", es.checkControlsMissingEvidenceForTenant},
	}
	err := es.runTenantChecks(ctx, checks)
	log.Info().Msg("evidence_scheduler: daily checks completed")
	return err
}

func (es *EvidenceScheduler) runTenantChecks(ctx context.Context, checks []evidenceTenantCheck) error {
	if es == nil || es.pool == nil || es.bus == nil || es.lifecycle == nil {
		return errors.New("evidence scheduler is not configured")
	}
	return runForScheduledTenants(ctx, es.pool, "evidence lifecycle", func(tenantCtx context.Context, tenantID string) error {
		var tenantErrors []error
		for _, check := range checks {
			if err := check.run(tenantCtx, tenantID); err != nil {
				log.Error().Err(err).Str("tenant_id", tenantID).Str("check", check.name).
					Msg("evidence_scheduler: tenant check failed")
				tenantErrors = append(tenantErrors, fmt.Errorf("%s: %w", check.name, err))
			}
		}
		return errors.Join(tenantErrors...)
	})
}

// CheckUpcomingCollections queries active evidence_collection_configs whose
// next_collection_at falls within the upcoming notification windows (7d, 3d, 1d)
// and emits reminder events.
func (es *EvidenceScheduler) CheckUpcomingCollections(ctx context.Context) error {
	return es.runTenantChecks(ctx, []evidenceTenantCheck{
		{"collection reminders", es.checkUpcomingCollectionsForTenant},
	})
}

func (es *EvidenceScheduler) checkUpcomingCollectionsForTenant(ctx context.Context, tenantID string) error {
	rows, err := database.QuerierFromContext(ctx, es.pool).Query(ctx, `
		SELECT ecc.id, ecc.organization_id, ecc.control_implementation_id,
		       ecc.name, ecc.next_collection_at, ecc.collection_method,
		       ecc.consecutive_failures,
		       fc.code
		FROM evidence_collection_configs ecc
		JOIN control_implementations ci
		  ON ci.organization_id = ecc.organization_id
		 AND ci.id = ecc.control_implementation_id
		JOIN framework_controls fc ON fc.id = ci.framework_control_id
		WHERE ecc.organization_id = $1::uuid
		  AND ecc.is_active = true
		  AND ecc.next_collection_at IS NOT NULL
		  AND ecc.next_collection_at <= NOW() + INTERVAL '7 days'
		  AND ecc.next_collection_at > NOW()
		ORDER BY ecc.next_collection_at, ecc.id
	`, tenantID)
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
	return es.runTenantChecks(ctx, []evidenceTenantCheck{
		{"expire stale evidence", es.expireStaleEvidenceForTenant},
	})
}

func (es *EvidenceScheduler) expireStaleEvidenceForTenant(ctx context.Context, tenantID string) error {
	notices, err := es.lifecycle.ExpireDueEvidence(ctx, tenantID, defaultEvidenceExpiryBatch)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, notice := range notices {
		if notice.NotificationQueued {
			continue
		}
		data := map[string]interface{}{
			"evidence_title":         notice.Title,
			"expires_at":             notice.ExpiresAt.Format(time.RFC3339),
			"control_implementation": notice.ControlImplementationID,
		}
		if notice.CollectedBy != nil {
			data["owner_id"] = *notice.CollectedBy
		}
		if notice.ControlCode != nil {
			data["control_code"] = *notice.ControlCode
		}
		es.bus.Publish(service.Event{
			Type:       "evidence.expired",
			Severity:   "high",
			OrgID:      notice.OrganizationID,
			EntityType: "control_evidence",
			EntityID:   notice.EvidenceID,
			EntityRef:  notice.Title,
			Data:       data,
			Timestamp:  now,
		})
	}
	log.Info().Int("expired_count", len(notices)).Str("tenant_id", tenantID).
		Msg("evidence_scheduler: marked due evidence as expired")
	return nil
}

// CheckOverdueCollections queries active evidence collection configs whose
// next_collection_at has passed (i.e. the automated or manual collection did
// not happen on schedule) and emits overdue notifications.
func (es *EvidenceScheduler) CheckOverdueCollections(ctx context.Context) error {
	return es.runTenantChecks(ctx, []evidenceTenantCheck{
		{"overdue collections", es.checkOverdueCollectionsForTenant},
	})
}

func (es *EvidenceScheduler) checkOverdueCollectionsForTenant(ctx context.Context, tenantID string) error {
	rows, err := database.QuerierFromContext(ctx, es.pool).Query(ctx, `
		SELECT ecc.id, ecc.organization_id, ecc.control_implementation_id,
		       ecc.name, ecc.next_collection_at, ecc.collection_method,
		       ecc.consecutive_failures, ecc.failure_threshold,
		       fc.code
		FROM evidence_collection_configs ecc
		JOIN control_implementations ci
		  ON ci.organization_id = ecc.organization_id
		 AND ci.id = ecc.control_implementation_id
		JOIN framework_controls fc ON fc.id = ci.framework_control_id
		WHERE ecc.organization_id = $1::uuid
		  AND ecc.is_active = true
		  AND ecc.next_collection_at IS NOT NULL
		  AND ecc.next_collection_at < NOW()
		ORDER BY ecc.next_collection_at, ecc.id
	`, tenantID)
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
			"config_name":            name,
			"collection_method":      collectionMethod,
			"next_collection_at":     nextCollectionAt.Format(time.RFC3339),
			"days_overdue":           fmt.Sprintf("%.0f", daysOverdue),
			"consecutive_failures":   consecutiveFailures,
			"failure_threshold":      failureThreshold,
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
	return es.runTenantChecks(ctx, []evidenceTenantCheck{
		{"controls missing evidence", es.checkControlsMissingEvidenceForTenant},
	})
}

func (es *EvidenceScheduler) checkControlsMissingEvidenceForTenant(ctx context.Context, tenantID string) error {
	rows, err := database.QuerierFromContext(ctx, es.pool).Query(ctx, `
		SELECT ci.id, ci.organization_id, fc.code, ci.owner_user_id
		FROM control_implementations ci
		JOIN framework_controls fc ON fc.id = ci.framework_control_id
		WHERE ci.organization_id = $1::uuid
		  AND ci.deleted_at IS NULL
		  AND ci.status IN ('implemented', 'partial')
		  AND NOT EXISTS (
		      SELECT 1 FROM control_evidence ce
		      WHERE ce.organization_id = ci.organization_id
		        AND ce.control_implementation_id = ci.id
		        AND ce.is_current = true
		        AND ce.lifecycle_status = 'active'
		        AND ce.deleted_at IS NULL
		  )
		ORDER BY fc.code, ci.id
	`, tenantID)
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
