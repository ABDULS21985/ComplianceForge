package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// UserRepository defines data-access operations for users.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, orgID, id string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, orgID, id string) error
	ListByOrganization(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.User, int, error)
	UpdateLastLogin(ctx context.Context, orgID, id string, loginTime time.Time) error
	UpdatePassword(ctx context.Context, orgID, id, passwordHash string) error
}

type userRepo struct {
	pool *pgxpool.Pool
}

// NewUserRepository returns a concrete UserRepository backed by pgxpool.
func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepo{pool: pool}
}

func (r *userRepo) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, organization_id, email, password_hash, first_name, last_name,
			role, department, phone, is_active, mfa_enabled, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		user.OrganizationID,
		user.Email,
		user.PasswordHash,
		user.FirstName,
		user.LastName,
		user.Role,
		user.Department,
		user.Phone,
		user.IsActive,
		user.MFAEnabled,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (r *userRepo) GetByID(ctx context.Context, orgID, id string) (*models.User, error) {
	query := `
		SELECT id, organization_id, email, password_hash, first_name, last_name,
			role, department, phone, is_active, last_login_at, mfa_enabled,
			created_at, updated_at
		FROM users
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	u := &models.User{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&u.ID,
		&u.OrganizationID,
		&u.Email,
		&u.PasswordHash,
		&u.FirstName,
		&u.LastName,
		&u.Role,
		&u.Department,
		&u.Phone,
		&u.IsActive,
		&u.LastLoginAt,
		&u.MFAEnabled,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	// TODO: SELECT ... FROM users WHERE email=$1 AND deleted_at IS NULL
	_ = ctx
	_ = email
	return nil, fmt.Errorf("not implemented")
}

func (r *userRepo) Update(ctx context.Context, user *models.User) error {
	// TODO: UPDATE users SET email=$3, first_name=$4, last_name=$5, role=$6,
	//   department=$7, phone=$8, is_active=$9, mfa_enabled=$10, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = user
	return fmt.Errorf("not implemented")
}

func (r *userRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE users SET deleted_at=NOW() WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *userRepo) ListByOrganization(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.User, int, error) {
	countQuery := `SELECT COUNT(*) FROM users WHERE organization_id = $1 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	query := `
		SELECT id, organization_id, email, password_hash, first_name, last_name,
			role, department, phone, is_active, last_login_at, mfa_enabled,
			created_at, updated_at
		FROM users
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID,
			&u.OrganizationID,
			&u.Email,
			&u.PasswordHash,
			&u.FirstName,
			&u.LastName,
			&u.Role,
			&u.Department,
			&u.Phone,
			&u.IsActive,
			&u.LastLoginAt,
			&u.MFAEnabled,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *userRepo) UpdateLastLogin(ctx context.Context, orgID, id string, loginTime time.Time) error {
	// TODO: UPDATE users SET last_login_at=$3, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	_ = loginTime
	return fmt.Errorf("not implemented")
}

func (r *userRepo) UpdatePassword(ctx context.Context, orgID, id, passwordHash string) error {
	// TODO: UPDATE users SET password_hash=$3, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	_ = passwordHash
	return fmt.Errorf("not implemented")
}
