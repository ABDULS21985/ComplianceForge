package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

var ErrAmbiguousUserEmail = errors.New("email belongs to more than one organization")

// UserRepository defines data-access operations for users and their sessions.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, orgID, id string) (*models.User, error)
	GetByEmail(ctx context.Context, orgID, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, orgID, id string) error
	ListByOrganization(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.User, int, error)
	UpdateLastLogin(ctx context.Context, orgID, id string, loginTime time.Time) error
	UpdatePassword(ctx context.Context, orgID, id, passwordHash string) error
	CreateSession(ctx context.Context, session *models.UserSession) error
	RotateSession(ctx context.Context, orgID, userID, currentRefreshHash string, session *models.UserSession) (bool, error)
	RevokeSession(ctx context.Context, orgID, userID, accessTokenHash string) error
	IsSessionActive(ctx context.Context, orgID, userID, accessTokenHash string) (bool, error)
}

type userRepo struct {
	pool *pgxpool.Pool
}

var _ UserRepository = (*userRepo)(nil)

// NewUserRepository returns a concrete UserRepository backed by pgxpool.
func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepo{pool: pool}
}

const userSelect = `
	SELECT u.id, u.organization_id, u.email, COALESCE(u.password_hash, ''),
		COALESCE(u.first_name, ''), COALESCE(u.last_name, ''),
		COALESCE(u.job_title, ''), COALESCE(u.department, ''),
		COALESCE(u.phone, ''), COALESCE(u.avatar_url, ''), u.status::text,
		u.is_super_admin, COALESCE(u.timezone, ''), COALESCE(u.language, 'en'),
		u.last_login_at,
		COALESCE((
			SELECT role.slug
			FROM effective_user_roles ur
			JOIN roles role ON role.id = ur.role_id AND role.deleted_at IS NULL
			WHERE ur.user_id = u.id AND ur.organization_id = u.organization_id
			ORDER BY ur.assigned_at ASC, role.slug ASC
			LIMIT 1
		), CASE WHEN u.is_super_admin THEN 'super_admin' ELSE 'viewer' END),
		EXISTS (
			SELECT 1 FROM user_mfa mfa
			WHERE mfa.organization_id = u.organization_id
				AND mfa.user_id = u.id
				AND mfa.is_verified = true
				AND mfa.disabled_at IS NULL
		),
		u.created_at, u.updated_at, u.deleted_at
	FROM users u`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*models.User, error) {
	user := &models.User{}
	var status string
	var role string
	if err := row.Scan(
		&user.ID,
		&user.OrganizationID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.JobTitle,
		&user.Department,
		&user.Phone,
		&user.AvatarURL,
		&status,
		&user.IsSuperAdmin,
		&user.Timezone,
		&user.Language,
		&user.LastLoginAt,
		&role,
		&user.MFAEnabled,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	); err != nil {
		return nil, err
	}
	user.Status = models.UserStatus(status)
	user.Role = models.UserRole(role)
	return user, nil
}

func (r *userRepo) Create(ctx context.Context, user *models.User) error {
	if user.ID == "" {
		user.ID = uuid.NewString()
	}
	if user.Status == "" {
		user.Status = models.UserStatusActive
	}
	if user.Role == "" {
		user.Role = models.UserRoleViewer
	}
	if user.Language == "" {
		user.Language = "en"
	}

	return database.WithTenantConnection(ctx, r.pool, user.OrganizationID, func(scopedCtx context.Context) error {
		querier := database.QuerierFromContext(scopedCtx, r.pool)
		return withTransaction(scopedCtx, querier, func(tx pgx.Tx) error {
			query := `
				INSERT INTO users (
					id, organization_id, email, password_hash, first_name, last_name,
					job_title, department, phone, avatar_url, status, is_super_admin,
					timezone, language
				)
				VALUES ($1, $2, LOWER($3), $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''),
					NULLIF($9, ''), NULLIF($10, ''), $11::user_status, $12,
					NULLIF($13, ''), $14)
				RETURNING created_at, updated_at`
			if err := tx.QueryRow(scopedCtx, query,
				user.ID,
				user.OrganizationID,
				strings.TrimSpace(user.Email),
				user.PasswordHash,
				user.FirstName,
				user.LastName,
				user.JobTitle,
				user.Department,
				user.Phone,
				user.AvatarURL,
				user.Status,
				user.IsSuperAdmin,
				user.Timezone,
				user.Language,
			).Scan(&user.CreatedAt, &user.UpdatedAt); err != nil {
				return fmt.Errorf("creating user: %w", err)
			}

			tag, err := tx.Exec(scopedCtx, `
				INSERT INTO user_roles (user_id, role_id, organization_id)
				SELECT $1, role.id, $2
				FROM roles role
				WHERE role.slug = $3
				  AND role.deleted_at IS NULL
				  AND (role.organization_id IS NULL OR role.organization_id = $2)
				ORDER BY (role.organization_id IS NOT NULL) DESC
				LIMIT 1
				ON CONFLICT DO NOTHING`, user.ID, user.OrganizationID, user.Role)
			if err != nil {
				return fmt.Errorf("assigning initial user role: %w", err)
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("assigning initial user role: role %q is not configured", user.Role)
			}
			return nil
		})
	})
}

func (r *userRepo) GetByID(ctx context.Context, orgID, id string) (*models.User, error) {
	var user *models.User
	err := database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		var err error
		user, err = scanUser(database.QuerierFromContext(scopedCtx, r.pool).QueryRow(
			scopedCtx,
			userSelect+` WHERE u.id = $1 AND u.organization_id = $2 AND u.deleted_at IS NULL`,
			id,
			orgID,
		))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return user, nil
}

func (r *userRepo) GetByEmail(ctx context.Context, orgID, email string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if orgID != "" {
		var user *models.User
		err := database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
			var err error
			user, err = scanUser(database.QuerierFromContext(scopedCtx, r.pool).QueryRow(
				scopedCtx,
				userSelect+` WHERE u.organization_id = $1 AND LOWER(u.email) = $2 AND u.deleted_at IS NULL`,
				orgID,
				email,
			))
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("getting user by email: %w", err)
		}
		return user, nil
	}

	// Legacy clients do not yet send an organization identifier. This lookup
	// succeeds only when the email resolves to exactly one tenant; production
	// database roles must grant this narrowly scoped authentication lookup.
	rows, err := r.pool.Query(ctx, userSelect+`
		WHERE LOWER(u.email) = $1 AND u.deleted_at IS NULL
		ORDER BY u.created_at ASC
		LIMIT 2`, email)
	if err != nil {
		return nil, fmt.Errorf("looking up login email: %w", err)
	}
	defer rows.Close()

	var found *models.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning login user: %w", err)
		}
		if found != nil {
			return nil, ErrAmbiguousUserEmail
		}
		found = user
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("looking up login email: %w", err)
	}
	if found == nil {
		return nil, pgx.ErrNoRows
	}
	return found, nil
}

func (r *userRepo) Update(ctx context.Context, user *models.User) error {
	return database.WithTenantConnection(ctx, r.pool, user.OrganizationID, func(scopedCtx context.Context) error {
		querier := database.QuerierFromContext(scopedCtx, r.pool)
		return withTransaction(scopedCtx, querier, func(tx pgx.Tx) error {
			tag, err := tx.Exec(scopedCtx, `
				UPDATE users
				SET email = LOWER($3), first_name = $4, last_name = $5,
					job_title = NULLIF($6, ''), department = NULLIF($7, ''),
					phone = NULLIF($8, ''), avatar_url = NULLIF($9, ''),
					status = $10::user_status, timezone = NULLIF($11, ''), language = $12
				WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
				user.ID, user.OrganizationID, strings.TrimSpace(user.Email), user.FirstName,
				user.LastName, user.JobTitle, user.Department, user.Phone, user.AvatarURL,
				user.Status, user.Timezone, user.Language)
			if err != nil {
				return fmt.Errorf("updating user: %w", err)
			}
			if tag.RowsAffected() != 1 {
				return pgx.ErrNoRows
			}
			return nil
		})
	})
}

func (r *userRepo) Delete(ctx context.Context, orgID, id string) error {
	return database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		tag, err := database.QuerierFromContext(scopedCtx, r.pool).Exec(scopedCtx, `
			UPDATE users
			SET deleted_at = NOW(), status = 'inactive'
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
		if err != nil {
			return fmt.Errorf("deleting user: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *userRepo) ListByOrganization(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.User, int, error) {
	var users []models.User
	var total int
	err := database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		querier := database.QuerierFromContext(scopedCtx, r.pool)
		if err := querier.QueryRow(scopedCtx,
			`SELECT COUNT(*) FROM users WHERE organization_id = $1 AND deleted_at IS NULL`, orgID,
		).Scan(&total); err != nil {
			return fmt.Errorf("counting users: %w", err)
		}

		offset := (pagination.Page - 1) * pagination.PageSize
		rows, err := querier.Query(scopedCtx, userSelect+`
			WHERE u.organization_id = $1 AND u.deleted_at IS NULL
			ORDER BY u.created_at DESC, u.id DESC
			LIMIT $2 OFFSET $3`, orgID, pagination.PageSize, offset)
		if err != nil {
			return fmt.Errorf("listing users: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			user, err := scanUser(rows)
			if err != nil {
				return fmt.Errorf("scanning user row: %w", err)
			}
			users = append(users, *user)
		}
		return rows.Err()
	})
	return users, total, err
}

func (r *userRepo) UpdateLastLogin(ctx context.Context, orgID, id string, loginTime time.Time) error {
	return database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		tag, err := database.QuerierFromContext(scopedCtx, r.pool).Exec(scopedCtx, `
			UPDATE users
			SET last_login_at = $3, failed_login_attempts = 0, locked_until = NULL
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID, loginTime)
		if err != nil {
			return fmt.Errorf("updating last login: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *userRepo) UpdatePassword(ctx context.Context, orgID, id, passwordHash string) error {
	return database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		tag, err := database.QuerierFromContext(scopedCtx, r.pool).Exec(scopedCtx, `
			UPDATE users
			SET password_hash = $3, password_changed_at = NOW()
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID, passwordHash)
		if err != nil {
			return fmt.Errorf("updating password: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *userRepo) CreateSession(ctx context.Context, session *models.UserSession) error {
	if session.ID == "" {
		session.ID = uuid.NewString()
	}
	if session.AuthenticationMethod == "" {
		session.AuthenticationMethod = models.IdentityMethodPassword
	}
	return database.WithTenantConnection(ctx, r.pool, session.OrganizationID, func(scopedCtx context.Context) error {
		return database.QuerierFromContext(scopedCtx, r.pool).QueryRow(scopedCtx, `
			INSERT INTO user_sessions (
				id, user_id, organization_id, token_hash, refresh_token_hash, ip_address,
				user_agent, device_name, authentication_method, mfa_verified_at, expires_at
			)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::inet, NULLIF($7, ''),
				NULLIF($8, ''), $9, $10, $11)
			RETURNING created_at`, session.ID, session.UserID, session.OrganizationID,
			session.TokenHash, session.RefreshTokenHash, session.IPAddress, session.UserAgent,
			session.DeviceName, session.AuthenticationMethod, session.MFAVerifiedAt, session.ExpiresAt,
		).Scan(&session.CreatedAt)
	})
}

func (r *userRepo) RotateSession(
	ctx context.Context,
	orgID string,
	userID string,
	currentRefreshHash string,
	session *models.UserSession,
) (bool, error) {
	var rotated bool
	err := database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		tag, err := database.QuerierFromContext(scopedCtx, r.pool).Exec(scopedCtx, `
			UPDATE user_sessions
			SET token_hash = $4, refresh_token_hash = $5, expires_at = $6,
				last_seen_at = NOW(), version = version + 1
			WHERE organization_id = $1 AND user_id = $2 AND refresh_token_hash = $3
			  AND revoked_at IS NULL AND expires_at > NOW()`,
			orgID, userID, currentRefreshHash, session.TokenHash,
			session.RefreshTokenHash, session.ExpiresAt)
		if err != nil {
			return fmt.Errorf("rotating user session: %w", err)
		}
		rotated = tag.RowsAffected() == 1
		return nil
	})
	return rotated, err
}

func (r *userRepo) RevokeSession(ctx context.Context, orgID, userID, accessTokenHash string) error {
	return database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		_, err := database.QuerierFromContext(scopedCtx, r.pool).Exec(scopedCtx, `
			UPDATE user_sessions
			SET revoked_at = COALESCE(revoked_at, NOW())
			WHERE organization_id = $1 AND user_id = $2 AND token_hash = $3`,
			orgID, userID, accessTokenHash)
		if err != nil {
			return fmt.Errorf("revoking user session: %w", err)
		}
		return nil
	})
}

func (r *userRepo) IsSessionActive(ctx context.Context, orgID, userID, accessTokenHash string) (bool, error) {
	var active bool
	err := database.WithTenantConnection(ctx, r.pool, orgID, func(scopedCtx context.Context) error {
		return database.QuerierFromContext(scopedCtx, r.pool).QueryRow(scopedCtx, `
			WITH eligible AS (
				SELECT session.id
				FROM user_sessions session
				JOIN users user_account
				  ON user_account.id = session.user_id
				 AND user_account.organization_id = session.organization_id
				WHERE session.organization_id = $1
				  AND session.user_id = $2
				  AND session.token_hash = $3
				  AND session.revoked_at IS NULL
				  AND session.expires_at > NOW()
				  AND user_account.status = 'active'
				  AND user_account.deleted_at IS NULL
			), touched AS (
				UPDATE user_sessions session
				SET last_seen_at = NOW()
				FROM eligible
				WHERE session.organization_id = $1
				  AND session.id = eligible.id
				  AND session.last_seen_at < NOW() - INTERVAL '5 minutes'
				RETURNING session.id
			)
			SELECT EXISTS (SELECT 1 FROM eligible)`, orgID, userID, accessTokenHash).Scan(&active)
	})
	return active, err
}

type transactionBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

func withTransaction(ctx context.Context, querier database.Querier, fn func(pgx.Tx) error) error {
	if tx, ok := querier.(pgx.Tx); ok {
		return fn(tx)
	}
	beginner, ok := querier.(transactionBeginner)
	if !ok {
		return fmt.Errorf("database executor does not support transactions")
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
