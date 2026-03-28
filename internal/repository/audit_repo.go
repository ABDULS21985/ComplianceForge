package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// AuditRepository defines data-access operations for audits and their findings.
type AuditRepository interface {
	Create(ctx context.Context, audit *models.Audit) error
	GetByID(ctx context.Context, orgID, id string) (*models.Audit, error)
	Update(ctx context.Context, audit *models.Audit) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Audit, int, error)
	CreateFinding(ctx context.Context, finding *models.AuditFinding) error
	GetFindingByID(ctx context.Context, orgID, id string) (*models.AuditFinding, error)
	ListFindings(ctx context.Context, orgID, auditID string, pagination models.PaginationRequest) ([]models.AuditFinding, int, error)
	UpdateFindingStatus(ctx context.Context, orgID, id string, status models.FindingStatus) error
}

type auditRepo struct {
	pool *pgxpool.Pool
}

// NewAuditRepository returns a concrete AuditRepository backed by pgxpool.
func NewAuditRepository(pool *pgxpool.Pool) AuditRepository {
	return &auditRepo{pool: pool}
}

func (r *auditRepo) Create(ctx context.Context, a *models.Audit) error {
	query := `
		INSERT INTO audits (id, organization_id, title, description, type, status,
			lead_auditor_id, scope, scheduled_start_date, scheduled_end_date,
			actual_start_date, actual_end_date, framework_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		a.OrganizationID,
		a.Title,
		a.Description,
		a.Type,
		a.Status,
		a.LeadAuditorID,
		a.Scope,
		a.ScheduledStartDate,
		a.ScheduledEndDate,
		a.ActualStartDate,
		a.ActualEndDate,
		a.FrameworkID,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *auditRepo) GetByID(ctx context.Context, orgID, id string) (*models.Audit, error) {
	query := `
		SELECT id, organization_id, title, description, type, status,
			lead_auditor_id, scope, scheduled_start_date, scheduled_end_date,
			actual_start_date, actual_end_date, framework_id,
			created_at, updated_at
		FROM audits
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	a := &models.Audit{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&a.ID,
		&a.OrganizationID,
		&a.Title,
		&a.Description,
		&a.Type,
		&a.Status,
		&a.LeadAuditorID,
		&a.Scope,
		&a.ScheduledStartDate,
		&a.ScheduledEndDate,
		&a.ActualStartDate,
		&a.ActualEndDate,
		&a.FrameworkID,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("audit not found: %w", err)
	}
	return a, nil
}

func (r *auditRepo) Update(ctx context.Context, a *models.Audit) error {
	// TODO: UPDATE audits SET title=$3, description=$4, type=$5, status=$6,
	//   lead_auditor_id=$7, scope=$8, scheduled_start_date=$9, scheduled_end_date=$10,
	//   actual_start_date=$11, actual_end_date=$12, framework_id=$13, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = a
	return fmt.Errorf("not implemented")
}

func (r *auditRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE audits SET deleted_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *auditRepo) List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Audit, int, error) {
	countQuery := `SELECT COUNT(*) FROM audits WHERE organization_id = $1 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audits: %w", err)
	}

	query := `
		SELECT id, organization_id, title, description, type, status,
			lead_auditor_id, scope, scheduled_start_date, scheduled_end_date,
			actual_start_date, actual_end_date, framework_id,
			created_at, updated_at
		FROM audits
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audits: %w", err)
	}
	defer rows.Close()

	var audits []models.Audit
	for rows.Next() {
		var a models.Audit
		if err := rows.Scan(
			&a.ID,
			&a.OrganizationID,
			&a.Title,
			&a.Description,
			&a.Type,
			&a.Status,
			&a.LeadAuditorID,
			&a.Scope,
			&a.ScheduledStartDate,
			&a.ScheduledEndDate,
			&a.ActualStartDate,
			&a.ActualEndDate,
			&a.FrameworkID,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning audit row: %w", err)
		}
		audits = append(audits, a)
	}
	return audits, total, rows.Err()
}

func (r *auditRepo) CreateFinding(ctx context.Context, f *models.AuditFinding) error {
	query := `
		INSERT INTO audit_findings (id, organization_id, audit_id, control_id, title,
			description, severity, status, remediation_plan, owner_id,
			due_date, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		f.OrganizationID,
		f.AuditID,
		f.ControlID,
		f.Title,
		f.Description,
		f.Severity,
		f.Status,
		f.RemediationPlan,
		f.OwnerID,
		f.DueDate,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

func (r *auditRepo) GetFindingByID(ctx context.Context, orgID, id string) (*models.AuditFinding, error) {
	// TODO: SELECT ... FROM audit_findings
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return nil, fmt.Errorf("not implemented")
}

func (r *auditRepo) ListFindings(ctx context.Context, orgID, auditID string, pagination models.PaginationRequest) ([]models.AuditFinding, int, error) {
	// TODO: SELECT COUNT(*) FROM audit_findings WHERE organization_id=$1 AND audit_id=$2 AND deleted_at IS NULL
	// TODO: SELECT ... FROM audit_findings
	//   WHERE organization_id=$1 AND audit_id=$2 AND deleted_at IS NULL
	//   ORDER BY severity DESC, created_at DESC
	//   LIMIT $3 OFFSET $4
	_ = ctx
	_ = orgID
	_ = auditID
	_ = pagination
	return nil, 0, fmt.Errorf("not implemented")
}

func (r *auditRepo) UpdateFindingStatus(ctx context.Context, orgID, id string, status models.FindingStatus) error {
	// TODO: UPDATE audit_findings SET status=$3, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	_ = status
	return fmt.Errorf("not implemented")
}
