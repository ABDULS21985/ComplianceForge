import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';

import { APIKeysPanel } from '@/components/integrations/api-keys-panel';

const integrationApi = vi.hoisted(() => ({
  createAPIKey: vi.fn(),
  listAPIKeys: vi.fn(),
  revokeAPIKey: vi.fn(),
}));

vi.mock('@/lib/api', () => ({ default: { integrations: integrationApi } }));
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  integrationApi.listAPIKeys.mockResolvedValue({ data: [] });
  integrationApi.createAPIKey.mockResolvedValue({
    data: {
      id: 'key-1', organization_id: 'org-1', name: 'CI collector', key_prefix: 'cf_live_abcd',
      permissions: ['read:controls'], rate_limit_per_minute: 60, expires_at: null,
      last_used_at: null, is_active: true, created_at: '2026-09-14T00:00:00Z',
    },
    key: 'cf_live_abcd_complete_secret',
    note: 'Store this key securely. It will not be shown again.',
  });
});

describe('API key lifecycle UI', () => {
  it('keeps a newly issued key in one-time component state and never browser storage', async () => {
    const storageSpy = vi.spyOn(window.localStorage, 'setItem');
    const cookieBefore = document.cookie;
    const user = userEvent.setup();
    render(<APIKeysPanel canConfigure />, { wrapper: Providers });

    await screen.findByText('No API keys');
    await user.click(screen.getByRole('button', { name: 'Create key' }));
    await user.type(screen.getByLabelText('Key name'), 'CI collector');
    await user.click(screen.getByRole('button', { name: 'Add' }));
    await user.click(screen.getByRole('button', { name: 'Create key' }));

    expect(await screen.findByRole('heading', { name: 'Store your API key now' })).toBeVisible();
    expect(screen.getByLabelText('API key for CI collector')).toHaveAttribute('type', 'password');
    expect(screen.getByLabelText('API key for CI collector')).toHaveValue('cf_live_abcd_complete_secret');
    expect(storageSpy).not.toHaveBeenCalled();
    expect(document.cookie).toBe(cookieBefore);

    await user.click(screen.getByRole('checkbox'));
    await user.click(screen.getByRole('button', { name: 'Finish and hide key' }));
    await waitFor(() => expect(screen.queryByText('cf_live_abcd_complete_secret')).not.toBeInTheDocument());
  });
});
