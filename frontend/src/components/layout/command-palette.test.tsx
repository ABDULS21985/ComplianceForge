import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { CommandPalette } from '@/components/layout/command-palette';
import type { ReactNode } from 'react';
import { useNavigationStore } from '@/store/navigation-store';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({
  autocomplete: vi.fn(),
  push: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mocks.push }),
}));

vi.mock('@/lib/api', () => ({
  default: {
    search: { autocomplete: mocks.autocomplete },
  },
}));

const context = { isSuperAdmin: true, permissions: {}, roleSlugs: [] };

function TestProviders({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

beforeEach(() => {
  mocks.autocomplete.mockResolvedValue({ data: [] });
  useNavigationStore.setState({ favoriteIds: [], recentIds: [] });
});

describe('command palette', () => {
  it('provides keyboard-searchable canonical page navigation', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <CommandPalette
        context={context}
        open
        onOpenChange={onOpenChange}
      />,
      { wrapper: TestProviders }
    );

    expect(screen.getByRole('dialog')).toBeVisible();
    expect(
      screen.getByRole('combobox', {
        name: 'Search pages and compliance records',
      })
    ).toBeVisible();

    await user.click(screen.getByText('Risk Register'));

    expect(mocks.push).toHaveBeenCalledWith('/risks');
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(useNavigationStore.getState().recentIds[0]).toBe('risks');
  });

  it('opens records returned by the stable autocomplete API', async () => {
    const user = userEvent.setup();
    mocks.autocomplete.mockResolvedValue({
      data: [
        {
          entity_type: 'risk',
          entity_id: 'risk-42',
          title: 'Cloud concentration',
          subtitle: 'RISK-0042',
        },
      ],
    });
    render(
      <CommandPalette context={context} open onOpenChange={vi.fn()} />,
      { wrapper: TestProviders }
    );

    await user.type(
      screen.getByRole('combobox', {
        name: 'Search pages and compliance records',
      }),
      'Cloud'
    );
    await waitFor(() => expect(mocks.autocomplete).toHaveBeenCalledWith('Cloud'));
    await user.click(await screen.findByText('Cloud concentration'));

    expect(mocks.push).toHaveBeenCalledWith('/risks/risk-42');
  });
});
