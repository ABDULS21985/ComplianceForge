package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// WorkflowScheduler checks for SLA breaches, timer expirations, and auto-triggers.
// Runs every 5 minutes via the background worker.
type WorkflowScheduler struct {
	pool *pgxpool.Pool
}

func NewWorkflowScheduler(pool *pgxpool.Pool) *WorkflowScheduler {
	return &WorkflowScheduler{pool: pool}
}

func (ws *WorkflowScheduler) Run(ctx context.Context) error {
	log.Info().Msg("workflow_scheduler: running checks")

	if err := ws.checkSLABreaches(ctx); err != nil {
		log.Error().Err(err).Msg("workflow_scheduler: SLA breach check failed")
	}
	if err := ws.checkTimerSteps(ctx); err != nil {
		log.Error().Err(err).Msg("workflow_scheduler: timer check failed")
	}

	return nil
}

// checkSLABreaches finds step executions where SLA deadline has passed.
func (ws *WorkflowScheduler) checkSLABreaches(ctx context.Context) error {
	now := time.Now().UTC()

	// Mark at_risk (within 80% of SLA)
	_, err := ws.pool.Exec(ctx, `
		UPDATE workflow_step_executions
		SET sla_status = 'at_risk'
		WHERE status IN ('pending', 'in_progress')
		  AND sla_deadline IS NOT NULL
		  AND sla_status = 'on_track'
		  AND sla_deadline - INTERVAL '20%' * (sla_deadline - started_at) <= $1
		  AND sla_deadline > $1
	`, now)
	if err != nil {
		return fmt.Errorf("marking at_risk: %w", err)
	}

	// Mark breached
	tag, err := ws.pool.Exec(ctx, `
		UPDATE workflow_step_executions
		SET sla_status = 'breached', status = 'escalated', escalated_at = $1
		WHERE status IN ('pending', 'in_progress')
		  AND sla_deadline IS NOT NULL
		  AND sla_deadline <= $1
		  AND sla_status != 'breached'
	`, now)
	if err != nil {
		return fmt.Errorf("marking breached: %w", err)
	}

	if tag.RowsAffected() > 0 {
		log.Warn().Int64("count", tag.RowsAffected()).Msg("workflow_scheduler: SLA breaches detected")
	}

	// Also update parent instance SLA
	_, err = ws.pool.Exec(ctx, `
		UPDATE workflow_instances wi
		SET sla_status = 'breached'
		FROM workflow_step_executions wse
		WHERE wse.workflow_instance_id = wi.id
		  AND wse.sla_status = 'breached'
		  AND wi.status = 'active'
		  AND wi.sla_status != 'breached'
	`)
	if err != nil {
		return fmt.Errorf("updating instance SLA: %w", err)
	}

	return nil
}

// checkTimerSteps advances workflows where timer steps have expired.
func (ws *WorkflowScheduler) checkTimerSteps(ctx context.Context) error {
	rows, err := ws.pool.Query(ctx, `
		SELECT wse.id, wse.workflow_instance_id
		FROM workflow_step_executions wse
		JOIN workflow_steps ws ON ws.id = wse.workflow_step_id
		WHERE ws.step_type = 'timer'
		  AND wse.status = 'pending'
		  AND wse.started_at IS NOT NULL
		  AND wse.started_at + (ws.timer_hours * INTERVAL '1 hour') <= NOW()
	`)
	if err != nil {
		return fmt.Errorf("querying expired timers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var execID, instanceID string
		if err := rows.Scan(&execID, &instanceID); err != nil {
			continue
		}
		// Mark timer step as completed
		_, err := ws.pool.Exec(ctx, `
			UPDATE workflow_step_executions
			SET status = 'completed', completed_at = NOW()
			WHERE id = $1
		`, execID)
		if err != nil {
			log.Error().Err(err).Str("execution_id", execID).Msg("workflow_scheduler: failed to complete timer step")
		}
		log.Info().Str("execution_id", execID).Str("instance_id", instanceID).Msg("workflow_scheduler: timer step expired, advancing")
	}

	return nil
}
