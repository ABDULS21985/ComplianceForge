import type {
  EntitlementSnapshot,
  FeatureFlagEvaluation,
} from '@/types/feature-flag';
import type { ApiError } from '@/lib/api';

export const FEATURE_FLAG_ROUTES = {
  capabilities: '/settings/capabilities',
  evaluation: (key: string) => `/settings/capabilities/${encodeURIComponent(key)}/evaluation`,
  entitlements: '/settings/entitlements',
  limit: (metric: string) => `/settings/entitlements/limits/${encodeURIComponent(metric)}/check`,
  flags: '/settings/feature-flags',
  flag: (key: string) => `/settings/feature-flags/${encodeURIComponent(key)}`,
  reset: (key: string) => `/settings/feature-flags/${encodeURIComponent(key)}/reset`,
  history: (key: string) => `/settings/feature-flags/${encodeURIComponent(key)}/history`,
} as const;

export const featureFlagKeys = {
  all: ['feature-flags'] as const,
  capabilities: ['feature-flags', 'capabilities'] as const,
  evaluation: (key: string) => ['feature-flags', 'evaluation', key] as const,
  entitlements: ['feature-flags', 'entitlements'] as const,
  history: (key: string, page: number) => ['feature-flags', 'history', key, page] as const,
  limit: (metric: string, requested: number) => ['feature-flags', 'limit', metric, requested] as const,
};

export function humanizeFeatureToken(value: string): string {
  return value.replace(/[_.-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function rolloutPercent(basisPoints: number): number {
  return Math.max(0, Math.min(10_000, basisPoints)) / 100;
}

export function featureEvaluationExplanation(evaluation: FeatureFlagEvaluation): string {
  const blocking = evaluation.blocking_capability
    ? ` Blocking capability: ${humanizeFeatureToken(evaluation.blocking_capability)}.`
    : '';
  switch (evaluation.evaluation_reason) {
    case 'enabled': return 'Available for this organization and inside the deterministic rollout.';
    case 'global_kill_switch': return 'Disabled by the platform kill switch. Tenant overrides cannot bypass this control.';
    case 'subscription_denied': return `Not included in the current subscription. Minimum tier: ${humanizeFeatureToken(evaluation.capability.minimum_tier)}.`;
    case 'tenant_disabled': return 'Disabled by the currently active tenant override.';
    case 'global_disabled': return 'Disabled by the deployment-managed catalogue default.';
    case 'prerequisite_disabled': return `A required capability is unavailable.${blocking}`;
    case 'prerequisite_cycle': return `A catalogue prerequisite cycle failed closed.${blocking}`;
    case 'capability_missing': return `A required catalogue definition is missing.${blocking}`;
    case 'outside_rollout': return `This organization is outside the ${rolloutPercent(evaluation.effective_rollout_basis_points)}% deterministic rollout.`;
    default: return humanizeFeatureToken(evaluation.evaluation_reason || 'Evaluation unavailable');
  }
}

export function overrideWindowState(evaluation: FeatureFlagEvaluation, now = new Date()): 'active' | 'scheduled' | 'expired' | 'none' {
  const override = evaluation.override;
  if (!override) return 'none';
  if (override.starts_at && new Date(override.starts_at) > now) return 'scheduled';
  if (override.expires_at && new Date(override.expires_at) <= now) return 'expired';
  return 'active';
}

export function formatFeatureFlagError(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (!error || typeof error !== 'object') return fallback;
  const apiError = error as ApiError;
  const detail = apiError.detail && typeof apiError.detail === 'object'
    ? apiError.detail as Record<string, unknown>
    : {};
  const message = typeof detail.details === 'string' && detail.details
    ? detail.details
    : typeof detail.message === 'string' && detail.message
      ? detail.message
      : apiError.message || fallback;
  const requestId = typeof detail.request_id === 'string' && detail.request_id
    ? ` Reference: ${detail.request_id}.`
    : '';
  return `${message}${requestId}`;
}

export function isFeatureFlagConflict(error: unknown): boolean {
  return Boolean(error && typeof error === 'object' && (error as ApiError).status === 409);
}

export function entitlementMetrics(snapshot: EntitlementSnapshot): Array<{
  limit: number;
  metric: string;
  usage: number;
}> {
  return Object.entries(snapshot.limits)
    .map(([metric, limit]) => ({ metric, limit, usage: snapshot.usage[metric] ?? 0 }))
    .sort((left, right) => left.metric.localeCompare(right.metric));
}

export function formatEntitlementAmount(metric: string, value: number): string {
  if (metric === 'storage_bytes') {
    const gibibytes = value / (1024 ** 3);
    return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(gibibytes)} GiB`;
  }
  return new Intl.NumberFormat().format(value);
}
