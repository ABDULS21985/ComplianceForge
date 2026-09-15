import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import api from '@/lib/api';
import EvidenceOverviewPage from '@/app/(dashboard)/evidence/page';
import type { ReactNode } from 'react';

const mocks = vi.hoisted(() => ({
  permission: {
    canApprove: false,
    canExport: false,
    canRead: false,
    canUpdate: false,
    isError: false,
    isLoading: false,
    retry: vi.fn(),
  },
}));

vi.mock('@/hooks/use-control-permissions', () => ({
  useControlPermissions: () => mocks.permission,
}));

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('evidence overview access and empty states', () => {
  beforeEach(() => {
    Object.assign(mocks.permission, {
      canApprove: false,
      canExport: false,
      canRead: false,
      canUpdate: false,
      isError: false,
      isLoading: false,
    });
  });

  it('fails closed without mounting the control query', () => {
    const list = vi.spyOn(api.controls, 'list');
    render(<EvidenceOverviewPage />, { wrapper: Providers });

    expect(screen.getByRole('heading', { name: 'Evidence workspace unavailable' })).toBeVisible();
    expect(list).not.toHaveBeenCalled();
  });

  it('labels page-scoped metrics and routes keyboard-accessible control cards', async () => {
    Object.assign(mocks.permission, { canRead: true });
    vi.spyOn(api.controls, 'list').mockResolvedValue({
      data: [
        { id: 'control-1', framework_id: 'framework-1', code: 'A.5.1', title: 'Access control' },
      ],
      pagination: { page: 1, page_size: 12, total_items: 1, total_pages: 1 },
    });
    render(<EvidenceOverviewPage />, { wrapper: Providers });

    expect(await screen.findByText('Controls on this page')).toBeVisible();
    expect(
      screen.getByRole('link', { name: 'Open evidence for A.5.1: Access control' }),
    ).toHaveAttribute('href', '/controls/control-1');
    expect(screen.getByText(/does not expose a cross-control evidence collection/)).toBeVisible();
  });
});
