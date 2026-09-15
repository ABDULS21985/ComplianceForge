import { act, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { DirectoryUser } from '@/types/directory';
import type { ManagedRole } from '@/types/access-admin';
import type { ReactNode } from 'react';
import { RoleAssignDialog } from '@/components/access-admin/role-assignment-dialogs';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({
  assignRole: vi.fn(),
  listUsers: vi.fn(),
}));

vi.mock('@/lib/api', () => ({
  default: {
    access: { assignRole: mocks.assignRole },
    directory: { listUsers: mocks.listUsers },
  },
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

const role = {
  id: 'role-1',
  name: 'Audit reviewer',
  version: 2,
} as ManagedRole;

const ada: DirectoryUser = {
  id: '11111111-1111-4111-8111-111111111111',
  organization_id: '22222222-2222-4222-8222-222222222222',
  email: 'ada@example.com',
  first_name: 'Ada',
  last_name: 'Lovelace',
  job_title: 'Assurance lead',
  department: 'Internal Audit',
  status: 'active',
  is_super_admin: false,
  language: 'en',
  mfa_enabled: true,
  invitation_status: 'accepted',
  version: 3,
  role_slugs: [],
  group_count: 1,
  created_at: '2026-09-01T10:00:00Z',
  updated_at: '2026-09-14T10:00:00Z',
};

function envelope(users: DirectoryUser[]) {
  return {
    data: users,
    pagination: { page: 1, page_size: 20, total_items: users.length, total_pages: users.length ? 1 : 0 },
  };
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function renderDialog() {
  return render(<RoleAssignDialog role={role} open onOpenChange={vi.fn()} />, { wrapper: Providers });
}

describe('directory-backed role assignment picker', () => {
  beforeEach(() => {
    mocks.assignRole.mockReset().mockResolvedValue({ message: 'assigned' });
    mocks.listUsers.mockReset();
  });

  it('debounces active-user search and supports keyboard selection without exposing UUID entry', async () => {
    const response = envelope([ada]);
    let resolveSearch: (value: typeof response) => void = () => undefined;
    mocks.listUsers.mockImplementation(() => new Promise<typeof response>((resolve) => {
      resolveSearch = resolve;
    }));
    const user = userEvent.setup();
    renderDialog();

    expect(screen.getByRole('group', { name: 'User *' })).toBeVisible();
    const combobox = screen.getByRole('combobox', { name: 'Search active users' });
    expect(combobox).toHaveAccessibleDescription(/arrow keys/i);
    expect(screen.queryByLabelText(/UUID/i)).not.toBeInTheDocument();
    await user.type(combobox, 'Ada');

    expect(await screen.findByText('Searching active users…')).toBeVisible();
    await waitFor(() => expect(mocks.listUsers).toHaveBeenCalledWith({
      search: 'Ada',
      status: 'active',
      sort_by: 'name',
      sort_dir: 'asc',
      page: 1,
      page_size: 20,
    }, expect.anything()));
    act(() => resolveSearch(response));
    expect(await screen.findByText('Ada Lovelace')).toBeVisible();

    await user.keyboard('{ArrowDown}{Enter}');
    expect(screen.getByLabelText('Selected user')).toHaveTextContent('Ada Lovelace');
    expect(screen.getByLabelText('Selected user')).toHaveTextContent('ada@example.com');
    expect(screen.getByLabelText('Selected user')).toHaveTextContent('Internal Audit');
    expect(screen.queryByRole('combobox', { name: 'Search active users' })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText('Business reason *'), 'Assigned to audit engagement');
    await user.click(screen.getByRole('button', { name: 'Assign role' }));
    expect(mocks.assignRole).toHaveBeenCalledWith('role-1', {
      user_id: ada.id,
      reason: 'Assigned to audit engagement',
    });
  });

  it('announces an empty active-user result set', async () => {
    mocks.listUsers.mockResolvedValue(envelope([]));
    const user = userEvent.setup();
    renderDialog();

    await user.type(screen.getByRole('combobox', { name: 'Search active users' }), 'Nobody');
    expect(await screen.findByText('No active users matched “Nobody”.')).toHaveAttribute('role', 'status');
  });

  it('requires a directory selection before assigning the role', async () => {
    const user = userEvent.setup();
    renderDialog();

    await user.type(screen.getByLabelText('Business reason *'), 'New audit duties');
    await user.click(screen.getByRole('button', { name: 'Assign role' }));
    expect(screen.getByRole('alert')).toHaveTextContent('Select an active user from the directory.');
    expect(mocks.assignRole).not.toHaveBeenCalled();
  });

  it('exposes a recoverable search error and retries on request', async () => {
    mocks.listUsers.mockRejectedValueOnce({ status: 503, message: 'Unavailable' }).mockResolvedValueOnce(envelope([ada]));
    const user = userEvent.setup();
    renderDialog();

    await user.type(screen.getByRole('combobox', { name: 'Search active users' }), 'Ada');
    expect(await screen.findByRole('alert')).toHaveTextContent('Active users could not be loaded.');
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Ada Lovelace')).toBeVisible();
    expect(mocks.listUsers).toHaveBeenCalledTimes(2);
  });
});
