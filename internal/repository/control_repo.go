package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

type ControlRepository interface {
	ListByFramework(ctx context.Context, orgID, frameworkID string, pagination models.PaginationRequest) ([]models.Control, int, error)
	ListAdopted(ctx context.Context, orgID, frameworkID string, pagination models.PaginationRequest) ([]models.Control, int, error)
	GetAdoptedByID(ctx context.Context, orgID, controlID string) (*models.Control, error)
	UpdateImplementation(ctx context.Context, orgID, controlID string, patch models.ControlImplementationPatch) (*models.ControlImplementation, error)
	AttachEvidence(ctx context.Context, orgID, userID, controlID string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
	ListEvidence(ctx context.Context, orgID, controlID string, pagination models.PaginationRequest) ([]models.ControlEvidence, int, error)
}

type controlRepo struct{ pool *pgxpool.Pool }

func NewControlRepository(pool *pgxpool.Pool) ControlRepository { return &controlRepo{pool: pool} }

const controlColumns = `
	fc.id, fc.framework_id, fc.domain_id, fc.code, fc.title, fc.description,
	fc.guidance, fc.objective, fc.control_type, fc.implementation_type,
	fc.is_mandatory, fc.priority, fc.sort_order, fc.parent_control_id,
	fc.depth_level, fc.evidence_requirements, fc.test_procedures,
	fc."references", fc.keywords, fc.metadata, fc.created_at, fc.updated_at,
	ci.id, ci.organization_id, ci.org_framework_id, ci.status,
	ci.implementation_status, ci.maturity_level, ci.owner_user_id,
	ci.reviewer_user_id, ci.implementation_description, ci.implementation_notes,
	ci.gap_description, ci.remediation_plan, ci.remediation_due_date,
	ci.automation_level, ci.tags, ci.metadata, ci.created_at, ci.updated_at,
	ci.deleted_at`

func (r *controlRepo) ListByFramework(ctx context.Context, orgID, frameworkID string, p models.PaginationRequest) ([]models.Control, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	visible := `fc.framework_id = $2::uuid AND cf.deleted_at IS NULL
		AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM framework_controls fc
		JOIN compliance_frameworks cf ON cf.id = fc.framework_id WHERE `+visible, orgID, frameworkID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting framework controls: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+controlColumns+`
		FROM framework_controls fc
		JOIN compliance_frameworks cf ON cf.id = fc.framework_id
		LEFT JOIN control_implementations ci ON ci.framework_control_id = fc.id
		 AND ci.organization_id = $1::uuid AND ci.deleted_at IS NULL
		WHERE `+visible+`
		ORDER BY fc.sort_order, fc.code LIMIT $3 OFFSET $4`, orgID, frameworkID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing framework controls: %w", err)
	}
	defer rows.Close()
	return collectControls(rows, total)
}

func (r *controlRepo) ListAdopted(ctx context.Context, orgID, frameworkID string, p models.PaginationRequest) ([]models.Control, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	filter := `ci.organization_id = $1::uuid AND ci.deleted_at IS NULL
		AND ($2 = '' OR fc.framework_id = NULLIF($2, '')::uuid)
		AND cf.deleted_at IS NULL AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM framework_controls fc
		JOIN compliance_frameworks cf ON cf.id = fc.framework_id
		JOIN control_implementations ci ON ci.framework_control_id = fc.id
		WHERE `+filter, orgID, frameworkID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting adopted controls: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+controlColumns+`
		FROM framework_controls fc
		JOIN compliance_frameworks cf ON cf.id = fc.framework_id
		JOIN control_implementations ci ON ci.framework_control_id = fc.id
		WHERE `+filter+`
		ORDER BY cf.name, fc.sort_order, fc.code LIMIT $3 OFFSET $4`, orgID, frameworkID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing adopted controls: %w", err)
	}
	defer rows.Close()
	return collectControls(rows, total)
}

func (r *controlRepo) GetAdoptedByID(ctx context.Context, orgID, controlID string) (*models.Control, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	control, err := scanControl(q.QueryRow(ctx, `SELECT `+controlColumns+`
		FROM framework_controls fc
		JOIN compliance_frameworks cf ON cf.id = fc.framework_id
		JOIN control_implementations ci ON ci.framework_control_id = fc.id
		WHERE fc.id = $2::uuid AND ci.organization_id = $1::uuid
		  AND ci.deleted_at IS NULL AND cf.deleted_at IS NULL
		  AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)`, orgID, controlID))
	if err != nil {
		return nil, fmt.Errorf("getting adopted control: %w", err)
	}
	return control, nil
}

func (r *controlRepo) UpdateImplementation(ctx context.Context, orgID, controlID string, patch models.ControlImplementationPatch) (*models.ControlImplementation, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	status := stringValue(patch.Status)
	implementationStatus := stringValue(patch.ImplementationStatus)
	owner := pointerValue(patch.OwnerUserID)
	reviewer := pointerValue(patch.ReviewerUserID)
	automation := pointerValue(patch.AutomationLevel)
	tag, err := q.Exec(ctx, `UPDATE control_implementations ci SET
		status = CASE WHEN $3 THEN $4 ELSE status END,
		implementation_status = CASE WHEN $5 THEN $6 ELSE implementation_status END,
		maturity_level = CASE WHEN $7 THEN $8 ELSE maturity_level END,
		owner_user_id = CASE WHEN $9 THEN NULLIF($10, '')::uuid ELSE owner_user_id END,
		reviewer_user_id = CASE WHEN $11 THEN NULLIF($12, '')::uuid ELSE reviewer_user_id END,
		implementation_description = CASE WHEN $13 THEN $14 ELSE implementation_description END,
		implementation_notes = CASE WHEN $15 THEN $16 ELSE implementation_notes END,
		gap_description = CASE WHEN $17 THEN $18 ELSE gap_description END,
		remediation_plan = CASE WHEN $19 THEN $20 ELSE remediation_plan END,
		remediation_due_date = CASE WHEN $21 THEN $22::date ELSE remediation_due_date END,
		automation_level = CASE WHEN $23 THEN $24 ELSE automation_level END,
		tags = CASE WHEN $25 THEN $26::text[] ELSE tags END
	FROM framework_controls fc
	JOIN compliance_frameworks cf ON cf.id = fc.framework_id
	WHERE ci.framework_control_id = fc.id AND ci.framework_control_id = $2::uuid
	  AND ci.organization_id = $1::uuid AND ci.deleted_at IS NULL
	  AND cf.deleted_at IS NULL AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)
	  AND (NOT $9 OR $10 = '' OR EXISTS (SELECT 1 FROM users u WHERE u.id = $10::uuid AND u.organization_id = $1::uuid AND u.deleted_at IS NULL))
	  AND (NOT $11 OR $12 = '' OR EXISTS (SELECT 1 FROM users u WHERE u.id = $12::uuid AND u.organization_id = $1::uuid AND u.deleted_at IS NULL))`,
		orgID, controlID, patch.Status != nil, status,
		patch.ImplementationStatus != nil, implementationStatus,
		patch.MaturityLevel != nil, intValue(patch.MaturityLevel),
		patch.OwnerUserID != nil, owner, patch.ReviewerUserID != nil, reviewer,
		patch.ImplementationDescription != nil, pointerValue(patch.ImplementationDescription),
		patch.ImplementationNotes != nil, pointerValue(patch.ImplementationNotes),
		patch.GapDescription != nil, pointerValue(patch.GapDescription),
		patch.RemediationPlan != nil, pointerValue(patch.RemediationPlan),
		patch.RemediationDueDate != nil, timeValue(patch.RemediationDueDate),
		patch.AutomationLevel != nil, automation, patch.Tags != nil, sliceValue(patch.Tags))
	if err != nil {
		return nil, fmt.Errorf("updating control implementation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	control, err := r.GetAdoptedByID(ctx, orgID, controlID)
	if err != nil {
		return nil, err
	}
	return control.Implementation, nil
}

func (r *controlRepo) AttachEvidence(ctx context.Context, orgID, userID, controlID string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	method := input.CollectionMethod
	if method == "" {
		method = "manual_upload"
	}
	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	var evidence *models.ControlEvidence
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if input.FileSizeBytes != nil && *input.FileSizeBytes > 0 {
			if err := EnsureEntitlementCapacity(ctx, tx, orgID, "storage_bytes", *input.FileSizeBytes); err != nil {
				return err
			}
		}
		item := &models.ControlEvidence{}
		var raw []byte
		if err := tx.QueryRow(ctx, `INSERT INTO control_evidence (
			organization_id, control_implementation_id, title, description,
			evidence_type, file_name, file_size_bytes, mime_type, file_hash,
			collection_method, collected_by, valid_from, valid_until, metadata)
		SELECT $1::uuid, ci.id, $3, $4, $5, $6, $7, $8, $9, $10,
			CASE WHEN EXISTS (SELECT 1 FROM users u WHERE u.id=$11::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL) THEN $11::uuid ELSE NULL END,
			$12::date, $13::date, $14::jsonb
		FROM control_implementations ci
		JOIN framework_controls fc ON fc.id = ci.framework_control_id
		JOIN compliance_frameworks cf ON cf.id = fc.framework_id
		WHERE ci.framework_control_id = $2::uuid AND ci.organization_id = $1::uuid
		  AND ci.deleted_at IS NULL AND cf.deleted_at IS NULL
		  AND (cf.organization_id IS NULL OR cf.organization_id = $1::uuid)
		RETURNING id, organization_id, control_implementation_id, title, description,
			evidence_type, file_name, file_size_bytes, mime_type, file_hash,
			collection_method, collected_at, collected_by, valid_from, valid_until,
			is_current, review_status, metadata, created_at, updated_at, deleted_at`,
			orgID, controlID, input.Title, input.Description, input.EvidenceType,
			input.FileName, input.FileSizeBytes, input.MIMEType, input.FileHash, method,
			userID, input.ValidFrom, input.ValidUntil, metadata).Scan(
			&item.ID, &item.OrganizationID, &item.ControlImplementationID,
			&item.Title, &item.Description, &item.EvidenceType,
			&item.FileName, &item.FileSizeBytes, &item.MIMEType,
			&item.FileHash, &item.CollectionMethod, &item.CollectedAt,
			&item.CollectedBy, &item.ValidFrom, &item.ValidUntil,
			&item.IsCurrent, &item.ReviewStatus, &raw, &item.CreatedAt,
			&item.UpdatedAt, &item.DeletedAt); err != nil {
			return err
		}
		item.Metadata = raw
		evidence = item
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("attaching control evidence: %w", err)
	}
	return evidence, nil
}

func (r *controlRepo) ListEvidence(ctx context.Context, orgID, controlID string, p models.PaginationRequest) ([]models.ControlEvidence, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	filter := `ci.framework_control_id = $2::uuid AND ci.organization_id = $1::uuid
		AND ci.deleted_at IS NULL AND ce.deleted_at IS NULL`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM control_evidence ce
		JOIN control_implementations ci ON ci.id=ce.control_implementation_id WHERE `+filter, orgID, controlID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting control evidence: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT ce.id, ce.organization_id, ce.control_implementation_id,
		ce.title, ce.description, ce.evidence_type, ce.file_name, ce.file_size_bytes,
		ce.mime_type, ce.file_hash, ce.collection_method, ce.collected_at,
		ce.collected_by, ce.valid_from, ce.valid_until, ce.is_current,
		ce.review_status, ce.metadata, ce.created_at, ce.updated_at, ce.deleted_at
		FROM control_evidence ce JOIN control_implementations ci ON ci.id=ce.control_implementation_id
		WHERE `+filter+` ORDER BY ce.collected_at DESC LIMIT $3 OFFSET $4`, orgID, controlID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing control evidence: %w", err)
	}
	defer rows.Close()
	items := make([]models.ControlEvidence, 0)
	for rows.Next() {
		var item models.ControlEvidence
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.ControlImplementationID,
			&item.Title, &item.Description, &item.EvidenceType, &item.FileName,
			&item.FileSizeBytes, &item.MIMEType, &item.FileHash, &item.CollectionMethod,
			&item.CollectedAt, &item.CollectedBy, &item.ValidFrom, &item.ValidUntil,
			&item.IsCurrent, &item.ReviewStatus, &metadata, &item.CreatedAt,
			&item.UpdatedAt, &item.DeletedAt); err != nil {
			return nil, 0, err
		}
		item.Metadata = metadata
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func collectControls(rows pgx.Rows, total int) ([]models.Control, int, error) {
	items := make([]models.Control, 0)
	for rows.Next() {
		control, err := scanControl(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning control: %w", err)
		}
		items = append(items, *control)
	}
	return items, total, rows.Err()
}

func scanControl(row complianceRowScanner) (*models.Control, error) {
	control := &models.Control{}
	var priority, description, guidance *string
	var evidenceRequirements, testProcedures, references, metadata []byte
	var implID, implOrgID, orgFrameworkID, state, implementationStatus *string
	var maturity *int
	var owner, reviewer, implDescription, implNotes, gap, remediation, automation *string
	var due *time.Time
	var tags []string
	var implMetadata []byte
	var implCreated, implUpdated *time.Time
	var implDeleted *time.Time
	err := row.Scan(&control.ID, &control.FrameworkID, &control.DomainID, &control.Code,
		&control.Title, &description, &guidance, &control.Objective,
		&control.ControlType, &control.ImplementationType, &control.IsMandatory,
		&priority, &control.SortOrder, &control.ParentControlID, &control.DepthLevel,
		&evidenceRequirements, &testProcedures, &references, &control.Keywords,
		&metadata, &control.CreatedAt, &control.UpdatedAt, &implID, &implOrgID,
		&orgFrameworkID, &state, &implementationStatus, &maturity, &owner, &reviewer,
		&implDescription, &implNotes, &gap, &remediation, &due, &automation, &tags,
		&implMetadata, &implCreated, &implUpdated, &implDeleted)
	if err != nil {
		return nil, err
	}
	if description != nil {
		control.Description = *description
	}
	if guidance != nil {
		control.Guidance = *guidance
	}
	if priority != nil {
		control.Priority = models.ControlPriority(*priority)
	}
	control.EvidenceRequirements, control.TestProcedures = evidenceRequirements, testProcedures
	control.References, control.Metadata = references, metadata
	if implID != nil {
		control.Implementation = &models.ControlImplementation{
			BaseModel:      models.BaseModel{ID: *implID, CreatedAt: *implCreated, UpdatedAt: *implUpdated, DeletedAt: implDeleted},
			OrganizationID: *implOrgID, FrameworkControlID: control.ID,
			OrganizationFrameworkID: *orgFrameworkID,
			Status:                  models.ControlImplementationState(*state),
			ImplementationStatus:    models.ImplementationStatus(*implementationStatus),
			MaturityLevel:           *maturity, OwnerUserID: owner, ReviewerUserID: reviewer,
			ImplementationDescription: implDescription, ImplementationNotes: implNotes,
			GapDescription: gap, RemediationPlan: remediation, RemediationDueDate: due,
			AutomationLevel: automation, Tags: tags, Metadata: implMetadata,
		}
	}
	return control, nil
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
func timeValue(value *time.Time) *time.Time { return value }
func sliceValue(value *[]string) []string {
	if value == nil {
		return []string{}
	}
	return *value
}
func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
