import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import type { DirectoryUser } from '@/types/directory';
import { LegalHoldEditorDialog } from './legal-hold-editor-dialog';
import userEvent from '@testing-library/user-event';

const { createHold, listUsers } = vi.hoisted(() => ({ createHold: vi.fn(), listUsers: vi.fn() }));
vi.mock('@/lib/api', () => ({
  default: { dataGovernance: { createHold, updateHold: vi.fn() }, directory: { listUsers } },
}));

const owner: DirectoryUser = {
  id: '11111111-1111-4111-8111-111111111111',
  organization_id: '22222222-2222-4222-8222-222222222222',
  email: 'ada@example.com',
  first_name: 'Ada',
  last_name: 'Lovelace',
  department: 'Legal',
  status: 'active',
  is_super_admin: false,
  language: 'en',
  mfa_enabled: true,
  invitation_status: 'accepted',
  version: 1,
  role_slugs: [],
  group_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('LegalHoldEditorDialog', () => {
  beforeEach(() => {
    listUsers
      .mockReset()
      .mockResolvedValue({
        data: [owner],
        pagination: { page: 1, page_size: 20, total_items: 1, total_pages: 1 },
      });
    createHold.mockReset().mockResolvedValue({ id: 'hold-1', version: 1 });
  });

  it('uses distinctly named directory pickers and submits the selected owner identity', async () => {
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LegalHoldEditorDialog onOpenChange={vi.fn()} onReload={vi.fn()} onSaved={vi.fn()} open />
      </QueryClientProvider>,
    );

    expect(screen.getByRole('combobox', { name: 'Search for hold owner' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Search for custodian' })).toBeInTheDocument();
    await user.type(screen.getByRole('combobox', { name: 'Search for hold owner' }), 'Ada');
    await screen.findByText('Ada Lovelace');
    await user.keyboard('{ArrowDown}{Enter}');
    expect(screen.getByLabelText('Selected hold owner')).toHaveTextContent('ada@example.com');

    await user.type(screen.getByLabelText('Hold name'), 'Regulatory investigation');
    await user.type(
      screen.getByLabelText('Description'),
      'Preserve evidence for the active investigation.',
    );
    await user.type(
      screen.getByLabelText('Legal authority'),
      'Written direction from external counsel.',
    );
    await user.type(
      screen.getByLabelText('Reason for change'),
      'Counsel issued a preservation notice',
    );
    await user.click(screen.getByRole('button', { name: 'Place hold' }));

    await waitFor(() =>
      expect(createHold).toHaveBeenCalledWith(
        expect.objectContaining({
          owner_user_id: owner.id,
          custodian_ids: [],
          reason: 'Counsel issued a preservation notice',
          scope: {},
        }),
      ),
    );
  });
});
