package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var (
	ErrIncidentVersionConflict     = errors.New("incident version conflict")
	ErrIncidentIdempotencyConflict = errors.New("incident idempotency conflict")
)

// IncidentRepository is the persistence contract for migration 000046. Every
// operation carries an explicit tenant ID and also uses the request-scoped RLS
// executor, providing two independent tenant boundaries.
type IncidentRepository interface {
	Create(context.Context, string, string, models.IncidentCreateInput) (*models.Incident, error)
	GetByID(context.Context, string, string) (*models.Incident, error)
	Update(context.Context, string, string, string, models.IncidentPatch) (*models.Incident, error)
	Delete(context.Context, string, string, string, int64) error
	List(context.Context, string, models.IncidentListFilter) ([]models.Incident, int, error)
	Transition(context.Context, string, string, string, models.IncidentTransitionInput) (*models.Incident, error)
	Escalate(context.Context, string, string, string, models.IncidentEscalationInput) (*models.Incident, error)
	AssessBreach(context.Context, string, string, string, models.IncidentBreachAssessmentInput) (*models.Incident, error)
	NotifyDPA(context.Context, string, string, string, models.IncidentDPANotificationInput) (*models.Incident, error)
	ListBreachDue(context.Context, string, time.Time, int) ([]models.Incident, error)
	CreateAssignment(context.Context, string, string, string, models.IncidentAssignmentInput) (*models.IncidentAssignment, *models.Incident, error)
	Unassign(context.Context, string, string, string, string, models.IncidentUnassignmentInput) (*models.IncidentAssignment, *models.Incident, error)
	ListAssignments(context.Context, string, string, bool) ([]models.IncidentAssignment, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.IncidentEvent, int, error)
	Statistics(context.Context, string) (*models.IncidentStatistics, error)
}

type incidentRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

func NewIncidentRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (IncidentRepository, error) {
	if pool == nil {
		return nil, errors.New("incident repository database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("incident repository outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("incident repository outbox queue is required")
	}
	return &incidentRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

var _ IncidentRepository = (*incidentRepo)(nil)

const incidentColumns = `
	i.id, i.organization_id, i.incident_ref, i.title, i.description, i.category,
	i.severity, i.status, i.reporter_id, i.assigned_to, i.detected_at, i.reported_at,
	i.occurred_at, i.triaged_at, i.investigation_started_at, i.contained_at,
	i.resolved_at, i.closed_at, i.cancelled_at, i.cancellation_reason, i.reopened_at,
	i.root_cause, i.impact, i.lessons_learned, i.related_asset_id, i.followup_date,
	i.is_data_breach, i.breach_assessment_status, i.is_breach_notifiable,
	i.breach_assessment_reason, i.breach_assessed_at, i.breach_assessed_by,
	i.breach_awareness_at, i.notification_deadline, i.data_subjects_affected,
	i.records_affected, i.data_categories, i.special_category_data, i.cross_border,
	i.breach_nature, i.likely_consequences, i.mitigation_measures,
	i.dpa_notified_at, i.dpa_notification_reference, i.dpa_notification_reason,
	i.version, i.retention_until, i.legal_hold, i.metadata,
	i.created_at, i.updated_at, i.deleted_at`

type incidentScanner interface{ Scan(...any) error }

func scanIncident(row incidentScanner) (*models.Incident, error) {
	item := &models.Incident{}
	var metadata []byte
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.IncidentRef, &item.Title, &item.Description,
		&item.Category, &item.Severity, &item.Status, &item.ReporterID, &item.AssigneeID,
		&item.DetectedAt, &item.ReportedAt, &item.OccurredAt, &item.TriagedAt,
		&item.InvestigationStartedAt, &item.ContainedAt, &item.ResolvedAt, &item.ClosedAt,
		&item.CancelledAt, &item.CancellationReason, &item.ReopenedAt, &item.RootCause,
		&item.Impact, &item.LessonsLearned, &item.RelatedAssetID, &item.FollowupDate,
		&item.IsDataBreach, &item.BreachAssessmentStatus, &item.IsBreachNotifiable,
		&item.BreachAssessmentReason, &item.BreachAssessedAt, &item.BreachAssessedBy,
		&item.BreachAwarenessAt, &item.NotificationDeadline, &item.DataSubjectsAffected,
		&item.RecordsAffected, &item.DataCategories, &item.SpecialCategoryData,
		&item.CrossBorder, &item.BreachNature, &item.LikelyConsequences,
		&item.MitigationMeasures, &item.DPANotifiedAt, &item.DPANotificationReference,
		&item.DPANotificationReason, &item.Version, &item.RetentionUntil, &item.LegalHold,
		&metadata, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	); err != nil {
		return nil, err
	}
	item.Metadata = metadata
	if item.DataCategories == nil {
		item.DataCategories = []string{}
	}
	return item, nil
}

type incidentTransactionStarter interface {
	Begin(context.Context) (pgx.Tx, error)
}

func (r *incidentRepo) begin(ctx context.Context) (pgx.Tx, error) {
	starter, ok := database.QuerierFromContext(ctx, r.pool).(incidentTransactionStarter)
	if !ok {
		return nil, errors.New("incident mutation requires a transactional database executor")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning incident mutation: %w", err)
	}
	return tx, nil
}

func (r *incidentRepo) Create(ctx context.Context, orgID, actorID string, input models.IncidentCreateInput) (*models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var sequence int64
	if err := tx.QueryRow(ctx, `INSERT INTO incident_reference_sequences
		(organization_id,next_incident_number) VALUES ($1::uuid,2)
		ON CONFLICT (organization_id) DO UPDATE SET
			next_incident_number=incident_reference_sequences.next_incident_number+1
		RETURNING next_incident_number-1`, orgID).Scan(&sequence); err != nil {
		return nil, fmt.Errorf("allocating incident reference: %w", err)
	}
	reference := fmt.Sprintf("INC-%06d", sequence)
	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	item, err := scanIncident(tx.QueryRow(ctx, `INSERT INTO incidents AS i (
		organization_id,incident_ref,title,description,category,severity,status,
		reporter_id,detected_at,reported_at,occurred_at,related_asset_id,followup_date,
		retention_until,metadata)
		SELECT $1::uuid,$2,$3,$4,$5,$6,'reported',$7::uuid,$8,NOW(),$9,$10::uuid,$11::date,$12,$13::jsonb
		WHERE EXISTS (SELECT 1 FROM users u WHERE u.id=$7::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL)
		RETURNING `+incidentColumns, orgID, reference, input.Title, input.Description,
		input.Category, input.Severity, actorID, input.DetectedAt, input.OccurredAt,
		input.RelatedAssetID, input.FollowupDate, input.RetentionUntil, metadata))
	if err != nil {
		return nil, fmt.Errorf("creating incident: %w", err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "created", nil, statusPointer(item.Status), "Incident reported", map[string]any{"severity": item.Severity}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing incident creation: %w", err)
	}
	return item, nil
}

func (r *incidentRepo) GetByID(ctx context.Context, orgID, id string) (*models.Incident, error) {
	return scanIncident(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx,
		`SELECT `+incidentColumns+` FROM incidents i
		 WHERE i.organization_id=$1::uuid AND i.id=$2::uuid AND i.deleted_at IS NULL`, orgID, id))
}

func (r *incidentRepo) Update(ctx context.Context, orgID, actorID, id string, patch models.IncidentPatch) (*models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	metadata := patch.Metadata
	if len(metadata) == 0 {
		metadata = nil
	}
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET
		title=COALESCE($5,title), description=COALESCE($6,description),
		category=COALESCE($7,category), severity=COALESCE($8,severity),
		detected_at=COALESCE($9,detected_at),
		occurred_at=CASE WHEN $10 THEN NULL ELSE COALESCE($11,occurred_at) END,
		root_cause=COALESCE($12,root_cause), impact=COALESCE($13,impact),
		lessons_learned=COALESCE($14,lessons_learned),
		related_asset_id=CASE WHEN $15 THEN NULL ELSE COALESCE($16::uuid,related_asset_id) END,
		followup_date=CASE WHEN $17 THEN NULL ELSE COALESCE($18::date,followup_date) END,
		retention_until=COALESCE($19,retention_until), legal_hold=COALESCE($20,legal_hold),
		metadata=COALESCE($21::jsonb,metadata), version=version+1
		WHERE i.organization_id=$1::uuid AND i.id=$2::uuid AND i.deleted_at IS NULL AND i.version=$3
		AND $4::uuid IS NOT NULL
		RETURNING `+incidentColumns, orgID, id, patch.Version, actorID,
		patch.Title, patch.Description, patch.Category, patch.Severity, patch.DetectedAt,
		patch.ClearOccurred, patch.OccurredAt, patch.RootCause, patch.Impact,
		patch.LessonsLearned, patch.ClearAsset, patch.RelatedAssetID,
		patch.ClearFollowup, patch.FollowupDate, patch.RetentionUntil, patch.LegalHold, metadata))
	if err != nil {
		return nil, r.mutationError(ctx, tx, orgID, id, err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "updated", nil, nil, "Incident details updated", map[string]any{"version": item.Version}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing incident update: %w", err)
	}
	return item, nil
}

func (r *incidentRepo) Delete(ctx context.Context, orgID, actorID, id string, version int64) error {
	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET deleted_at=NOW(),version=version+1
		WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL AND version=$3
		RETURNING `+incidentColumns, orgID, id, version))
	if err != nil {
		return r.mutationError(ctx, tx, orgID, id, err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "deleted", nil, nil, "Incident soft-deleted", map[string]any{"version": item.Version}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing incident deletion: %w", err)
	}
	return nil
}

func (r *incidentRepo) List(ctx context.Context, orgID string, filter models.IncidentListFilter) ([]models.Incident, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM incidents i WHERE i.organization_id=$1::uuid AND i.deleted_at IS NULL
		AND ($2='' OR i.status=$2) AND ($3='' OR i.severity=$3)
		AND ($4='' OR i.category=$4) AND ($5='' OR i.assigned_to=$5::uuid)
		AND ($6='' OR i.search_vector @@ websearch_to_tsquery('english',$6))
		AND ($7::boolean IS NULL OR i.is_breach_notifiable=$7::boolean)`
	args := []any{orgID, filter.Status, filter.Severity, filter.Category, filter.AssigneeID, filter.Search, filter.Breach}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting incidents: %w", err)
	}
	sorts := map[string]string{"reported_at": "i.reported_at", "updated_at": "i.updated_at", "severity": "severity_ord(i.severity)", "status": "i.status", "title": "lower(i.title)", "notification_deadline": "i.notification_deadline"}
	sortColumn, ok := sorts[filter.Sort]
	if !ok {
		sortColumn = "i.reported_at"
	}
	direction := "DESC"
	if strings.EqualFold(filter.Direction, "asc") {
		direction = "ASC"
	}
	rows, err := q.Query(ctx, `SELECT `+incidentColumns+where+` ORDER BY `+sortColumn+` `+direction+` NULLS LAST,i.id DESC LIMIT $8 OFFSET $9`, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing incidents: %w", err)
	}
	defer rows.Close()
	items := make([]models.Incident, 0)
	for rows.Next() {
		item, err := scanIncident(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning incident: %w", err)
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *incidentRepo) Transition(ctx context.Context, orgID, actorID, id string, input models.IncidentTransitionInput) (*models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	current, err := scanIncident(tx.QueryRow(ctx, `SELECT `+incidentColumns+` FROM incidents i
		WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, orgID, id))
	if err != nil {
		return nil, err
	}
	if current.Version != input.Version {
		return nil, ErrIncidentVersionConflict
	}
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET status=$4::varchar,
		triaged_at=CASE WHEN $4::varchar='triaged' THEN NOW() ELSE triaged_at END,
		investigation_started_at=CASE WHEN $4::varchar='investigating' THEN NOW() ELSE investigation_started_at END,
		contained_at=CASE WHEN $4::varchar='contained' THEN NOW() ELSE contained_at END,
		resolved_at=CASE WHEN $4::varchar='resolved' THEN NOW() ELSE resolved_at END,
		closed_at=CASE WHEN $4::varchar='closed' THEN NOW() ELSE closed_at END,
		cancelled_at=CASE WHEN $4::varchar='cancelled' THEN NOW() ELSE cancelled_at END,
		cancellation_reason=CASE WHEN $4::varchar='cancelled' THEN $5 ELSE cancellation_reason END,
		reopened_at=CASE WHEN $4::varchar IN ('reported','investigating') AND $6::varchar IN ('resolved','closed','cancelled') THEN NOW() ELSE reopened_at END,
		version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL
		RETURNING `+incidentColumns, orgID, id, input.Version, input.Status, nullIfBlank(input.Reason), current.Status))
	if err != nil {
		return nil, r.mutationError(ctx, tx, orgID, id, err)
	}
	eventType, summary := "status_changed", "Incident status changed"
	if input.Status == models.IncidentStatusCancelled {
		eventType, summary = "cancelled", "Incident cancelled"
	} else if (input.Status == models.IncidentStatusReported || input.Status == models.IncidentStatusInvestigating) && (current.Status == models.IncidentStatusResolved || current.Status == models.IncidentStatusClosed || current.Status == models.IncidentStatusCancelled) {
		eventType, summary = "reopened", "Incident reopened"
	}
	if err := r.recordChange(ctx, tx, item, actorID, eventType, statusPointer(current.Status), statusPointer(item.Status), summary, map[string]any{"reason": input.Reason}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing incident transition: %w", err)
	}
	return item, nil
}

func (r *incidentRepo) Escalate(ctx context.Context, orgID, actorID, id string, input models.IncidentEscalationInput) (*models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var previous models.IncidentSeverity
	if err := tx.QueryRow(ctx, `SELECT severity FROM incidents WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, orgID, id).Scan(&previous); err != nil {
		return nil, err
	}
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET severity=$4,version=version+1
		WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL RETURNING `+incidentColumns,
		orgID, id, input.Version, input.Severity))
	if err != nil {
		return nil, r.mutationError(ctx, tx, orgID, id, err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "escalated", nil, nil, "Incident severity escalated", map[string]any{"from_severity": previous, "to_severity": input.Severity, "reason": input.Reason}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing incident escalation: %w", err)
	}
	return item, nil
}

func (r *incidentRepo) AssessBreach(ctx context.Context, orgID, actorID, id string, input models.IncidentBreachAssessmentInput) (*models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	notifiable := input.Status == models.BreachAssessmentNotifiable
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET
		is_data_breach=$4,breach_assessment_status=$5,is_breach_notifiable=$6,
		breach_assessment_reason=$7,breach_assessed_at=NOW(),breach_assessed_by=$8::uuid,
		breach_awareness_at=CASE WHEN $6 THEN $9::timestamptz ELSE NULL END,
		notification_deadline=CASE WHEN $6 THEN $9::timestamptz + INTERVAL '72 hours' ELSE NULL END,
		data_subjects_affected=$10,records_affected=$11,data_categories=$12::text[],
		special_category_data=$13,cross_border=$14,breach_nature=$15,
		likely_consequences=$16,mitigation_measures=$17,version=version+1
		WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM users u WHERE u.id=$8::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL)
		RETURNING `+incidentColumns, orgID, id, input.Version, input.IsDataBreach, input.Status,
		notifiable, input.Reason, actorID, input.AwarenessAt, input.DataSubjectsAffected,
		input.RecordsAffected, input.DataCategories, input.SpecialCategoryData,
		input.CrossBorder, input.BreachNature, input.LikelyConsequences, input.MitigationMeasures))
	if err != nil {
		return nil, r.mutationError(ctx, tx, orgID, id, err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "breach_assessed", nil, nil, "Personal data breach assessment recorded", map[string]any{"assessment_status": input.Status, "is_data_breach": input.IsDataBreach, "deadline": item.NotificationDeadline}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing breach assessment: %w", err)
	}
	return item, nil
}

func (r *incidentRepo) NotifyDPA(ctx context.Context, orgID, actorID, id string, input models.IncidentDPANotificationInput) (*models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	current, err := scanIncident(tx.QueryRow(ctx, `SELECT `+incidentColumns+` FROM incidents i
		WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, orgID, id))
	if err != nil {
		return nil, err
	}
	if current.DPANotifiedAt != nil {
		var key string
		if err := tx.QueryRow(ctx, `SELECT dpa_notification_key::text FROM incidents WHERE organization_id=$1::uuid AND id=$2::uuid`, orgID, id).Scan(&key); err != nil {
			return nil, err
		}
		if key == input.IdempotencyKey && current.DPANotificationReference != nil && current.DPANotificationReason != nil &&
			*current.DPANotificationReference == input.Reference && *current.DPANotificationReason == input.Reason && current.DPANotifiedAt.Equal(input.NotifiedAt) {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return current, nil
		}
		return nil, ErrIncidentIdempotencyConflict
	}
	if current.Version != input.Version {
		return nil, ErrIncidentVersionConflict
	}
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET dpa_notified_at=$4,
		dpa_notification_reference=$5,dpa_notification_reason=$6,dpa_notification_key=$7::uuid,
		version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3
		AND is_breach_notifiable AND deleted_at IS NULL RETURNING `+incidentColumns,
		orgID, id, input.Version, input.NotifiedAt, input.Reference, input.Reason, input.IdempotencyKey))
	if err != nil {
		return nil, r.mutationError(ctx, tx, orgID, id, err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "dpa_notified", nil, nil, "Supervisory authority notification recorded", map[string]any{"notified_at": input.NotifiedAt, "reference": input.Reference, "reason": input.Reason}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing DPA notification: %w", err)
	}
	return item, nil
}

func (r *incidentRepo) ListBreachDue(ctx context.Context, orgID string, before time.Time, limit int) ([]models.Incident, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT `+incidentColumns+`
		FROM incidents i WHERE organization_id=$1::uuid AND deleted_at IS NULL
		AND is_breach_notifiable AND dpa_notified_at IS NULL AND notification_deadline <= $2
		AND status NOT IN ('closed','cancelled') ORDER BY notification_deadline,reported_at LIMIT $3`, orgID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("listing due breach notifications: %w", err)
	}
	defer rows.Close()
	items := make([]models.Incident, 0)
	for rows.Next() {
		item, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *incidentRepo) CreateAssignment(ctx context.Context, orgID, actorID, incidentID string, input models.IncidentAssignmentInput) (*models.IncidentAssignment, *models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if input.Role == models.IncidentAssignmentPrimary {
		if _, err := tx.Exec(ctx, `UPDATE incident_assignments SET unassigned_at=NOW(),unassigned_by=$3::uuid,
			unassign_reason='Replaced by a new primary assignment' WHERE organization_id=$1::uuid
			AND incident_id=$2::uuid AND assignment_role='primary' AND unassigned_at IS NULL`, orgID, incidentID, actorID); err != nil {
			return nil, nil, fmt.Errorf("replacing primary incident assignment: %w", err)
		}
	}
	assignment := &models.IncidentAssignment{}
	err = tx.QueryRow(ctx, `INSERT INTO incident_assignments (
		organization_id,incident_id,assignee_user_id,assignment_role,assigned_by,reason)
		SELECT $1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6
		WHERE EXISTS (SELECT 1 FROM incidents i WHERE i.id=$2::uuid AND i.organization_id=$1::uuid AND i.deleted_at IS NULL AND i.version=$7)
		AND EXISTS (SELECT 1 FROM users u WHERE u.id=$3::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL)
		AND EXISTS (SELECT 1 FROM users u WHERE u.id=$5::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL)
		RETURNING id,organization_id,incident_id,assignee_user_id,assignment_role,assigned_by,
		reason,assigned_at,unassigned_at,unassigned_by,unassign_reason,created_at,updated_at`,
		orgID, incidentID, input.AssigneeID, input.Role, actorID, input.Reason, input.Version).Scan(
		&assignment.ID, &assignment.OrganizationID, &assignment.IncidentID,
		&assignment.AssigneeUserID, &assignment.Role, &assignment.AssignedBy,
		&assignment.Reason, &assignment.AssignedAt, &assignment.UnassignedAt,
		&assignment.UnassignedBy, &assignment.UnassignReason, &assignment.CreatedAt,
		&assignment.UpdatedAt)
	if err != nil {
		return nil, nil, r.mutationError(ctx, tx, orgID, incidentID, err)
	}
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET
		assigned_to=CASE WHEN $4='primary' THEN $5::uuid ELSE assigned_to END,version=version+1
		WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL RETURNING `+incidentColumns,
		orgID, incidentID, input.Version, input.Role, input.AssigneeID))
	if err != nil {
		return nil, nil, r.mutationError(ctx, tx, orgID, incidentID, err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "assigned", nil, nil, "Incident responder assigned", map[string]any{"assignee_id": input.AssigneeID, "role": input.Role, "reason": input.Reason}); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("committing incident assignment: %w", err)
	}
	return assignment, item, nil
}

func (r *incidentRepo) Unassign(ctx context.Context, orgID, actorID, incidentID, assignmentID string, input models.IncidentUnassignmentInput) (*models.IncidentAssignment, *models.Incident, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	assignment := &models.IncidentAssignment{}
	err = tx.QueryRow(ctx, `UPDATE incident_assignments a SET unassigned_at=NOW(),unassigned_by=$5::uuid,unassign_reason=$6
		WHERE a.organization_id=$1::uuid AND a.incident_id=$2::uuid AND a.id=$3::uuid AND a.unassigned_at IS NULL
		AND EXISTS (SELECT 1 FROM incidents i WHERE i.organization_id=$1::uuid AND i.id=$2::uuid AND i.deleted_at IS NULL AND i.version=$4)
		RETURNING id,organization_id,incident_id,assignee_user_id,assignment_role,assigned_by,
		reason,assigned_at,unassigned_at,unassigned_by,unassign_reason,created_at,updated_at`,
		orgID, incidentID, assignmentID, input.Version, actorID, input.Reason).Scan(
		&assignment.ID, &assignment.OrganizationID, &assignment.IncidentID,
		&assignment.AssigneeUserID, &assignment.Role, &assignment.AssignedBy,
		&assignment.Reason, &assignment.AssignedAt, &assignment.UnassignedAt,
		&assignment.UnassignedBy, &assignment.UnassignReason, &assignment.CreatedAt,
		&assignment.UpdatedAt)
	if err != nil {
		return nil, nil, r.mutationError(ctx, tx, orgID, incidentID, err)
	}
	item, err := scanIncident(tx.QueryRow(ctx, `UPDATE incidents i SET
		assigned_to=CASE WHEN $4='primary' AND assigned_to=$5::uuid THEN NULL ELSE assigned_to END,
		version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL RETURNING `+incidentColumns,
		orgID, incidentID, input.Version, assignment.Role, assignment.AssigneeUserID))
	if err != nil {
		return nil, nil, r.mutationError(ctx, tx, orgID, incidentID, err)
	}
	if err := r.recordChange(ctx, tx, item, actorID, "unassigned", nil, nil, "Incident responder unassigned", map[string]any{"assignee_id": assignment.AssigneeUserID, "role": assignment.Role, "reason": input.Reason}); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("committing incident unassignment: %w", err)
	}
	return assignment, item, nil
}

func (r *incidentRepo) ListAssignments(ctx context.Context, orgID, incidentID string, activeOnly bool) ([]models.IncidentAssignment, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT a.id,a.organization_id,
		a.incident_id,a.assignee_user_id,a.assignment_role,a.assigned_by,a.reason,a.assigned_at,
		a.unassigned_at,a.unassigned_by,a.unassign_reason,a.created_at,a.updated_at
		FROM incident_assignments a JOIN incidents i ON i.id=a.incident_id AND i.organization_id=a.organization_id
		WHERE a.organization_id=$1::uuid AND a.incident_id=$2::uuid AND i.deleted_at IS NULL
		AND (NOT $3 OR a.unassigned_at IS NULL) ORDER BY a.assigned_at DESC,a.id DESC`, orgID, incidentID, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("listing incident assignments: %w", err)
	}
	defer rows.Close()
	items := make([]models.IncidentAssignment, 0)
	for rows.Next() {
		var item models.IncidentAssignment
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.IncidentID,
			&item.AssigneeUserID, &item.Role, &item.AssignedBy, &item.Reason,
			&item.AssignedAt, &item.UnassignedAt, &item.UnassignedBy,
			&item.UnassignReason, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *incidentRepo) ListEvents(ctx context.Context, orgID, incidentID string, pagination models.PaginationRequest) ([]models.IncidentEvent, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM incident_events e JOIN incidents i
		ON i.organization_id=e.organization_id AND i.id=e.incident_id
		WHERE e.organization_id=$1::uuid AND e.incident_id=$2::uuid AND i.deleted_at IS NULL`, orgID, incidentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting incident timeline: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT e.id,e.organization_id,e.incident_id,e.event_type,e.actor_user_id,
		e.from_status,e.to_status,e.summary,e.details,e.occurred_at,e.created_at
		FROM incident_events e JOIN incidents i ON i.organization_id=e.organization_id AND i.id=e.incident_id
		WHERE e.organization_id=$1::uuid AND e.incident_id=$2::uuid AND i.deleted_at IS NULL
		ORDER BY e.occurred_at DESC,e.id DESC LIMIT $3 OFFSET $4`, orgID, incidentID, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing incident timeline: %w", err)
	}
	defer rows.Close()
	items := make([]models.IncidentEvent, 0)
	for rows.Next() {
		var item models.IncidentEvent
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.IncidentID,
			&item.EventType, &item.ActorUserID, &item.FromStatus, &item.ToStatus,
			&item.Summary, &item.Details, &item.OccurredAt, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *incidentRepo) Statistics(ctx context.Context, orgID string) (*models.IncidentStatistics, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	stats := &models.IncidentStatistics{ByStatus: make(map[models.IncidentStatus]int), BySeverity: make(map[models.IncidentSeverity]int)}
	if err := q.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE status NOT IN ('resolved','closed','cancelled')),
		count(*) FILTER (WHERE is_breach_notifiable AND dpa_notified_at IS NULL AND notification_deadline<NOW()),
		count(*) FILTER (WHERE is_breach_notifiable AND dpa_notified_at IS NULL AND notification_deadline BETWEEN NOW() AND NOW()+INTERVAL '24 hours'),
		count(*) FILTER (WHERE dpa_notified_at IS NOT NULL),
		COALESCE(avg(extract(epoch FROM (resolved_at-detected_at))/3600) FILTER (WHERE resolved_at IS NOT NULL),0)
		FROM incidents WHERE organization_id=$1::uuid AND deleted_at IS NULL`, orgID).Scan(
		&stats.Total, &stats.Active, &stats.OverdueBreaches, &stats.UrgentBreaches,
		&stats.NotifiedBreaches, &stats.AverageResolution); err != nil {
		return nil, fmt.Errorf("calculating incident statistics: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT status,severity,count(*) FROM incidents
		WHERE organization_id=$1::uuid AND deleted_at IS NULL GROUP BY status,severity`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status models.IncidentStatus
		var severity models.IncidentSeverity
		var count int
		if err := rows.Scan(&status, &severity, &count); err != nil {
			return nil, err
		}
		stats.ByStatus[status] += count
		stats.BySeverity[severity] += count
	}
	return stats, rows.Err()
}

func (r *incidentRepo) mutationError(ctx context.Context, tx pgx.Tx, orgID, id string, cause error) error {
	if !errors.Is(cause, pgx.ErrNoRows) {
		return cause
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM incidents WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`, orgID, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrIncidentVersionConflict
	}
	return pgx.ErrNoRows
}

func (r *incidentRepo) recordChange(ctx context.Context, tx pgx.Tx, item *models.Incident, actorID, eventType string, fromStatus, toStatus *models.IncidentStatus, summary string, details map[string]any) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encoding incident event details: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO incident_events
		(organization_id,incident_id,event_type,actor_user_id,from_status,to_status,summary,details)
		VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7,$8::jsonb)`, item.OrganizationID,
		item.ID, eventType, actorID, fromStatus, toStatus, summary, detailsJSON); err != nil {
		return fmt.Errorf("appending incident timeline: %w", err)
	}
	payload := map[string]any{
		"type": eventTypeToNotification(eventType), "severity": item.Severity,
		"org_id": item.OrganizationID, "entity_type": "incident", "entity_id": item.ID,
		"entity_ref": item.IncidentRef, "data": map[string]any{
			"incident_id": item.ID, "incident_ref": item.IncidentRef,
			"status": item.Status, "severity": item.Severity,
			"is_data_breach": item.IsDataBreach, "is_breach_notifiable": item.IsBreachNotifiable,
			"notification_deadline": item.NotificationDeadline, "dpa_notified_at": item.DPANotifiedAt,
		}, "timestamp": time.Now().UTC(),
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", item.OrganizationID, payload)
	if err != nil {
		return fmt.Errorf("creating incident outbox envelope: %w", err)
	}
	envelope.CausationID = item.ID
	envelope.Metadata = map[string]string{"entity_type": "incident", "entity_id": item.ID, "event_type": eventType}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueuing incident event: %w", err)
	}
	return nil
}

func eventTypeToNotification(eventType string) string { return "incident." + eventType }

func statusPointer(status models.IncidentStatus) *models.IncidentStatus { return &status }

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
