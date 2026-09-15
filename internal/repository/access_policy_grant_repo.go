package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/accesscontrol"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const accessObjectGrantColumns = `object_grant.id,object_grant.organization_id,object_grant.user_id,object_grant.entity_type,
	object_grant.entity_id,object_grant.actions,
	CASE WHEN object_grant.status='approved' AND object_grant.expires_at <= NOW() THEN 'expired' ELSE object_grant.status END,
	COALESCE(object_grant.granted_by::text,''),object_grant.approved_by,object_grant.approved_at,object_grant.valid_from,
	object_grant.expires_at,object_grant.allow_download,object_grant.require_watermark,COALESCE(object_grant.watermark_text,''),
	COALESCE(object_grant.grant_reason,''),object_grant.version,object_grant.created_at,object_grant.updated_at,
	object_grant.revoked_at,object_grant.revoked_by`

func (r *policyAccessRepo) ListObjectGrants(
	ctx context.Context, organizationID string, filter models.AccessObjectGrantFilter,
) ([]models.AccessObjectGrant, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	statusExpression := `CASE WHEN object_grant.status='approved' AND object_grant.expires_at <= NOW() THEN 'expired' ELSE object_grant.status END`
	predicate := ` FROM user_entity_permissions object_grant
		WHERE object_grant.organization_id=$1::uuid
		  AND ($2='' OR object_grant.user_id=$2::uuid)
		  AND ($3='' OR object_grant.entity_type=$3)
		  AND ($4='' OR object_grant.entity_id=$4::uuid)
		  AND ($5='' OR ` + statusExpression + `=$5)
		  AND ($6::timestamptz IS NULL OR (
			object_grant.status='approved' AND object_grant.valid_from <= $6 AND object_grant.expires_at > $6
		  ))`
	args := []any{organizationID, filter.SubjectID, filter.ResourceType, filter.ResourceID, string(filter.Status), filter.ActiveAt}
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)`+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count object grants: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT `+accessObjectGrantColumns+predicate+`
		ORDER BY object_grant.created_at DESC,object_grant.id LIMIT $7 OFFSET $8`,
		append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list object grants: %w", err)
	}
	defer rows.Close()
	items := make([]models.AccessObjectGrant, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanAccessObjectGrant(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan object grant: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate object grants: %w", err)
	}
	return items, total, nil
}

func (r *policyAccessRepo) CreateObjectGrant(
	ctx context.Context, organizationID, actorID, requestID string, input models.AccessObjectGrantInput,
) (*models.AccessObjectGrant, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var created *models.AccessObjectGrant
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := requireActiveTenantUser(ctx, tx, organizationID, input.SubjectID); err != nil {
			return err
		}
		if err := requireActiveTenantUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		if _, err := loadAccessResource(ctx, tx, organizationID, input.ResourceType, input.ResourceID); err != nil {
			return err
		}
		permissionLevel := objectGrantPermissionLevel(input.Actions)
		var err error
		created, err = scanAccessObjectGrant(tx.QueryRow(ctx, `INSERT INTO user_entity_permissions AS object_grant (
			user_id,organization_id,entity_type,entity_id,permission_level,actions,status,granted_by,
			valid_from,expires_at,allow_download,require_watermark,watermark_text,grant_reason
		) VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,'pending',$7::uuid,$8,$9,$10,$11,NULLIF($12,''),$13)
		RETURNING `+accessObjectGrantColumns,
			input.SubjectID, organizationID, input.ResourceType, input.ResourceID, permissionLevel,
			input.Actions, actorID, input.ValidFrom, input.ValidUntil, input.AllowDownload,
			input.RequireWatermark, input.WatermarkText, input.Reason))
		if err != nil {
			return classifyPolicyAccessWrite("create object grant", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, "", "object_grant", created.ID,
			"object_grant_requested", actorID, requestID, input.Reason, nil, created)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *policyAccessRepo) DecideObjectGrant(
	ctx context.Context, organizationID, grantID, actorID, requestID string,
	input models.AccessObjectGrantDecisionInput,
) (*models.AccessObjectGrant, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.AccessObjectGrant
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := lockAccessObjectGrant(ctx, tx, organizationID, grantID)
		if err != nil {
			return err
		}
		if before.Version != input.ExpectedVersion {
			return accesscontrol.ErrConflict
		}
		if before.Status != models.AccessObjectGrantPending {
			return accesscontrol.ErrState
		}
		if before.SubjectID == actorID {
			return accesscontrol.ErrInvalidRelation
		}
		if err := requireActiveTenantUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		status := models.AccessObjectGrantApproved
		eventType := "object_grant_approved"
		if input.Decision == "reject" {
			status = models.AccessObjectGrantRejected
			eventType = "object_grant_rejected"
		}
		result, err = scanAccessObjectGrant(tx.QueryRow(ctx, `UPDATE user_entity_permissions object_grant SET
			status=$3::varchar,approved_by=CASE WHEN $3::varchar='approved' THEN $4::uuid ELSE NULL END,
			approved_at=CASE WHEN $3::varchar='approved' THEN NOW() ELSE NULL END,
			decision_reason=$5,updated_at=NOW(),version=version+1
			WHERE object_grant.organization_id=$1::uuid AND object_grant.id=$2::uuid AND object_grant.version=$6
			RETURNING `+accessObjectGrantColumns,
			organizationID, grantID, status, actorID, input.Reason, input.ExpectedVersion))
		if errors.Is(err, pgx.ErrNoRows) {
			return accesscontrol.ErrConflict
		}
		if err != nil {
			return classifyPolicyAccessWrite("decide object grant", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, "", "object_grant", grantID,
			eventType, actorID, requestID, input.Reason, before, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *policyAccessRepo) RevokeObjectGrant(
	ctx context.Context, organizationID, grantID, actorID, requestID string,
	input models.AccessObjectGrantRevocationInput,
) (*models.AccessObjectGrant, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.AccessObjectGrant
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := lockAccessObjectGrant(ctx, tx, organizationID, grantID)
		if err != nil {
			return err
		}
		if before.Version != input.ExpectedVersion {
			return accesscontrol.ErrConflict
		}
		if before.Status != models.AccessObjectGrantApproved {
			return accesscontrol.ErrState
		}
		if err := requireActiveTenantUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		result, err = scanAccessObjectGrant(tx.QueryRow(ctx, `UPDATE user_entity_permissions object_grant SET
			status='revoked',revoked_by=$3::uuid,revoked_at=NOW(),revocation_reason=$4,
			updated_at=NOW(),version=version+1
			WHERE object_grant.organization_id=$1::uuid AND object_grant.id=$2::uuid AND object_grant.version=$5
			RETURNING `+accessObjectGrantColumns,
			organizationID, grantID, actorID, input.Reason, input.ExpectedVersion))
		if errors.Is(err, pgx.ErrNoRows) {
			return accesscontrol.ErrConflict
		}
		if err != nil {
			return classifyPolicyAccessWrite("revoke object grant", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, "", "object_grant", grantID,
			"object_grant_revoked", actorID, requestID, input.Reason, before, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func lockAccessObjectGrant(
	ctx context.Context, querier database.Querier, organizationID, grantID string,
) (*models.AccessObjectGrant, error) {
	item, err := scanAccessObjectGrant(querier.QueryRow(ctx, `SELECT `+accessObjectGrantColumns+`
		FROM user_entity_permissions object_grant
		WHERE object_grant.organization_id=$1::uuid AND object_grant.id=$2::uuid FOR UPDATE`, organizationID, grantID))
	return item, mapPolicyAccessNotFound("lock object grant", err)
}

func requireActiveTenantUser(ctx context.Context, querier database.Querier, organizationID, userID string) error {
	var exists bool
	if err := querier.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users
		WHERE organization_id=$1::uuid AND id=$2::uuid AND status='active' AND deleted_at IS NULL)`,
		organizationID, userID).Scan(&exists); err != nil {
		return fmt.Errorf("validate object grant user: %w", err)
	}
	if !exists {
		return accesscontrol.ErrInvalidRelation
	}
	return nil
}

func objectGrantPermissionLevel(actions []string) string {
	for _, action := range actions {
		if action == "approve" {
			return "approver"
		}
	}
	for _, action := range actions {
		if action == "update" || action == "delete" || action == "assign" {
			return "editor"
		}
	}
	return "viewer"
}

func normalizedGrantStatus(value models.AccessObjectGrantStatus) string {
	return strings.ToLower(strings.TrimSpace(string(value)))
}
