import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { FeatureFlagEditorDialog } from '@/components/feature-flags/feature-flag-editor-dialog';
import type { FeatureFlagEvaluation } from '@/types/feature-flag';
import userEvent from '@testing-library/user-event';

const { upsertOverride } = vi.hoisted(() => ({ upsertOverride: vi.fn() }));
vi.mock('@/lib/api', () => ({ default: { featureFlags: { upsertOverride } } }));

const evaluation: FeatureFlagEvaluation = {
  capability: { id: 'capability-1', key: 'advanced_reporting', display_name: 'Advanced reporting', description: 'Advanced reporting tools.', owner_team: 'Reporting', maturity: 'beta', minimum_tier: 'professional', prerequisites: ['basic_reporting'], default_enabled: true, kill_switch: false, rollout_basis_points: 10_000, is_active: true, version: 1, created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z' },
  enabled: true, entitled: true, in_rollout: true, effective_rollout_basis_points: 10_000, source: 'global_default', evaluation_reason: 'enabled', variant: {}, evaluated_at: '2026-09-14T00:00:00Z',
};

function renderEditor(value = evaluation) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><FeatureFlagEditorDialog evaluation={value} open onOpenChange={vi.fn()} onReload={vi.fn()} onSaved={vi.fn()} /></QueryClientProvider>);
}

describe('FeatureFlagEditorDialog', () => {
  beforeEach(() => upsertOverride.mockReset().mockResolvedValue({ version: 1 }));

  it('validates JSON variants and converts rollout percent to basis points', async () => {
    const user = userEvent.setup();
    renderEditor();
    await user.click(screen.getByRole('checkbox', { name: /Override rollout percentage/i }));
    const rollout = screen.getByLabelText('Rollout percentage value');
    await user.clear(rollout);
    await user.type(rollout, '25.5');
    const variant = screen.getByLabelText(/Variant/);
    await user.clear(variant);
    fireEvent.change(variant, { target: { value: '{broken' } });
    await user.type(screen.getByLabelText(/Business reason/), 'Pilot for reporting team');
    await user.click(screen.getByRole('button', { name: 'Save audited override' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('valid JSON');
    expect(upsertOverride).not.toHaveBeenCalled();
    fireEvent.change(variant, { target: { value: '{"layout":"dense"}' } });
    await user.click(screen.getByRole('button', { name: 'Save audited override' }));
    expect(upsertOverride).toHaveBeenCalledWith('advanced_reporting', expect.objectContaining({ rollout_basis_points: 2550, reason: 'Pilot for reporting team', variant: { layout: 'dense' } }));
    expect(upsertOverride.mock.calls[0][1]).not.toHaveProperty('expected_version');
  });

  it('includes the current optimistic version when updating', async () => {
    const user = userEvent.setup();
    renderEditor({ ...evaluation, override: { organization_id: 'org-1', capability_key: 'advanced_reporting', enabled: true, variant: {}, reason: 'Initial', version: 4, created_by: 'user-1', updated_by: 'user-1', created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z' } });
    await user.type(screen.getByLabelText(/Business reason/), 'Extend approved pilot');
    await user.click(screen.getByRole('button', { name: 'Save audited override' }));
    expect(upsertOverride).toHaveBeenCalledWith('advanced_reporting', expect.objectContaining({ expected_version: 4 }));
  });
});
