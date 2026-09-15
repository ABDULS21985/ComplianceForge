import type { ControlEvidence, ControlRecord } from '@/types/control-evidence';
import { describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';

import api from '@/lib/api';
import { ControlEvidencePanel } from '@/components/evidence/control-evidence-panel';
import type { ReactNode } from 'react';

const evidence: ControlEvidence = {
  id: 'evidence/unsafe',
  organization_id: 'organization-1',
  control_implementation_id: 'implementation-1',
  title: 'Access review',
  evidence_type: 'report',
  file_name: '../../access\u202ereview.pdf',
  file_size_bytes: 1200,
  collection_method: 'manual_upload',
  collected_at: '2026-09-14T00:00:00Z',
  valid_until: '2026-09-20T00:00:00Z',
  is_current: true,
  review_status: 'pending',
  metadata: { object_security: { scan_engine: 'withheld from presentation' } },
  series_id: 'series-1',
  version_number: 1,
  lifecycle_status: 'active',
  version_reason: 'Initial evidence upload',
  content_fingerprint: 'b'.repeat(64),
  created_at: '2026-09-14T00:00:00Z',
  updated_at: '2026-09-14T00:00:00Z',
};

const control: ControlRecord = {
  id: 'control/unsafe',
  framework_id: 'framework-1',
  code: 'A.5.1',
  title: 'Access control policy',
  implementation: {
    id: 'implementation-1',
    organization_id: 'organization-1',
    framework_control_id: 'control/unsafe',
    organization_framework_id: 'organization-framework-1',
    status: 'implemented',
    implementation_status: 'completed',
    maturity_level: 4,
    tags: [],
    metadata: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
};

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('control evidence panel permission boundary', () => {
  it('does not request an evidence collection for a control the tenant has not adopted', async () => {
    const listEvidence = vi.spyOn(api.controls, 'listEvidence');
    render(
      <ControlEvidencePanel
        control={{ ...control, implementation: undefined }}
        canUpdate
        canExport
        canApprove
      />,
      { wrapper: Providers },
    );

    expect(screen.getByText(/has not been adopted by the tenant/i)).toBeVisible();
    expect(screen.getByRole('button', { name: 'Upload evidence' })).toBeDisabled();
    await waitFor(() => expect(listEvidence).not.toHaveBeenCalled());
  });

  it('renders non-colour statuses and withholds every action without its permission', async () => {
    vi.spyOn(api.controls, 'listEvidence').mockResolvedValue({
      data: [evidence],
      pagination: { page: 1, page_size: 10, total_items: 1, total_pages: 1 },
    });
    render(
      <ControlEvidencePanel
        control={control}
        canUpdate={false}
        canExport={false}
        canApprove={false}
      />,
      { wrapper: Providers },
    );

    expect(await screen.findByRole('heading', { name: 'Access review' })).toBeVisible();
    expect(screen.getByText('Pending')).toBeVisible();
    expect(screen.getByText(/Expir/)).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Upload evidence' })).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Open history for Access review, version 1' }),
    ).toBeVisible();
    expect(screen.queryByRole('link', { name: 'Download Access review' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Review Access review' })).not.toBeInTheDocument();
    expect(screen.queryByText('withheld from presentation')).not.toBeInTheDocument();
  });

  it('exposes only permission-authorized actions with hygienic download attributes', async () => {
    vi.spyOn(api.controls, 'listEvidence').mockResolvedValue({
      data: [evidence],
      pagination: { page: 1, page_size: 10, total_items: 1, total_pages: 1 },
    });
    render(<ControlEvidencePanel control={control} canUpdate canExport canApprove />, {
      wrapper: Providers,
    });

    expect(await screen.findByRole('button', { name: 'Upload evidence' })).toBeVisible();
    const download = await screen.findByRole('link', { name: 'Download Access review' });
    expect(download).toHaveAttribute(
      'href',
      '/api/bff/controls/control%2Funsafe/evidence/evidence%2Funsafe/download',
    );
    expect(download).toHaveAttribute('download', '_.._access_review.pdf');
    expect(download).toHaveAttribute('referrerpolicy', 'no-referrer');
    expect(screen.getByRole('button', { name: 'Review Access review' })).toBeVisible();
    await waitFor(() => expect(api.controls.listEvidence).toHaveBeenCalledOnce());
  });

  it('keeps superseded versions inspectable and exportable but not reviewable', async () => {
    vi.spyOn(api.controls, 'listEvidence').mockResolvedValue({
      data: [{ ...evidence, is_current: false, lifecycle_status: 'superseded' }],
      pagination: { page: 1, page_size: 10, total_items: 1, total_pages: 1 },
    });
    render(<ControlEvidencePanel control={control} canUpdate canExport canApprove />, {
      wrapper: Providers,
    });

    expect((await screen.findAllByText('Superseded')).length).toBeGreaterThan(0);
    expect(screen.getByRole('link', { name: 'Download Access review' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Review Access review' })).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Open history for Access review, version 1' }),
    ).toBeVisible();
  });
});
