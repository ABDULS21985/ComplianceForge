package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	featureFlagTestOrg   = "10000000-0000-0000-0000-000000000001"
	featureFlagTestActor = "20000000-0000-0000-0000-000000000001"
)

type featureFlagStoreStub struct {
	capabilities []models.ProductCapability
	overrides    []models.TenantFeatureFlagOverride
	entitlements *models.EntitlementSnapshot
	upserted     models.FeatureFlagOverrideInput
	reset        models.FeatureFlagResetInput
	err          error
}

func (s *featureFlagStoreStub) ListCapabilities(context.Context) ([]models.ProductCapability, error) {
	return s.capabilities, s.err
}

func (s *featureFlagStoreStub) GetEntitlements(context.Context, string) (*models.EntitlementSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	copy := *s.entitlements
	return &copy, nil
}

func (s *featureFlagStoreStub) ListOverrides(context.Context, string) ([]models.TenantFeatureFlagOverride, error) {
	return s.overrides, s.err
}

func (s *featureFlagStoreStub) UpsertOverride(
	_ context.Context, organizationID, capabilityKey, actorID, _ string, input models.FeatureFlagOverrideInput,
) (*models.TenantFeatureFlagOverride, error) {
	s.upserted = input
	if s.err != nil {
		return nil, s.err
	}
	return &models.TenantFeatureFlagOverride{
		OrganizationID: organizationID, CapabilityKey: capabilityKey, Enabled: input.Enabled,
		RolloutBasisPoints: input.RolloutBasisPoints, Variant: input.Variant, Reason: input.Reason,
		StartsAt: input.StartsAt, ExpiresAt: input.ExpiresAt, Version: 1, CreatedBy: actorID, UpdatedBy: actorID,
	}, nil
}

func (s *featureFlagStoreStub) ResetOverride(
	_ context.Context, _, _, _, _ string, input models.FeatureFlagResetInput,
) error {
	s.reset = input
	return s.err
}

func (s *featureFlagStoreStub) ListEvents(
	context.Context, string, string, models.PaginationRequest,
) ([]models.FeatureFlagChangeEvent, int, error) {
	return []models.FeatureFlagChangeEvent{}, 0, s.err
}

func TestFeatureFlagEvaluationFailsClosedAcrossEntitlementsOverridesAndPrerequisites(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	rollout := featureFlagRolloutBucket(featureFlagTestOrg, "gradual")
	rolloutBelowBucket := rollout
	if rolloutBelowBucket > 0 {
		rolloutBelowBucket--
	}
	store := &featureFlagStoreStub{
		capabilities: []models.ProductCapability{
			{Key: "core", MinimumTier: "starter", RequiredPlanFeature: "core", DefaultEnabled: true, RolloutBasisPoints: 10000},
			{Key: "dependent", MinimumTier: "starter", RequiredPlanFeature: "dependent", Prerequisites: []string{"core"}, DefaultEnabled: true, RolloutBasisPoints: 10000},
			{Key: "professional", MinimumTier: "professional", RequiredPlanFeature: "professional", DefaultEnabled: true, RolloutBasisPoints: 10000},
			{Key: "killed", MinimumTier: "starter", RequiredPlanFeature: "killed", DefaultEnabled: true, KillSwitch: true, RolloutBasisPoints: 10000},
			{Key: "gradual", MinimumTier: "starter", RequiredPlanFeature: "gradual", DefaultEnabled: true, RolloutBasisPoints: rolloutBelowBucket},
		},
		overrides: []models.TenantFeatureFlagOverride{
			{OrganizationID: featureFlagTestOrg, CapabilityKey: "core", Enabled: false, Version: 2, Variant: map[string]any{}},
			{OrganizationID: featureFlagTestOrg, CapabilityKey: "killed", Enabled: true, Version: 1, Variant: map[string]any{}},
		},
		entitlements: &models.EntitlementSnapshot{
			Source: "subscription_plan", SubscriptionStatus: "active", Tier: "professional",
			Features: map[string]bool{"core": true, "dependent": true, "professional": false, "killed": true, "gradual": true},
			Limits:   map[string]int64{}, Usage: map[string]int64{},
		},
	}
	service := NewFeatureFlagServiceWithClock(store, zerolog.Nop(), func() time.Time { return now })
	items, err := service.ListEvaluations(context.Background(), featureFlagTestOrg)
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]models.FeatureFlagEvaluation, len(items))
	for _, item := range items {
		byKey[item.Capability.Key] = item
	}
	if byKey["core"].Enabled || byKey["core"].Reason != "tenant_disabled" || byKey["core"].Source != "tenant_override" {
		t.Fatalf("core evaluation=%+v", byKey["core"])
	}
	if byKey["dependent"].Enabled || byKey["dependent"].Reason != "prerequisite_disabled" || byKey["dependent"].BlockingCapability != "core" {
		t.Fatalf("dependent evaluation=%+v", byKey["dependent"])
	}
	if byKey["professional"].Entitled || byKey["professional"].Reason != "subscription_denied" {
		t.Fatalf("professional evaluation=%+v", byKey["professional"])
	}
	if byKey["killed"].Enabled || byKey["killed"].Reason != "global_kill_switch" {
		t.Fatalf("kill-switch evaluation=%+v", byKey["killed"])
	}
	if rollout > 0 && (byKey["gradual"].Enabled || byKey["gradual"].Reason != "outside_rollout") {
		t.Fatalf("rollout evaluation=%+v bucket=%d", byKey["gradual"], rollout)
	}
}

func TestFeatureFlagScheduledOverrideAndTierFallback(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	store := &featureFlagStoreStub{
		capabilities: []models.ProductCapability{
			{Key: "starter", MinimumTier: "starter", DefaultEnabled: true, RolloutBasisPoints: 10000},
			{Key: "enterprise", MinimumTier: "enterprise", DefaultEnabled: true, RolloutBasisPoints: 10000},
		},
		overrides: []models.TenantFeatureFlagOverride{
			{OrganizationID: featureFlagTestOrg, CapabilityKey: "starter", Enabled: false, StartsAt: &future, Version: 1, Variant: map[string]any{}},
		},
		entitlements: &models.EntitlementSnapshot{
			Source: "organization_tier", Tier: "professional", Features: map[string]bool{},
			Limits: map[string]int64{}, Usage: map[string]int64{},
		},
	}
	service := NewFeatureFlagServiceWithClock(store, zerolog.Nop(), func() time.Time { return now })
	starter, err := service.Evaluate(context.Background(), featureFlagTestOrg, " STARTER ")
	if err != nil {
		t.Fatal(err)
	}
	if !starter.Enabled || starter.Source != "global_default" || starter.Override == nil {
		t.Fatalf("scheduled override evaluation=%+v", starter)
	}
	enterprise, err := service.Evaluate(context.Background(), featureFlagTestOrg, "enterprise")
	if err != nil {
		t.Fatal(err)
	}
	if enterprise.Enabled || enterprise.Entitled || enterprise.Reason != "subscription_denied" {
		t.Fatalf("tier evaluation=%+v", enterprise)
	}
}

func TestFeatureFlagOverrideValidationAndConflictMapping(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	store := &featureFlagStoreStub{entitlements: &models.EntitlementSnapshot{Limits: map[string]int64{}, Usage: map[string]int64{}}}
	service := NewFeatureFlagServiceWithClock(store, zerolog.Nop(), func() time.Time { return now })
	badRollout, zeroVersion := 10001, int64(0)
	end, start := now, now.Add(time.Hour)
	tests := []struct {
		name      string
		org, key  string
		actor     string
		requestID string
		input     models.FeatureFlagOverrideInput
	}{
		{name: "organization", org: "bad", key: "valid", actor: featureFlagTestActor, input: models.FeatureFlagOverrideInput{Reason: "valid reason"}},
		{name: "actor", org: featureFlagTestOrg, key: "valid", actor: "bad", input: models.FeatureFlagOverrideInput{Reason: "valid reason"}},
		{name: "key", org: featureFlagTestOrg, key: "INVALID KEY", actor: featureFlagTestActor, input: models.FeatureFlagOverrideInput{Reason: "valid reason"}},
		{name: "reason", org: featureFlagTestOrg, key: "valid", actor: featureFlagTestActor, input: models.FeatureFlagOverrideInput{Reason: "x"}},
		{name: "rollout", org: featureFlagTestOrg, key: "valid", actor: featureFlagTestActor, input: models.FeatureFlagOverrideInput{Reason: "valid reason", RolloutBasisPoints: &badRollout}},
		{name: "version", org: featureFlagTestOrg, key: "valid", actor: featureFlagTestActor, input: models.FeatureFlagOverrideInput{Reason: "valid reason", ExpectedVersion: &zeroVersion}},
		{name: "window", org: featureFlagTestOrg, key: "valid", actor: featureFlagTestActor, input: models.FeatureFlagOverrideInput{Reason: "valid reason", StartsAt: &start, ExpiresAt: &end}},
		{name: "request", org: featureFlagTestOrg, key: "valid", actor: featureFlagTestActor, requestID: strings.Repeat("x", 161), input: models.FeatureFlagOverrideInput{Reason: "valid reason"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.UpsertOverride(context.Background(), test.org, test.key, test.actor, test.requestID, test.input); !errors.Is(err, ErrFeatureFlagInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}

	rollout := 5000
	result, err := service.UpsertOverride(context.Background(), featureFlagTestOrg, " Valid.Flag ", featureFlagTestActor, " request-1 ", models.FeatureFlagOverrideInput{
		Enabled: true, RolloutBasisPoints: &rollout, Reason: "  staged enablement  ", Variant: map[string]any{"experience": "new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CapabilityKey != "valid.flag" || store.upserted.Reason != "staged enablement" {
		t.Fatalf("normalized result=%+v input=%+v", result, store.upserted)
	}

	store.err = repository.ErrFeatureFlagVersionConflict
	if _, err := service.UpsertOverride(context.Background(), featureFlagTestOrg, "valid.flag", featureFlagTestActor, "", models.FeatureFlagOverrideInput{Reason: "valid reason"}); !errors.Is(err, ErrFeatureFlagConflict) {
		t.Fatalf("conflict error=%v", err)
	}
	store.err = repository.ErrCapabilityNotFound
	if err := service.ResetOverride(context.Background(), featureFlagTestOrg, "valid.flag", featureFlagTestActor, "", models.FeatureFlagResetInput{ExpectedVersion: 1, Reason: "valid reset"}); !errors.Is(err, ErrFeatureFlagNotFound) {
		t.Fatalf("not-found error=%v", err)
	}
}

func TestFeatureFlagEntitlementLimitDecision(t *testing.T) {
	store := &featureFlagStoreStub{entitlements: &models.EntitlementSnapshot{
		Limits: map[string]int64{"users": 5, "risks": 0}, Usage: map[string]int64{"users": 4, "risks": 200},
	}}
	service := NewFeatureFlagService(store, zerolog.Nop())
	decision, err := service.CheckLimit(context.Background(), featureFlagTestOrg, "users", 2)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Remaining != 1 || decision.Reason != "limit_exceeded" {
		t.Fatalf("bounded decision=%+v", decision)
	}
	unlimited, err := service.CheckLimit(context.Background(), featureFlagTestOrg, "risks", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !unlimited.Allowed || unlimited.Remaining != -1 || unlimited.Reason != "unlimited" {
		t.Fatalf("unlimited decision=%+v", unlimited)
	}
	if _, err := service.CheckLimit(context.Background(), featureFlagTestOrg, "unknown", 1); !errors.Is(err, ErrFeatureFlagInvalid) {
		t.Fatalf("unknown metric error=%v", err)
	}
}
