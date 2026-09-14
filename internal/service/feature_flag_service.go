package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrFeatureFlagInvalid        = errors.New("feature flag request is invalid")
	ErrFeatureFlagNotFound       = errors.New("feature flag capability or override was not found")
	ErrFeatureFlagConflict       = errors.New("feature flag override conflicts with current state")
	ErrSubscriptionLimitExceeded = errors.New("subscription entitlement limit is exhausted")
)

var featureFlagKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,99}$`)

type FeatureFlagStore interface {
	ListCapabilities(context.Context) ([]models.ProductCapability, error)
	GetEntitlements(context.Context, string) (*models.EntitlementSnapshot, error)
	ListOverrides(context.Context, string) ([]models.TenantFeatureFlagOverride, error)
	UpsertOverride(context.Context, string, string, string, string, models.FeatureFlagOverrideInput) (*models.TenantFeatureFlagOverride, error)
	ResetOverride(context.Context, string, string, string, string, models.FeatureFlagResetInput) error
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.FeatureFlagChangeEvent, int, error)
}

type FeatureFlagService struct {
	store  FeatureFlagStore
	logger zerolog.Logger
	now    func() time.Time
}

func NewFeatureFlagService(store FeatureFlagStore, logger zerolog.Logger) *FeatureFlagService {
	return NewFeatureFlagServiceWithClock(store, logger, time.Now)
}

func NewFeatureFlagServiceWithClock(store FeatureFlagStore, logger zerolog.Logger, now func() time.Time) *FeatureFlagService {
	if now == nil {
		now = time.Now
	}
	return &FeatureFlagService{
		store: store, logger: logger.With().Str("service", "feature_flags").Logger(), now: now,
	}
}

func (s *FeatureFlagService) ListEvaluations(ctx context.Context, organizationID string) ([]models.FeatureFlagEvaluation, error) {
	if err := validateFeatureFlagOrganization(organizationID); err != nil {
		return nil, err
	}
	capabilities, overrides, entitlements, err := s.loadEvaluationState(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return evaluateCapabilities(organizationID, capabilities, overrides, entitlements, s.now().UTC()), nil
}

func (s *FeatureFlagService) Evaluate(ctx context.Context, organizationID, capabilityKey string) (*models.FeatureFlagEvaluation, error) {
	capabilityKey = normalizeFeatureFlagKey(capabilityKey)
	if err := validateFeatureFlagObject(organizationID, capabilityKey); err != nil {
		return nil, err
	}
	evaluations, err := s.ListEvaluations(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	for index := range evaluations {
		if evaluations[index].Capability.Key == capabilityKey {
			return &evaluations[index], nil
		}
	}
	return nil, ErrFeatureFlagNotFound
}

func (s *FeatureFlagService) GetEntitlements(ctx context.Context, organizationID string) (*models.EntitlementSnapshot, error) {
	if err := validateFeatureFlagOrganization(organizationID); err != nil {
		return nil, err
	}
	snapshot, err := s.store.GetEntitlements(ctx, organizationID)
	if err != nil {
		return nil, mapFeatureFlagError(err)
	}
	snapshot.EvaluatedAt = s.now().UTC()
	return snapshot, nil
}

func (s *FeatureFlagService) CheckLimit(
	ctx context.Context, organizationID, metric string, requested int64,
) (*models.EntitlementLimitDecision, error) {
	metric = strings.TrimSpace(strings.ToLower(metric))
	if err := validateFeatureFlagOrganization(organizationID); err != nil {
		return nil, err
	}
	if requested < 1 {
		return nil, fmt.Errorf("%w: requested usage must be positive", ErrFeatureFlagInvalid)
	}
	snapshot, err := s.GetEntitlements(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	limit, known := snapshot.Limits[metric]
	if !known {
		return nil, fmt.Errorf("%w: unknown entitlement metric %q", ErrFeatureFlagInvalid, metric)
	}
	usage := snapshot.Usage[metric]
	decision := &models.EntitlementLimitDecision{
		Metric: metric, Limit: limit, Usage: usage, Requested: requested,
		Allowed: true, Reason: "within_limit",
	}
	if limit == 0 {
		decision.Remaining = -1
		decision.Reason = "unlimited"
		return decision, nil
	}
	decision.Remaining = maxInt64(limit-usage, 0)
	if requested > decision.Remaining {
		decision.Allowed = false
		decision.Reason = "limit_exceeded"
	}
	return decision, nil
}

func (s *FeatureFlagService) UpsertOverride(
	ctx context.Context, organizationID, capabilityKey, actorID, requestID string, input models.FeatureFlagOverrideInput,
) (*models.TenantFeatureFlagOverride, error) {
	capabilityKey = normalizeFeatureFlagKey(capabilityKey)
	requestID = strings.TrimSpace(requestID)
	if err := validateFeatureFlagIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	if err := validateFeatureFlagKey(capabilityKey); err != nil {
		return nil, err
	}
	normalizeFeatureFlagOverrideInput(&input)
	if err := validateFeatureFlagOverrideInput(input, requestID); err != nil {
		return nil, err
	}
	result, err := s.store.UpsertOverride(ctx, organizationID, capabilityKey, actorID, requestID, input)
	if err != nil {
		return nil, mapFeatureFlagError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("capability_key", capabilityKey).
		Int64("version", result.Version).Msg("tenant feature flag override saved")
	return result, nil
}

func (s *FeatureFlagService) ResetOverride(
	ctx context.Context, organizationID, capabilityKey, actorID, requestID string, input models.FeatureFlagResetInput,
) error {
	capabilityKey = normalizeFeatureFlagKey(capabilityKey)
	requestID = strings.TrimSpace(requestID)
	if err := validateFeatureFlagIdentity(organizationID, actorID); err != nil {
		return err
	}
	if err := validateFeatureFlagKey(capabilityKey); err != nil {
		return err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ExpectedVersion < 1 || utf8.RuneCountInString(input.Reason) < 3 || utf8.RuneCountInString(input.Reason) > 1000 || len(requestID) > 160 {
		return fmt.Errorf("%w: positive expected_version, a 3-1000 character reason, and valid request id are required", ErrFeatureFlagInvalid)
	}
	if err := s.store.ResetOverride(ctx, organizationID, capabilityKey, actorID, requestID, input); err != nil {
		return mapFeatureFlagError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("capability_key", capabilityKey).
		Msg("tenant feature flag override reset")
	return nil
}

func (s *FeatureFlagService) ListEvents(
	ctx context.Context, organizationID, capabilityKey string, pagination models.PaginationRequest,
) ([]models.FeatureFlagChangeEvent, int, error) {
	capabilityKey = normalizeFeatureFlagKey(capabilityKey)
	if err := validateFeatureFlagObject(organizationID, capabilityKey); err != nil {
		return nil, 0, err
	}
	pagination = normalizeFeatureFlagPagination(pagination)
	items, total, err := s.store.ListEvents(ctx, organizationID, capabilityKey, pagination)
	if err != nil {
		return nil, 0, mapFeatureFlagError(err)
	}
	return items, total, nil
}

func (s *FeatureFlagService) loadEvaluationState(
	ctx context.Context, organizationID string,
) ([]models.ProductCapability, []models.TenantFeatureFlagOverride, *models.EntitlementSnapshot, error) {
	capabilities, err := s.store.ListCapabilities(ctx)
	if err != nil {
		return nil, nil, nil, mapFeatureFlagError(err)
	}
	overrides, err := s.store.ListOverrides(ctx, organizationID)
	if err != nil {
		return nil, nil, nil, mapFeatureFlagError(err)
	}
	entitlements, err := s.store.GetEntitlements(ctx, organizationID)
	if err != nil {
		return nil, nil, nil, mapFeatureFlagError(err)
	}
	entitlements.EvaluatedAt = s.now().UTC()
	return capabilities, overrides, entitlements, nil
}

type featureEvaluationEngine struct {
	organizationID string
	capabilities   map[string]models.ProductCapability
	overrides      map[string]models.TenantFeatureFlagOverride
	entitlements   *models.EntitlementSnapshot
	evaluatedAt    time.Time
	memo           map[string]models.FeatureFlagEvaluation
	visiting       map[string]bool
}

func evaluateCapabilities(
	organizationID string, capabilities []models.ProductCapability, overrides []models.TenantFeatureFlagOverride,
	entitlements *models.EntitlementSnapshot, evaluatedAt time.Time,
) []models.FeatureFlagEvaluation {
	engine := &featureEvaluationEngine{
		organizationID: organizationID,
		capabilities:   make(map[string]models.ProductCapability, len(capabilities)),
		overrides:      make(map[string]models.TenantFeatureFlagOverride, len(overrides)),
		entitlements:   entitlements,
		evaluatedAt:    evaluatedAt,
		memo:           make(map[string]models.FeatureFlagEvaluation, len(capabilities)),
		visiting:       make(map[string]bool, len(capabilities)),
	}
	keys := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		engine.capabilities[capability.Key] = capability
		keys = append(keys, capability.Key)
	}
	for _, override := range overrides {
		engine.overrides[override.CapabilityKey] = override
	}
	sort.Strings(keys)
	items := make([]models.FeatureFlagEvaluation, 0, len(keys))
	for _, key := range keys {
		items = append(items, engine.evaluate(key))
	}
	return items
}

func (e *featureEvaluationEngine) evaluate(key string) models.FeatureFlagEvaluation {
	if cached, ok := e.memo[key]; ok {
		return cached
	}
	capability, exists := e.capabilities[key]
	if !exists {
		return models.FeatureFlagEvaluation{
			Enabled: false, Entitled: false, InRollout: false, Source: "catalogue",
			Reason: "capability_missing", BlockingCapability: key, Variant: map[string]any{}, EvaluatedAt: e.evaluatedAt,
		}
	}
	result := models.FeatureFlagEvaluation{
		Capability: capability, Source: "global_default", Variant: map[string]any{},
		EffectiveRollout: capability.RolloutBasisPoints, EvaluatedAt: e.evaluatedAt,
	}
	if override, ok := e.overrides[key]; ok {
		overrideCopy := override
		result.Override = &overrideCopy
	}
	result.Entitled = capabilityIsEntitled(capability, e.entitlements)
	if capability.KillSwitch {
		result.Reason = "global_kill_switch"
		return e.remember(key, result)
	}
	if !result.Entitled {
		result.Reason = "subscription_denied"
		return e.remember(key, result)
	}

	enabled := capability.DefaultEnabled
	if override, ok := e.overrides[key]; ok && featureFlagOverrideIsActive(override, e.evaluatedAt) {
		enabled = override.Enabled
		result.Source = "tenant_override"
		if override.RolloutBasisPoints != nil {
			result.EffectiveRollout = *override.RolloutBasisPoints
		}
		result.Variant = cloneFeatureFlagVariant(override.Variant)
	}
	if !enabled {
		if result.Source == "tenant_override" {
			result.Reason = "tenant_disabled"
		} else {
			result.Reason = "global_disabled"
		}
		return e.remember(key, result)
	}
	if e.visiting[key] {
		result.Reason = "prerequisite_cycle"
		result.BlockingCapability = key
		return result
	}
	e.visiting[key] = true
	defer delete(e.visiting, key)
	for _, prerequisite := range capability.Prerequisites {
		prerequisiteResult := e.evaluate(prerequisite)
		if !prerequisiteResult.Enabled {
			result.Reason = "prerequisite_disabled"
			result.BlockingCapability = prerequisite
			return e.remember(key, result)
		}
	}
	result.InRollout = featureFlagRolloutBucket(e.organizationID, key) < result.EffectiveRollout
	if !result.InRollout {
		result.Reason = "outside_rollout"
		return e.remember(key, result)
	}
	result.Enabled = true
	result.Reason = "enabled"
	return e.remember(key, result)
}

func (e *featureEvaluationEngine) remember(key string, value models.FeatureFlagEvaluation) models.FeatureFlagEvaluation {
	e.memo[key] = value
	return value
}

func capabilityIsEntitled(capability models.ProductCapability, entitlements *models.EntitlementSnapshot) bool {
	if entitlements == nil {
		return false
	}
	if entitlements.Source != "organization_tier" && entitlements.SubscriptionStatus != "active" && entitlements.SubscriptionStatus != "trialing" {
		return false
	}
	if capability.RequiredPlanFeature != "" && entitlements.Source != "organization_tier" {
		enabled, exists := entitlements.Features[capability.RequiredPlanFeature]
		return exists && enabled
	}
	return entitlementTierRank(entitlements.Tier) >= entitlementTierRank(capability.MinimumTier)
}

func entitlementTierRank(tier string) int {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "starter":
		return 1
	case "professional":
		return 2
	case "enterprise":
		return 3
	case "unlimited":
		return 4
	default:
		return 0
	}
}

func featureFlagOverrideIsActive(override models.TenantFeatureFlagOverride, at time.Time) bool {
	if override.StartsAt != nil && at.Before(*override.StartsAt) {
		return false
	}
	return override.ExpiresAt == nil || at.Before(*override.ExpiresAt)
}

func featureFlagRolloutBucket(organizationID, capabilityKey string) int {
	digest := sha256.Sum256([]byte(organizationID + "\x00" + capabilityKey))
	return int(binary.BigEndian.Uint32(digest[:4]) % 10000)
}

func cloneFeatureFlagVariant(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func validateFeatureFlagOrganization(organizationID string) error {
	if _, err := uuid.Parse(strings.TrimSpace(organizationID)); err != nil {
		return fmt.Errorf("%w: organization id must be a UUID", ErrFeatureFlagInvalid)
	}
	return nil
}

func validateFeatureFlagIdentity(organizationID, actorID string) error {
	if err := validateFeatureFlagOrganization(organizationID); err != nil {
		return err
	}
	if _, err := uuid.Parse(strings.TrimSpace(actorID)); err != nil {
		return fmt.Errorf("%w: actor id must be a UUID", ErrFeatureFlagInvalid)
	}
	return nil
}

func validateFeatureFlagObject(organizationID, capabilityKey string) error {
	if err := validateFeatureFlagOrganization(organizationID); err != nil {
		return err
	}
	return validateFeatureFlagKey(capabilityKey)
}

func validateFeatureFlagKey(capabilityKey string) error {
	if !featureFlagKeyPattern.MatchString(capabilityKey) {
		return fmt.Errorf("%w: capability key is invalid", ErrFeatureFlagInvalid)
	}
	return nil
}

func normalizeFeatureFlagKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeFeatureFlagOverrideInput(input *models.FeatureFlagOverrideInput) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Variant == nil {
		input.Variant = map[string]any{}
	}
	if input.StartsAt != nil {
		value := input.StartsAt.UTC()
		input.StartsAt = &value
	}
	if input.ExpiresAt != nil {
		value := input.ExpiresAt.UTC()
		input.ExpiresAt = &value
	}
}

func validateFeatureFlagOverrideInput(input models.FeatureFlagOverrideInput, requestID string) error {
	if utf8.RuneCountInString(input.Reason) < 3 || utf8.RuneCountInString(input.Reason) > 1000 {
		return fmt.Errorf("%w: reason must contain 3-1000 characters", ErrFeatureFlagInvalid)
	}
	if input.RolloutBasisPoints != nil && (*input.RolloutBasisPoints < 0 || *input.RolloutBasisPoints > 10000) {
		return fmt.Errorf("%w: rollout_basis_points must be between 0 and 10000", ErrFeatureFlagInvalid)
	}
	if input.ExpectedVersion != nil && *input.ExpectedVersion < 1 {
		return fmt.Errorf("%w: expected_version must be positive when supplied", ErrFeatureFlagInvalid)
	}
	if input.StartsAt != nil && input.ExpiresAt != nil && !input.ExpiresAt.After(*input.StartsAt) {
		return fmt.Errorf("%w: expires_at must be after starts_at", ErrFeatureFlagInvalid)
	}
	if len(requestID) > 160 {
		return fmt.Errorf("%w: request id must not exceed 160 bytes", ErrFeatureFlagInvalid)
	}
	encoded, err := json.Marshal(input.Variant)
	if err != nil {
		return fmt.Errorf("%w: variant must be valid JSON", ErrFeatureFlagInvalid)
	}
	if len(encoded) > 16*1024 {
		return fmt.Errorf("%w: variant must not exceed 16 KiB", ErrFeatureFlagInvalid)
	}
	return nil
}

func normalizeFeatureFlagPagination(value models.PaginationRequest) models.PaginationRequest {
	if value.Page < 1 {
		value.Page = 1
	}
	if value.PageSize < 1 {
		value.PageSize = 20
	}
	if value.PageSize > 100 {
		value.PageSize = 100
	}
	return value
}

func mapFeatureFlagError(err error) error {
	switch {
	case errors.Is(err, repository.ErrCapabilityNotFound), errors.Is(err, pgx.ErrNoRows):
		return ErrFeatureFlagNotFound
	case errors.Is(err, repository.ErrFeatureFlagVersionConflict):
		return ErrFeatureFlagConflict
	default:
		return err
	}
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
