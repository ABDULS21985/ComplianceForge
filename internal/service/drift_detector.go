package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// DriftDetector analyses compliance monitoring results and creates drift events
// when controls fall out of compliance.
type DriftDetector struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// NewDriftDetector creates a new drift detector instance.
func NewDriftDetector(pool *pgxpool.Pool, bus *EventBus) *DriftDetector {
	return &DriftDetector{pool: pool, bus: bus}
}

// DetectAndRecord checks if a monitor failure constitutes a drift event and records it.
func (dd *DriftDetector) DetectAndRecord(ctx context.Context, orgID string, driftType, severity, entityType string, entityID *string, entityRef, description, previousState, currentState string) error {
	// Check if there's already an active (unresolved) drift event for this entity
	var existingID *string
	err := dd.pool.QueryRow(ctx, `
		SELECT id FROM compliance_drift_events
		WHERE organization_id = $1 AND entity_type = $2
		  AND ($3::uuid IS NULL OR entity_id = $3)
		  AND entity_ref = $4
		  AND resolved_at IS NULL
		LIMIT 1
	`, orgID, entityType, entityID, entityRef).Scan(&existingID)

	if existingID != nil {
		log.Debug().Str("entity_ref", entityRef).Msg("drift_detector: active drift already exists, skipping")
		return nil
	}

	// Create new drift event
	var driftID string
	err = dd.pool.QueryRow(ctx, `
		INSERT INTO compliance_drift_events (
			organization_id, drift_type, severity, entity_type, entity_id, entity_ref,
			description, previous_state, current_state, detected_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, orgID, driftType, severity, entityType, entityID, entityRef,
		description, previousState, currentState, time.Now().UTC(),
	).Scan(&driftID)
	if err != nil {
		return fmt.Errorf("creating drift event: %w", err)
	}

	log.Warn().
		Str("drift_id", driftID).
		Str("drift_type", driftType).
		Str("severity", severity).
		Str("entity_ref", entityRef).
		Str("description", description).
		Msg("drift_detector: new drift event detected")

	// Emit notification event
	if dd.bus != nil {
		dd.bus.Publish(Event{
			Type:       fmt.Sprintf("drift.%s", driftType),
			Severity:   severity,
			OrgID:      orgID,
			EntityType: entityType,
			EntityRef:  entityRef,
			Data: map[string]interface{}{
				"drift_id":       driftID,
				"drift_type":     driftType,
				"severity":       severity,
				"entity_type":    entityType,
				"entity_ref":     entityRef,
				"description":    description,
				"previous_state": previousState,
				"current_state":  currentState,
			},
			Timestamp: time.Now().UTC(),
		})
	}

	return nil
}

// GetActiveDriftSummary returns a summary of active (unresolved) drift events per org.
func (dd *DriftDetector) GetActiveDriftSummary(ctx context.Context, orgID string) (map[string]int, error) {
	rows, err := dd.pool.Query(ctx, `
		SELECT severity, COUNT(*)
		FROM compliance_drift_events
		WHERE organization_id = $1 AND resolved_at IS NULL
		GROUP BY severity
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying drift summary: %w", err)
	}
	defer rows.Close()

	summary := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	for rows.Next() {
		var sev string
		var count int
		if err := rows.Scan(&sev, &count); err != nil {
			continue
		}
		summary[sev] = count
	}
	return summary, nil
}

// ResolveStale auto-resolves drift events that have been active for too long
// without acknowledgment (configurable, default 30 days).
func (dd *DriftDetector) ResolveStale(ctx context.Context, orgID string, staleDays int) (int, error) {
	if staleDays <= 0 {
		staleDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -staleDays)

	tag, err := dd.pool.Exec(ctx, `
		UPDATE compliance_drift_events
		SET resolved_at = NOW(), resolution_notes = 'Auto-resolved: stale drift event'
		WHERE organization_id = $1
		  AND resolved_at IS NULL
		  AND detected_at < $2
	`, orgID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("resolving stale drifts: %w", err)
	}

	count := int(tag.RowsAffected())
	if count > 0 {
		log.Info().Int("count", count).Int("stale_days", staleDays).Msg("drift_detector: resolved stale drift events")
	}
	return count, nil
}
