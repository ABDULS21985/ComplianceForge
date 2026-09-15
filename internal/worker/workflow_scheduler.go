package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
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
	return runForScheduledTenants(ctx, ws.pool, "workflow SLA", ws.runForTenant)
}

func (ws *WorkflowScheduler) runForTenant(ctx context.Context, organizationID string) error {
	if err := ws.checkSLABreaches(ctx, organizationID); err != nil {
		log.Error().Err(err).Msg("workflow_scheduler: SLA breach check failed")
		return err
	}
	if err := ws.checkTimerSteps(ctx, organizationID); err != nil {
		log.Error().Err(err).Msg("workflow_scheduler: timer check failed")
		return err
	}

	return nil
}

// checkSLABreaches finds step executions where SLA deadline has passed.
func (ws *WorkflowScheduler) checkSLABreaches(ctx context.Context, organizationID string) error {
	now := time.Now().UTC()
	querier := database.QuerierFromContext(ctx, ws.pool)

	// Mark at_risk (within 80% of SLA)
	_, err := querier.Exec(ctx, `
		UPDATE workflow_step_executions
		SET sla_status = 'at_risk'
		WHERE organization_id = $2::uuid
		  AND status IN ('pending', 'in_progress')
		  AND sla_deadline IS NOT NULL
		  AND sla_status = 'on_track'
		  AND sla_deadline - ((sla_deadline - started_at) * 0.2) <= $1
		  AND sla_deadline > $1
	`, now, organizationID)
	if err != nil {
		return fmt.Errorf("marking at_risk: %w", err)
	}

	// Mark breached
	tag, err := querier.Exec(ctx, `
		UPDATE workflow_step_executions
		SET sla_status = 'breached', status = 'escalated', escalated_at = $1
		WHERE organization_id = $2::uuid
		  AND status IN ('pending', 'in_progress')
		  AND sla_deadline IS NOT NULL
		  AND sla_deadline <= $1
		  AND sla_status != 'breached'
	`, now, organizationID)
	if err != nil {
		return fmt.Errorf("marking breached: %w", err)
	}

	if tag.RowsAffected() > 0 {
		log.Warn().Int64("count", tag.RowsAffected()).Msg("workflow_scheduler: SLA breaches detected")
	}

	// Also update parent instance SLA
	_, err = querier.Exec(ctx, `
		UPDATE workflow_instances wi
		SET sla_status = 'breached'
		FROM workflow_step_executions wse
		WHERE wi.organization_id = $1::uuid
		  AND wse.organization_id = wi.organization_id
		  AND wse.workflow_instance_id = wi.id
		  AND wse.sla_status = 'breached'
		  AND wi.status = 'active'
		  AND wi.sla_status != 'breached'
	`, organizationID)
	if err != nil {
		return fmt.Errorf("updating instance SLA: %w", err)
	}

	return nil
}

// checkTimerSteps advances workflows where timer steps have expired.
func (ws *WorkflowScheduler) checkTimerSteps(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, ws.pool)
	rows, err := querier.Query(ctx, `
		SELECT wse.id, wse.workflow_instance_id
		FROM workflow_step_executions wse
		JOIN workflow_steps ws ON ws.organization_id = wse.organization_id AND ws.id = wse.workflow_step_id
		WHERE wse.organization_id = $1::uuid
		  AND ws.step_type = 'timer'
		  AND wse.status = 'pending'
		  AND wse.started_at IS NOT NULL
		  AND wse.started_at + (ws.timer_hours * INTERVAL '1 hour') <= NOW()
	`, organizationID)
	if err != nil {
		return fmt.Errorf("querying expired timers: %w", err)
	}
	type timerExecution struct{ executionID, instanceID string }
	executions := make([]timerExecution, 0)
	for rows.Next() {
		var execution timerExecution
		if err := rows.Scan(&execution.executionID, &execution.instanceID); err != nil {
			rows.Close()
			return fmt.Errorf("scan expired timer step: %w", err)
		}
		executions = append(executions, execution)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return fmt.Errorf("iterate expired timer steps: %w", rowErr)
	}

	for _, execution := range executions {
		// Mark timer step as completed
		_, err := querier.Exec(ctx, `
			UPDATE workflow_step_executions
			SET status = 'completed', completed_at = NOW()
			WHERE id = $1 AND organization_id = $2::uuid
		`, execution.executionID, organizationID)
		if err != nil {
			return fmt.Errorf("complete timer step %s: %w", execution.executionID, err)
		}
		log.Info().Str("execution_id", execution.executionID).Str("instance_id", execution.instanceID).Msg("workflow_scheduler: timer step expired, advancing")
	}

	return nil
}
