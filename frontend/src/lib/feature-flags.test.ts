import { describe, expect, it } from 'vitest';
import {
  entitlementMetrics,
  featureEvaluationExplanation,
  formatEntitlementAmount,
  overrideWindowState,
  rolloutPercent,
} from '@/lib/feature-flags';
import type { EntitlementSnapshot, FeatureFlagEvaluation } from '@/types/feature-flag';

function evaluation(reason: string, overrides: Partial<FeatureFlagEvaluation> = {}): FeatureFlagEvaluation {
  return {
    capability: {
      id: 'capability-1', key: 'advanced_reporting', display_name: 'Advanced reporting', description: 'Advanced reports.', owner_team: 'Reporting', maturity: 'beta', minimum_tier: 'professional', prerequisites: [], default_enabled: true, kill_switch: false, rollout_basis_points: 10_000, is_active: true, version: 1, created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z',
    },
    enabled: reason === 'enabled', entitled: reason !== 'subscription_denied', in_rollout: reason === 'enabled', effective_rollout_basis_points: 10_000, source: 'global_default', evaluation_reason: reason, variant: {}, evaluated_at: '2026-09-14T00:00:00Z', ...overrides,
  };
}

describe('feature flag helpers', () => {
  it('explains subscription, kill-switch, prerequisite, and rollout decisions distinctly', () => {
    expect(featureEvaluationExplanation(evaluation('subscription_denied'))).toMatch(/current subscription/i);
    expect(featureEvaluationExplanation(evaluation('global_kill_switch'))).toMatch(/cannot bypass/i);
    expect(featureEvaluationExplanation(evaluation('prerequisite_disabled', { blocking_capability: 'basic_reporting' }))).toMatch(/Basic Reporting/);
    expect(featureEvaluationExplanation(evaluation('outside_rollout', { effective_rollout_basis_points: 2550 }))).toMatch(/25.5%/);
  });

  it('reports scheduled, active, and expired override windows', () => {
    const now = new Date('2026-09-14T12:00:00Z');
    expect(overrideWindowState(evaluation('tenant_disabled', { override: { starts_at: '2026-09-15T00:00:00Z' } as never }), now)).toBe('scheduled');
    expect(overrideWindowState(evaluation('tenant_disabled', { override: { expires_at: '2026-09-14T11:00:00Z' } as never }), now)).toBe('expired');
    expect(overrideWindowState(evaluation('tenant_disabled', { override: {} as never }), now)).toBe('active');
  });

  it('normalizes rollout and entitlement usage presentation', () => {
    expect(rolloutPercent(2550)).toBe(25.5);
    const snapshot: EntitlementSnapshot = {
      organization_id: 'org-1', source: 'subscription', subscription_status: 'active', tier: 'starter', features: {}, limits: { users: 5, storage_bytes: 5 * 1024 ** 3 }, usage: { users: 3 }, evaluated_at: '2026-09-14T00:00:00Z',
    };
    expect(entitlementMetrics(snapshot)).toEqual([
      { metric: 'storage_bytes', limit: 5 * 1024 ** 3, usage: 0 },
      { metric: 'users', limit: 5, usage: 3 },
    ]);
    expect(formatEntitlementAmount('storage_bytes', 1.5 * 1024 ** 3)).toContain('1.5');
  });
});
