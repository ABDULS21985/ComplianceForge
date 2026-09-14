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

// AuditRepository is the canonical persistence contract for migration 000045.
// Every operation carries an explicit tenant ID and uses the request-scoped
// executor so application predicates and PostgreSQL RLS protect the boundary.
type AuditRepository interface {
	Create(context.Context, *models.Audit) (*models.Audit, error)
	GetByID(context.Context, string, string) (*models.Audit, error)
	Update(context.Context, string, *models.Audit) (*models.Audit, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.AuditListFilter) ([]models.Audit, int, error)
	ListUpcoming(context.Context, string, int) ([]models.Audit, error)
	CreateFinding(context.Context, *models.AuditFinding) (*models.AuditFinding, error)
	GetFindingByID(context.Context, string, string, string) (*models.AuditFinding, error)
	UpdateFinding(context.Context, string, string, *models.AuditFinding) (*models.AuditFinding, error)
	DeleteFinding(context.Context, string, string, string) error
	ListFindings(context.Context, string, string, models.PaginationRequest) ([]models.AuditFinding, int, error)
	FindingStats(context.Context, string, string) (*models.AuditFindingStats, error)
}

type auditRepo struct{ pool *pgxpool.Pool }

func NewAuditRepository(pool *pgxpool.Pool) AuditRepository { return &auditRepo{pool: pool} }

var _ AuditRepository = (*auditRepo)(nil)

type auditTransactionStarter interface {
	Begin(context.Context) (pgx.Tx, error)
}

const auditColumns = `
	a.id, a.organization_id, a.audit_ref, a.title, a.description,
	a.audit_type, a.status, a.lead_auditor_id, a.scope,
	a.scheduled_start_date, a.scheduled_end_date, a.actual_start_date,
	a.actual_end_date, a.framework_id, a.created_by, a.metadata,
	a.created_at, a.updated_at, a.deleted_at,
	u.id, COALESCE(u.first_name, ''), COALESCE(u.last_name, ''), u.email,
	f.id, f.code, f.name,
	COALESCE(stats.findings_count, 0), COALESCE(stats.critical_open, 0),
	COALESCE(stats.high_open, 0)`

const auditJoins = `
	JOIN users u ON u.id=a.lead_auditor_id AND u.organization_id=a.organization_id
	LEFT JOIN compliance_frameworks f ON f.id=a.framework_id
		AND (f.organization_id IS NULL OR f.organization_id=a.organization_id)
	LEFT JOIN LATERAL (
		SELECT count(*)::int AS findings_count,
			count(*) FILTER (WHERE af.severity='critical' AND af.status IN ('open','in_progress'))::int AS critical_open,
			count(*) FILTER (WHERE af.severity='high' AND af.status IN ('open','in_progress'))::int AS high_open
		FROM audit_findings af
		WHERE af.organization_id=a.organization_id AND af.audit_id=a.id AND af.deleted_at IS NULL
	) stats ON true`

type auditRowScanner interface{ Scan(...any) error }

func scanAudit(row auditRowScanner) (*models.Audit, error) {
	audit := &models.Audit{}
	var frameworkID, frameworkCode, frameworkName *string
	var leadID, leadFirst, leadLast, leadEmail string
	if err := row.Scan(
		&audit.ID, &audit.OrganizationID, &audit.AuditRef, &audit.Title, &audit.Description,
		&audit.Type, &audit.Status, &audit.LeadAuditorID, &audit.Scope,
		&audit.ScheduledStartDate, &audit.ScheduledEndDate, &audit.ActualStartDate,
		&audit.ActualEndDate, &audit.FrameworkID, &audit.CreatedBy, &audit.Metadata,
		&audit.CreatedAt, &audit.UpdatedAt, &audit.DeletedAt,
		&leadID, &leadFirst, &leadLast, &leadEmail,
		&frameworkID, &frameworkCode, &frameworkName,
		&audit.FindingsCount, &audit.CriticalOpen, &audit.HighOpen,
	); err != nil {
		return nil, err
	}
	audit.LeadAuditor = &models.AuditPerson{ID: leadID, FirstName: leadFirst, LastName: leadLast, Email: leadEmail}
	if frameworkID != nil {
		audit.Framework = &models.AuditFramework{ID: *frameworkID}
		if frameworkCode != nil {
			audit.Framework.Code = *frameworkCode
		}
		if frameworkName != nil {
			audit.Framework.Name = *frameworkName
		}
	}
	return audit, nil
}

func (r *auditRepo) Create(ctx context.Context, audit *models.Audit) (*models.Audit, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(auditTransactionStarter)
	if !ok {
		return nil, errors.New("audit creation requires a transactional database executor")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning audit creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sequence int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO audit_reference_sequences (organization_id, next_audit_number, next_finding_number)
		VALUES ($1::uuid, 2, 1)
		ON CONFLICT (organization_id) DO UPDATE
		SET next_audit_number=audit_reference_sequences.next_audit_number+1
		RETURNING next_audit_number-1`, audit.OrganizationID).Scan(&sequence); err != nil {
		return nil, fmt.Errorf("allocating audit reference: %w", err)
	}
	auditRef := fmt.Sprintf("AUD-%04d", sequence)
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO audits (
			organization_id, audit_ref, title, description, audit_type, status,
			lead_auditor_id, scope, scheduled_start_date, scheduled_end_date,
			framework_id, created_by, metadata
		)
		SELECT $1::uuid, $2, $3, $4, $5, $6, $7::uuid, $8, $9::date, $10::date,
			$11::uuid, $12::uuid, $13::jsonb
		WHERE EXISTS (
			SELECT 1 FROM users creator WHERE creator.id=$12::uuid
			AND creator.organization_id=$1::uuid AND creator.deleted_at IS NULL
		) AND EXISTS (
			SELECT 1 FROM users lead_user WHERE lead_user.id=$7::uuid
			AND lead_user.organization_id=$1::uuid AND lead_user.deleted_at IS NULL
		) AND ($11::uuid IS NULL OR EXISTS (
			SELECT 1 FROM compliance_frameworks framework WHERE framework.id=$11::uuid
			AND framework.deleted_at IS NULL
			AND (framework.organization_id IS NULL OR framework.organization_id=$1::uuid)
		))
		RETURNING id`, audit.OrganizationID, auditRef, audit.Title, audit.Description,
		audit.Type, audit.Status, audit.LeadAuditorID, audit.Scope,
		audit.ScheduledStartDate, audit.ScheduledEndDate, audit.FrameworkID,
		audit.CreatedBy, audit.Metadata).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("creating audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing audit creation: %w", err)
	}
	return r.GetByID(ctx, audit.OrganizationID, id)
}

func (r *auditRepo) GetByID(ctx context.Context, orgID, id string) (*models.Audit, error) {
	return scanAudit(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		SELECT `+auditColumns+` FROM audits a `+auditJoins+`
		WHERE a.organization_id=$1::uuid AND a.id=$2::uuid AND a.deleted_at IS NULL`, orgID, id))
}

func (r *auditRepo) Update(ctx context.Context, orgID string, audit *models.Audit) (*models.Audit, error) {
	var id string
	err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		UPDATE audits a SET title=$3, description=$4, audit_type=$5, status=$6,
			lead_auditor_id=$7::uuid, scope=$8, scheduled_start_date=$9::date,
			scheduled_end_date=$10::date, actual_start_date=$11::date,
			actual_end_date=$12::date, framework_id=$13::uuid, metadata=$14::jsonb
		WHERE a.organization_id=$1::uuid AND a.id=$2::uuid AND a.deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM users lead_user WHERE lead_user.id=$7::uuid
			AND lead_user.organization_id=$1::uuid AND lead_user.deleted_at IS NULL)
		AND ($13::uuid IS NULL OR EXISTS (
			SELECT 1 FROM compliance_frameworks framework WHERE framework.id=$13::uuid
			AND framework.deleted_at IS NULL
			AND (framework.organization_id IS NULL OR framework.organization_id=$1::uuid)))
		RETURNING a.id`, orgID, audit.ID, audit.Title, audit.Description, audit.Type,
		audit.Status, audit.LeadAuditorID, audit.Scope, audit.ScheduledStartDate,
		audit.ScheduledEndDate, audit.ActualStartDate, audit.ActualEndDate,
		audit.FrameworkID, audit.Metadata).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("updating audit: %w", err)
	}
	return r.GetByID(ctx, orgID, id)
}

func (r *auditRepo) Delete(ctx context.Context, orgID, id string) error {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(auditTransactionStarter)
	if !ok {
		return errors.New("audit deletion requires a transactional database executor")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning audit deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE audits SET deleted_at=NOW()
		WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL`, orgID, id)
	if err != nil {
		return fmt.Errorf("soft-deleting audit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `UPDATE audit_findings SET deleted_at=NOW()
		WHERE organization_id=$1::uuid AND audit_id=$2::uuid AND deleted_at IS NULL`, orgID, id); err != nil {
		return fmt.Errorf("soft-deleting audit findings: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing audit deletion: %w", err)
	}
	return nil
}

func (r *auditRepo) List(ctx context.Context, orgID string, filter models.AuditListFilter) ([]models.Audit, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` WHERE a.organization_id=$1::uuid AND a.deleted_at IS NULL
		AND ($2='' OR a.status=$2)
		AND ($3='' OR a.audit_type=$3)
		AND ($4='' OR a.lead_auditor_id=NULLIF($4,'')::uuid)
		AND ($5='' OR a.framework_id=NULLIF($5,'')::uuid)
		AND ($6='' OR a.search_vector @@ websearch_to_tsquery('english', $6))`
	args := []any{orgID, filter.Status, filter.AuditType, filter.LeadAuditorID, filter.FrameworkID, filter.Search}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM audits a`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audits: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+auditColumns+` FROM audits a `+auditJoins+where+`
		ORDER BY a.scheduled_start_date DESC, a.created_at DESC
		LIMIT $7 OFFSET $8`, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audits: %w", err)
	}
	defer rows.Close()
	items := make([]models.Audit, 0)
	for rows.Next() {
		item, err := scanAudit(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning audit: %w", err)
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *auditRepo) ListUpcoming(ctx context.Context, orgID string, limit int) ([]models.Audit, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT `+auditColumns+` FROM audits a `+auditJoins+`
		WHERE a.organization_id=$1::uuid AND a.deleted_at IS NULL
		AND a.status='planned' AND a.scheduled_start_date >= CURRENT_DATE
		ORDER BY a.scheduled_start_date, a.created_at LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing upcoming audits: %w", err)
	}
	defer rows.Close()
	items := make([]models.Audit, 0)
	for rows.Next() {
		item, err := scanAudit(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning upcoming audit: %w", err)
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

const findingColumns = `
	af.id, af.organization_id, af.audit_id, af.finding_ref, af.control_id,
	af.title, af.description, af.severity, af.status, af.finding_type,
	af.root_cause, af.recommendation, af.remediation_plan,
	af.responsible_user_id, af.due_date, af.resolved_at,
	af.accepted_risk_reason, af.created_by, af.metadata,
	af.created_at, af.updated_at, af.deleted_at,
	u.id, COALESCE(u.first_name, ''), COALESCE(u.last_name, ''), u.email`

type findingRowScanner interface{ Scan(...any) error }

func scanFinding(row findingRowScanner) (*models.AuditFinding, error) {
	item := &models.AuditFinding{}
	var userID, firstName, lastName, email string
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.AuditID, &item.FindingRef, &item.ControlID,
		&item.Title, &item.Description, &item.Severity, &item.Status, &item.FindingType,
		&item.RootCause, &item.Recommendation, &item.RemediationPlan,
		&item.ResponsibleUserID, &item.DueDate, &item.ResolvedAt,
		&item.AcceptedRiskReason, &item.CreatedBy, &item.Metadata,
		&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		&userID, &firstName, &lastName, &email,
	); err != nil {
		return nil, err
	}
	item.ResponsibleUser = &models.AuditPerson{ID: userID, FirstName: firstName, LastName: lastName, Email: email}
	return item, nil
}

const findingJoin = ` JOIN users u ON u.id=af.responsible_user_id AND u.organization_id=af.organization_id `

func (r *auditRepo) CreateFinding(ctx context.Context, finding *models.AuditFinding) (*models.AuditFinding, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(auditTransactionStarter)
	if !ok {
		return nil, errors.New("finding creation requires a transactional database executor")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning finding creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var sequence int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO audit_reference_sequences (organization_id, next_audit_number, next_finding_number)
		VALUES ($1::uuid, 1, 2)
		ON CONFLICT (organization_id) DO UPDATE
		SET next_finding_number=audit_reference_sequences.next_finding_number+1
		RETURNING next_finding_number-1`, finding.OrganizationID).Scan(&sequence); err != nil {
		return nil, fmt.Errorf("allocating finding reference: %w", err)
	}
	findingRef := fmt.Sprintf("FND-%04d", sequence)
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO audit_findings (
			organization_id, audit_id, finding_ref, control_id, title, description,
			severity, status, finding_type, root_cause, recommendation,
			remediation_plan, responsible_user_id, due_date, created_by, metadata
		)
		SELECT $1::uuid, $2::uuid, $3, $4::uuid, $5, $6, $7, $8, $9, $10,
			$11, $12, $13::uuid, $14::date, $15::uuid, $16::jsonb
		WHERE EXISTS (SELECT 1 FROM audits a WHERE a.id=$2::uuid
			AND a.organization_id=$1::uuid AND a.deleted_at IS NULL)
		AND EXISTS (SELECT 1 FROM users owner_user WHERE owner_user.id=$13::uuid
			AND owner_user.organization_id=$1::uuid AND owner_user.deleted_at IS NULL)
		AND EXISTS (SELECT 1 FROM users creator WHERE creator.id=$15::uuid
			AND creator.organization_id=$1::uuid AND creator.deleted_at IS NULL)
		AND ($4::uuid IS NULL OR EXISTS (
			SELECT 1 FROM framework_controls control
			JOIN compliance_frameworks framework ON framework.id=control.framework_id
			WHERE control.id=$4::uuid AND framework.deleted_at IS NULL
			AND (framework.organization_id IS NULL OR framework.organization_id=$1::uuid)))
		RETURNING id`, finding.OrganizationID, finding.AuditID, findingRef,
		finding.ControlID, finding.Title, finding.Description, finding.Severity,
		finding.Status, finding.FindingType, finding.RootCause, finding.Recommendation,
		finding.RemediationPlan, finding.ResponsibleUserID, finding.DueDate,
		finding.CreatedBy, finding.Metadata).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("creating audit finding: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing finding creation: %w", err)
	}
	return r.GetFindingByID(ctx, finding.OrganizationID, finding.AuditID, id)
}

func (r *auditRepo) GetFindingByID(ctx context.Context, orgID, auditID, id string) (*models.AuditFinding, error) {
	return scanFinding(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		SELECT `+findingColumns+` FROM audit_findings af `+findingJoin+`
		WHERE af.organization_id=$1::uuid AND af.audit_id=$2::uuid
		AND af.id=$3::uuid AND af.deleted_at IS NULL`, orgID, auditID, id))
}

func (r *auditRepo) UpdateFinding(ctx context.Context, orgID, auditID string, finding *models.AuditFinding) (*models.AuditFinding, error) {
	var id string
	err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		UPDATE audit_findings af SET control_id=$4::uuid, title=$5, description=$6,
			severity=$7, status=$8, finding_type=$9, root_cause=$10,
			recommendation=$11, remediation_plan=$12, responsible_user_id=$13::uuid,
			due_date=$14::date, resolved_at=$15, accepted_risk_reason=$16,
			metadata=$17::jsonb
		WHERE af.organization_id=$1::uuid AND af.audit_id=$2::uuid
		AND af.id=$3::uuid AND af.deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM users owner_user WHERE owner_user.id=$13::uuid
			AND owner_user.organization_id=$1::uuid AND owner_user.deleted_at IS NULL)
		AND ($4::uuid IS NULL OR EXISTS (
			SELECT 1 FROM framework_controls control
			JOIN compliance_frameworks framework ON framework.id=control.framework_id
			WHERE control.id=$4::uuid AND framework.deleted_at IS NULL
			AND (framework.organization_id IS NULL OR framework.organization_id=$1::uuid)))
		RETURNING af.id`, orgID, auditID, finding.ID, finding.ControlID,
		finding.Title, finding.Description, finding.Severity, finding.Status,
		finding.FindingType, finding.RootCause, finding.Recommendation,
		finding.RemediationPlan, finding.ResponsibleUserID, finding.DueDate,
		finding.ResolvedAt, finding.AcceptedRiskReason, finding.Metadata).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("updating audit finding: %w", err)
	}
	return r.GetFindingByID(ctx, orgID, auditID, id)
}

func (r *auditRepo) DeleteFinding(ctx context.Context, orgID, auditID, id string) error {
	tag, err := database.QuerierFromContext(ctx, r.pool).Exec(ctx, `
		UPDATE audit_findings SET deleted_at=NOW()
		WHERE organization_id=$1::uuid AND audit_id=$2::uuid AND id=$3::uuid AND deleted_at IS NULL`, orgID, auditID, id)
	if err != nil {
		return fmt.Errorf("soft-deleting audit finding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *auditRepo) ListFindings(ctx context.Context, orgID, auditID string, pagination models.PaginationRequest) ([]models.AuditFinding, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` WHERE af.organization_id=$1::uuid AND af.audit_id=$2::uuid AND af.deleted_at IS NULL`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM audit_findings af`+where, orgID, auditID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit findings: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+findingColumns+` FROM audit_findings af `+findingJoin+where+`
		ORDER BY CASE af.severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2
			WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5 END,
			af.due_date, af.created_at DESC LIMIT $3 OFFSET $4`, orgID, auditID,
		pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audit findings: %w", err)
	}
	defer rows.Close()
	items := make([]models.AuditFinding, 0)
	for rows.Next() {
		item, err := scanFinding(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning audit finding: %w", err)
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *auditRepo) FindingStats(ctx context.Context, orgID, auditID string) (*models.AuditFindingStats, error) {
	stats := &models.AuditFindingStats{}
	err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		SELECT count(*)::int,
			count(*) FILTER (WHERE status='open')::int,
			count(*) FILTER (WHERE status='in_progress')::int,
			count(*) FILTER (WHERE status='resolved')::int,
			count(*) FILTER (WHERE status='closed')::int,
			count(*) FILTER (WHERE status='accepted')::int,
			count(*) FILTER (WHERE severity='critical' AND status IN ('open','in_progress'))::int,
			count(*) FILTER (WHERE severity='high' AND status IN ('open','in_progress'))::int,
			count(*) FILTER (WHERE due_date<CURRENT_DATE AND status IN ('open','in_progress'))::int
		FROM audit_findings
		WHERE organization_id=$1::uuid AND audit_id=$2::uuid AND deleted_at IS NULL`, orgID, auditID).Scan(
		&stats.Total, &stats.Open, &stats.InProgress, &stats.Resolved,
		&stats.Closed, &stats.Accepted, &stats.CriticalOpen, &stats.HighOpen,
		&stats.Overdue,
	)
	if err != nil {
		return nil, fmt.Errorf("calculating audit finding statistics: %w", err)
	}
	return stats, nil
}
