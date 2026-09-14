import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import type { ManagedRole } from '@/types/access-admin';
import { RoleDeleteDialog } from '@/components/access-admin/role-delete-dialog';
import userEvent from '@testing-library/user-event';

const { deleteRole } = vi.hoisted(() => ({ deleteRole: vi.fn() }));
vi.mock('@/lib/api', () => ({ default: { access: { deleteRole } } }));

const role = {
  id: 'role-1',
  name: 'Audit reviewer',
  version: 4,
  assigned_users: 0,
  is_system_role: false,
} as ManagedRole;

function renderDialog(value: ManagedRole) {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <RoleDeleteDialog role={value} open onOpenChange={vi.fn()} onRefresh={vi.fn()} onDeleted={vi.fn()} />
    </QueryClientProvider>
  );
}

describe('RoleDeleteDialog', () => {
  beforeEach(() => deleteRole.mockReset().mockResolvedValue(undefined));

  it('requires the exact role name and sends the current version', async () => {
    const user = userEvent.setup();
    renderDialog(role);
    const button = screen.getByRole('button', { name: 'Delete role' });
    expect(button).toBeDisabled();
    await user.type(screen.getByLabelText(/Type Audit reviewer to confirm/), 'Audit reviewer');
    await user.click(button);
    expect(deleteRole).toHaveBeenCalledWith('role-1', 4);
  });

  it('blocks deletion while users remain assigned', () => {
    renderDialog({ ...role, assigned_users: 2 });
    expect(screen.getByRole('alert')).toHaveTextContent('Remove or transfer all 2 user assignments');
    expect(screen.queryByRole('button', { name: 'Delete role' })).not.toBeInTheDocument();
  });
});
