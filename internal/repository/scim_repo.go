package repository

import (
	"context"
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
	protocol "github.com/complianceforge/platform/internal/scim"
)

var (
	ErrSCIMNotFound     = errors.New("SCIM resource not found")
	ErrSCIMConflict     = errors.New("SCIM resource conflict")
	ErrSCIMVersion      = errors.New("SCIM resource version conflict")
	ErrSCIMLastAdmin    = errors.New("SCIM operation would remove the final tenant administrator")
	ErrSCIMDynamicGroup = errors.New("SCIM cannot mutate a dynamic group")
)

type SCIMRepository interface {
	CreateSCIMToken(context.Context, string, string, *models.SCIMToken, string) (*models.SCIMToken, error)
	RotateSCIMToken(context.Context, string, string, string, int64, string, string, string, *time.Time) (*models.SCIMToken, error)
	RevokeSCIMToken(context.Context, string, string, string, int64, string) error
	ListSCIMTokens(context.Context, string, models.PaginationRequest) ([]models.SCIMToken, int, error)
	ResolveSCIMTokenTenant(context.Context, string) (string, string, error)
	GetSCIMTokenForAuthentication(context.Context, string, string, string) (*models.SCIMToken, error)
	RecordSCIMTokenUse(context.Context, string, string, string, int64, string, time.Time) error

	CreateSCIMUser(context.Context, string, string, string, *models.SCIMUser) (*models.SCIMUser, error)
	GetSCIMUser(context.Context, string, string) (*models.SCIMUser, error)
	ListSCIMUsers(context.Context, string, models.SCIMListRequest, protocol.Filter) ([]models.SCIMUser, int, error)
	ReplaceSCIMUser(context.Context, string, string, string, string, int64, string, *models.SCIMUser) (*models.SCIMUser, error)
	DeleteSCIMUser(context.Context, string, string, string, string, int64) error
	CreateSCIMGroup(context.Context, string, string, string, *models.SCIMGroup) (*models.SCIMGroup, error)
	GetSCIMGroup(context.Context, string, string) (*models.SCIMGroup, error)
	ListSCIMGroups(context.Context, string, models.SCIMListRequest, protocol.Filter) ([]models.SCIMGroup, int, error)
	ReplaceSCIMGroup(context.Context, string, string, string, string, int64, string, *models.SCIMGroup) (*models.SCIMGroup, error)
	DeleteSCIMGroup(context.Context, string, string, string, string, int64) error
}

type scimRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

func NewSCIMRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (SCIMRepository, error) {
	if pool == nil {
		return nil, errors.New("SCIM database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("SCIM outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("SCIM outbox queue is required")
	}
	return &scimRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

func (r *scimRepo) withTenant(ctx context.Context, organizationID string, operation func(context.Context, database.Querier) error) error {
	return database.WithTenantConnection(ctx, r.pool, organizationID, func(scoped context.Context) error {
		return operation(scoped, database.QuerierFromContext(scoped, r.pool))
	})
}

type scimRowScanner interface{ Scan(...any) error }

// #nosec G101 -- Static SQL column names only; no hardcoded credential. token_hash names the persisted one-way hash column.
const scimTokenColumns = `token.id,token.organization_id,token.name,token.token_prefix,token.token_hash,
	token.scopes,token.rate_limit_per_minute,token.expires_at,token.last_used_at,
	COALESCE(token.last_used_ip::text,''),token.is_active,token.created_by,token.revoked_by,
	token.revoked_at,COALESCE(token.revoke_reason,''),token.version,token.created_at,token.updated_at`

func scanSCIMToken(row scimRowScanner) (*models.SCIMToken, error) {
	item := &models.SCIMToken{}
	if err := row.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Prefix, &item.TokenHash,
		&item.Scopes, &item.RateLimitPerMinute, &item.ExpiresAt, &item.LastUsedAt, &item.LastUsedIP,
		&item.Active, &item.CreatedBy, &item.RevokedBy, &item.RevokedAt, &item.RevokeReason,
		&item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if item.Scopes == nil {
		item.Scopes = []string{}
	}
	return item, nil
}

func (r *scimRepo) recordTokenEvent(ctx context.Context, tx pgx.Tx, token *models.SCIMToken, actorID, eventType, reason string, details any) error {
	detailsJSON, err := marshalSCIMState(details)
	if err != nil {
		return err
	}
	if detailsJSON == nil {
		detailsJSON = []byte(`{}`)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO scim_token_events(
		organization_id,token_id,event_type,actor_user_id,reason,token_version,details)
		VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::jsonb)`, token.OrganizationID, token.ID,
		eventType, actorID, reason, token.Version, detailsJSON); err != nil {
		return fmt.Errorf("append SCIM token event: %w", err)
	}
	return r.enqueueSCIMEvent(ctx, tx, token.OrganizationID, token.ID, "Token", token.ID,
		"token_"+eventType, token.Version, actorID)
}

func (r *scimRepo) recordResourceEvent(ctx context.Context, tx pgx.Tx, organizationID, tokenID, actorID,
	resourceType, resourceID, eventType string, version int64, before, after any) error {
	beforeJSON, err := marshalSCIMState(before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalSCIMState(after)
	if err != nil {
		return err
	}
	reason := "SCIM " + strings.ToLower(eventType) + " through scoped bearer token"
	if _, err := tx.Exec(ctx, `INSERT INTO scim_resource_events(
		organization_id,token_id,actor_user_id,resource_type,resource_id,event_type,resource_version,
		reason,before_state,after_state) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6,$7,$8,$9::jsonb,$10::jsonb)`,
		organizationID, tokenID, actorID, resourceType, resourceID, eventType, version, reason,
		beforeJSON, afterJSON); err != nil {
		return fmt.Errorf("append SCIM resource event: %w", err)
	}
	return r.enqueueSCIMEvent(ctx, tx, organizationID, tokenID, resourceType, resourceID, eventType, version, actorID)
}

func (r *scimRepo) enqueueSCIMEvent(ctx context.Context, tx pgx.Tx, organizationID, tokenID, resourceType,
	resourceID, eventType string, version int64, actorID string) error {
	severity := "medium"
	if eventType == "deprovisioned" || eventType == "deleted" || eventType == "token_revoked" || eventType == "token_auto_revoked" {
		severity = "high"
	}
	payload := map[string]any{
		"type": "scim." + strings.ToLower(resourceType) + "." + eventType, "severity": severity,
		"org_id": organizationID, "entity_type": strings.ToLower(resourceType), "entity_id": resourceID,
		"entity_ref": resourceID, "data": map[string]any{"token_id": tokenID, "actor_user_id": actorID,
			"resource_type": resourceType, "resource_id": resourceID, "version": version},
		"timestamp": time.Now().UTC(),
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", organizationID, payload)
	if err != nil {
		return fmt.Errorf("create SCIM outbox envelope: %w", err)
	}
	envelope.CausationID = resourceID
	envelope.Metadata = map[string]string{"resource_type": resourceType, "resource_id": resourceID,
		"event_type": eventType, "scim_token_id": tokenID}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue SCIM event: %w", err)
	}
	return nil
}

func marshalSCIMState(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode SCIM audit state: %w", err)
	}
	if string(encoded) == "null" {
		return nil, nil
	}
	return encoded, nil
}

func classifySCIMRead(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSCIMNotFound
	}
	return err
}

func classifySCIMWrite(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23503", "23505", "23514":
			return fmt.Errorf("%w: %s", ErrSCIMConflict, postgresError.ConstraintName)
		}
	}
	return err
}
