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

const scimGroupSelect = `SELECT directory_group.id,COALESCE(directory_group.scim_external_id,''),
	directory_group.name,directory_group.version,directory_group.created_at,directory_group.updated_at,
	directory_group.group_type,directory_group.membership_rule,
	COALESCE((SELECT jsonb_agg(jsonb_build_object(
		'value',member.id::text,'$ref','/api/scim/v2/Users/'||member.id::text,
		'display',member.email,'type','User') ORDER BY lower(member.email),member.id)
		FROM directory_group_memberships membership
		JOIN users member ON member.organization_id=membership.organization_id
			AND member.id=membership.user_id AND member.deleted_at IS NULL
		WHERE membership.organization_id=directory_group.organization_id
			AND membership.group_id=directory_group.id AND membership.removed_at IS NULL),'[]'::jsonb)
	FROM directory_groups directory_group`

type scimGroupRecord struct {
	item           *models.SCIMGroup
	groupType      models.DirectoryGroupType
	membershipRule json.RawMessage
}

func scanSCIMGroup(row scimRowScanner) (*scimGroupRecord, error) {
	record := &scimGroupRecord{item: &models.SCIMGroup{}}
	var groupType string
	var membersJSON []byte
	if err := row.Scan(&record.item.ID, &record.item.ExternalID, &record.item.DisplayName,
		&record.item.Version, &record.item.Meta.Created, &record.item.Meta.LastModified,
		&groupType, &record.membershipRule, &membersJSON); err != nil {
		return nil, err
	}
	record.groupType = models.DirectoryGroupType(groupType)
	if err := json.Unmarshal(membersJSON, &record.item.Members); err != nil {
		return nil, fmt.Errorf("decode SCIM group members: %w", err)
	}
	return record, nil
}

func (r *scimRepo) CreateSCIMGroup(ctx context.Context, organizationID, tokenID, actorID string,
	input *models.SCIMGroup) (*models.SCIMGroup, error) {
	var result *models.SCIMGroup
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := ensureSCIMTokenActor(scoped, tx, organizationID, tokenID, actorID); err != nil {
				return err
			}
			if err := ensureSCIMMemberUsers(scoped, tx, organizationID, input.Members); err != nil {
				return err
			}
			id := uuid.NewString()
			slug := "scim-" + strings.ReplaceAll(id, "-", "")
			if _, err := tx.Exec(scoped, `INSERT INTO directory_groups(
				id,organization_id,name,slug,group_type,membership_rule,created_by,updated_by,
				scim_external_id,scim_managed) VALUES($1::uuid,$2::uuid,$3,$4,'static','{}'::jsonb,
				$5::uuid,$5::uuid,NULLIF($6,''),true)`, id, organizationID, input.DisplayName, slug,
				actorID, input.ExternalID); err != nil {
				return classifySCIMWrite(err)
			}
			if err := insertSCIMGroupMembers(scoped, tx, organizationID, id, actorID, input.Members); err != nil {
				return err
			}
			var err error
			result, _, err = getSCIMGroup(scoped, tx, organizationID, id, false)
			if err != nil {
				return err
			}
			return r.recordResourceEvent(scoped, tx, organizationID, tokenID, actorID, "Group", id,
				"created", result.Version, nil, result)
		})
	})
	return result, err
}

func (r *scimRepo) GetSCIMGroup(ctx context.Context, organizationID, groupID string) (*models.SCIMGroup, error) {
	var result *models.SCIMGroup
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		var groupType models.DirectoryGroupType
		var err error
		result, groupType, err = getSCIMGroup(scoped, querier, organizationID, groupID, false)
		if err != nil {
			return classifySCIMRead(err)
		}
		if groupType == models.DirectoryGroupDynamic {
			return r.loadDynamicSCIMMembers(scoped, querier, organizationID, groupID, result)
		}
		return nil
	})
	return result, err
}

func getSCIMGroup(ctx context.Context, querier database.Querier, organizationID, groupID string,
	includeDeleted bool) (*models.SCIMGroup, models.DirectoryGroupType, error) {
	predicate := " AND directory_group.deleted_at IS NULL"
	if includeDeleted {
		predicate = ""
	}
	record, err := scanSCIMGroup(querier.QueryRow(ctx, scimGroupSelect+`
		WHERE directory_group.organization_id=$1::uuid AND directory_group.id=$2::uuid`+predicate,
		organizationID, groupID))
	if err != nil {
		return nil, "", err
	}
	return record.item, record.groupType, nil
}

func (r *scimRepo) ListSCIMGroups(ctx context.Context, organizationID string, request models.SCIMListRequest,
	filter protocol.Filter) ([]models.SCIMGroup, int, error) {
	items := []models.SCIMGroup{}
	total := 0
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		where := []string{"directory_group.organization_id=$1::uuid", "directory_group.deleted_at IS NULL"}
		args := []any{organizationID}
		for _, clause := range filter.Clauses {
			if clause.Attribute == "displayname" {
				args = append(args, clause.Value)
				where = append(where, fmt.Sprintf("lower(directory_group.name)=lower($%d)", len(args)))
			}
		}
		predicate := strings.Join(where, " AND ")
		if err := querier.QueryRow(scoped, `SELECT COUNT(*) FROM directory_groups directory_group WHERE `+
			predicate, args...).Scan(&total); err != nil {
			return fmt.Errorf("count SCIM groups: %w", err)
		}
		if request.Count == 0 {
			return nil
		}
		args = append(args, request.Count, request.StartIndex-1)
		rows, err := querier.Query(scoped, scimGroupSelect+` WHERE `+predicate+`
			ORDER BY lower(directory_group.name),directory_group.id LIMIT $`+fmt.Sprint(len(args)-1)+
			` OFFSET $`+fmt.Sprint(len(args)), args...)
		if err != nil {
			return fmt.Errorf("list SCIM groups: %w", err)
		}
		types := make([]models.DirectoryGroupType, 0, request.Count)
		for rows.Next() {
			record, err := scanSCIMGroup(rows)
			if err != nil {
				rows.Close()
				return fmt.Errorf("scan SCIM group: %w", err)
			}
			items = append(items, *record.item)
			types = append(types, record.groupType)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for index := range items {
			if types[index] == models.DirectoryGroupDynamic {
				if err := r.loadDynamicSCIMMembers(scoped, querier, organizationID, items[index].ID,
					&items[index]); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return items, total, err
}

func (r *scimRepo) ReplaceSCIMGroup(ctx context.Context, organizationID, groupID, tokenID, actorID string,
	expectedVersion int64, requestedEvent string, input *models.SCIMGroup) (*models.SCIMGroup, error) {
	var result *models.SCIMGroup
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := ensureSCIMTokenActor(scoped, tx, organizationID, tokenID, actorID); err != nil {
				return err
			}
			current, groupType, err := lockSCIMGroup(scoped, tx, organizationID, groupID, expectedVersion)
			if err != nil {
				return err
			}
			if groupType != models.DirectoryGroupStatic {
				return ErrSCIMDynamicGroup
			}
			if err := ensureSCIMMemberUsers(scoped, tx, organizationID, input.Members); err != nil {
				return err
			}
			tag, err := tx.Exec(scoped, `UPDATE directory_groups SET name=$4,
				scim_external_id=NULLIF($5,''),scim_managed=true,updated_by=$6::uuid,version=version+1
				WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`,
				organizationID, groupID, expectedVersion, input.DisplayName, input.ExternalID, actorID)
			if err != nil {
				return classifySCIMWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrSCIMVersion
			}
			if err := replaceSCIMGroupMembers(scoped, tx, organizationID, groupID, actorID, input.Members); err != nil {
				return err
			}
			result, _, err = getSCIMGroup(scoped, tx, organizationID, groupID, false)
			if err != nil {
				return err
			}
			return r.recordResourceEvent(scoped, tx, organizationID, tokenID, actorID, "Group", groupID,
				requestedEvent, result.Version, current, result)
		})
	})
	return result, err
}

func (r *scimRepo) DeleteSCIMGroup(ctx context.Context, organizationID, groupID, tokenID, actorID string,
	expectedVersion int64) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := ensureSCIMTokenActor(scoped, tx, organizationID, tokenID, actorID); err != nil {
				return err
			}
			current, groupType, err := lockSCIMGroup(scoped, tx, organizationID, groupID, expectedVersion)
			if err != nil {
				return err
			}
			if groupType != models.DirectoryGroupStatic {
				return ErrSCIMDynamicGroup
			}
			now := time.Now().UTC()
			if _, err := tx.Exec(scoped, `UPDATE directory_group_memberships SET removed_at=$3,
				removed_by=$4::uuid,remove_reason='Group deleted through SCIM' WHERE organization_id=$1::uuid
				AND group_id=$2::uuid AND removed_at IS NULL`, organizationID, groupID, now, actorID); err != nil {
				return fmt.Errorf("close deleted SCIM group memberships: %w", err)
			}
			tag, err := tx.Exec(scoped, `UPDATE directory_groups SET deleted_at=$4,updated_by=$5::uuid,
				version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3
				AND deleted_at IS NULL`, organizationID, groupID, expectedVersion, now, actorID)
			if err != nil {
				return classifySCIMWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrSCIMVersion
			}
			deleted, _, err := getSCIMGroup(scoped, tx, organizationID, groupID, true)
			if err != nil {
				return err
			}
			return r.recordResourceEvent(scoped, tx, organizationID, tokenID, actorID, "Group", groupID,
				"deleted", deleted.Version, current, deleted)
		})
	})
}

func lockSCIMGroup(ctx context.Context, querier database.Querier, organizationID, groupID string,
	expectedVersion int64) (*models.SCIMGroup, models.DirectoryGroupType, error) {
	var actual int64
	if err := querier.QueryRow(ctx, `SELECT version FROM directory_groups WHERE organization_id=$1::uuid
		AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, organizationID, groupID).Scan(&actual); err != nil {
		return nil, "", classifySCIMRead(err)
	}
	if actual != expectedVersion {
		return nil, "", ErrSCIMVersion
	}
	return getSCIMGroup(ctx, querier, organizationID, groupID, false)
}

func ensureSCIMMemberUsers(ctx context.Context, querier database.Querier, organizationID string,
	members []models.SCIMMember) error {
	if len(members) == 0 {
		return nil
	}
	ids := make([]string, len(members))
	for index := range members {
		ids[index] = members[index].Value
	}
	var count int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE organization_id=$1::uuid
		AND id=ANY($2::uuid[]) AND deleted_at IS NULL`, organizationID, ids).Scan(&count); err != nil {
		return fmt.Errorf("validate SCIM group members: %w", err)
	}
	if count != len(ids) {
		return ErrSCIMConflict
	}
	return nil
}

func insertSCIMGroupMembers(ctx context.Context, tx pgx.Tx, organizationID, groupID, actorID string,
	members []models.SCIMMember) error {
	for _, member := range members {
		if _, err := tx.Exec(ctx, `INSERT INTO directory_group_memberships(
			organization_id,group_id,user_id,added_by,add_reason) VALUES($1::uuid,$2::uuid,$3::uuid,
			$4::uuid,'Added through SCIM')`, organizationID, groupID, member.Value, actorID); err != nil {
			return classifySCIMWrite(err)
		}
	}
	return nil
}

func replaceSCIMGroupMembers(ctx context.Context, tx pgx.Tx, organizationID, groupID, actorID string,
	members []models.SCIMMember) error {
	requested := make([]string, len(members))
	for index := range members {
		requested[index] = members[index].Value
	}
	if _, err := tx.Exec(ctx, `UPDATE directory_group_memberships SET removed_at=NOW(),removed_by=$3::uuid,
		remove_reason='Removed through SCIM' WHERE organization_id=$1::uuid AND group_id=$2::uuid
		AND removed_at IS NULL AND NOT (user_id=ANY($4::uuid[]))`, organizationID, groupID, actorID,
		requested); err != nil {
		return fmt.Errorf("remove replaced SCIM group members: %w", err)
	}
	for _, member := range members {
		if _, err := tx.Exec(ctx, `INSERT INTO directory_group_memberships(
			organization_id,group_id,user_id,added_by,add_reason)
			SELECT $1::uuid,$2::uuid,$3::uuid,$4::uuid,'Added through SCIM'
			WHERE NOT EXISTS(SELECT 1 FROM directory_group_memberships WHERE organization_id=$1::uuid
				AND group_id=$2::uuid AND user_id=$3::uuid AND removed_at IS NULL)`, organizationID,
			groupID, member.Value, actorID); err != nil {
			return classifySCIMWrite(err)
		}
	}
	return nil
}

func (r *scimRepo) loadDynamicSCIMMembers(ctx context.Context, querier database.Querier, organizationID,
	groupID string, destination *models.SCIMGroup) error {
	var rule json.RawMessage
	if err := querier.QueryRow(ctx, `SELECT membership_rule FROM directory_groups
		WHERE organization_id=$1::uuid AND id=$2::uuid AND group_type='dynamic' AND deleted_at IS NULL`,
		organizationID, groupID).Scan(&rule); err != nil {
		return classifySCIMRead(err)
	}
	predicate, args, err := dynamicGroupPredicate(organizationID, rule)
	if err != nil {
		return err
	}
	rows, err := querier.Query(ctx, `SELECT u.id,u.email,COALESCE(u.first_name,''),COALESCE(u.last_name,'')
		FROM users u WHERE `+predicate+` ORDER BY lower(u.email),u.id LIMIT 1001`, args...)
	if err != nil {
		return fmt.Errorf("list dynamic SCIM group members: %w", err)
	}
	defer rows.Close()
	destination.Members = []models.SCIMMember{}
	for rows.Next() {
		var id, email, firstName, lastName string
		if err := rows.Scan(&id, &email, &firstName, &lastName); err != nil {
			return err
		}
		if len(destination.Members) == 1000 {
			return fmt.Errorf("%w: dynamic group exceeds the SCIM member response limit", ErrSCIMConflict)
		}
		display := strings.TrimSpace(firstName + " " + lastName)
		if display == "" {
			display = email
		}
		destination.Members = append(destination.Members, models.SCIMMember{Value: id,
			Ref: "/api/scim/v2/Users/" + id, Display: display, Type: "User"})
	}
	return rows.Err()
}

var _ SCIMRepository = (*scimRepo)(nil)
