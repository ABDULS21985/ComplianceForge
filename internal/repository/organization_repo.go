package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

// OrganizationRepository defines data-access operations for organizations.
type OrganizationRepository interface {
	Create(ctx context.Context, org *models.Organization) error
	GetByID(ctx context.Context, id string) (*models.Organization, error)
	Update(ctx context.Context, org *models.Organization) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, pagination models.PaginationRequest) ([]models.Organization, int, error)
	GetBySlug(ctx context.Context, slug string) (*models.Organization, error)
}

type organizationRepo struct {
	pool *pgxpool.Pool
}

var _ OrganizationRepository = (*organizationRepo)(nil)

// NewOrganizationRepository returns a concrete OrganizationRepository backed by pgxpool.
func NewOrganizationRepository(pool *pgxpool.Pool) OrganizationRepository {
	return &organizationRepo{pool: pool}
}

const organizationSelect = `
	SELECT id, name, slug, COALESCE(legal_name, ''),
		COALESCE(registration_number, ''), COALESCE(tax_id, ''),
		COALESCE(industry, ''), COALESCE(sector, ''), COALESCE(country_code, ''),
		headquarters_address, status::text, tier::text, settings, branding,
		COALESCE(timezone, 'Europe/London'), COALESCE(default_language, 'en'),
		supported_languages, COALESCE(employee_count_range, ''),
		COALESCE(annual_revenue_range, ''), parent_organization_id, metadata,
		created_at, updated_at, deleted_at
	FROM organizations`

func scanOrganization(row rowScanner) (*models.Organization, error) {
	org := &models.Organization{}
	if err := row.Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.LegalName,
		&org.RegistrationNumber,
		&org.TaxID,
		&org.Industry,
		&org.Sector,
		&org.CountryCode,
		&org.HeadquartersAddress,
		&org.Status,
		&org.Tier,
		&org.Settings,
		&org.Branding,
		&org.Timezone,
		&org.DefaultLanguage,
		&org.SupportedLanguages,
		&org.EmployeeCountRange,
		&org.AnnualRevenueRange,
		&org.ParentOrganizationID,
		&org.Metadata,
		&org.CreatedAt,
		&org.UpdatedAt,
		&org.DeletedAt,
	); err != nil {
		return nil, err
	}
	return org, nil
}

func (r *organizationRepo) Create(ctx context.Context, org *models.Organization) error {
	if org.ID == "" {
		org.ID = uuid.NewString()
	}
	applyOrganizationDefaults(org)

	query := `
		INSERT INTO organizations (
			id, name, slug, legal_name, registration_number, tax_id, industry,
			sector, country_code, headquarters_address, status, tier, settings,
			branding, timezone, default_language, supported_languages,
			employee_count_range, annual_revenue_range, parent_organization_id,
			metadata
		)
		VALUES (
			$1, $2, LOWER($3), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''),
			NULLIF($7, ''), NULLIF($8, ''), NULLIF(UPPER($9), ''), $10,
			$11::org_status, $12::org_tier, $13, $14, $15, $16, $17,
			NULLIF($18, ''), NULLIF($19, ''), $20, $21
		)
		RETURNING created_at, updated_at`
	if err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, query,
		org.ID,
		org.Name,
		strings.TrimSpace(org.Slug),
		org.LegalName,
		org.RegistrationNumber,
		org.TaxID,
		org.Industry,
		org.Sector,
		org.CountryCode,
		org.HeadquartersAddress,
		org.Status,
		org.Tier,
		org.Settings,
		org.Branding,
		org.Timezone,
		org.DefaultLanguage,
		org.SupportedLanguages,
		org.EmployeeCountRange,
		org.AnnualRevenueRange,
		org.ParentOrganizationID,
		org.Metadata,
	).Scan(&org.CreatedAt, &org.UpdatedAt); err != nil {
		return fmt.Errorf("creating organization: %w", err)
	}
	return nil
}

func (r *organizationRepo) GetByID(ctx context.Context, id string) (*models.Organization, error) {
	org, err := scanOrganization(database.QuerierFromContext(ctx, r.pool).QueryRow(
		ctx,
		organizationSelect+` WHERE id = $1 AND deleted_at IS NULL`,
		id,
	))
	if err != nil {
		return nil, fmt.Errorf("getting organization: %w", err)
	}
	return org, nil
}

func (r *organizationRepo) Update(ctx context.Context, org *models.Organization) error {
	applyOrganizationDefaults(org)
	tag, err := database.QuerierFromContext(ctx, r.pool).Exec(ctx, `
		UPDATE organizations
		SET name = $2, slug = LOWER($3), legal_name = NULLIF($4, ''),
			registration_number = NULLIF($5, ''), tax_id = NULLIF($6, ''),
			industry = NULLIF($7, ''), sector = NULLIF($8, ''),
			country_code = NULLIF(UPPER($9), ''), headquarters_address = $10,
			status = $11::org_status, tier = $12::org_tier, settings = $13,
			branding = $14, timezone = $15, default_language = $16,
			supported_languages = $17, employee_count_range = NULLIF($18, ''),
			annual_revenue_range = NULLIF($19, ''), parent_organization_id = $20,
			metadata = $21
		WHERE id = $1 AND deleted_at IS NULL`,
		org.ID,
		org.Name,
		strings.TrimSpace(org.Slug),
		org.LegalName,
		org.RegistrationNumber,
		org.TaxID,
		org.Industry,
		org.Sector,
		org.CountryCode,
		org.HeadquartersAddress,
		org.Status,
		org.Tier,
		org.Settings,
		org.Branding,
		org.Timezone,
		org.DefaultLanguage,
		org.SupportedLanguages,
		org.EmployeeCountRange,
		org.AnnualRevenueRange,
		org.ParentOrganizationID,
		org.Metadata,
	)
	if err != nil {
		return fmt.Errorf("updating organization: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *organizationRepo) Delete(ctx context.Context, id string) error {
	tag, err := database.QuerierFromContext(ctx, r.pool).Exec(ctx, `
		UPDATE organizations
		SET deleted_at = NOW(), status = 'deactivated'
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("deleting organization: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *organizationRepo) List(ctx context.Context, pagination models.PaginationRequest) ([]models.Organization, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*) FROM organizations WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting organizations: %w", err)
	}

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := querier.Query(ctx, organizationSelect+`
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2`, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing organizations: %w", err)
	}
	defer rows.Close()

	organizations := make([]models.Organization, 0)
	for rows.Next() {
		org, err := scanOrganization(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning organization row: %w", err)
		}
		organizations = append(organizations, *org)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listing organizations: %w", err)
	}
	return organizations, total, nil
}

func (r *organizationRepo) GetBySlug(ctx context.Context, slug string) (*models.Organization, error) {
	org, err := scanOrganization(database.QuerierFromContext(ctx, r.pool).QueryRow(
		ctx,
		organizationSelect+` WHERE LOWER(slug) = LOWER($1) AND deleted_at IS NULL`,
		strings.TrimSpace(slug),
	))
	if err != nil {
		return nil, fmt.Errorf("getting organization by slug: %w", err)
	}
	return org, nil
}

func applyOrganizationDefaults(org *models.Organization) {
	if org.Status == "" {
		org.Status = "trial"
	}
	if org.Tier == "" {
		org.Tier = "starter"
	}
	if org.Timezone == "" {
		org.Timezone = "Europe/London"
	}
	if org.DefaultLanguage == "" {
		org.DefaultLanguage = "en"
	}
	if len(org.SupportedLanguages) == 0 {
		org.SupportedLanguages = []string{org.DefaultLanguage}
	}
	if org.HeadquartersAddress == nil {
		org.HeadquartersAddress = map[string]any{}
	}
	if org.Settings == nil {
		org.Settings = map[string]any{}
	}
	if org.Branding == nil {
		org.Branding = map[string]any{}
	}
	if org.Metadata == nil {
		org.Metadata = map[string]any{}
	}
}
