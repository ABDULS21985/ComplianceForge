import type { ControlEvidence, EvidenceLifecycleRecord } from '@/types/control-evidence';
import { describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';

import api from '@/lib/api';
import { EvidenceLifecycleDialog } from '@/components/evidence/evidence-lifecycle-dialog';
import type { ReactNode } from 'react';
import userEvent from '@testing-library/user-event';

const evidence: ControlEvidence = {
  id: 'evidence-1',
  organization_id: 'organization-1',
  control_implementation_id: 'implementation-1',
  title: 'Access review',
  evidence_type: 'document',
  file_name: '../../review.pdf',
  file_size_bytes: 120,
  mime_type: 'application/pdf',
  file_hash: 'a'.repeat(64),
  collection_method: 'manual_upload',
  collected_at: '2026-09-14T00:00:00Z',
  is_current: true,
  review_status: 'accepted',
  metadata: {},
  series_id: 'series-1',
  version_number: 2,
  supersedes_evidence_id: 'evidence-0',
  lifecycle_status: 'active',
  version_reason: 'Quarterly refresh',
  content_fingerprint: 'b'.repeat(64),
  created_at: '2026-09-14T00:00:00Z',
  updated_at: '2026-09-14T00:00:00Z',
};

function lifecycle(overrides: Partial<EvidenceLifecycleRecord> = {}): EvidenceLifecycleRecord {
  return {
    evidence,
    versions: [
      evidence,
      {
        ...evidence,
        id: 'evidence-0',
        is_current: false,
        lifecycle_status: 'superseded',
        superseded_by_evidence_id: evidence.id,
        version_number: 1,
        version_reason: 'Initial evidence upload',
      },
    ],
    reviews: [
      {
        id: 'review-1',
        organization_id: 'organization-1',
        evidence_id: evidence.id,
        decision: 'accepted',
        comment: 'Coverage confirmed',
        reviewer_id: 'reviewer-1',
        evidence_sha256: 'a'.repeat(64),
        metadata: {},
        created_at: '2026-09-14T02:00:00Z',
      },
    ],
    custody_events: [
      {
        id: 'event-1',
        organization_id: 'organization-1',
        evidence_id: evidence.id,
        series_id: evidence.series_id,
        sequence: 1,
        previous_hash: '',
        event_hash: 'c'.repeat(64),
        event_type: 'uploaded',
        actor_user_id: 'uploader-1',
        actor_type: 'user',
        reason: 'Evidence uploaded',
        details: {},
        created_at: '2026-09-14T00:00:00Z',
      },
      {
        id: 'event-2',
        organization_id: 'organization-1',
        evidence_id: evidence.id,
        series_id: evidence.series_id,
        sequence: 2,
        previous_hash: 'c'.repeat(64),
        event_hash: 'd'.repeat(64),
        event_type: 'integrity_failed',
        actor_type: 'system',
        reason: 'Evidence object integrity verification failed',
        details: {},
        created_at: '2026-09-14T03:00:00Z',
      },
    ],
    chain: {
      evidence_id: evidence.id,
      valid: true,
      event_count: 2,
      head_sequence: 2,
      head_hash: 'd'.repeat(64),
    },
    legal_hold_active: false,
    ...overrides,
  };
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('evidence lifecycle dialog', () => {
  it('presents legal-hold, invalid-chain, review, and custody state without relying on colour', async () => {
    const user = userEvent.setup();
    vi.spyOn(api.controls, 'evidenceHistory').mockResolvedValue(
      lifecycle({
        chain: {
          evidence_id: evidence.id,
          valid: false,
          event_count: 2,
          head_sequence: 2,
          head_hash: 'd'.repeat(64),
          first_invalid_sequence: 2,
        },
        legal_hold_active: true,
      }),
    );
    render(
      <EvidenceLifecycleDialog
        canExport={false}
        canUpdate={false}
        controlId="control-1"
        evidence={evidence}
        open
        onOpenChange={vi.fn()}
        onSupersede={vi.fn()}
      />,
      { wrapper: Providers },
    );

    expect(await screen.findByText('Chain verification failed')).toBeVisible();
    expect(screen.getByText('Active hold')).toBeVisible();
    expect(screen.getByText('Mismatch detected')).toBeVisible();
    expect(screen.getByRole('alert')).toHaveTextContent('failed at sequence 2');
    expect(screen.queryByRole('button', { name: 'Verify stored file' })).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'Upload replacement version' }),
    ).not.toBeInTheDocument();

    const versionsTab = screen.getByRole('tab', { name: 'Versions (2)' });
    versionsTab.focus();
    await user.keyboard('{ArrowRight}');
    expect(screen.getByRole('tab', { name: 'Reviews (1)' })).toHaveFocus();
    expect(screen.getByText('Coverage confirmed')).toBeVisible();
    await user.click(screen.getByRole('tab', { name: 'Custody (2)' }));
    expect(screen.getByText('Integrity failed')).toBeVisible();
    expect(screen.getByText('Evidence object integrity verification failed')).toBeVisible();
  });

  it('allows an updater to verify integrity and start a replacement with safe history downloads', async () => {
    const user = userEvent.setup();
    vi.spyOn(api.controls, 'evidenceHistory').mockResolvedValue(lifecycle());
    const verify = vi.spyOn(api.controls, 'verifyEvidenceIntegrity').mockResolvedValue({
      evidence_id: evidence.id,
      valid: true,
      sha256: 'a'.repeat(64),
      size_bytes: 120,
      verified_at: '2026-09-14T04:00:00Z',
    });
    const onOpenChange = vi.fn();
    const onSupersede = vi.fn();
    render(
      <EvidenceLifecycleDialog
        canExport
        canUpdate
        controlId="control-1"
        evidence={evidence}
        open
        onOpenChange={onOpenChange}
        onSupersede={onSupersede}
      />,
      { wrapper: Providers },
    );

    await user.click(await screen.findByRole('button', { name: 'Verify stored file' }));
    await waitFor(() => expect(verify).toHaveBeenCalledWith('control-1', evidence.id, undefined));
    expect(await screen.findByRole('status')).toHaveTextContent('matched the immutable fingerprint');

    const download = screen.getByRole('link', {
      name: 'Download version 2 of Access review',
    });
    expect(download).toHaveAttribute(
      'href',
      '/api/bff/controls/control-1/evidence/evidence-1/download',
    );
    expect(download).toHaveAttribute('download', '_.._review.pdf');
    expect(download).toHaveAttribute('referrerpolicy', 'no-referrer');

    await user.click(screen.getByRole('button', { name: 'Upload replacement version' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onSupersede).toHaveBeenCalledWith(evidence);
  });

  it('focuses and explains an integrity mismatch without exposing provider errors', async () => {
    const user = userEvent.setup();
    vi.spyOn(api.controls, 'evidenceHistory').mockResolvedValue(lifecycle());
    vi.spyOn(api.controls, 'verifyEvidenceIntegrity').mockRejectedValue({
      status: 409,
      message: 's3 provider path and secret detail',
    });
    render(
      <EvidenceLifecycleDialog
        canExport={false}
        canUpdate
        controlId="control-1"
        evidence={evidence}
        open
        onOpenChange={vi.fn()}
        onSupersede={vi.fn()}
      />,
      { wrapper: Providers },
    );

    await user.click(await screen.findByRole('button', { name: 'Verify stored file' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('stored object no longer matches');
    expect(alert).not.toHaveTextContent('provider');
    expect(alert).toHaveFocus();
  });
});
