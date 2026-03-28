package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// FrameworkRepository defines data-access operations for compliance frameworks.
type FrameworkRepository interface {
	Create(ctx context.Context, framework *models.ComplianceFramework) error
	GetByID(ctx context.Context, orgID, id string) (*models.ComplianceFramework, error)
	Update(ctx context.Context, framework *models.ComplianceFramework) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.ComplianceFramework, int, error)
	ListByCategory(ctx context.Context, orgID, category string) ([]models.ComplianceFramework, error)
	GetWithControls(ctx context.Context, orgID, id string) (*models.ComplianceFramework, error)
}

type frameworkRepo struct {
	pool *pgxpool.Pool
}

// NewFrameworkRepository returns a concrete FrameworkRepository backed by pgxpool.
func NewFrameworkRepository(pool *pgxpool.Pool) FrameworkRepository {
	return &frameworkRepo{pool: pool}
}

func (r *frameworkRepo) Create(ctx context.Context, fw *models.ComplianceFramework) error {
	query := `
		INSERT INTO compliance_frameworks (id, organization_id, name, version, description,
			authority, category, is_active, effective_date, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		fw.OrganizationID,
		fw.Name,
		fw.Version,
		fw.Description,
		fw.Authority,
		fw.Category,
		fw.IsActive,
		fw.EffectiveDate,
	).Scan(&fw.ID, &fw.CreatedAt, &fw.UpdatedAt)
}

func (r *frameworkRepo) GetByID(ctx context.Context, orgID, id string) (*models.ComplianceFramework, error) {
	query := `
		SELECT id, organization_id, name, version, description, authority, category,
			is_active, effective_date, created_at, updated_at
		FROM compliance_frameworks
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	fw := &models.ComplianceFramework{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&fw.ID,
		&fw.OrganizationID,
		&fw.Name,
		&fw.Version,
		&fw.Description,
		&fw.Authority,
		&fw.Category,
		&fw.IsActive,
		&fw.EffectiveDate,
		&fw.CreatedAt,
		&fw.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("framework not found: %w", err)
	}
	return fw, nil
}

func (r *frameworkRepo) Update(ctx context.Context, fw *models.ComplianceFramework) error {
	// TODO: UPDATE compliance_frameworks SET name=$3, version=$4, description=$5,
	//   authority=$6, category=$7, is_active=$8, effective_date=$9, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = fw
	return fmt.Errorf("not implemented")
}

func (r *frameworkRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE compliance_frameworks SET deleted_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *frameworkRepo) List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.ComplianceFramework, int, error) {
	countQuery := `SELECT COUNT(*) FROM compliance_frameworks WHERE organization_id = $1 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting frameworks: %w", err)
	}

	query := `
		SELECT id, organization_id, name, version, description, authority, category,
			is_active, effective_date, created_at, updated_at
		FROM compliance_frameworks
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing frameworks: %w", err)
	}
	defer rows.Close()

	var frameworks []models.ComplianceFramework
	for rows.Next() {
		var fw models.ComplianceFramework
		if err := rows.Scan(
			&fw.ID,
			&fw.OrganizationID,
			&fw.Name,
			&fw.Version,
			&fw.Description,
			&fw.Authority,
			&fw.Category,
			&fw.IsActive,
			&fw.EffectiveDate,
			&fw.CreatedAt,
			&fw.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning framework row: %w", err)
		}
		frameworks = append(frameworks, fw)
	}
	return frameworks, total, rows.Err()
}

func (r *frameworkRepo) ListByCategory(ctx context.Context, orgID, category string) ([]models.ComplianceFramework, error) {
	// TODO: SELECT ... FROM compliance_frameworks
	//   WHERE organization_id=$1 AND category=$2 AND deleted_at IS NULL
	//   ORDER BY name ASC
	_ = ctx
	_ = orgID
	_ = category
	return nil, fmt.Errorf("not implemented")
}

func (r *frameworkRepo) GetWithControls(ctx context.Context, orgID, id string) (*models.ComplianceFramework, error) {
	// TODO: 1) Fetch framework via GetByID
	//       2) SELECT ... FROM controls WHERE framework_id=$1 AND organization_id=$2 AND deleted_at IS NULL
	//       3) Attach controls slice to framework
	_ = ctx
	_ = orgID
	_ = id
	return nil, fmt.Errorf("not implemented")
}
