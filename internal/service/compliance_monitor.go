package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ComplianceMonitor continuously checks compliance status and detects drift.
type ComplianceMonitor struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// Monitor represents a configured compliance watchdog.
type Monitor struct {
	ID                  string  `json:"id"`
	OrgID               string  `json:"organization_id"`
	Name                string  `json:"name"`
	MonitorType         string  `json:"monitor_type"`
	TargetEntityType    string  `json:"target_entity_type"`
	TargetEntityID      *string `json:"target_entity_id"`
	IsActive            bool    `json:"is_active"`
	LastCheckStatus     string  `json:"last_check_status"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
}

// DriftEvent records a deviation from the desired compliance posture.
type DriftEvent struct {
	ID             string  `json:"id"`
	OrgID          string  `json:"organization_id"`
	DriftType      string  `json:"drift_type"`
	Severity       string  `json:"severity"`
	EntityType     string  `json:"entity_type"`
	EntityID       *string `json:"entity_id"`
	EntityRef      string  `json:"entity_ref"`
	Description    string  `json:"description"`
	PreviousState  string  `json:"previous_state"`
	CurrentState   string  `json:"current_state"`
	DetectedAt     string  `json:"detected_at"`
	AcknowledgedAt *string `json:"acknowledged_at"`
	ResolvedAt     *string `json:"resolved_at"`
}

// MonitoringDashboard provides an overview of continuous monitoring health.
type MonitoringDashboard struct {
	OverallHealth            string  `json:"overall_health"` // green, amber, red
	ActiveDriftEvents        int     `json:"active_drift_events"`
	CriticalDrifts           int     `json:"critical_drifts"`
	HighDrifts               int     `json:"high_drifts"`
	CollectionSuccessRate24h float64 `json:"collection_success_rate_24h"`
	CollectionSuccessRate7d  float64 `json:"collection_success_rate_7d"`
	MonitorsTotal            int     `json:"monitors_total"`
	MonitorsPassing          int     `json:"monitors_passing"`
	MonitorsFailing          int     `json:"monitors_failing"`
}

// NewComplianceMonitor creates a new ComplianceMonitor.
func NewComplianceMonitor(pool *pgxpool.Pool, bus *EventBus) *ComplianceMonitor {
	return &ComplianceMonitor{pool: pool, bus: bus}
}

// RunAllChecks iterates all active monitors and runs their checks.
func (cm *ComplianceMonitor) RunAllChecks(ctx context.Context) error {
	rows, err := cm.pool.Query(ctx, `
		SELECT id, organization_id, name, monitor_type, target_entity_type,
		       target_entity_id, conditions, alert_severity
		FROM compliance_monitors
		WHERE is_active = true
		ORDER BY organization_id, name`)
	if err != nil {
		return fmt.Errorf("query active monitors: %w", err)
	}
	defer rows.Close()

	type monitorRow struct {
		id, orgID, name, monType, entityType string
		entityID                              *string
		conditions                            []byte
		alertSeverity                         string
	}

	var monitors []monitorRow
	for rows.Next() {
		var m monitorRow
		if err := rows.Scan(&m.id, &m.orgID, &m.name, &m.monType,
			&m.entityType, &m.entityID, &m.conditions, &m.alertSeverity); err != nil {
			log.Error().Err(err).Msg("failed to scan monitor row")
			continue
		}
		monitors = append(monitors, m)
	}

	for _, m := range monitors {
		var passed bool
		var message string
		var checkErr error

		mon := Monitor{
			ID:               m.id,
			OrgID:            m.orgID,
			Name:             m.name,
			MonitorType:      m.monType,
			TargetEntityType: m.entityType,
			TargetEntityID:   m.entityID,
		}

		switch m.monType {
		case "control_effectiveness":
			passed, message, checkErr = cm.CheckControlEffectiveness(ctx, mon)
		case "evidence_freshness":
			passed, message, checkErr = cm.CheckEvidenceFreshness(ctx, mon)
		case "kri_threshold":
			passed, message, checkErr = cm.CheckKRIThreshold(ctx, mon)
		default:
			passed = true
			message = fmt.Sprintf("unsupported monitor type: %s (skipped)", m.monType)
		}

		if checkErr != nil {
			log.Error().Err(checkErr).Str("monitor_id", m.id).Str("type", m.monType).Msg("monitor check error")
			message = fmt.Sprintf("check error: %v", checkErr)
			passed = false
		}

		status := "passing"
		if !passed {
			status = "failing"
		}

		// Record result.
		_, _ = cm.pool.Exec(ctx, `
			INSERT INTO compliance_monitor_results (
				organization_id, monitor_id, status, message
			) VALUES ($1, $2, $3, $4)`,
			m.orgID, m.id, status, message)

		// Update monitor state.
		if passed {
			_, _ = cm.pool.Exec(ctx, `
				UPDATE compliance_monitors
				SET last_check_at = NOW(), last_check_status = 'passing',
				    consecutive_failures = 0, failure_since = NULL
				WHERE id = $1`, m.id)
		} else {
			_, _ = cm.pool.Exec(ctx, `
				UPDATE compliance_monitors
				SET last_check_at = NOW(), last_check_status = 'failing',
				    consecutive_failures = consecutive_failures + 1,
				    failure_since = COALESCE(failure_since, NOW())
				WHERE id = $1`, m.id)

			// Detect drift.
			if err := cm.DetectDrift(ctx, mon, passed, message); err != nil {
				log.Error().Err(err).Str("monitor_id", m.id).Msg("drift detection failed")
			}
		}
	}

	log.Info().Int("monitors_checked", len(monitors)).Msg("compliance monitor run completed")
	return nil
}

// CheckControlEffectiveness verifies that a control implementation remains effective.
func (cm *ComplianceMonitor) CheckControlEffectiveness(ctx context.Context, monitor Monitor) (bool, string, error) {
	if monitor.TargetEntityID == nil {
		return false, "no target entity ID specified", nil
	}

	var status string
	var effectivenessScore *float64
	var lastTestedAt *time.Time

	err := cm.pool.QueryRow(ctx, `
		SELECT status, effectiveness_score, last_tested_at
		FROM control_implementations
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
		*monitor.TargetEntityID, monitor.OrgID,
	).Scan(&status, &effectivenessScore, &lastTestedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, "control implementation not found", nil
		}
		return false, "", fmt.Errorf("query control implementation: %w", err)
	}

	// Check if control is in an effective or implemented state.
	if status != "effective" && status != "implemented" {
		return false, fmt.Sprintf("control status is '%s' (expected 'effective' or 'implemented')", status), nil
	}

	// Check effectiveness score if available.
	if effectivenessScore != nil && *effectivenessScore < 70.0 {
		return false, fmt.Sprintf("effectiveness score %.1f%% is below 70%% threshold", *effectivenessScore), nil
	}

	// Check test freshness (warn if last test was over 90 days ago).
	if lastTestedAt != nil && time.Since(*lastTestedAt) > 90*24*time.Hour {
		return false, fmt.Sprintf("last test was %d days ago (>90 day threshold)", int(time.Since(*lastTestedAt).Hours()/24)), nil
	}

	return true, fmt.Sprintf("control effective, status=%s", status), nil
}

// CheckEvidenceFreshness verifies that evidence for a control is current and not expired.
func (cm *ComplianceMonitor) CheckEvidenceFreshness(ctx context.Context, monitor Monitor) (bool, string, error) {
	if monitor.TargetEntityID == nil {
		return false, "no target entity ID specified", nil
	}

	var currentCount, expiredCount int
	err := cm.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE is_current = true AND (valid_until IS NULL OR valid_until >= CURRENT_DATE)),
			COUNT(*) FILTER (WHERE is_current = true AND valid_until IS NOT NULL AND valid_until < CURRENT_DATE)
		FROM control_evidence
		WHERE control_implementation_id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
		*monitor.TargetEntityID, monitor.OrgID,
	).Scan(&currentCount, &expiredCount)
	if err != nil {
		return false, "", fmt.Errorf("query evidence freshness: %w", err)
	}

	if currentCount == 0 {
		return false, "no current evidence found for control", nil
	}
	if expiredCount > 0 {
		return false, fmt.Sprintf("%d evidence items have expired", expiredCount), nil
	}

	return true, fmt.Sprintf("%d current evidence items, none expired", currentCount), nil
}

// CheckKRIThreshold verifies that a Key Risk Indicator is within acceptable limits.
func (cm *ComplianceMonitor) CheckKRIThreshold(ctx context.Context, monitor Monitor) (bool, string, error) {
	if monitor.TargetEntityID == nil {
		return false, "no target entity ID specified", nil
	}

	var name string
	var currentValue, thresholdRed *float64

	err := cm.pool.QueryRow(ctx, `
		SELECT name, current_value, threshold_red
		FROM risk_indicators
		WHERE id = $1 AND organization_id = $2`,
		*monitor.TargetEntityID, monitor.OrgID,
	).Scan(&name, &currentValue, &thresholdRed)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, "KRI not found", nil
		}
		return false, "", fmt.Errorf("query KRI: %w", err)
	}

	if currentValue == nil {
		return false, fmt.Sprintf("KRI '%s' has no current value", name), nil
	}

	if thresholdRed != nil && *currentValue >= *thresholdRed {
		return false, fmt.Sprintf("KRI '%s' current value %.2f exceeds red threshold %.2f", name, *currentValue, *thresholdRed), nil
	}

	return true, fmt.Sprintf("KRI '%s' value %.2f within acceptable limits", name, *currentValue), nil
}

// DetectDrift creates a drift event when a monitor check fails.
func (cm *ComplianceMonitor) DetectDrift(ctx context.Context, monitor Monitor, checkPassed bool, message string) error {
	if checkPassed {
		return nil
	}

	// Determine drift type based on monitor type.
	driftTypeMap := map[string]string{
		"control_effectiveness": "control_degraded",
		"evidence_freshness":    "evidence_expired",
		"kri_threshold":         "kri_breached",
		"policy_attestation":    "policy_unattested",
		"vendor_assessment":     "vendor_overdue",
		"training_completion":   "training_expired",
	}

	driftType := driftTypeMap[monitor.MonitorType]
	if driftType == "" {
		driftType = "control_degraded"
	}

	// Check if there's already an active (unresolved) drift for this entity.
	var existingCount int
	_ = cm.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM compliance_drift_events
		WHERE organization_id = $1 AND drift_type = $2
		  AND entity_type = $3 AND entity_id = $4
		  AND resolved_at IS NULL`,
		monitor.OrgID, driftType, monitor.TargetEntityType, monitor.TargetEntityID,
	).Scan(&existingCount)

	if existingCount > 0 {
		// Drift already tracked, no need to create another.
		return nil
	}

	// Determine severity based on monitor's alert_severity.
	var alertSeverity string
	_ = cm.pool.QueryRow(ctx, `
		SELECT alert_severity FROM compliance_monitors WHERE id = $1`, monitor.ID,
	).Scan(&alertSeverity)
	if alertSeverity == "" {
		alertSeverity = "high"
	}

	var driftID string
	err := cm.pool.QueryRow(ctx, `
		INSERT INTO compliance_drift_events (
			organization_id, drift_type, severity, entity_type, entity_id,
			entity_ref, description, previous_state, current_state
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		monitor.OrgID, driftType, alertSeverity, monitor.TargetEntityType,
		monitor.TargetEntityID, monitor.Name,
		message, "passing", "failing",
	).Scan(&driftID)
	if err != nil {
		return fmt.Errorf("insert drift event: %w", err)
	}

	// Emit notification event.
	cm.bus.Publish(Event{
		Type:       "compliance.drift_detected",
		Severity:   alertSeverity,
		OrgID:      monitor.OrgID,
		EntityType: monitor.TargetEntityType,
		EntityID:   driftID,
		EntityRef:  monitor.Name,
		Data: map[string]interface{}{
			"drift_type":  driftType,
			"monitor_id":  monitor.ID,
			"description": message,
		},
		Timestamp: time.Now(),
	})

	log.Warn().
		Str("drift_id", driftID).
		Str("drift_type", driftType).
		Str("severity", alertSeverity).
		Str("monitor", monitor.Name).
		Msg("compliance drift detected")

	return nil
}

// ListDriftEvents returns a paginated, filtered list of drift events.
func (cm *ComplianceMonitor) ListDriftEvents(ctx context.Context, orgID string, filters map[string]string) ([]DriftEvent, int, error) {
	page := 1
	pageSize := 20

	whereClause := "organization_id = $1"
	args := []interface{}{orgID}
	argIdx := 2

	if v, ok := filters["severity"]; ok && v != "" {
		whereClause += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, v)
		argIdx++
	}
	if v, ok := filters["drift_type"]; ok && v != "" {
		whereClause += fmt.Sprintf(" AND drift_type = $%d", argIdx)
		args = append(args, v)
		argIdx++
	}
	if v, ok := filters["status"]; ok {
		switch v {
		case "active":
			whereClause += " AND resolved_at IS NULL"
		case "resolved":
			whereClause += " AND resolved_at IS NOT NULL"
		case "acknowledged":
			whereClause += " AND acknowledged_at IS NOT NULL AND resolved_at IS NULL"
		}
	}

	var total int
	err := cm.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM compliance_drift_events WHERE %s", whereClause),
		args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count drift events: %w", err)
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := cm.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, organization_id, drift_type, severity, entity_type, entity_id,
		       COALESCE(entity_ref, ''), description,
		       COALESCE(previous_state, ''), COALESCE(current_state, ''),
		       detected_at, acknowledged_at, resolved_at
		FROM compliance_drift_events
		WHERE %s
		ORDER BY detected_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list drift events: %w", err)
	}
	defer rows.Close()

	var events []DriftEvent
	for rows.Next() {
		var e DriftEvent
		var detectedAt time.Time
		var ackAt, resAt *time.Time
		if err := rows.Scan(&e.ID, &e.OrgID, &e.DriftType, &e.Severity,
			&e.EntityType, &e.EntityID, &e.EntityRef, &e.Description,
			&e.PreviousState, &e.CurrentState,
			&detectedAt, &ackAt, &resAt); err != nil {
			return nil, 0, fmt.Errorf("scan drift event: %w", err)
		}
		e.DetectedAt = detectedAt.Format(time.RFC3339)
		if ackAt != nil {
			aa := ackAt.Format(time.RFC3339)
			e.AcknowledgedAt = &aa
		}
		if resAt != nil {
			ra := resAt.Format(time.RFC3339)
			e.ResolvedAt = &ra
		}
		events = append(events, e)
	}

	return events, total, nil
}

// AcknowledgeDrift marks a drift event as acknowledged.
func (cm *ComplianceMonitor) AcknowledgeDrift(ctx context.Context, orgID, driftID, userID string) error {
	tag, err := cm.pool.Exec(ctx, `
		UPDATE compliance_drift_events
		SET acknowledged_at = NOW(), acknowledged_by = $1
		WHERE id = $2 AND organization_id = $3 AND acknowledged_at IS NULL`,
		userID, driftID, orgID)
	if err != nil {
		return fmt.Errorf("acknowledge drift: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("drift event not found or already acknowledged")
	}

	log.Info().Str("drift_id", driftID).Str("user_id", userID).Msg("drift event acknowledged")
	return nil
}

// ResolveDrift marks a drift event as resolved with notes.
func (cm *ComplianceMonitor) ResolveDrift(ctx context.Context, orgID, driftID, userID, notes string) error {
	tag, err := cm.pool.Exec(ctx, `
		UPDATE compliance_drift_events
		SET resolved_at = NOW(), resolved_by = $1, resolution_notes = $2
		WHERE id = $3 AND organization_id = $4 AND resolved_at IS NULL`,
		userID, notes, driftID, orgID)
	if err != nil {
		return fmt.Errorf("resolve drift: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("drift event not found or already resolved")
	}

	cm.bus.Publish(Event{
		Type:       "compliance.drift_resolved",
		Severity:   "low",
		OrgID:      orgID,
		EntityType: "compliance_drift_event",
		EntityID:   driftID,
		Data:       map[string]interface{}{"resolved_by": userID, "notes": notes},
		Timestamp:  time.Now(),
	})

	log.Info().Str("drift_id", driftID).Str("user_id", userID).Msg("drift event resolved")
	return nil
}

// GetDashboard returns the continuous monitoring dashboard for an organization.
func (cm *ComplianceMonitor) GetDashboard(ctx context.Context, orgID string) (*MonitoringDashboard, error) {
	d := &MonitoringDashboard{}

	// Active drift events.
	_ = cm.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE resolved_at IS NULL),
			COUNT(*) FILTER (WHERE resolved_at IS NULL AND severity = 'critical'),
			COUNT(*) FILTER (WHERE resolved_at IS NULL AND severity = 'high')
		FROM compliance_drift_events
		WHERE organization_id = $1`, orgID).
		Scan(&d.ActiveDriftEvents, &d.CriticalDrifts, &d.HighDrifts)

	// Evidence collection success rates.
	_ = cm.pool.QueryRow(ctx, `
		SELECT
			COALESCE(
				COUNT(*) FILTER (WHERE status = 'success')::FLOAT /
				NULLIF(COUNT(*)::FLOAT, 0) * 100, 0
			)
		FROM evidence_collection_runs
		WHERE organization_id = $1 AND created_at > NOW() - INTERVAL '24 hours'`, orgID).
		Scan(&d.CollectionSuccessRate24h)

	_ = cm.pool.QueryRow(ctx, `
		SELECT
			COALESCE(
				COUNT(*) FILTER (WHERE status = 'success')::FLOAT /
				NULLIF(COUNT(*)::FLOAT, 0) * 100, 0
			)
		FROM evidence_collection_runs
		WHERE organization_id = $1 AND created_at > NOW() - INTERVAL '7 days'`, orgID).
		Scan(&d.CollectionSuccessRate7d)

	// Monitor summary.
	_ = cm.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE last_check_status = 'passing'),
			COUNT(*) FILTER (WHERE last_check_status = 'failing')
		FROM compliance_monitors
		WHERE organization_id = $1 AND is_active = true`, orgID).
		Scan(&d.MonitorsTotal, &d.MonitorsPassing, &d.MonitorsFailing)

	// Determine overall health.
	switch {
	case d.CriticalDrifts > 0 || d.MonitorsFailing > d.MonitorsTotal/2:
		d.OverallHealth = "red"
	case d.HighDrifts > 0 || d.MonitorsFailing > 0 || d.CollectionSuccessRate24h < 90:
		d.OverallHealth = "amber"
	default:
		d.OverallHealth = "green"
	}

	return d, nil
}

// ListMonitors returns all monitors for an organization.
func (cm *ComplianceMonitor) ListMonitors(ctx context.Context, orgID string) ([]Monitor, error) {
	rows, err := cm.pool.Query(ctx, `
		SELECT id, organization_id, name, monitor_type, target_entity_type,
		       target_entity_id, is_active, last_check_status, consecutive_failures
		FROM compliance_monitors
		WHERE organization_id = $1
		ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list monitors: %w", err)
	}
	defer rows.Close()

	var monitors []Monitor
	for rows.Next() {
		var m Monitor
		if err := rows.Scan(&m.ID, &m.OrgID, &m.Name, &m.MonitorType,
			&m.TargetEntityType, &m.TargetEntityID, &m.IsActive,
			&m.LastCheckStatus, &m.ConsecutiveFailures); err != nil {
			return nil, fmt.Errorf("scan monitor: %w", err)
		}
		monitors = append(monitors, m)
	}

	return monitors, nil
}

// CreateMonitor creates a new compliance monitor.
func (cm *ComplianceMonitor) CreateMonitor(ctx context.Context, orgID string, monitor Monitor) (*Monitor, error) {
	conditionsJSON, _ := json.Marshal(map[string]interface{}{})

	err := cm.pool.QueryRow(ctx, `
		INSERT INTO compliance_monitors (
			organization_id, name, monitor_type, target_entity_type,
			target_entity_id, conditions, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, true)
		RETURNING id`,
		orgID, monitor.Name, monitor.MonitorType, monitor.TargetEntityType,
		monitor.TargetEntityID, conditionsJSON,
	).Scan(&monitor.ID)
	if err != nil {
		return nil, fmt.Errorf("insert monitor: %w", err)
	}

	monitor.OrgID = orgID
	monitor.IsActive = true
	monitor.LastCheckStatus = "unknown"
	monitor.ConsecutiveFailures = 0

	log.Info().Str("monitor_id", monitor.ID).Str("name", monitor.Name).Msg("compliance monitor created")
	return &monitor, nil
}
