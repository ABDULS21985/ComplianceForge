package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// VendorRepository defines data-access operations for third-party vendors.
type VendorRepository interface {
	Create(ctx context.Context, vendor *models.Vendor) error
	GetByID(ctx context.Context, orgID, id string) (*models.Vendor, error)
	Update(ctx context.Context, vendor *models.Vendor) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Vendor, int, error)
	ListByRiskLevel(ctx context.Context, orgID string, level models.RiskLevel) ([]models.Vendor, error)
	ListDueForAssessment(ctx context.Context, orgID string, before time.Time) ([]models.Vendor, error)
}

type vendorRepo struct {
	pool *pgxpool.Pool
}

// NewVendorRepository returns a concrete VendorRepository backed by pgxpool.
func NewVendorRepository(pool *pgxpool.Pool) VendorRepository {
	return &vendorRepo{pool: pool}
}

func (r *vendorRepo) Create(ctx context.Context, v *models.Vendor) error {
	query := `
		INSERT INTO vendors (id, organization_id, name, description, contact_name,
			contact_email, contact_phone, website, category, risk_level, status,
			contract_start_date, contract_end_date, last_assessment_date,
			next_assessment_date, data_processing_agreement, sub_processors,
			created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		v.OrganizationID,
		v.Name,
		v.Description,
		v.ContactName,
		v.ContactEmail,
		v.ContactPhone,
		v.Website,
		v.Category,
		v.RiskLevel,
		v.Status,
		v.ContractStartDate,
		v.ContractEndDate,
		v.LastAssessmentDate,
		v.NextAssessmentDate,
		v.DataProcessingAgreement,
		v.SubProcessors,
	).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
}

func (r *vendorRepo) GetByID(ctx context.Context, orgID, id string) (*models.Vendor, error) {
	query := `
		SELECT id, organization_id, name, description, contact_name,
			contact_email, contact_phone, website, category, risk_level, status,
			contract_start_date, contract_end_date, last_assessment_date,
			next_assessment_date, data_processing_agreement, sub_processors,
			created_at, updated_at
		FROM vendors
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	v := &models.Vendor{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&v.ID,
		&v.OrganizationID,
		&v.Name,
		&v.Description,
		&v.ContactName,
		&v.ContactEmail,
		&v.ContactPhone,
		&v.Website,
		&v.Category,
		&v.RiskLevel,
		&v.Status,
		&v.ContractStartDate,
		&v.ContractEndDate,
		&v.LastAssessmentDate,
		&v.NextAssessmentDate,
		&v.DataProcessingAgreement,
		&v.SubProcessors,
		&v.CreatedAt,
		&v.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("vendor not found: %w", err)
	}
	return v, nil
}

func (r *vendorRepo) Update(ctx context.Context, v *models.Vendor) error {
	// TODO: UPDATE vendors SET name=$3, description=$4, contact_name=$5,
	//   contact_email=$6, contact_phone=$7, website=$8, category=$9,
	//   risk_level=$10, status=$11, contract_start_date=$12, contract_end_date=$13,
	//   last_assessment_date=$14, next_assessment_date=$15,
	//   data_processing_agreement=$16, sub_processors=$17, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = v
	return fmt.Errorf("not implemented")
}

func (r *vendorRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE vendors SET deleted_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *vendorRepo) List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Vendor, int, error) {
	countQuery := `SELECT COUNT(*) FROM vendors WHERE organization_id = $1 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting vendors: %w", err)
	}

	query := `
		SELECT id, organization_id, name, description, contact_name,
			contact_email, contact_phone, website, category, risk_level, status,
			contract_start_date, contract_end_date, last_assessment_date,
			next_assessment_date, data_processing_agreement, sub_processors,
			created_at, updated_at
		FROM vendors
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing vendors: %w", err)
	}
	defer rows.Close()

	var vendors []models.Vendor
	for rows.Next() {
		var v models.Vendor
		if err := rows.Scan(
			&v.ID,
			&v.OrganizationID,
			&v.Name,
			&v.Description,
			&v.ContactName,
			&v.ContactEmail,
			&v.ContactPhone,
			&v.Website,
			&v.Category,
			&v.RiskLevel,
			&v.Status,
			&v.ContractStartDate,
			&v.ContractEndDate,
			&v.LastAssessmentDate,
			&v.NextAssessmentDate,
			&v.DataProcessingAgreement,
			&v.SubProcessors,
			&v.CreatedAt,
			&v.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning vendor row: %w", err)
		}
		vendors = append(vendors, v)
	}
	return vendors, total, rows.Err()
}

func (r *vendorRepo) ListByRiskLevel(ctx context.Context, orgID string, level models.RiskLevel) ([]models.Vendor, error) {
	// TODO: SELECT ... FROM vendors
	//   WHERE organization_id=$1 AND risk_level=$2 AND deleted_at IS NULL
	//   ORDER BY name ASC
	_ = ctx
	_ = orgID
	_ = level
	return nil, fmt.Errorf("not implemented")
}

func (r *vendorRepo) ListDueForAssessment(ctx context.Context, orgID string, before time.Time) ([]models.Vendor, error) {
	// TODO: SELECT ... FROM vendors
	//   WHERE organization_id=$1 AND next_assessment_date <= $2 AND deleted_at IS NULL
	//   ORDER BY next_assessment_date ASC
	_ = ctx
	_ = orgID
	_ = before
	return nil, fmt.Errorf("not implemented")
}
