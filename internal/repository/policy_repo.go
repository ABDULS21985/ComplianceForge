package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

type PolicyRepository interface {
	Create(context.Context, string, string, models.PolicyCreateInput, int) (*models.Policy, error)
	GetByID(context.Context, string, string) (*models.Policy, error)
	Update(context.Context, string, *models.Policy) (*models.Policy, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.PolicyListFilter) ([]models.Policy, int, error)
	ListCategories(context.Context, string) ([]models.PolicyCategory, error)
	CreateVersion(context.Context, string, string, string, models.PolicyVersionInput, int) (*models.PolicyVersion, error)
	GetVersion(context.Context, string, string, string) (*models.PolicyVersion, error)
	ListVersions(context.Context, string, string, models.PaginationRequest) ([]models.PolicyVersion, int, error)
	CreateApprovalWorkflow(context.Context, string, string, string, models.PolicySubmitInput) (*models.PolicyApprovalWorkflow, error)
	GetActiveApprovalWorkflow(context.Context, string, string) (*models.PolicyApprovalWorkflow, error)
	DecideApproval(context.Context, string, string, string, string, models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error)
	Publish(context.Context, string, string, string) (*models.Policy, error)
	CreateReview(context.Context, string, string, models.PolicyReviewInput) (*models.PolicyReview, error)
	GetReview(context.Context, string, string, string) (*models.PolicyReview, error)
	UpdateReview(context.Context, string, string, *models.PolicyReview) (*models.PolicyReview, error)
	ListReviews(context.Context, string, string, models.PaginationRequest) ([]models.PolicyReview, int, error)
	Acknowledge(context.Context, string, string, string, string, models.PolicyAttestationInput) (*models.PolicyAttestation, error)
	ListAttestations(context.Context, string, string, models.PaginationRequest) ([]models.PolicyAttestation, int, error)
	CreateException(context.Context, string, string, string, models.PolicyExceptionInput) (*models.PolicyException, error)
	GetException(context.Context, string, string, string) (*models.PolicyException, error)
	UpdateExceptionDecision(context.Context, string, string, string, string, models.PolicyExceptionDecisionInput) (*models.PolicyException, error)
	ListExceptions(context.Context, string, string, models.PaginationRequest) ([]models.PolicyException, int, error)
}

type policyRepo struct{ pool *pgxpool.Pool }

func NewPolicyRepository(pool *pgxpool.Pool) PolicyRepository { return &policyRepo{pool: pool} }

var _ PolicyRepository = (*policyRepo)(nil)

type policyRowScanner interface{ Scan(...any) error }
type policyTransactionStarter interface {
	Begin(context.Context) (pgx.Tx, error)
}

const policyColumns = `p.id, p.organization_id, p.policy_ref, p.title, p.category_id,
	p.status, p.classification, p.owner_user_id, p.author_user_id, p.approver_user_id,
	p.department_id, p.current_version, p.current_version_id, p.review_frequency_months,
	p.last_review_date, p.next_review_date, p.review_status, p.applies_to_all,
	p.applicable_departments, p.applicable_roles, p.applicable_locations,
	p.linked_framework_ids, p.linked_control_ids, p.linked_risk_ids,
	p.parent_policy_id, p.supersedes_policy_id, p.effective_date, p.expiry_date,
	p.tags, p.priority, p.is_mandatory, p.requires_attestation,
	p.attestation_frequency_months, p.metadata, p.created_at, p.updated_at, p.deleted_at`

func scanPolicy(row policyRowScanner) (*models.Policy, error) {
	p := &models.Policy{}
	var metadata []byte
	if err := row.Scan(&p.ID, &p.OrganizationID, &p.PolicyRef, &p.Title, &p.CategoryID,
		&p.Status, &p.Classification, &p.OwnerUserID, &p.AuthorUserID, &p.ApproverUserID,
		&p.DepartmentID, &p.CurrentVersion, &p.CurrentVersionID, &p.ReviewFrequencyMonths,
		&p.LastReviewDate, &p.NextReviewDate, &p.ReviewStatus, &p.AppliesToAll,
		&p.ApplicableDepartments, &p.ApplicableRoles, &p.ApplicableLocations,
		&p.LinkedFrameworkIDs, &p.LinkedControlIDs, &p.LinkedRiskIDs,
		&p.ParentPolicyID, &p.SupersedesPolicyID, &p.EffectiveDate, &p.ExpiryDate,
		&p.Tags, &p.Priority, &p.IsMandatory, &p.RequiresAttestation,
		&p.AttestationFrequencyMonths, &metadata, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
		return nil, err
	}
	p.Metadata = metadata
	return p, nil
}

const policyVersionColumns = `v.id, v.policy_id, v.organization_id, v.version_number,
	v.version_label, v.title, v.content_html, v.content_text, v.summary,
	v.change_description, v.change_type, v.language, v.word_count, v.status,
	v.created_by, v.published_at, v.published_by, v.file_path, v.file_hash,
	v.metadata, v.created_at, v.updated_at`

func scanPolicyVersion(row policyRowScanner) (*models.PolicyVersion, error) {
	v := &models.PolicyVersion{}
	var metadata []byte
	if err := row.Scan(&v.ID, &v.PolicyID, &v.OrganizationID, &v.VersionNumber,
		&v.VersionLabel, &v.Title, &v.ContentHTML, &v.ContentText, &v.Summary,
		&v.ChangeDescription, &v.ChangeType, &v.Language, &v.WordCount, &v.Status,
		&v.CreatedBy, &v.PublishedAt, &v.PublishedBy, &v.FilePath, &v.FileHash,
		&metadata, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Metadata = metadata
	return v, nil
}

func policyReferenceChecks() string {
	return `
		AND ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM policy_categories c WHERE c.id=$4::uuid AND (c.organization_id IS NULL OR c.organization_id=$1::uuid)))
		AND ($6::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$6::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		AND ($7::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$7::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		AND ($18::uuid IS NULL OR EXISTS (SELECT 1 FROM policies x WHERE x.id=$18::uuid AND x.organization_id=$1::uuid AND x.deleted_at IS NULL))
		AND ($19::uuid IS NULL OR EXISTS (SELECT 1 FROM policies x WHERE x.id=$19::uuid AND x.organization_id=$1::uuid AND x.deleted_at IS NULL))
		AND NOT EXISTS (SELECT 1 FROM unnest($15::uuid[]) AS linked(id) WHERE NOT EXISTS (
			SELECT 1 FROM compliance_frameworks f WHERE f.id=linked.id AND f.deleted_at IS NULL
			AND (f.organization_id IS NULL OR f.organization_id=$1::uuid)))
		AND NOT EXISTS (SELECT 1 FROM unnest($16::uuid[]) AS linked(id) WHERE NOT EXISTS (
			SELECT 1 FROM framework_controls fc JOIN compliance_frameworks f ON f.id=fc.framework_id
			WHERE fc.id=linked.id AND f.deleted_at IS NULL AND (f.organization_id IS NULL OR f.organization_id=$1::uuid)))
		AND NOT EXISTS (SELECT 1 FROM unnest($17::uuid[]) AS linked(id) WHERE NOT EXISTS (SELECT 1 FROM risks r WHERE r.id=linked.id AND r.organization_id=$1::uuid AND r.deleted_at IS NULL))`
}

func (r *policyRepo) Create(ctx context.Context, orgID, userID string, input models.PolicyCreateInput, wordCount int) (*models.Policy, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(policyTransactionStarter)
	if !ok {
		return nil, errors.New("policy creation requires a transactional database executor")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning policy creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "policy-ref:"+orgID); err != nil {
		return nil, fmt.Errorf("locking tenant policy reference allocation: %w", err)
	}

	p, err := scanPolicy(tx.QueryRow(ctx, `INSERT INTO policies AS p (
		organization_id, policy_ref, title, category_id, classification,
		owner_user_id, author_user_id, approver_user_id, department_id,
		review_frequency_months, applies_to_all, applicable_departments,
		applicable_roles, applicable_locations, linked_framework_ids,
		linked_control_ids, linked_risk_ids, parent_policy_id, supersedes_policy_id,
		expiry_date, tags, priority, is_mandatory, requires_attestation,
		attestation_frequency_months, metadata)
		SELECT $1::uuid, NULLIF($2,''), $3, $4::uuid, $5, $6::uuid, $8::uuid,
			$7::uuid, $9::uuid, $10, $11, $12::uuid[], $13::text[], $14::text[],
			$15::uuid[], $16::uuid[], $17::uuid[], $18::uuid, $19::uuid,
			$20::date, $21::text[], $22, $23, $24, $25, $26::jsonb
		WHERE EXISTS (SELECT 1 FROM users u WHERE u.id=$8::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL)`+
		policyReferenceChecks()+`
		RETURNING `+policyColumns, orgID, input.PolicyRef, input.Title, input.CategoryID,
		input.Classification, input.OwnerUserID, input.ApproverUserID, userID,
		input.DepartmentID, input.ReviewFrequencyMonths, *input.AppliesToAll,
		input.ApplicableDepartments, input.ApplicableRoles, input.ApplicableLocations,
		input.LinkedFrameworkIDs, input.LinkedControlIDs, input.LinkedRiskIDs,
		input.ParentPolicyID, input.SupersedesPolicyID, input.ExpiryDate,
		input.Tags, input.Priority, *input.IsMandatory, *input.RequiresAttestation,
		input.AttestationFrequencyMonths, input.Metadata))
	if err != nil {
		return nil, fmt.Errorf("creating policy: %w", err)
	}
	versionTitle := input.InitialVersion.Title
	if versionTitle == "" {
		versionTitle = input.Title
	}
	v, err := scanPolicyVersion(tx.QueryRow(ctx, `INSERT INTO policy_versions AS v (
		policy_id, organization_id, version_number, version_label, title,
		content_html, content_text, summary, change_description, change_type,
		language, word_count, status, created_by, file_path, file_hash, metadata)
		VALUES ($1::uuid,$2::uuid,1,COALESCE(NULLIF($3,''),'1.0'),$4,$5,$6,$7,$8,$9,
			COALESCE(NULLIF($10,''),'en'),$11,'draft',$12::uuid,$13,$14,$15::jsonb)
		RETURNING `+policyVersionColumns, p.ID, orgID, input.InitialVersion.VersionLabel,
		versionTitle, input.InitialVersion.ContentHTML, input.InitialVersion.ContentText,
		input.InitialVersion.Summary, input.InitialVersion.ChangeDescription,
		input.InitialVersion.ChangeType, input.InitialVersion.Language, wordCount,
		userID, input.InitialVersion.FilePath, input.InitialVersion.FileHash,
		input.InitialVersion.Metadata))
	if err != nil {
		return nil, fmt.Errorf("creating initial policy version: %w", err)
	}
	p.CurrentVersion, p.CurrentVersionID, p.CurrentVersionRecord = 1, &v.ID, v
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing policy creation: %w", err)
	}
	return p, nil
}

func (r *policyRepo) GetByID(ctx context.Context, orgID, id string) (*models.Policy, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	p, err := scanPolicy(q.QueryRow(ctx, `SELECT `+policyColumns+` FROM policies p
		WHERE p.id=$2::uuid AND p.organization_id=$1::uuid AND p.deleted_at IS NULL`, orgID, id))
	if err != nil {
		return nil, err
	}
	if p.CurrentVersionID != nil {
		version, err := scanPolicyVersion(q.QueryRow(ctx, `SELECT `+policyVersionColumns+`
			FROM policy_versions v WHERE v.id=$3::uuid AND v.policy_id=$2::uuid AND v.organization_id=$1::uuid`, orgID, id, *p.CurrentVersionID))
		if err != nil {
			return nil, fmt.Errorf("getting current policy version: %w", err)
		}
		p.CurrentVersionRecord = version
	}
	return p, nil
}

func (r *policyRepo) Update(ctx context.Context, orgID string, p *models.Policy) (*models.Policy, error) {
	updated, err := scanPolicy(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		UPDATE policies p SET title=$3, category_id=$4::uuid, classification=$5,
			owner_user_id=$6::uuid, approver_user_id=$7::uuid, department_id=$8::uuid,
			review_frequency_months=$9, applies_to_all=$10,
			applicable_departments=$11::uuid[], applicable_roles=$12::text[],
			applicable_locations=$13::text[], linked_framework_ids=$14::uuid[],
			linked_control_ids=$15::uuid[], linked_risk_ids=$16::uuid[],
			parent_policy_id=$18::uuid, supersedes_policy_id=$19::uuid,
			expiry_date=$20::date, tags=$21::text[], priority=$22,
			is_mandatory=$23, requires_attestation=$24,
			attestation_frequency_months=$25, metadata=$26::jsonb
		WHERE p.id=$2::uuid AND p.organization_id=$1::uuid AND p.deleted_at IS NULL`+
		policyReferenceChecks()+`
		RETURNING `+policyColumns, orgID, p.ID, p.Title, p.CategoryID,
		p.Classification, p.OwnerUserID, p.ApproverUserID, p.AuthorUserID,
		p.DepartmentID, p.ReviewFrequencyMonths, p.AppliesToAll,
		p.ApplicableDepartments, p.ApplicableRoles, p.ApplicableLocations,
		p.LinkedFrameworkIDs, p.LinkedControlIDs, p.LinkedRiskIDs,
		p.ParentPolicyID, p.SupersedesPolicyID, p.ExpiryDate,
		p.Tags, p.Priority, p.IsMandatory, p.RequiresAttestation,
		p.AttestationFrequencyMonths, p.Metadata))
	if err != nil {
		return nil, err
	}
	updated.CurrentVersionRecord = p.CurrentVersionRecord
	return updated, nil
}

func (r *policyRepo) Delete(ctx context.Context, orgID, id string) error {
	tag, err := database.QuerierFromContext(ctx, r.pool).Exec(ctx, `UPDATE policies
		SET deleted_at=NOW() WHERE id=$2::uuid AND organization_id=$1::uuid AND deleted_at IS NULL`, orgID, id)
	if err != nil {
		return fmt.Errorf("soft-deleting policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *policyRepo) List(ctx context.Context, orgID string, f models.PolicyListFilter) ([]models.Policy, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM policies p WHERE p.organization_id=$1::uuid AND p.deleted_at IS NULL
		AND ($2='' OR p.status=$2) AND ($3='' OR p.classification=$3)
		AND ($4='' OR p.category_id=$4::uuid) AND ($5='' OR p.owner_user_id=$5::uuid)
		AND ($6='' OR p.search_vector @@ websearch_to_tsquery('english',$6))
		AND ($7::date IS NULL OR p.next_review_date <= $7::date)`
	args := []any{orgID, f.Status, f.Classification, f.CategoryID, f.OwnerUserID, f.Search, f.DueBefore}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting policies: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+policyColumns+where+`
		ORDER BY p.updated_at DESC, p.created_at DESC LIMIT $8 OFFSET $9`,
		append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing policies: %w", err)
	}
	defer rows.Close()
	items := make([]models.Policy, 0)
	for rows.Next() {
		item, err := scanPolicy(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning policy: %w", err)
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *policyRepo) ListCategories(ctx context.Context, orgID string) ([]models.PolicyCategory, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT id,
		organization_id, name, code, description, parent_category_id, sort_order,
		is_system_default, created_at, updated_at FROM policy_categories
		WHERE organization_id IS NULL OR organization_id=$1::uuid ORDER BY sort_order,name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing policy categories: %w", err)
	}
	defer rows.Close()
	items := make([]models.PolicyCategory, 0)
	for rows.Next() {
		var item models.PolicyCategory
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Code,
			&item.Description, &item.ParentCategoryID, &item.SortOrder,
			&item.IsSystemDefault, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning policy category: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *policyRepo) CreateVersion(ctx context.Context, orgID, policyID, userID string, input models.PolicyVersionInput, wordCount int) (*models.PolicyVersion, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(policyTransactionStarter)
	if !ok {
		return nil, errors.New("policy version creation requires a transaction")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning policy version creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "policy-version:"+policyID); err != nil {
		return nil, err
	}
	v, err := scanPolicyVersion(tx.QueryRow(ctx, `WITH target AS MATERIALIZED (
		SELECT id,title FROM policies WHERE id=$2::uuid AND organization_id=$1::uuid AND deleted_at IS NULL
	), next_version AS (
		SELECT COALESCE(MAX(version_number),0)+1 n FROM policy_versions WHERE policy_id=$2::uuid AND organization_id=$1::uuid
	)
	INSERT INTO policy_versions AS v (policy_id,organization_id,version_number,
		version_label,title,content_html,content_text,summary,change_description,
		change_type,language,word_count,status,created_by,file_path,file_hash,metadata)
	SELECT target.id,$1::uuid,next_version.n,
		COALESCE(NULLIF($4,''),next_version.n::text||'.0'),COALESCE(NULLIF($5,''),target.title),
		$6,$7,$8,$9,$10,COALESCE(NULLIF($11,''),'en'),$12,'draft',$3::uuid,$13,$14,$15::jsonb
	FROM target CROSS JOIN next_version RETURNING `+policyVersionColumns,
		orgID, policyID, userID, input.VersionLabel, input.Title, input.ContentHTML,
		input.ContentText, input.Summary, input.ChangeDescription, input.ChangeType,
		input.Language, wordCount, input.FilePath, input.FileHash, input.Metadata))
	if err != nil {
		return nil, fmt.Errorf("creating policy version: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE policies SET status='draft', title=$3
		WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, policyID, v.Title); err != nil {
		return nil, fmt.Errorf("reopening policy draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

func (r *policyRepo) GetVersion(ctx context.Context, orgID, policyID, versionID string) (*models.PolicyVersion, error) {
	return scanPolicyVersion(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx,
		`SELECT `+policyVersionColumns+` FROM policy_versions v JOIN policies p ON p.id=v.policy_id
		WHERE v.id=$3::uuid AND v.policy_id=$2::uuid AND v.organization_id=$1::uuid AND p.deleted_at IS NULL`, orgID, policyID, versionID))
}

func (r *policyRepo) ListVersions(ctx context.Context, orgID, policyID string, p models.PaginationRequest) ([]models.PolicyVersion, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM policy_versions v JOIN policies p ON p.id=v.policy_id
		WHERE v.organization_id=$1::uuid AND v.policy_id=$2::uuid AND p.deleted_at IS NULL`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, orgID, policyID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+policyVersionColumns+where+`
		ORDER BY v.version_number DESC LIMIT $3 OFFSET $4`, orgID, policyID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]models.PolicyVersion, 0)
	for rows.Next() {
		item, err := scanPolicyVersion(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

const workflowColumns = `w.id,w.policy_id,w.policy_version_id,w.organization_id,
	w.workflow_type,w.status,w.initiated_by,w.initiated_at,w.completed_at,w.due_date,
	w.current_step,w.total_steps,w.comments,w.metadata,w.created_at,w.updated_at`

func scanWorkflow(row policyRowScanner) (*models.PolicyApprovalWorkflow, error) {
	w := &models.PolicyApprovalWorkflow{}
	var metadata []byte
	if err := row.Scan(&w.ID, &w.PolicyID, &w.PolicyVersionID, &w.OrganizationID,
		&w.WorkflowType, &w.Status, &w.InitiatedBy, &w.InitiatedAt, &w.CompletedAt,
		&w.DueDate, &w.CurrentStep, &w.TotalSteps, &w.Comments, &metadata,
		&w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	w.Metadata = metadata
	return w, nil
}

func loadWorkflowSteps(ctx context.Context, q database.Querier, workflow *models.PolicyApprovalWorkflow) error {
	rows, err := q.Query(ctx, `SELECT id,workflow_id,organization_id,step_number,
		approver_user_id,approver_role,status,decision_date,comments,delegation_user_id,
		due_date,reminder_sent,created_at,updated_at FROM policy_approval_steps
		WHERE workflow_id=$1::uuid AND organization_id=$2::uuid ORDER BY step_number`, workflow.ID, workflow.OrganizationID)
	if err != nil {
		return err
	}
	defer rows.Close()
	workflow.Steps = make([]models.PolicyApprovalStep, 0)
	for rows.Next() {
		var step models.PolicyApprovalStep
		if err := rows.Scan(&step.ID, &step.WorkflowID, &step.OrganizationID,
			&step.StepNumber, &step.ApproverUserID, &step.ApproverRole, &step.Status,
			&step.DecisionDate, &step.Comments, &step.DelegationUserID, &step.DueDate,
			&step.ReminderSent, &step.CreatedAt, &step.UpdatedAt); err != nil {
			return err
		}
		workflow.Steps = append(workflow.Steps, step)
	}
	return rows.Err()
}

func (r *policyRepo) CreateApprovalWorkflow(ctx context.Context, orgID, policyID, userID string, input models.PolicySubmitInput) (*models.PolicyApprovalWorkflow, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(policyTransactionStarter)
	if !ok {
		return nil, errors.New("policy approval requires a transaction")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "policy-workflow:"+policyID); err != nil {
		return nil, err
	}
	w, err := scanWorkflow(tx.QueryRow(ctx, `INSERT INTO policy_approval_workflows AS w
		(policy_id,policy_version_id,organization_id,workflow_type,status,initiated_by,
		due_date,current_step,total_steps,comments)
		SELECT p.id,p.current_version_id,$1::uuid,$4,'in_progress',$3::uuid,$5::date,1,$6,$7
		FROM policies p WHERE p.id=$2::uuid AND p.organization_id=$1::uuid
		AND p.deleted_at IS NULL AND p.current_version_id IS NOT NULL
		AND ((p.status='draft' AND $4::varchar<>'retirement') OR (p.status='published' AND $4::varchar='retirement'))
		AND NOT EXISTS (SELECT 1 FROM policy_approval_workflows active WHERE active.policy_id=p.id AND active.status IN ('pending','in_progress'))
		RETURNING `+workflowColumns, orgID, policyID, userID, input.WorkflowType,
		input.DueDate, len(input.Approvers), input.Comments))
	if err != nil {
		return nil, fmt.Errorf("creating approval workflow: %w", err)
	}
	for index, approver := range input.Approvers {
		tag, err := tx.Exec(ctx, `INSERT INTO policy_approval_steps
			(workflow_id,organization_id,step_number,approver_user_id,approver_role,due_date)
			SELECT $1::uuid,$2::uuid,$3,$4::uuid,$5,$6::date
			WHERE $4::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$4::uuid AND u.organization_id=$2::uuid AND u.deleted_at IS NULL)`,
			w.ID, orgID, index+1, approver.ApproverUserID, approver.ApproverRole, approver.DueDate)
		if err != nil {
			return nil, fmt.Errorf("creating approval step: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, pgx.ErrNoRows
		}
	}
	if input.WorkflowType != "retirement" {
		if _, err := tx.Exec(ctx, `UPDATE policies SET status='under_review'
			WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, policyID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status='under_review'
			WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, w.PolicyVersionID); err != nil {
			return nil, err
		}
	}
	if err := loadWorkflowSteps(ctx, tx, w); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func (r *policyRepo) GetActiveApprovalWorkflow(ctx context.Context, orgID, policyID string) (*models.PolicyApprovalWorkflow, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	w, err := scanWorkflow(q.QueryRow(ctx, `SELECT `+workflowColumns+` FROM policy_approval_workflows w
		JOIN policies p ON p.id=w.policy_id WHERE w.organization_id=$1::uuid AND w.policy_id=$2::uuid
		AND w.status IN ('pending','in_progress') AND p.deleted_at IS NULL ORDER BY w.created_at DESC LIMIT 1`, orgID, policyID))
	if err != nil {
		return nil, err
	}
	if err := loadWorkflowSteps(ctx, q, w); err != nil {
		return nil, err
	}
	return w, nil
}

func (r *policyRepo) DecideApproval(ctx context.Context, orgID, policyID, userID, role string, input models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(policyTransactionStarter)
	if !ok {
		return nil, errors.New("policy decision requires a transaction")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var workflowID, stepID, workflowType string
	var current, total int
	var versionID *string
	err = tx.QueryRow(ctx, `SELECT w.id,s.id,w.current_step,w.total_steps,w.policy_version_id,w.workflow_type
		FROM policy_approval_workflows w JOIN policy_approval_steps s
		ON s.workflow_id=w.id AND s.step_number=w.current_step
		JOIN policies p ON p.id=w.policy_id
		WHERE w.organization_id=$1::uuid AND w.policy_id=$2::uuid AND w.status='in_progress'
		AND p.deleted_at IS NULL AND s.status='pending'
		AND (s.approver_user_id=$3::uuid OR s.delegation_user_id=$3::uuid OR (s.approver_user_id IS NULL AND s.approver_role=$4))
		FOR UPDATE OF w,s`, orgID, policyID, userID, role).Scan(&workflowID, &stepID, &current, &total, &versionID, &workflowType)
	if err != nil {
		return nil, fmt.Errorf("locking approval step: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE policy_approval_steps SET status=$3,
		decision_date=NOW(),comments=$4,digital_signature=$5 WHERE id=$1::uuid AND organization_id=$2::uuid`,
		stepID, orgID, input.Decision, input.Comments, input.DigitalSignature); err != nil {
		return nil, err
	}
	if input.Decision == "rejected" {
		if _, err := tx.Exec(ctx, `UPDATE policy_approval_workflows SET status='rejected',completed_at=NOW()
			WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, workflowID); err != nil {
			return nil, err
		}
		if workflowType != "retirement" {
			if _, err := tx.Exec(ctx, `UPDATE policies SET status='draft'
				WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, policyID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status='draft'
				WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, versionID); err != nil {
				return nil, err
			}
		}
	} else if current == total {
		if _, err := tx.Exec(ctx, `UPDATE policy_approval_workflows SET status='approved',completed_at=NOW(),current_step=total_steps+1
			WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, workflowID); err != nil {
			return nil, err
		}
		if workflowType == "retirement" {
			if _, err := tx.Exec(ctx, `UPDATE policies SET status='retired',approver_user_id=$3::uuid
				WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, policyID, userID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status='archived'
				WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, versionID); err != nil {
				return nil, err
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE policies SET status='approved',approver_user_id=$3::uuid
				WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, policyID, userID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status='approved'
				WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, versionID); err != nil {
				return nil, err
			}
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE policy_approval_workflows SET current_step=current_step+1 WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, workflowID); err != nil {
			return nil, err
		}
	}
	w, err := scanWorkflow(tx.QueryRow(ctx, `SELECT `+workflowColumns+` FROM policy_approval_workflows w WHERE w.id=$2::uuid AND w.organization_id=$1::uuid`, orgID, workflowID))
	if err != nil {
		return nil, err
	}
	if err := loadWorkflowSteps(ctx, tx, w); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func (r *policyRepo) Publish(ctx context.Context, orgID, policyID, userID string) (*models.Policy, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(policyTransactionStarter)
	if !ok {
		return nil, errors.New("policy publication requires a transaction")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var versionID string
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM policies WHERE id=$2::uuid
		AND organization_id=$1::uuid AND deleted_at IS NULL AND status='approved' FOR UPDATE`, orgID, policyID).Scan(&versionID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status='archived'
		WHERE policy_id=$2::uuid AND organization_id=$1::uuid AND status='published' AND id<>$3::uuid`, orgID, policyID, versionID); err != nil {
		return nil, err
	}
	published, err := tx.Exec(ctx, `UPDATE policy_versions SET status='published',published_at=NOW(),published_by=$4::uuid
		WHERE id=$3::uuid AND policy_id=$2::uuid AND organization_id=$1::uuid AND status='approved'`, orgID, policyID, versionID, userID)
	if err != nil {
		return nil, err
	}
	if published.RowsAffected() != 1 {
		return nil, pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `UPDATE policies SET status='published'
		WHERE id=$2::uuid AND organization_id=$1::uuid`, orgID, policyID); err != nil {
		return nil, err
	}
	p, err := scanPolicy(tx.QueryRow(ctx, `SELECT `+policyColumns+` FROM policies p WHERE p.id=$2::uuid AND p.organization_id=$1::uuid`, orgID, policyID))
	if err != nil {
		return nil, err
	}
	v, err := scanPolicyVersion(tx.QueryRow(ctx, `SELECT `+policyVersionColumns+` FROM policy_versions v WHERE v.id=$2::uuid AND v.organization_id=$1::uuid`, orgID, versionID))
	if err != nil {
		return nil, err
	}
	p.CurrentVersionRecord = v
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

const reviewColumns = `r.id,r.organization_id,r.policy_id,r.review_type,r.status,
	r.reviewer_user_id,r.review_date,r.due_date,r.completed_date,r.outcome,r.findings,
	r.recommendations,r.triggered_by,r.new_version_id,r.created_at,r.updated_at`

func scanPolicyReview(row policyRowScanner) (*models.PolicyReview, error) {
	r := &models.PolicyReview{}
	err := row.Scan(&r.ID, &r.OrganizationID, &r.PolicyID, &r.ReviewType, &r.Status,
		&r.ReviewerUserID, &r.ReviewDate, &r.DueDate, &r.CompletedDate, &r.Outcome,
		&r.Findings, &r.Recommendations, &r.TriggeredBy, &r.NewVersionID,
		&r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *policyRepo) CreateReview(ctx context.Context, orgID, policyID string, input models.PolicyReviewInput) (*models.PolicyReview, error) {
	return scanPolicyReview(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO policy_reviews AS r (organization_id,policy_id,review_type,status,
			reviewer_user_id,review_date,due_date,triggered_by)
		SELECT $1::uuid,p.id,$3,'scheduled',$4::uuid,$5::date,$6::date,$7 FROM policies p
		WHERE p.id=$2::uuid AND p.organization_id=$1::uuid AND p.deleted_at IS NULL
		AND ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$4::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		RETURNING `+reviewColumns, orgID, policyID, input.ReviewType,
		input.ReviewerUserID, input.ReviewDate, input.DueDate, input.TriggeredBy))
}

func (r *policyRepo) GetReview(ctx context.Context, orgID, policyID, reviewID string) (*models.PolicyReview, error) {
	return scanPolicyReview(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `SELECT `+reviewColumns+`
		FROM policy_reviews r JOIN policies p ON p.id=r.policy_id WHERE r.id=$3::uuid
		AND r.policy_id=$2::uuid AND r.organization_id=$1::uuid AND p.deleted_at IS NULL`, orgID, policyID, reviewID))
}

func (r *policyRepo) UpdateReview(ctx context.Context, orgID, policyID string, review *models.PolicyReview) (*models.PolicyReview, error) {
	return scanPolicyReview(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `WITH updated AS (
		UPDATE policy_reviews r SET status=$4::varchar,outcome=$5,findings=$6,recommendations=$7,
			new_version_id=$8::uuid,completed_date=CASE WHEN $4::varchar='completed' THEN CURRENT_DATE ELSE completed_date END
		FROM policies p WHERE r.id=$3::uuid AND r.policy_id=$2::uuid AND r.organization_id=$1::uuid
		AND p.id=r.policy_id AND p.deleted_at IS NULL
		AND ($8::uuid IS NULL OR EXISTS (SELECT 1 FROM policy_versions v WHERE v.id=$8::uuid AND v.policy_id=$2::uuid AND v.organization_id=$1::uuid))
		RETURNING r.*
	), policy_updated AS (
		UPDATE policies p SET last_review_date=CASE WHEN updated.status='completed' THEN CURRENT_DATE ELSE p.last_review_date END,
			next_review_date=CASE WHEN updated.status='completed' THEN CURRENT_DATE+(p.review_frequency_months||' months')::interval ELSE p.next_review_date END,
			review_status=CASE WHEN updated.status='completed' THEN 'current' ELSE p.review_status END
		FROM updated WHERE p.id=updated.policy_id RETURNING p.id
	)
	SELECT `+reviewColumns+` FROM updated r`, orgID, policyID, review.ID, review.Status,
		review.Outcome, review.Findings, review.Recommendations, review.NewVersionID))
}

func (r *policyRepo) ListReviews(ctx context.Context, orgID, policyID string, p models.PaginationRequest) ([]models.PolicyReview, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM policy_reviews r JOIN policies p ON p.id=r.policy_id WHERE r.organization_id=$1::uuid AND r.policy_id=$2::uuid AND p.deleted_at IS NULL`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, orgID, policyID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+reviewColumns+where+` ORDER BY r.created_at DESC LIMIT $3 OFFSET $4`, orgID, policyID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]models.PolicyReview, 0)
	for rows.Next() {
		item, err := scanPolicyReview(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

const attestationColumns = `a.id,a.policy_id,a.policy_version_id,a.organization_id,a.user_id,
	a.campaign_id,a.status,a.attested_at,a.attested_from_ip::text,a.attestation_method,
	a.attestation_text,a.declined_reason,a.due_date,a.expires_at,a.metadata,a.created_at`

func scanAttestation(row policyRowScanner) (*models.PolicyAttestation, error) {
	a := &models.PolicyAttestation{}
	var metadata []byte
	err := row.Scan(&a.ID, &a.PolicyID, &a.PolicyVersionID, &a.OrganizationID, &a.UserID,
		&a.CampaignID, &a.Status, &a.AttestedAt, &a.AttestedFromIP, &a.AttestationMethod,
		&a.AttestationText, &a.DeclinedReason, &a.DueDate, &a.ExpiresAt, &metadata, &a.CreatedAt)
	a.Metadata = metadata
	return a, err
}

func (r *policyRepo) Acknowledge(ctx context.Context, orgID, policyID, userID, ip string, input models.PolicyAttestationInput) (*models.PolicyAttestation, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(policyTransactionStarter)
	if !ok {
		return nil, errors.New("policy acknowledgement requires a transaction")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "policy-attestation:"+policyID+":"+userID); err != nil {
		return nil, err
	}
	var versionID string
	var months int
	if err := tx.QueryRow(ctx, `SELECT current_version_id,attestation_frequency_months FROM policies WHERE id=$2::uuid AND organization_id=$1::uuid AND deleted_at IS NULL AND status='published' AND requires_attestation`, orgID, policyID).Scan(&versionID, &months); err != nil {
		return nil, err
	}
	status := "attested"
	if input.Decision == "decline" {
		status = "declined"
	}
	a, err := scanAttestation(tx.QueryRow(ctx, `WITH existing AS (
		SELECT id FROM policy_attestations WHERE organization_id=$1::uuid AND policy_id=$2::uuid AND policy_version_id=$3::uuid AND user_id=$4::uuid ORDER BY created_at DESC LIMIT 1
	), updated AS (
		UPDATE policy_attestations a SET status=$5::varchar,attested_at=CASE WHEN $5::varchar='attested' THEN NOW() ELSE NULL END,
			attested_from_ip=NULLIF($6,'')::inet,attestation_method=$7,attestation_text=COALESCE($8,a.attestation_text),
			declined_reason=$9,expires_at=CASE WHEN $5::varchar='attested' THEN NOW()+($10::int||' months')::interval ELSE NULL END,metadata=$11::jsonb
		FROM existing WHERE a.id=existing.id RETURNING a.*
	), inserted AS (
		INSERT INTO policy_attestations AS a (policy_id,policy_version_id,organization_id,user_id,status,attested_at,attested_from_ip,attestation_method,attestation_text,declined_reason,expires_at,metadata)
		SELECT $2::uuid,$3::uuid,$1::uuid,$4::uuid,$5::varchar,CASE WHEN $5::varchar='attested' THEN NOW() ELSE NULL END,NULLIF($6,'')::inet,$7,$8,$9,
			CASE WHEN $5::varchar='attested' THEN NOW()+($10::int||' months')::interval ELSE NULL END,$11::jsonb
		WHERE NOT EXISTS(SELECT 1 FROM existing) AND EXISTS(SELECT 1 FROM users u WHERE u.id=$4::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL) RETURNING a.*
	)
	SELECT `+attestationColumns+` FROM updated a UNION ALL SELECT `+attestationColumns+` FROM inserted a`, orgID, policyID, versionID, userID, status, ip, input.AttestationMethod, input.AttestationText, input.DeclinedReason, months, input.Metadata))
	if err != nil {
		return nil, fmt.Errorf("recording policy acknowledgement: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *policyRepo) ListAttestations(ctx context.Context, orgID, policyID string, p models.PaginationRequest) ([]models.PolicyAttestation, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM policy_attestations a JOIN policies p ON p.id=a.policy_id WHERE a.organization_id=$1::uuid AND a.policy_id=$2::uuid AND p.deleted_at IS NULL`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, orgID, policyID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+attestationColumns+where+` ORDER BY a.created_at DESC LIMIT $3 OFFSET $4`, orgID, policyID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]models.PolicyAttestation, 0)
	for rows.Next() {
		item, err := scanAttestation(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

const exceptionColumns = `e.id,e.organization_id,e.policy_id,e.exception_ref,e.title,e.description,
	e.justification,e.risk_assessment,e.compensating_controls,e.status,e.requested_by,
	e.approved_by,e.approved_at,e.effective_date,e.expiry_date,e.review_date,e.risk_level,
	e.linked_risk_id,e.conditions,e.created_at,e.updated_at`

func scanPolicyException(row policyRowScanner) (*models.PolicyException, error) {
	e := &models.PolicyException{}
	err := row.Scan(&e.ID, &e.OrganizationID, &e.PolicyID, &e.ExceptionRef,
		&e.Title, &e.Description, &e.Justification, &e.RiskAssessment, &e.CompensatingControls,
		&e.Status, &e.RequestedBy, &e.ApprovedBy, &e.ApprovedAt, &e.EffectiveDate, &e.ExpiryDate,
		&e.ReviewDate, &e.RiskLevel, &e.LinkedRiskID, &e.Conditions, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func (r *policyRepo) CreateException(ctx context.Context, orgID, policyID, userID string, input models.PolicyExceptionInput) (*models.PolicyException, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(policyTransactionStarter)
	if !ok {
		return nil, errors.New("policy exception creation requires a transaction")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "policy-exception:"+orgID); err != nil {
		return nil, err
	}
	e, err := scanPolicyException(tx.QueryRow(ctx, `INSERT INTO policy_exceptions AS e (organization_id,policy_id,exception_ref,title,description,justification,risk_assessment,compensating_controls,status,requested_by,effective_date,expiry_date,review_date,risk_level,linked_risk_id)
		SELECT $1::uuid,p.id,NULL,$4,$5,$6,$7,$8,'requested',$3::uuid,$9::date,$10::date,$11::date,$12,$13::uuid FROM policies p
		WHERE p.id=$2::uuid AND p.organization_id=$1::uuid AND p.deleted_at IS NULL
		AND ($13::uuid IS NULL OR EXISTS(SELECT 1 FROM risks r WHERE r.id=$13::uuid AND r.organization_id=$1::uuid AND r.deleted_at IS NULL)) RETURNING `+exceptionColumns,
		orgID, policyID, userID, input.Title, input.Description, input.Justification, input.RiskAssessment, input.CompensatingControls, input.EffectiveDate, input.ExpiryDate, input.ReviewDate, input.RiskLevel, input.LinkedRiskID))
	if err != nil {
		return nil, fmt.Errorf("creating policy exception: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

func (r *policyRepo) GetException(ctx context.Context, orgID, policyID, exceptionID string) (*models.PolicyException, error) {
	return scanPolicyException(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `SELECT `+exceptionColumns+` FROM policy_exceptions e JOIN policies p ON p.id=e.policy_id WHERE e.id=$3::uuid AND e.policy_id=$2::uuid AND e.organization_id=$1::uuid AND p.deleted_at IS NULL`, orgID, policyID, exceptionID))
}

func (r *policyRepo) UpdateExceptionDecision(ctx context.Context, orgID, policyID, exceptionID, userID string, input models.PolicyExceptionDecisionInput) (*models.PolicyException, error) {
	return scanPolicyException(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `UPDATE policy_exceptions e SET
		status=$5::varchar,conditions=$6,expiry_date=COALESCE($7::date,e.expiry_date),
		approved_by=CASE WHEN $5::varchar='approved' THEN $4::uuid ELSE e.approved_by END,
		approved_at=CASE WHEN $5::varchar='approved' THEN NOW() ELSE e.approved_at END,
		effective_date=CASE WHEN $5::varchar='approved' THEN COALESCE(e.effective_date,CURRENT_DATE) ELSE e.effective_date END
		FROM policies p WHERE e.id=$3::uuid AND e.policy_id=$2::uuid AND e.organization_id=$1::uuid
		AND p.id=e.policy_id AND p.deleted_at IS NULL RETURNING `+exceptionColumns,
		orgID, policyID, exceptionID, userID, input.Decision, input.Conditions, input.ExpiryDate))
}

func (r *policyRepo) ListExceptions(ctx context.Context, orgID, policyID string, p models.PaginationRequest) ([]models.PolicyException, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM policy_exceptions e JOIN policies p ON p.id=e.policy_id WHERE e.organization_id=$1::uuid AND e.policy_id=$2::uuid AND p.deleted_at IS NULL`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, orgID, policyID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+exceptionColumns+where+` ORDER BY e.created_at DESC LIMIT $3 OFFSET $4`, orgID, policyID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]models.PolicyException, 0)
	for rows.Next() {
		item, err := scanPolicyException(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}
