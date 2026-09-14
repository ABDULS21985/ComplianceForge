package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

// FrameworkRepository exposes only catalog reads and tenant adoption. Catalog
// mutation is intentionally not part of the tenant API.
type FrameworkRepository interface {
	List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.ComplianceFramework, int, error)
	GetByID(ctx context.Context, orgID, frameworkID string) (*models.ComplianceFramework, error)
	Adopt(ctx context.Context, orgID, userID, frameworkID string) (*models.OrganizationFramework, error)
}

type frameworkRepo struct{ pool *pgxpool.Pool }

func NewFrameworkRepository(pool *pgxpool.Pool) FrameworkRepository {
	return &frameworkRepo{pool: pool}
}

const frameworkColumns = `
	cf.id, cf.organization_id, cf.code, cf.name, cf.full_name, cf.version,
	cf.description, cf.issuing_body, cf.category, cf.applicable_regions,
	cf.applicable_industries, cf.is_system_framework, cf.is_active,
	cf.effective_date, cf.sunset_date, cf.total_controls, cf.icon_url,
	cf.color_hex, cf.metadata, cf.created_at, cf.updated_at, cf.deleted_at,
	of.id, of.status, of.adoption_date, of.target_completion_date,
	of.compliance_score, of.responsible_user_id, of.created_at, of.updated_at`

func (r *frameworkRepo) List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.ComplianceFramework, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := q.QueryRow(ctx, `
		SELECT count(*)
		FROM compliance_frameworks cf
		WHERE cf.deleted_at IS NULL AND cf.is_active
		  AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)`, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting visible frameworks: %w", err)
	}

	rows, err := q.Query(ctx, `SELECT `+frameworkColumns+`
		FROM compliance_frameworks cf
		LEFT JOIN organization_frameworks of
		  ON of.framework_id = cf.id AND of.organization_id = $1::uuid
		WHERE cf.deleted_at IS NULL AND cf.is_active
		  AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)
		ORDER BY cf.name, cf.version DESC
		LIMIT $2 OFFSET $3`, orgID, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing visible frameworks: %w", err)
	}
	defer rows.Close()

	frameworks := make([]models.ComplianceFramework, 0)
	for rows.Next() {
		framework, err := scanFramework(rows, orgID)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning framework: %w", err)
		}
		frameworks = append(frameworks, *framework)
	}
	return frameworks, total, rows.Err()
}

func (r *frameworkRepo) GetByID(ctx context.Context, orgID, frameworkID string) (*models.ComplianceFramework, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	row := q.QueryRow(ctx, `SELECT `+frameworkColumns+`
		FROM compliance_frameworks cf
		LEFT JOIN organization_frameworks of
		  ON of.framework_id = cf.id AND of.organization_id = $1::uuid
		WHERE cf.id = $2::uuid AND cf.deleted_at IS NULL
		  AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)`, orgID, frameworkID)
	framework, err := scanFramework(row, orgID)
	if err != nil {
		return nil, fmt.Errorf("getting visible framework: %w", err)
	}
	return framework, nil
}

type transactionStarter interface {
	Begin(context.Context) (pgx.Tx, error)
}

func (r *frameworkRepo) Adopt(ctx context.Context, orgID, userID, frameworkID string) (*models.OrganizationFramework, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(transactionStarter)
	if !ok {
		return nil, errors.New("framework adoption requires a transactional database executor")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning framework adoption: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockEntitlementCapacity(ctx, tx, orgID, "frameworks"); err != nil {
		return nil, err
	}
	var alreadyAdopted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_frameworks
		WHERE organization_id=$1::uuid AND framework_id=$2::uuid)`, orgID, frameworkID).Scan(&alreadyAdopted); err != nil {
		return nil, fmt.Errorf("checking existing framework adoption: %w", err)
	}
	if !alreadyAdopted {
		if err := ensureEntitlementCapacityLocked(ctx, tx, orgID, "frameworks", 1); err != nil {
			return nil, err
		}
	}

	var adoption models.OrganizationFramework
	err = tx.QueryRow(ctx, `
		INSERT INTO organization_frameworks (
			organization_id, framework_id, status, adoption_date, responsible_user_id
		)
		SELECT $1::uuid, cf.id, 'not_started', CURRENT_DATE,
			CASE WHEN EXISTS (
				SELECT 1 FROM users u
				WHERE u.id = $2::uuid AND u.organization_id = $1::uuid AND u.deleted_at IS NULL
			) THEN $2::uuid ELSE NULL END
		FROM compliance_frameworks cf
		WHERE cf.id = $3::uuid AND cf.deleted_at IS NULL AND cf.is_active
		  AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)
		ON CONFLICT (organization_id, framework_id) DO UPDATE
		SET responsible_user_id = COALESCE(organization_frameworks.responsible_user_id, EXCLUDED.responsible_user_id)
		RETURNING id, organization_id, framework_id, status, adoption_date,
			target_completion_date, compliance_score, responsible_user_id,
			created_at, updated_at`, orgID, userID, frameworkID).Scan(
		&adoption.ID, &adoption.OrganizationID, &adoption.FrameworkID,
		&adoption.Status, &adoption.AdoptionDate, &adoption.TargetCompletionDate,
		&adoption.ComplianceScore, &adoption.ResponsibleUserID,
		&adoption.CreatedAt, &adoption.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("adopting visible framework: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO control_implementations (
			organization_id, framework_control_id, org_framework_id,
			status, implementation_status, maturity_level
		)
		SELECT $1::uuid, fc.id, $2::uuid, 'not_implemented', 'not_started', 0
		FROM framework_controls fc
		JOIN compliance_frameworks cf ON cf.id = fc.framework_id
		WHERE fc.framework_id = $3::uuid AND cf.deleted_at IS NULL
		  AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)
		ON CONFLICT (organization_id, framework_control_id) DO NOTHING`, orgID, adoption.ID, frameworkID); err != nil {
		return nil, fmt.Errorf("seeding control implementations: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing framework adoption: %w", err)
	}
	return &adoption, nil
}

type complianceRowScanner interface{ Scan(...any) error }

func scanFramework(row complianceRowScanner, orgID string) (*models.ComplianceFramework, error) {
	framework := &models.ComplianceFramework{}
	var metadata []byte
	var adoptionID, adoptionStatus *string
	var adoptionDate, targetDate *time.Time
	var complianceScore *float64
	var responsibleUserID *string
	var adoptionCreatedAt, adoptionUpdatedAt *time.Time
	if err := row.Scan(
		&framework.ID, &framework.OrganizationID, &framework.Code, &framework.Name,
		&framework.FullName, &framework.Version, &framework.Description,
		&framework.IssuingBody, &framework.Category, &framework.ApplicableRegions,
		&framework.ApplicableIndustries, &framework.IsSystemFramework,
		&framework.IsActive, &framework.EffectiveDate, &framework.SunsetDate,
		&framework.TotalControls, &framework.IconURL, &framework.ColorHex,
		&metadata, &framework.CreatedAt, &framework.UpdatedAt, &framework.DeletedAt,
		&adoptionID, &adoptionStatus, &adoptionDate, &targetDate, &complianceScore,
		&responsibleUserID, &adoptionCreatedAt, &adoptionUpdatedAt,
	); err != nil {
		return nil, err
	}
	framework.Metadata = metadata
	if adoptionID != nil {
		framework.Adoption = &models.OrganizationFramework{
			BaseModel:      models.BaseModel{ID: *adoptionID, CreatedAt: *adoptionCreatedAt, UpdatedAt: *adoptionUpdatedAt},
			OrganizationID: orgID, FrameworkID: framework.ID, Status: *adoptionStatus,
			AdoptionDate: adoptionDate, TargetCompletionDate: targetDate,
			ComplianceScore: *complianceScore, ResponsibleUserID: responsibleUserID,
		}
	}
	return framework, nil
}
