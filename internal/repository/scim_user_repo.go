package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	protocol "github.com/complianceforge/platform/internal/scim"
)

const scimUserSelect = `SELECT user_account.id,COALESCE(user_account.scim_external_id,''),user_account.email,
	COALESCE(user_account.first_name,''),COALESCE(user_account.last_name,''),COALESCE(user_account.job_title,''),
	COALESCE(user_account.department,''),COALESCE(user_account.phone,''),COALESCE(user_account.timezone,''),
	COALESCE(user_account.language,'en'),COALESCE(user_account.employee_id,''),user_account.manager_user_id,
	COALESCE(manager.email,''),COALESCE(manager.first_name,''),COALESCE(manager.last_name,''),
	user_account.status='active',user_account.version,user_account.created_at,user_account.updated_at,
	user_account.scim_profile,
	COALESCE((SELECT jsonb_agg(jsonb_build_object(
		'value',directory_group.id::text,'$ref','/api/scim/v2/Groups/'||directory_group.id::text,
		'display',directory_group.name,'type','direct') ORDER BY lower(directory_group.name),directory_group.id)
		FROM directory_group_memberships membership
		JOIN directory_groups directory_group ON directory_group.organization_id=membership.organization_id
			AND directory_group.id=membership.group_id AND directory_group.deleted_at IS NULL
		WHERE membership.organization_id=user_account.organization_id AND membership.user_id=user_account.id
			AND membership.removed_at IS NULL),'[]'::jsonb)
	FROM users user_account
	LEFT JOIN users manager ON manager.organization_id=user_account.organization_id
		AND manager.id=user_account.manager_user_id`

func scanSCIMUser(row scimRowScanner) (*models.SCIMUser, error) {
	item := &models.SCIMUser{}
	var profile, groupsJSON []byte
	var id, externalID, userName string
	var active bool
	var version int64
	var createdAt, updatedAt time.Time
	var firstName, lastName, title, department, phone, timezone, language, employeeNumber string
	var managerID *string
	var managerEmail, managerFirst, managerLast string
	if err := row.Scan(&id, &externalID, &userName, &firstName, &lastName, &title,
		&department, &phone, &timezone, &language, &employeeNumber, &managerID, &managerEmail,
		&managerFirst, &managerLast, &active, &version, &createdAt,
		&updatedAt, &profile, &groupsJSON); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(profile, item); err != nil {
		return nil, fmt.Errorf("decode SCIM user profile: %w", err)
	}
	// Canonical directory columns always win over the convenience profile.
	item.ID, item.ExternalID, item.UserName = id, externalID, strings.ToLower(userName)
	item.Active, item.Version = active, version
	item.Meta.Created, item.Meta.LastModified = createdAt, updatedAt
	item.Name.GivenName, item.Name.FamilyName = firstName, lastName
	item.Title, item.Timezone = title, timezone
	if item.PreferredLanguage == "" {
		item.PreferredLanguage = language
	}
	if phone != "" && len(item.PhoneNumbers) == 0 {
		item.PhoneNumbers = []models.SCIMMultiValue{{Value: phone, Type: "work", Primary: true}}
	}
	if err := json.Unmarshal(groupsJSON, &item.Groups); err != nil {
		return nil, fmt.Errorf("decode SCIM user groups: %w", err)
	}
	if employeeNumber != "" || department != "" || managerID != nil || item.Enterprise != nil {
		if item.Enterprise == nil {
			item.Enterprise = &models.SCIMEnterpriseUser{}
		}
		item.Enterprise.EmployeeNumber = employeeNumber
		item.Enterprise.Department = department
		if managerID != nil {
			display := strings.TrimSpace(managerFirst + " " + managerLast)
			if display == "" {
				display = managerEmail
			}
			item.Enterprise.Manager = &models.SCIMManager{Value: *managerID,
				Ref: "/api/scim/v2/Users/" + *managerID, Display: display}
		} else {
			item.Enterprise.Manager = nil
		}
	}
	return item, nil
}

func (r *scimRepo) CreateSCIMUser(ctx context.Context, organizationID, tokenID, actorID string,
	input *models.SCIMUser) (*models.SCIMUser, error) {
	var result *models.SCIMUser
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := ensureSCIMTokenActor(scoped, tx, organizationID, tokenID, actorID); err != nil {
				return err
			}
			if err := EnsureEntitlementCapacity(scoped, tx, organizationID, "users", 1); err != nil {
				return err
			}
			id := uuid.NewString()
			if err := validateSCIMManager(scoped, tx, organizationID, id, scimManagerID(input)); err != nil {
				return err
			}
			profile, err := marshalSCIMUserProfile(input)
			if err != nil {
				return err
			}
			status := models.UserStatusInactive
			if input.Active {
				status = models.UserStatusActive
			}
			var deprovisionedAt *time.Time
			var deprovisionedBy *string
			deprovisionReason := ""
			if !input.Active {
				now := time.Now().UTC()
				deprovisionedAt, deprovisionedBy = &now, &actorID
				deprovisionReason = "Provisioned inactive through SCIM"
			}
			_, err = tx.Exec(scoped, `INSERT INTO users(
				id,organization_id,email,first_name,last_name,job_title,department,phone,status,timezone,
				language,manager_user_id,employee_id,invitation_status,email_verified_at,updated_by,
				deprovisioned_at,deprovisioned_by,deprovision_reason,scim_external_id,scim_profile,scim_managed)
				VALUES($1::uuid,$2::uuid,lower($3),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),
				NULLIF($8,''),$9::user_status,NULLIF($10,''),$11,NULLIF($12,'')::uuid,NULLIF($13,''),'not_required',
				CASE WHEN $9::user_status='active' THEN NOW() ELSE NULL END,$14::uuid,$15,$16::uuid,NULLIF($17,''),
				NULLIF($18,''),$19::jsonb,true)`, id, organizationID, input.UserName, input.Name.GivenName,
				input.Name.FamilyName, input.Title, scimDepartment(input), primarySCIMPhone(input), status,
				input.Timezone, scimLanguage(input), scimManagerID(input), scimEmployeeNumber(input), actorID,
				deprovisionedAt, deprovisionedBy, deprovisionReason, input.ExternalID, profile)
			if err != nil {
				return classifySCIMWrite(err)
			}
			if input.Active {
				if err := assignSCIMViewerRole(scoped, tx, organizationID, id, actorID); err != nil {
					return err
				}
			}
			result, err = getSCIMUser(scoped, tx, organizationID, id, false)
			if err != nil {
				return err
			}
			return r.recordResourceEvent(scoped, tx, organizationID, tokenID, actorID, "User", id,
				"created", result.Version, nil, result)
		})
	})
	return result, err
}

func (r *scimRepo) GetSCIMUser(ctx context.Context, organizationID, userID string) (*models.SCIMUser, error) {
	var result *models.SCIMUser
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		var err error
		result, err = getSCIMUser(scoped, querier, organizationID, userID, false)
		return classifySCIMRead(err)
	})
	return result, err
}

func getSCIMUser(ctx context.Context, querier database.Querier, organizationID, userID string, includeDeleted bool) (*models.SCIMUser, error) {
	predicate := " AND user_account.deleted_at IS NULL"
	if includeDeleted {
		predicate = ""
	}
	return scanSCIMUser(querier.QueryRow(ctx, scimUserSelect+`
		WHERE user_account.organization_id=$1::uuid AND user_account.id=$2::uuid`+predicate,
		organizationID, userID))
}

func (r *scimRepo) ListSCIMUsers(ctx context.Context, organizationID string, request models.SCIMListRequest,
	filter protocol.Filter) ([]models.SCIMUser, int, error) {
	items := []models.SCIMUser{}
	total := 0
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		where := []string{"user_account.organization_id=$1::uuid", "user_account.deleted_at IS NULL"}
		args := []any{organizationID}
		for _, clause := range filter.Clauses {
			args = append(args, clause.Value)
			switch clause.Attribute {
			case "username":
				where = append(where, fmt.Sprintf("lower(user_account.email)=lower($%d)", len(args)))
			case "externalid":
				where = append(where, fmt.Sprintf("user_account.scim_external_id=$%d", len(args)))
			}
		}
		predicate := strings.Join(where, " AND ")
		if err := querier.QueryRow(scoped, `SELECT COUNT(*) FROM users user_account WHERE `+predicate,
			args...).Scan(&total); err != nil {
			return fmt.Errorf("count SCIM users: %w", err)
		}
		if request.Count == 0 {
			return nil
		}
		args = append(args, request.Count, request.StartIndex-1)
		rows, err := querier.Query(scoped, scimUserSelect+` WHERE `+predicate+`
			ORDER BY lower(user_account.email),user_account.id LIMIT $`+fmt.Sprint(len(args)-1)+
			` OFFSET $`+fmt.Sprint(len(args)), args...)
		if err != nil {
			return fmt.Errorf("list SCIM users: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanSCIMUser(rows)
			if err != nil {
				return fmt.Errorf("scan SCIM user: %w", err)
			}
			items = append(items, *item)
		}
		return rows.Err()
	})
	return items, total, err
}

func (r *scimRepo) ReplaceSCIMUser(ctx context.Context, organizationID, userID, tokenID, actorID string,
	expectedVersion int64, requestedEvent string, input *models.SCIMUser) (*models.SCIMUser, error) {
	var result *models.SCIMUser
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := lockDirectoryOrganization(scoped, tx, organizationID); err != nil {
				return err
			}
			if err := ensureSCIMTokenActor(scoped, tx, organizationID, tokenID, actorID); err != nil {
				return err
			}
			current, err := lockSCIMUser(scoped, tx, organizationID, userID, expectedVersion)
			if err != nil {
				return err
			}
			if err := validateSCIMManager(scoped, tx, organizationID, userID, scimManagerID(input)); err != nil {
				return err
			}
			eventType := requestedEvent
			if !input.Active {
				if current.Active {
					if err := ensureSCIMUserCanBeDeprovisioned(scoped, tx, organizationID, userID); err != nil {
						return err
					}
				}
				if err := r.revokeSCIMUserAccess(scoped, tx, organizationID, userID, tokenID, actorID); err != nil {
					return err
				}
				if current.Active {
					eventType = "deprovisioned"
				}
			} else if !current.Active && input.Active {
				if err := assignSCIMViewerRole(scoped, tx, organizationID, userID, actorID); err != nil {
					return err
				}
				eventType = "reactivated"
			}
			profile, err := marshalSCIMUserProfile(input)
			if err != nil {
				return err
			}
			status := models.UserStatusInactive
			if input.Active {
				status = models.UserStatusActive
			}
			deprovisioned := !input.Active
			tag, err := tx.Exec(scoped, `UPDATE users SET
				email=lower($4),first_name=NULLIF($5,''),last_name=NULLIF($6,''),job_title=NULLIF($7,''),
				department=NULLIF($8,''),phone=NULLIF($9,''),status=$10::user_status,timezone=NULLIF($11,''),
				language=$12,manager_user_id=NULLIF($13,'')::uuid,employee_id=NULLIF($14,''),updated_by=$15::uuid,
				scim_external_id=NULLIF($16,''),scim_profile=$17::jsonb,scim_managed=true,
				deprovisioned_at=CASE WHEN $18 THEN COALESCE(deprovisioned_at,NOW()) ELSE NULL END,
				deprovisioned_by=CASE WHEN $18 THEN $15::uuid ELSE NULL END,
				deprovision_reason=CASE WHEN $18 THEN 'Deprovisioned through SCIM' ELSE NULL END,
				reactivated_at=CASE WHEN NOT $18 AND status<>'active' THEN NOW() ELSE reactivated_at END,
				invitation_status=CASE WHEN $18 THEN 'revoked' ELSE 'not_required' END,
				email_verified_at=CASE WHEN NOT $18 THEN COALESCE(email_verified_at,NOW()) ELSE email_verified_at END,
				password_hash=CASE WHEN $18 THEN NULL ELSE password_hash END,version=version+1
				WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`,
				organizationID, userID, expectedVersion, input.UserName, input.Name.GivenName,
				input.Name.FamilyName, input.Title, scimDepartment(input), primarySCIMPhone(input), status,
				input.Timezone, scimLanguage(input), scimManagerID(input), scimEmployeeNumber(input), actorID,
				input.ExternalID, profile, deprovisioned)
			if err != nil {
				return classifySCIMWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrSCIMVersion
			}
			result, err = getSCIMUser(scoped, tx, organizationID, userID, false)
			if err != nil {
				return err
			}
			return r.recordResourceEvent(scoped, tx, organizationID, tokenID, actorID, "User", userID,
				eventType, result.Version, current, result)
		})
	})
	return result, err
}

func (r *scimRepo) DeleteSCIMUser(ctx context.Context, organizationID, userID, tokenID, actorID string,
	expectedVersion int64) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := lockDirectoryOrganization(scoped, tx, organizationID); err != nil {
				return err
			}
			if err := ensureSCIMTokenActor(scoped, tx, organizationID, tokenID, actorID); err != nil {
				return err
			}
			current, err := lockSCIMUser(scoped, tx, organizationID, userID, expectedVersion)
			if err != nil {
				return err
			}
			if err := ensureSCIMUserCanBeDeprovisioned(scoped, tx, organizationID, userID); err != nil {
				return err
			}
			if err := r.revokeSCIMUserAccess(scoped, tx, organizationID, userID, tokenID, actorID); err != nil {
				return err
			}
			now := time.Now().UTC()
			tag, err := tx.Exec(scoped, `UPDATE users SET status='inactive',password_hash=NULL,
				failed_login_attempts=0,locked_until=NULL,manager_user_id=NULL,invitation_status='revoked',
				deprovisioned_at=$4,deprovisioned_by=$5::uuid,deprovision_reason='Deleted through SCIM',
				deleted_at=$4,updated_by=$5::uuid,version=version+1 WHERE organization_id=$1::uuid
				AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`, organizationID, userID,
				expectedVersion, now, actorID)
			if err != nil {
				return classifySCIMWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrSCIMVersion
			}
			deleted, err := getSCIMUser(scoped, tx, organizationID, userID, true)
			if err != nil {
				return err
			}
			return r.recordResourceEvent(scoped, tx, organizationID, tokenID, actorID, "User", userID,
				"deleted", deleted.Version, current, deleted)
		})
	})
}

func lockSCIMUser(ctx context.Context, querier database.Querier, organizationID, userID string,
	expectedVersion int64) (*models.SCIMUser, error) {
	var actual int64
	if err := querier.QueryRow(ctx, `SELECT version FROM users WHERE organization_id=$1::uuid
		AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, organizationID, userID).Scan(&actual); err != nil {
		return nil, classifySCIMRead(err)
	}
	if actual != expectedVersion {
		return nil, ErrSCIMVersion
	}
	return getSCIMUser(ctx, querier, organizationID, userID, false)
}
