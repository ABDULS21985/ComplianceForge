export type CapabilityMaturity =
  | 'experimental'
  | 'beta'
  | 'general_availability'
  | 'deprecated';

export interface ProductCapability {
  id: string;
  key: string;
  display_name: string;
  description: string;
  owner_team: string;
  maturity: CapabilityMaturity;
  minimum_tier: string;
  required_plan_feature?: string;
  prerequisites: string[];
  default_enabled: boolean;
  kill_switch: boolean;
  rollout_basis_points: number;
  is_active: boolean;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface TenantFeatureFlagOverride {
  organization_id: string;
  capability_key: string;
  enabled: boolean;
  rollout_basis_points?: number;
  variant: Record<string, unknown>;
  reason: string;
  starts_at?: string;
  expires_at?: string;
  version: number;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

export interface FeatureFlagOverrideInput {
  enabled: boolean;
  rollout_basis_points?: number;
  variant?: Record<string, unknown>;
  reason: string;
  starts_at?: string;
  expires_at?: string;
  expected_version?: number;
}

export interface FeatureFlagResetInput {
  expected_version: number;
  reason: string;
}

export interface FeatureFlagEvaluation {
  capability: ProductCapability;
  override?: TenantFeatureFlagOverride;
  enabled: boolean;
  entitled: boolean;
  in_rollout: boolean;
  effective_rollout_basis_points: number;
  source: 'catalogue' | 'global_default' | 'tenant_override' | string;
  evaluation_reason: string;
  blocking_capability?: string;
  variant: Record<string, unknown>;
  evaluated_at: string;
}

export interface EntitlementSnapshot {
  organization_id: string;
  source: string;
  subscription_status: string;
  plan_id?: string;
  plan_name?: string;
  tier: string;
  features: Record<string, boolean>;
  limits: Record<string, number>;
  usage: Record<string, number>;
  evaluated_at: string;
}

export interface EntitlementLimitDecision {
  metric: string;
  allowed: boolean;
  limit: number;
  usage: number;
  requested: number;
  remaining: number;
  reason: string;
}

export interface FeatureFlagChangeEvent {
  id: string;
  capability_key: string;
  event_type: 'created' | 'updated' | 'reset' | string;
  actor_user_id: string;
  override_version: number;
  reason: string;
  before_state?: unknown;
  after_state?: unknown;
  request_id?: string;
  created_at: string;
}
