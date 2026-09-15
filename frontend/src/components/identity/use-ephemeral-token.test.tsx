import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

import { useEphemeralQueryToken } from '@/components/identity/use-ephemeral-token';

vi.mock('next/navigation', () => ({
  useSearchParams: () => new URLSearchParams(window.location.search),
}));

function Harness() {
  const token = useEphemeralQueryToken();
  return <p data-testid="captured">{token}</p>;
}

describe('one-time query credential handling', () => {
  afterEach(() => {
    window.history.replaceState({}, '', '/');
  });

  it('captures the token only in component memory and immediately cleans browser history', async () => {
    const storage = vi.spyOn(Storage.prototype, 'setItem');
    const replaceState = vi.spyOn(window.history, 'replaceState');
    window.history.replaceState(
      { workflow: 'invite' },
      '',
      '/accept-invitation?token=one-time-secret&locale=en#form',
    );
    replaceState.mockClear();

    render(<Harness />);

    expect(screen.getByTestId('captured')).toHaveTextContent('one-time-secret');
    await waitFor(() =>
      expect(replaceState).toHaveBeenCalledWith(
        expect.anything(),
        '',
        '/accept-invitation?locale=en#form',
      ),
    );
    expect(window.location.href).not.toContain('one-time-secret');
    expect(storage).not.toHaveBeenCalled();
  });
});
