import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import { AuditConfirmationDialog } from '@/components/audits/audit-confirmation-dialog';
import userEvent from '@testing-library/user-event';

describe('AuditConfirmationDialog', () => {
  it('exposes an accessible confirmation and requires the exact record reference', async () => {
    const user = userEvent.setup();
    const confirm = vi.fn().mockResolvedValue(undefined);
    const onOpenChange = vi.fn();
    render(
      <AuditConfirmationDialog
        open
        onOpenChange={onOpenChange}
        title="Delete audit?"
        description={<p>This action is destructive.</p>}
        confirmLabel="Delete audit"
        confirmationText="AUD-1042"
        destructive
        onConfirm={confirm}
      />,
    );

    expect(screen.getByRole('alertdialog', { name: 'Delete audit?' })).toHaveAccessibleDescription(
      'This action is destructive.',
    );
    const deleteButton = screen.getByRole('button', { name: 'Delete audit' });
    expect(deleteButton).toBeDisabled();

    await user.type(screen.getByLabelText(/Type AUD-1042 to confirm/), 'AUD-104');
    expect(deleteButton).toBeDisabled();
    await user.type(screen.getByLabelText(/Type AUD-1042 to confirm/), '2');
    await user.click(deleteButton);

    expect(confirm).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it('keeps the dialog open and announces a failed action', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <AuditConfirmationDialog
        open
        onOpenChange={onOpenChange}
        title="Close audit?"
        description={<p>All active findings must be resolved.</p>}
        confirmLabel="Close audit"
        onConfirm={() => Promise.reject(new Error('Two findings still block closure.'))}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Close audit' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Two findings still block closure.');
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
