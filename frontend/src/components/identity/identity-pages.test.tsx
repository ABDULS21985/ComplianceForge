import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import IdentityAdministrationPage from '@/app/(dashboard)/settings/identity/page';
import type { ReactNode } from 'react';
import { RecoveryCodesDialog } from '@/components/identity/recovery-codes-dialog';
import SecuritySettingsPage from '@/app/(dashboard)/settings/security/page';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({
  permission: {
    canAdminReset: false,
    canConfigurePolicy: false,
    canInvite: false,
    canReadAdministration: false,
    canSelfService: false,
    isError: false,
    isLoading: false,
    retry: vi.fn(),
  },
}));
const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');

vi.mock('@/hooks/use-identity-permissions', () => ({
  useIdentityPermissions: () => mocks.permission,
}));
vi.mock('@/components/identity/sessions-panel', () => ({
  SessionsPanel: () => <div>Session inventory loaded</div>,
}));
vi.mock('@/components/identity/mfa-panel', () => ({
  MFAPanel: () => <div>MFA inventory loaded</div>,
}));
vi.mock('@/components/identity/passkeys-panel', () => ({
  PasskeysPanel: () => <div>Passkey inventory loaded</div>,
}));
vi.mock('@/components/access-admin/directory-user-picker', () => ({
  DirectoryUserPicker: ({ searchLabel }: { searchLabel: string }) => <div>{searchLabel}</div>,
}));

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('identity page permission boundaries', () => {
  beforeEach(() => {
    Object.assign(mocks.permission, {
      canAdminReset: false,
      canConfigurePolicy: false,
      canInvite: false,
      canReadAdministration: false,
      canSelfService: false,
      isError: false,
      isLoading: false,
    });
  });

  afterEach(() => {
    if (originalClipboard) Object.defineProperty(navigator, 'clipboard', originalClipboard);
    else Reflect.deleteProperty(navigator, 'clipboard');
  });

  it('fails closed before mounting any self-service identity panels', () => {
    render(<SecuritySettingsPage />);

    expect(screen.getByRole('heading', { name: 'Security settings unavailable' })).toBeVisible();
    expect(screen.queryByText('Session inventory loaded')).not.toBeInTheDocument();
    expect(screen.queryByText('MFA inventory loaded')).not.toBeInTheDocument();
    expect(screen.queryByText('Passkey inventory loaded')).not.toBeInTheDocument();
  });

  it('fails closed before loading tenant policy, recovery, or history', () => {
    render(<IdentityAdministrationPage />);

    expect(
      screen.getByRole('heading', { name: 'Identity administration unavailable' }),
    ).toBeVisible();
    expect(screen.queryByText('Tenant authentication policy')).not.toBeInTheDocument();
    expect(screen.queryByText('Search pending users')).not.toBeInTheDocument();
  });

  it('shows self-service security only after the canonical users:read grant', () => {
    Object.assign(mocks.permission, { canSelfService: true });
    render(<SecuritySettingsPage />, { wrapper: Providers });

    expect(screen.getByRole('heading', { name: 'Security settings' })).toBeVisible();
    expect(screen.getByRole('tab', { name: 'Sessions' })).toBeVisible();
    expect(screen.getByText('Session inventory loaded')).toBeVisible();
  });

  it('gives an invitation-only administrator only the authorized recovery surface', () => {
    Object.assign(mocks.permission, { canInvite: true });
    render(<IdentityAdministrationPage />, { wrapper: Providers });

    expect(screen.getByRole('tab', { name: 'Invitations & recovery' })).toBeVisible();
    expect(screen.queryByRole('tab', { name: 'Policy' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'History' })).not.toBeInTheDocument();
    expect(screen.getByText('Search pending users')).toBeVisible();
    expect(screen.queryByText('Search user for MFA recovery')).not.toBeInTheDocument();
  });

  it('keeps one-time recovery codes visible until the user confirms secure storage', async () => {
    const onClose = vi.fn();
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    render(<RecoveryCodesDialog codes={['alpha-code', 'beta-code']} onClose={onClose} />);

    expect(screen.getByText('alpha-code')).toBeVisible();
    const finish = screen.getByRole('button', { name: 'Clear codes and finish' });
    expect(finish).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Copy codes' }));
    expect(writeText).toHaveBeenCalledWith('alpha-code\nbeta-code');
    await user.click(
      screen.getByRole('checkbox', {
        name: 'I saved these codes in an approved secure location.',
      }),
    );
    await user.click(finish);
    expect(onClose).toHaveBeenCalledOnce();
  });
});
