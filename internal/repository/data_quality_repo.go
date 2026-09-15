package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

type DataQualityRepository interface {
	LoadDataQualityCounts(context.Context, string, string) (*models.DataQualityCounts, error)
}

// dataQualityRepo intentionally holds no pool: this read-only endpoint must
// use the request-bound connection carrying the tenant's FORCE-RLS context.
type dataQualityRepo struct{}

func NewDataQualityRepository() DataQualityRepository { return &dataQualityRepo{} }

var _ DataQualityRepository = (*dataQualityRepo)(nil)

// The fixed projection and clock share one PostgreSQL statement/MVCC snapshot.
// Parent joins intentionally retain soft-deleted/inactive historical parents.
// Nullable SET NULL references are tested only when populated. No business
// values or identifiers are selected into the returned projection.
const dataQualityCountsQuery = `
WITH request_scope AS MATERIALIZED (
    SELECT statement_timestamp() AS as_of,
           COALESCE(get_current_tenant()=$1::uuid,false)
           AND EXISTS (
               SELECT 1
               FROM organizations organization
               JOIN users actor ON actor.organization_id=organization.id
               WHERE organization.id=$1::uuid AND organization.status='active'
                 AND organization.deleted_at IS NULL
                 AND actor.id=$2::uuid AND actor.status='active'
                 AND actor.deleted_at IS NULL
           ) AS allowed,
           (SELECT COUNT(*)=1 AND COALESCE(BOOL_AND(version=$3::bigint AND NOT dirty),false)
            FROM schema_migrations) AS schema_ready,
           NOT EXISTS (
               SELECT 1 FROM control_implementations child
               JOIN organization_frameworks adoption
                 ON adoption.organization_id=child.organization_id AND adoption.id=child.org_framework_id
               JOIN compliance_frameworks framework ON framework.id=adoption.framework_id
               WHERE child.organization_id=$1::uuid AND framework.deleted_at IS NOT NULL
                 AND (framework.organization_id=child.organization_id
                      OR (framework.organization_id IS NULL AND framework.is_system_framework))
           ) AS scope_complete
)
SELECT scope.as_of, scope.allowed, scope.schema_ready, scope.scope_complete,
       CASE WHEN scope.allowed AND scope.schema_ready AND scope.scope_complete THEN ARRAY[
           (SELECT COUNT(*) FROM control_implementations child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM organization_frameworks parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.org_framework_id)),
           (SELECT COUNT(*) FROM control_implementations child
            JOIN organization_frameworks adoption ON adoption.organization_id=child.organization_id AND adoption.id=child.org_framework_id
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM framework_controls control
                              WHERE control.id=child.framework_control_id AND control.framework_id=adoption.framework_id)),
           (SELECT COUNT(*) FROM policy_versions child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM policies parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.policy_id)),
           (SELECT COUNT(*) FROM policies child
            WHERE child.organization_id=$1::uuid AND child.current_version_id IS NOT NULL
              AND NOT EXISTS (SELECT 1 FROM policy_versions parent
                              WHERE parent.organization_id=child.organization_id AND parent.policy_id=child.id
                                AND parent.id=child.current_version_id AND parent.version_number=child.current_version)),
           (SELECT COUNT(*) FROM policy_approval_workflows child
            WHERE child.organization_id=$1::uuid
              AND (NOT EXISTS (SELECT 1 FROM policies parent
                               WHERE parent.organization_id=child.organization_id AND parent.id=child.policy_id)
                   OR (child.policy_version_id IS NOT NULL AND NOT EXISTS (
                       SELECT 1 FROM policy_versions version
                       WHERE version.organization_id=child.organization_id AND version.policy_id=child.policy_id
                         AND version.id=child.policy_version_id)))),
           (SELECT COUNT(*) FROM policy_approval_steps child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM policy_approval_workflows parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.workflow_id)),
           (SELECT COUNT(*) FROM risk_assessments child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM risks parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.risk_id)),
           (SELECT COUNT(*) FROM risk_treatments child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM risks parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.risk_id)),
           (SELECT COUNT(*) FROM risk_indicators child
            WHERE child.organization_id=$1::uuid AND child.risk_id IS NOT NULL
              AND NOT EXISTS (SELECT 1 FROM risks parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.risk_id)),
           (SELECT COUNT(*) FROM risk_indicator_values child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM risk_indicators parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.indicator_id)),
           (SELECT COUNT(*) FROM audit_findings child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM audits parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.audit_id)),
           (SELECT COUNT(*) FROM assets child
            WHERE child.organization_id=$1::uuid AND child.linked_vendor_id IS NOT NULL
              AND NOT EXISTS (SELECT 1 FROM vendors parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.linked_vendor_id)),
           (SELECT COUNT(*) FROM notifications child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM users parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.recipient_user_id)),
           (SELECT COUNT(*) FROM notifications child
            WHERE child.organization_id=$1::uuid AND child.channel_id IS NOT NULL
              AND NOT EXISTS (SELECT 1 FROM notification_channels parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.channel_id
                                AND parent.channel_type=child.channel_type)),
           (SELECT COUNT(*) FROM notifications child
            WHERE child.organization_id=$1::uuid AND child.parent_notification_id IS NOT NULL
              AND NOT EXISTS (SELECT 1 FROM notifications parent
                              WHERE parent.organization_id=child.organization_id AND parent.id=child.parent_notification_id)),
           (SELECT COUNT(*) FROM user_roles child
            WHERE child.organization_id=$1::uuid
              AND NOT EXISTS (SELECT 1 FROM roles parent
                              WHERE parent.id=child.role_id
                                AND (parent.organization_id=child.organization_id
                                     OR (parent.organization_id IS NULL AND parent.is_system_role)))),
           (SELECT COUNT(*) FROM notifications child
            WHERE child.organization_id=$1::uuid AND child.channel_type='email'
              AND child.status IN ('pending','failed') AND child.dead_at IS NULL
              AND child.retry_count < child.max_retries
              AND COALESCE(child.next_retry_at,child.scheduled_for) <= scope.as_of
              AND (child.leased_until IS NULL OR child.leased_until <= scope.as_of)
              AND EXISTS (SELECT 1 FROM users recipient
                          WHERE recipient.organization_id=child.organization_id AND recipient.id=child.recipient_user_id
                            AND (recipient.status<>'active' OR recipient.deleted_at IS NOT NULL))),
           (SELECT COUNT(*) FROM directory_group_memberships child
            WHERE child.organization_id=$1::uuid AND child.removed_at IS NULL
              AND (EXISTS (SELECT 1 FROM directory_groups parent
                           WHERE parent.organization_id=child.organization_id AND parent.id=child.group_id
                             AND parent.deleted_at IS NOT NULL)
                   OR EXISTS (SELECT 1 FROM users subject
                              WHERE subject.organization_id=child.organization_id AND subject.id=child.user_id
                                AND subject.deprovisioned_at IS NOT NULL)))
       ]::bigint[] ELSE ARRAY[]::bigint[] END
FROM request_scope scope`

func (r *dataQualityRepo) LoadDataQualityCounts(ctx context.Context, organizationID, actorID string) (*models.DataQualityCounts, error) {
	if !validDataQualityScopeUUID(organizationID) || !validDataQualityScopeUUID(actorID) {
		return nil, models.ErrDataQualityScope
	}
	querier := database.QuerierFromContext(ctx, nil)
	if querier == nil || ctx.Err() != nil {
		return nil, models.ErrDataQualityUnavailable
	}
	result := &models.DataQualityCounts{
		OrganizationID: organizationID, ActorID: actorID, SchemaVersion: database.SupportedSchemaVersion,
	}
	var allowed, schemaReady, scopeComplete bool
	var counts []int64
	if err := querier.QueryRow(ctx, dataQualityCountsQuery, organizationID, actorID, database.SupportedSchemaVersion).Scan(
		&result.AsOf, &allowed, &schemaReady, &scopeComplete, &counts,
	); err != nil {
		return nil, models.ErrDataQualityUnavailable
	}
	if !allowed {
		return nil, models.ErrDataQualityScope
	}
	if !schemaReady || !scopeComplete {
		return nil, models.ErrDataQualityUnavailable
	}
	definitions := models.DataQualityCheckDefinitions()
	if len(counts) != len(definitions) {
		return nil, errors.New("data quality fixed count projection is incomplete")
	}
	result.Counts = make([]models.DataQualityCount, len(definitions))
	for index, definition := range definitions {
		if counts[index] < 0 || counts[index] > models.MaximumDataQualityCount {
			return nil, models.ErrDataQualityUnavailable
		}
		result.Counts[index] = models.DataQualityCount{Key: definition.Key, Count: counts[index]}
	}
	return result, nil
}

func validDataQualityScopeUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
