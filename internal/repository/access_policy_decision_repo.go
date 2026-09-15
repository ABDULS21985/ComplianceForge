package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/accesscontrol"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

func (r *policyAccessRepo) LoadEvaluationBundle(
	ctx context.Context,
	organizationID, subjectID, resourceType, resourceID, action string,
	at time.Time,
) (*models.AccessEvaluationBundle, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	resourceType = normalizeRepositoryResource(resourceType)
	bundle := &models.AccessEvaluationBundle{
		Policies:         []models.AccessPolicy{},
		ObjectGrants:     []models.AccessObjectGrant{},
		FieldPermissions: []models.AccessFieldPermission{},
	}
	if err := loadAccessSubject(ctx, querier, organizationID, subjectID, &bundle.Subject); err != nil {
		return nil, err
	}
	bundle.Resource = models.AccessResource{
		ID: resourceID, Type: resourceType, Attributes: map[string]json.RawMessage{},
	}

	rows, err := querier.Query(ctx, `SELECT DISTINCT `+accessPolicyColumns+`
		FROM access_policies p
		JOIN access_policy_assignments assignment
		  ON assignment.organization_id=p.organization_id AND assignment.access_policy_id=p.id
		WHERE p.organization_id=$1::uuid AND p.is_active AND p.deleted_at IS NULL
		  AND p.resource_type IN ('*',$3)
		  AND ('*'=ANY(p.actions) OR $4=ANY(p.actions))
		  AND (p.valid_from IS NULL OR p.valid_from <= $5)
		  AND (p.valid_until IS NULL OR p.valid_until > $5)
		  AND (assignment.valid_from IS NULL OR assignment.valid_from <= $5)
		  AND (assignment.valid_until IS NULL OR assignment.valid_until > $5)
		  AND (
		    assignment.assignee_type='all_users'
		    OR (assignment.assignee_type='user' AND assignment.assignee_id=$2::uuid)
		    OR (assignment.assignee_type='role' AND EXISTS (
		      SELECT 1 FROM effective_user_roles ur
		      WHERE ur.organization_id=$1::uuid AND ur.user_id=$2::uuid
		        AND ur.role_id=assignment.assignee_id
		    ))
		    OR (assignment.assignee_type='group' AND EXISTS (
		      SELECT 1 FROM directory_group_memberships membership
		      JOIN directory_groups directory_group
		        ON directory_group.organization_id=membership.organization_id
		       AND directory_group.id=membership.group_id
		       AND directory_group.deleted_at IS NULL
		      WHERE membership.organization_id=$1::uuid AND membership.user_id=$2::uuid
		        AND membership.group_id=assignment.assignee_id AND membership.removed_at IS NULL
		    ))
		  )
		ORDER BY p.priority,p.id`, organizationID, subjectID, resourceType, strings.ToLower(action), at)
	if err != nil {
		return nil, fmt.Errorf("load relevant access policies: %w", err)
	}
	for rows.Next() {
		policy, scanErr := scanAccessPolicy(rows)
		if scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("scan relevant access policy: %w", scanErr)
		}
		bundle.Policies = append(bundle.Policies, *policy)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate relevant access policies: %w", err)
	}
	rows.Close()

	externalAuditor := false
	for _, role := range bundle.Subject.Roles {
		if strings.EqualFold(role, "external_auditor") {
			externalAuditor = true
			break
		}
	}
	// Domain-specific object loading is avoided on the RBAC-only fast path.
	// Once a policy is relevant (or the subject is an external auditor), a
	// missing/unsupported object must fail closed rather than becoming a grant.
	if len(bundle.Policies) > 0 || externalAuditor {
		resource, loadErr := loadAccessResource(ctx, querier, organizationID, resourceType, resourceID)
		if loadErr != nil {
			return nil, loadErr
		}
		bundle.Resource = *resource
	}
	if resourceID != "" && (len(bundle.Policies) > 0 || externalAuditor) {
		grants, err := loadEvaluationObjectGrants(ctx, querier, organizationID, subjectID, resourceType, resourceID)
		if err != nil {
			return nil, err
		}
		bundle.ObjectGrants = grants
	}
	if len(bundle.Policies) > 0 {
		policyIDs := make([]string, len(bundle.Policies))
		for index := range bundle.Policies {
			policyIDs[index] = bundle.Policies[index].ID
		}
		fieldPermissions, err := loadEvaluationFieldPermissions(ctx, querier, organizationID, resourceType, policyIDs)
		if err != nil {
			return nil, err
		}
		bundle.FieldPermissions = fieldPermissions
	}
	return bundle, nil
}

func loadAccessSubject(
	ctx context.Context, querier database.Querier, organizationID, subjectID string, subject *models.AccessSubject,
) error {
	rows, err := querier.Query(ctx, `SELECT u.id,COALESCE(u.department,''),COALESCE(u.location,''),
		u.status::text,u.is_super_admin,r.slug
		FROM users u
		LEFT JOIN effective_user_roles ur ON ur.organization_id=u.organization_id AND ur.user_id=u.id
		LEFT JOIN roles r ON r.id=ur.role_id AND r.deleted_at IS NULL
		WHERE u.organization_id=$1::uuid AND u.id=$2::uuid
		  AND u.status='active' AND u.deleted_at IS NULL
		ORDER BY r.slug NULLS LAST`, organizationID, subjectID)
	if err != nil {
		return fmt.Errorf("load access subject: %w", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var role *string
		if err := rows.Scan(&subject.ID, &subject.Department, &subject.Location, &subject.Status, &subject.SuperAdmin, &role); err != nil {
			return fmt.Errorf("scan access subject: %w", err)
		}
		found = true
		if role != nil && !containsRepositoryString(subject.Roles, *role) {
			subject.Roles = append(subject.Roles, *role)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate access subject: %w", err)
	}
	if !found {
		return accesscontrol.ErrNotFound
	}
	if subject.Roles == nil {
		subject.Roles = []string{}
	}
	subject.Attributes = map[string]json.RawMessage{}
	return nil
}

func loadAccessResource(
	ctx context.Context, querier database.Querier, organizationID, resourceType, resourceID string,
) (*models.AccessResource, error) {
	resource := &models.AccessResource{ID: resourceID, Type: resourceType, Attributes: map[string]json.RawMessage{}}
	if strings.TrimSpace(resourceID) == "" {
		return resource, nil
	}
	var row pgx.Row
	switch resourceType {
	case "risks":
		row = querier.QueryRow(ctx, `SELECT COALESCE(r.owner_user_id::text,''),COALESCE(owner.department,''),
			COALESCE(owner.location,''),COALESCE(r.metadata->>'classification','')
			FROM risks r LEFT JOIN users owner ON owner.organization_id=r.organization_id AND owner.id=r.owner_user_id
			WHERE r.organization_id=$1::uuid AND r.id=$2::uuid AND r.deleted_at IS NULL`, organizationID, resourceID)
	case "policies":
		row = querier.QueryRow(ctx, `SELECT COALESCE(p.owner_user_id::text,''),COALESCE(owner.department,''),
			COALESCE(owner.location,''),p.classification
			FROM policies p LEFT JOIN users owner ON owner.organization_id=p.organization_id AND owner.id=p.owner_user_id
			WHERE p.organization_id=$1::uuid AND p.id=$2::uuid AND p.deleted_at IS NULL`, organizationID, resourceID)
	case "controls":
		row = querier.QueryRow(ctx, `SELECT COALESCE(c.owner_user_id::text,''),COALESCE(owner.department,''),
			COALESCE(owner.location,''),COALESCE(c.metadata->>'classification','')
			FROM control_implementations c LEFT JOIN users owner ON owner.organization_id=c.organization_id AND owner.id=c.owner_user_id
			WHERE c.organization_id=$1::uuid AND c.id=$2::uuid AND c.deleted_at IS NULL`, organizationID, resourceID)
	case "audits":
		row = querier.QueryRow(ctx, `SELECT a.lead_auditor_id::text,COALESCE(owner.department,''),
			COALESCE(owner.location,''),COALESCE(a.metadata->>'classification','')
			FROM audits a LEFT JOIN users owner ON owner.organization_id=a.organization_id AND owner.id=a.lead_auditor_id
			WHERE a.organization_id=$1::uuid AND a.id=$2::uuid AND a.deleted_at IS NULL`, organizationID, resourceID)
	case "findings":
		row = querier.QueryRow(ctx, `SELECT f.responsible_user_id::text,COALESCE(owner.department,''),
			COALESCE(owner.location,''),COALESCE(f.metadata->>'classification','')
			FROM audit_findings f LEFT JOIN users owner ON owner.organization_id=f.organization_id AND owner.id=f.responsible_user_id
			WHERE f.organization_id=$1::uuid AND f.id=$2::uuid AND f.deleted_at IS NULL`, organizationID, resourceID)
	case "incidents":
		row = querier.QueryRow(ctx, `SELECT COALESCE(i.assigned_to,i.reporter_id)::text,COALESCE(owner.department,''),
			COALESCE(owner.location,''),COALESCE(i.metadata->>'classification',CASE WHEN i.special_category_data THEN 'restricted' ELSE '' END)
			FROM incidents i LEFT JOIN users owner ON owner.organization_id=i.organization_id
			 AND owner.id=COALESCE(i.assigned_to,i.reporter_id)
			WHERE i.organization_id=$1::uuid AND i.id=$2::uuid AND i.deleted_at IS NULL`, organizationID, resourceID)
	case "assets":
		row = querier.QueryRow(ctx, `SELECT COALESCE(a.owner_user_id::text,''),COALESCE(owner.department,''),
			COALESCE(a.location,''),a.classification
			FROM assets a LEFT JOIN users owner ON owner.organization_id=a.organization_id AND owner.id=a.owner_user_id
			WHERE a.organization_id=$1::uuid AND a.id=$2::uuid AND a.deleted_at IS NULL`, organizationID, resourceID)
	case "vendors":
		row = querier.QueryRow(ctx, `SELECT COALESCE(v.owner_user_id::text,''),COALESCE(owner.department,''),
			COALESCE(owner.location,''),COALESCE(v.metadata->>'classification','')
			FROM vendors v LEFT JOIN users owner ON owner.organization_id=v.organization_id AND owner.id=v.owner_user_id
			WHERE v.organization_id=$1::uuid AND v.id=$2::uuid AND v.deleted_at IS NULL`, organizationID, resourceID)
	case "reports":
		row = querier.QueryRow(ctx, `SELECT COALESCE(report.created_by::text,''),COALESCE(owner.department,''),
			COALESCE(owner.location,''),report.classification
			FROM report_definitions report LEFT JOIN users owner
			 ON owner.organization_id=report.organization_id AND owner.id=report.created_by
			WHERE report.organization_id=$1::uuid AND report.id=$2::uuid`, organizationID, resourceID)
	case "users":
		row = querier.QueryRow(ctx, `SELECT target.id::text,COALESCE(target.department,''),
			COALESCE(target.location,''),'personal'
			FROM users target WHERE target.organization_id=$1::uuid AND target.id=$2::uuid AND target.deleted_at IS NULL`, organizationID, resourceID)
	case "frameworks":
		row = querier.QueryRow(ctx, `SELECT '', '', '', CASE WHEN framework.is_system_framework THEN 'public' ELSE 'internal' END
			FROM compliance_frameworks framework
			WHERE framework.id=$2::uuid AND framework.deleted_at IS NULL
			  AND (framework.organization_id=$1::uuid OR framework.organization_id IS NULL)`, organizationID, resourceID)
	case "organizations":
		row = querier.QueryRow(ctx, `SELECT '', '', '', 'internal'
			FROM organizations organization WHERE organization.id=$1::uuid AND organization.id=$2::uuid`, organizationID, resourceID)
	default:
		return nil, fmt.Errorf("unsupported object-level resource %q", resourceType)
	}
	if err := row.Scan(&resource.OwnerID, &resource.Department, &resource.Location, &resource.Classification); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, accesscontrol.ErrNotFound
		}
		return nil, fmt.Errorf("load %s access attributes: %w", resourceType, err)
	}
	return resource, nil
}

func loadEvaluationFieldPermissions(
	ctx context.Context, querier database.Querier, organizationID, resourceType string, policyIDs []string,
) ([]models.AccessFieldPermission, error) {
	rows, err := querier.Query(ctx, `SELECT field.id,field.organization_id,field.access_policy_id,
		policy.priority,field.resource_type,field.field_path,field.classification,field.visibility,
		COALESCE(field.mask_strategy,''),COALESCE(field.mask_pattern,''),field.version,field.created_at
		FROM field_level_permissions field
		JOIN access_policies policy ON policy.organization_id=field.organization_id AND policy.id=field.access_policy_id
		WHERE field.organization_id=$1::uuid AND field.resource_type=$2
		  AND field.access_policy_id=ANY($3::uuid[])
		ORDER BY field.field_path,policy.priority,field.access_policy_id,field.id`, organizationID, resourceType, policyIDs)
	if err != nil {
		return nil, fmt.Errorf("load evaluation field permissions: %w", err)
	}
	defer rows.Close()
	items := []models.AccessFieldPermission{}
	for rows.Next() {
		item, err := scanAccessFieldPermission(rows)
		if err != nil {
			return nil, fmt.Errorf("scan evaluation field permission: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evaluation field permissions: %w", err)
	}
	return items, nil
}

func (r *policyAccessRepo) RecordDecision(ctx context.Context, evidence models.AccessDecisionEvidence) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	matchedPolicyIDs := evidence.MatchedPolicyIDs
	if matchedPolicyIDs == nil {
		matchedPolicyIDs = []string{}
	}
	_, err := querier.Exec(ctx, `INSERT INTO access_audit_log (
		id,organization_id,user_id,action,resource_type,resource_id,decision,matched_policy_id,
		evaluation_time_us,subject_attributes,resource_attributes,environment_attributes,
		rbac_allowed,constraint_outcome,reason_code,matched_policy_ids,matched_object_grant_id,request_id
	) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6::uuid,$7,$8::uuid,$9,$10::jsonb,$11::jsonb,$12::jsonb,
		$13,$14,$15,$16::uuid[],$17::uuid,$18)`,
		evidence.ID, evidence.OrganizationID, evidence.SubjectID, evidence.Action, evidence.ResourceType,
		evidence.ResourceID, evidence.Decision, evidence.WinningPolicyID, evidence.EvaluationTimeUS,
		evidence.SubjectSnapshot, evidence.ResourceSnapshot, evidence.EnvironmentSnapshot,
		evidence.RBACAllowed, evidence.ConstraintOutcome, evidence.ReasonCode, matchedPolicyIDs,
		evidence.MatchedObjectGrantID, nullableAccessString(evidence.RequestID))
	if err != nil {
		return fmt.Errorf("record access decision evidence: %w", err)
	}
	return nil
}

func (r *policyAccessRepo) ListDecisionEvidence(
	ctx context.Context, organizationID string, filter models.AccessDecisionEvidenceFilter,
) ([]models.AccessDecisionEvidence, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	predicate := ` FROM access_audit_log decision
		WHERE decision.organization_id=$1::uuid
		  AND ($2='' OR decision.user_id=$2::uuid)
		  AND ($3='' OR decision.resource_type=$3)
		  AND ($4='' OR decision.resource_id=$4::uuid)
		  AND ($5='' OR decision.action=$5)
		  AND ($6='' OR decision.decision=$6)
		  AND ($7::timestamptz IS NULL OR decision.created_at >= $7)
		  AND ($8::timestamptz IS NULL OR decision.created_at < $8)`
	args := []any{organizationID, filter.SubjectID, filter.ResourceType, filter.ResourceID,
		filter.Action, filter.Decision, filter.From, filter.Until}
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)`+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count access decision evidence: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT decision.id,decision.organization_id,decision.user_id,
		decision.action,decision.resource_type,decision.resource_id,decision.rbac_allowed,
		decision.constraint_outcome,decision.decision,decision.reason_code,
		COALESCE(decision.matched_policy_ids,'{}'::uuid[]),decision.matched_policy_id,
		decision.matched_object_grant_id,COALESCE(decision.request_id,''),
		COALESCE(decision.subject_attributes,'{}'::jsonb),COALESCE(decision.resource_attributes,'{}'::jsonb),
		COALESCE(decision.environment_attributes,'{}'::jsonb),COALESCE(decision.evaluation_time_us,0),decision.created_at`+
		predicate+` ORDER BY decision.created_at DESC,decision.id LIMIT $9 OFFSET $10`,
		append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list access decision evidence: %w", err)
	}
	defer rows.Close()
	items := make([]models.AccessDecisionEvidence, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanAccessDecisionEvidence(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan access decision evidence: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate access decision evidence: %w", err)
	}
	return items, total, nil
}

func scanAccessDecisionEvidence(row policyAccessRowScanner) (*models.AccessDecisionEvidence, error) {
	item := new(models.AccessDecisionEvidence)
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.SubjectID, &item.Action, &item.ResourceType,
		&item.ResourceID, &item.RBACAllowed, &item.ConstraintOutcome, &item.Decision,
		&item.ReasonCode, &item.MatchedPolicyIDs, &item.WinningPolicyID,
		&item.MatchedObjectGrantID, &item.RequestID, &item.SubjectSnapshot,
		&item.ResourceSnapshot, &item.EnvironmentSnapshot, &item.EvaluationTimeUS, &item.CreatedAt,
	); err != nil {
		return nil, err
	}
	if item.MatchedPolicyIDs == nil {
		item.MatchedPolicyIDs = []string{}
	}
	return item, nil
}

func nullableAccessString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func containsRepositoryString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// loadEvaluationObjectGrants is implemented in access_policy_grant_repo.go.
// Keep evaluation fail-closed if the grant slice cannot be loaded.
func loadEvaluationObjectGrants(
	ctx context.Context,
	querier database.Querier,
	organizationID, subjectID, resourceType, resourceID string,
) ([]models.AccessObjectGrant, error) {
	rows, err := querier.Query(ctx, `SELECT id,organization_id,user_id,entity_type,entity_id,
		actions,status,COALESCE(granted_by::text,''),approved_by,approved_at,valid_from,expires_at,
		allow_download,require_watermark,COALESCE(watermark_text,''),COALESCE(grant_reason,''),
		version,created_at,updated_at,revoked_at,revoked_by
		FROM user_entity_permissions
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND entity_type=$3 AND entity_id=$4::uuid
		ORDER BY expires_at,id`, organizationID, subjectID, resourceType, resourceID)
	if err != nil {
		return nil, fmt.Errorf("load evaluation object grants: %w", err)
	}
	defer rows.Close()
	items := []models.AccessObjectGrant{}
	for rows.Next() {
		item, err := scanAccessObjectGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan evaluation object grant: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evaluation object grants: %w", err)
	}
	return items, nil
}

func scanAccessObjectGrant(row policyAccessRowScanner) (*models.AccessObjectGrant, error) {
	item := new(models.AccessObjectGrant)
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.SubjectID, &item.ResourceType, &item.ResourceID,
		&item.Actions, &item.Status, &item.SponsorID, &item.ApprovedBy, &item.ApprovedAt,
		&item.ValidFrom, &item.ValidUntil, &item.AllowDownload, &item.RequireWatermark,
		&item.WatermarkText, &item.Reason, &item.Version, &item.CreatedAt, &item.UpdatedAt,
		&item.RevokedAt, &item.RevokedBy,
	); err != nil {
		return nil, err
	}
	if item.Actions == nil {
		item.Actions = []string{}
	}
	return item, nil
}
