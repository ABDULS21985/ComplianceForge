import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { Incident } from '@/types/incident';
import { IncidentDeleteDialog } from '@/components/incidents/incident-delete-dialog';
import userEvent from '@testing-library/user-event';

const mutateAsync = vi.fn();
vi.mock('@/lib/api-hooks', () => ({
  useDeleteIncident: () => ({ mutateAsync, isPending: false, error: null }),
}));

const INCIDENT = {
  id: 'incident-1', incident_ref: 'INC-2026-0001', version: 7, status: 'closed', legal_hold: false,
} as Incident;

describe('IncidentDeleteDialog', () => {
  beforeEach(() => mutateAsync.mockReset().mockResolvedValue(undefined));

  it('has an accessible description and requires the exact incident reference', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    const onDeleted = vi.fn();
    render(<IncidentDeleteDialog incident={INCIDENT} open onOpenChange={onOpenChange} onDeleted={onDeleted} />);
    expect(screen.getByRole('alertdialog', { name: 'Delete incident record?' })).toHaveAccessibleDescription(/soft-deletes the terminal incident/i);
    const button = screen.getByRole('button', { name: 'Delete incident' });
    expect(button).toBeDisabled();
    await user.type(screen.getByLabelText(/Type INC-2026-0001 to confirm/), 'INC-2026-0001');
    await user.click(button);
    expect(mutateAsync).toHaveBeenCalledWith({ id: 'incident-1', version: 7 });
    expect(onDeleted).toHaveBeenCalledOnce();
  });
});
