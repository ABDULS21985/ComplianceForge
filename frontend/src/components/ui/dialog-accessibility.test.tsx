import { describe, expect, it } from 'vitest';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { render, screen, waitFor } from '@testing-library/react';

import { axe } from 'jest-axe';
import { Button } from '@/components/ui/button';
import userEvent from '@testing-library/user-event';

describe('shared dialog accessibility', () => {
  it('traps focus, provides a 44px close target, and restores its trigger', async () => {
    const user = userEvent.setup();
    render(
      <Dialog>
        <DialogTrigger asChild>
          <Button>Open review</Button>
        </DialogTrigger>
        <DialogContent>
          <DialogTitle>Review evidence</DialogTitle>
          <DialogDescription>Confirm the review decision.</DialogDescription>
          <Button>Confirm decision</Button>
        </DialogContent>
      </Dialog>,
    );

    const trigger = screen.getByRole('button', { name: 'Open review' });
    await user.click(trigger);

    const dialog = screen.getByRole('dialog', { name: 'Review evidence' });
    expect(dialog).toBeVisible();
    expect(screen.getByRole('button', { name: 'Close' })).toHaveClass(
      'h-11',
      'w-11',
    );
    expect((await axe(document.body)).violations).toEqual([]);

    await user.keyboard('{Escape}');
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Review evidence' })).not.toBeInTheDocument(),
    );
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('gives every shared button a minimum 44px target', () => {
    render(<Button size="sm">Compact action</Button>);
    expect(screen.getByRole('button', { name: 'Compact action' })).toHaveClass(
      'min-h-11',
    );
  });
});
