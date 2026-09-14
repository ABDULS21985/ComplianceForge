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
	ErrCapabilityNotFound         = errors.New("product capability not found")
	ErrFeatureFlagVersionConflict = errors.New("feature flag override version conflict")
)

type FeatureFlagRepository interface {
	ListCapabilities(context.Context) ([]models.ProductCapability, error)
	GetEntitlements(context.Context, string) (*models.EntitlementSnapshot, error)
	ListOverrides(context.Context, string) ([]models.TenantFeatureFlagOverride, error)
	UpsertOverride(context.Context, string, string, string, string, models.FeatureFlagOverrideInput) (*models.TenantFeatureFlagOverride, error)
	ResetOverride(context.Context, string, string, string, string, models.FeatureFlagResetInput) error
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.FeatureFlagChangeEvent, int, error)
}

type featureFlagRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

func NewFeatureFlagRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (FeatureFlagRepository, error) {
	if pool == nil {
		return nil, errors.New("feature flag database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("feature flag outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("feature flag outbox queue is required")
	}
	return &featureFlagRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

var _ FeatureFlagRepository = (*featureFlagRepo)(nil)

func (r *featureFlagRepo) ListCapabilities(ctx context.Context) ([]models.ProductCapability, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT id,capability_key,display_name,description,owner_team,maturity,minimum_tier::text,
			COALESCE(required_plan_feature,''),prerequisites,default_enabled,kill_switch,
			rollout_basis_points,is_active,version,created_at,updated_at
		FROM product_capabilities WHERE is_active ORDER BY capability_key`)
	if err != nil {
		return nil, fmt.Errorf("list product capabilities: %w", err)
	}
	defer rows.Close()
	items := make([]models.ProductCapability, 0, 32)
	for rows.Next() {
		var item models.ProductCapability
		if err := rows.Scan(
			&item.ID, &item.Key, &item.DisplayName, &item.Description, &item.OwnerTeam,
			&item.Maturity, &item.MinimumTier, &item.RequiredPlanFeature, &item.Prerequisites,
			&item.DefaultEnabled, &item.KillSwitch, &item.RolloutBasisPoints, &item.IsActive,
			&item.Version, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan product capability: %w", err)
		}
		if item.Prerequisites == nil {
			item.Prerequisites = []string{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product capabilities: %w", err)
	}
	return items, nil
}

func (r *featureFlagRepo) GetEntitlements(ctx context.Context, organizationID string) (*models.EntitlementSnapshot, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var organizationTier string
	if err := querier.QueryRow(ctx, `SELECT tier::text FROM organizations
		WHERE id=$1::uuid AND deleted_at IS NULL`, organizationID).Scan(&organizationTier); err != nil {
		return nil, fmt.Errorf("load entitlement organization: %w", err)
	}

	snapshot := &models.EntitlementSnapshot{
		OrganizationID: organizationID,
		Source:         "organization_tier",
		Tier:           organizationTier,
		Features:       map[string]bool{},
		Limits:         defaultEntitlementLimits(organizationTier),
		Usage:          map[string]int64{},
	}
	var rawFeatures []byte
	var maxUsers, maxFrameworks, maxRisks, maxVendors, maxStorageGB int64
	err := querier.QueryRow(ctx, `
		SELECT plan.id,plan.name,plan.tier::text,subscription.status,
			plan.features,plan.max_users,plan.max_frameworks,plan.max_risks,
			plan.max_vendors,plan.max_storage_gb
		FROM organization_subscriptions_v2 subscription
		JOIN subscription_plans plan ON plan.id=subscription.plan_id
		WHERE subscription.organization_id=$1::uuid
		LIMIT 1`, organizationID).Scan(
		&snapshot.PlanID, &snapshot.PlanName, &snapshot.Tier, &snapshot.SubscriptionStatus,
		&rawFeatures, &maxUsers, &maxFrameworks, &maxRisks, &maxVendors, &maxStorageGB,
	)
	switch {
	case err == nil:
		snapshot.Source = "subscription_plan"
		snapshot.Limits = entitlementLimits(maxUsers, maxFrameworks, maxRisks, maxVendors, maxStorageGB)
		if err := json.Unmarshal(rawFeatures, &snapshot.Features); err != nil {
			return nil, fmt.Errorf("decode subscription features: %w", err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		if err := r.loadLegacyEntitlements(ctx, querier, snapshot); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("load subscription entitlements: %w", err)
	}
	if snapshot.Features == nil {
		snapshot.Features = map[string]bool{}
	}
	if err := loadEntitlementUsage(ctx, querier, organizationID, snapshot.Usage); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *featureFlagRepo) loadLegacyEntitlements(ctx context.Context, querier database.Querier, snapshot *models.EntitlementSnapshot) error {
	var rawFeatures []byte
	var maxUsers, maxFrameworks int64
	err := querier.QueryRow(ctx, `SELECT plan_name,status::text,max_users,max_frameworks,features_enabled
		FROM organization_subscriptions WHERE organization_id=$1::uuid
		ORDER BY created_at DESC LIMIT 1`, snapshot.OrganizationID).Scan(
		&snapshot.PlanName, &snapshot.SubscriptionStatus, &maxUsers, &maxFrameworks, &rawFeatures,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load legacy subscription entitlements: %w", err)
	}
	snapshot.Source = "legacy_subscription"
	snapshot.Limits["users"] = maxUsers
	snapshot.Limits["frameworks"] = maxFrameworks
	if err := json.Unmarshal(rawFeatures, &snapshot.Features); err != nil {
		return fmt.Errorf("decode legacy subscription features: %w", err)
	}
	return nil
}

func loadEntitlementUsage(ctx context.Context, querier database.Querier, organizationID string, usage map[string]int64) error {
	var users, frameworks, risks, vendors, storageBytes int64
	if err := querier.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM users WHERE organization_id=$1::uuid AND deleted_at IS NULL),
		(SELECT COUNT(*) FROM organization_frameworks WHERE organization_id=$1::uuid),
		(SELECT COUNT(*) FROM risks WHERE organization_id=$1::uuid AND deleted_at IS NULL),
		(SELECT COUNT(*) FROM vendors WHERE organization_id=$1::uuid AND deleted_at IS NULL),
		(SELECT COALESCE(SUM(file_size_bytes),0) FROM control_evidence
		 WHERE organization_id=$1::uuid AND deleted_at IS NULL)`, organizationID).Scan(
		&users, &frameworks, &risks, &vendors, &storageBytes,
	); err != nil {
		return fmt.Errorf("load entitlement usage: %w", err)
	}
	usage["users"], usage["frameworks"], usage["risks"], usage["vendors"], usage["storage_bytes"] =
		users, frameworks, risks, vendors, storageBytes
	return nil
}

func entitlementLimits(users, frameworks, risks, vendors, storageGB int64) map[string]int64 {
	return map[string]int64{
		"users": users, "frameworks": frameworks, "risks": risks, "vendors": vendors,
		"storage_bytes": storageGB * 1024 * 1024 * 1024,
	}
}

func defaultEntitlementLimits(tier string) map[string]int64 {
	switch tier {
	case "professional":
		return entitlementLimits(25, 5, 0, 50, 25)
	case "enterprise":
		return entitlementLimits(100, 9, 0, 0, 100)
	case "unlimited":
		return entitlementLimits(0, 0, 0, 0, 0)
	default:
		return entitlementLimits(5, 3, 50, 10, 5)
	}
}

func (r *featureFlagRepo) ListOverrides(ctx context.Context, organizationID string) ([]models.TenantFeatureFlagOverride, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT organization_id,capability_key,enabled,rollout_basis_points,variant,reason,
			starts_at,expires_at,version,created_by,updated_by,created_at,updated_at
		FROM tenant_feature_flag_overrides WHERE organization_id=$1::uuid
		ORDER BY capability_key`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list tenant feature flag overrides: %w", err)
	}
	defer rows.Close()
	items := make([]models.TenantFeatureFlagOverride, 0)
	for rows.Next() {
		item, err := scanFeatureFlagOverride(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenant feature flag overrides: %w", err)
	}
	return items, nil
}

func (r *featureFlagRepo) UpsertOverride(
	ctx context.Context, organizationID, capabilityKey, actorID, requestID string, input models.FeatureFlagOverrideInput,
) (*models.TenantFeatureFlagOverride, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var result *models.TenantFeatureFlagOverride
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if err := ensureActiveCapability(ctx, tx, capabilityKey); err != nil {
			return err
		}
		before, err := getFeatureFlagOverride(ctx, tx, organizationID, capabilityKey, true)
		eventType := "updated"
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if input.ExpectedVersion != nil {
				return ErrFeatureFlagVersionConflict
			}
			eventType = "created"
			variant, err := json.Marshal(input.Variant)
			if err != nil {
				return fmt.Errorf("encode feature flag variant: %w", err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO tenant_feature_flag_overrides
				(organization_id,capability_key,enabled,rollout_basis_points,variant,reason,
				 starts_at,expires_at,created_by,updated_by)
				VALUES ($1::uuid,$2,$3,$4,$5::jsonb,$6,$7,$8,$9::uuid,$9::uuid)`,
				organizationID, capabilityKey, input.Enabled, input.RolloutBasisPoints, variant,
				input.Reason, input.StartsAt, input.ExpiresAt, actorID,
			); err != nil {
				return fmt.Errorf("create tenant feature flag override: %w", err)
			}
			before = nil
		case err != nil:
			return err
		default:
			if input.ExpectedVersion == nil || *input.ExpectedVersion != before.Version {
				return ErrFeatureFlagVersionConflict
			}
			variant, err := json.Marshal(input.Variant)
			if err != nil {
				return fmt.Errorf("encode feature flag variant: %w", err)
			}
			command, err := tx.Exec(ctx, `UPDATE tenant_feature_flag_overrides SET
				enabled=$3,rollout_basis_points=$4,variant=$5::jsonb,reason=$6,
				starts_at=$7,expires_at=$8,updated_by=$9::uuid,version=version+1
				WHERE organization_id=$1::uuid AND capability_key=$2 AND version=$10`,
				organizationID, capabilityKey, input.Enabled, input.RolloutBasisPoints, variant,
				input.Reason, input.StartsAt, input.ExpiresAt, actorID, *input.ExpectedVersion,
			)
			if err != nil {
				return fmt.Errorf("update tenant feature flag override: %w", err)
			}
			if command.RowsAffected() != 1 {
				return ErrFeatureFlagVersionConflict
			}
		}
		result, err = getFeatureFlagOverride(ctx, tx, organizationID, capabilityKey, false)
		if err != nil {
			return err
		}
		return r.recordFeatureFlagEvent(ctx, tx, eventType, actorID, requestID, input.Reason, before, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *featureFlagRepo) ResetOverride(
	ctx context.Context, organizationID, capabilityKey, actorID, requestID string, input models.FeatureFlagResetInput,
) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		before, err := getFeatureFlagOverride(ctx, tx, organizationID, capabilityKey, true)
		if err != nil {
			return err
		}
		if before.Version != input.ExpectedVersion {
			return ErrFeatureFlagVersionConflict
		}
		command, err := tx.Exec(ctx, `DELETE FROM tenant_feature_flag_overrides
			WHERE organization_id=$1::uuid AND capability_key=$2 AND version=$3`,
			organizationID, capabilityKey, input.ExpectedVersion)
		if err != nil {
			return fmt.Errorf("reset tenant feature flag override: %w", err)
		}
		if command.RowsAffected() != 1 {
			return ErrFeatureFlagVersionConflict
		}
		return r.recordFeatureFlagEvent(ctx, tx, "reset", actorID, requestID, input.Reason, before, nil)
	})
}

func (r *featureFlagRepo) ListEvents(
	ctx context.Context, organizationID, capabilityKey string, pagination models.PaginationRequest,
) ([]models.FeatureFlagChangeEvent, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	if err := ensureActiveCapability(ctx, querier, capabilityKey); err != nil {
		return nil, 0, err
	}
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*) FROM feature_flag_change_events
		WHERE organization_id=$1::uuid AND capability_key=$2`, organizationID, capabilityKey).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count feature flag events: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT id,capability_key,event_type,actor_user_id,
		override_version,reason,before_state,after_state,COALESCE(request_id,''),created_at
		FROM feature_flag_change_events
		WHERE organization_id=$1::uuid AND capability_key=$2
		ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, organizationID, capabilityKey,
		pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list feature flag events: %w", err)
	}
	defer rows.Close()
	items := make([]models.FeatureFlagChangeEvent, 0, pagination.PageSize)
	for rows.Next() {
		var item models.FeatureFlagChangeEvent
		if err := rows.Scan(&item.ID, &item.CapabilityKey, &item.EventType, &item.ActorUserID,
			&item.OverrideVersion, &item.Reason, &item.BeforeState, &item.AfterState,
			&item.RequestID, &item.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan feature flag event: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate feature flag events: %w", err)
	}
	return items, total, nil
}

func getFeatureFlagOverride(
	ctx context.Context, querier database.Querier, organizationID, capabilityKey string, lock bool,
) (*models.TenantFeatureFlagOverride, error) {
	query := `SELECT organization_id,capability_key,enabled,rollout_basis_points,variant,reason,
		starts_at,expires_at,version,created_by,updated_by,created_at,updated_at
		FROM tenant_feature_flag_overrides
		WHERE organization_id=$1::uuid AND capability_key=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanFeatureFlagOverride(querier.QueryRow(ctx, query, organizationID, capabilityKey))
}

func scanFeatureFlagOverride(scanner interface{ Scan(...any) error }) (*models.TenantFeatureFlagOverride, error) {
	var item models.TenantFeatureFlagOverride
	var variant []byte
	if err := scanner.Scan(
		&item.OrganizationID, &item.CapabilityKey, &item.Enabled, &item.RolloutBasisPoints,
		&variant, &item.Reason, &item.StartsAt, &item.ExpiresAt, &item.Version,
		&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(variant, &item.Variant); err != nil {
		return nil, fmt.Errorf("decode feature flag variant: %w", err)
	}
	if item.Variant == nil {
		item.Variant = map[string]any{}
	}
	return &item, nil
}

func ensureActiveCapability(ctx context.Context, querier database.Querier, capabilityKey string) error {
	var exists bool
	if err := querier.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_capabilities
		WHERE capability_key=$1 AND is_active)`, capabilityKey).Scan(&exists); err != nil {
		return fmt.Errorf("check product capability: %w", err)
	}
	if !exists {
		return ErrCapabilityNotFound
	}
	return nil
}

func (r *featureFlagRepo) recordFeatureFlagEvent(
	ctx context.Context, tx pgx.Tx, eventType, actorID, requestID, reason string,
	before, after *models.TenantFeatureFlagOverride,
) error {
	current := after
	if current == nil {
		current = before
	}
	beforeJSON, err := marshalNullableObject(featureFlagOverrideState(before))
	if err != nil {
		return err
	}
	afterJSON, err := marshalNullableObject(featureFlagOverrideState(after))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO feature_flag_change_events
		(organization_id,capability_key,event_type,actor_user_id,override_version,reason,
		 before_state,after_state,request_id)
		VALUES ($1::uuid,$2,$3,$4::uuid,$5,$6,$7::jsonb,$8::jsonb,NULLIF($9,''))`,
		current.OrganizationID, current.CapabilityKey, eventType, actorID, current.Version,
		reason, beforeJSON, afterJSON, requestID,
	); err != nil {
		return fmt.Errorf("append feature flag event: %w", err)
	}
	payload := map[string]any{
		"type": "settings.feature_flag." + eventType, "severity": "medium",
		"org_id": current.OrganizationID, "entity_type": "feature_flag",
		"entity_id": current.CapabilityKey, "entity_ref": current.CapabilityKey,
		"data": map[string]any{
			"capability_key": current.CapabilityKey, "override_version": current.Version,
			"actor_user_id": actorID, "reason": reason,
		},
		"timestamp": time.Now().UTC(),
	}
	envelope, err := queuepkg.NewEnvelope("notification.event", current.OrganizationID, payload)
	if err != nil {
		return fmt.Errorf("create feature flag outbox envelope: %w", err)
	}
	envelope.CausationID = current.CapabilityKey
	envelope.Metadata = map[string]string{
		"entity_type": "feature_flag", "entity_id": current.CapabilityKey, "event_type": eventType,
	}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue feature flag event: %w", err)
	}
	return nil
}

func featureFlagOverrideState(item *models.TenantFeatureFlagOverride) any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"enabled": item.Enabled, "rollout_basis_points": item.RolloutBasisPoints,
		"variant": item.Variant, "starts_at": item.StartsAt, "expires_at": item.ExpiresAt,
		"version": item.Version,
	}
}
