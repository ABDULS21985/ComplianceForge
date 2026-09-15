package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const directoryGroupSelect = `
	SELECT g.id,g.organization_id,g.name,g.slug,COALESCE(g.description,''),g.group_type,
		g.membership_rule,g.version,g.created_by,g.updated_by,
		(SELECT COUNT(*) FROM directory_group_memberships gm
			WHERE gm.organization_id=g.organization_id AND gm.group_id=g.id AND gm.removed_at IS NULL),
		g.created_at,g.updated_at,g.deleted_at
	FROM directory_groups g`

func scanDirectoryGroup(row directoryRowScanner) (*models.DirectoryGroup, error) {
	item := &models.DirectoryGroup{}
	var groupType string
	if err := row.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Slug, &item.Description,
		&groupType, &item.MembershipRule, &item.Version, &item.CreatedBy, &item.UpdatedBy,
		&item.MemberCount, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt); err != nil {
		return nil, err
	}
	item.GroupType = models.DirectoryGroupType(groupType)
	return item, nil
}

func (r *userAdministrationRepo) CreateGroup(ctx context.Context, organizationID, actorID string, input models.DirectoryGroupCreateInput) (*models.DirectoryGroup, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var created *models.DirectoryGroup
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		var err error
		created, err = scanDirectoryGroup(tx.QueryRow(ctx, `INSERT INTO directory_groups(
			organization_id,name,slug,description,group_type,membership_rule,created_by,updated_by)
			VALUES($1::uuid,$2,$3,NULLIF($4,''),$5,$6::jsonb,$7::uuid,$7::uuid)
			RETURNING id,organization_id,name,slug,COALESCE(description,''),group_type,membership_rule,
			version,created_by,updated_by,0,created_at,updated_at,deleted_at`, organizationID, input.Name,
			input.Slug, input.Description, input.GroupType, input.MembershipRule, actorID))
		if err != nil {
			return classifyDirectoryWrite(err)
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "group", created.ID, nil, "group_created",
			actorID, created.Version, input.Reason, nil, directoryGroupState(created))
	})
	return created, err
}

func (r *userAdministrationRepo) GetGroup(ctx context.Context, organizationID, groupID string) (*models.DirectoryGroup, error) {
	item, err := getDirectoryGroup(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, groupID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDirectoryGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	if item.GroupType == models.DirectoryGroupDynamic {
		count, err := countDynamicGroupMembers(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, item.MembershipRule)
		if err != nil {
			return nil, err
		}
		item.MemberCount = count
	}
	return item, nil
}

func getDirectoryGroup(ctx context.Context, q database.Querier, organizationID, groupID string, includeDeleted bool) (*models.DirectoryGroup, error) {
	predicate := " AND g.deleted_at IS NULL"
	if includeDeleted {
		predicate = ""
	}
	return scanDirectoryGroup(q.QueryRow(ctx, directoryGroupSelect+` WHERE g.organization_id=$1::uuid AND g.id=$2::uuid`+predicate, organizationID, groupID))
}

func (r *userAdministrationRepo) ListGroups(ctx context.Context, organizationID string, filter models.DirectoryGroupListFilter) ([]models.DirectoryGroup, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := []string{"g.organization_id=$1::uuid", "g.deleted_at IS NULL"}
	args := []any{organizationID}
	if filter.Search != "" {
		args = append(args, filter.Search)
		where = append(where, fmt.Sprintf(`(g.name ILIKE '%%'||$%d||'%%' OR g.slug ILIKE '%%'||$%d||'%%' OR COALESCE(g.description,'') ILIKE '%%'||$%d||'%%')`, len(args), len(args), len(args)))
	}
	if filter.GroupType != "" {
		args = append(args, filter.GroupType)
		where = append(where, fmt.Sprintf(`g.group_type=$%d`, len(args)))
	}
	predicate := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM directory_groups g WHERE `+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count directory groups: %w", err)
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := q.Query(ctx, directoryGroupSelect+` WHERE `+predicate+` ORDER BY `+directoryGroupSortColumn(filter.SortBy)+` `+filter.SortDirection+`,g.id `+filter.SortDirection+fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list directory groups: %w", err)
	}
	defer rows.Close()
	items := make([]models.DirectoryGroup, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanDirectoryGroup(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan directory group: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate directory groups: %w", err)
	}
	for i := range items {
		if items[i].GroupType == models.DirectoryGroupDynamic {
			count, err := countDynamicGroupMembers(ctx, q, organizationID, items[i].MembershipRule)
			if err != nil {
				return nil, 0, err
			}
			items[i].MemberCount = count
		}
	}
	return items, total, nil
}

func (r *userAdministrationRepo) UpdateGroup(ctx context.Context, organizationID, groupID, actorID string, patch models.DirectoryGroupPatch) (*models.DirectoryGroup, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var updated *models.DirectoryGroup
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryGroup(ctx, tx, organizationID, groupID, patch.ExpectedVersion)
		if err != nil {
			return err
		}
		name, slug, description, rule := current.Name, current.Slug, current.Description, current.MembershipRule
		if patch.Name != nil {
			name = *patch.Name
		}
		if patch.Slug != nil {
			slug = *patch.Slug
		}
		if patch.ClearDescription {
			description = ""
		} else if patch.Description != nil {
			description = *patch.Description
		}
		if len(patch.MembershipRule) > 0 {
			rule = patch.MembershipRule
		}
		tag, err := tx.Exec(ctx, `UPDATE directory_groups SET name=$4,slug=$5,description=NULLIF($6,''),
			membership_rule=$7::jsonb,updated_by=$3::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$8 AND deleted_at IS NULL`,
			organizationID, groupID, actorID, name, slug, description, rule, patch.ExpectedVersion)
		if err != nil {
			return classifyDirectoryWrite(err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDirectoryVersionConflict
		}
		updated, err = getDirectoryGroup(ctx, tx, organizationID, groupID, false)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "group", groupID, nil, "group_updated",
			actorID, updated.Version, patch.Reason, directoryGroupState(current), directoryGroupState(updated))
	})
	return updated, err
}

func (r *userAdministrationRepo) DeleteGroup(ctx context.Context, organizationID, groupID, actorID string, expectedVersion int64, reason string) error {
	q := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryGroup(ctx, tx, organizationID, groupID, expectedVersion)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE directory_group_memberships SET removed_at=$3,removed_by=$4::uuid,remove_reason=$5
			WHERE organization_id=$1::uuid AND group_id=$2::uuid AND removed_at IS NULL`, organizationID, groupID, now, actorID, reason); err != nil {
			return fmt.Errorf("close deleted group memberships: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE directory_groups SET deleted_at=$4,updated_by=$3::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$5`, organizationID, groupID, actorID, now, expectedVersion); err != nil {
			return classifyDirectoryWrite(err)
		}
		deleted, err := getDirectoryGroup(ctx, tx, organizationID, groupID, true)
		if err != nil {
			return err
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "group", groupID, nil, "group_deleted",
			actorID, deleted.Version, reason, directoryGroupState(current), directoryGroupState(deleted))
	})
}

func lockDirectoryGroup(ctx context.Context, q database.Querier, organizationID, groupID string, expectedVersion int64) (*models.DirectoryGroup, error) {
	var actual int64
	if err := q.QueryRow(ctx, `SELECT version FROM directory_groups WHERE organization_id=$1::uuid AND id=$2::uuid
		AND deleted_at IS NULL FOR UPDATE`, organizationID, groupID).Scan(&actual); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDirectoryGroupNotFound
		}
		return nil, err
	}
	if actual != expectedVersion {
		return nil, ErrDirectoryVersionConflict
	}
	return getDirectoryGroup(ctx, q, organizationID, groupID, false)
}

func (r *userAdministrationRepo) ListGroupMembers(ctx context.Context, organizationID, groupID string, pagination models.PaginationRequest) ([]models.DirectoryUser, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	group, err := getDirectoryGroup(ctx, q, organizationID, groupID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrDirectoryGroupNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	if group.GroupType == models.DirectoryGroupDynamic {
		return listDynamicGroupMembers(ctx, q, organizationID, group.MembershipRule, pagination)
	}
	var total int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM directory_group_memberships gm JOIN users u
		ON u.organization_id=gm.organization_id AND u.id=gm.user_id AND u.deleted_at IS NULL
		WHERE gm.organization_id=$1::uuid AND gm.group_id=$2::uuid AND gm.removed_at IS NULL`, organizationID, groupID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count directory group members: %w", err)
	}
	rows, err := q.Query(ctx, directoryUserSelect+` JOIN directory_group_memberships member_filter
		ON member_filter.organization_id=u.organization_id AND member_filter.user_id=u.id
		AND member_filter.group_id=$2::uuid AND member_filter.removed_at IS NULL
		WHERE u.organization_id=$1::uuid AND u.deleted_at IS NULL
		ORDER BY u.last_name,u.first_name,u.email,u.id LIMIT $3 OFFSET $4`, organizationID, groupID,
		pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list directory group members: %w", err)
	}
	return scanDirectoryUsers(rows, pagination.PageSize, total)
}

func (r *userAdministrationRepo) ChangeGroupMembers(ctx context.Context, organizationID, groupID, actorID string, input models.DirectoryGroupBulkMembersInput) (*models.DirectoryGroup, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var updated *models.DirectoryGroup
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		current, err := lockDirectoryGroup(ctx, tx, organizationID, groupID, input.ExpectedVersion)
		if err != nil {
			return err
		}
		if current.GroupType != models.DirectoryGroupStatic {
			return ErrDirectoryDynamicGroup
		}
		for _, userID := range input.AddUserIDs {
			if err := ensureDirectoryActiveUser(ctx, tx, organizationID, userID); err != nil {
				return err
			}
		}
		changed := int64(0)
		for _, userID := range input.AddUserIDs {
			tag, err := tx.Exec(ctx, `INSERT INTO directory_group_memberships(
				organization_id,group_id,user_id,added_by,add_reason)
				SELECT $1::uuid,$2::uuid,$3::uuid,$4::uuid,$5 WHERE NOT EXISTS(
					SELECT 1 FROM directory_group_memberships WHERE organization_id=$1::uuid
					AND group_id=$2::uuid AND user_id=$3::uuid AND removed_at IS NULL)`,
				organizationID, groupID, userID, actorID, input.Reason)
			if err != nil {
				return classifyDirectoryWrite(err)
			}
			changed += tag.RowsAffected()
		}
		now := time.Now().UTC()
		for _, userID := range input.RemoveUserIDs {
			tag, err := tx.Exec(ctx, `UPDATE directory_group_memberships SET removed_at=$4,
				removed_by=$5::uuid,remove_reason=$6 WHERE organization_id=$1::uuid AND group_id=$2::uuid
				AND user_id=$3::uuid AND removed_at IS NULL`, organizationID, groupID, userID, now, actorID, input.Reason)
			if err != nil {
				return fmt.Errorf("remove directory group member: %w", err)
			}
			changed += tag.RowsAffected()
		}
		if changed == 0 {
			updated = current
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE directory_groups SET version=version+1,updated_by=$3::uuid
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$4`, organizationID, groupID, actorID, input.ExpectedVersion); err != nil {
			return err
		}
		updated, err = getDirectoryGroup(ctx, tx, organizationID, groupID, false)
		if err != nil {
			return err
		}
		eventType := "members_bulk_changed"
		var target *string
		if len(input.AddUserIDs) == 1 && len(input.RemoveUserIDs) == 0 {
			eventType, target = "member_added", &input.AddUserIDs[0]
		} else if len(input.RemoveUserIDs) == 1 && len(input.AddUserIDs) == 0 {
			eventType, target = "member_removed", &input.RemoveUserIDs[0]
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "group", groupID, target, eventType,
			actorID, updated.Version, input.Reason, directoryGroupState(current), map[string]any{
				"group": directoryGroupState(updated), "added_user_ids": input.AddUserIDs,
				"removed_user_ids": input.RemoveUserIDs, "changed_count": changed})
	})
	return updated, err
}

func listDynamicGroupMembers(ctx context.Context, q database.Querier, organizationID string, raw json.RawMessage, pagination models.PaginationRequest) ([]models.DirectoryUser, int, error) {
	predicate, args, err := dynamicGroupPredicate(organizationID, raw)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM users u WHERE `+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count dynamic group members: %w", err)
	}
	args = append(args, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	rows, err := q.Query(ctx, directoryUserSelect+` WHERE `+predicate+` ORDER BY u.last_name,u.first_name,u.email,u.id`+
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list dynamic group members: %w", err)
	}
	return scanDirectoryUsers(rows, pagination.PageSize, total)
}

func countDynamicGroupMembers(ctx context.Context, q database.Querier, organizationID string, raw json.RawMessage) (int, error) {
	predicate, args, err := dynamicGroupPredicate(organizationID, raw)
	if err != nil {
		return 0, err
	}
	var count int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM users u WHERE `+predicate, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count dynamic directory members: %w", err)
	}
	return count, nil
}

func dynamicGroupPredicate(organizationID string, raw json.RawMessage) (string, []any, error) {
	var rule models.DirectoryDynamicGroupRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		return "", nil, fmt.Errorf("decode dynamic group rule: %w", err)
	}
	where := []string{"u.organization_id=$1::uuid", "u.deleted_at IS NULL"}
	args := []any{organizationID}
	if len(rule.Departments) > 0 {
		args = append(args, rule.Departments)
		where = append(where, fmt.Sprintf(`COALESCE(u.department,'')=ANY($%d::text[])`, len(args)))
	}
	if len(rule.Locations) > 0 {
		args = append(args, rule.Locations)
		where = append(where, fmt.Sprintf(`COALESCE(u.location,'')=ANY($%d::text[])`, len(args)))
	}
	if len(rule.Statuses) > 0 {
		statuses := make([]string, len(rule.Statuses))
		for i := range rule.Statuses {
			statuses[i] = string(rule.Statuses[i])
		}
		args = append(args, statuses)
		where = append(where, fmt.Sprintf(`u.status::text=ANY($%d::text[])`, len(args)))
	}
	if len(rule.RoleSlugs) > 0 {
		args = append(args, rule.RoleSlugs)
		where = append(where, fmt.Sprintf(`EXISTS(SELECT 1 FROM effective_user_roles dynamic_ur JOIN roles dynamic_role
			ON dynamic_role.id=dynamic_ur.role_id AND dynamic_role.deleted_at IS NULL
			WHERE dynamic_ur.organization_id=u.organization_id AND dynamic_ur.user_id=u.id
			AND dynamic_role.slug=ANY($%d::text[]))`, len(args)))
	}
	return strings.Join(where, " AND "), args, nil
}

func scanDirectoryUsers(rows pgx.Rows, capacity, total int) ([]models.DirectoryUser, int, error) {
	defer rows.Close()
	items := make([]models.DirectoryUser, 0, capacity)
	for rows.Next() {
		item, err := scanDirectoryUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan directory group member: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate directory group members: %w", err)
	}
	return items, total, nil
}

func directoryGroupState(item *models.DirectoryGroup) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{"id": item.ID, "name": item.Name, "slug": item.Slug, "description": item.Description,
		"group_type": item.GroupType, "membership_rule": item.MembershipRule, "member_count": item.MemberCount,
		"version": item.Version}
}

func directoryGroupSortColumn(value string) string {
	switch value {
	case "name":
		return "g.name"
	case "slug":
		return "g.slug"
	case "created_at":
		return "g.created_at"
	default:
		return "g.updated_at"
	}
}
