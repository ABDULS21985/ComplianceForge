import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ManagedRole, PermissionGrant } from '@/types/access-admin';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { RoleEditorDialog } from '@/components/access-admin/role-editor-dialog';
import userEvent from '@testing-library/user-event';

const { previewRoleImpact, updateRole } = vi.hoisted(() => ({
  previewRoleImpact: vi.fn(),
  updateRole: vi.fn(),
}));
vi.mock('@/lib/api', () => ({
  default: {
    access: {
      createRole: vi.fn(),
      previewRoleImpact,
      updateRole,
    },
  },
}));

const catalogue: PermissionGrant[] = [
  { id: 'permission-1', resource: 'audits', action: 'read', description: 'Read audits' },
  { id: 'permission-2', resource: 'audits', action: 'update', description: 'Update audits' },
];
const role: ManagedRole = {
  id: 'role-1',
  name: 'Audit reviewer',
  slug: 'audit-reviewer',
  is_system_role: false,
  is_custom: true,
  version: 7,
  permissions: catalogue,
  assigned_users: 3,
  created_at: '2026-09-10T09:00:00Z',
  updated_at: '2026-09-10T09:00:00Z',
};

describe('RoleEditorDialog', () => {
  beforeEach(() => {
    previewRoleImpact.mockReset().mockResolvedValue({
      role_id: 'role-1',
      assigned_users: 3,
      current_permissions: 2,
      proposed_permissions: 1,
      added: [],
      removed: [catalogue[1]],
    });
    updateRole.mockReset().mockResolvedValue({ ...role, version: 8, permissions: [catalogue[0]] });
  });

  it('requires an impact preview and a second confirmation before removing permissions', async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <RoleEditorDialog catalogue={catalogue} role={role} open onOpenChange={vi.fn()} onSaved={vi.fn()} />
      </QueryClientProvider>
    );
    await user.click(screen.getByRole('checkbox', { name: /Update/i }));
    await user.click(screen.getByRole('button', { name: 'Review and save' }));
    expect(previewRoleImpact).toHaveBeenCalledWith('role-1', [{ resource: 'audits', action: 'read' }]);
    expect(updateRole).not.toHaveBeenCalled();
    expect(await screen.findByRole('alert')).toHaveTextContent('3 assigned users');
    await user.click(screen.getByRole('button', { name: 'Confirm and save' }));
    expect(updateRole).toHaveBeenCalledWith('role-1', expect.objectContaining({
      expected_version: 7,
      permissions: [{ resource: 'audits', action: 'read' }],
    }));
  });

  it('never bypasses impact review after a preview failure', async () => {
    previewRoleImpact.mockRejectedValueOnce({ status: 500, message: 'Unavailable' });
    const user = userEvent.setup();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <RoleEditorDialog catalogue={catalogue} role={role} open onOpenChange={vi.fn()} onSaved={vi.fn()} />
      </QueryClientProvider>
    );
    await user.click(screen.getByRole('checkbox', { name: /Update/i }));
    await user.click(screen.getByRole('button', { name: 'Review and save' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Unavailable');
    await user.click(screen.getByRole('button', { name: 'Review and save' }));
    expect(previewRoleImpact).toHaveBeenCalledTimes(2);
    expect(updateRole).not.toHaveBeenCalled();
  });
});
