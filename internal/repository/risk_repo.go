package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// RiskMatrixEntry holds aggregated risk counts grouped by likelihood and impact.
type RiskMatrixEntry struct {
	Likelihood int `json:"likelihood"`
	Impact     int `json:"impact"`
	Count      int `json:"count"`
}

// RiskRepository defines data-access operations for risks.
type RiskRepository interface {
	Create(ctx context.Context, risk *models.Risk) error
	GetByID(ctx context.Context, orgID, id string) (*models.Risk, error)
	Update(ctx context.Context, risk *models.Risk) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Risk, int, error)
	ListByRiskLevel(ctx context.Context, orgID string, level models.RiskLevel) ([]models.Risk, error)
	GetRiskMatrix(ctx context.Context, orgID string) ([]RiskMatrixEntry, error)
}

type riskRepo struct {
	pool *pgxpool.Pool
}

// NewRiskRepository returns a concrete RiskRepository backed by pgxpool.
func NewRiskRepository(pool *pgxpool.Pool) RiskRepository {
	return &riskRepo{pool: pool}
}

func (r *riskRepo) Create(ctx context.Context, risk *models.Risk) error {
	query := `
		INSERT INTO risks (id, organization_id, title, description, category, source,
			likelihood, impact, risk_score, risk_level, mitigation_strategy,
			mitigation_status, owner_id, related_control_id, review_date,
			created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		risk.OrganizationID,
		risk.Title,
		risk.Description,
		risk.Category,
		risk.Source,
		risk.Likelihood,
		risk.Impact,
		risk.RiskScore,
		risk.RiskLevel,
		risk.MitigationStrategy,
		risk.MitigationStatus,
		risk.OwnerID,
		risk.RelatedControlID,
		risk.ReviewDate,
	).Scan(&risk.ID, &risk.CreatedAt, &risk.UpdatedAt)
}

func (r *riskRepo) GetByID(ctx context.Context, orgID, id string) (*models.Risk, error) {
	query := `
		SELECT id, organization_id, title, description, category, source,
			likelihood, impact, risk_score, risk_level, mitigation_strategy,
			mitigation_status, owner_id, related_control_id, review_date,
			created_at, updated_at
		FROM risks
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	risk := &models.Risk{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&risk.ID,
		&risk.OrganizationID,
		&risk.Title,
		&risk.Description,
		&risk.Category,
		&risk.Source,
		&risk.Likelihood,
		&risk.Impact,
		&risk.RiskScore,
		&risk.RiskLevel,
		&risk.MitigationStrategy,
		&risk.MitigationStatus,
		&risk.OwnerID,
		&risk.RelatedControlID,
		&risk.ReviewDate,
		&risk.CreatedAt,
		&risk.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("risk not found: %w", err)
	}
	return risk, nil
}

func (r *riskRepo) Update(ctx context.Context, risk *models.Risk) error {
	// TODO: UPDATE risks SET title=$3, description=$4, category=$5, source=$6,
	//   likelihood=$7, impact=$8, risk_score=$9, risk_level=$10,
	//   mitigation_strategy=$11, mitigation_status=$12, owner_id=$13,
	//   related_control_id=$14, review_date=$15, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = risk
	return fmt.Errorf("not implemented")
}

func (r *riskRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE risks SET deleted_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *riskRepo) List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Risk, int, error) {
	countQuery := `SELECT COUNT(*) FROM risks WHERE organization_id = $1 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting risks: %w", err)
	}

	query := `
		SELECT id, organization_id, title, description, category, source,
			likelihood, impact, risk_score, risk_level, mitigation_strategy,
			mitigation_status, owner_id, related_control_id, review_date,
			created_at, updated_at
		FROM risks
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY risk_score DESC, created_at DESC
		LIMIT $2 OFFSET $3`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing risks: %w", err)
	}
	defer rows.Close()

	var risks []models.Risk
	for rows.Next() {
		var risk models.Risk
		if err := rows.Scan(
			&risk.ID,
			&risk.OrganizationID,
			&risk.Title,
			&risk.Description,
			&risk.Category,
			&risk.Source,
			&risk.Likelihood,
			&risk.Impact,
			&risk.RiskScore,
			&risk.RiskLevel,
			&risk.MitigationStrategy,
			&risk.MitigationStatus,
			&risk.OwnerID,
			&risk.RelatedControlID,
			&risk.ReviewDate,
			&risk.CreatedAt,
			&risk.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning risk row: %w", err)
		}
		risks = append(risks, risk)
	}
	return risks, total, rows.Err()
}

func (r *riskRepo) ListByRiskLevel(ctx context.Context, orgID string, level models.RiskLevel) ([]models.Risk, error) {
	// TODO: SELECT ... FROM risks
	//   WHERE organization_id=$1 AND risk_level=$2 AND deleted_at IS NULL
	//   ORDER BY risk_score DESC
	_ = ctx
	_ = orgID
	_ = level
	return nil, fmt.Errorf("not implemented")
}

func (r *riskRepo) GetRiskMatrix(ctx context.Context, orgID string) ([]RiskMatrixEntry, error) {
	query := `
		SELECT likelihood, impact, COUNT(*) as count
		FROM risks
		WHERE organization_id = $1 AND deleted_at IS NULL
		GROUP BY likelihood, impact
		ORDER BY likelihood, impact`

	rows, err := r.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying risk matrix: %w", err)
	}
	defer rows.Close()

	var entries []RiskMatrixEntry
	for rows.Next() {
		var e RiskMatrixEntry
		if err := rows.Scan(&e.Likelihood, &e.Impact, &e.Count); err != nil {
			return nil, fmt.Errorf("scanning risk matrix row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
