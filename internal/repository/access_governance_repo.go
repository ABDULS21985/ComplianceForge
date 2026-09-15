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

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var (
	ErrAccessGovernanceConflict   = errors.New("access governance version, snapshot, or request conflict")
	ErrAccessGovernanceSeparation = errors.New("access governance requires an independent active reviewer")
	ErrAccessGovernanceInvalid    = errors.New("access governance input or transition is invalid")
	ErrAccessGovernanceBound      = errors.New("access review exceeds the 1000 item campaign limit")
)

type AccessGovernanceRepository interface {
	CreateCampaign(context.Context, string, string, models.AccessReviewCampaignInput) (*models.AccessReviewCampaign, error)
	ListCampaigns(context.Context, string, models.PaginationRequest) ([]models.AccessReviewCampaign, int, error)
	GetCampaign(context.Context, string, string) (*models.AccessReviewCampaign, error)
	ListItems(context.Context, string, string, models.PaginationRequest) ([]models.AccessReviewItem, int, error)
	DecideItem(context.Context, string, string, string, string, models.AccessReviewDecisionInput) (*models.AccessReviewDecision, error)
	TransitionCampaign(context.Context, string, string, string, string, models.AccessGovernanceTransitionInput) (*models.AccessReviewCampaign, error)
	CreateSoDRule(context.Context, string, string, models.AccessSoDRuleInput) (*models.AccessSoDRule, error)
	ListSoDRules(context.Context, string, models.PaginationRequest) ([]models.AccessSoDRule, int, error)
	DisableSoDRule(context.Context, string, string, string, models.AccessGovernanceTransitionInput) (*models.AccessSoDRule, error)
	RequestException(context.Context, string, string, string, models.AccessSoDExceptionInput) (*models.AccessSoDException, error)
	DecideException(context.Context, string, string, string, string, models.AccessGovernanceTransitionInput) (*models.AccessSoDException, error)
	ListExceptions(context.Context, string, models.PaginationRequest) ([]models.AccessSoDException, int, error)
	ListViolations(context.Context, string, models.PaginationRequest) ([]models.AccessSoDViolation, int, error)
	ListEvents(context.Context, string, models.PaginationRequest) ([]models.AccessGovernanceEvent, int, error)
	SetAssignmentWindow(context.Context, string, string, string, string, models.ManagedRoleWindowInput) (*models.ManagedRoleAssignment, error)
}

type accessGovernanceRepo struct {
	pool   *pgxpool.Pool
	outbox queuepkg.OutboxEnqueuer
	queue  string
}

var _ AccessGovernanceRepository = (*accessGovernanceRepo)(nil)

func NewAccessGovernanceRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, queue string) (AccessGovernanceRepository, error) {
	if pool == nil || outbox == nil || strings.TrimSpace(queue) == "" {
		return nil, errors.New("access governance database and durable outbox are required")
	}
	return &accessGovernanceRepo{pool: pool, outbox: outbox, queue: strings.TrimSpace(queue)}, nil
}

const campaignColumns = `id,organization_id,name,reviewer_id,created_by,status,due_at,version,created_at`

func scanGovernanceCampaign(row interface{ Scan(...any) error }) (*models.AccessReviewCampaign, error) {
	c := new(models.AccessReviewCampaign)
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.ReviewerID, &c.CreatedBy, &c.Status, &c.DueAt, &c.Version, &c.CreatedAt)
	return c, err
}
func governanceLock(ctx context.Context, tx pgx.Tx, org, actor string) error {
	if err := lockDirectoryOrganization(ctx, tx, org); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SET LOCAL TIME ZONE 'UTC'`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,580))`, org); err != nil {
		return err
	}
	if err := ensureManagedRoleUser(ctx, tx, org, actor); err != nil {
		return ErrAccessGovernanceSeparation
	}
	return nil
}
func (r *accessGovernanceRepo) CreateCampaign(ctx context.Context, org, actor string, in models.AccessReviewCampaignInput) (*models.AccessReviewCampaign, error) {
	var c *models.AccessReviewCampaign
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		if in.ReviewerID == actor {
			return ErrAccessGovernanceSeparation
		}
		if err := ensureManagedRoleUser(ctx, tx, org, in.ReviewerID); err != nil {
			return ErrAccessGovernanceSeparation
		}
		for _, subject := range in.SubjectIDs {
			if subject == in.ReviewerID {
				return ErrAccessGovernanceSeparation
			}
			if err := ensureManagedRoleUser(ctx, tx, org, subject); err != nil {
				return ErrAccessGovernanceInvalid
			}
		}
		var err error
		c, err = scanGovernanceCampaign(tx.QueryRow(ctx, `INSERT INTO access_review_campaigns(organization_id,name,reviewer_id,created_by,due_at)
			VALUES($1::uuid,$2,$3::uuid,$4::uuid,$5) RETURNING `+campaignColumns, org, in.Name, in.ReviewerID, actor, in.DueAt))
		if err != nil {
			return classifyGovernance(err)
		}
		rows, err := tx.Query(ctx, `SELECT subject_id,kind,resource_id FROM (
			SELECT user_id subject_id,'role_assignment' kind,role_id resource_id FROM user_roles
			 WHERE organization_id=$1::uuid AND user_id=ANY($2::uuid[])
			   AND valid_from<=statement_timestamp() AND (expires_at IS NULL OR expires_at>statement_timestamp())
			UNION ALL SELECT user_id,'object_grant',id FROM user_entity_permissions
			 WHERE organization_id=$1::uuid AND user_id=ANY($2::uuid[]) AND status='approved'
			   AND valid_from<=statement_timestamp() AND expires_at>statement_timestamp()
		) items ORDER BY subject_id,kind,resource_id LIMIT 1001`, org, in.SubjectIDs)
		if err != nil {
			return err
		}
		type target struct{ subject, kind, resource string }
		targets := []target{}
		for rows.Next() {
			var item target
			if err := rows.Scan(&item.subject, &item.kind, &item.resource); err != nil {
				rows.Close()
				return err
			}
			targets = append(targets, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(targets) > 1000 {
			return ErrAccessGovernanceBound
		}
		if len(targets) == 0 {
			return ErrAccessGovernanceInvalid
		}
		for _, target := range targets {
			snapshot, hash, err := governanceSnapshot(ctx, tx, org, target.subject, target.kind, target.resource)
			if err != nil {
				return err
			}
			// Object-grant sponsor and approver cannot certify their own grant.
			if target.kind == "object_grant" {
				var self bool
				if err := tx.QueryRow(ctx, `SELECT COALESCE(granted_by=$3::uuid,false) OR COALESCE(approved_by=$3::uuid,false) FROM user_entity_permissions WHERE organization_id=$1::uuid AND id=$2::uuid`, org, target.resource, in.ReviewerID).Scan(&self); err != nil {
					return err
				}
				if self {
					return ErrAccessGovernanceSeparation
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO access_review_items(organization_id,campaign_id,subject_id,kind,resource_id,snapshot,snapshot_sha256)
			 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6::jsonb,$7)`, org, c.ID, target.subject, target.kind, target.resource, snapshot, hash); err != nil {
				return classifyGovernance(err)
			}
		}
		return r.event(ctx, tx, org, c.ID, actor, "campaign_created", in.Reason, map[string]any{"campaign": c, "item_count": len(targets)})
	})
	return c, err
}

// All snapshot authority is read under the mutation transaction's locks. Role
// version captures permission edits; assignment/grant versions capture windows.
func governanceSnapshot(ctx context.Context, tx pgx.Tx, org, subject, kind, resource string) ([]byte, string, error) {
	var raw []byte
	if kind == "role_assignment" {
		var id string
		if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE id=$2::uuid AND deleted_at IS NULL AND (organization_id=$1::uuid OR organization_id IS NULL) FOR SHARE`, org, resource).Scan(&id); err != nil {
			return nil, "", err
		}
		if err := tx.QueryRow(ctx, `SELECT user_id FROM user_roles WHERE organization_id=$1::uuid AND user_id=$2::uuid AND role_id=$3::uuid FOR UPDATE`, org, subject, resource).Scan(&id); err != nil {
			return nil, "", err
		}
		err := tx.QueryRow(ctx, `SELECT jsonb_build_object('subject_id',ur.user_id,'role_id',ur.role_id,
		 'assignment_version',ur.version,'role_version',role.version,'role_slug',role.slug,
		 'valid_from',ur.valid_from::text,'expires_at',ur.expires_at::text,'assigned_by',ur.assigned_by,
		 'assigned_at',ur.assigned_at::text,'assignment_id',ur.assignment_id,
		 'permissions',COALESCE((SELECT jsonb_agg(permission.resource||':'||permission.action::text ORDER BY permission.resource,permission.action::text)
		 FROM role_permissions rp JOIN permissions permission ON permission.id=rp.permission_id WHERE rp.role_id=role.id),'[]'::jsonb))
		 FROM user_roles ur JOIN roles role ON role.id=ur.role_id
		 WHERE ur.organization_id=$1::uuid AND ur.user_id=$2::uuid AND ur.role_id=$3::uuid
		 AND ur.valid_from<=statement_timestamp() AND (ur.expires_at IS NULL OR ur.expires_at>statement_timestamp())
		 AND EXISTS(SELECT 1 FROM users u WHERE u.organization_id=ur.organization_id AND u.id=ur.user_id
		   AND u.status='active' AND u.deleted_at IS NULL)`, org, subject, resource).Scan(&raw)
		if err != nil {
			return nil, "", err
		}
	} else {
		var id string
		if err := tx.QueryRow(ctx, `SELECT id FROM user_entity_permissions WHERE organization_id=$1::uuid AND user_id=$2::uuid AND id=$3::uuid AND status='approved'
		 AND valid_from<=statement_timestamp() AND expires_at>statement_timestamp()
		 AND EXISTS(SELECT 1 FROM users u WHERE u.organization_id=$1::uuid AND u.id=$2::uuid AND u.status='active' AND u.deleted_at IS NULL) FOR UPDATE`, org, subject, resource).Scan(&id); err != nil {
			return nil, "", err
		}
		if err := tx.QueryRow(ctx, `SELECT to_jsonb(grant_row) FROM user_entity_permissions grant_row WHERE organization_id=$1::uuid AND user_id=$2::uuid AND id=$3::uuid`, org, subject, resource).Scan(&raw); err != nil {
			return nil, "", err
		}
	}
	var canonical map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, "", err
	}
	if len(encoded) > 65536 {
		return nil, "", ErrAccessGovernanceBound
	}
	hash := sha256.Sum256(encoded)
	return encoded, hex.EncodeToString(hash[:]), nil
}

func (r *accessGovernanceRepo) GetCampaign(ctx context.Context, org, id string) (*models.AccessReviewCampaign, error) {
	return scanGovernanceCampaign(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `SELECT `+campaignColumns+` FROM access_review_campaigns WHERE organization_id=$1::uuid AND id=$2::uuid`, org, id))
}
func (r *accessGovernanceRepo) ListCampaigns(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessReviewCampaign, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM access_review_campaigns WHERE organization_id=$1::uuid`, org).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+campaignColumns+` FROM access_review_campaigns WHERE organization_id=$1::uuid ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, org, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.AccessReviewCampaign{}
	for rows.Next() {
		c, err := scanGovernanceCampaign(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *c)
	}
	return items, count, rows.Err()
}
func (r *accessGovernanceRepo) ListItems(ctx context.Context, org, campaign string, p models.PaginationRequest) ([]models.AccessReviewItem, int, error) {
	if _, err := r.GetCampaign(ctx, org, campaign); err != nil {
		return nil, 0, err
	}
	q := database.QuerierFromContext(ctx, r.pool)
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM access_review_items WHERE organization_id=$1::uuid AND campaign_id=$2::uuid`, org, campaign).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT item.id,item.campaign_id,item.subject_id,item.kind,item.resource_id,item.snapshot,item.snapshot_sha256,to_jsonb(decision)
	 FROM access_review_items item LEFT JOIN access_review_decisions decision ON decision.organization_id=item.organization_id AND decision.item_id=item.id
	 WHERE item.organization_id=$1::uuid AND item.campaign_id=$2::uuid ORDER BY item.id LIMIT $3 OFFSET $4`, org, campaign, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.AccessReviewItem{}
	for rows.Next() {
		var item models.AccessReviewItem
		var decision []byte
		if err := rows.Scan(&item.ID, &item.CampaignID, &item.SubjectID, &item.Kind, &item.ResourceID, &item.Snapshot, &item.SnapshotSHA256, &decision); err != nil {
			return nil, 0, err
		}
		if len(decision) > 0 {
			item.Decision = new(models.AccessReviewDecision)
			if err := json.Unmarshal(decision, item.Decision); err != nil {
				return nil, 0, err
			}
		}
		items = append(items, item)
	}
	return items, count, rows.Err()
}

const decisionColumns = `id,item_id,reviewer_id,decision,reason,request_id,snapshot_sha256,decided_at`

func scanGovernanceDecision(row interface{ Scan(...any) error }) (*models.AccessReviewDecision, error) {
	d := new(models.AccessReviewDecision)
	err := row.Scan(&d.ID, &d.ItemID, &d.ReviewerID, &d.Decision, &d.Reason, &d.RequestID, &d.SnapshotSHA256, &d.DecidedAt)
	return d, err
}
func (r *accessGovernanceRepo) DecideItem(ctx context.Context, org, campaign, itemID, actor string, in models.AccessReviewDecisionInput) (*models.AccessReviewDecision, error) {
	var result *models.AccessReviewDecision
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		c, err := scanGovernanceCampaign(tx.QueryRow(ctx, `SELECT `+campaignColumns+` FROM access_review_campaigns WHERE organization_id=$1::uuid AND id=$2::uuid FOR UPDATE`, org, campaign))
		if err != nil {
			return err
		}
		var item models.AccessReviewItem
		if err := tx.QueryRow(ctx, `SELECT id,subject_id,kind,resource_id,snapshot_sha256 FROM access_review_items WHERE organization_id=$1::uuid AND campaign_id=$2::uuid AND id=$3::uuid`, org, campaign, itemID).Scan(&item.ID, &item.SubjectID, &item.Kind, &item.ResourceID, &item.SnapshotSHA256); err != nil {
			return err
		}
		if actor != c.ReviewerID || actor == item.SubjectID || actor == c.CreatedBy {
			return ErrAccessGovernanceSeparation
		}
		previous, err := scanGovernanceDecision(tx.QueryRow(ctx, `SELECT `+decisionColumns+` FROM access_review_decisions WHERE organization_id=$1::uuid AND (item_id=$2::uuid OR request_id=$3)`, org, itemID, in.RequestID))
		if err == nil {
			if previous.ItemID != itemID || previous.ReviewerID != actor || previous.Decision != in.Decision || previous.Reason != in.Reason || previous.RequestID != in.RequestID || previous.SnapshotSHA256 != in.SnapshotSHA256 {
				return ErrAccessGovernanceConflict
			}
			result = previous
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if c.Status != "open" || item.SnapshotSHA256 != in.SnapshotSHA256 {
			return ErrAccessGovernanceConflict
		}
		_, hash, err := governanceSnapshot(ctx, tx, org, item.SubjectID, item.Kind, item.ResourceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccessGovernanceConflict
		}
		if err != nil {
			return err
		}
		if hash != item.SnapshotSHA256 {
			return ErrAccessGovernanceConflict
		}
		if item.Kind == "object_grant" {
			var self bool
			if err := tx.QueryRow(ctx, `SELECT COALESCE(granted_by=$3::uuid,false) OR COALESCE(approved_by=$3::uuid,false) FROM user_entity_permissions WHERE organization_id=$1::uuid AND id=$2::uuid`, org, item.ResourceID, actor).Scan(&self); err != nil {
				return err
			}
			if self {
				return ErrAccessGovernanceSeparation
			}
		}
		if in.Decision == "revoke" {
			if item.Kind == "role_assignment" {
				role, err := getManagedRoleWithQuerier(ctx, tx, org, item.ResourceID)
				if err != nil {
					return err
				}
				if managedRoleHasPermission(role.Permissions, "settings", "configure") {
					if lockout, err := removingAdministrativeGrantWouldLockOut(ctx, tx, org, item.ResourceID, item.SubjectID); err != nil {
						return err
					} else if lockout {
						return ErrLastTenantAdministrator
					}
				}
				if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE organization_id=$1::uuid AND user_id=$2::uuid AND role_id=$3::uuid`, org, item.SubjectID, item.ResourceID); err != nil {
					return err
				}
			} else {
				if _, err := tx.Exec(ctx, `UPDATE user_entity_permissions SET status='revoked',revoked_by=$3::uuid,revoked_at=statement_timestamp(),revocation_reason=$4,version=version+1,updated_at=statement_timestamp() WHERE organization_id=$1::uuid AND id=$2::uuid`, org, item.ResourceID, actor, in.Reason); err != nil {
					return err
				}
			}
		}
		result, err = scanGovernanceDecision(tx.QueryRow(ctx, `INSERT INTO access_review_decisions(organization_id,item_id,reviewer_id,decision,reason,request_id,snapshot_sha256) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7) RETURNING `+decisionColumns, org, itemID, actor, in.Decision, in.Reason, in.RequestID, in.SnapshotSHA256))
		if err != nil {
			return classifyGovernance(err)
		}
		return r.event(ctx, tx, org, itemID, actor, "review_decided", in.Reason, map[string]any{"campaign_id": campaign, "decision": result, "subject_id": item.SubjectID, "kind": item.Kind, "resource_id": item.ResourceID})
	})
	return result, err
}
func (r *accessGovernanceRepo) TransitionCampaign(ctx context.Context, org, id, actor, status string, in models.AccessGovernanceTransitionInput) (*models.AccessReviewCampaign, error) {
	var result *models.AccessReviewCampaign
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if err := governanceLock(ctx, tx, org, actor); err != nil {
			return err
		}
		var err error
		result, err = scanGovernanceCampaign(tx.QueryRow(ctx, `UPDATE access_review_campaigns SET status=$4,version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND status='open' RETURNING `+campaignColumns, org, id, in.ExpectedVersion, status))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccessGovernanceConflict
		}
		if err != nil {
			return classifyGovernance(err)
		}
		return r.event(ctx, tx, org, id, actor, "campaign_"+status, in.Reason, map[string]any{"campaign": result})
	})
	return result, err
}

func (r *accessGovernanceRepo) event(ctx context.Context, tx pgx.Tx, org, id, actor, typ, reason string, details map[string]any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO access_governance_events(organization_id,entity_id,event_type,actor_id,reason,details) VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6::jsonb)`, org, id, typ, actor, reason, raw); err != nil {
		return err
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", org, map[string]any{"type": "access.governance." + typ, "severity": "medium", "org_id": org, "entity_type": "access_governance", "entity_id": id, "data": details, "timestamp": time.Now().UTC()})
	if err != nil {
		return err
	}
	envelope.CausationID = id
	return r.outbox.Enqueue(ctx, tx, r.queue, envelope)
}
func classifyGovernance(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505":
			return ErrAccessGovernanceConflict
		case "23503", "23514", "P0002":
			return ErrAccessGovernanceInvalid
		}
	}
	return fmt.Errorf("persist access governance: %w", err)
}
