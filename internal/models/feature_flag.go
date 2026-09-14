package models

import (
	"encoding/json"
	"time"
)

type CapabilityMaturity string

const (
	CapabilityExperimental        CapabilityMaturity = "experimental"
	CapabilityBeta                CapabilityMaturity = "beta"
	CapabilityGeneralAvailability CapabilityMaturity = "general_availability"
	CapabilityDeprecated          CapabilityMaturity = "deprecated"
)

// ProductCapability is a deployment-managed entry in the versioned product
// catalogue. Tenant administrators may inspect definitions but can only change
// their own override, never the global kill switch or plan requirements.
type ProductCapability struct {
	ID                  string             `json:"id"`
	Key                 string             `json:"key"`
	DisplayName         string             `json:"display_name"`
	Description         string             `json:"description"`
	OwnerTeam           string             `json:"owner_team"`
	Maturity            CapabilityMaturity `json:"maturity"`
	MinimumTier         string             `json:"minimum_tier"`
	RequiredPlanFeature string             `json:"required_plan_feature,omitempty"`
	Prerequisites       []string           `json:"prerequisites"`
	DefaultEnabled      bool               `json:"default_enabled"`
	KillSwitch          bool               `json:"kill_switch"`
	RolloutBasisPoints  int                `json:"rollout_basis_points"`
	IsActive            bool               `json:"is_active"`
	Version             int64              `json:"version"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

type TenantFeatureFlagOverride struct {
	OrganizationID     string         `json:"organization_id"`
	CapabilityKey      string         `json:"capability_key"`
	Enabled            bool           `json:"enabled"`
	RolloutBasisPoints *int           `json:"rollout_basis_points,omitempty"`
	Variant            map[string]any `json:"variant"`
	Reason             string         `json:"reason"`
	StartsAt           *time.Time     `json:"starts_at,omitempty"`
	ExpiresAt          *time.Time     `json:"expires_at,omitempty"`
	Version            int64          `json:"version"`
	CreatedBy          string         `json:"created_by"`
	UpdatedBy          string         `json:"updated_by"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type FeatureFlagOverrideInput struct {
	Enabled            bool           `json:"enabled"`
	RolloutBasisPoints *int           `json:"rollout_basis_points,omitempty"`
	Variant            map[string]any `json:"variant,omitempty"`
	Reason             string         `json:"reason"`
	StartsAt           *time.Time     `json:"starts_at,omitempty"`
	ExpiresAt          *time.Time     `json:"expires_at,omitempty"`
	ExpectedVersion    *int64         `json:"expected_version,omitempty"`
}

type FeatureFlagResetInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type FeatureFlagEvaluation struct {
	Capability         ProductCapability          `json:"capability"`
	Override           *TenantFeatureFlagOverride `json:"override,omitempty"`
	Enabled            bool                       `json:"enabled"`
	Entitled           bool                       `json:"entitled"`
	InRollout          bool                       `json:"in_rollout"`
	EffectiveRollout   int                        `json:"effective_rollout_basis_points"`
	Source             string                     `json:"source"`
	Reason             string                     `json:"evaluation_reason"`
	BlockingCapability string                     `json:"blocking_capability,omitempty"`
	Variant            map[string]any             `json:"variant"`
	EvaluatedAt        time.Time                  `json:"evaluated_at"`
}

type EntitlementSnapshot struct {
	OrganizationID     string           `json:"organization_id"`
	Source             string           `json:"source"`
	SubscriptionStatus string           `json:"subscription_status"`
	PlanID             string           `json:"plan_id,omitempty"`
	PlanName           string           `json:"plan_name,omitempty"`
	Tier               string           `json:"tier"`
	Features           map[string]bool  `json:"features"`
	Limits             map[string]int64 `json:"limits"`
	Usage              map[string]int64 `json:"usage"`
	EvaluatedAt        time.Time        `json:"evaluated_at"`
}

type EntitlementLimitDecision struct {
	Metric    string `json:"metric"`
	Allowed   bool   `json:"allowed"`
	Limit     int64  `json:"limit"`
	Usage     int64  `json:"usage"`
	Requested int64  `json:"requested"`
	Remaining int64  `json:"remaining"`
	Reason    string `json:"reason"`
}

type FeatureFlagChangeEvent struct {
	ID              string          `json:"id"`
	CapabilityKey   string          `json:"capability_key"`
	EventType       string          `json:"event_type"`
	ActorUserID     string          `json:"actor_user_id"`
	OverrideVersion int64           `json:"override_version"`
	Reason          string          `json:"reason"`
	BeforeState     json.RawMessage `json:"before_state,omitempty"`
	AfterState      json.RawMessage `json:"after_state,omitempty"`
	RequestID       string          `json:"request_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}
