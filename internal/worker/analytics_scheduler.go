package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// AnalyticsScheduler takes periodic metric snapshots for trend analysis.
type AnalyticsScheduler struct {
	pool *pgxpool.Pool
}

func NewAnalyticsScheduler(pool *pgxpool.Pool) *AnalyticsScheduler {
	return &AnalyticsScheduler{pool: pool}
}

// RunDaily takes daily snapshots for all active organizations.
// Called once per day at 00:00 UTC by the background worker.
func (as *AnalyticsScheduler) RunDaily(ctx context.Context) error {
	log.Info().Msg("analytics_scheduler: taking daily snapshots")
	return as.takeSnapshotsForAll(ctx, "daily")
}

// RunWeekly takes weekly snapshots (Mondays) and calculates compliance trends.
func (as *AnalyticsScheduler) RunWeekly(ctx context.Context) error {
	if time.Now().Weekday() != time.Monday {
		return nil
	}
	log.Info().Msg("analytics_scheduler: taking weekly snapshots")
	if err := as.takeSnapshotsForAll(ctx, "weekly"); err != nil {
		return err
	}
	return as.calculateTrends(ctx)
}

// RunMonthly takes monthly snapshots (1st of month).
func (as *AnalyticsScheduler) RunMonthly(ctx context.Context) error {
	if time.Now().Day() != 1 {
		return nil
	}
	log.Info().Msg("analytics_scheduler: taking monthly snapshots")
	return as.takeSnapshotsForAll(ctx, "monthly")
}

func (as *AnalyticsScheduler) takeSnapshotsForAll(ctx context.Context, snapshotType string) error {
	rows, err := as.pool.Query(ctx, `
		SELECT id FROM organizations WHERE status = 'active' AND deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("querying organizations: %w", err)
	}
	defer rows.Close()

	var orgIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		orgIDs = append(orgIDs, id)
	}

	log.Info().Int("org_count", len(orgIDs)).Str("type", snapshotType).Msg("analytics_scheduler: processing orgs")

	for _, orgID := range orgIDs {
		if err := as.takeOrgSnapshot(ctx, orgID, snapshotType); err != nil {
			log.Error().Err(err).Str("org_id", orgID).Msg("analytics_scheduler: snapshot failed")
			continue
		}
	}
	return nil
}

func (as *AnalyticsScheduler) takeOrgSnapshot(ctx context.Context, orgID, snapshotType string) error {
	// Collect metrics
	metrics := make(map[string]interface{})

	// Compliance
	var compScore float64
	as.pool.QueryRow(ctx, `SELECT COALESCE(AVG(compliance_score), 0) FROM organization_frameworks WHERE organization_id = $1`, orgID).Scan(&compScore)
	metrics["compliance_score"] = compScore

	// Risks
	var totalRisks, critical, high int
	as.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE residual_risk_level='critical'), COUNT(*) FILTER (WHERE residual_risk_level='high')
		FROM risks WHERE organization_id=$1 AND deleted_at IS NULL AND status!='closed'
	`, orgID).Scan(&totalRisks, &critical, &high)
	metrics["risks_total"] = totalRisks
	metrics["risks_critical"] = critical

	// Incidents
	var openIncidents int
	as.pool.QueryRow(ctx, `SELECT COUNT(*) FROM incidents WHERE organization_id=$1 AND deleted_at IS NULL AND status NOT IN ('closed','resolved')`, orgID).Scan(&openIncidents)
	metrics["incidents_open"] = openIncidents

	// Findings
	var openFindings int
	as.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_findings WHERE organization_id=$1 AND deleted_at IS NULL AND status NOT IN ('resolved','closed')`, orgID).Scan(&openFindings)
	metrics["findings_open"] = openFindings

	// Insert/upsert
	_, err := as.pool.Exec(ctx, `
		INSERT INTO analytics_snapshots (organization_id, snapshot_type, snapshot_date, metrics)
		VALUES ($1, $2, CURRENT_DATE, $3)
		ON CONFLICT (organization_id, snapshot_type, snapshot_date)
		DO UPDATE SET metrics = $3
	`, orgID, snapshotType, metrics)

	return err
}

func (as *AnalyticsScheduler) calculateTrends(ctx context.Context) error {
	log.Info().Msg("analytics_scheduler: calculating compliance trends")

	_, err := as.pool.Exec(ctx, `
		INSERT INTO analytics_compliance_trends (organization_id, framework_id, framework_code, measurement_date, compliance_score, controls_implemented, controls_total, maturity_avg, score_change_7d, score_change_30d, score_change_90d, trend_direction)
		SELECT
			ofw.organization_id, ofw.framework_id, cf.code, CURRENT_DATE,
			ofw.compliance_score,
			(SELECT COUNT(*) FROM control_implementations ci WHERE ci.org_framework_id = ofw.id AND ci.status IN ('implemented','effective') AND ci.deleted_at IS NULL),
			(SELECT COUNT(*) FROM control_implementations ci WHERE ci.org_framework_id = ofw.id AND ci.deleted_at IS NULL),
			0, -- maturity_avg placeholder
			ofw.compliance_score - COALESCE((SELECT act.compliance_score FROM analytics_compliance_trends act WHERE act.organization_id = ofw.organization_id AND act.framework_id = ofw.framework_id AND act.measurement_date = CURRENT_DATE - 7), ofw.compliance_score),
			ofw.compliance_score - COALESCE((SELECT act.compliance_score FROM analytics_compliance_trends act WHERE act.organization_id = ofw.organization_id AND act.framework_id = ofw.framework_id AND act.measurement_date = CURRENT_DATE - 30), ofw.compliance_score),
			ofw.compliance_score - COALESCE((SELECT act.compliance_score FROM analytics_compliance_trends act WHERE act.organization_id = ofw.organization_id AND act.framework_id = ofw.framework_id AND act.measurement_date = CURRENT_DATE - 90), ofw.compliance_score),
			CASE
				WHEN ofw.compliance_score > COALESCE((SELECT act.compliance_score FROM analytics_compliance_trends act WHERE act.organization_id = ofw.organization_id AND act.framework_id = ofw.framework_id AND act.measurement_date = CURRENT_DATE - 30), ofw.compliance_score) + 0.5 THEN 'improving'
				WHEN ofw.compliance_score < COALESCE((SELECT act.compliance_score FROM analytics_compliance_trends act WHERE act.organization_id = ofw.organization_id AND act.framework_id = ofw.framework_id AND act.measurement_date = CURRENT_DATE - 30), ofw.compliance_score) - 0.5 THEN 'declining'
				ELSE 'stable'
			END
		FROM organization_frameworks ofw
		JOIN compliance_frameworks cf ON cf.id = ofw.framework_id
		ON CONFLICT (organization_id, framework_id, measurement_date) DO UPDATE
		SET compliance_score = EXCLUDED.compliance_score,
		    controls_implemented = EXCLUDED.controls_implemented,
		    controls_total = EXCLUDED.controls_total,
		    score_change_7d = EXCLUDED.score_change_7d,
		    score_change_30d = EXCLUDED.score_change_30d,
		    score_change_90d = EXCLUDED.score_change_90d,
		    trend_direction = EXCLUDED.trend_direction
	`)

	if err != nil {
		return fmt.Errorf("calculating trends: %w", err)
	}
	return nil
}
