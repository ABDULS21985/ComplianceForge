package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
)

// DSRScheduler runs daily to update DSR SLA statuses and emit deadline notifications.
type DSRScheduler struct {
	pool *pgxpool.Pool
}

func NewDSRScheduler(pool *pgxpool.Pool) *DSRScheduler {
	return &DSRScheduler{pool: pool}
}

// Run updates SLA status for all active DSR requests and logs any that are at-risk or overdue.
func (ds *DSRScheduler) Run(ctx context.Context) error {
	log.Info().Msg("dsr_scheduler: running daily SLA status update")
	return runForScheduledTenants(ctx, ds.pool, "DSR lifecycle", ds.runForTenant)
}

func (ds *DSRScheduler) runForTenant(ctx context.Context, organizationID string) error {
	now := time.Now()
	querier := database.QuerierFromContext(ctx, ds.pool)

	// Update overdue: response_deadline < today AND status NOT IN completed/rejected/withdrawn
	tagOverdue, err := querier.Exec(ctx, `
		UPDATE dsr_requests
		SET sla_status = 'overdue',
		    days_remaining = EXTRACT(DAY FROM (
		        (CASE WHEN status = 'extended' THEN COALESCE(extended_deadline, response_deadline)
		              ELSE response_deadline END)::timestamp - $1::timestamp
		    ))::INT
		WHERE status NOT IN ('completed', 'rejected', 'withdrawn')
		  AND deleted_at IS NULL
		  AND (CASE WHEN status = 'extended' THEN COALESCE(extended_deadline, response_deadline)
		            ELSE response_deadline END) < $1::date
		  AND sla_status != 'overdue'
		  AND organization_id = $2::uuid
	`, now, organizationID)
	if err != nil {
		return fmt.Errorf("updating overdue DSRs: %w", err)
	}

	// Update at_risk: deadline within 7 days
	tagAtRisk, err := querier.Exec(ctx, `
		UPDATE dsr_requests
		SET sla_status = 'at_risk',
		    days_remaining = EXTRACT(DAY FROM (
		        (CASE WHEN status = 'extended' THEN COALESCE(extended_deadline, response_deadline)
		              ELSE response_deadline END)::timestamp - $1::timestamp
		    ))::INT
		WHERE status NOT IN ('completed', 'rejected', 'withdrawn')
		  AND deleted_at IS NULL
		  AND (CASE WHEN status = 'extended' THEN COALESCE(extended_deadline, response_deadline)
		            ELSE response_deadline END) >= $1::date
		  AND (CASE WHEN status = 'extended' THEN COALESCE(extended_deadline, response_deadline)
		            ELSE response_deadline END) <= ($1::date + INTERVAL '7 days')
		  AND sla_status != 'at_risk'
		  AND organization_id = $2::uuid
	`, now, organizationID)
	if err != nil {
		return fmt.Errorf("updating at-risk DSRs: %w", err)
	}

	// Update on_track: everything else still active
	tagOnTrack, err := querier.Exec(ctx, `
		UPDATE dsr_requests
		SET sla_status = 'on_track',
		    days_remaining = EXTRACT(DAY FROM (
		        (CASE WHEN status = 'extended' THEN COALESCE(extended_deadline, response_deadline)
		              ELSE response_deadline END)::timestamp - $1::timestamp
		    ))::INT
		WHERE status NOT IN ('completed', 'rejected', 'withdrawn')
		  AND deleted_at IS NULL
		  AND (CASE WHEN status = 'extended' THEN COALESCE(extended_deadline, response_deadline)
		            ELSE response_deadline END) > ($1::date + INTERVAL '7 days')
		  AND sla_status != 'on_track'
		  AND organization_id = $2::uuid
	`, now, organizationID)
	if err != nil {
		return fmt.Errorf("updating on-track DSRs: %w", err)
	}

	log.Info().
		Int64("overdue", tagOverdue.RowsAffected()).
		Int64("at_risk", tagAtRisk.RowsAffected()).
		Int64("on_track", tagOnTrack.RowsAffected()).
		Msg("dsr_scheduler: SLA status update complete")

	return nil
}
