package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var (
	ErrDirectoryUserNotFound      = errors.New("directory user not found")
	ErrDirectoryGroupNotFound     = errors.New("directory group not found")
	ErrDirectoryConflict          = errors.New("directory record conflicts with existing data")
	ErrDirectoryVersionConflict   = errors.New("directory record version conflict")
	ErrDirectoryInvalidUser       = errors.New("directory user is not active in this tenant")
	ErrDirectoryLastAdmin         = errors.New("tenant must retain an active administrator")
	ErrDirectoryOwnershipRequired = errors.New("ownership replacement is required")
	ErrDirectoryDynamicGroup      = errors.New("dynamic group membership is rule-managed")
	ErrDirectoryImportConflict    = errors.New("idempotency key was used with different import content")
	ErrDirectoryImportInvalid     = errors.New("directory import contains invalid rows")
)

type UserAdministrationRepository interface {
	CreateUser(context.Context, string, string, models.DirectoryUserCreateInput) (*models.DirectoryUser, error)
	GetUser(context.Context, string, string) (*models.DirectoryUser, error)
	ListUsers(context.Context, string, models.DirectoryUserListFilter) ([]models.DirectoryUser, int, error)
	UpdateUser(context.Context, string, string, *models.DirectoryUser, int64, string) (*models.DirectoryUser, error)
	SuspendUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error)
	ReactivateUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error)
	PreviewOwnership(context.Context, string, string) (*models.DirectoryOwnershipImpact, error)
	TransferOwnership(context.Context, string, string, string, models.DirectoryOwnershipTransferInput) (*models.DirectoryOwnershipImpact, *models.DirectoryUser, error)
	DeprovisionUser(context.Context, string, string, string, models.DirectoryUserDeprovisionInput) error
	ListEvents(context.Context, string, string, string, models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error)

	CreateGroup(context.Context, string, string, models.DirectoryGroupCreateInput) (*models.DirectoryGroup, error)
	GetGroup(context.Context, string, string) (*models.DirectoryGroup, error)
	ListGroups(context.Context, string, models.DirectoryGroupListFilter) ([]models.DirectoryGroup, int, error)
	UpdateGroup(context.Context, string, string, string, models.DirectoryGroupPatch) (*models.DirectoryGroup, error)
	DeleteGroup(context.Context, string, string, string, int64, string) error
	ListGroupMembers(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryUser, int, error)
	ChangeGroupMembers(context.Context, string, string, string, models.DirectoryGroupBulkMembersInput) (*models.DirectoryGroup, error)

	PreviewImport(context.Context, string, []models.DirectoryImportRow, string) (*models.DirectoryImportPreview, error)
	ApplyImport(context.Context, string, string, string, string, string, []models.DirectoryImportRow) (*models.DirectoryImportResult, error)
}

type userAdministrationRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

var _ UserAdministrationRepository = (*userAdministrationRepo)(nil)

func NewUserAdministrationRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (UserAdministrationRepository, error) {
	if pool == nil {
		return nil, errors.New("user administration database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("user administration outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("user administration outbox queue is required")
	}
	return &userAdministrationRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

const directoryUserSelect = `
	SELECT u.id,u.organization_id,u.email,COALESCE(u.first_name,''),COALESCE(u.last_name,''),
		COALESCE(u.job_title,''),COALESCE(u.department,''),COALESCE(u.phone,''),COALESCE(u.avatar_url,''),
		u.status::text,u.is_super_admin,COALESCE(u.timezone,''),COALESCE(u.language,'en'),u.last_login_at,
		EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id=u.id AND m.is_verified),
		u.manager_user_id,manager.email,manager.first_name,manager.last_name,
		COALESCE(u.employee_id,''),COALESCE(u.location,''),u.invitation_status,
		u.invited_at,u.invitation_expires_at,u.invited_by,u.suspended_at,u.suspended_by,
		COALESCE(u.suspension_reason,''),u.reactivated_at,u.deprovisioned_at,u.deprovisioned_by,
		COALESCE(u.deprovision_reason,''),u.updated_by,u.version,
		COALESCE((SELECT jsonb_agg(role.slug ORDER BY role.slug)
			FROM effective_user_roles ur JOIN roles role ON role.id=ur.role_id AND role.deleted_at IS NULL
			WHERE ur.organization_id=u.organization_id AND ur.user_id=u.id),'[]'::jsonb),
		(SELECT COUNT(*) FROM directory_group_memberships gm
			WHERE gm.organization_id=u.organization_id AND gm.user_id=u.id AND gm.removed_at IS NULL),
		u.created_at,u.updated_at,u.deleted_at
	FROM users u
	LEFT JOIN users manager ON manager.organization_id=u.organization_id AND manager.id=u.manager_user_id`

type directoryRowScanner interface{ Scan(...any) error }

func scanDirectoryUser(row directoryRowScanner) (*models.DirectoryUser, error) {
	item := &models.DirectoryUser{}
	var status, invitation string
	var managerEmail, managerFirst, managerLast *string
	var rolesJSON []byte
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.Email, &item.FirstName, &item.LastName,
		&item.JobTitle, &item.Department, &item.Phone, &item.AvatarURL, &status,
		&item.IsSuperAdmin, &item.Timezone, &item.Language, &item.LastLoginAt, &item.MFAEnabled,
		&item.ManagerUserID, &managerEmail, &managerFirst, &managerLast,
		&item.EmployeeID, &item.Location, &invitation, &item.InvitedAt, &item.InvitationExpiresAt,
		&item.InvitedBy, &item.SuspendedAt, &item.SuspendedBy, &item.SuspensionReason,
		&item.ReactivatedAt, &item.DeprovisionedAt, &item.DeprovisionedBy, &item.DeprovisionReason,
		&item.UpdatedBy, &item.Version, &rolesJSON, &item.GroupCount,
		&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	); err != nil {
		return nil, err
	}
	item.Status = models.UserStatus(status)
	item.InvitationStatus = models.DirectoryInvitationStatus(invitation)
	if item.ManagerUserID != nil {
		item.Manager = &models.DirectoryUserReference{ID: *item.ManagerUserID}
		if managerEmail != nil {
			item.Manager.Email = *managerEmail
		}
		if managerFirst != nil {
			item.Manager.FirstName = *managerFirst
		}
		if managerLast != nil {
			item.Manager.LastName = *managerLast
		}
	}
	if err := json.Unmarshal(rolesJSON, &item.RoleSlugs); err != nil {
		return nil, fmt.Errorf("decode directory user roles: %w", err)
	}
	if item.RoleSlugs == nil {
		item.RoleSlugs = []string{}
	}
	return item, nil
}

func (r *userAdministrationRepo) CreateUser(ctx context.Context, organizationID, actorID string, input models.DirectoryUserCreateInput) (*models.DirectoryUser, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var created *models.DirectoryUser
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		if input.ManagerUserID != nil {
			if err := ensureDirectoryAssignableUser(ctx, tx, organizationID, *input.ManagerUserID); err != nil {
				return err
			}
		}
		roleID, roleVersion, err := resolveDirectoryRole(ctx, tx, organizationID, input.InitialRoleSlug)
		if err != nil {
			return err
		}
		var duplicate bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE organization_id=$1::uuid
			AND (lower(email)=lower($2) OR ($3<>'' AND deleted_at IS NULL AND lower(COALESCE(employee_id,''))=lower($3))))`,
			organizationID, input.Email, input.EmployeeID).Scan(&duplicate); err != nil {
			return fmt.Errorf("check duplicate directory user: %w", err)
		}
		if duplicate {
			return ErrDirectoryConflict
		}
		if err := EnsureEntitlementCapacity(ctx, tx, organizationID, "users", 1); err != nil {
			return err
		}
		id := uuid.NewString()
		invitation := models.DirectoryInvitationReady
		var invitedAt *time.Time
		var invitedBy *string
		if input.InitialStatus == models.UserStatusActive {
			invitation = models.DirectoryInvitationNotRequired
		} else {
			now := time.Now().UTC()
			invitedAt, invitedBy = &now, &actorID
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users(
			id,organization_id,email,first_name,last_name,job_title,department,phone,avatar_url,status,
			timezone,language,manager_user_id,employee_id,location,invitation_status,invited_at,
			invitation_expires_at,invited_by,updated_by)
			VALUES($1::uuid,$2::uuid,lower($3),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),
			NULLIF($8,''),NULLIF($9,''),$10::user_status,NULLIF($11,''),$12,$13::uuid,NULLIF($14,''),
			NULLIF($15,''),$16,$17,$18,$19::uuid,$20::uuid)`,
			id, organizationID, input.Email, input.FirstName, input.LastName, input.JobTitle, input.Department,
			input.Phone, input.AvatarURL, input.InitialStatus, input.Timezone, input.Language, input.ManagerUserID,
			input.EmployeeID, input.Location, invitation, invitedAt, input.InvitationExpiresAt, invitedBy, actorID); err != nil {
			return classifyDirectoryWrite(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, id, roleID, organizationID, actorID); err != nil {
			return classifyDirectoryWrite(err)
		}
		if err := appendInitialDirectoryRoleEvent(ctx, tx, organizationID, roleID, roleVersion, id, actorID, input.Reason); err != nil {
			return err
		}
		created, err = getDirectoryUser(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "user", id, &id, "user_created", actorID, created.Version, input.Reason, nil, directoryUserState(created))
	})
	return created, err
}

func (r *userAdministrationRepo) GetUser(ctx context.Context, organizationID, userID string) (*models.DirectoryUser, error) {
	item, err := getDirectoryUser(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, userID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDirectoryUserNotFound
	}
	return item, err
}

func getDirectoryUser(ctx context.Context, q database.Querier, organizationID, userID string, includeDeleted bool) (*models.DirectoryUser, error) {
	predicate := " AND u.deleted_at IS NULL"
	if includeDeleted {
		predicate = ""
	}
	return scanDirectoryUser(q.QueryRow(ctx, directoryUserSelect+` WHERE u.organization_id=$1::uuid AND u.id=$2::uuid`+predicate, organizationID, userID))
}

func (r *userAdministrationRepo) ListUsers(ctx context.Context, organizationID string, filter models.DirectoryUserListFilter) ([]models.DirectoryUser, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := []string{"u.organization_id=$1::uuid", "u.deleted_at IS NULL"}
	args := []any{organizationID}
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if filter.GroupID != "" {
		group, err := getDirectoryGroup(ctx, q, organizationID, filter.GroupID, false)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrDirectoryGroupNotFound
		}
		if err != nil {
			return nil, 0, fmt.Errorf("load directory group filter: %w", err)
		}
		if group.GroupType == models.DirectoryGroupDynamic {
			predicate, dynamicArgs, err := dynamicGroupPredicate(organizationID, group.MembershipRule)
			if err != nil {
				return nil, 0, err
			}
			where, args = []string{predicate}, dynamicArgs
		} else {
			add(`EXISTS(SELECT 1 FROM directory_group_memberships filter_gm
				WHERE filter_gm.organization_id=u.organization_id AND filter_gm.user_id=u.id
				AND filter_gm.group_id=$%d::uuid AND filter_gm.removed_at IS NULL)`, filter.GroupID)
		}
	}
	if filter.Search != "" {
		add(`u.directory_search @@ websearch_to_tsquery('simple',$%d)`, filter.Search)
	}
	if filter.Status != "" {
		add(`u.status::text=$%d`, filter.Status)
	}
	if filter.Department != "" {
		add(`lower(COALESCE(u.department,''))=lower($%d)`, filter.Department)
	}
	if filter.Location != "" {
		add(`lower(COALESCE(u.location,''))=lower($%d)`, filter.Location)
	}
	if filter.ManagerID != "" {
		add(`u.manager_user_id=$%d::uuid`, filter.ManagerID)
	}
	if filter.RoleSlug != "" {
		add(`EXISTS(SELECT 1 FROM effective_user_roles filter_ur JOIN roles filter_role ON filter_role.id=filter_ur.role_id
			WHERE filter_ur.organization_id=u.organization_id AND filter_ur.user_id=u.id
			AND filter_role.deleted_at IS NULL AND filter_role.slug=$%d)`, filter.RoleSlug)
	}
	predicate := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM users u WHERE `+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count directory users: %w", err)
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := q.Query(ctx, directoryUserSelect+` WHERE `+predicate+` ORDER BY `+directoryUserSortColumn(filter.SortBy)+` `+filter.SortDirection+`,u.id `+filter.SortDirection+fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list directory users: %w", err)
	}
	defer rows.Close()
	items := make([]models.DirectoryUser, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanDirectoryUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan directory user: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate directory users: %w", err)
	}
	return items, total, nil
}

func (r *userAdministrationRepo) UpdateUser(ctx context.Context, organizationID, actorID string, next *models.DirectoryUser, expectedVersion int64, reason string) (*models.DirectoryUser, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var updated *models.DirectoryUser
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryUser(ctx, tx, organizationID, next.ID, expectedVersion)
		if err != nil {
			return err
		}
		if next.ManagerUserID != nil {
			if err := ensureDirectoryManagerAssignment(ctx, tx, organizationID, next.ID, *next.ManagerUserID); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE users SET email=lower($4),first_name=NULLIF($5,''),last_name=NULLIF($6,''),
			job_title=NULLIF($7,''),department=NULLIF($8,''),phone=NULLIF($9,''),avatar_url=NULLIF($10,''),
			timezone=NULLIF($11,''),language=$12,manager_user_id=$13::uuid,employee_id=NULLIF($14,''),
			location=NULLIF($15,''),updated_by=$3::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$16 AND deleted_at IS NULL`,
			organizationID, next.ID, actorID, next.Email, next.FirstName, next.LastName, next.JobTitle,
			next.Department, next.Phone, next.AvatarURL, next.Timezone, next.Language, next.ManagerUserID,
			next.EmployeeID, next.Location, expectedVersion)
		if err != nil {
			return classifyDirectoryWrite(err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDirectoryVersionConflict
		}
		updated, err = getDirectoryUser(ctx, tx, organizationID, next.ID, false)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "user", next.ID, &next.ID, "user_updated", actorID, updated.Version, reason, directoryUserState(current), directoryUserState(updated))
	})
	return updated, err
}

func (r *userAdministrationRepo) SuspendUser(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var updated *models.DirectoryUser
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := lockDirectoryOrganization(ctx, tx, organizationID); err != nil {
			return err
		}
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryUser(ctx, tx, organizationID, userID, input.ExpectedVersion)
		if err != nil {
			return err
		}
		if current.Status == models.UserStatusInactive {
			return ErrDirectoryConflict
		}
		if last, err := directoryLastAdministrator(ctx, tx, organizationID, userID); err != nil {
			return err
		} else if last {
			return ErrDirectoryLastAdmin
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE users SET status='inactive',suspended_at=$4,suspended_by=$3::uuid,
			suspension_reason=$5,invitation_status=CASE WHEN invitation_status IN ('ready','sent') THEN 'revoked' ELSE invitation_status END,
			updated_by=$3::uuid,version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$6`,
			organizationID, userID, actorID, now, input.Reason, input.ExpectedVersion); err != nil {
			return classifyDirectoryWrite(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,$3)
			WHERE organization_id=$1::uuid AND user_id=$2::uuid AND revoked_at IS NULL`, organizationID, userID, now); err != nil {
			return fmt.Errorf("revoke suspended user sessions: %w", err)
		}
		updated, err = getDirectoryUser(ctx, tx, organizationID, userID, false)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "user", userID, &userID, "user_suspended", actorID, updated.Version, input.Reason, directoryUserState(current), directoryUserState(updated))
	})
	return updated, err
}

func (r *userAdministrationRepo) ReactivateUser(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var updated *models.DirectoryUser
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryUser(ctx, tx, organizationID, userID, input.ExpectedVersion)
		if err != nil {
			return err
		}
		if current.Status != models.UserStatusInactive || current.DeprovisionedAt != nil {
			return ErrDirectoryConflict
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE users SET status='active',reactivated_at=$4,
			invitation_status=CASE WHEN invitation_status='revoked' THEN 'accepted' ELSE invitation_status END,
			updated_by=$3::uuid,version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$5`,
			organizationID, userID, actorID, now, input.ExpectedVersion); err != nil {
			return classifyDirectoryWrite(err)
		}
		updated, err = getDirectoryUser(ctx, tx, organizationID, userID, false)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "user", userID, &userID, "user_reactivated", actorID, updated.Version, input.Reason, directoryUserState(current), directoryUserState(updated))
	})
	return updated, err
}

type ownershipBinding struct{ resource, table, column string }

var directoryOwnershipBindings = []ownershipBinding{
	{"frameworks.responsible", "organization_frameworks", "responsible_user_id"},
	{"controls.owner", "control_implementations", "owner_user_id"},
	{"controls.reviewer", "control_implementations", "reviewer_user_id"},
	{"risks.owner", "risks", "owner_user_id"}, {"risks.delegate", "risks", "delegate_user_id"},
	{"risk_treatments.owner", "risk_treatments", "owner_user_id"},
	{"risk_indicators.owner", "risk_indicators", "owner_user_id"},
	{"policies.owner", "policies", "owner_user_id"}, {"policies.approver", "policies", "approver_user_id"},
	{"policy_steps.approver", "policy_approval_steps", "approver_user_id"},
	{"policy_reviews.reviewer", "policy_reviews", "reviewer_user_id"},
	{"audits.lead", "audits", "lead_auditor_id"}, {"audit_findings.responsible", "audit_findings", "responsible_user_id"},
	{"incidents.assignee", "incidents", "assigned_to"}, {"assets.owner", "assets", "owner_user_id"},
	{"vendors.owner", "vendors", "owner_user_id"}, {"vendor_contracts.owner", "vendor_contracts", "owner_user_id"},
	{"processing.owner", "processing_activities", "process_owner_user_id"},
	{"processing.steward", "processing_activities", "data_steward_user_id"},
	{"continuity.process_owner", "business_processes", "process_owner_user_id"},
	{"continuity.plan_owner", "continuity_plans", "owner_user_id"},
	{"remediation.plan_owner", "remediation_plans", "owner_user_id"},
	{"remediation.action_assignee", "remediation_actions", "assigned_to"},
	{"evidence.assignee", "evidence_requirements", "assigned_to"},
	{"calendar.assignee", "calendar_events", "assigned_to"},
	{"dsr.request_assignee", "dsr_requests", "assigned_to"}, {"dsr.task_assignee", "dsr_tasks", "assigned_to"},
	{"analytics.dashboard_owner", "analytics_custom_dashboards", "owner_user_id"},
	{"nis2.measure_owner", "nis2_security_measures", "owner_user_id"},
	{"board.action_owner", "board_decisions", "action_owner_user_id"},
	{"workflow.step_assignee", "workflow_step_executions", "assigned_to"},
	{"exception.reviewer", "exception_reviews", "reviewer_user_id"},
}

func (r *userAdministrationRepo) PreviewOwnership(ctx context.Context, organizationID, userID string) (*models.DirectoryOwnershipImpact, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	if _, err := getDirectoryUser(ctx, q, organizationID, userID, false); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDirectoryUserNotFound
	} else if err != nil {
		return nil, err
	}
	return directoryOwnershipImpact(ctx, q, organizationID, userID)
}

func directoryOwnershipImpact(ctx context.Context, q database.Querier, organizationID, userID string) (*models.DirectoryOwnershipImpact, error) {
	impact := &models.DirectoryOwnershipImpact{UserID: userID, Resources: []models.DirectoryOwnershipResource{}}
	for _, binding := range directoryOwnershipBindings {
		var count int
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE organization_id=$1::uuid AND %s=$2::uuid`, binding.table, binding.column)
		if err := q.QueryRow(ctx, query, organizationID, userID).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s ownership: %w", binding.resource, err)
		}
		if count > 0 {
			impact.Resources = append(impact.Resources, models.DirectoryOwnershipResource{Resource: binding.resource, Count: count})
			impact.Total += count
		}
	}
	if err := q.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM users WHERE organization_id=$1::uuid AND manager_user_id=$2::uuid AND deleted_at IS NULL),
		(SELECT COUNT(*) FROM directory_group_memberships WHERE organization_id=$1::uuid AND user_id=$2::uuid AND removed_at IS NULL),
		(SELECT COUNT(*) FROM user_roles WHERE organization_id=$1::uuid AND user_id=$2::uuid)`, organizationID, userID).Scan(
		&impact.DirectReports, &impact.StaticGroupMemberships, &impact.PersistedRoleAssignments); err != nil {
		return nil, fmt.Errorf("count user directory impact: %w", err)
	}
	impact.Total += impact.DirectReports
	var remainingAdmin bool
	if err := q.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM users u WHERE u.organization_id=$1::uuid AND u.id=$2::uuid
			AND u.status='active' AND u.deleted_at IS NULL AND (u.is_super_admin OR EXISTS(
				SELECT 1 FROM effective_user_roles ur JOIN roles role ON role.id=ur.role_id AND role.deleted_at IS NULL
				JOIN role_permissions rp ON rp.role_id=role.id JOIN permissions p ON p.id=rp.permission_id
				WHERE ur.organization_id=u.organization_id AND ur.user_id=u.id
				AND p.resource='settings' AND p.action::text='configure'))),
		EXISTS(SELECT 1 FROM users u WHERE u.organization_id=$1::uuid AND u.id<>$2::uuid
			AND u.status='active' AND u.deleted_at IS NULL AND (u.is_super_admin OR EXISTS(
				SELECT 1 FROM effective_user_roles ur JOIN roles role ON role.id=ur.role_id AND role.deleted_at IS NULL
				JOIN role_permissions rp ON rp.role_id=role.id JOIN permissions p ON p.id=rp.permission_id
				WHERE ur.organization_id=u.organization_id AND ur.user_id=u.id
				AND ur.expires_at IS NULL AND NOT EXISTS(SELECT 1 FROM access_sod_rules rule
				 WHERE rule.organization_id=ur.organization_id AND rule.enabled AND ur.role_id IN(rule.role_a_id,rule.role_b_id))
				AND p.resource='settings' AND p.action::text='configure')))`, organizationID, userID).Scan(&impact.HasAdministrativeGrant, &remainingAdmin); err != nil {
		return nil, fmt.Errorf("calculate administrator impact: %w", err)
	}
	impact.IsLastActiveAdministrator = impact.HasAdministrativeGrant && !remainingAdmin
	impact.RequiresReplacement = impact.Total > 0
	return impact, nil
}

func (r *userAdministrationRepo) TransferOwnership(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryOwnershipTransferInput) (*models.DirectoryOwnershipImpact, *models.DirectoryUser, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var impact *models.DirectoryOwnershipImpact
	var updated *models.DirectoryUser
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryUser(ctx, tx, organizationID, userID, input.ExpectedVersion)
		if err != nil {
			return err
		}
		if err := ensureOwnershipReplacement(ctx, tx, organizationID, userID, input.ReplacementUserID); err != nil {
			return err
		}
		impact, err = directoryOwnershipImpact(ctx, tx, organizationID, userID)
		if err != nil {
			return err
		}
		if err := transferDirectoryOwnership(ctx, tx, organizationID, userID, input.ReplacementUserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET version=version+1,updated_by=$3::uuid
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$4`, organizationID, userID, actorID, input.ExpectedVersion); err != nil {
			return err
		}
		updated, err = getDirectoryUser(ctx, tx, organizationID, userID, false)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "user", userID, &userID, "ownership_transferred", actorID, updated.Version, input.Reason,
			directoryUserState(current), map[string]any{"replacement_user_id": input.ReplacementUserID, "impact": impact})
	})
	return impact, updated, err
}

func (r *userAdministrationRepo) DeprovisionUser(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryUserDeprovisionInput) error {
	q := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := lockDirectoryOrganization(ctx, tx, organizationID); err != nil {
			return err
		}
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryUser(ctx, tx, organizationID, userID, input.ExpectedVersion)
		if err != nil {
			return err
		}
		if last, err := directoryLastAdministrator(ctx, tx, organizationID, userID); err != nil {
			return err
		} else if last {
			return ErrDirectoryLastAdmin
		}
		impact, err := directoryOwnershipImpact(ctx, tx, organizationID, userID)
		if err != nil {
			return err
		}
		if impact.RequiresReplacement && input.ReplacementUserID == nil {
			return ErrDirectoryOwnershipRequired
		}
		if input.ReplacementUserID != nil {
			if err := ensureOwnershipReplacement(ctx, tx, organizationID, userID, *input.ReplacementUserID); err != nil {
				return err
			}
			if err := transferDirectoryOwnership(ctx, tx, organizationID, userID, *input.ReplacementUserID); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `SELECT role.id,role.version FROM user_roles ur JOIN roles role ON role.id=ur.role_id
			WHERE ur.organization_id=$1::uuid AND ur.user_id=$2::uuid FOR UPDATE OF ur`, organizationID, userID)
		if err != nil {
			return fmt.Errorf("list deprovisioned user roles: %w", err)
		}
		type assignedRole struct {
			id      string
			version int64
		}
		roles := []assignedRole{}
		for rows.Next() {
			var role assignedRole
			if err := rows.Scan(&role.id, &role.version); err != nil {
				rows.Close()
				return err
			}
			roles = append(roles, role)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE organization_id=$1::uuid AND user_id=$2::uuid`, organizationID, userID); err != nil {
			return fmt.Errorf("remove deprovisioned user roles: %w", err)
		}
		for _, role := range roles {
			if err := appendDirectoryRoleUnassignmentEvent(ctx, tx, organizationID, role.id, role.version, userID, actorID, input.Reason); err != nil {
				return err
			}
		}
		groupRows, err := tx.Query(ctx, `SELECT g.id,g.version FROM directory_groups g
			JOIN directory_group_memberships gm ON gm.organization_id=g.organization_id AND gm.group_id=g.id
			WHERE g.organization_id=$1::uuid AND gm.user_id=$2::uuid AND gm.removed_at IS NULL
			AND g.deleted_at IS NULL ORDER BY g.id FOR UPDATE OF g`, organizationID, userID)
		if err != nil {
			return fmt.Errorf("lock deprovisioned user groups: %w", err)
		}
		type affectedGroup struct {
			id      string
			version int64
		}
		groups := []affectedGroup{}
		for groupRows.Next() {
			var group affectedGroup
			if err := groupRows.Scan(&group.id, &group.version); err != nil {
				groupRows.Close()
				return err
			}
			groups = append(groups, group)
		}
		if err := groupRows.Err(); err != nil {
			groupRows.Close()
			return err
		}
		groupRows.Close()
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE directory_group_memberships SET removed_at=$3,removed_by=$4::uuid,remove_reason=$5
			WHERE organization_id=$1::uuid AND user_id=$2::uuid AND removed_at IS NULL`, organizationID, userID, now, actorID, input.Reason); err != nil {
			return fmt.Errorf("remove deprovisioned user groups: %w", err)
		}
		for _, group := range groups {
			if _, err := tx.Exec(ctx, `UPDATE directory_groups SET version=version+1,updated_by=$3::uuid
				WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$4`, organizationID, group.id, actorID, group.version); err != nil {
				return fmt.Errorf("version deprovisioned user group: %w", err)
			}
			updatedGroup, err := getDirectoryGroup(ctx, tx, organizationID, group.id, false)
			if err != nil {
				return err
			}
			if err := r.recordDirectoryEvent(ctx, tx, organizationID, "group", group.id, &userID, "member_removed",
				actorID, updatedGroup.Version, input.Reason, map[string]any{"user_id": userID}, directoryGroupState(updatedGroup)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_entity_permissions WHERE organization_id=$1::uuid AND user_id=$2::uuid`, organizationID, userID); err != nil {
			return fmt.Errorf("remove deprovisioned entity grants: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,$3)
			WHERE organization_id=$1::uuid AND user_id=$2::uuid`, organizationID, userID, now); err != nil {
			return fmt.Errorf("revoke deprovisioned sessions: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_id=$1::uuid`, userID); err != nil {
			return fmt.Errorf("remove deprovisioned password resets: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_mfa WHERE user_id=$1::uuid`, userID); err != nil {
			return fmt.Errorf("remove deprovisioned MFA: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET status='inactive',password_hash=NULL,failed_login_attempts=0,locked_until=NULL,
			manager_user_id=NULL,invitation_status='revoked',deprovisioned_at=$4,deprovisioned_by=$3::uuid,
			deprovision_reason=$5,deleted_at=$4,updated_by=$3::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$6`, organizationID, userID, actorID, now, input.Reason, input.ExpectedVersion); err != nil {
			return classifyDirectoryWrite(err)
		}
		deleted, err := getDirectoryUser(ctx, tx, organizationID, userID, true)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "user", userID, &userID, "user_deprovisioned", actorID, deleted.Version, input.Reason,
			directoryUserState(current), map[string]any{"replacement_user_id": input.ReplacementUserID, "impact": impact})
	})
}

func transferDirectoryOwnership(ctx context.Context, q database.Querier, organizationID, userID, replacementID string) error {
	for _, binding := range directoryOwnershipBindings {
		query := fmt.Sprintf(`UPDATE %s SET %s=$3::uuid WHERE organization_id=$1::uuid AND %s=$2::uuid`, binding.table, binding.column, binding.column)
		if _, err := q.Exec(ctx, query, organizationID, userID, replacementID); err != nil {
			return fmt.Errorf("transfer %s ownership: %w", binding.resource, err)
		}
	}
	if _, err := q.Exec(ctx, `UPDATE users SET manager_user_id=$3::uuid,updated_at=NOW(),version=version+1
		WHERE organization_id=$1::uuid AND manager_user_id=$2::uuid AND id<>$3::uuid AND deleted_at IS NULL`, organizationID, userID, replacementID); err != nil {
		return fmt.Errorf("transfer direct reports: %w", err)
	}
	return nil
}

func ensureOwnershipReplacement(ctx context.Context, q database.Querier, organizationID, userID, replacementID string) error {
	if userID == replacementID {
		return ErrDirectoryInvalidUser
	}
	if err := ensureDirectoryActiveUser(ctx, q, organizationID, replacementID); err != nil {
		return err
	}
	var reportsToTarget bool
	if err := q.QueryRow(ctx, `SELECT COALESCE(manager_user_id=$2::uuid,FALSE) FROM users
		WHERE organization_id=$1::uuid AND id=$3::uuid AND deleted_at IS NULL`, organizationID, userID, replacementID).Scan(&reportsToTarget); err != nil {
		return err
	}
	if reportsToTarget {
		return fmt.Errorf("%w: replacement cannot directly report to the departing user", ErrDirectoryConflict)
	}
	return nil
}

func (r *userAdministrationRepo) ListEvents(ctx context.Context, organizationID, entityType, entityID string, pagination models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM directory_change_events
		WHERE organization_id=$1::uuid AND entity_type=$2 AND entity_id=$3::uuid`, organizationID, entityType, entityID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count directory events: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT id,entity_type,entity_id,target_user_id,event_type,actor_user_id,
		entity_version,reason,before_state,after_state,COALESCE(request_id,''),created_at
		FROM directory_change_events WHERE organization_id=$1::uuid AND entity_type=$2 AND entity_id=$3::uuid
		ORDER BY created_at DESC,id DESC LIMIT $4 OFFSET $5`, organizationID, entityType, entityID,
		pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list directory events: %w", err)
	}
	defer rows.Close()
	items := make([]models.DirectoryChangeEvent, 0, pagination.PageSize)
	for rows.Next() {
		var item models.DirectoryChangeEvent
		if err := rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.TargetUserID, &item.EventType,
			&item.ActorUserID, &item.EntityVersion, &item.Reason, &item.BeforeState, &item.AfterState,
			&item.RequestID, &item.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan directory event: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *userAdministrationRepo) recordDirectoryEvent(ctx context.Context, tx pgx.Tx, organizationID, entityType, entityID string, targetUserID *string, eventType, actorID string, version int64, reason string, before, after any) error {
	beforeJSON, err := marshalDirectoryState(before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalDirectoryState(after)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO directory_change_events(
		organization_id,entity_type,entity_id,target_user_id,event_type,actor_user_id,entity_version,reason,before_state,after_state)
		VALUES($1::uuid,$2,$3::uuid,$4::uuid,$5,$6::uuid,$7,$8,$9::jsonb,$10::jsonb)`,
		organizationID, entityType, entityID, targetUserID, eventType, actorID, version, reason, beforeJSON, afterJSON); err != nil {
		return fmt.Errorf("append directory change event: %w", err)
	}
	severity := "medium"
	if eventType == "user_suspended" || eventType == "user_deprovisioned" || eventType == "ownership_transferred" {
		severity = "high"
	}
	payload := map[string]any{
		"type": "directory." + eventType, "severity": severity, "org_id": organizationID,
		"entity_type": entityType, "entity_id": entityID, "entity_ref": entityID,
		"data": map[string]any{"entity_type": entityType, "entity_id": entityID, "target_user_id": targetUserID,
			"actor_user_id": actorID, "version": version}, "timestamp": time.Now().UTC(),
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", organizationID, payload)
	if err != nil {
		return fmt.Errorf("create directory event envelope: %w", err)
	}
	envelope.CausationID = entityID
	envelope.Metadata = map[string]string{"entity_type": entityType, "entity_id": entityID, "event_type": eventType}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue directory event: %w", err)
	}
	return nil
}

func lockDirectoryOrganization(ctx context.Context, q database.Querier, organizationID string) error {
	var id string
	if err := q.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1::uuid FOR UPDATE`, organizationID).Scan(&id); err != nil {
		return fmt.Errorf("lock tenant administration state: %w", err)
	}
	return nil
}

func lockDirectoryUser(ctx context.Context, q database.Querier, organizationID, userID string, expectedVersion int64) (*models.DirectoryUser, error) {
	var actual int64
	if err := q.QueryRow(ctx, `SELECT version FROM users WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, organizationID, userID).Scan(&actual); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDirectoryUserNotFound
		}
		return nil, err
	}
	if actual != expectedVersion {
		return nil, ErrDirectoryVersionConflict
	}
	return getDirectoryUser(ctx, q, organizationID, userID, false)
}

func ensureDirectoryActiveUser(ctx context.Context, q database.Querier, organizationID, userID string) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE organization_id=$1::uuid AND id=$2::uuid
		AND status='active' AND deleted_at IS NULL)`, organizationID, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrDirectoryInvalidUser
	}
	return nil
}

func ensureDirectoryAssignableUser(ctx context.Context, q database.Querier, organizationID, userID string) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE organization_id=$1::uuid AND id=$2::uuid
		AND status IN ('active','pending_verification') AND deleted_at IS NULL)`, organizationID, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrDirectoryInvalidUser
	}
	return nil
}

func ensureDirectoryManagerAssignment(ctx context.Context, q database.Querier, organizationID, userID, managerID string) error {
	if userID == managerID {
		return fmt.Errorf("%w: a user cannot manage themselves", ErrDirectoryConflict)
	}
	if err := ensureDirectoryAssignableUser(ctx, q, organizationID, managerID); err != nil {
		return err
	}
	var createsCycle bool
	if err := q.QueryRow(ctx, `WITH RECURSIVE manager_chain(id,manager_user_id) AS (
		SELECT id,manager_user_id FROM users
		WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL
		UNION
		SELECT manager.id,manager.manager_user_id FROM users manager
		JOIN manager_chain child ON manager.id=child.manager_user_id
		WHERE manager.organization_id=$1::uuid AND manager.deleted_at IS NULL
	)
	SELECT EXISTS(SELECT 1 FROM manager_chain WHERE id=$3::uuid)`, organizationID, managerID, userID).Scan(&createsCycle); err != nil {
		return fmt.Errorf("validate directory manager hierarchy: %w", err)
	}
	if createsCycle {
		return fmt.Errorf("%w: manager assignment creates a reporting cycle", ErrDirectoryConflict)
	}
	return nil
}

func directoryLastAdministrator(ctx context.Context, q database.Querier, organizationID, userID string) (bool, error) {
	impact, err := directoryOwnershipImpact(ctx, q, organizationID, userID)
	if err != nil {
		return false, err
	}
	return impact.IsLastActiveAdministrator, nil
}

func resolveDirectoryRole(ctx context.Context, q database.Querier, organizationID, slug string) (string, int64, error) {
	var id string
	var version int64
	if err := q.QueryRow(ctx, `SELECT id,version FROM roles WHERE slug=$2 AND deleted_at IS NULL
		AND (organization_id=$1::uuid OR (organization_id IS NULL AND is_system_role))
		ORDER BY organization_id IS NOT NULL DESC LIMIT 1`, organizationID, slug).Scan(&id, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrDirectoryConflict
		}
		return "", 0, err
	}
	return id, version, nil
}

func appendInitialDirectoryRoleEvent(ctx context.Context, q database.Querier, organizationID, roleID string, roleVersion int64, userID, actorID, reason string) error {
	state, _ := json.Marshal(map[string]any{"role_id": roleID, "user_id": userID, "source": "directory_user_create"})
	if _, err := q.Exec(ctx, `INSERT INTO role_change_events(organization_id,role_id,target_user_id,event_type,
		actor_user_id,role_version,reason,after_state) VALUES($1::uuid,$2::uuid,$3::uuid,'assigned',$4::uuid,$5,$6,$7::jsonb)`,
		organizationID, roleID, userID, actorID, roleVersion, reason, state); err != nil {
		return fmt.Errorf("append initial role assignment event: %w", err)
	}
	return nil
}

func appendDirectoryRoleUnassignmentEvent(ctx context.Context, q database.Querier, organizationID, roleID string, roleVersion int64, userID, actorID, reason string) error {
	state, _ := json.Marshal(map[string]any{"role_id": roleID, "user_id": userID, "source": "directory_deprovision"})
	if _, err := q.Exec(ctx, `INSERT INTO role_change_events(organization_id,role_id,target_user_id,event_type,
		actor_user_id,role_version,reason,before_state) VALUES($1::uuid,$2::uuid,$3::uuid,'unassigned',$4::uuid,$5,$6,$7::jsonb)`,
		organizationID, roleID, userID, actorID, roleVersion, reason, state); err != nil {
		return fmt.Errorf("append deprovision role event: %w", err)
	}
	return nil
}

func directoryUserState(item *models.DirectoryUser) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{"id": item.ID, "email": item.Email, "first_name": item.FirstName, "last_name": item.LastName,
		"job_title": item.JobTitle, "department": item.Department, "location": item.Location, "employee_id": item.EmployeeID,
		"manager_user_id": item.ManagerUserID, "status": item.Status, "invitation_status": item.InvitationStatus,
		"role_slugs": item.RoleSlugs, "version": item.Version}
}

func marshalDirectoryState(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode directory history state: %w", err)
	}
	// An interface containing a typed nil map or pointer is non-nil, but JSON
	// encodes it as `null`. Pass a real SQL NULL so the database's history
	// invariant (states are objects when present) remains true.
	if string(encoded) == "null" {
		return nil, nil
	}
	return encoded, nil
}

func classifyDirectoryWrite(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505", "23514", "23503":
			return fmt.Errorf("%w: %s", ErrDirectoryConflict, postgresError.ConstraintName)
		}
	}
	return fmt.Errorf("persist directory record: %w", err)
}

func directoryUserSortColumn(value string) string {
	switch value {
	case "email":
		return "u.email"
	case "name":
		return "u.last_name"
	case "department":
		return "u.department"
	case "last_login_at":
		return "u.last_login_at"
	case "created_at":
		return "u.created_at"
	default:
		return "u.updated_at"
	}
}
