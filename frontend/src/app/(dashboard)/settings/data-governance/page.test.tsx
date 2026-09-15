import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import DataGovernancePage from './page';
import type { FeatureFlagEvaluation } from '@/types/feature-flag';

const { evaluate, permissionState } = vi.hoisted(() => ({
  evaluate: vi.fn(),
  permissionState: {
    canConfigure: true,
    canRead: true,
    isError: false,
    isLoading: false,
    retry: vi.fn(),
  },
}));
vi.mock('@/lib/api', () => ({ default: { featureFlags: { evaluate } } }));
vi.mock('@/hooks/use-capability-permissions', () => ({
  useCapabilityPermissions: () => permissionState,
}));
vi.mock('@/components/data-governance/policy-panel', () => ({
  PolicyPanel: () => <div>Policy panel</div>,
}));
vi.mock('@/components/data-governance/schedules-panel', () => ({
  SchedulesPanel: () => <div>Schedules panel</div>,
}));
vi.mock('@/components/data-governance/records-panel', () => ({
  RecordsPanel: () => <div>Records panel</div>,
}));
vi.mock('@/components/data-governance/legal-holds-panel', () => ({
  LegalHoldsPanel: () => <div>Holds panel</div>,
}));
vi.mock('@/components/data-governance/history-panel', () => ({
  GovernanceHistoryPanel: () => <div>History panel</div>,
}));

function evaluation(
  enabled: boolean,
  reason = enabled ? 'enabled' : 'tenant_disabled',
): FeatureFlagEvaluation {
  return {
    capability: {
      id: 'cap-1',
      key: 'data_lifecycle',
      display_name: 'Data lifecycle',
      description: 'Retention and legal holds.',
      owner_team: 'Governance',
      maturity: 'general_availability',
      minimum_tier: 'enterprise',
      prerequisites: [],
      default_enabled: true,
      kill_switch: false,
      rollout_basis_points: 10_000,
      is_active: true,
      version: 1,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    enabled,
    entitled: reason !== 'subscription_denied',
    in_rollout: true,
    effective_rollout_basis_points: 10_000,
    source: 'tenant_override',
    evaluation_reason: reason,
    variant: {},
    evaluated_at: '2026-09-14T00:00:00Z',
  };
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <DataGovernancePage />
    </QueryClientProvider>,
  );
}

describe('DataGovernancePage access states', () => {
  beforeEach(() => evaluate.mockReset());

  it('fails closed with a distinct subscription upgrade path', async () => {
    evaluate.mockResolvedValue(evaluation(false, 'subscription_denied'));
    renderPage();
    expect(
      await screen.findByRole('heading', { name: 'Data lifecycle upgrade required' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review plan and usage' })).toHaveAttribute(
      'href',
      '/settings/subscription',
    );
    expect(screen.queryByRole('tab')).not.toBeInTheDocument();
  });

  it('renders five keyboard-accessible workflow tabs only when enabled', async () => {
    evaluate.mockResolvedValue(evaluation(true));
    renderPage();
    expect(
      await screen.findByRole('heading', { name: 'Data lifecycle governance' }),
    ).toBeInTheDocument();
    expect(screen.getAllByRole('tab')).toHaveLength(5);
    expect(screen.getByRole('tab', { name: /Policy & residency/ })).toHaveAttribute(
      'data-state',
      'active',
    );
  });
});
