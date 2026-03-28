package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
)

// IncidentRepository defines data-access operations for security incidents.
type IncidentRepository interface {
	Create(ctx context.Context, incident *models.Incident) error
	GetByID(ctx context.Context, orgID, id string) (*models.Incident, error)
	Update(ctx context.Context, incident *models.Incident) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Incident, int, error)
	ListByStatus(ctx context.Context, orgID string, status models.IncidentStatus) ([]models.Incident, error)
	ListBreachNotifiable(ctx context.Context, orgID string) ([]models.Incident, error)
	UpdateStatus(ctx context.Context, orgID, id string, status models.IncidentStatus) error
}

type incidentRepo struct {
	pool *pgxpool.Pool
}

// NewIncidentRepository returns a concrete IncidentRepository backed by pgxpool.
func NewIncidentRepository(pool *pgxpool.Pool) IncidentRepository {
	return &incidentRepo{pool: pool}
}

func (r *incidentRepo) Create(ctx context.Context, inc *models.Incident) error {
	query := `
		INSERT INTO incidents (id, organization_id, title, description, severity, status,
			category, reporter_id, assignee_id, detected_at, contained_at, resolved_at,
			root_cause, impact, lessons_learned, is_breach_notifiable,
			notification_deadline, related_asset_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		inc.OrganizationID,
		inc.Title,
		inc.Description,
		inc.Severity,
		inc.Status,
		inc.Category,
		inc.ReporterID,
		inc.AssigneeID,
		inc.DetectedAt,
		inc.ContainedAt,
		inc.ResolvedAt,
		inc.RootCause,
		inc.Impact,
		inc.LessonsLearned,
		inc.IsBreachNotifiable,
		inc.NotificationDeadline,
		inc.RelatedAssetID,
	).Scan(&inc.ID, &inc.CreatedAt, &inc.UpdatedAt)
}

func (r *incidentRepo) GetByID(ctx context.Context, orgID, id string) (*models.Incident, error) {
	query := `
		SELECT id, organization_id, title, description, severity, status,
			category, reporter_id, assignee_id, detected_at, contained_at, resolved_at,
			root_cause, impact, lessons_learned, is_breach_notifiable,
			notification_deadline, related_asset_id, created_at, updated_at
		FROM incidents
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	inc := &models.Incident{}
	err := r.pool.QueryRow(ctx, query, id, orgID).Scan(
		&inc.ID,
		&inc.OrganizationID,
		&inc.Title,
		&inc.Description,
		&inc.Severity,
		&inc.Status,
		&inc.Category,
		&inc.ReporterID,
		&inc.AssigneeID,
		&inc.DetectedAt,
		&inc.ContainedAt,
		&inc.ResolvedAt,
		&inc.RootCause,
		&inc.Impact,
		&inc.LessonsLearned,
		&inc.IsBreachNotifiable,
		&inc.NotificationDeadline,
		&inc.RelatedAssetID,
		&inc.CreatedAt,
		&inc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("incident not found: %w", err)
	}
	return inc, nil
}

func (r *incidentRepo) Update(ctx context.Context, inc *models.Incident) error {
	// TODO: UPDATE incidents SET title=$3, description=$4, severity=$5, status=$6,
	//   category=$7, reporter_id=$8, assignee_id=$9, detected_at=$10,
	//   contained_at=$11, resolved_at=$12, root_cause=$13, impact=$14,
	//   lessons_learned=$15, is_breach_notifiable=$16, notification_deadline=$17,
	//   related_asset_id=$18, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = inc
	return fmt.Errorf("not implemented")
}

func (r *incidentRepo) Delete(ctx context.Context, orgID, id string) error {
	// TODO: UPDATE incidents SET deleted_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	return fmt.Errorf("not implemented")
}

func (r *incidentRepo) List(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]models.Incident, int, error) {
	countQuery := `SELECT COUNT(*) FROM incidents WHERE organization_id = $1 AND deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting incidents: %w", err)
	}

	query := `
		SELECT id, organization_id, title, description, severity, status,
			category, reporter_id, assignee_id, detected_at, contained_at, resolved_at,
			root_cause, impact, lessons_learned, is_breach_notifiable,
			notification_deadline, related_asset_id, created_at, updated_at
		FROM incidents
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	offset := (pagination.Page - 1) * pagination.PageSize
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing incidents: %w", err)
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		var inc models.Incident
		if err := rows.Scan(
			&inc.ID,
			&inc.OrganizationID,
			&inc.Title,
			&inc.Description,
			&inc.Severity,
			&inc.Status,
			&inc.Category,
			&inc.ReporterID,
			&inc.AssigneeID,
			&inc.DetectedAt,
			&inc.ContainedAt,
			&inc.ResolvedAt,
			&inc.RootCause,
			&inc.Impact,
			&inc.LessonsLearned,
			&inc.IsBreachNotifiable,
			&inc.NotificationDeadline,
			&inc.RelatedAssetID,
			&inc.CreatedAt,
			&inc.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning incident row: %w", err)
		}
		incidents = append(incidents, inc)
	}
	return incidents, total, rows.Err()
}

func (r *incidentRepo) ListByStatus(ctx context.Context, orgID string, status models.IncidentStatus) ([]models.Incident, error) {
	// TODO: SELECT ... FROM incidents
	//   WHERE organization_id=$1 AND status=$2 AND deleted_at IS NULL
	//   ORDER BY severity DESC, created_at DESC
	_ = ctx
	_ = orgID
	_ = status
	return nil, fmt.Errorf("not implemented")
}

func (r *incidentRepo) ListBreachNotifiable(ctx context.Context, orgID string) ([]models.Incident, error) {
	// TODO: SELECT ... FROM incidents
	//   WHERE organization_id=$1 AND is_breach_notifiable=true AND deleted_at IS NULL
	//   ORDER BY detected_at DESC
	_ = ctx
	_ = orgID
	return nil, fmt.Errorf("not implemented")
}

func (r *incidentRepo) UpdateStatus(ctx context.Context, orgID, id string, status models.IncidentStatus) error {
	// TODO: UPDATE incidents SET status=$3, updated_at=NOW()
	//   WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL
	_ = ctx
	_ = orgID
	_ = id
	_ = status
	return fmt.Errorf("not implemented")
}
