import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GovernanceReasonDialog } from './governance-reason-dialog';
import userEvent from '@testing-library/user-event';

describe('GovernanceReasonDialog', () => {
  it('requires a meaningful audit reason before invoking the action', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(
      <GovernanceReasonDialog
        actionLabel="Retire schedule"
        description="Retirement is permanent."
        onOpenChange={vi.fn()}
        onSubmit={onSubmit}
        open
        title="Retire schedule?"
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Retire schedule' }));
    expect(screen.getByRole('alert')).toHaveTextContent('at least 3 characters');
    expect(onSubmit).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText('Reason *'), 'Superseded by the 2027 schedule');
    await user.click(screen.getByRole('button', { name: 'Retire schedule' }));
    expect(onSubmit).toHaveBeenCalledWith('Superseded by the 2027 schedule');
  });
});
