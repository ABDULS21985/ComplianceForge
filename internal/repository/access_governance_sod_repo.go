package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const sodRuleColumns = `id,organization_id,name,role_a_id,role_b_id,enabled,version,created_by,created_at`

func scanSoDRule(row interface{ Scan(...any) error }) (*models.AccessSoDRule, error) {
	r := new(models.AccessSoDRule)
	err := row.Scan(&r.ID, &r.OrganizationID, &r.Name, &r.RoleAID, &r.RoleBID, &r.Enabled, &r.Version, &r.CreatedBy, &r.CreatedAt)
	return r, err
}
func (r *accessGovernanceRepo) CreateSoDRule(ctx context.Context, org, actor string, in models.AccessSoDRuleInput) (*models.AccessSoDRule, error) {
	var result *models.AccessSoDRule
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		for _, id := range []string{in.RoleAID, in.RoleBID} {
			if _, err := getManagedRoleWithQuerier(ctx, tx, org, id); err != nil {
				return err
			}
		}
		var err error
		result, err = scanSoDRule(tx.QueryRow(ctx, `INSERT INTO access_sod_rules(organization_id,name,role_a_id,role_b_id,created_by) VALUES($1::uuid,$2,$3::uuid,$4::uuid,$5::uuid) RETURNING `+sodRuleColumns, org, in.Name, in.RoleAID, in.RoleBID, actor))
		if err != nil {
			return classifyGovernance(err)
		}
		if err := ensureIndependentPermanentAdmin(ctx, tx, org, in.RoleAID, in.RoleBID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO access_sod_violations(organization_id,rule_id,subject_id)
		 SELECT $1::uuid,$2::uuid,a.user_id FROM user_roles a JOIN user_roles b
		 ON b.organization_id=a.organization_id AND b.user_id=a.user_id
		 WHERE a.organization_id=$1::uuid AND a.role_id=$3::uuid AND b.role_id=$4::uuid
		 AND a.valid_from<=statement_timestamp() AND (a.expires_at IS NULL OR a.expires_at>statement_timestamp())
		 AND b.valid_from<=statement_timestamp() AND (b.expires_at IS NULL OR b.expires_at>statement_timestamp())`, org, result.ID, in.RoleAID, in.RoleBID); err != nil {
			return err
		}
		return r.event(ctx, tx, org, result.ID, actor, "sod_rule_created", in.Reason, map[string]any{"rule": result})
	})
	return result, err
}
func (r *accessGovernanceRepo) ListSoDRules(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessSoDRule, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM access_sod_rules WHERE organization_id=$1::uuid`, org).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+sodRuleColumns+` FROM access_sod_rules WHERE organization_id=$1::uuid ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, org, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.AccessSoDRule{}
	for rows.Next() {
		v, err := scanSoDRule(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *v)
	}
	return items, count, rows.Err()
}
func (r *accessGovernanceRepo) DisableSoDRule(ctx context.Context, org, id, actor string, in models.AccessGovernanceTransitionInput) (*models.AccessSoDRule, error) {
	var result *models.AccessSoDRule
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		var err error
		result, err = scanSoDRule(tx.QueryRow(ctx, `UPDATE access_sod_rules SET enabled=false,version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND enabled RETURNING `+sodRuleColumns, org, id, in.ExpectedVersion))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccessGovernanceConflict
		}
		if err != nil {
			return classifyGovernance(err)
		}
		return r.event(ctx, tx, org, id, actor, "sod_rule_disabled", in.Reason, map[string]any{"rule": result})
	})
	return result, err
}

const governanceExceptionColumns = `id,organization_id,rule_id,subject_id,requested_by,approved_by,status,reason,valid_from,expires_at,version,created_at`

func scanSoDException(row interface{ Scan(...any) error }) (*models.AccessSoDException, error) {
	r := new(models.AccessSoDException)
	err := row.Scan(&r.ID, &r.OrganizationID, &r.RuleID, &r.SubjectID, &r.RequestedBy, &r.ApprovedBy, &r.Status, &r.Reason, &r.ValidFrom, &r.ExpiresAt, &r.Version, &r.CreatedAt)
	return r, err
}
func (r *accessGovernanceRepo) RequestException(ctx context.Context, org, rule, actor string, in models.AccessSoDExceptionInput) (*models.AccessSoDException, error) {
	var result *models.AccessSoDException
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		if err := ensureManagedRoleUser(ctx, tx, org, in.SubjectID); err != nil {
			return ErrAccessGovernanceInvalid
		}
		var enabled bool
		if err := tx.QueryRow(ctx, `SELECT enabled FROM access_sod_rules WHERE organization_id=$1::uuid AND id=$2::uuid FOR SHARE`, org, rule).Scan(&enabled); err != nil {
			return err
		}
		if !enabled {
			return ErrAccessGovernanceInvalid
		}
		// Replacement requires an explicit reasoned revoke of an old window;
		// never silently rewrite its lifecycle as a side effect of a request.
		var err error
		result, err = scanSoDException(tx.QueryRow(ctx, `INSERT INTO access_sod_exceptions(organization_id,rule_id,subject_id,requested_by,reason,valid_from,expires_at) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7) RETURNING `+governanceExceptionColumns, org, rule, in.SubjectID, actor, in.Reason, in.ValidFrom, in.ExpiresAt))
		if err != nil {
			return classifyGovernance(err)
		}
		return r.event(ctx, tx, org, result.ID, actor, "sod_exception_requested", in.Reason, map[string]any{"exception": result})
	})
	return result, err
}
func (r *accessGovernanceRepo) DecideException(ctx context.Context, org, id, actor, status string, in models.AccessGovernanceTransitionInput) (*models.AccessSoDException, error) {
	var result *models.AccessSoDException
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		before, err := scanSoDException(tx.QueryRow(ctx, `SELECT `+governanceExceptionColumns+` FROM access_sod_exceptions WHERE organization_id=$1::uuid AND id=$2::uuid FOR UPDATE`, org, id))
		if err != nil {
			return err
		}
		if actor == before.SubjectID || actor == before.RequestedBy {
			return ErrAccessGovernanceSeparation
		}
		if before.Version != in.ExpectedVersion {
			return ErrAccessGovernanceConflict
		}
		if status == "approved" {
			var a, b string
			var enabled bool
			if err := tx.QueryRow(ctx, `SELECT role_a_id,role_b_id,enabled FROM access_sod_rules WHERE organization_id=$1::uuid AND id=$2::uuid FOR SHARE`, org, before.RuleID).Scan(&a, &b, &enabled); err != nil {
				return err
			}
			if !enabled {
				return ErrAccessGovernanceInvalid
			}
			if err := ensureIndependentPermanentAdmin(ctx, tx, org, a, b); err != nil {
				return err
			}
		}
		result, err = scanSoDException(tx.QueryRow(ctx, `UPDATE access_sod_exceptions SET status=$4::varchar,approved_by=CASE WHEN $4::varchar='approved' THEN $3::uuid ELSE approved_by END,version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid RETURNING `+governanceExceptionColumns, org, id, actor, status))
		if err != nil {
			return classifyGovernance(err)
		}
		return r.event(ctx, tx, org, id, actor, "sod_exception_"+status, in.Reason, map[string]any{"exception": result})
	})
	return result, err
}
func (r *accessGovernanceRepo) ListExceptions(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessSoDException, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM access_sod_exceptions WHERE organization_id=$1::uuid`, org).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+governanceExceptionColumns+` FROM access_sod_exceptions WHERE organization_id=$1::uuid ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, org, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.AccessSoDException{}
	for rows.Next() {
		v, err := scanSoDException(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *v)
	}
	return items, count, rows.Err()
}
func (r *accessGovernanceRepo) ListViolations(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessSoDViolation, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM access_sod_violations WHERE organization_id=$1::uuid`, org).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT id,rule_id,subject_id,detected_at FROM access_sod_violations WHERE organization_id=$1::uuid ORDER BY detected_at DESC,id LIMIT $2 OFFSET $3`, org, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.AccessSoDViolation{}
	for rows.Next() {
		var v models.AccessSoDViolation
		if err := rows.Scan(&v.ID, &v.RuleID, &v.SubjectID, &v.DetectedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, v)
	}
	return items, count, rows.Err()
}
func (r *accessGovernanceRepo) ListEvents(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessGovernanceEvent, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM access_governance_events WHERE organization_id=$1::uuid`, org).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT id,entity_id,event_type,actor_id,reason,details,created_at FROM access_governance_events WHERE organization_id=$1::uuid ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, org, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.AccessGovernanceEvent{}
	for rows.Next() {
		var v models.AccessGovernanceEvent
		if err := rows.Scan(&v.ID, &v.EntityID, &v.EventType, &v.ActorID, &v.Reason, &v.Details, &v.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, v)
	}
	return items, count, rows.Err()
}

// Any expiring administrative authority must leave a different, permanent,
// effective administrator. Otherwise an apparently safe assignment can lock
// the tenant out later, when no transaction is running to repair it.
func ensureIndependentPermanentAdmin(ctx context.Context, q database.Querier, org, roleA, roleB string, excludedSubject ...string) error {
	subject := ""
	if len(excludedSubject) > 0 {
		subject = excludedSubject[0]
	}
	var administrative, remaining bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id
	 WHERE rp.role_id IN ($2::uuid,$3::uuid) AND p.resource='settings' AND p.action='configure'),
	 EXISTS(SELECT 1 FROM users u WHERE u.organization_id=$1::uuid AND u.status='active' AND u.deleted_at IS NULL
	 AND (u.is_super_admin OR EXISTS(SELECT 1 FROM effective_user_roles ur JOIN roles r ON r.id=ur.role_id AND r.deleted_at IS NULL
	 JOIN role_permissions rp ON rp.role_id=r.id JOIN permissions p ON p.id=rp.permission_id
	 WHERE ur.organization_id=u.organization_id AND ur.user_id=u.id
	 AND (($4='' AND ur.role_id NOT IN ($2::uuid,$3::uuid)) OR ($4<>'' AND ur.user_id<>NULLIF($4,'')::uuid))
	 AND NOT EXISTS (SELECT 1 FROM access_sod_rules rule WHERE rule.organization_id=ur.organization_id
	   AND rule.enabled AND ur.role_id IN (rule.role_a_id,rule.role_b_id))
	 AND ur.expires_at IS NULL AND p.resource='settings' AND p.action='configure')))`, org, roleA, roleB, subject).Scan(&administrative, &remaining)
	if err != nil {
		return err
	}
	if administrative && !remaining {
		return ErrLastTenantAdministrator
	}
	return nil
}
func (r *accessGovernanceRepo) SetAssignmentWindow(ctx context.Context, org, role, user, actor string, in models.ManagedRoleWindowInput) (*models.ManagedRoleAssignment, error) {
	var result *models.ManagedRoleAssignment
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		if actor == user {
			return ErrAccessGovernanceSeparation
		}
		if err := ensureIndependentPermanentAdmin(ctx, tx, org, role, uuid.Nil.String(), user); err != nil {
			return err
		}
		result = &models.ManagedRoleAssignment{AssignmentID: in.AssignmentID, RoleID: role, UserID: user}
		err := tx.QueryRow(ctx, `UPDATE user_roles SET valid_from=$4,expires_at=$5,assignment_reason=$6,window_approved_by=$8::uuid WHERE organization_id=$1::uuid AND role_id=$2::uuid AND user_id=$3::uuid AND version=$7 AND assignment_id=$9::uuid RETURNING assigned_by,assigned_at,NULLIF(valid_from,'-infinity'::timestamptz),expires_at,version`, org, role, user, in.ValidFrom, in.ExpiresAt, in.Reason, in.ExpectedVersion, actor, in.AssignmentID).Scan(&result.AssignedBy, &result.AssignedAt, &result.ValidFrom, &result.ExpiresAt, &result.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccessGovernanceConflict
		}
		if err != nil {
			return classifyGovernance(err)
		}
		return r.event(ctx, tx, org, role, actor, "assignment_timeboxed", in.Reason, map[string]any{"assignment": result})
	})
	return result, err
}
