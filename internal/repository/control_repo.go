package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// ControlRepository defines data-access operations for controls.
type ControlRepository interface {
	Create(ctx context.Context, control *models.Control) error
	GetByID(ctx context.Context, orgID, id string) (*models.Control, error)
	Update(ctx context.Context, control *models.Control) error
	Delete(ctx context.Context, orgID, id string) error
	ListByFramework(ctx context.Context, orgID, frameworkID string, pagination models.PaginationRequest) ([]models.Control, int, error)
	ListByStatus(ctx context.Context, orgID string, status models.ComplianceStatus) ([]models.Control, error)
	UpdateComplianceStatus(ctx context.Context, orgID, id string, status models.ComplianceStatus) error
	BulkCreate(ctx context.Context, controls []models.Control) error
}

type controlRepo struct {
	pool *pgxpool.Pool
}

// NewControlRepository returns a concrete ControlRepository backed by pgxpool.
func NewControlRepository(pool *pgxpool.Pool) ControlRepository {
	return &controlRepo{pool: pool}
}

func (r *controlRepo) Create(ctx context.Context, c *models.Control) error {
	query := `
		INSERT INTO controls (id, organization_id, framework_id, code, title, description,
			category, guidance, compliance_status, priority, implementation_status,
			owner_id, evidence_required, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		c.OrganizationID,
		c.FrameworkID,
		c.Code,
		c.Title,
		c.Description,
		c.Category,
		c.Guidance,
		c.ComplianceStatus,
		c.Priority,
		c.ImplementationStatus,
		c.OwnerID,
		c.EvidenceRequired,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *controlRepo) GetByID(ctx context.Context, orgID, id string) (*models.Control, error) {
	query := `
		SELECT id, organization_id, framework_id, code, title, description,
			category, guidance, compliance_status, priority, implementation_status,
			owner_id, evidence_required, created_at, updated_at
		FROM controls
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	c := &models.Control{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&c.ID,
		&c.OrganizationID,
		&c.FrameworkID,
		&c.Code,
		&c.Title,
		&c.Description,
		&c.Category,
		&c.Guidance,
		&c.ComplianceStatus,
		&c.Priority,
		&c.ImplementationStatus,
		&c.OwnerID,
		&c.EvidenceRequired,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("control not found: %w", err)
	}
	return c, nil
}

func (r *controlRepo) Update(ctx context.Context, c *models.Control) error {
	// TODO: UPDATE controls SET code=$3, title=$4, description=$5, category=$6,
	//   guidance=$7, compliance_status=$8, priority=$9, implementation_status=$10,
	//   owner_id=$11, evidence_required=$12, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = c
	return fmt.Errorf("not implemented")
}

func (r *controlRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE controls SET deleted_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *controlRepo) ListByFramework(ctx context.Context, orgID, frameworkID string, pagination models.PaginationRequest) ([]models.Control, int, error) {
	countQuery := `
		SELECT COUNT(*) FROM controls
		WHERE organization_id = $1 AND framework_id = $2 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID, frameworkID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting controls: %w", err)
	}

	query := `
		SELECT id, organization_id, framework_id, code, title, description,
			category, guidance, compliance_status, priority, implementation_status,
			owner_id, evidence_required, created_at, updated_at
		FROM controls
		WHERE organization_id = $1 AND framework_id = $2 AND deleted_at IS NULL
		ORDER BY code ASC
		LIMIT $3 OFFSET $4`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, frameworkID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing controls: %w", err)
	}
	defer rows.Close()

	var controls []models.Control
	for rows.Next() {
		var c models.Control
		if err := rows.Scan(
			&c.ID,
			&c.OrganizationID,
			&c.FrameworkID,
			&c.Code,
			&c.Title,
			&c.Description,
			&c.Category,
			&c.Guidance,
			&c.ComplianceStatus,
			&c.Priority,
			&c.ImplementationStatus,
			&c.OwnerID,
			&c.EvidenceRequired,
			&c.CreatedAt,
			&c.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning control row: %w", err)
		}
		controls = append(controls, c)
	}
	return controls, total, rows.Err()
}

func (r *controlRepo) ListByStatus(ctx context.Context, orgID string, status models.ComplianceStatus) ([]models.Control, error) {
	// TODO: SELECT ... FROM controls
	//   WHERE organization_id=$1 AND compliance_status=$2 AND deleted_at IS NULL
	//   ORDER BY code ASC
	_ = ctx
	_ = orgID
	_ = status
	return nil, fmt.Errorf("not implemented")
}

func (r *controlRepo) UpdateComplianceStatus(ctx context.Context, orgID, id string, status models.ComplianceStatus) error {
	// TODO: UPDATE controls SET compliance_status=$3, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	_ = status
	return fmt.Errorf("not implemented")
}

func (r *controlRepo) BulkCreate(ctx context.Context, controls []models.Control) error {
	// TODO: Use pgx.Batch or COPY protocol for efficiency:
	//   batch := &pgx.Batch{}
	//   for _, c := range controls {
	//       batch.Queue("INSERT INTO controls (...) VALUES (...)", c.OrganizationID, ...)
	//   }
	//   br := r.pool.SendBatch(ctx, batch)
	//   defer br.Close()
	//   for range controls { br.Exec() }
	_ = ctx
	_ = controls
	return fmt.Errorf("not implemented")
}
