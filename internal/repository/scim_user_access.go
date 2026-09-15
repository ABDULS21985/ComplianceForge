package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

func ensureSCIMTokenActor(ctx context.Context, querier database.Querier, organizationID, tokenID, actorID string) error {
	if err := ensureDirectoryActiveUser(ctx, querier, organizationID, actorID); err != nil {
		return ErrSCIMNotFound
	}
	var valid bool
	if err := querier.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scim_tokens WHERE organization_id=$1::uuid
		AND id=$2::uuid AND created_by=$3::uuid AND is_active AND revoked_at IS NULL
		AND (expires_at IS NULL OR expires_at>NOW()))`, organizationID, tokenID, actorID).Scan(&valid); err != nil {
		return fmt.Errorf("validate SCIM token actor: %w", err)
	}
	if !valid {
		return ErrSCIMNotFound
	}
	return nil
}

func validateSCIMManager(ctx context.Context, querier database.Querier, organizationID, userID, managerID string) error {
	if managerID == "" {
		return nil
	}
	if err := ensureDirectoryManagerAssignment(ctx, querier, organizationID, userID, managerID); err != nil {
		if errors.Is(err, ErrDirectoryInvalidUser) || errors.Is(err, ErrDirectoryConflict) {
			return ErrSCIMConflict
		}
		return err
	}
	return nil
}

func assignSCIMViewerRole(ctx context.Context, tx pgx.Tx, organizationID, userID, actorID string) error {
	var hasRole bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM effective_user_roles
		WHERE organization_id=$1::uuid AND user_id=$2::uuid)`, organizationID, userID).Scan(&hasRole); err != nil {
		return fmt.Errorf("check SCIM user role: %w", err)
	}
	if hasRole {
		return nil
	}
	roleID, roleVersion, err := resolveDirectoryRole(ctx, tx, organizationID, "viewer")
	if err != nil {
		return ErrSCIMConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, userID, roleID, organizationID, actorID); err != nil {
		return classifySCIMWrite(err)
	}
	return appendInitialDirectoryRoleEvent(ctx, tx, organizationID, roleID, roleVersion, userID, actorID,
		"Provisioned through SCIM")
}

func ensureSCIMUserCanBeDeprovisioned(ctx context.Context, tx pgx.Tx, organizationID, userID string) error {
	if last, err := directoryLastAdministrator(ctx, tx, organizationID, userID); err != nil {
		return err
	} else if last {
		return ErrSCIMLastAdmin
	}
	impact, err := directoryOwnershipImpact(ctx, tx, organizationID, userID)
	if err != nil {
		return err
	}
	// SCIM has no portable ownership-transfer attribute. Fail closed instead of
	// leaving critical GRC records assigned to an offboarded principal; an
	// administrator can use the directory transfer workflow and retry.
	if impact.RequiresReplacement {
		return fmt.Errorf("%w: transfer owned GRC records before deprovisioning", ErrSCIMConflict)
	}
	return nil
}

func (r *scimRepo) revokeSCIMUserAccess(ctx context.Context, tx pgx.Tx, organizationID, userID,
	tokenID, actorID string) error {
	reason := "User deprovisioned through SCIM"
	if err := r.removeSCIMUserRoles(ctx, tx, organizationID, userID, actorID, reason); err != nil {
		return err
	}
	if err := r.removeSCIMUserGroups(ctx, tx, organizationID, userID, tokenID, actorID, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE user_entity_permissions SET status='revoked',revoked_at=NOW(),
		revoked_by=$3::uuid,revocation_reason=$4,version=version+1,updated_at=NOW()
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND status IN ('pending','approved')`,
		organizationID, userID, actorID, reason); err != nil {
		return fmt.Errorf("revoke SCIM user object grants: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM access_policy_assignments WHERE organization_id=$1::uuid
		AND assignee_type='user' AND assignee_id=$2::uuid`, organizationID, userID); err != nil {
		return fmt.Errorf("remove SCIM user policy assignments: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,NOW()),
		revoked_by=CASE WHEN revoked_at IS NULL THEN $3::uuid ELSE revoked_by END,
		revoke_reason=CASE WHEN revoked_at IS NULL THEN $4 ELSE revoke_reason END,version=version+1
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND revoked_at IS NULL`,
		organizationID, userID, actorID, reason); err != nil {
		return fmt.Errorf("revoke SCIM user sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE api_keys SET is_active=false WHERE organization_id=$1::uuid
		AND created_by=$2::uuid AND is_active`, organizationID, userID); err != nil {
		return fmt.Errorf("revoke SCIM user API keys: %w", err)
	}
	if err := r.revokeOwnedSCIMTokens(ctx, tx, organizationID, userID, actorID, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_authentication_challenges SET consumed_at=COALESCE(consumed_at,NOW())
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND consumed_at IS NULL`, organizationID, userID); err != nil {
		return fmt.Errorf("consume SCIM user identity challenges: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_step_up_grants SET revoked_at=NOW()
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND consumed_at IS NULL AND revoked_at IS NULL`,
		organizationID, userID); err != nil {
		return fmt.Errorf("revoke SCIM user step-up grants: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_invitations SET revoked_at=NOW()
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND accepted_at IS NULL AND revoked_at IS NULL`,
		organizationID, userID); err != nil {
		return fmt.Errorf("revoke SCIM user invitations: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_email_verification_tokens SET invalidated_at=NOW()
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND verified_at IS NULL AND invalidated_at IS NULL`,
		organizationID, userID); err != nil {
		return fmt.Errorf("invalidate SCIM user email verification: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE password_reset_tokens SET invalidated_at=NOW()
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND used_at IS NULL AND invalidated_at IS NULL`,
		organizationID, userID); err != nil {
		return fmt.Errorf("invalidate SCIM user password resets: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_passkeys SET removed_at=NOW(),removed_by=$3::uuid,
		removal_reason=$4,version=version+1 WHERE organization_id=$1::uuid AND user_id=$2::uuid
		AND removed_at IS NULL`, organizationID, userID, actorID, reason); err != nil {
		return fmt.Errorf("revoke SCIM user passkeys: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE user_mfa SET disabled_at=NOW(),disabled_by=$3::uuid,
		disable_reason=$4,is_primary=false,version=version+1 WHERE organization_id=$1::uuid
		AND user_id=$2::uuid AND disabled_at IS NULL`, organizationID, userID, actorID, reason); err != nil {
		return fmt.Errorf("disable SCIM user MFA: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_mfa_recovery_codes SET consumed_at=NOW()
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND consumed_at IS NULL`, organizationID, userID); err != nil {
		return fmt.Errorf("consume SCIM user recovery codes: %w", err)
	}
	return nil
}

func (r *scimRepo) removeSCIMUserRoles(ctx context.Context, tx pgx.Tx, organizationID, userID, actorID, reason string) error {
	rows, err := tx.Query(ctx, `SELECT role.id,role.version FROM user_roles assignment
		JOIN roles role ON role.id=assignment.role_id WHERE assignment.organization_id=$1::uuid
		AND assignment.user_id=$2::uuid FOR UPDATE OF assignment`, organizationID, userID)
	if err != nil {
		return fmt.Errorf("list SCIM user roles: %w", err)
	}
	type roleState struct {
		id      string
		version int64
	}
	roles := []roleState{}
	for rows.Next() {
		var role roleState
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
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE organization_id=$1::uuid AND user_id=$2::uuid`,
		organizationID, userID); err != nil {
		return fmt.Errorf("remove SCIM user roles: %w", err)
	}
	for _, role := range roles {
		if err := appendDirectoryRoleUnassignmentEvent(ctx, tx, organizationID, role.id, role.version,
			userID, actorID, reason); err != nil {
			return err
		}
	}
	return nil
}

func (r *scimRepo) removeSCIMUserGroups(ctx context.Context, tx pgx.Tx, organizationID, userID,
	tokenID, actorID, reason string) error {
	rows, err := tx.Query(ctx, `SELECT directory_group.id,directory_group.version,directory_group.name
		FROM directory_groups directory_group JOIN directory_group_memberships membership
		ON membership.organization_id=directory_group.organization_id AND membership.group_id=directory_group.id
		WHERE directory_group.organization_id=$1::uuid AND membership.user_id=$2::uuid
		AND membership.removed_at IS NULL AND directory_group.deleted_at IS NULL
		ORDER BY directory_group.id FOR UPDATE OF directory_group`, organizationID, userID)
	if err != nil {
		return fmt.Errorf("list SCIM user groups: %w", err)
	}
	type groupState struct {
		id      string
		version int64
		name    string
	}
	groups := []groupState{}
	for rows.Next() {
		var group groupState
		if err := rows.Scan(&group.id, &group.version, &group.name); err != nil {
			rows.Close()
			return err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err := tx.Exec(ctx, `UPDATE directory_group_memberships SET removed_at=NOW(),removed_by=$3::uuid,
		remove_reason=$4 WHERE organization_id=$1::uuid AND user_id=$2::uuid AND removed_at IS NULL`,
		organizationID, userID, actorID, reason); err != nil {
		return fmt.Errorf("remove SCIM user group memberships: %w", err)
	}
	for _, group := range groups {
		tag, err := tx.Exec(ctx, `UPDATE directory_groups SET version=version+1,updated_by=$3::uuid
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$4`, organizationID, group.id,
			actorID, group.version)
		if err != nil {
			return fmt.Errorf("version SCIM user group: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrSCIMVersion
		}
		before := map[string]any{"id": group.id, "displayName": group.name, "version": group.version,
			"removed_user_id": userID}
		after := map[string]any{"id": group.id, "displayName": group.name, "version": group.version + 1}
		if err := r.recordResourceEvent(ctx, tx, organizationID, tokenID, actorID, "Group", group.id,
			"patched", group.version+1, before, after); err != nil {
			return err
		}
	}
	return nil
}

func (r *scimRepo) revokeOwnedSCIMTokens(ctx context.Context, tx pgx.Tx, organizationID, userID,
	actorID, reason string) error {
	rows, err := tx.Query(ctx, `UPDATE scim_tokens AS token SET is_active=false,revoked_at=NOW(),
		revoked_by=$3::uuid,revoke_reason=$4,version=version+1 WHERE organization_id=$1::uuid
		AND created_by=$2::uuid AND is_active AND revoked_at IS NULL RETURNING `+scimTokenColumns,
		organizationID, userID, actorID, reason)
	if err != nil {
		return fmt.Errorf("revoke SCIM user tokens: %w", err)
	}
	tokens := []*models.SCIMToken{}
	for rows.Next() {
		token, err := scanSCIMToken(rows)
		if err != nil {
			rows.Close()
			return err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, token := range tokens {
		if err := r.recordTokenEvent(ctx, tx, token, actorID, "auto_revoked", reason,
			map[string]any{"trigger": "user_deprovisioned", "user_id": userID}); err != nil {
			return err
		}
	}
	return nil
}

func marshalSCIMUserProfile(input *models.SCIMUser) ([]byte, error) {
	copy := *input
	copy.ID, copy.Groups, copy.Meta, copy.Version = "", nil, models.SCIMMeta{}, 0
	encoded, err := json.Marshal(copy)
	if err != nil {
		return nil, fmt.Errorf("encode SCIM user profile: %w", err)
	}
	return encoded, nil
}

func scimManagerID(input *models.SCIMUser) string {
	if input.Enterprise == nil || input.Enterprise.Manager == nil {
		return ""
	}
	return input.Enterprise.Manager.Value
}

func scimEmployeeNumber(input *models.SCIMUser) string {
	if input.Enterprise == nil {
		return ""
	}
	return input.Enterprise.EmployeeNumber
}

func scimDepartment(input *models.SCIMUser) string {
	if input.Enterprise == nil {
		return ""
	}
	return input.Enterprise.Department
}

func scimLanguage(input *models.SCIMUser) string {
	if input.PreferredLanguage == "" || len(input.PreferredLanguage) > 10 {
		return "en"
	}
	return input.PreferredLanguage
}

func primarySCIMPhone(input *models.SCIMUser) string {
	for _, value := range input.PhoneNumbers {
		if value.Primary {
			return value.Value
		}
	}
	if len(input.PhoneNumbers) > 0 {
		return input.PhoneNumbers[0].Value
	}
	return ""
}
