package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/accesscontrol"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

// PolicyAccessRepository is the typed, tenant-explicit persistence contract
// for policy constraints, object grants, field controls, and immutable access
// evidence. Every implementation method uses database.QuerierFromContext.
type PolicyAccessRepository interface {
	ListPolicies(context.Context, string, models.AccessPolicyListFilter) ([]models.AccessPolicy, int, error)
	GetPolicy(context.Context, string, string) (*models.AccessPolicy, error)
	CreatePolicy(context.Context, string, string, string, models.AccessPolicyInput) (*models.AccessPolicy, error)
	UpdatePolicy(context.Context, string, string, string, string, models.AccessPolicyInput) (*models.AccessPolicy, error)
	DeletePolicy(context.Context, string, string, string, string, int64, string) error
	ListAssignments(context.Context, string, string) ([]models.AccessPolicyAssignment, error)
	CreateAssignment(context.Context, string, string, string, string, models.AccessPolicyAssignmentInput) (*models.AccessPolicyAssignment, error)
	RemoveAssignment(context.Context, string, string, string, string, string, string) error
	ListFieldPermissions(context.Context, string, string, string) ([]models.AccessFieldPermission, error)
	UpsertFieldPermission(context.Context, string, string, string, string, models.AccessFieldPermissionInput) (*models.AccessFieldPermission, error)
	DeleteFieldPermission(context.Context, string, string, string, string, string, int64, string) error
	ListObjectGrants(context.Context, string, models.AccessObjectGrantFilter) ([]models.AccessObjectGrant, int, error)
	CreateObjectGrant(context.Context, string, string, string, models.AccessObjectGrantInput) (*models.AccessObjectGrant, error)
	DecideObjectGrant(context.Context, string, string, string, string, models.AccessObjectGrantDecisionInput) (*models.AccessObjectGrant, error)
	RevokeObjectGrant(context.Context, string, string, string, string, models.AccessObjectGrantRevocationInput) (*models.AccessObjectGrant, error)
	LoadEvaluationBundle(context.Context, string, string, string, string, string, time.Time) (*models.AccessEvaluationBundle, error)
	RecordDecision(context.Context, models.AccessDecisionEvidence) error
	ListDecisionEvidence(context.Context, string, models.AccessDecisionEvidenceFilter) ([]models.AccessDecisionEvidence, int, error)
	CertifyPolicy(context.Context, string, string, string, string, models.AccessPolicyCertificationInput) (*models.AccessPolicyCertification, error)
	ListPolicyCertifications(context.Context, string, string, models.PaginationRequest) ([]models.AccessPolicyCertification, int, error)
}

type policyAccessRepo struct{ pool *pgxpool.Pool }

var _ PolicyAccessRepository = (*policyAccessRepo)(nil)

func NewPolicyAccessRepository(pool *pgxpool.Pool) (PolicyAccessRepository, error) {
	if pool == nil {
		return nil, errors.New("policy access database pool is required")
	}
	return &policyAccessRepo{pool: pool}, nil
}

const accessPolicyColumns = `p.id,p.organization_id,p.name,COALESCE(p.description,''),p.priority,
	p.effect,p.is_active,p.subject_conditions,p.resource_type,
	COALESCE(p.resource_conditions,'[]'::jsonb),p.actions,
	COALESCE(p.environment_conditions,'[]'::jsonb),p.valid_from,p.valid_until,
	p.version,p.created_by,p.updated_by,p.created_at,p.updated_at,p.deleted_at`

type policyAccessRowScanner interface{ Scan(...any) error }

func scanAccessPolicy(row policyAccessRowScanner) (*models.AccessPolicy, error) {
	item := new(models.AccessPolicy)
	var subjectJSON, resourceJSON, environmentJSON []byte
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.Name, &item.Description, &item.Priority,
		&item.Effect, &item.IsActive, &subjectJSON, &item.ResourceType,
		&resourceJSON, &item.Actions, &environmentJSON, &item.ValidFrom, &item.ValidUntil,
		&item.Version, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	); err != nil {
		return nil, err
	}
	for name, encoded := range map[string][]byte{
		"subject": subjectJSON, "resource": resourceJSON, "environment": environmentJSON,
	} {
		var conditions []models.AccessCondition
		if err := json.Unmarshal(encoded, &conditions); err != nil {
			return nil, fmt.Errorf("decode %s policy conditions: %w", name, err)
		}
		switch name {
		case "subject":
			item.SubjectConditions = conditions
		case "resource":
			item.ResourceConditions = conditions
		default:
			item.EnvironmentConditions = conditions
		}
	}
	if item.SubjectConditions == nil {
		item.SubjectConditions = []models.AccessCondition{}
	}
	if item.ResourceConditions == nil {
		item.ResourceConditions = []models.AccessCondition{}
	}
	if item.EnvironmentConditions == nil {
		item.EnvironmentConditions = []models.AccessCondition{}
	}
	if item.Actions == nil {
		item.Actions = []string{}
	}
	return item, nil
}

func (r *policyAccessRepo) ListPolicies(
	ctx context.Context, organizationID string, filter models.AccessPolicyListFilter,
) ([]models.AccessPolicy, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	predicate := ` FROM access_policies p
		WHERE p.organization_id=$1::uuid AND p.deleted_at IS NULL
		  AND ($2='' OR p.name ILIKE '%%'||$2||'%%' OR COALESCE(p.description,'') ILIKE '%%'||$2||'%%')
		  AND ($3='' OR p.resource_type=$3)
		  AND ($4='' OR p.effect=$4)
		  AND ($5::boolean IS NULL OR p.is_active=$5)`
	args := []any{organizationID, filter.Search, filter.ResourceType, string(filter.Effect), filter.Active}
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)`+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count access policies: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT `+accessPolicyColumns+predicate+`
		ORDER BY p.priority,p.id LIMIT $6 OFFSET $7`, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list access policies: %w", err)
	}
	defer rows.Close()
	items := make([]models.AccessPolicy, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanAccessPolicy(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan access policy: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate access policies: %w", err)
	}
	return items, total, nil
}

func (r *policyAccessRepo) GetPolicy(ctx context.Context, organizationID, policyID string) (*models.AccessPolicy, error) {
	item, err := scanAccessPolicy(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx,
		`SELECT `+accessPolicyColumns+` FROM access_policies p
		 WHERE p.organization_id=$1::uuid AND p.id=$2::uuid AND p.deleted_at IS NULL`, organizationID, policyID))
	return item, mapPolicyAccessNotFound("get access policy", err)
}

func (r *policyAccessRepo) CreatePolicy(
	ctx context.Context, organizationID, actorID, requestID string, input models.AccessPolicyInput,
) (*models.AccessPolicy, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var created *models.AccessPolicy
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		subjectJSON, resourceJSON, environmentJSON, err := marshalPolicyConditions(input)
		if err != nil {
			return err
		}
		created, err = scanAccessPolicy(tx.QueryRow(ctx, `INSERT INTO access_policies AS p (
			organization_id,name,description,priority,effect,is_active,subject_conditions,
			resource_type,resource_conditions,actions,environment_conditions,valid_from,
			valid_until,created_by,updated_by
		) VALUES ($1::uuid,$2,NULLIF($3,''),$4,$5,$6,$7::jsonb,$8,$9::jsonb,$10,$11::jsonb,$12,$13,$14::uuid,$14::uuid)
		RETURNING `+accessPolicyColumns,
			organizationID, input.Name, input.Description, input.Priority, input.Effect, input.IsActive,
			subjectJSON, input.ResourceType, resourceJSON, input.Actions, environmentJSON,
			input.ValidFrom, input.ValidUntil, actorID))
		if err != nil {
			return classifyPolicyAccessWrite("create access policy", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, created.ID, "policy", created.ID,
			"policy_created", actorID, requestID, input.Reason, nil, created)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *policyAccessRepo) UpdatePolicy(
	ctx context.Context, organizationID, policyID, actorID, requestID string, input models.AccessPolicyInput,
) (*models.AccessPolicy, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var updated *models.AccessPolicy
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := lockAccessPolicy(ctx, tx, organizationID, policyID)
		if err != nil {
			return err
		}
		if input.ExpectedVersion == nil || before.Version != *input.ExpectedVersion {
			return accesscontrol.ErrConflict
		}
		subjectJSON, resourceJSON, environmentJSON, err := marshalPolicyConditions(input)
		if err != nil {
			return err
		}
		updated, err = scanAccessPolicy(tx.QueryRow(ctx, `UPDATE access_policies p SET
			name=$3,description=NULLIF($4,''),priority=$5,effect=$6,is_active=$7,
			subject_conditions=$8::jsonb,resource_type=$9,resource_conditions=$10::jsonb,
			actions=$11,environment_conditions=$12::jsonb,valid_from=$13,valid_until=$14,
			updated_by=$15::uuid,version=version+1
		WHERE p.organization_id=$1::uuid AND p.id=$2::uuid AND p.version=$16 AND p.deleted_at IS NULL
		RETURNING `+accessPolicyColumns,
			organizationID, policyID, input.Name, input.Description, input.Priority, input.Effect,
			input.IsActive, subjectJSON, input.ResourceType, resourceJSON, input.Actions,
			environmentJSON, input.ValidFrom, input.ValidUntil, actorID, *input.ExpectedVersion))
		if errors.Is(err, pgx.ErrNoRows) {
			return accesscontrol.ErrConflict
		}
		if err != nil {
			return classifyPolicyAccessWrite("update access policy", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, policyID, "policy", policyID,
			"policy_updated", actorID, requestID, input.Reason, before, updated)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *policyAccessRepo) DeletePolicy(
	ctx context.Context, organizationID, policyID, actorID, requestID string, expectedVersion int64, reason string,
) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := lockAccessPolicy(ctx, tx, organizationID, policyID)
		if err != nil {
			return err
		}
		if before.Version != expectedVersion {
			return accesscontrol.ErrConflict
		}
		tag, err := tx.Exec(ctx, `UPDATE access_policies SET is_active=false,deleted_at=NOW(),
			updated_by=$4::uuid,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`,
			organizationID, policyID, expectedVersion, actorID)
		if err != nil {
			return classifyPolicyAccessWrite("retire access policy", err)
		}
		if tag.RowsAffected() != 1 {
			return accesscontrol.ErrConflict
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, policyID, "policy", policyID,
			"policy_retired", actorID, requestID, reason, before, nil)
	})
}

func lockAccessPolicy(ctx context.Context, querier database.Querier, organizationID, policyID string) (*models.AccessPolicy, error) {
	item, err := scanAccessPolicy(querier.QueryRow(ctx, `SELECT `+accessPolicyColumns+`
		FROM access_policies p WHERE p.organization_id=$1::uuid AND p.id=$2::uuid
		AND p.deleted_at IS NULL FOR UPDATE`, organizationID, policyID))
	return item, mapPolicyAccessNotFound("lock access policy", err)
}

func marshalPolicyConditions(input models.AccessPolicyInput) ([]byte, []byte, []byte, error) {
	subjectJSON, err := json.Marshal(input.SubjectConditions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode subject conditions: %w", err)
	}
	resourceJSON, err := json.Marshal(input.ResourceConditions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode resource conditions: %w", err)
	}
	environmentJSON, err := json.Marshal(input.EnvironmentConditions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode environment conditions: %w", err)
	}
	return subjectJSON, resourceJSON, environmentJSON, nil
}

func recordAccessPolicyChange(
	ctx context.Context,
	querier database.Querier,
	organizationID, policyID, entityType, entityID, eventType, actorID, requestID, reason string,
	before, after any,
) error {
	beforeJSON, err := marshalAccessChangeState(before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalAccessChangeState(after)
	if err != nil {
		return err
	}
	hashInput := append(append([]byte(nil), beforeJSON...), afterJSON...)
	hashInput = append(hashInput, []byte(eventType+actorID+requestID+reason)...)
	hashValue := sha256.Sum256(hashInput)
	_, err = querier.Exec(ctx, `INSERT INTO access_policy_change_events (
		organization_id,access_policy_id,entity_type,entity_id,event_type,actor_user_id,
		request_id,reason,before_state,after_state,evidence_sha256
	) VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6::uuid,$7,$8,$9::jsonb,$10::jsonb,$11)`,
		organizationID, nullableAccessString(policyID), entityType, entityID, eventType, actorID, requestID, reason,
		nullableAccessJSON(beforeJSON), nullableAccessJSON(afterJSON), hex.EncodeToString(hashValue[:]))
	if err != nil {
		return fmt.Errorf("record access policy change: %w", err)
	}
	return nil
}

func marshalAccessChangeState(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode access policy change state: %w", err)
	}
	if len(encoded) > 64*1024 {
		return nil, errors.New("access policy change state exceeds 65536 bytes")
	}
	return encoded, nil
}

func nullableAccessJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func mapPolicyAccessNotFound(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return accesscontrol.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func classifyPolicyAccessWrite(operation string, err error) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%w: %s", accesscontrol.ErrConflict, operation)
		case "23503", "23514", "22P02":
			return fmt.Errorf("%w: %s", accesscontrol.ErrInvalidRelation, operation)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func normalizeRepositoryResource(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "risk":
		return "risks"
	case "policy":
		return "policies"
	case "control", "control_implementation":
		return "controls"
	case "framework":
		return "frameworks"
	case "audit":
		return "audits"
	case "finding":
		return "findings"
	case "incident":
		return "incidents"
	case "asset":
		return "assets"
	case "vendor":
		return "vendors"
	case "report":
		return "reports"
	case "user":
		return "users"
	default:
		return value
	}
}
