'use client';

import * as React from 'react';

import {
  AlertTriangle,
  CalendarClock,
  CalendarX2,
  CheckCircle2,
  Clock3,
  Download,
  FileCheck2,
  FileQuestion,
  History,
  Loader2,
  RefreshCw,
  ShieldCheck,
  UploadCloud,
  XCircle,
} from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn, formatDate, formatDateTime } from '@/lib/utils';
import type {
  ControlEvidence,
  ControlEvidenceEnvelope,
  ControlRecord,
  EvidenceReviewStatus,
} from '@/types/control-evidence';
import {
  evidenceDownloadHref,
  evidenceFreshness,
  type EvidenceFreshness,
  evidenceStatusLabel,
  formatEvidenceError,
  safeEvidenceFilename,
} from '@/lib/control-evidence';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EvidenceLifecycleDialog } from '@/components/evidence/evidence-lifecycle-dialog';
import { EvidenceReviewDialog } from '@/components/evidence/evidence-review-dialog';
import { EvidenceUploadDialog } from '@/components/evidence/evidence-upload-dialog';
import { Skeleton } from '@/components/ui/skeleton';

interface ControlEvidencePanelProps {
  canApprove: boolean;
  canExport: boolean;
  canUpdate: boolean;
  control: ControlRecord;
}

const PAGE_SIZE = 10;

const reviewPresentation: Record<EvidenceReviewStatus, { className: string; icon: typeof Clock3 }> =
  {
    accepted: {
      className:
        'border-emerald-300 bg-emerald-50 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300',
      icon: CheckCircle2,
    },
    rejected: {
      className: 'border-red-300 bg-red-50 text-red-800 dark:bg-red-950/40 dark:text-red-300',
      icon: XCircle,
    },
    expired: {
      className:
        'border-slate-300 bg-slate-50 text-slate-800 dark:bg-slate-900 dark:text-slate-300',
      icon: CalendarX2,
    },
    pending: {
      className:
        'border-amber-300 bg-amber-50 text-amber-800 dark:bg-amber-950/40 dark:text-amber-300',
      icon: Clock3,
    },
  };

const freshnessPresentation: Record<
  EvidenceFreshness,
  { className: string; icon: typeof ShieldCheck }
> = {
  current: { className: 'text-emerald-700 dark:text-emerald-300', icon: ShieldCheck },
  expiring: { className: 'text-amber-700 dark:text-amber-300', icon: AlertTriangle },
  expired: { className: 'text-red-700 dark:text-red-300', icon: CalendarX2 },
  scheduled: { className: 'text-blue-700 dark:text-blue-300', icon: CalendarClock },
  superseded: { className: 'text-muted-foreground', icon: History },
};

function formatBytes(value: number | undefined): string {
  if (value === undefined) return 'Size unavailable';
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  return `${(value / 1024 / 1024).toFixed(1)} MiB`;
}

function ReviewBadge({ status }: { status: EvidenceReviewStatus }) {
  const presentation = reviewPresentation[status];
  const Icon = presentation.icon;
  return (
    <Badge variant="outline" className={cn('gap-1', presentation.className)}>
      <Icon aria-hidden="true" className="h-3.5 w-3.5" />
      {evidenceStatusLabel(status)}
    </Badge>
  );
}

function FreshnessStatus({ evidence }: { evidence: ControlEvidence }) {
  const status = evidenceFreshness(evidence);
  const presentation = freshnessPresentation[status];
  const Icon = presentation.icon;
  return (
    <span
      className={cn('inline-flex items-center gap-1 text-xs font-medium', presentation.className)}
    >
      <Icon aria-hidden="true" className="h-3.5 w-3.5" />
      {evidenceStatusLabel(status)}
    </span>
  );
}

function EvidenceCard({
  canApprove,
  canExport,
  controlId,
  evidence,
  onHistory,
  onReview,
}: {
  canApprove: boolean;
  canExport: boolean;
  controlId: string;
  evidence: ControlEvidence;
  onHistory: (evidence: ControlEvidence) => void;
  onReview: (evidence: ControlEvidence) => void;
}) {
  return (
    <article className="rounded-lg border p-4" aria-labelledby={`evidence-${evidence.id}-title`}>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-muted">
            <FileCheck2 aria-hidden="true" className="h-5 w-5 text-muted-foreground" />
          </div>
          <div className="min-w-0 space-y-1">
            <h3 id={`evidence-${evidence.id}-title`} className="break-words font-medium">
              {evidence.title}
            </h3>
            {evidence.file_name && (
              <p className="break-all text-xs text-muted-foreground">
                {safeEvidenceFilename(evidence.file_name)} · {formatBytes(evidence.file_size_bytes)}
              </p>
            )}
            {evidence.description && (
              <p className="max-w-3xl whitespace-pre-wrap text-sm text-muted-foreground">
                {evidence.description}
              </p>
            )}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <ReviewBadge status={evidence.review_status} />
          <Badge variant="outline">Version {evidence.version_number}</Badge>
          <Badge variant="secondary">{evidenceStatusLabel(evidence.lifecycle_status)}</Badge>
          <Badge variant="secondary">{evidenceStatusLabel(evidence.evidence_type)}</Badge>
        </div>
      </div>

      <dl className="mt-4 grid gap-3 border-t pt-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <dt className="text-xs text-muted-foreground">Freshness</dt>
          <dd className="mt-1">
            <FreshnessStatus evidence={evidence} />
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Collected</dt>
          <dd className="mt-1">{formatDateTime(evidence.collected_at)}</dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Valid from</dt>
          <dd className="mt-1">{formatDate(evidence.valid_from)}</dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Valid until</dt>
          <dd className="mt-1">{formatDate(evidence.valid_until)}</dd>
        </div>
      </dl>

      {evidence.review_notes && (
        <div className="mt-4 rounded-md bg-muted/50 p-3 text-sm">
          <p className="text-xs font-medium text-muted-foreground">Latest review note</p>
          <p className="mt-1 whitespace-pre-wrap">{evidence.review_notes}</p>
          {evidence.reviewed_at && (
            <p className="mt-1 text-xs text-muted-foreground">
              Reviewed {formatDateTime(evidence.reviewed_at)}
            </p>
          )}
        </div>
      )}

      <div className="mt-4 flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          aria-label={`Open history for ${evidence.title}, version ${evidence.version_number}`}
          onClick={() => onHistory(evidence)}
        >
          <History aria-hidden="true" className="mr-2 h-4 w-4" />
          History &amp; integrity
        </Button>
        {canExport && evidence.file_name && (
          <Button asChild size="sm" variant="outline">
            <a
              href={evidenceDownloadHref(controlId, evidence.id)}
              download={safeEvidenceFilename(evidence.file_name)}
              referrerPolicy="no-referrer"
              aria-label={`Download ${evidence.title}`}
            >
              <Download aria-hidden="true" className="mr-2 h-4 w-4" />
              Download
            </a>
          </Button>
        )}
        {canApprove && evidence.is_current && evidence.lifecycle_status === 'active' && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            aria-label={`Review ${evidence.title}`}
            onClick={() => onReview(evidence)}
          >
            <ShieldCheck aria-hidden="true" className="mr-2 h-4 w-4" />
            Review
          </Button>
        )}
      </div>
    </article>
  );
}

export function ControlEvidencePanel({
  canApprove,
  canExport,
  canUpdate,
  control,
}: ControlEvidencePanelProps) {
  const [announcement, setAnnouncement] = React.useState('');
  const [inspecting, setInspecting] = React.useState<ControlEvidence | null>(null);
  const [page, setPage] = React.useState(1);
  const [reviewing, setReviewing] = React.useState<ControlEvidence | null>(null);
  const [superseding, setSuperseding] = React.useState<ControlEvidence | null>(null);
  const [uploadOpen, setUploadOpen] = React.useState(false);
  const queryClient = useQueryClient();
  const queryKey = ['controls', control.id, 'evidence', { page, page_size: PAGE_SIZE }] as const;
  const query = useQuery({
    queryKey,
    queryFn: () => api.controls.listEvidence(control.id, { page, page_size: PAGE_SIZE }),
    enabled: Boolean(control.implementation),
  });

  const refreshEvidence = async (message: string) => {
    setAnnouncement(message);
    await queryClient.invalidateQueries({ queryKey: ['controls', control.id, 'evidence'] });
  };

  const updateReviewedEvidence = (reviewed: ControlEvidence) => {
    queryClient.setQueryData<ControlEvidenceEnvelope>(queryKey, (current) =>
      current
        ? {
            ...current,
            data: current.data.map((item) => (item.id === reviewed.id ? reviewed : item)),
          }
        : current,
    );
    setReviewing(null);
    void queryClient.invalidateQueries({
      queryKey: ['controls', control.id, 'evidence', reviewed.id, 'history'],
    });
    void refreshEvidence(`Review decision recorded for ${reviewed.title}.`);
  };

  return (
    <Card>
      <CardHeader className="gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle>Control evidence</CardTitle>
          <CardDescription className="mt-1">
            Evidence is tenant-scoped and ordered by collection time. Storage keys and provider
            details are never displayed.
          </CardDescription>
        </div>
        {canUpdate && (
          <Button
            type="button"
            disabled={!control.implementation}
            onClick={() => setUploadOpen(true)}
          >
            <UploadCloud aria-hidden="true" className="mr-2 h-4 w-4" />
            Upload evidence
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="sr-only" aria-live="polite">
          {announcement}
        </p>
        {!control.implementation && (
          <div
            role="status"
            className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200"
          >
            This framework control has not been adopted by the tenant. Adopt its framework before
            uploading control evidence.
          </div>
        )}
        {!canUpdate && (
          <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
            Read-only access: uploading requires the controls:update permission.
          </p>
        )}
        {query.isLoading && (
          <div role="status" aria-label="Loading control evidence" className="space-y-3">
            <Skeleton className="h-36" />
            <Skeleton className="h-36" />
          </div>
        )}
        {query.isError && (
          <div className="rounded-md border p-6 text-center">
            <FileQuestion aria-hidden="true" className="mx-auto h-8 w-8 text-muted-foreground" />
            <p role="alert" className="mt-3 text-sm">
              {formatEvidenceError(query.error, 'list')}
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-4"
              onClick={() => void query.refetch()}
            >
              <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
              Try again
            </Button>
          </div>
        )}
        {query.isSuccess && query.data.data.length === 0 && (
          <div className="rounded-md border border-dashed py-10 text-center">
            <FileQuestion aria-hidden="true" className="mx-auto h-9 w-9 text-muted-foreground" />
            <h3 className="mt-3 font-medium">No evidence records</h3>
            <p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
              Upload a supported file for this adopted control. An evidence record appears only
              after security scanning completes successfully.
            </p>
          </div>
        )}
        {query.isSuccess && query.data.data.length > 0 && (
          <div className="space-y-3">
            {query.data.data.map((evidence) => (
              <EvidenceCard
                key={evidence.id}
                canApprove={canApprove}
                canExport={canExport}
                controlId={control.id}
                evidence={evidence}
                onHistory={setInspecting}
                onReview={setReviewing}
              />
            ))}
          </div>
        )}
        {query.isSuccess && query.data.pagination.total_pages > 1 && (
          <nav
            aria-label="Evidence pages"
            className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between"
          >
            <p className="text-sm text-muted-foreground">
              Page {query.data.pagination.page} of {query.data.pagination.total_pages} ·{' '}
              {query.data.pagination.total_items} records
            </p>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page <= 1 || query.isFetching}
                onClick={() => setPage((value) => Math.max(1, value - 1))}
              >
                Previous
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page >= query.data.pagination.total_pages || query.isFetching}
                onClick={() => setPage((value) => value + 1)}
              >
                {query.isFetching && (
                  <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
                )}
                Next
              </Button>
            </div>
          </nav>
        )}
      </CardContent>

      {canUpdate && (
        <EvidenceUploadDialog
          controlId={control.id}
          open={uploadOpen}
          onOpenChange={setUploadOpen}
          onUploaded={(uploaded) => {
            void refreshEvidence(`${uploaded.title} was uploaded and passed security scanning.`);
          }}
        />
      )}
      {inspecting && (
        <EvidenceLifecycleDialog
          canExport={canExport}
          canUpdate={canUpdate}
          controlId={control.id}
          evidence={inspecting}
          open
          onOpenChange={(open) => {
            if (!open) setInspecting(null);
          }}
          onSupersede={setSuperseding}
        />
      )}
      {canUpdate && superseding && (
        <EvidenceUploadDialog
          controlId={control.id}
          supersedes={superseding}
          open
          onOpenChange={(open) => {
            if (!open) setSuperseding(null);
          }}
          onUploaded={(replacement) => {
            setSuperseding(null);
            void refreshEvidence(
              `${replacement.title} version ${replacement.version_number} was uploaded and passed security scanning.`,
            );
          }}
        />
      )}
      {canApprove && (
        <EvidenceReviewDialog
          controlId={control.id}
          evidence={reviewing}
          open={Boolean(reviewing)}
          onOpenChange={(open) => {
            if (!open) setReviewing(null);
          }}
          onReviewed={updateReviewedEvidence}
        />
      )}
    </Card>
  );
}
