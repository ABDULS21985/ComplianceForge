package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

var ErrDiagnosticsTenantContext = errors.New("diagnostics tenant context does not match request")

// DiagnosticsRepository reads only bounded counts and health metadata. Every
// tenant-owned query includes the organization id even when FORCE RLS is also
// active; infrastructure tables that contain multiple tenants are never
// exposed without an explicit tenant predicate.
type DiagnosticsRepository interface {
	LoadOperationalDiagnostics(context.Context, string) (*models.OperationalDiagnostics, error)
}

type diagnosticsRepo struct{ pool *pgxpool.Pool }

func NewDiagnosticsRepository(pool *pgxpool.Pool) (DiagnosticsRepository, error) {
	if pool == nil {
		return nil, errors.New("diagnostics database pool is required")
	}
	return &diagnosticsRepo{pool: pool}, nil
}

var _ DiagnosticsRepository = (*diagnosticsRepo)(nil)

func (r *diagnosticsRepo) LoadOperationalDiagnostics(
	ctx context.Context, organizationID string,
) (*models.OperationalDiagnostics, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	result := &models.OperationalDiagnostics{}
	var tenantMatches bool
	if err := querier.QueryRow(ctx, `SELECT COALESCE(get_current_tenant()=$1::uuid,false)`, organizationID).Scan(&tenantMatches); err != nil {
		return nil, fmt.Errorf("verify diagnostics tenant context: %w", err)
	}
	if !tenantMatches {
		return nil, ErrDiagnosticsTenantContext
	}

	if err := querier.QueryRow(ctx, `
		SELECT COALESCE(MAX(version),0)::bigint, COALESCE(BOOL_OR(dirty),false)
		FROM schema_migrations`).Scan(
		&result.Migration.CurrentVersion, &result.Migration.Dirty,
	); err != nil {
		return nil, fmt.Errorf("load migration diagnostics: %w", err)
	}

	if err := querier.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status='pending' AND available_at <= statement_timestamp()),
			COUNT(*) FILTER (WHERE status='leased'),
			COUNT(*) FILTER (WHERE status='dead'),
			COUNT(*) FILTER (WHERE status='leased' AND leased_until <= statement_timestamp()),
			COALESCE(GREATEST(0, MAX(EXTRACT(EPOCH FROM statement_timestamp()-available_at))
				FILTER (WHERE status='pending' AND available_at <= statement_timestamp())),0)::bigint
		FROM queue_outbox
		WHERE tenant_id=$1::uuid`, organizationID).Scan(
		&result.Queue.Pending, &result.Queue.Leased, &result.Queue.Dead,
		&result.Queue.ExpiredLeases, &result.Queue.OldestReadyAgeSeconds,
	); err != nil {
		return nil, fmt.Errorf("load outbox diagnostics: %w", err)
	}

	if err := querier.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status='processing'),
			COUNT(*) FILTER (WHERE status='processing' AND leased_until <= statement_timestamp())
		FROM queue_inbox
		WHERE tenant_id=$1::uuid`, organizationID).Scan(
		&result.Queue.InboxProcessing, &result.Queue.InboxExpiredLeases,
	); err != nil {
		return nil, fmt.Errorf("load inbox diagnostics: %w", err)
	}

	if err := querier.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (
				WHERE status IN ('pending','failed') AND dead_at IS NULL
				  AND retry_count < max_retries
				  AND COALESCE(next_retry_at,scheduled_for) <= statement_timestamp()
				  AND (leased_until IS NULL OR leased_until <= statement_timestamp())
			),
			COUNT(*) FILTER (WHERE dead_at IS NULL AND leased_until <= statement_timestamp()),
			COUNT(*) FILTER (WHERE dead_at IS NOT NULL),
			COALESCE(GREATEST(0,MAX(EXTRACT(EPOCH FROM statement_timestamp()-COALESCE(next_retry_at,scheduled_for)))
				FILTER (
					WHERE status IN ('pending','failed') AND dead_at IS NULL
					  AND retry_count < max_retries
					  AND COALESCE(next_retry_at,scheduled_for) <= statement_timestamp()
				)),0)::bigint
		FROM notifications
		WHERE organization_id=$1::uuid`, organizationID).Scan(
		&result.Notifications.Due, &result.Notifications.ExpiredLeases,
		&result.Notifications.TerminalFailures, &result.Notifications.OldestDueAgeSeconds,
	); err != nil {
		return nil, fmt.Errorf("load notification diagnostics: %w", err)
	}

	if err := querier.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE health_status='healthy'),
			COUNT(*) FILTER (WHERE health_status='degraded'),
			COUNT(*) FILTER (WHERE health_status='unhealthy'),
			COUNT(*) FILTER (WHERE health_status='unknown'),
			(SELECT COUNT(*) FROM integration_sync_logs sync_log
			 WHERE sync_log.organization_id=$1::uuid
			   AND sync_log.status='failed'
			   AND sync_log.created_at >= statement_timestamp()-INTERVAL '24 hours')
		FROM integrations
		WHERE organization_id=$1::uuid`, organizationID).Scan(
		&result.Connectors.Total, &result.Connectors.Healthy, &result.Connectors.Degraded,
		&result.Connectors.Unhealthy, &result.Connectors.Unknown,
		&result.Connectors.RecentFailedSyncs,
	); err != nil {
		return nil, fmt.Errorf("load connector diagnostics: %w", err)
	}

	return result, nil
}
