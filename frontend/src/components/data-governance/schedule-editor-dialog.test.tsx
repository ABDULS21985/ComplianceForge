import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import type { RetentionSchedule } from '@/types/data-governance';
import { ScheduleEditorDialog } from './schedule-editor-dialog';
import userEvent from '@testing-library/user-event';

const { updateSchedule } = vi.hoisted(() => ({ updateSchedule: vi.fn() }));
vi.mock('@/lib/api', () => ({
  default: { dataGovernance: { createSchedule: vi.fn(), updateSchedule } },
}));

const schedule: RetentionSchedule = {
  id: '0b5069ba-c28f-42d7-b41d-6c4efc9492b6',
  organization_id: '8f286a77-ec78-4621-ab44-d269cb8296b4',
  name: 'Incident evidence schedule',
  description: 'Preserve material incident evidence.',
  record_type: 'incident',
  data_classification: 'confidential',
  jurisdiction: 'EU',
  trigger_event: 'record_closed',
  legal_basis: 'Regulatory incident recordkeeping requirement',
  retention_days: 2555,
  archive_after_days: 365,
  disposition_action: 'review',
  review_required: true,
  priority: 50,
  status: 'active',
  effective_from: '2026-01-01T00:00:00Z',
  version: 7,
  created_by: '12abde07-2548-4191-be9f-92b9a32c67b1',
  updated_by: '12abde07-2548-4191-be9f-92b9a32c67b1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
};

describe('ScheduleEditorDialog', () => {
  beforeEach(() => updateSchedule.mockReset().mockResolvedValue({ ...schedule, version: 8 }));

  it('locks record scope and sends the current version plus audit reason', async () => {
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <ScheduleEditorDialog
          onOpenChange={vi.fn()}
          onReload={vi.fn()}
          onSaved={vi.fn()}
          open
          schedule={schedule}
        />
      </QueryClientProvider>,
    );

    expect(screen.getByLabelText('Record type')).toBeDisabled();
    await user.type(screen.getByLabelText('Reason for change'), 'Annual legal review completed');
    await user.click(screen.getByRole('button', { name: 'Save schedule' }));

    await waitFor(() =>
      expect(updateSchedule).toHaveBeenCalledWith(
        schedule.id,
        expect.objectContaining({
          expected_version: 7,
          reason: 'Annual legal review completed',
        }),
      ),
    );
    expect(updateSchedule.mock.calls[0][1]).not.toHaveProperty('record_type');
  });
});
