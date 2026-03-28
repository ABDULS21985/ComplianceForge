package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// PolicyRepository defines data-access operations for policies.
type PolicyRepository interface {
	Create(ctx context.Context, policy *models.Policy) error
	GetByID(ctx context.Context, orgID, id string) (*models.Policy, error)
	Update(ctx context.Context, policy *models.Policy) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Policy, int, error)
	ListByStatus(ctx context.Context, orgID string, status models.PolicyStatus) ([]models.Policy, error)
	ListDueForReview(ctx context.Context, orgID string, before time.Time) ([]models.Policy, error)
}

type policyRepo struct {
	pool *pgxpool.Pool
}

// NewPolicyRepository returns a concrete PolicyRepository backed by pgxpool.
func NewPolicyRepository(pool *pgxpool.Pool) PolicyRepository {
	return &policyRepo{pool: pool}
}

func (r *policyRepo) Create(ctx context.Context, p *models.Policy) error {
	query := `
		INSERT INTO policies (id, organization_id, title, version, content, category,
			status, owner_id, approver_id, approved_at, effective_date,
			review_date, related_framework_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		p.OrganizationID,
		p.Title,
		p.Version,
		p.Content,
		p.Category,
		p.Status,
		p.OwnerID,
		p.ApproverID,
		p.ApprovedAt,
		p.EffectiveDate,
		p.ReviewDate,
		p.RelatedFrameworkID,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *policyRepo) GetByID(ctx context.Context, orgID, id string) (*models.Policy, error) {
	query := `
		SELECT id, organization_id, title, version, content, category,
			status, owner_id, approver_id, approved_at, effective_date,
			review_date, related_framework_id, created_at, updated_at
		FROM policies
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	p := &models.Policy{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&p.ID,
		&p.OrganizationID,
		&p.Title,
		&p.Version,
		&p.Content,
		&p.Category,
		&p.Status,
		&p.OwnerID,
		&p.ApproverID,
		&p.ApprovedAt,
		&p.EffectiveDate,
		&p.ReviewDate,
		&p.RelatedFrameworkID,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("policy not found: %w", err)
	}
	return p, nil
}

func (r *policyRepo) Update(ctx context.Context, p *models.Policy) error {
	// TODO: UPDATE policies SET title=$3, version=$4, content=$5, category=$6,
	//   status=$7, owner_id=$8, approver_id=$9, approved_at=$10,
	//   effective_date=$11, review_date=$12, related_framework_id=$13, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = p
	return fmt.Errorf("not implemented")
}

func (r *policyRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE policies SET deleted_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *policyRepo) List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Policy, int, error) {
	countQuery := `SELECT COUNT(*) FROM policies WHERE organization_id = $1 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting policies: %w", err)
	}

	query := `
		SELECT id, organization_id, title, version, content, category,
			status, owner_id, approver_id, approved_at, effective_date,
			review_date, related_framework_id, created_at, updated_at
		FROM policies
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing policies: %w", err)
	}
	defer rows.Close()

	var policies []models.Policy
	for rows.Next() {
		var p models.Policy
		if err := rows.Scan(
			&p.ID,
			&p.OrganizationID,
			&p.Title,
			&p.Version,
			&p.Content,
			&p.Category,
			&p.Status,
			&p.OwnerID,
			&p.ApproverID,
			&p.ApprovedAt,
			&p.EffectiveDate,
			&p.ReviewDate,
			&p.RelatedFrameworkID,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning policy row: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, total, rows.Err()
}

func (r *policyRepo) ListByStatus(ctx context.Context, orgID string, status models.PolicyStatus) ([]models.Policy, error) {
	// TODO: SELECT ... FROM policies
	//   WHERE organization_id=$1 AND status=$2 AND deleted_at IS NULL
	//   ORDER BY title ASC
	_ = ctx
	_ = orgID
	_ = status
	return nil, fmt.Errorf("not implemented")
}

func (r *policyRepo) ListDueForReview(ctx context.Context, orgID string, before time.Time) ([]models.Policy, error) {
	// TODO: SELECT ... FROM policies
	//   WHERE organization_id=$1 AND review_date <= $2
	//     AND status NOT IN ('Archived', 'Retired') AND deleted_at IS NULL
	//   ORDER BY review_date ASC
	_ = ctx
	_ = orgID
	_ = before
	return nil, fmt.Errorf("not implemented")
}
