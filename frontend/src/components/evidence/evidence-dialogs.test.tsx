import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import api from '@/lib/api';
import type { ControlEvidence } from '@/types/control-evidence';
import { EvidenceReviewDialog } from '@/components/evidence/evidence-review-dialog';
import { EvidenceUploadDialog } from '@/components/evidence/evidence-upload-dialog';
import userEvent from '@testing-library/user-event';

const evidence: ControlEvidence = {
  id: 'evidence-1',
  organization_id: 'organization-1',
  control_implementation_id: 'implementation-1',
  title: 'Access review',
  evidence_type: 'document',
  file_name: 'review.pdf',
  file_size_bytes: 100,
  mime_type: 'application/pdf',
  file_hash: 'a'.repeat(64),
  collection_method: 'manual_upload',
  collected_at: '2026-09-14T00:00:00Z',
  is_current: true,
  review_status: 'pending',
  metadata: {},
  series_id: 'series-1',
  version_number: 1,
  lifecycle_status: 'active',
  version_reason: 'Initial evidence upload',
  content_fingerprint: 'b'.repeat(64),
  created_at: '2026-09-14T00:00:00Z',
  updated_at: '2026-09-14T00:00:00Z',
};

describe('evidence upload dialog', () => {
  it('uses an accessible single-file input and focuses validation feedback', async () => {
    const user = userEvent.setup();
    render(
      <EvidenceUploadDialog
        controlId="control-1"
        open
        onOpenChange={vi.fn()}
        onUploaded={vi.fn()}
      />,
    );

    const input = screen.getByLabelText('Evidence file');
    expect(input).not.toHaveAttribute('multiple');
    expect(input).toHaveAttribute('accept', expect.stringContaining('.pdf'));
    await user.click(screen.getByRole('button', { name: 'Upload and scan' }));
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('Choose exactly one evidence file');
    expect(alert).toHaveFocus();
  });

  it('submits typed metadata and a cancel signal without client-derived security fields', async () => {
    const user = userEvent.setup();
    const onUploaded = vi.fn();
    const upload = vi.spyOn(api.controls, 'uploadEvidence').mockResolvedValue(evidence);
    render(
      <EvidenceUploadDialog
        controlId="control-1"
        open
        onOpenChange={vi.fn()}
        onUploaded={onUploaded}
      />,
    );

    await user.upload(
      screen.getByLabelText('Evidence file'),
      new File(['%PDF'], 'access-review.pdf', { type: 'application/pdf' }),
    );
    await user.clear(screen.getByLabelText('Title'));
    await user.type(screen.getByLabelText('Title'), 'Quarterly access review');
    await user.selectOptions(screen.getByLabelText('Evidence type'), 'report');
    fireEvent.change(screen.getByLabelText('Metadata (optional JSON object)'), {
      target: { value: '{"source":"IAM"}' },
    });
    await user.click(screen.getByRole('button', { name: 'Upload and scan' }));

    await waitFor(() => expect(onUploaded).toHaveBeenCalledWith(evidence));
    expect(upload).toHaveBeenCalledWith(
      'control-1',
      expect.objectContaining({
        title: 'Quarterly access review',
        evidence_type: 'report',
        metadata: { source: 'IAM' },
      }),
      expect.any(AbortSignal),
    );
    const input = upload.mock.calls[0][1] as unknown as Record<string, unknown>;
    expect(input).not.toHaveProperty('file_hash');
    expect(input).not.toHaveProperty('object_key');
  });

  it('allows an in-flight upload to be cancelled', async () => {
    const user = userEvent.setup();
    vi.spyOn(api.controls, 'uploadEvidence').mockImplementation(
      (_controlId, _input, signal) =>
        new Promise((_resolve, reject) => {
          signal?.addEventListener(
            'abort',
            () => reject(Object.assign(new Error('cancelled'), { name: 'AbortError' })),
            { once: true },
          );
        }),
    );
    render(
      <EvidenceUploadDialog
        controlId="control-1"
        open
        onOpenChange={vi.fn()}
        onUploaded={vi.fn()}
      />,
    );
    await user.upload(
      screen.getByLabelText('Evidence file'),
      new File(['%PDF'], 'access-review.pdf', { type: 'application/pdf' }),
    );
    await user.click(screen.getByRole('button', { name: 'Upload and scan' }));
    await user.click(await screen.findByRole('button', { name: 'Cancel upload' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Upload cancelled');
  });

  it('requires an immutable reason and submits a single replacement version', async () => {
    const user = userEvent.setup();
    const replacement = {
      ...evidence,
      id: 'evidence-2',
      version_number: 2,
      supersedes_evidence_id: evidence.id,
      version_reason: 'Quarterly refresh',
    };
    const supersede = vi.spyOn(api.controls, 'supersedeEvidence').mockResolvedValue(replacement);
    const onUploaded = vi.fn();
    render(
      <EvidenceUploadDialog
        controlId="control-1"
        supersedes={evidence}
        open
        onOpenChange={vi.fn()}
        onUploaded={onUploaded}
      />,
    );

    expect(
      screen.getByRole('dialog', { name: 'Upload replacement evidence version' }),
    ).toBeVisible();
    await user.upload(
      screen.getByLabelText('Evidence file'),
      new File(['%PDF-v2'], 'access-review-v2.pdf', { type: 'application/pdf' }),
    );
    await user.click(screen.getByRole('button', { name: 'Upload replacement' }));
    expect(supersede).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('Version reason must contain');

    await user.type(screen.getByLabelText('Version reason'), 'Quarterly refresh');
    await user.click(screen.getByRole('button', { name: 'Upload replacement' }));

    await waitFor(() => expect(onUploaded).toHaveBeenCalledWith(replacement));
    expect(supersede).toHaveBeenCalledWith(
      'control-1',
      evidence.id,
      expect.objectContaining({
        file: expect.any(File),
        title: evidence.title,
        evidence_type: evidence.evidence_type,
        version_reason: 'Quarterly refresh',
      }),
      expect.any(AbortSignal),
    );
  });
});

describe('evidence review dialog', () => {
  it('requires and submits a rejection rationale through the canonical decision contract', async () => {
    const user = userEvent.setup();
    const reviewed = {
      ...evidence,
      review_status: 'rejected' as const,
      review_notes: 'Wrong period',
    };
    const review = vi.spyOn(api.controls, 'reviewEvidence').mockResolvedValue(reviewed);
    const onReviewed = vi.fn();
    render(
      <EvidenceReviewDialog
        controlId="control-1"
        evidence={evidence}
        open
        onOpenChange={vi.fn()}
        onReviewed={onReviewed}
      />,
    );

    expect(screen.getByRole('dialog', { name: 'Review evidence' })).toBeVisible();
    await user.click(screen.getByRole('radio', { name: /Reject/ }));
    await user.click(screen.getByRole('button', { name: 'Record rejection' }));
    expect(review).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('rejection rationale is required');
    expect(screen.getByRole('alert')).toHaveFocus();

    await user.type(screen.getByLabelText('Review notes (required)'), 'Wrong period');
    await user.click(screen.getByRole('button', { name: 'Record rejection' }));
    await waitFor(() => expect(onReviewed).toHaveBeenCalledWith(reviewed));
    expect(review).toHaveBeenCalledWith('control-1', 'evidence-1', {
      status: 'rejected',
      comment: 'Wrong period',
    });
  });
});
