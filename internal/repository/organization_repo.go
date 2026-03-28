package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// OrganizationRepository defines data-access operations for organizations.
type OrganizationRepository interface {
	Create(ctx context.Context, org *models.Organization) error
	GetByID(ctx context.Context, id string) (*models.Organization, error)
	Update(ctx context.Context, org *models.Organization) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Organization, int, error)
	GetByDomain(ctx context.Context, domain string) (*models.Organization, error)
}

type organizationRepo struct {
	pool *pgxpool.Pool
}

// NewOrganizationRepository returns a concrete OrganizationRepository backed by pgxpool.
func NewOrganizationRepository(pool *pgxpool.Pool) OrganizationRepository {
	return &organizationRepo{pool: pool}
}

func (r *organizationRepo) Create(ctx context.Context, org *models.Organization) error {
	query := `
		INSERT INTO organizations (id, name, domain, industry, country, timezone,
			subscription_tier, logo_url, is_active, settings, max_users, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		org.Name,
		org.Domain,
		org.Industry,
		org.Country,
		org.Timezone,
		org.SubscriptionTier,
		org.LogoURL,
		org.IsActive,
		org.Settings,
		org.MaxUsers,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
}

func (r *organizationRepo) GetByID(ctx context.Context, id string) (*models.Organization, error) {
	query := `
		SELECT id, name, domain, industry, country, timezone,
			subscription_tier, logo_url, is_active, settings, max_users,
			created_at, updated_at
		FROM organizations
		WHERE id = $1 AND deleted_at IS NULL`

	org := &models.Organization{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&org.ID,
		&org.Name,
		&org.Domain,
		&org.Industry,
		&org.Country,
		&org.Timezone,
		&org.SubscriptionTier,
		&org.LogoURL,
		&org.IsActive,
		&org.Settings,
		&org.MaxUsers,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}
	return org, nil
}

func (r *organizationRepo) Update(ctx context.Context, org *models.Organization) error {
	// TODO: UPDATE organizations SET name=$2, domain=$3, industry=$4, country=$5,
	//   timezone=$6, subscription_tier=$7, logo_url=$8, is_active=$9, settings=$10,
	//   max_users=$11, updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL
	_ = ctx
	_ = org
	return fmt.Errorf("not implemented")
}

func (r *organizationRepo) Delete(ctx context.Context, id string) error {
	// TODO: UPDATE organizations SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL
	_ = ctx
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *organizationRepo) List(ctx context.Context, pagination models.PaginationRequest) ([]models.Organization, int, error) {
	countQuery := `SELECT COUNT(*) FROM organizations WHERE deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting organizations: %w", err)
	}

	query := `
		SELECT id, name, domain, industry, country, timezone,
			subscription_tier, logo_url, is_active, settings, max_users,
			created_at, updated_at
		FROM organizations
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing organizations: %w", err)
	}
	defer rows.Close()

	var orgs []models.Organization
	for rows.Next() {
		var org models.Organization
		if err := rows.Scan(
			&org.ID,
			&org.Name,
			&org.Domain,
			&org.Industry,
			&org.Country,
			&org.Timezone,
			&org.SubscriptionTier,
			&org.LogoURL,
			&org.IsActive,
			&org.Settings,
			&org.MaxUsers,
			&org.CreatedAt,
			&org.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning organization row: %w", err)
		}
		orgs = append(orgs, org)
	}
	return orgs, total, rows.Err()
}

func (r *organizationRepo) GetByDomain(ctx context.Context, domain string) (*models.Organization, error) {
	// TODO: SELECT ... FROM organizations WHERE domain=$1 AND deleted_at IS NULL
	_ = ctx
	_ = domain
	return nil, fmt.Errorf("not implemented")
}
