package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

var directoryRoleSlugPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,99}$`)

type importExistingUser struct {
	id       string
	deleted  bool
	status   models.UserStatus
	version  int64
	employee string
}

func (r *userAdministrationRepo) PreviewImport(ctx context.Context, organizationID string, rows []models.DirectoryImportRow, contentHash string) (*models.DirectoryImportPreview, error) {
	return previewDirectoryImport(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, rows, contentHash)
}

func previewDirectoryImport(ctx context.Context, q database.Querier, organizationID string, rows []models.DirectoryImportRow, contentHash string) (*models.DirectoryImportPreview, error) {
	preview := &models.DirectoryImportPreview{ContentSHA256: contentHash, RowCount: len(rows), Rows: make([]models.DirectoryImportRowResult, len(rows))}
	emails := make([]string, 0, len(rows)*2)
	roleSlugs := make([]string, 0, len(rows))
	for _, row := range rows {
		emails = append(emails, row.Email)
		if row.ManagerEmail != "" {
			emails = append(emails, row.ManagerEmail)
		}
		roleSlugs = append(roleSlugs, row.RoleSlug)
	}
	existing, err := loadImportUsers(ctx, q, organizationID, emails)
	if err != nil {
		return nil, err
	}
	roles, err := loadImportRoles(ctx, q, organizationID, roleSlugs)
	if err != nil {
		return nil, err
	}
	inputEmails := make(map[string]bool, len(rows))
	inputEmployees := make(map[string]bool, len(rows))
	for _, row := range rows {
		inputEmails[row.Email] = true
	}
	existingEmployees, err := loadImportEmployeeIDs(ctx, q, organizationID, rows)
	if err != nil {
		return nil, err
	}
	seenEmails := map[string]bool{}
	for index, row := range rows {
		result := models.DirectoryImportRowResult{RowNumber: row.RowNumber, Email: row.Email, Errors: []string{}, Warnings: []string{}}
		if !validImportEmail(row.Email) {
			result.Errors = append(result.Errors, "email is invalid")
		}
		if seenEmails[row.Email] {
			result.Errors = append(result.Errors, "email is duplicated in the import")
		}
		seenEmails[row.Email] = true
		if !validImportText(row.FirstName, 0, 100) || !validImportText(row.LastName, 0, 100) ||
			!validImportText(row.JobTitle, 0, 200) || !validImportText(row.Department, 0, 200) ||
			!validImportText(row.Location, 0, 200) || !validImportText(row.EmployeeID, 0, 100) {
			result.Errors = append(result.Errors, "one or more profile fields exceed their allowed length")
		}
		if row.FirstName == "" && row.LastName == "" {
			result.Errors = append(result.Errors, "first_name or last_name is required")
		}
		if row.RoleSlug == "" || !directoryRoleSlugPattern.MatchString(row.RoleSlug) || roles[row.RoleSlug] == "" {
			result.Errors = append(result.Errors, "role_slug is not available in this tenant")
		}
		if row.ManagerEmail != "" {
			if !validImportEmail(row.ManagerEmail) || row.ManagerEmail == row.Email {
				result.Errors = append(result.Errors, "manager_email is invalid")
			} else if manager, ok := existing[row.ManagerEmail]; ok && (manager.deleted || manager.status == models.UserStatusInactive) {
				result.Errors = append(result.Errors, "manager_email belongs to an inactive or deprovisioned user")
			} else if !ok && !inputEmails[row.ManagerEmail] {
				result.Errors = append(result.Errors, "manager_email is not present in the tenant or this import")
			}
		}
		if row.EmployeeID != "" {
			key := strings.ToLower(row.EmployeeID)
			if inputEmployees[key] {
				result.Errors = append(result.Errors, "employee_id is duplicated in the import")
			}
			inputEmployees[key] = true
			if existingEmail := existingEmployees[key]; existingEmail != "" && existingEmail != row.Email {
				result.Errors = append(result.Errors, "employee_id belongs to another user")
			}
		}
		if current, ok := existing[row.Email]; ok {
			if current.deleted {
				result.Errors = append(result.Errors, "email belongs to a deprovisioned account; restore it through an administrator workflow")
			} else {
				result.Action = "update"
				preview.UpdateCount++
				if current.status == models.UserStatusInactive || current.status == models.UserStatusLocked {
					result.Warnings = append(result.Warnings, "profile will update without reactivating the account")
				}
			}
		} else {
			result.Action = "create"
			preview.CreateCount++
		}
		if len(result.Errors) > 0 {
			preview.InvalidCount++
		} else {
			preview.ValidCount++
		}
		preview.Rows[index] = result
	}
	return preview, nil
}

func (r *userAdministrationRepo) ApplyImport(ctx context.Context, organizationID, actorID, idempotencyKey, contentHash, reason string, rows []models.DirectoryImportRow) (*models.DirectoryImportResult, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var result *models.DirectoryImportResult
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if err := ensureDirectoryActiveUser(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':directory-import:'||$2,0))`, organizationID, idempotencyKey); err != nil {
			return fmt.Errorf("lock directory import idempotency key: %w", err)
		}
		existingResult, err := getDirectoryImportByKey(ctx, tx, organizationID, idempotencyKey)
		if err == nil {
			if existingResult.ContentSHA256 != contentHash {
				return ErrDirectoryImportConflict
			}
			existingResult.Replayed = true
			result = existingResult
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		preview, err := previewDirectoryImport(ctx, tx, organizationID, rows, contentHash)
		if err != nil {
			return err
		}
		if preview.InvalidCount > 0 {
			return ErrDirectoryImportInvalid
		}
		if preview.CreateCount > 0 {
			if err := EnsureEntitlementCapacity(ctx, tx, organizationID, "users", int64(preview.CreateCount)); err != nil {
				return err
			}
		}
		result = &models.DirectoryImportResult{
			ID: uuid.NewString(), IdempotencyKey: idempotencyKey, ContentSHA256: contentHash,
			RowCount: len(rows), CreatedCount: preview.CreateCount, UpdatedCount: preview.UpdateCount,
			Rows: preview.Rows, AppliedAt: time.Now().UTC(),
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("encode directory import result: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO directory_imports(id,organization_id,idempotency_key,
			content_sha256,row_count,created_count,updated_count,skipped_count,result,created_by,applied_at)
			VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::uuid,$11)`, result.ID,
			organizationID, idempotencyKey, contentHash, result.RowCount, result.CreatedCount,
			result.UpdatedCount, result.SkippedCount, resultJSON, actorID, result.AppliedAt); err != nil {
			var postgresError *pgconn.PgError
			if errors.As(err, &postgresError) && postgresError.Code == "23505" {
				return ErrDirectoryImportConflict
			}
			return fmt.Errorf("create directory import: %w", err)
		}

		before := make(map[string]*models.DirectoryUser, len(rows))
		userIDs := make(map[string]string, len(rows))
		for _, row := range rows {
			current, getErr := getDirectoryUserByEmail(ctx, tx, organizationID, row.Email, false)
			switch {
			case getErr == nil:
				before[row.Email] = current
				userIDs[row.Email] = current.ID
				if _, err := tx.Exec(ctx, `UPDATE users SET first_name=NULLIF($4,''),last_name=NULLIF($5,''),
					job_title=NULLIF($6,''),department=NULLIF($7,''),location=NULLIF($8,''),employee_id=NULLIF($9,''),
					updated_by=$3::uuid,version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid`,
					organizationID, current.ID, actorID, row.FirstName, row.LastName, row.JobTitle,
					row.Department, row.Location, row.EmployeeID); err != nil {
					return classifyDirectoryWrite(err)
				}
			case errors.Is(getErr, pgx.ErrNoRows):
				id := uuid.NewString()
				userIDs[row.Email] = id
				if _, err := tx.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,job_title,
					department,location,employee_id,status,language,invitation_status,invited_at,invitation_expires_at,
					invited_by,updated_by) VALUES($1::uuid,$2::uuid,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),
					NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),'pending_verification','en','ready',$10,$11,$12::uuid,$12::uuid)`,
					id, organizationID, row.Email, row.FirstName, row.LastName, row.JobTitle, row.Department,
					row.Location, row.EmployeeID, result.AppliedAt, result.AppliedAt.Add(7*24*time.Hour), actorID); err != nil {
					return classifyDirectoryWrite(err)
				}
			default:
				return getErr
			}
		}

		for _, row := range rows {
			userID := userIDs[row.Email]
			if row.ManagerEmail != "" {
				managerID := userIDs[row.ManagerEmail]
				if managerID == "" {
					manager, err := getDirectoryUserByEmail(ctx, tx, organizationID, row.ManagerEmail, false)
					if err != nil {
						return fmt.Errorf("resolve imported manager: %w", err)
					}
					managerID = manager.ID
				}
				if _, err := tx.Exec(ctx, `UPDATE users SET manager_user_id=$3::uuid
					WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, userID, managerID); err != nil {
					return classifyDirectoryWrite(err)
				}
			}
			roleID, roleVersion, err := resolveDirectoryRole(ctx, tx, organizationID, row.RoleSlug)
			if err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id,assigned_by)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid) ON CONFLICT DO NOTHING`, userID, roleID, organizationID, actorID)
			if err != nil {
				return classifyDirectoryWrite(err)
			}
			if tag.RowsAffected() == 1 {
				if err := appendInitialDirectoryRoleEvent(ctx, tx, organizationID, roleID, roleVersion, userID, actorID, reason); err != nil {
					return err
				}
			}
			after, err := getDirectoryUser(ctx, tx, organizationID, userID, false)
			if err != nil {
				return err
			}
			eventType := "user_created"
			if before[row.Email] != nil {
				eventType = "user_updated"
			}
			if err := r.recordDirectoryEvent(ctx, tx, organizationID, "user", userID, &userID, eventType,
				actorID, after.Version, reason, directoryUserState(before[row.Email]), directoryUserState(after)); err != nil {
				return err
			}
		}
		return r.recordDirectoryEvent(ctx, tx, organizationID, "import", result.ID, nil, "import_applied",
			actorID, 1, reason, nil, map[string]any{"content_sha256": contentHash, "row_count": len(rows),
				"created_count": result.CreatedCount, "updated_count": result.UpdatedCount})
	})
	return result, err
}

func getDirectoryImportByKey(ctx context.Context, q database.Querier, organizationID, key string) (*models.DirectoryImportResult, error) {
	var result models.DirectoryImportResult
	var encoded []byte
	if err := q.QueryRow(ctx, `SELECT id,idempotency_key,content_sha256,row_count,created_count,
		updated_count,skipped_count,result,applied_at FROM directory_imports
		WHERE organization_id=$1::uuid AND idempotency_key=$2`, organizationID, key).Scan(
		&result.ID, &result.IdempotencyKey, &result.ContentSHA256, &result.RowCount,
		&result.CreatedCount, &result.UpdatedCount, &result.SkippedCount, &encoded, &result.AppliedAt); err != nil {
		return nil, err
	}
	var stored models.DirectoryImportResult
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, fmt.Errorf("decode stored directory import: %w", err)
	}
	result.Rows = stored.Rows
	return &result, nil
}

func getDirectoryUserByEmail(ctx context.Context, q database.Querier, organizationID, email string, includeDeleted bool) (*models.DirectoryUser, error) {
	predicate := " AND u.deleted_at IS NULL"
	if includeDeleted {
		predicate = ""
	}
	return scanDirectoryUser(q.QueryRow(ctx, directoryUserSelect+` WHERE u.organization_id=$1::uuid AND lower(u.email)=$2`+predicate, organizationID, email))
}

func loadImportUsers(ctx context.Context, q database.Querier, organizationID string, emails []string) (map[string]importExistingUser, error) {
	items := map[string]importExistingUser{}
	if len(emails) == 0 {
		return items, nil
	}
	rows, err := q.Query(ctx, `SELECT lower(email),id,deleted_at IS NOT NULL,status::text,version,COALESCE(employee_id,'')
		FROM users WHERE organization_id=$1::uuid AND lower(email)=ANY($2::text[])`, organizationID, emails)
	if err != nil {
		return nil, fmt.Errorf("load import users: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var email, status string
		var item importExistingUser
		if err := rows.Scan(&email, &item.id, &item.deleted, &status, &item.version, &item.employee); err != nil {
			return nil, err
		}
		item.status = models.UserStatus(status)
		items[email] = item
	}
	return items, rows.Err()
}

func loadImportRoles(ctx context.Context, q database.Querier, organizationID string, slugs []string) (map[string]string, error) {
	items := map[string]string{}
	rows, err := q.Query(ctx, `SELECT DISTINCT ON (slug) slug,id FROM roles WHERE slug=ANY($2::text[])
		AND deleted_at IS NULL AND (organization_id=$1::uuid OR (organization_id IS NULL AND is_system_role))
		ORDER BY slug,organization_id IS NOT NULL DESC`, organizationID, slugs)
	if err != nil {
		return nil, fmt.Errorf("load import roles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug, id string
		if err := rows.Scan(&slug, &id); err != nil {
			return nil, err
		}
		items[slug] = id
	}
	return items, rows.Err()
}

func loadImportEmployeeIDs(ctx context.Context, q database.Querier, organizationID string, input []models.DirectoryImportRow) (map[string]string, error) {
	ids := []string{}
	for _, row := range input {
		if row.EmployeeID != "" {
			ids = append(ids, strings.ToLower(row.EmployeeID))
		}
	}
	items := map[string]string{}
	if len(ids) == 0 {
		return items, nil
	}
	rows, err := q.Query(ctx, `SELECT lower(employee_id),lower(email) FROM users
		WHERE organization_id=$1::uuid AND employee_id IS NOT NULL AND deleted_at IS NULL
		AND lower(employee_id)=ANY($2::text[])`, organizationID, ids)
	if err != nil {
		return nil, fmt.Errorf("load import employee IDs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var employeeID, email string
		if err := rows.Scan(&employeeID, &email); err != nil {
			return nil, err
		}
		items[employeeID] = email
	}
	return items, rows.Err()
}

func validImportEmail(value string) bool {
	if value == "" || len(value) > 320 {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}

func validImportText(value string, minimum, maximum int) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	return length >= minimum && length <= maximum && !strings.ContainsRune(value, '\x00')
}
