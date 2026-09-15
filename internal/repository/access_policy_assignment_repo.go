package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/accesscontrol"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

func (r *policyAccessRepo) ListAssignments(
	ctx context.Context, organizationID, policyID string,
) ([]models.AccessPolicyAssignment, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT id,organization_id,
		access_policy_id,assignee_type,assignee_id,valid_from,valid_until,created_by,created_at
		FROM access_policy_assignments
		WHERE organization_id=$1::uuid AND access_policy_id=$2::uuid
		ORDER BY assignee_type,assignee_id NULLS FIRST,id`, organizationID, policyID)
	if err != nil {
		return nil, fmt.Errorf("list access policy assignments: %w", err)
	}
	defer rows.Close()
	items := []models.AccessPolicyAssignment{}
	for rows.Next() {
		item, err := scanAccessPolicyAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan access policy assignment: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access policy assignments: %w", err)
	}
	return items, nil
}

func (r *policyAccessRepo) CreateAssignment(
	ctx context.Context, organizationID, policyID, actorID, requestID string,
	input models.AccessPolicyAssignmentInput,
) (*models.AccessPolicyAssignment, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var created *models.AccessPolicyAssignment
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if _, err := lockAccessPolicy(ctx, tx, organizationID, policyID); err != nil {
			return err
		}
		if err := validatePolicyAssignee(ctx, tx, organizationID, input); err != nil {
			return err
		}
		var err error
		created, err = scanAccessPolicyAssignment(tx.QueryRow(ctx, `INSERT INTO access_policy_assignments (
			organization_id,access_policy_id,assignee_type,assignee_id,valid_from,valid_until,created_by
		) VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::uuid)
		RETURNING id,organization_id,access_policy_id,assignee_type,assignee_id,valid_from,valid_until,created_by,created_at`,
			organizationID, policyID, input.AssigneeType, input.AssigneeID, input.ValidFrom, input.ValidUntil, actorID))
		if err != nil {
			return classifyPolicyAccessWrite("create access policy assignment", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, policyID, "assignment", created.ID,
			"assignment_created", actorID, requestID, input.Reason, nil, created)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *policyAccessRepo) RemoveAssignment(
	ctx context.Context, organizationID, policyID, assignmentID, actorID, requestID, reason string,
) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		assignment, err := scanAccessPolicyAssignment(tx.QueryRow(ctx, `SELECT id,organization_id,
			access_policy_id,assignee_type,assignee_id,valid_from,valid_until,created_by,created_at
			FROM access_policy_assignments WHERE organization_id=$1::uuid AND access_policy_id=$2::uuid
			AND id=$3::uuid FOR UPDATE`, organizationID, policyID, assignmentID))
		if errors.Is(err, pgx.ErrNoRows) {
			return accesscontrol.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock access policy assignment: %w", err)
		}
		tag, err := tx.Exec(ctx, `DELETE FROM access_policy_assignments
			WHERE organization_id=$1::uuid AND access_policy_id=$2::uuid AND id=$3::uuid`,
			organizationID, policyID, assignmentID)
		if err != nil {
			return fmt.Errorf("remove access policy assignment: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return accesscontrol.ErrConflict
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, policyID, "assignment", assignmentID,
			"assignment_removed", actorID, requestID, reason, assignment, nil)
	})
}

func scanAccessPolicyAssignment(row policyAccessRowScanner) (*models.AccessPolicyAssignment, error) {
	item := new(models.AccessPolicyAssignment)
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.PolicyID, &item.AssigneeType, &item.AssigneeID,
		&item.ValidFrom, &item.ValidUntil, &item.CreatedBy, &item.CreatedAt,
	); err != nil {
		return nil, err
	}
	return item, nil
}

func validatePolicyAssignee(
	ctx context.Context, querier database.Querier, organizationID string, input models.AccessPolicyAssignmentInput,
) error {
	if input.AssigneeType == models.AccessAssigneeAllUsers {
		return nil
	}
	if input.AssigneeID == nil {
		return accesscontrol.ErrInvalidRelation
	}
	var exists bool
	var err error
	switch input.AssigneeType {
	case models.AccessAssigneeUser:
		err = querier.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users
			WHERE organization_id=$1::uuid AND id=$2::uuid AND status='active' AND deleted_at IS NULL)`,
			organizationID, *input.AssigneeID).Scan(&exists)
	case models.AccessAssigneeRole:
		err = querier.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles
			WHERE id=$2::uuid AND deleted_at IS NULL
			AND (organization_id=$1::uuid OR (organization_id IS NULL AND is_system_role)))`,
			organizationID, *input.AssigneeID).Scan(&exists)
	case models.AccessAssigneeGroup:
		err = querier.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM directory_groups
			WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
			organizationID, *input.AssigneeID).Scan(&exists)
	default:
		return accesscontrol.ErrInvalidRelation
	}
	if err != nil {
		return fmt.Errorf("validate access policy assignee: %w", err)
	}
	if !exists {
		return accesscontrol.ErrInvalidRelation
	}
	return nil
}

func (r *policyAccessRepo) ListFieldPermissions(
	ctx context.Context, organizationID, policyID, resourceType string,
) ([]models.AccessFieldPermission, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT field.id,field.organization_id,
		field.access_policy_id,policy.priority,field.resource_type,field.field_path,field.classification,
		field.visibility,COALESCE(field.mask_strategy,''),COALESCE(field.mask_pattern,''),field.version,field.created_at
		FROM field_level_permissions field
		JOIN access_policies policy ON policy.organization_id=field.organization_id AND policy.id=field.access_policy_id
		WHERE field.organization_id=$1::uuid AND policy.deleted_at IS NULL
		  AND ($2='' OR field.access_policy_id=$2::uuid)
		  AND ($3='' OR field.resource_type=$3)
		ORDER BY field.resource_type,field.field_path,policy.priority,field.id`, organizationID, policyID, resourceType)
	if err != nil {
		return nil, fmt.Errorf("list field permissions: %w", err)
	}
	defer rows.Close()
	items := []models.AccessFieldPermission{}
	for rows.Next() {
		item, err := scanAccessFieldPermission(rows)
		if err != nil {
			return nil, fmt.Errorf("scan field permission: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate field permissions: %w", err)
	}
	return items, nil
}

func (r *policyAccessRepo) UpsertFieldPermission(
	ctx context.Context, organizationID, policyID, actorID, requestID string,
	input models.AccessFieldPermissionInput,
) (*models.AccessFieldPermission, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.AccessFieldPermission
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		policy, err := lockAccessPolicy(ctx, tx, organizationID, policyID)
		if err != nil {
			return err
		}
		var existing *models.AccessFieldPermission
		existing, err = scanAccessFieldPermission(tx.QueryRow(ctx, `SELECT field.id,field.organization_id,
			field.access_policy_id,$3::int,field.resource_type,field.field_path,field.classification,
			field.visibility,COALESCE(field.mask_strategy,''),COALESCE(field.mask_pattern,''),field.version,field.created_at
			FROM field_level_permissions field WHERE field.organization_id=$1::uuid
			AND field.access_policy_id=$2::uuid AND field.resource_type=$4 AND field.field_path=$5 FOR UPDATE`,
			organizationID, policyID, policy.Priority, input.ResourceType, input.FieldPath))
		if errors.Is(err, pgx.ErrNoRows) {
			existing = nil
		} else if err != nil {
			return fmt.Errorf("lock field permission: %w", err)
		}
		if existing == nil && input.ExpectedVersion != nil {
			return accesscontrol.ErrConflict
		}
		if existing != nil && (input.ExpectedVersion == nil || existing.Version != *input.ExpectedVersion) {
			return accesscontrol.ErrConflict
		}
		if existing == nil {
			result, err = scanAccessFieldPermission(tx.QueryRow(ctx, `INSERT INTO field_level_permissions AS field (
				organization_id,access_policy_id,resource_type,field_path,classification,visibility,
				mask_strategy,mask_pattern,created_by,updated_by
			) VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9::uuid,$9::uuid)
			RETURNING field.id,field.organization_id,field.access_policy_id,$10::int,field.resource_type,
				field.field_path,field.classification,field.visibility,COALESCE(field.mask_strategy,''),
				COALESCE(field.mask_pattern,''),field.version,field.created_at`,
				organizationID, policyID, input.ResourceType, input.FieldPath, input.Classification,
				input.Visibility, input.MaskStrategy, input.MaskPattern, actorID, policy.Priority))
		} else {
			result, err = scanAccessFieldPermission(tx.QueryRow(ctx, `UPDATE field_level_permissions field SET
				classification=$5,visibility=$6,mask_strategy=NULLIF($7,''),mask_pattern=NULLIF($8,''),
				updated_by=$9::uuid,version=version+1
				WHERE field.organization_id=$1::uuid AND field.access_policy_id=$2::uuid
				AND field.resource_type=$3 AND field.field_path=$4 AND field.version=$10
			RETURNING field.id,field.organization_id,field.access_policy_id,$11::int,field.resource_type,
				field.field_path,field.classification,field.visibility,COALESCE(field.mask_strategy,''),
				COALESCE(field.mask_pattern,''),field.version,field.created_at`,
				organizationID, policyID, input.ResourceType, input.FieldPath, input.Classification,
				input.Visibility, input.MaskStrategy, input.MaskPattern, actorID, *input.ExpectedVersion, policy.Priority))
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return accesscontrol.ErrConflict
		}
		if err != nil {
			return classifyPolicyAccessWrite("save field permission", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, policyID, "field_permission", result.ID,
			"field_permission_saved", actorID, requestID, input.Reason, existing, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *policyAccessRepo) DeleteFieldPermission(
	ctx context.Context, organizationID, policyID, fieldPermissionID, actorID, requestID string,
	expectedVersion int64, reason string,
) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		policy, err := lockAccessPolicy(ctx, tx, organizationID, policyID)
		if err != nil {
			return err
		}
		before, err := scanAccessFieldPermission(tx.QueryRow(ctx, `SELECT field.id,field.organization_id,
			field.access_policy_id,$4::int,field.resource_type,field.field_path,field.classification,
			field.visibility,COALESCE(field.mask_strategy,''),COALESCE(field.mask_pattern,''),field.version,field.created_at
			FROM field_level_permissions field WHERE field.organization_id=$1::uuid
			AND field.access_policy_id=$2::uuid AND field.id=$3::uuid FOR UPDATE`,
			organizationID, policyID, fieldPermissionID, policy.Priority))
		if errors.Is(err, pgx.ErrNoRows) {
			return accesscontrol.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock field permission: %w", err)
		}
		if before.Version != expectedVersion {
			return accesscontrol.ErrConflict
		}
		tag, err := tx.Exec(ctx, `DELETE FROM field_level_permissions
			WHERE organization_id=$1::uuid AND access_policy_id=$2::uuid AND id=$3::uuid AND version=$4`,
			organizationID, policyID, fieldPermissionID, expectedVersion)
		if err != nil {
			return fmt.Errorf("delete field permission: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return accesscontrol.ErrConflict
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, policyID, "field_permission", fieldPermissionID,
			"field_permission_deleted", actorID, requestID, reason, before, nil)
	})
}

func scanAccessFieldPermission(row policyAccessRowScanner) (*models.AccessFieldPermission, error) {
	item := new(models.AccessFieldPermission)
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.PolicyID, &item.PolicyPriority,
		&item.ResourceType, &item.FieldPath, &item.Classification, &item.Visibility,
		&item.MaskStrategy, &item.MaskPattern, &item.Version, &item.CreatedAt,
	); err != nil {
		return nil, err
	}
	return item, nil
}
