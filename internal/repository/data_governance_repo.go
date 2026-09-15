package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var (
	ErrDataGovernanceNotFound = errors.New("data governance record not found")
	ErrDataGovernanceConflict = errors.New("data governance version or uniqueness conflict")
	ErrDataGovernanceState    = errors.New("data governance state prevents operation")
	ErrGovernedRecordNotFound = errors.New("governed source record not found")
)

type DataGovernanceRepository interface {
	GetPolicy(context.Context, string) (*models.DataGovernancePolicy, error)
	UpsertPolicy(context.Context, string, string, string, models.DataGovernancePolicyInput) (*models.DataGovernancePolicy, error)
	CreateSchedule(context.Context, string, string, string, models.RetentionScheduleInput) (*models.RetentionSchedule, error)
	GetSchedule(context.Context, string, string) (*models.RetentionSchedule, error)
	ListSchedules(context.Context, string, models.RetentionScheduleFilter) ([]models.RetentionSchedule, int, error)
	UpdateSchedule(context.Context, string, string, string, string, models.RetentionSchedulePatch) (*models.RetentionSchedule, error)
	RetireSchedule(context.Context, string, string, string, string, int64, string) error
	CreateAssignment(context.Context, string, string, string, models.RetentionAssignmentInput) (*models.RecordRetentionAssignment, error)
	GetAssignment(context.Context, string, string) (*models.RecordRetentionAssignment, error)
	GetRecordDisposition(context.Context, string, string, string) (*models.RecordDispositionDecision, error)
	ReviewDisposition(context.Context, string, string, string, string, models.RetentionReviewInput) (*models.RecordRetentionAssignment, error)
	RequestException(context.Context, string, string, string, string, models.RetentionExceptionInput) (*models.RetentionException, error)
	DecideException(context.Context, string, string, string, string, models.RetentionExceptionDecisionInput) (*models.RetentionException, error)
	ListExceptions(context.Context, string, string) ([]models.RetentionException, error)
	CreateLegalHold(context.Context, string, string, string, models.LegalHoldInput) (*models.LegalHold, error)
	GetLegalHold(context.Context, string, string) (*models.LegalHold, error)
	ListLegalHolds(context.Context, string, string, models.PaginationRequest) ([]models.LegalHold, int, error)
	UpdateLegalHold(context.Context, string, string, string, string, models.LegalHoldPatch) (*models.LegalHold, error)
	ReleaseLegalHold(context.Context, string, string, string, string, models.LegalHoldReleaseInput) (*models.LegalHold, error)
	AddLegalHoldRecord(context.Context, string, string, string, string, models.LegalHoldRecordInput) (*models.LegalHoldRecord, error)
	ReleaseLegalHoldRecord(context.Context, string, string, string, string, string, models.LegalHoldRecordReleaseInput) error
	ListLegalHoldRecords(context.Context, string, string, bool) ([]models.LegalHoldRecord, error)
	ListEvents(context.Context, string, models.DataGovernanceEventFilter) ([]models.DataGovernanceEvent, int, error)
	VerifyEventChain(context.Context, string) (*models.GovernanceChainVerification, error)
}

type dataGovernanceRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
	now         func() time.Time
}

func NewDataGovernanceRepository(
	pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string,
) (DataGovernanceRepository, error) {
	if pool == nil {
		return nil, errors.New("data governance database pool is required")
	}
	if outbox == nil || isNilDataGovernanceDependency(outbox) {
		return nil, errors.New("data governance outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("data governance outbox queue is required")
	}
	return &dataGovernanceRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue, now: time.Now}, nil
}

func isNilDataGovernanceDependency(value any) bool {
	reflected := reflect.ValueOf(value)
	return reflected.IsValid() && (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
		reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer ||
		reflected.Kind() == reflect.Slice) && reflected.IsNil()
}

var _ DataGovernanceRepository = (*dataGovernanceRepo)(nil)

const dataGovernancePolicyColumns = `organization_id,primary_region,allowed_regions,
	cross_border_transfer_mode,default_retention_days,default_archive_after_days,
	deletion_grace_days,disposition_approval_mode,require_processor_confirmation,
	legal_hold_enabled,policy_statement,version,metadata,created_by,updated_by,created_at,updated_at`

func (r *dataGovernanceRepo) GetPolicy(ctx context.Context, organizationID string) (*models.DataGovernancePolicy, error) {
	return getDataGovernancePolicy(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, false)
}

func getDataGovernancePolicy(
	ctx context.Context, querier database.Querier, organizationID string, lock bool,
) (*models.DataGovernancePolicy, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	item := new(models.DataGovernancePolicy)
	err := querier.QueryRow(ctx, `SELECT `+dataGovernancePolicyColumns+`
		FROM tenant_data_governance_policies WHERE organization_id=$1::uuid`+suffix, organizationID).Scan(
		&item.OrganizationID, &item.PrimaryRegion, &item.AllowedRegions,
		&item.CrossBorderTransferMode, &item.DefaultRetentionDays, &item.DefaultArchiveAfterDays,
		&item.DeletionGraceDays, &item.DispositionApprovalMode, &item.RequireProcessorConfirmation,
		&item.LegalHoldEnabled, &item.PolicyStatement, &item.Version, &item.Metadata,
		&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDataGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get data governance policy: %w", err)
	}
	if item.AllowedRegions == nil {
		item.AllowedRegions = []string{}
	}
	return item, nil
}

func (r *dataGovernanceRepo) UpsertPolicy(
	ctx context.Context, organizationID, actorID, requestID string, input models.DataGovernancePolicyInput,
) (*models.DataGovernancePolicy, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.DataGovernancePolicy
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getDataGovernancePolicy(ctx, tx, organizationID, true)
		switch {
		case errors.Is(err, ErrDataGovernanceNotFound):
			if input.ExpectedVersion != nil {
				return ErrDataGovernanceConflict
			}
			_, err = tx.Exec(ctx, `INSERT INTO tenant_data_governance_policies (
				organization_id,primary_region,allowed_regions,cross_border_transfer_mode,
				default_retention_days,default_archive_after_days,deletion_grace_days,
				disposition_approval_mode,require_processor_confirmation,legal_hold_enabled,
				policy_statement,metadata,created_by,updated_by
			) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,COALESCE($12::jsonb,'{}'::jsonb),$13::uuid,$13::uuid)`,
				organizationID, input.PrimaryRegion, input.AllowedRegions, input.CrossBorderTransferMode,
				input.DefaultRetentionDays, input.DefaultArchiveAfterDays, input.DeletionGraceDays,
				input.DispositionApprovalMode, input.RequireProcessorConfirmation, input.LegalHoldEnabled,
				input.PolicyStatement, nullableJSON(input.Metadata), actorID,
			)
			if err != nil {
				return mapDataGovernanceWriteError("insert data governance policy", err)
			}
		case err != nil:
			return err
		default:
			if input.ExpectedVersion == nil || *input.ExpectedVersion != before.Version {
				return ErrDataGovernanceConflict
			}
			tag, err := tx.Exec(ctx, `UPDATE tenant_data_governance_policies SET
				primary_region=$3,allowed_regions=$4,cross_border_transfer_mode=$5,
				default_retention_days=$6,default_archive_after_days=$7,deletion_grace_days=$8,
				disposition_approval_mode=$9,require_processor_confirmation=$10,
				legal_hold_enabled=$11,policy_statement=$12,
				metadata=COALESCE($13::jsonb,'{}'::jsonb),updated_by=$14::uuid,version=version+1
				WHERE organization_id=$1::uuid AND version=$2`,
				organizationID, *input.ExpectedVersion, input.PrimaryRegion, input.AllowedRegions,
				input.CrossBorderTransferMode, input.DefaultRetentionDays, input.DefaultArchiveAfterDays,
				input.DeletionGraceDays, input.DispositionApprovalMode,
				input.RequireProcessorConfirmation, input.LegalHoldEnabled, input.PolicyStatement,
				nullableJSON(input.Metadata), actorID,
			)
			if err != nil {
				return mapDataGovernanceWriteError("update data governance policy", err)
			}
			if tag.RowsAffected() != 1 {
				return ErrDataGovernanceConflict
			}
		}
		result, err = getDataGovernancePolicy(ctx, tx, organizationID, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "policy", organizationID, "policy_saved",
			actorID, input.Reason, before, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

const retentionScheduleColumns = `id,organization_id,name,description,record_type,
	COALESCE(data_classification,''),COALESCE(jurisdiction,''),trigger_event,legal_basis,
	retention_days,archive_after_days,disposition_action,review_required,priority,status,
	effective_from,effective_until,version,created_by,updated_by,created_at,updated_at,deleted_at`

func (r *dataGovernanceRepo) CreateSchedule(
	ctx context.Context, organizationID, actorID, requestID string, input models.RetentionScheduleInput,
) (*models.RetentionSchedule, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.RetentionSchedule
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO retention_schedules (
			organization_id,name,description,record_type,data_classification,jurisdiction,
			trigger_event,legal_basis,retention_days,archive_after_days,disposition_action,
			review_required,priority,status,effective_from,effective_until,created_by,updated_by
		) VALUES ($1::uuid,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::uuid,$17::uuid)
		RETURNING id`, organizationID, input.Name, input.Description, input.RecordType,
			input.DataClassification, input.Jurisdiction, input.TriggerEvent, input.LegalBasis,
			input.RetentionDays, input.ArchiveAfterDays, input.DispositionAction,
			input.ReviewRequired, input.Priority, input.Status, input.EffectiveFrom,
			input.EffectiveUntil, actorID).Scan(&id); err != nil {
			return mapDataGovernanceWriteError("insert retention schedule", err)
		}
		var err error
		result, err = getRetentionSchedule(ctx, tx, organizationID, id, false, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "retention_schedule", id, "schedule_created",
			actorID, input.Reason, nil, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) GetSchedule(ctx context.Context, organizationID, id string) (*models.RetentionSchedule, error) {
	return getRetentionSchedule(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, id, false, false)
}

func getRetentionSchedule(
	ctx context.Context, querier database.Querier, organizationID, id string, lock, includeDeleted bool,
) (*models.RetentionSchedule, error) {
	deletedClause := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedClause = ""
	}
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE"
	}
	item, err := scanRetentionSchedule(querier.QueryRow(ctx, `SELECT `+retentionScheduleColumns+`
		FROM retention_schedules WHERE organization_id=$1::uuid AND id=$2::uuid`+deletedClause+lockClause,
		organizationID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDataGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get retention schedule: %w", err)
	}
	return item, nil
}

func scanRetentionSchedule(scanner interface{ Scan(...any) error }) (*models.RetentionSchedule, error) {
	item := new(models.RetentionSchedule)
	if err := scanner.Scan(
		&item.ID, &item.OrganizationID, &item.Name, &item.Description, &item.RecordType,
		&item.DataClassification, &item.Jurisdiction, &item.TriggerEvent, &item.LegalBasis,
		&item.RetentionDays, &item.ArchiveAfterDays, &item.DispositionAction,
		&item.ReviewRequired, &item.Priority, &item.Status, &item.EffectiveFrom,
		&item.EffectiveUntil, &item.Version, &item.CreatedBy, &item.UpdatedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *dataGovernanceRepo) ListSchedules(
	ctx context.Context, organizationID string, filter models.RetentionScheduleFilter,
) ([]models.RetentionSchedule, int, error) {
	where := []string{"organization_id=$1::uuid", "deleted_at IS NULL"}
	args := []any{organizationID}
	add := func(value, clause string) {
		if value != "" {
			args = append(args, value)
			where = append(where, fmt.Sprintf(clause, len(args)))
		}
	}
	add(filter.RecordType, "record_type=$%d")
	add(filter.Status, "status=$%d")
	add(filter.DataClassification, "data_classification=$%d")
	add(filter.Jurisdiction, "jurisdiction=$%d")
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where = append(where, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d OR legal_basis ILIKE $%d)", len(args), len(args), len(args)))
	}
	condition := strings.Join(where, " AND ")
	var total int
	if err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx,
		`SELECT COUNT(*) FROM retention_schedules WHERE `+condition, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count retention schedules: %w", err)
	}
	sortColumns := map[string]string{
		"name": "name", "record_type": "record_type", "retention_days": "retention_days",
		"priority": "priority", "effective_from": "effective_from", "updated_at": "updated_at",
	}
	sortColumn := sortColumns[filter.SortBy]
	if sortColumn == "" {
		sortColumn = "priority"
	}
	direction := "ASC"
	if strings.EqualFold(filter.SortDirection, "desc") {
		direction = "DESC"
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT `+retentionScheduleColumns+`
		FROM retention_schedules WHERE `+condition+` ORDER BY `+sortColumn+` `+direction+`,id
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list retention schedules: %w", err)
	}
	defer rows.Close()
	items := make([]models.RetentionSchedule, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanRetentionSchedule(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan retention schedule: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate retention schedules: %w", err)
	}
	return items, total, nil
}

func (r *dataGovernanceRepo) UpdateSchedule(
	ctx context.Context, organizationID, id, actorID, requestID string, patch models.RetentionSchedulePatch,
) (*models.RetentionSchedule, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.RetentionSchedule
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getRetentionSchedule(ctx, tx, organizationID, id, true, false)
		if err != nil {
			return err
		}
		if before.Version != patch.ExpectedVersion {
			return ErrDataGovernanceConflict
		}
		sets := []string{"version=version+1", "updated_by=$4::uuid"}
		args := []any{organizationID, id, patch.ExpectedVersion, actorID}
		add := func(column, expression string, value any) {
			args = append(args, value)
			sets = append(sets, fmt.Sprintf("%s="+expression, column, len(args)))
		}
		if patch.Name != nil {
			add("name", "$%d", *patch.Name)
		}
		if patch.Description != nil {
			add("description", "$%d", *patch.Description)
		}
		if patch.ClearClassification {
			sets = append(sets, "data_classification=NULL")
		} else if patch.DataClassification != nil {
			add("data_classification", "NULLIF($%d,'')", *patch.DataClassification)
		}
		if patch.ClearJurisdiction {
			sets = append(sets, "jurisdiction=NULL")
		} else if patch.Jurisdiction != nil {
			add("jurisdiction", "NULLIF($%d,'')", *patch.Jurisdiction)
		}
		if patch.TriggerEvent != nil {
			add("trigger_event", "$%d", *patch.TriggerEvent)
		}
		if patch.LegalBasis != nil {
			add("legal_basis", "$%d", *patch.LegalBasis)
		}
		if patch.RetentionDays != nil {
			add("retention_days", "$%d", *patch.RetentionDays)
		}
		if patch.ClearArchiveAfter {
			sets = append(sets, "archive_after_days=NULL")
		} else if patch.ArchiveAfterDays != nil {
			add("archive_after_days", "$%d", *patch.ArchiveAfterDays)
		}
		if patch.DispositionAction != nil {
			add("disposition_action", "$%d", *patch.DispositionAction)
		}
		if patch.ReviewRequired != nil {
			add("review_required", "$%d", *patch.ReviewRequired)
		}
		if patch.Priority != nil {
			add("priority", "$%d", *patch.Priority)
		}
		if patch.Status != nil {
			add("status", "$%d", *patch.Status)
		}
		if patch.EffectiveFrom != nil {
			add("effective_from", "$%d", *patch.EffectiveFrom)
		}
		if patch.ClearEffectiveUntil {
			sets = append(sets, "effective_until=NULL")
		} else if patch.EffectiveUntil != nil {
			add("effective_until", "$%d", *patch.EffectiveUntil)
		}
		tag, err := tx.Exec(ctx, `UPDATE retention_schedules SET `+strings.Join(sets, ",")+`
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`, args...)
		if err != nil {
			return mapDataGovernanceWriteError("update retention schedule", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDataGovernanceConflict
		}
		result, err = getRetentionSchedule(ctx, tx, organizationID, id, false, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "retention_schedule", id, "schedule_updated",
			actorID, patch.Reason, before, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) RetireSchedule(
	ctx context.Context, organizationID, id, actorID, requestID string, expectedVersion int64, reason string,
) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getRetentionSchedule(ctx, tx, organizationID, id, true, false)
		if err != nil {
			return err
		}
		if before.Version != expectedVersion {
			return ErrDataGovernanceConflict
		}
		if before.Status == "retired" {
			return ErrDataGovernanceState
		}
		tag, err := tx.Exec(ctx, `UPDATE retention_schedules SET status='retired',deleted_at=NOW(),
			updated_by=$4::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`,
			organizationID, id, expectedVersion, actorID)
		if err != nil {
			return fmt.Errorf("retire retention schedule: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDataGovernanceConflict
		}
		after, err := getRetentionSchedule(ctx, tx, organizationID, id, false, true)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "retention_schedule", id, "schedule_retired",
			actorID, reason, before, after, requestID)
	})
}

const retentionAssignmentColumns = `id,organization_id,schedule_id,record_type,record_id,
	COALESCE(data_classification,''),COALESCE(jurisdiction,''),trigger_event,
	retention_started_at,archive_eligible_at,disposition_due_at,disposition_action,
	review_required,review_status,reviewed_by,reviewed_at,COALESCE(review_reason,''),
	state,source,reason,version,created_by,created_at,updated_at,disposed_at`

func (r *dataGovernanceRepo) CreateAssignment(
	ctx context.Context, organizationID, actorID, requestID string, input models.RetentionAssignmentInput,
) (*models.RecordRetentionAssignment, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.RecordRetentionAssignment
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		schedule, err := getRetentionSchedule(ctx, tx, organizationID, input.ScheduleID, true, false)
		if err != nil {
			return err
		}
		if schedule.Status != "active" || schedule.RecordType != input.RecordType {
			return ErrDataGovernanceState
		}
		if schedule.DataClassification != "" && schedule.DataClassification != input.DataClassification {
			return ErrDataGovernanceState
		}
		if schedule.Jurisdiction != "" && schedule.Jurisdiction != input.Jurisdiction {
			return ErrDataGovernanceState
		}
		date := input.RetentionStartedAt.UTC()
		if date.Before(schedule.EffectiveFrom) ||
			(schedule.EffectiveUntil != nil && date.After(schedule.EffectiveUntil.Add(24*time.Hour))) {
			return ErrDataGovernanceState
		}
		if err := ensureGovernedRecord(ctx, tx, organizationID, input.RecordType, input.RecordID); err != nil {
			return err
		}
		var archiveAt *time.Time
		if schedule.ArchiveAfterDays != nil {
			value := date.AddDate(0, 0, *schedule.ArchiveAfterDays)
			archiveAt = &value
		}
		dueAt := date.AddDate(0, 0, schedule.RetentionDays)
		reviewStatus := "pending"
		if !schedule.ReviewRequired {
			reviewStatus = "not_required"
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO record_retention_assignments (
			organization_id,schedule_id,record_type,record_id,data_classification,jurisdiction,
			trigger_event,retention_started_at,archive_eligible_at,disposition_due_at,
			disposition_action,review_required,review_status,source,reason,created_by
		) VALUES ($1::uuid,$2::uuid,$3,$4::uuid,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::uuid)
		RETURNING id`, organizationID, input.ScheduleID, input.RecordType, input.RecordID,
			input.DataClassification, input.Jurisdiction, schedule.TriggerEvent, date, archiveAt,
			dueAt, schedule.DispositionAction, schedule.ReviewRequired, reviewStatus,
			input.Source, input.Reason, actorID).Scan(&id); err != nil {
			return mapDataGovernanceWriteError("insert record retention assignment", err)
		}
		result, err = getRetentionAssignment(ctx, tx, organizationID, id, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "retention_assignment", id, "assignment_created",
			actorID, input.Reason, nil, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) GetAssignment(
	ctx context.Context, organizationID, id string,
) (*models.RecordRetentionAssignment, error) {
	return getRetentionAssignment(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, id, false)
}

func getRetentionAssignment(
	ctx context.Context, querier database.Querier, organizationID, id string, lock bool,
) (*models.RecordRetentionAssignment, error) {
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE"
	}
	item, err := scanRetentionAssignment(querier.QueryRow(ctx, `SELECT `+retentionAssignmentColumns+`
		FROM record_retention_assignments WHERE organization_id=$1::uuid AND id=$2::uuid`+lockClause,
		organizationID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDataGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get retention assignment: %w", err)
	}
	return item, nil
}

func getRetentionAssignmentForRecord(
	ctx context.Context, querier database.Querier, organizationID, recordType, recordID string,
) (*models.RecordRetentionAssignment, error) {
	item, err := scanRetentionAssignment(querier.QueryRow(ctx, `SELECT `+retentionAssignmentColumns+`
		FROM record_retention_assignments
		WHERE organization_id=$1::uuid AND record_type=$2 AND record_id=$3::uuid
		ORDER BY (state <> 'disposed') DESC,created_at DESC LIMIT 1`, organizationID, recordType, recordID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDataGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get record retention assignment: %w", err)
	}
	return item, nil
}

func scanRetentionAssignment(scanner interface{ Scan(...any) error }) (*models.RecordRetentionAssignment, error) {
	item := new(models.RecordRetentionAssignment)
	if err := scanner.Scan(
		&item.ID, &item.OrganizationID, &item.ScheduleID, &item.RecordType, &item.RecordID,
		&item.DataClassification, &item.Jurisdiction, &item.TriggerEvent,
		&item.RetentionStartedAt, &item.ArchiveEligibleAt, &item.DispositionDueAt,
		&item.DispositionAction, &item.ReviewRequired, &item.ReviewStatus,
		&item.ReviewedBy, &item.ReviewedAt, &item.ReviewReason, &item.State, &item.Source,
		&item.Reason, &item.Version, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
		&item.DisposedAt,
	); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *dataGovernanceRepo) GetRecordDisposition(
	ctx context.Context, organizationID, recordType, recordID string,
) (*models.RecordDispositionDecision, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	if err := ensureGovernedRecord(ctx, querier, organizationID, recordType, recordID); err != nil {
		return nil, err
	}
	decision := &models.RecordDispositionDecision{
		RecordType: recordType, RecordID: recordID, ActiveLegalHolds: []models.LegalHoldRecord{},
		EvaluatedAt: r.now().UTC(),
	}
	assignment, err := getRetentionAssignmentForRecord(ctx, querier, organizationID, recordType, recordID)
	if err != nil && !errors.Is(err, ErrDataGovernanceNotFound) {
		return nil, err
	}
	decision.Assignment = assignment
	holds, err := listLegalHoldRecordsForRecord(ctx, querier, organizationID, recordType, recordID)
	if err != nil {
		return nil, err
	}
	decision.ActiveLegalHolds = holds
	switch {
	case len(holds) > 0:
		decision.Reason = "active_legal_hold"
	case assignment == nil:
		decision.Reason = "retention_assignment_missing"
	case assignment.State == "disposed":
		decision.Allowed = true
		decision.Reason = "disposition_already_authorized"
	case assignment.State == "exception":
		decision.Reason = "retention_exception_pending"
	case assignment.DispositionDueAt.After(decision.EvaluatedAt):
		decision.Reason = "retention_period_active"
	case assignment.ReviewRequired && assignment.ReviewStatus != "approved":
		decision.Reason = "disposition_review_required"
	default:
		decision.Allowed = true
		decision.Reason = "eligible_for_disposition"
	}
	return decision, nil
}

func (r *dataGovernanceRepo) ReviewDisposition(
	ctx context.Context, organizationID, id, actorID, requestID string, input models.RetentionReviewInput,
) (*models.RecordRetentionAssignment, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.RecordRetentionAssignment
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getRetentionAssignment(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		if before.Version != input.ExpectedVersion || before.State == "disposed" {
			return ErrDataGovernanceConflict
		}
		status, state, eventType := "rejected", "active", "disposition_rejected"
		if input.Decision == "approve" {
			status, state, eventType = "approved", "disposition_pending", "disposition_approved"
		}
		tag, err := tx.Exec(ctx, `UPDATE record_retention_assignments SET
			review_status=$4,reviewed_by=$5::uuid,reviewed_at=NOW(),review_reason=$6,
			state=$7,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3`,
			organizationID, id, input.ExpectedVersion, status, actorID, input.Reason, state)
		if err != nil {
			return mapDataGovernanceWriteError("review record disposition", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDataGovernanceConflict
		}
		result, err = getRetentionAssignment(ctx, tx, organizationID, id, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "retention_assignment", id, eventType,
			actorID, input.Reason, before, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

const retentionExceptionColumns = `id,organization_id,assignment_id,requested_until,reason,
	status,requested_by,decided_by,decided_at,COALESCE(decision_reason,''),version,created_at,updated_at`

func (r *dataGovernanceRepo) RequestException(
	ctx context.Context, organizationID, assignmentID, actorID, requestID string, input models.RetentionExceptionInput,
) (*models.RetentionException, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.RetentionException
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		assignment, err := getRetentionAssignment(ctx, tx, organizationID, assignmentID, true)
		if err != nil {
			return err
		}
		if assignment.State == "disposed" || !input.RequestedUntil.After(assignment.DispositionDueAt) {
			return ErrDataGovernanceState
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO retention_exceptions (
			organization_id,assignment_id,requested_until,reason,requested_by
		) VALUES ($1::uuid,$2::uuid,$3,$4,$5::uuid) RETURNING id`, organizationID,
			assignmentID, input.RequestedUntil.UTC(), input.Reason, actorID).Scan(&id); err != nil {
			return mapDataGovernanceWriteError("request retention exception", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE record_retention_assignments
			SET state='exception',version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid`,
			organizationID, assignmentID); err != nil {
			return fmt.Errorf("mark retention assignment exception pending: %w", err)
		}
		result, err = getRetentionException(ctx, tx, organizationID, id, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "retention_exception", id, "exception_requested",
			actorID, input.Reason, nil, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) DecideException(
	ctx context.Context, organizationID, id, actorID, requestID string,
	input models.RetentionExceptionDecisionInput,
) (*models.RetentionException, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.RetentionException
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getRetentionException(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		if before.Status != "pending" || before.Version != input.ExpectedVersion {
			return ErrDataGovernanceConflict
		}
		status, eventType := "rejected", "exception_rejected"
		if input.Decision == "approve" {
			status, eventType = "approved", "exception_approved"
		}
		tag, err := tx.Exec(ctx, `UPDATE retention_exceptions SET status=$4,decided_by=$5::uuid,
			decided_at=NOW(),decision_reason=$6,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND status='pending'`,
			organizationID, id, input.ExpectedVersion, status, actorID, input.Reason)
		if err != nil {
			return mapDataGovernanceWriteError("decide retention exception", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDataGovernanceConflict
		}
		if status == "approved" {
			_, err = tx.Exec(ctx, `UPDATE record_retention_assignments SET
				disposition_due_at=$3,state='active',review_status=CASE WHEN review_required THEN 'pending' ELSE 'not_required' END,
				reviewed_by=NULL,reviewed_at=NULL,review_reason=NULL,version=version+1
				WHERE organization_id=$1::uuid AND id=$2::uuid`,
				organizationID, before.AssignmentID, before.RequestedUntil)
		} else {
			_, err = tx.Exec(ctx, `UPDATE record_retention_assignments SET state='active',version=version+1
				WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, before.AssignmentID)
		}
		if err != nil {
			return fmt.Errorf("apply retention exception decision: %w", err)
		}
		result, err = getRetentionException(ctx, tx, organizationID, id, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "retention_exception", id, eventType,
			actorID, input.Reason, before, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) ListExceptions(
	ctx context.Context, organizationID, assignmentID string,
) ([]models.RetentionException, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT `+retentionExceptionColumns+`
		FROM retention_exceptions WHERE organization_id=$1::uuid AND assignment_id=$2::uuid
		ORDER BY created_at DESC,id DESC`, organizationID, assignmentID)
	if err != nil {
		return nil, fmt.Errorf("list retention exceptions: %w", err)
	}
	defer rows.Close()
	items := make([]models.RetentionException, 0)
	for rows.Next() {
		item, err := scanRetentionException(rows)
		if err != nil {
			return nil, fmt.Errorf("scan retention exception: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retention exceptions: %w", err)
	}
	return items, nil
}

func getRetentionException(
	ctx context.Context, querier database.Querier, organizationID, id string, lock bool,
) (*models.RetentionException, error) {
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE"
	}
	item, err := scanRetentionException(querier.QueryRow(ctx, `SELECT `+retentionExceptionColumns+`
		FROM retention_exceptions WHERE organization_id=$1::uuid AND id=$2::uuid`+lockClause,
		organizationID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDataGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get retention exception: %w", err)
	}
	return item, nil
}

func scanRetentionException(scanner interface{ Scan(...any) error }) (*models.RetentionException, error) {
	item := new(models.RetentionException)
	if err := scanner.Scan(
		&item.ID, &item.OrganizationID, &item.AssignmentID, &item.RequestedUntil,
		&item.Reason, &item.Status, &item.RequestedBy, &item.DecidedBy, &item.DecidedAt,
		&item.DecisionReason, &item.Version, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return item, nil
}

const legalHoldColumns = `h.id,h.organization_id,h.hold_ref,h.name,
	COALESCE(h.matter_reference,''),h.description,h.legal_authority,h.scope,h.status,
	h.owner_user_id,h.placed_by,h.placed_at,h.review_due_at,h.released_by,h.released_at,
	COALESCE(h.release_reason,''),h.version,h.created_at,h.updated_at,
	COALESCE((SELECT array_agg(c.user_id::text ORDER BY c.user_id::text)
		FROM legal_hold_custodians c WHERE c.organization_id=h.organization_id
		AND c.hold_id=h.id AND c.released_at IS NULL),'{}'::text[]),
	(SELECT COUNT(*) FROM legal_hold_records r WHERE r.organization_id=h.organization_id
		AND r.hold_id=h.id AND r.released_at IS NULL)`

func (r *dataGovernanceRepo) CreateLegalHold(
	ctx context.Context, organizationID, actorID, requestID string, input models.LegalHoldInput,
) (*models.LegalHold, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.LegalHold
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := ensureGovernanceUser(ctx, tx, organizationID, input.OwnerUserID); err != nil {
			return err
		}
		for _, id := range input.CustodianIDs {
			if err := ensureGovernanceUser(ctx, tx, organizationID, id); err != nil {
				return err
			}
		}
		var sequence int64
		if err := tx.QueryRow(ctx, `INSERT INTO legal_hold_reference_sequences (organization_id,next_value)
			VALUES ($1::uuid,2) ON CONFLICT (organization_id)
			DO UPDATE SET next_value=legal_hold_reference_sequences.next_value+1
			RETURNING next_value-1`, organizationID).Scan(&sequence); err != nil {
			return fmt.Errorf("allocate legal hold reference: %w", err)
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO legal_holds (
			organization_id,hold_ref,name,matter_reference,description,legal_authority,scope,
			owner_user_id,placed_by,review_due_at
		) VALUES ($1::uuid,$2,$3,NULLIF($4,''),$5,$6,COALESCE($7::jsonb,'{}'::jsonb),$8::uuid,$9::uuid,$10)
		RETURNING id`, organizationID, fmt.Sprintf("HOLD-%06d", sequence), input.Name,
			input.MatterReference, input.Description, input.LegalAuthority, nullableJSON(input.Scope),
			input.OwnerUserID, actorID, input.ReviewDueAt).Scan(&id); err != nil {
			return mapDataGovernanceWriteError("insert legal hold", err)
		}
		for _, custodianID := range input.CustodianIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO legal_hold_custodians
				(organization_id,hold_id,user_id,added_by) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid)`,
				organizationID, id, custodianID, actorID); err != nil {
				return mapDataGovernanceWriteError("insert legal hold custodian", err)
			}
		}
		var err error
		result, err = getLegalHold(ctx, tx, organizationID, id, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "legal_hold", id, "hold_created",
			actorID, input.Reason, nil, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) GetLegalHold(
	ctx context.Context, organizationID, id string,
) (*models.LegalHold, error) {
	return getLegalHold(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, id, false)
}

func getLegalHold(
	ctx context.Context, querier database.Querier, organizationID, id string, lock bool,
) (*models.LegalHold, error) {
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE OF h"
	}
	item, err := scanLegalHold(querier.QueryRow(ctx, `SELECT `+legalHoldColumns+`
		FROM legal_holds h WHERE h.organization_id=$1::uuid AND h.id=$2::uuid`+lockClause,
		organizationID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDataGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get legal hold: %w", err)
	}
	return item, nil
}

func scanLegalHold(scanner interface{ Scan(...any) error }) (*models.LegalHold, error) {
	item := new(models.LegalHold)
	if err := scanner.Scan(
		&item.ID, &item.OrganizationID, &item.HoldRef, &item.Name, &item.MatterReference,
		&item.Description, &item.LegalAuthority, &item.Scope, &item.Status,
		&item.OwnerUserID, &item.PlacedBy, &item.PlacedAt, &item.ReviewDueAt,
		&item.ReleasedBy, &item.ReleasedAt, &item.ReleaseReason, &item.Version,
		&item.CreatedAt, &item.UpdatedAt, &item.CustodianIDs, &item.RecordCount,
	); err != nil {
		return nil, err
	}
	if item.CustodianIDs == nil {
		item.CustodianIDs = []string{}
	}
	return item, nil
}

func (r *dataGovernanceRepo) ListLegalHolds(
	ctx context.Context, organizationID, status string, pagination models.PaginationRequest,
) ([]models.LegalHold, int, error) {
	args := []any{organizationID}
	condition := "h.organization_id=$1::uuid"
	if status != "" {
		args = append(args, status)
		condition += " AND h.status=$2"
	}
	var total int
	if err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx,
		`SELECT COUNT(*) FROM legal_holds h WHERE `+condition, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count legal holds: %w", err)
	}
	args = append(args, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT `+legalHoldColumns+`
		FROM legal_holds h WHERE `+condition+` ORDER BY h.placed_at DESC,h.id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list legal holds: %w", err)
	}
	defer rows.Close()
	items := make([]models.LegalHold, 0, pagination.PageSize)
	for rows.Next() {
		item, err := scanLegalHold(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan legal hold: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate legal holds: %w", err)
	}
	return items, total, nil
}

func (r *dataGovernanceRepo) UpdateLegalHold(
	ctx context.Context, organizationID, id, actorID, requestID string, patch models.LegalHoldPatch,
) (*models.LegalHold, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.LegalHold
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getLegalHold(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		if before.Version != patch.ExpectedVersion {
			return ErrDataGovernanceConflict
		}
		if before.Status != "active" {
			return ErrDataGovernanceState
		}
		sets := []string{"version=version+1"}
		args := []any{organizationID, id, patch.ExpectedVersion}
		add := func(column, expression string, value any) {
			args = append(args, value)
			sets = append(sets, fmt.Sprintf("%s="+expression, column, len(args)))
		}
		if patch.Name != nil {
			add("name", "$%d", *patch.Name)
		}
		if patch.ClearMatterReference {
			sets = append(sets, "matter_reference=NULL")
		} else if patch.MatterReference != nil {
			add("matter_reference", "NULLIF($%d,'')", *patch.MatterReference)
		}
		if patch.Description != nil {
			add("description", "$%d", *patch.Description)
		}
		if patch.LegalAuthority != nil {
			add("legal_authority", "$%d", *patch.LegalAuthority)
		}
		if patch.Scope != nil {
			add("scope", "$%d::jsonb", nullableJSON(*patch.Scope))
		}
		if patch.OwnerUserID != nil {
			if err := ensureGovernanceUser(ctx, tx, organizationID, *patch.OwnerUserID); err != nil {
				return err
			}
			add("owner_user_id", "$%d::uuid", *patch.OwnerUserID)
		}
		if patch.ClearReviewDue {
			sets = append(sets, "review_due_at=NULL")
		} else if patch.ReviewDueAt != nil {
			add("review_due_at", "$%d", *patch.ReviewDueAt)
		}
		tag, err := tx.Exec(ctx, `UPDATE legal_holds SET `+strings.Join(sets, ",")+`
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3`, args...)
		if err != nil {
			return mapDataGovernanceWriteError("update legal hold", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDataGovernanceConflict
		}
		result, err = getLegalHold(ctx, tx, organizationID, id, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "legal_hold", id, "hold_updated",
			actorID, patch.Reason, before, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) ReleaseLegalHold(
	ctx context.Context, organizationID, id, actorID, requestID string, input models.LegalHoldReleaseInput,
) (*models.LegalHold, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.LegalHold
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getLegalHold(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		if before.Version != input.ExpectedVersion {
			return ErrDataGovernanceConflict
		}
		if before.Status != "active" {
			return ErrDataGovernanceState
		}
		status, eventType := "released", "hold_released"
		if input.Outcome == "cancel" {
			status, eventType = "cancelled", "hold_cancelled"
		}
		// Release preservation targets before closing the parent hold. Migration
		// 056 enforces this ordering so a direct status rewrite cannot silently
		// bypass an active record hold.
		if _, err := tx.Exec(ctx, `UPDATE legal_hold_records SET released_at=NOW(),released_by=$3::uuid,
			release_reason=$4 WHERE organization_id=$1::uuid AND hold_id=$2::uuid AND released_at IS NULL`,
			organizationID, id, actorID, input.Reason); err != nil {
			return fmt.Errorf("release legal hold records: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE legal_hold_custodians SET released_at=NOW(),released_by=$3::uuid,
			release_reason=$4 WHERE organization_id=$1::uuid AND hold_id=$2::uuid AND released_at IS NULL`,
			organizationID, id, actorID, input.Reason); err != nil {
			return fmt.Errorf("release legal hold custodians: %w", err)
		}
		tag, err := tx.Exec(ctx, `UPDATE legal_holds SET status=$4,released_by=$5::uuid,
			released_at=NOW(),release_reason=$6,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND status='active'`,
			organizationID, id, input.ExpectedVersion, status, actorID, input.Reason)
		if err != nil {
			return mapDataGovernanceWriteError("release legal hold", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDataGovernanceConflict
		}
		result, err = getLegalHold(ctx, tx, organizationID, id, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "legal_hold", id, eventType,
			actorID, input.Reason, before, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

const legalHoldRecordColumns = `id,organization_id,hold_id,record_type,record_id,reason,
	placed_by,placed_at,released_at,released_by,COALESCE(release_reason,'')`

func (r *dataGovernanceRepo) AddLegalHoldRecord(
	ctx context.Context, organizationID, holdID, actorID, requestID string, input models.LegalHoldRecordInput,
) (*models.LegalHoldRecord, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.LegalHoldRecord
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		hold, err := getLegalHold(ctx, tx, organizationID, holdID, true)
		if err != nil {
			return err
		}
		if hold.Status != "active" {
			return ErrDataGovernanceState
		}
		if err := ensureGovernedRecord(ctx, tx, organizationID, input.RecordType, input.RecordID); err != nil {
			return err
		}
		result = new(models.LegalHoldRecord)
		if err := tx.QueryRow(ctx, `INSERT INTO legal_hold_records
			(organization_id,hold_id,record_type,record_id,reason,placed_by)
			VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6::uuid)
			RETURNING `+legalHoldRecordColumns, organizationID, holdID, input.RecordType,
			input.RecordID, input.Reason, actorID).Scan(
			&result.ID, &result.OrganizationID, &result.HoldID, &result.RecordType,
			&result.RecordID, &result.Reason, &result.PlacedBy, &result.PlacedAt,
			&result.ReleasedAt, &result.ReleasedBy, &result.ReleaseReason,
		); err != nil {
			return mapDataGovernanceWriteError("add legal hold record", err)
		}
		return r.appendEvent(ctx, tx, organizationID, "legal_hold", holdID, "hold_record_added",
			actorID, input.Reason, nil, result, requestID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *dataGovernanceRepo) ReleaseLegalHoldRecord(
	ctx context.Context, organizationID, holdID, recordID, actorID, requestID string,
	input models.LegalHoldRecordReleaseInput,
) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if _, err := getLegalHold(ctx, tx, organizationID, holdID, true); err != nil {
			return err
		}
		before, err := getLegalHoldRecord(ctx, tx, organizationID, holdID, recordID, true)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE legal_hold_records SET released_at=NOW(),released_by=$4::uuid,
			release_reason=$5 WHERE organization_id=$1::uuid AND hold_id=$2::uuid AND id=$3::uuid
			AND released_at IS NULL`, organizationID, holdID, recordID, actorID, input.Reason)
		if err != nil {
			return mapDataGovernanceWriteError("release legal hold record", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDataGovernanceConflict
		}
		after, err := getLegalHoldRecord(ctx, tx, organizationID, holdID, recordID, false)
		if err != nil {
			return err
		}
		return r.appendEvent(ctx, tx, organizationID, "legal_hold", holdID, "hold_record_released",
			actorID, input.Reason, before, after, requestID)
	})
}

func (r *dataGovernanceRepo) ListLegalHoldRecords(
	ctx context.Context, organizationID, holdID string, activeOnly bool,
) ([]models.LegalHoldRecord, error) {
	condition := ""
	if activeOnly {
		condition = " AND released_at IS NULL"
	}
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT `+legalHoldRecordColumns+`
		FROM legal_hold_records WHERE organization_id=$1::uuid AND hold_id=$2::uuid`+condition+`
		ORDER BY placed_at DESC,id DESC`, organizationID, holdID)
	if err != nil {
		return nil, fmt.Errorf("list legal hold records: %w", err)
	}
	defer rows.Close()
	return scanLegalHoldRecords(rows)
}

func getLegalHoldRecord(
	ctx context.Context, querier database.Querier, organizationID, holdID, id string, lock bool,
) (*models.LegalHoldRecord, error) {
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE"
	}
	item := new(models.LegalHoldRecord)
	err := querier.QueryRow(ctx, `SELECT `+legalHoldRecordColumns+` FROM legal_hold_records
		WHERE organization_id=$1::uuid AND hold_id=$2::uuid AND id=$3::uuid`+lockClause,
		organizationID, holdID, id).Scan(
		&item.ID, &item.OrganizationID, &item.HoldID, &item.RecordType, &item.RecordID,
		&item.Reason, &item.PlacedBy, &item.PlacedAt, &item.ReleasedAt,
		&item.ReleasedBy, &item.ReleaseReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDataGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get legal hold record: %w", err)
	}
	return item, nil
}

func listLegalHoldRecordsForRecord(
	ctx context.Context, querier database.Querier, organizationID, recordType, recordID string,
) ([]models.LegalHoldRecord, error) {
	rows, err := querier.Query(ctx, `SELECT `+legalHoldRecordColumns+`
		FROM legal_hold_records record
		WHERE record.organization_id=$1::uuid AND record.record_type=$2
		AND record.record_id=$3::uuid AND record.released_at IS NULL
		AND EXISTS (SELECT 1 FROM legal_holds hold WHERE hold.organization_id=record.organization_id
			AND hold.id=record.hold_id AND hold.status='active')
		ORDER BY record.placed_at DESC,record.id DESC`, organizationID, recordType, recordID)
	if err != nil {
		return nil, fmt.Errorf("list active legal holds for record: %w", err)
	}
	defer rows.Close()
	return scanLegalHoldRecords(rows)
}

func scanLegalHoldRecords(rows pgx.Rows) ([]models.LegalHoldRecord, error) {
	items := make([]models.LegalHoldRecord, 0)
	for rows.Next() {
		var item models.LegalHoldRecord
		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.HoldID, &item.RecordType, &item.RecordID,
			&item.Reason, &item.PlacedBy, &item.PlacedAt, &item.ReleasedAt,
			&item.ReleasedBy, &item.ReleaseReason,
		); err != nil {
			return nil, fmt.Errorf("scan legal hold record: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legal hold records: %w", err)
	}
	return items, nil
}

const dataGovernanceEventColumns = `id,chain_sequence,encode(previous_hash,'hex'),
	encode(event_hash,'hex'),entity_type,entity_id,event_type,actor_user_id,reason,
	before_state,after_state,source,COALESCE(request_id,''),created_at`

func (r *dataGovernanceRepo) ListEvents(
	ctx context.Context, organizationID string, filter models.DataGovernanceEventFilter,
) ([]models.DataGovernanceEvent, int, error) {
	where := []string{"organization_id=$1::uuid"}
	args := []any{organizationID}
	if filter.EntityType != "" {
		args = append(args, filter.EntityType)
		where = append(where, fmt.Sprintf("entity_type=$%d", len(args)))
	}
	if filter.EntityID != "" {
		args = append(args, filter.EntityID)
		where = append(where, fmt.Sprintf("entity_id=$%d::uuid", len(args)))
	}
	if filter.EventType != "" {
		args = append(args, filter.EventType)
		where = append(where, fmt.Sprintf("event_type=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")
	querier := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*) FROM data_governance_events WHERE `+condition,
		args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count data governance events: %w", err)
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := querier.Query(ctx, `SELECT `+dataGovernanceEventColumns+`
		FROM data_governance_events WHERE `+condition+` ORDER BY chain_sequence DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list data governance events: %w", err)
	}
	defer rows.Close()
	items := make([]models.DataGovernanceEvent, 0, filter.PageSize)
	for rows.Next() {
		var item models.DataGovernanceEvent
		if err := rows.Scan(
			&item.ID, &item.ChainSequence, &item.PreviousHash, &item.EventHash,
			&item.EntityType, &item.EntityID, &item.EventType, &item.ActorUserID,
			&item.Reason, &item.BeforeState, &item.AfterState, &item.Source,
			&item.RequestID, &item.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan data governance event: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate data governance events: %w", err)
	}
	return items, total, nil
}

func (r *dataGovernanceRepo) VerifyEventChain(
	ctx context.Context, organizationID string,
) (*models.GovernanceChainVerification, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	result := &models.GovernanceChainVerification{Valid: true, VerifiedAt: r.now().UTC()}
	var rowsValid bool
	if err := querier.QueryRow(ctx, `WITH ordered AS (
		SELECT event.*,
			lag(event_hash) OVER (ORDER BY chain_sequence) AS expected_previous_hash
		FROM data_governance_events event
		WHERE organization_id=$1::uuid
	), checked AS (
		SELECT chain_sequence,
			previous_hash = COALESCE(expected_previous_hash,decode(repeat('00',32),'hex'))
			AND event_hash = calculate_data_governance_event_hash(
				organization_id,chain_sequence,previous_hash,entity_type,entity_id,event_type,
				actor_user_id,reason,before_state,after_state,source,request_id,created_at
			) AS valid
		FROM ordered
	)
	SELECT COUNT(*),COALESCE(MAX(chain_sequence),0),COALESCE(bool_and(valid),true)
	FROM checked`, organizationID).Scan(&result.EventCount, &result.LastSequence, &rowsValid); err != nil {
		return nil, fmt.Errorf("verify data governance event rows: %w", err)
	}
	var headSequence int64
	var headHash string
	err := querier.QueryRow(ctx, `SELECT last_sequence,encode(last_hash,'hex')
		FROM data_governance_event_chain_heads WHERE organization_id=$1::uuid`, organizationID).
		Scan(&headSequence, &headHash)
	if errors.Is(err, pgx.ErrNoRows) && result.EventCount == 0 {
		result.LastHash = strings.Repeat("0", 64)
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load data governance event chain head: %w", err)
	}
	result.LastHash = headHash
	if !rowsValid {
		result.Valid = false
		result.FailureReason = "event_hash_or_link_mismatch"
		return result, nil
	}
	if headSequence != result.LastSequence {
		result.Valid = false
		result.FailureReason = "chain_head_sequence_mismatch"
		return result, nil
	}
	if result.EventCount > 0 {
		var eventHash string
		if err := querier.QueryRow(ctx, `SELECT encode(event_hash,'hex')
			FROM data_governance_events WHERE organization_id=$1::uuid
			ORDER BY chain_sequence DESC LIMIT 1`, organizationID).Scan(&eventHash); err != nil {
			return nil, fmt.Errorf("load last data governance event hash: %w", err)
		}
		if eventHash != headHash {
			result.Valid = false
			result.FailureReason = "chain_head_hash_mismatch"
		}
	}
	return result, nil
}

func (r *dataGovernanceRepo) appendEvent(
	ctx context.Context, tx pgx.Tx, organizationID, entityType, entityID, eventType,
	actorID, reason string, before, after any, requestID string,
) error {
	beforeJSON, err := marshalDataGovernanceState(before)
	if err != nil {
		return fmt.Errorf("marshal data governance event before state: %w", err)
	}
	afterJSON, err := marshalDataGovernanceState(after)
	if err != nil {
		return fmt.Errorf("marshal data governance event after state: %w", err)
	}
	var sequence int64
	if err := tx.QueryRow(ctx, `INSERT INTO data_governance_events (
		organization_id,entity_type,entity_id,event_type,actor_user_id,reason,
		before_state,after_state,request_id
	) VALUES ($1::uuid,$2,$3::uuid,$4,$5::uuid,$6,$7::jsonb,$8::jsonb,NULLIF($9,''))
	RETURNING chain_sequence`, organizationID, entityType, entityID, eventType, actorID,
		reason, beforeJSON, afterJSON, requestID).Scan(&sequence); err != nil {
		return fmt.Errorf("append data governance event: %w", err)
	}
	payload := map[string]any{
		"type": "data_governance." + eventType, "severity": "high",
		"org_id": organizationID, "entity_type": entityType, "entity_id": entityID,
		"data": map[string]any{
			"event_type": eventType, "actor_user_id": actorID, "reason": reason,
			"chain_sequence": sequence,
		},
		"timestamp": r.now().UTC(),
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", organizationID, payload)
	if err != nil {
		return fmt.Errorf("create data governance outbox envelope: %w", err)
	}
	if strings.TrimSpace(requestID) != "" {
		envelope.CorrelationID = requestID
	}
	envelope.CausationID = entityID
	envelope.Metadata = map[string]string{
		"entity_type": entityType, "entity_id": entityID, "event_type": eventType,
	}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue data governance event: %w", err)
	}
	return nil
}

func marshalDataGovernanceState(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	reflected := reflect.ValueOf(value)
	if (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice) && reflected.IsNil() {
		return nil, nil
	}
	return marshalNullableObject(value)
}

func ensureGovernanceUser(
	ctx context.Context, querier database.Querier, organizationID, userID string,
) error {
	var exists bool
	if err := querier.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users
		WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		organizationID, userID).Scan(&exists); err != nil {
		return fmt.Errorf("check data governance user: %w", err)
	}
	if !exists {
		return ErrGovernedRecordNotFound
	}
	return nil
}

func ensureGovernedRecord(
	ctx context.Context, querier database.Querier, organizationID, recordType, recordID string,
) error {
	queries := map[string]string{
		"asset":         `SELECT EXISTS (SELECT 1 FROM assets WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"audit":         `SELECT EXISTS (SELECT 1 FROM audits WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"audit_finding": `SELECT EXISTS (SELECT 1 FROM audit_findings WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"comment":       `SELECT EXISTS (SELECT 1 FROM comments WHERE organization_id=$1::uuid AND id=$2::uuid AND NOT is_deleted)`,
		"control":       `SELECT EXISTS (SELECT 1 FROM control_implementations WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"evidence":      `SELECT EXISTS (SELECT 1 FROM control_evidence WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"incident":      `SELECT EXISTS (SELECT 1 FROM incidents WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"policy":        `SELECT EXISTS (SELECT 1 FROM policies WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"report":        `SELECT EXISTS (SELECT 1 FROM report_runs WHERE organization_id=$1::uuid AND id=$2::uuid)`,
		"risk":          `SELECT EXISTS (SELECT 1 FROM risks WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
		"vendor":        `SELECT EXISTS (SELECT 1 FROM vendors WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`,
	}
	query := queries[recordType]
	if query == "" {
		return ErrGovernedRecordNotFound
	}
	var exists bool
	if err := querier.QueryRow(ctx, query, organizationID, recordID).Scan(&exists); err != nil {
		return fmt.Errorf("check governed %s record: %w", recordType, err)
	}
	if !exists {
		return ErrGovernedRecordNotFound
	}
	return nil
}

func mapDataGovernanceWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrDataGovernanceConflict, operation)
		case "23503", "23514", "55000":
			return fmt.Errorf("%w: %s", ErrDataGovernanceState, operation)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
