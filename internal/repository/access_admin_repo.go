package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var (
	ErrManagedRoleConflict        = errors.New("managed role conflicts with an existing role")
	ErrManagedRoleVersionConflict = errors.New("managed role version conflict")
	ErrManagedRoleImmutable       = errors.New("system role is immutable")
	ErrManagedRoleInUse           = errors.New("managed role is assigned to users")
	ErrManagedRolePermission      = errors.New("managed role contains an unknown permission")
	ErrManagedRoleAssignment      = errors.New("managed role assignment is invalid")
	ErrLastTenantAdministrator    = errors.New("cannot remove the tenant's final administrative grant")
)

type AccessAdministrationRepository interface {
	ListPermissions(context.Context) ([]models.PermissionGrant, error)
	CreateRole(context.Context, string, string, models.ManagedRoleCreateInput) (*models.ManagedRole, error)
	GetRole(context.Context, string, string) (*models.ManagedRole, error)
	ListRoles(context.Context, string, models.ManagedRoleListFilter) ([]models.ManagedRole, int, error)
	UpdateRole(context.Context, string, string, string, models.ManagedRolePatch) (*models.ManagedRole, error)
	DeleteRole(context.Context, string, string, string, int64) error
	CloneRole(context.Context, string, string, string, models.ManagedRoleCloneInput) (*models.ManagedRole, error)
	PreviewImpact(context.Context, string, string, []models.PermissionGrant) (*models.ManagedRoleImpact, error)
	ListAssignments(context.Context, string, string) ([]models.ManagedRoleAssignment, error)
	AssignRole(context.Context, string, string, string, models.ManagedRoleAssignmentInput) error
	UnassignRole(context.Context, string, string, string, string, models.ManagedRoleUnassignmentInput) error
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.RoleChangeEvent, int, error)
}

type accessAdministrationRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

func NewAccessAdministrationRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (AccessAdministrationRepository, error) {
	if pool == nil {
		return nil, errors.New("access administration database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("access administration outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("access administration outbox queue is required")
	}
	return &accessAdministrationRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

var _ AccessAdministrationRepository = (*accessAdministrationRepo)(nil)

func (r *accessAdministrationRepo) ListPermissions(ctx context.Context) ([]models.PermissionGrant, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT id,resource,action::text,COALESCE(description,'')
		FROM permissions ORDER BY resource,action::text,id`)
	if err != nil {
		return nil, fmt.Errorf("list permission catalogue: %w", err)
	}
	defer rows.Close()
	permissions := make([]models.PermissionGrant, 0, 64)
	for rows.Next() {
		var permission models.PermissionGrant
		if err := rows.Scan(&permission.ID, &permission.Resource, &permission.Action, &permission.Description); err != nil {
			return nil, fmt.Errorf("scan permission catalogue: %w", err)
		}
		permissions = append(permissions, permission)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permission catalogue: %w", err)
	}
	return permissions, nil
}

func (r *accessAdministrationRepo) CreateRole(ctx context.Context, organizationID, actorID string, input models.ManagedRoleCreateInput) (*models.ManagedRole, error) {
	return r.createRole(ctx, organizationID, actorID, input, "created", "")
}

func (r *accessAdministrationRepo) createRole(ctx context.Context, organizationID, actorID string, input models.ManagedRoleCreateInput, eventType, sourceRoleID string) (*models.ManagedRole, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var created *models.ManagedRole
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		var roleID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO roles
				(organization_id,name,slug,description,is_system_role,is_custom,created_by,updated_by)
			VALUES ($1::uuid,$2,$3,NULLIF($4,''),false,true,$5::uuid,$5::uuid)
			RETURNING id`, organizationID, input.Name, input.Slug, input.Description, actorID).Scan(&roleID); err != nil {
			return classifyManagedRoleWrite(err)
		}
		if err := replaceManagedRolePermissions(ctx, tx, roleID, input.Permissions); err != nil {
			return err
		}
		var err error
		created, err = getManagedRoleWithQuerier(ctx, tx, organizationID, roleID)
		if err != nil {
			return err
		}
		details := map[string]any{}
		if sourceRoleID != "" {
			details["source_role_id"] = sourceRoleID
		}
		return r.recordManagedRoleEvent(ctx, tx, organizationID, created, actorID, nil, eventType, "", nil, roleState(created), details)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *accessAdministrationRepo) GetRole(ctx context.Context, organizationID, roleID string) (*models.ManagedRole, error) {
	return getManagedRoleWithQuerier(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, roleID)
}

const managedRoleColumns = `
	r.id,r.organization_id,r.name,r.slug,COALESCE(r.description,''),
	r.is_system_role,r.is_custom,r.version,r.created_by,r.updated_by,
	r.created_at,r.updated_at,r.deleted_at,
	COALESCE((
		SELECT jsonb_agg(jsonb_build_object(
			'id',p.id,'resource',p.resource,'action',p.action::text,
			'description',COALESCE(p.description,'')) ORDER BY p.resource,p.action::text,p.id)
		FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id
		WHERE rp.role_id=r.id
	),'[]'::jsonb),
	(SELECT COUNT(*)::int FROM user_roles ur WHERE ur.role_id=r.id AND ur.organization_id=$1::uuid)`

func getManagedRoleWithQuerier(ctx context.Context, querier database.Querier, organizationID, roleID string) (*models.ManagedRole, error) {
	return scanManagedRole(querier.QueryRow(ctx, `SELECT `+managedRoleColumns+`
		FROM roles r
		WHERE r.id=$2::uuid AND r.deleted_at IS NULL
		  AND (r.organization_id=$1::uuid OR (r.organization_id IS NULL AND r.is_system_role))`, organizationID, roleID))
}

func (r *accessAdministrationRepo) ListRoles(ctx context.Context, organizationID string, filter models.ManagedRoleListFilter) ([]models.ManagedRole, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	predicate := ` FROM roles r
		WHERE r.deleted_at IS NULL
		  AND (r.organization_id=$1::uuid OR ($2 AND r.organization_id IS NULL AND r.is_system_role))
		  AND ($3='' OR r.name ILIKE '%%'||$3||'%%' OR r.slug ILIKE '%%'||$3||'%%'
		       OR COALESCE(r.description,'') ILIKE '%%'||$3||'%%')`
	args := []any{organizationID, filter.IncludeSystem, filter.Search}
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)`+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count managed roles: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT `+managedRoleColumns+predicate+`
		ORDER BY r.is_system_role DESC,lower(r.name),r.id LIMIT $4 OFFSET $5`,
		organizationID, filter.IncludeSystem, filter.Search, filter.PageSize, (filter.Page-1)*filter.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list managed roles: %w", err)
	}
	defer rows.Close()
	roles := make([]models.ManagedRole, 0, filter.PageSize)
	for rows.Next() {
		role, err := scanManagedRole(rows)
		if err != nil {
			return nil, 0, err
		}
		roles = append(roles, *role)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate managed roles: %w", err)
	}
	return roles, total, nil
}

func scanManagedRole(scanner interface{ Scan(...any) error }) (*models.ManagedRole, error) {
	var role models.ManagedRole
	var permissionJSON []byte
	if err := scanner.Scan(
		&role.ID, &role.OrganizationID, &role.Name, &role.Slug, &role.Description,
		&role.IsSystemRole, &role.IsCustom, &role.Version, &role.CreatedBy, &role.UpdatedBy,
		&role.CreatedAt, &role.UpdatedAt, &role.DeletedAt, &permissionJSON, &role.AssignedUsers,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(permissionJSON, &role.Permissions); err != nil {
		return nil, fmt.Errorf("decode managed role permissions: %w", err)
	}
	if role.Permissions == nil {
		role.Permissions = []models.PermissionGrant{}
	}
	return &role, nil
}

func (r *accessAdministrationRepo) UpdateRole(ctx context.Context, organizationID, roleID, actorID string, patch models.ManagedRolePatch) (*models.ManagedRole, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var updated *models.ManagedRole
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		before, err := lockManagedCustomRole(ctx, tx, organizationID, roleID)
		if err != nil {
			return err
		}
		if before.Version != patch.ExpectedVersion {
			return ErrManagedRoleVersionConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE roles SET
			name=COALESCE($3,name),slug=COALESCE($4,slug),
			description=CASE WHEN $6 THEN NULL WHEN $5::text IS NULL THEN description ELSE NULLIF($5,'') END,
			updated_by=$7::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL`,
			organizationID, roleID, patch.Name, patch.Slug, patch.Description, patch.ClearDescription, actorID); err != nil {
			return classifyManagedRoleWrite(err)
		}
		if patch.Permissions != nil {
			if managedRoleHasPermission(before.Permissions, "settings", "configure") &&
				!managedRoleHasPermission(*patch.Permissions, "settings", "configure") {
				wouldLockOut, err := removingAdministrativeGrantWouldLockOut(ctx, tx, organizationID, roleID, "")
				if err != nil {
					return err
				}
				if wouldLockOut {
					return ErrLastTenantAdministrator
				}
			}
			if err := replaceManagedRolePermissions(ctx, tx, roleID, *patch.Permissions); err != nil {
				return err
			}
		}
		updated, err = getManagedRoleWithQuerier(ctx, tx, organizationID, roleID)
		if err != nil {
			return err
		}
		return r.recordManagedRoleEvent(ctx, tx, organizationID, updated, actorID, nil, "updated", "", roleState(before), roleState(updated), nil)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *accessAdministrationRepo) DeleteRole(ctx context.Context, organizationID, roleID, actorID string, expectedVersion int64) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		before, err := lockManagedCustomRole(ctx, tx, organizationID, roleID)
		if err != nil {
			return err
		}
		if before.Version != expectedVersion {
			return ErrManagedRoleVersionConflict
		}
		if before.AssignedUsers > 0 {
			return ErrManagedRoleInUse
		}
		if _, err := tx.Exec(ctx, `UPDATE roles SET deleted_at=NOW(),updated_by=$3::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL`, organizationID, roleID, actorID); err != nil {
			return fmt.Errorf("delete managed role: %w", err)
		}
		eventRole := *before
		eventRole.Version++
		return r.recordManagedRoleEvent(ctx, tx, organizationID, &eventRole, actorID, nil, "deleted", "", roleState(before), map[string]any{"deleted": true}, nil)
	})
}

func (r *accessAdministrationRepo) CloneRole(ctx context.Context, organizationID, sourceRoleID, actorID string, input models.ManagedRoleCloneInput) (*models.ManagedRole, error) {
	source, err := r.GetRole(ctx, organizationID, sourceRoleID)
	if err != nil {
		return nil, err
	}
	description := input.Description
	if description == "" {
		description = source.Description
	}
	return r.createRole(ctx, organizationID, actorID, models.ManagedRoleCreateInput{
		Name: input.Name, Slug: input.Slug, Description: description, Permissions: source.Permissions,
	}, "cloned", sourceRoleID)
}

func (r *accessAdministrationRepo) PreviewImpact(ctx context.Context, organizationID, roleID string, proposed []models.PermissionGrant) (*models.ManagedRoleImpact, error) {
	role, err := r.GetRole(ctx, organizationID, roleID)
	if err != nil {
		return nil, err
	}
	if _, err := resolveManagedPermissions(ctx, database.QuerierFromContext(ctx, r.pool), proposed); err != nil {
		return nil, err
	}
	current := make(map[string]models.PermissionGrant, len(role.Permissions))
	for _, permission := range role.Permissions {
		current[permission.Resource+":"+permission.Action] = permission
	}
	next := make(map[string]models.PermissionGrant, len(proposed))
	for _, permission := range proposed {
		next[permission.Resource+":"+permission.Action] = permission
	}
	impact := &models.ManagedRoleImpact{
		RoleID: role.ID, AssignedUsers: role.AssignedUsers,
		CurrentPermissions: len(current), ProposedPermissions: len(next),
		Added: []models.PermissionGrant{}, Removed: []models.PermissionGrant{},
	}
	for key, permission := range next {
		if _, exists := current[key]; !exists {
			impact.Added = append(impact.Added, permission)
		}
	}
	for key, permission := range current {
		if _, exists := next[key]; !exists {
			impact.Removed = append(impact.Removed, permission)
		}
	}
	sortPermissionGrants(impact.Added)
	sortPermissionGrants(impact.Removed)
	return impact, nil
}

func (r *accessAdministrationRepo) ListAssignments(ctx context.Context, organizationID, roleID string) ([]models.ManagedRoleAssignment, error) {
	if _, err := r.GetRole(ctx, organizationID, roleID); err != nil {
		return nil, err
	}
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT ur.assignment_id,ur.role_id,ur.user_id,u.email,COALESCE(u.first_name,''),COALESCE(u.last_name,''),ur.assigned_by,ur.assigned_at,
		NULLIF(ur.valid_from,'-infinity'::timestamptz),ur.expires_at,ur.version
		FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id
		WHERE ur.organization_id=$1::uuid AND ur.role_id=$2::uuid AND u.deleted_at IS NULL
		ORDER BY lower(u.email),u.id`, organizationID, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role assignments: %w", err)
	}
	defer rows.Close()
	items := make([]models.ManagedRoleAssignment, 0)
	for rows.Next() {
		var item models.ManagedRoleAssignment
		if err := rows.Scan(&item.AssignmentID, &item.RoleID, &item.UserID, &item.Email, &item.FirstName, &item.LastName, &item.AssignedBy, &item.AssignedAt,
			&item.ValidFrom, &item.ExpiresAt, &item.Version); err != nil {
			return nil, fmt.Errorf("scan role assignment: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role assignments: %w", err)
	}
	return items, nil
}

func (r *accessAdministrationRepo) AssignRole(ctx context.Context, organizationID, roleID, actorID string, input models.ManagedRoleAssignmentInput) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		role, err := getManagedRoleWithQuerier(ctx, tx, organizationID, roleID)
		if err != nil {
			return err
		}
		if err := ensureManagedRoleUser(ctx, tx, organizationID, input.UserID); err != nil {
			return err
		}
		if input.ExpiresAt != nil {
			if actorID == input.UserID {
				return ErrAccessGovernanceSeparation
			}
			if err := ensureIndependentPermanentAdmin(ctx, tx, organizationID, roleID, "00000000-0000-0000-0000-000000000000", input.UserID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by,valid_from,expires_at,assignment_reason,window_approved_by)
			VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,COALESCE($5::timestamptz,'-infinity'::timestamptz),$6,$7,CASE WHEN $6::timestamptz IS NOT NULL THEN $4::uuid ELSE NULL END)`, input.UserID, roleID, organizationID, actorID, input.ValidFrom, input.ExpiresAt, input.Reason); err != nil {
			return classifyManagedRoleAssignment(err)
		}
		return r.recordManagedRoleEvent(ctx, tx, organizationID, role, actorID, &input.UserID, "assigned", input.Reason, nil,
			map[string]any{"role_id": roleID, "user_id": input.UserID}, nil)
	})
}

func (r *accessAdministrationRepo) UnassignRole(ctx context.Context, organizationID, roleID, userID, actorID string, input models.ManagedRoleUnassignmentInput) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		role, err := getManagedRoleWithQuerier(ctx, tx, organizationID, roleID)
		if err != nil {
			return err
		}
		if managedRoleHasPermission(role.Permissions, "settings", "configure") {
			wouldLockOut, err := removingAdministrativeGrantWouldLockOut(ctx, tx, organizationID, roleID, userID)
			if err != nil {
				return err
			}
			if wouldLockOut {
				return ErrLastTenantAdministrator
			}
		}
		result, err := tx.Exec(ctx, `DELETE FROM user_roles
			WHERE organization_id=$1::uuid AND role_id=$2::uuid AND user_id=$3::uuid`, organizationID, roleID, userID)
		if err != nil {
			return fmt.Errorf("remove role assignment: %w", err)
		}
		if result.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return r.recordManagedRoleEvent(ctx, tx, organizationID, role, actorID, &userID, "unassigned", input.Reason,
			map[string]any{"role_id": roleID, "user_id": userID}, nil, nil)
	})
}

func (r *accessAdministrationRepo) ListEvents(ctx context.Context, organizationID, roleID string, pagination models.PaginationRequest) ([]models.RoleChangeEvent, int, error) {
	if _, err := r.GetRole(ctx, organizationID, roleID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, err
	}
	querier := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*) FROM role_change_events
		WHERE organization_id=$1::uuid AND role_id=$2::uuid`, organizationID, roleID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count role events: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT id,role_id,target_user_id,event_type,actor_user_id,
		role_version,COALESCE(reason,''),before_state,after_state,created_at
		FROM role_change_events WHERE organization_id=$1::uuid AND role_id=$2::uuid
		ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, organizationID, roleID,
		pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list role events: %w", err)
	}
	defer rows.Close()
	items := make([]models.RoleChangeEvent, 0, pagination.PageSize)
	for rows.Next() {
		var item models.RoleChangeEvent
		if err := rows.Scan(&item.ID, &item.RoleID, &item.TargetUserID, &item.EventType, &item.ActorUserID,
			&item.RoleVersion, &item.Reason, &item.BeforeState, &item.AfterState, &item.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan role event: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate role events: %w", err)
	}
	return items, total, nil
}

func lockManagedCustomRole(ctx context.Context, tx pgx.Tx, organizationID, roleID string) (*models.ManagedRole, error) {
	var isSystem bool
	if err := tx.QueryRow(ctx, `SELECT is_system_role FROM roles
		WHERE id=$2::uuid AND (organization_id=$1::uuid OR (organization_id IS NULL AND is_system_role))
		  AND deleted_at IS NULL`, organizationID, roleID).Scan(&isSystem); err != nil {
		return nil, err
	}
	if isSystem {
		return nil, ErrManagedRoleImmutable
	}
	if err := tx.QueryRow(ctx, `SELECT is_system_role FROM roles
		WHERE id=$2::uuid AND organization_id=$1::uuid AND deleted_at IS NULL
		FOR UPDATE`, organizationID, roleID).Scan(&isSystem); err != nil {
		return nil, err
	}
	return getManagedRoleWithQuerier(ctx, tx, organizationID, roleID)
}

func replaceManagedRolePermissions(ctx context.Context, tx pgx.Tx, roleID string, grants []models.PermissionGrant) error {
	resolved, err := resolveManagedPermissions(ctx, tx, grants)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1::uuid`, roleID); err != nil {
		return fmt.Errorf("clear managed role permissions: %w", err)
	}
	for _, permission := range resolved {
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_id) VALUES ($1::uuid,$2::uuid)`, roleID, permission.ID); err != nil {
			return fmt.Errorf("grant managed role permission: %w", err)
		}
	}
	return nil
}

func resolveManagedPermissions(ctx context.Context, querier database.Querier, grants []models.PermissionGrant) ([]models.PermissionGrant, error) {
	resolved := make([]models.PermissionGrant, 0, len(grants))
	seen := make(map[string]bool, len(grants))
	for _, grant := range grants {
		key := grant.Resource + ":" + grant.Action
		if seen[key] {
			continue
		}
		seen[key] = true
		var permission models.PermissionGrant
		if err := querier.QueryRow(ctx, `SELECT id,resource,action::text,COALESCE(description,'')
			FROM permissions WHERE resource=$1 AND action::text=$2`, grant.Resource, grant.Action).Scan(
			&permission.ID, &permission.Resource, &permission.Action, &permission.Description,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("%w: %s", ErrManagedRolePermission, key)
			}
			return nil, fmt.Errorf("resolve managed role permission: %w", err)
		}
		resolved = append(resolved, permission)
	}
	sortPermissionGrants(resolved)
	return resolved, nil
}

func ensureManagedRoleUser(ctx context.Context, querier database.Querier, organizationID, userID string) error {
	var valid bool
	if err := querier.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users
		WHERE organization_id=$1::uuid AND id=$2::uuid AND status='active' AND deleted_at IS NULL)`, organizationID, userID).Scan(&valid); err != nil {
		return fmt.Errorf("validate managed role assignee: %w", err)
	}
	if !valid {
		return ErrManagedRoleAssignment
	}
	return nil
}

func managedRoleHasPermission(values []models.PermissionGrant, resource, action string) bool {
	for _, value := range values {
		if value.Resource == resource && value.Action == action {
			return true
		}
	}
	return false
}

func removingAdministrativeGrantWouldLockOut(ctx context.Context, querier database.Querier, organizationID, roleID, userID string) (bool, error) {
	var affected, remaining bool
	if err := querier.QueryRow(ctx, `
		SELECT
		EXISTS (
			SELECT 1
			FROM users u
			JOIN effective_user_roles ur ON ur.user_id=u.id AND ur.organization_id=u.organization_id
			WHERE u.organization_id=$1::uuid AND u.status='active' AND u.deleted_at IS NULL
			  AND ur.role_id=$2::uuid AND ($3='' OR ur.user_id=$3::uuid)
		),
		EXISTS (
			SELECT 1
			FROM users u
			WHERE u.organization_id=$1::uuid AND u.status='active' AND u.deleted_at IS NULL
			  AND (
				u.is_super_admin
				OR EXISTS (
					SELECT 1 FROM effective_user_roles ur
					JOIN roles role ON role.id=ur.role_id AND role.deleted_at IS NULL
					JOIN role_permissions rp ON rp.role_id=role.id
					JOIN permissions permission ON permission.id=rp.permission_id
					WHERE ur.user_id=u.id AND ur.organization_id=u.organization_id
					  AND permission.resource='settings' AND permission.action::text='configure'
					  AND ur.expires_at IS NULL
					  AND NOT EXISTS(SELECT 1 FROM access_sod_rules rule WHERE rule.organization_id=ur.organization_id
					    AND rule.enabled AND ur.role_id IN(rule.role_a_id,rule.role_b_id))
					  AND NOT (ur.role_id=$2::uuid AND ($3='' OR ur.user_id=$3::uuid))
				)
			  )
		)`, organizationID, roleID, userID).Scan(&affected, &remaining); err != nil {
		return false, fmt.Errorf("check remaining tenant administrators: %w", err)
	}
	return affected && !remaining, nil
}

func (r *accessAdministrationRepo) recordManagedRoleEvent(
	ctx context.Context, tx pgx.Tx, organizationID string, role *models.ManagedRole, actorID string, targetUserID *string,
	eventType, reason string, before, after any, details map[string]any,
) error {
	beforeJSON, err := marshalNullableObject(before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalNullableObject(after)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO role_change_events
		(organization_id,role_id,target_user_id,event_type,actor_user_id,role_version,reason,before_state,after_state)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6,NULLIF($7,''),$8::jsonb,$9::jsonb)`,
		organizationID, role.ID, targetUserID, eventType, actorID, role.Version, reason, beforeJSON, afterJSON); err != nil {
		return fmt.Errorf("append managed role event: %w", err)
	}
	data := map[string]any{
		"role_id": role.ID, "role_name": role.Name, "role_slug": role.Slug,
		"role_version": role.Version, "target_user_id": targetUserID, "actor_user_id": actorID,
	}
	for key, value := range details {
		data[key] = value
	}
	payload := map[string]any{
		"type": "access.role." + eventType, "severity": "medium", "org_id": organizationID,
		"entity_type": "role", "entity_id": role.ID, "entity_ref": role.Slug,
		"data": data, "timestamp": time.Now().UTC(),
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", organizationID, payload)
	if err != nil {
		return fmt.Errorf("create managed role outbox envelope: %w", err)
	}
	envelope.CausationID = role.ID
	envelope.Metadata = map[string]string{"entity_type": "role", "entity_id": role.ID, "event_type": eventType}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue managed role event: %w", err)
	}
	return nil
}

func roleState(role *models.ManagedRole) map[string]any {
	return map[string]any{
		"id": role.ID, "name": role.Name, "slug": role.Slug, "description": role.Description,
		"version": role.Version, "permissions": role.Permissions, "assigned_users": role.AssignedUsers,
	}
}

func marshalNullableObject(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode managed role audit state: %w", err)
	}
	return encoded, nil
}

func classifyManagedRoleWrite(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return ErrManagedRoleConflict
	}
	return fmt.Errorf("persist managed role: %w", err)
}

func classifyManagedRoleAssignment(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23505" || postgresError.Code == "23503" || postgresError.Code == "23514") {
		return ErrManagedRoleAssignment
	}
	return fmt.Errorf("persist managed role assignment: %w", err)
}

func sortPermissionGrants(values []models.PermissionGrant) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0; j-- {
			left, right := values[j-1], values[j]
			if left.Resource < right.Resource || (left.Resource == right.Resource && left.Action <= right.Action) {
				break
			}
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}
