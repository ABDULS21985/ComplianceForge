import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

import { StepUpDialog } from '@/components/identity/step-up-dialog';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({
  beginStepUp: vi.fn(),
  verifyStepUp: vi.fn(),
}));

vi.mock('@/lib/api', () => ({
  default: {
    identity: {
      beginStepUp: mocks.beginStepUp,
      verifyStepUp: mocks.verifyStepUp,
    },
  },
}));

describe('step-up authentication dialog', () => {
  beforeEach(() => {
    mocks.beginStepUp.mockReset().mockResolvedValue({
      challenge_token: 'challenge-secret',
      methods: ['totp', 'recovery_code'],
      expires_at: '2030-01-01T00:00:00Z',
    });
    mocks.verifyStepUp.mockReset().mockResolvedValue({
      grant_token: 'one-time-grant',
    });
  });

  it('requires a proof and passes the one-time grant directly to the protected action', async () => {
    const onGranted = vi.fn().mockResolvedValue(undefined);
    const onOpenChange = vi.fn();
    const storage = vi.spyOn(Storage.prototype, 'setItem');
    const user = userEvent.setup();
    render(
      <StepUpDialog
        open
        purpose="mfa_change"
        title="Verify change"
        description="Confirm this factor change."
        onGranted={onGranted}
        onOpenChange={onOpenChange}
      />,
    );

    expect(await screen.findByRole('combobox', { name: 'Verification method' })).toHaveValue(
      'totp',
    );
    await user.type(screen.getByLabelText('Six-digit authenticator code'), '123456');
    await user.click(screen.getByRole('button', { name: 'Verify and continue' }));

    await waitFor(() =>
      expect(mocks.verifyStepUp).toHaveBeenCalledWith({
        challenge_token: 'challenge-secret',
        method: 'totp',
        code: '123456',
        credential: undefined,
      }),
    );
    expect(onGranted).toHaveBeenCalledWith('one-time-grant');
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(storage).not.toHaveBeenCalled();
  });

  it('announces a challenge initialization failure and remains dismissible', async () => {
    mocks.beginStepUp.mockRejectedValue({ status: 503, message: 'Identity unavailable' });
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    render(
      <StepUpDialog
        open
        purpose="mfa_change"
        title="Verify change"
        description="Confirm this factor change."
        onGranted={vi.fn()}
        onOpenChange={onOpenChange}
      />,
    );

    expect(await screen.findByRole('alert')).toHaveTextContent('Identity unavailable');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
