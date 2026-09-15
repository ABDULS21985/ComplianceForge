package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

var ErrEvidenceReviewConflict = errors.New("evidence review request conflicts with a prior review")

type ControlRepository interface {
	ListByFramework(ctx context.Context, orgID, frameworkID string, pagination models.PaginationRequest) ([]models.Control, int, error)
	ListAdopted(ctx context.Context, orgID, frameworkID string, pagination models.PaginationRequest) ([]models.Control, int, error)
	GetAdoptedByID(ctx context.Context, orgID, controlID string) (*models.Control, error)
	UpdateImplementation(ctx context.Context, orgID, controlID string, patch models.ControlImplementationPatch) (*models.ControlImplementation, error)
	AttachEvidence(ctx context.Context, orgID, userID, controlID string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
	ListEvidence(ctx context.Context, orgID, controlID string, pagination models.PaginationRequest) ([]models.ControlEvidence, int, error)
	GetEvidence(ctx context.Context, orgID, controlID, evidenceID string) (*models.ControlEvidence, error)
	ReviewEvidence(ctx context.Context, orgID, userID, controlID, evidenceID string, input models.ReviewControlEvidenceInput) (*models.ControlEvidence, error)
	SupersedeEvidence(ctx context.Context, orgID, userID, controlID, evidenceID string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
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
	input.SupersedesEvidenceID = nil
	input.VersionReason = "Initial evidence version"
	var evidence *models.ControlEvidence
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if input.FileSizeBytes != nil && *input.FileSizeBytes > 0 {
			if err := EnsureEntitlementCapacity(ctx, tx, orgID, "storage_bytes", *input.FileSizeBytes); err != nil {
				return err
			}
		}
		var err error
		evidence, err = insertControlEvidence(ctx, tx, orgID, userID, controlID, input)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("attaching control evidence: %w", err)
	}
	return evidence, nil
}

func (r *controlRepo) SupersedeEvidence(
	ctx context.Context, orgID, userID, controlID, evidenceID string, input models.AttachControlEvidenceInput,
) (*models.ControlEvidence, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	input.SupersedesEvidenceID = &evidenceID
	var evidence *models.ControlEvidence
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		if input.FileSizeBytes != nil && *input.FileSizeBytes > 0 {
			if err := EnsureEntitlementCapacity(ctx, tx, orgID, "storage_bytes", *input.FileSizeBytes); err != nil {
				return err
			}
		}
		item, err := insertControlEvidence(ctx, tx, orgID, userID, controlID, input)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE control_evidence AS evidence
			SET lifecycle_status=CASE WHEN lifecycle_status='expired' THEN 'expired' ELSE 'superseded' END,
				is_current=FALSE,superseded_by_evidence_id=$4::uuid,
				superseded_at=statement_timestamp(),updated_at=statement_timestamp()
			FROM control_implementations AS implementation
			WHERE evidence.organization_id=$1::uuid AND evidence.id=$3::uuid
			  AND implementation.id=evidence.control_implementation_id
			  AND implementation.organization_id=$1::uuid
			  AND implementation.framework_control_id=$2::uuid
			  AND evidence.deleted_at IS NULL AND implementation.deleted_at IS NULL
			  AND evidence.superseded_by_evidence_id IS NULL`, orgID, controlID, evidenceID, item.ID)
		if err != nil {
			return fmt.Errorf("linking superseded evidence: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		tag, err = tx.Exec(ctx, `UPDATE control_evidence
			SET is_current=TRUE,updated_at=statement_timestamp()
			WHERE organization_id=$1::uuid AND id=$2::uuid
			  AND lifecycle_status='active' AND NOT is_current`, orgID, item.ID)
		if err != nil {
			return fmt.Errorf("activating replacement evidence: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return errors.New("replacement evidence could not be activated")
		}
		evidence, err = getLifecycleEvidence(ctx, tx, orgID, controlID, item.ID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("superseding control evidence: %w", err)
	}
	return evidence, nil
}

func insertControlEvidence(
	ctx context.Context, tx pgx.Tx, orgID, userID, controlID string, input models.AttachControlEvidenceInput,
) (*models.ControlEvidence, error) {
	method := input.CollectionMethod
	if method == "" {
		method = "manual_upload"
	}
	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	supersedes := ""
	if input.SupersedesEvidenceID != nil {
		supersedes = *input.SupersedesEvidenceID
	}
	versionReason := input.VersionReason
	if versionReason == "" {
		versionReason = "Initial evidence version"
	}
	return scanLifecycleEvidence(tx.QueryRow(ctx, `INSERT INTO control_evidence AS ce (
		organization_id,control_implementation_id,title,description,evidence_type,
		file_path,file_name,file_size_bytes,mime_type,file_hash,collection_method,
		collected_by,valid_from,valid_until,metadata,supersedes_evidence_id,version_reason)
	SELECT $1::uuid,implementation.id,$3,$4,$5,$6,$7,$8,$9,$10,$11,
		CASE WHEN EXISTS (
			SELECT 1 FROM users AS account WHERE account.id=$12::uuid
			  AND account.organization_id=$1::uuid AND account.deleted_at IS NULL
		) THEN $12::uuid ELSE NULL END,
		$13::date,$14::date,$15::jsonb,NULLIF($16::text,'')::uuid,$17
	FROM control_implementations AS implementation
	JOIN framework_controls AS control ON control.id=implementation.framework_control_id
	JOIN compliance_frameworks AS framework ON framework.id=control.framework_id
	WHERE implementation.framework_control_id=$2::uuid
	  AND implementation.organization_id=$1::uuid
	  AND implementation.deleted_at IS NULL AND framework.deleted_at IS NULL
	  AND (framework.organization_id IS NULL OR framework.organization_id=$1::uuid)
	RETURNING `+evidenceLifecycleColumns,
		orgID, controlID, input.Title, input.Description, input.EvidenceType,
		input.ObjectKey, input.FileName, input.FileSizeBytes, input.MIMEType,
		input.FileHash, method, userID, input.ValidFrom, input.ValidUntil,
		metadata, supersedes, versionReason))
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
	rows, err := q.Query(ctx, `SELECT `+evidenceLifecycleColumns+`
		FROM control_evidence ce JOIN control_implementations ci ON ci.id=ce.control_implementation_id
		WHERE `+filter+` ORDER BY ce.collected_at DESC LIMIT $3 OFFSET $4`, orgID, controlID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing control evidence: %w", err)
	}
	defer rows.Close()
	items := make([]models.ControlEvidence, 0)
	for rows.Next() {
		item, err := scanLifecycleEvidence(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *controlRepo) GetEvidence(ctx context.Context, orgID, controlID, evidenceID string) (*models.ControlEvidence, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	item, err := getLifecycleEvidence(ctx, q, orgID, controlID, evidenceID)
	if err != nil {
		return nil, fmt.Errorf("getting control evidence: %w", err)
	}
	return item, nil
}

func (r *controlRepo) ReviewEvidence(ctx context.Context, orgID, userID, controlID, evidenceID string, input models.ReviewControlEvidenceInput) (*models.ControlEvidence, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	requestID := strings.TrimSpace(input.RequestID)
	var evidence *models.ControlEvidence
	err := withTransaction(ctx, q, func(tx pgx.Tx) error {
		var reviewID string
		err := tx.QueryRow(ctx, `SELECT review_id FROM submit_evidence_review(
			$1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,NULLIF($7::text,''))`,
			orgID, controlID, evidenceID, userID, input.Status,
			input.Comment, requestID).Scan(&reviewID)
		if errors.Is(err, pgx.ErrNoRows) && requestID != "" {
			var sameRequest bool
			err = tx.QueryRow(ctx, `SELECT reviewer_id=$3::uuid
				AND decision=lower(BTRIM($4::text))
				AND comment IS NOT DISTINCT FROM NULLIF(BTRIM($5::text),'')
				FROM evidence_reviews
				WHERE organization_id=$1::uuid AND evidence_id=$2::uuid
				  AND request_id=$6`, orgID, evidenceID, userID, input.Status,
				input.Comment, requestID).Scan(&sameRequest)
			if err == nil && !sameRequest {
				return ErrEvidenceReviewConflict
			}
		}
		if err != nil {
			return err
		}
		evidence, err = getLifecycleEvidence(ctx, tx, orgID, controlID, evidenceID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("reviewing control evidence: %w", err)
	}
	return evidence, nil
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
