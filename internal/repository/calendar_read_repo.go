package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrCalendarReadScope = errors.New("calendar requires an active tenant-scoped subject")
	ErrCalendarReadLimit = errors.New("calendar candidate budget exceeded")
)

type CalendarReadRepository interface {
	LoadCandidates(context.Context, string, string, models.CalendarEventQuery) ([]models.CalendarEventView, error)
}

type calendarReadRepo struct{ pool *pgxpool.Pool }

var _ CalendarReadRepository = (*calendarReadRepo)(nil)

func NewCalendarReadRepository(pool *pgxpool.Pool) (CalendarReadRepository, error) {
	if pool == nil {
		return nil, errors.New("calendar database pool is required")
	}
	return &calendarReadRepo{pool: pool}, nil
}

func (r *calendarReadRepo) LoadCandidates(ctx context.Context, organizationID, userID string, filter models.CalendarEventQuery) ([]models.CalendarEventView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A pool fallback would bypass the request's tenant setting. The caller
	// must establish its RLS connection before reaching this repository.
	q := database.QuerierFromContext(ctx, nil)
	if r == nil || r.pool == nil || q == nil {
		return nil, ErrCalendarReadScope
	}
	var active bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users subject
		JOIN organizations organization ON organization.id=subject.organization_id
		WHERE organization.id=$1::uuid AND organization.id=get_current_tenant()
		AND organization.status='active' AND organization.deleted_at IS NULL
		AND subject.id=$2::uuid AND subject.status='active' AND subject.deleted_at IS NULL)`, organizationID, userID).Scan(&active); err != nil {
		return nil, fmt.Errorf("validate calendar request scope: %w", err)
	}
	if !active {
		return nil, ErrCalendarReadScope
	}
	search := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(filter.Search)
	rows, err := q.Query(ctx, calendarReadSQL, organizationID, filter.StartDate, filter.EndDate,
		filter.AsOf.UTC().Format("2006-01-02"), filter.EventType, filter.Category, filter.Priority,
		filter.Status, filter.AssignedTo, search, models.CalendarMaximumCandidates+1)
	if err != nil {
		return nil, fmt.Errorf("load calendar candidates: %w", err)
	}
	defer rows.Close()
	items := make([]models.CalendarEventView, 0)
	for rows.Next() {
		var item models.CalendarEventView
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.EventRef, &item.Title, &item.Description,
			&item.EventType, &item.Category, &item.Priority, &item.Status,
			&item.SourceEntityType, &item.SourceEntityID, &item.SourceEntityRef,
			&item.StartDate, &item.EndDate, &item.StartTime, &item.EndTime,
			&item.IsAllDay, &item.Timezone, &item.IsRecurring, &item.AssignedTo,
			&item.CreatedAt, &item.UpdatedAt, &item.AuthorizationResource, &item.AuthorizationResourceID); err != nil {
			return nil, fmt.Errorf("scan calendar candidate: %w", err)
		}
		if len(items) == models.CalendarMaximumCandidates {
			return nil, ErrCalendarReadLimit
		}
		item.AssignmentAvailable = item.AssignedTo != nil
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendar candidates: %w", err)
	}
	return items, nil
}

// The source allowlist is also an existence/tenant/lifecycle guard. Unknown
// polymorphic source types have no authorization anchor and cannot leak into
// any count, metadata or page. No legacy controls/evidence_items tables used.
const calendarReadSQL = `WITH candidates AS (
	SELECT event.*, assignee.id::text AS live_assignee,
	 CASE event.source_entity_type
	  WHEN 'risk' THEN 'risks' WHEN 'policy' THEN 'policies' WHEN 'audit' THEN 'audits'
	  WHEN 'vendor' THEN 'vendors' WHEN 'incident' THEN 'incidents' WHEN 'asset' THEN 'assets'
	  WHEN 'control' THEN 'controls' WHEN 'control_implementation' THEN 'controls'
	  WHEN 'evidence' THEN 'controls' END AS authorization_resource,
	 CASE WHEN event.source_entity_type='evidence' THEN evidence.control_implementation_id
	 ELSE event.source_entity_id END AS authorization_id,
	 CASE WHEN event.status IN ('upcoming','in_progress','overdue') AND event.start_date<$4::date THEN 'overdue'
	 WHEN event.status='overdue' AND event.start_date>=$4::date THEN 'upcoming'
	 ELSE event.status END AS effective_status
	FROM calendar_events event
	LEFT JOIN users assignee ON assignee.organization_id=event.organization_id AND assignee.id=event.assigned_to
	 AND assignee.status='active' AND assignee.deleted_at IS NULL
	LEFT JOIN control_evidence evidence ON event.source_entity_type='evidence'
	 AND evidence.organization_id=event.organization_id AND evidence.id=event.source_entity_id
	 AND evidence.deleted_at IS NULL AND evidence.is_current AND evidence.lifecycle_status='active'
	WHERE event.organization_id=$1::uuid AND event.organization_id=get_current_tenant()
	 AND event.start_date BETWEEN $2::date AND $3::date
	 AND ($5::text='' OR event.event_type=$5) AND ($6::text='' OR event.category=$6)
	 AND ($7::text='' OR event.priority=$7)
	 AND ($9::text='' OR assignee.id=NULLIF($9,'')::uuid)
	 AND ($10::text='' OR event.title ILIKE '%' || $10 || '%' ESCAPE '\')
	 AND CASE event.source_entity_type
	  WHEN 'risk' THEN EXISTS(SELECT 1 FROM risks source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'policy' THEN EXISTS(SELECT 1 FROM policies source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'audit' THEN EXISTS(SELECT 1 FROM audits source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'vendor' THEN EXISTS(SELECT 1 FROM vendors source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'incident' THEN EXISTS(SELECT 1 FROM incidents source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'asset' THEN EXISTS(SELECT 1 FROM assets source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'control' THEN EXISTS(SELECT 1 FROM control_implementations source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'control_implementation' THEN EXISTS(SELECT 1 FROM control_implementations source WHERE source.organization_id=event.organization_id AND source.id=event.source_entity_id AND source.deleted_at IS NULL)
	  WHEN 'evidence' THEN evidence.id IS NOT NULL AND EXISTS(SELECT 1 FROM control_implementations source WHERE source.organization_id=event.organization_id AND source.id=evidence.control_implementation_id AND source.deleted_at IS NULL)
	  ELSE false END
)
SELECT id::text,organization_id::text,COALESCE(event_ref,''),title,COALESCE(LEFT(description,4000),''),
	event_type,COALESCE(category,''),COALESCE(priority,''),effective_status,
	source_entity_type,source_entity_id::text,COALESCE(source_entity_ref,''),
	start_date::text,end_date::text,start_time::text,end_time::text,is_all_day,timezone,is_recurring,
	live_assignee,created_at,updated_at,authorization_resource,authorization_id::text
FROM candidates WHERE ($8::text='' OR effective_status=$8)
ORDER BY start_date,id LIMIT $11`
