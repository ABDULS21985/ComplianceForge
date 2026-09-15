'use client';

import * as React from 'react';

import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Download,
  FileClock,
  Fingerprint,
  History,
  Loader2,
  LockKeyhole,
  RefreshCw,
  Replace,
  ShieldAlert,
  ShieldCheck,
  UserRound,
} from 'lucide-react';
import { cn, formatDate, formatDateTime } from '@/lib/utils';
import type {
  ControlEvidence,
  EvidenceCustodyEvent,
  EvidenceIntegrityResult,
} from '@/types/control-evidence';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  evidenceDownloadHref,
  evidenceStatusLabel,
  formatEvidenceError,
  safeEvidenceFilename,
} from '@/lib/control-evidence';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useEvidenceHistory, useVerifyEvidenceIntegrity } from '@/lib/api-hooks';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';

interface EvidenceLifecycleDialogProps {
  canExport: boolean;
  canUpdate: boolean;
  controlId: string;
  evidence: ControlEvidence;
  onOpenChange: (open: boolean) => void;
  onSupersede: (evidence: ControlEvidence) => void;
  open: boolean;
}

function latestIntegrityEvent(events: EvidenceCustodyEvent[]): EvidenceCustodyEvent | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    if (['integrity_verified', 'integrity_failed'].includes(events[index].event_type)) {
      return events[index];
    }
  }
  return undefined;
}

function LifecycleBadge({ evidence }: { evidence: ControlEvidence }) {
  const status = evidence.lifecycle_status;
  return (
    <Badge
      variant="outline"
      className={cn(
        'gap-1',
        status === 'active' && 'border-emerald-300 text-emerald-700 dark:text-emerald-300',
        status === 'expired' && 'border-red-300 text-red-700 dark:text-red-300',
        status === 'superseded' && 'text-muted-foreground',
      )}
    >
      {status === 'active' ? (
        <CheckCircle2 aria-hidden="true" className="h-3.5 w-3.5" />
      ) : status === 'expired' ? (
        <AlertTriangle aria-hidden="true" className="h-3.5 w-3.5" />
      ) : (
        <History aria-hidden="true" className="h-3.5 w-3.5" />
      )}
      {evidenceStatusLabel(status)}
    </Badge>
  );
}

export function EvidenceLifecycleDialog({
  canExport,
  canUpdate,
  controlId,
  evidence,
  onOpenChange,
  onSupersede,
  open,
}: EvidenceLifecycleDialogProps) {
  const evidenceId = evidence.id;
  const history = useEvidenceHistory(controlId, evidenceId, {
    enabled: open,
    refetchOnMount: 'always',
  });
  const verify = useVerifyEvidenceIntegrity(controlId);
  const [actionError, setActionError] = React.useState('');
  const [announcement, setAnnouncement] = React.useState('');
  const [verification, setVerification] = React.useState<EvidenceIntegrityResult | null>(null);
  const errorRef = React.useRef<HTMLParagraphElement>(null);

  React.useEffect(() => {
    if (actionError) errorRef.current?.focus();
  }, [actionError]);

  const verifyIntegrity = async () => {
    setActionError('');
    setVerification(null);
    try {
      const result = await verify.mutateAsync({ evidenceId: evidence.id });
      setVerification(result);
      setAnnouncement(`Integrity verification passed for ${evidence.title}.`);
      await history.refetch();
    } catch (caught) {
      setActionError(formatEvidenceError(caught, 'integrity'));
      setAnnouncement('Integrity verification did not pass.');
      await history.refetch();
    }
  };

  const record = history.data;
  const latestVerification = record ? latestIntegrityEvent(record.custody_events) : undefined;
  const canSupersede = Boolean(
    canUpdate &&
      record?.evidence.is_current &&
      record.evidence.lifecycle_status === 'active',
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[94vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Evidence history and integrity</DialogTitle>
          <DialogDescription>
            Immutable versions, reviewer decisions, and custody events for “
            {evidence.title}”.
          </DialogDescription>
        </DialogHeader>

        <p className="sr-only" aria-live="polite">
          {announcement}
        </p>

        {history.isLoading && (
          <div role="status" aria-label="Loading evidence history" className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-3">
              <Skeleton className="h-24" />
              <Skeleton className="h-24" />
              <Skeleton className="h-24" />
            </div>
            <Skeleton className="h-72" />
          </div>
        )}

        {history.isError && (
          <div className="rounded-md border p-6 text-center">
            <ShieldAlert aria-hidden="true" className="mx-auto h-9 w-9 text-destructive" />
            <p role="alert" className="mt-3 text-sm">
              {formatEvidenceError(history.error, 'history')}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="mt-4"
              onClick={() => void history.refetch()}
            >
              <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
              Retry history
            </Button>
          </div>
        )}

        {record && (
          <div className="space-y-5">
            <section
              aria-label="Evidence lifecycle posture"
              className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"
            >
              <div className="rounded-md border p-4">
                <p className="text-xs font-medium text-muted-foreground">Selected version</p>
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <span className="font-semibold">Version {record.evidence.version_number}</span>
                  <LifecycleBadge evidence={record.evidence} />
                </div>
              </div>
              <div className="rounded-md border p-4">
                <p className="text-xs font-medium text-muted-foreground">Custody chain</p>
                <p
                  className={cn(
                    'mt-2 flex items-center gap-2 font-semibold',
                    record.chain.valid ? 'text-emerald-700 dark:text-emerald-300' : 'text-destructive',
                  )}
                >
                  {record.chain.valid ? (
                    <ShieldCheck aria-hidden="true" className="h-4 w-4" />
                  ) : (
                    <ShieldAlert aria-hidden="true" className="h-4 w-4" />
                  )}
                  {record.chain.valid ? 'Chain verified' : 'Chain verification failed'}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {record.chain.event_count} immutable events
                </p>
              </div>
              <div className="rounded-md border p-4">
                <p className="text-xs font-medium text-muted-foreground">Legal hold</p>
                <p className="mt-2 flex items-center gap-2 font-semibold">
                  {record.legal_hold_active ? (
                    <LockKeyhole aria-hidden="true" className="h-4 w-4 text-amber-600" />
                  ) : (
                    <CheckCircle2 aria-hidden="true" className="h-4 w-4 text-emerald-600" />
                  )}
                  {record.legal_hold_active ? 'Active hold' : 'No active hold'}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {record.legal_hold_active
                    ? 'This version must remain preserved.'
                    : 'Standard lifecycle policy applies.'}
                </p>
              </div>
              <div className="rounded-md border p-4">
                <p className="text-xs font-medium text-muted-foreground">Stored object</p>
                <p
                  className={cn(
                    'mt-2 flex items-center gap-2 font-semibold',
                    latestVerification?.event_type === 'integrity_failed' && 'text-destructive',
                    latestVerification?.event_type === 'integrity_verified' &&
                      'text-emerald-700 dark:text-emerald-300',
                  )}
                >
                  {latestVerification?.event_type === 'integrity_failed' ? (
                    <ShieldAlert aria-hidden="true" className="h-4 w-4" />
                  ) : latestVerification?.event_type === 'integrity_verified' ? (
                    <ShieldCheck aria-hidden="true" className="h-4 w-4" />
                  ) : (
                    <Fingerprint aria-hidden="true" className="h-4 w-4 text-muted-foreground" />
                  )}
                  {latestVerification?.event_type === 'integrity_failed'
                    ? 'Mismatch detected'
                    : latestVerification?.event_type === 'integrity_verified'
                      ? 'Object verified'
                      : 'Not manually verified'}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {latestVerification
                    ? formatDateTime(latestVerification.created_at)
                    : 'Run a byte-level checksum and size check.'}
                </p>
              </div>
            </section>

            {!record.chain.valid && (
              <div
                role="alert"
                className="rounded-md border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive"
              >
                The custody chain failed at sequence {record.chain.first_invalid_sequence ?? 'unknown'}.
                Escalate before relying on or exporting this evidence.
              </div>
            )}

            {actionError && (
              <p
                ref={errorRef}
                role="alert"
                tabIndex={-1}
                className="rounded-md bg-destructive/10 p-3 text-sm text-destructive"
              >
                {actionError}
              </p>
            )}
            {verification && (
              <div
                role="status"
                className="rounded-md border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-200"
              >
                Stored bytes matched the immutable fingerprint and size at{' '}
                {formatDateTime(verification.verified_at)}.
              </div>
            )}

            <div className="flex flex-wrap gap-2">
              {canUpdate && (
                <Button
                  type="button"
                  variant="outline"
                  disabled={verify.isPending}
                  onClick={() => void verifyIntegrity()}
                >
                  {verify.isPending ? (
                    <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Fingerprint aria-hidden="true" className="mr-2 h-4 w-4" />
                  )}
                  {verify.isPending ? 'Verifying stored file…' : 'Verify stored file'}
                </Button>
              )}
              {canSupersede && (
                <Button
                  type="button"
                  onClick={() => {
                    onOpenChange(false);
                    onSupersede(record.evidence);
                  }}
                >
                  <Replace aria-hidden="true" className="mr-2 h-4 w-4" />
                  Upload replacement version
                </Button>
              )}
            </div>
            {!canUpdate && (
              <p className="text-sm text-muted-foreground">
                Integrity verification and version replacement require controls:update permission.
              </p>
            )}
            {canUpdate && !canSupersede && (
              <p className="text-sm text-muted-foreground">
                Only the active current version can be superseded. Historical versions remain immutable.
              </p>
            )}

            <Tabs defaultValue="versions">
              <TabsList className="grid h-auto w-full grid-cols-3">
                <TabsTrigger value="versions">Versions ({record.versions.length})</TabsTrigger>
                <TabsTrigger value="reviews">Reviews ({record.reviews.length})</TabsTrigger>
                <TabsTrigger value="custody">Custody ({record.custody_events.length})</TabsTrigger>
              </TabsList>

              <TabsContent value="versions" className="space-y-3 pt-3">
                {record.versions.length === 0 ? (
                  <EmptyTimeline icon={FileClock} message="No version records were returned." />
                ) : (
                  record.versions.map((version) => (
                    <article key={version.id} className="rounded-md border p-4">
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div>
                          <div className="flex flex-wrap items-center gap-2">
                            <h3 className="font-semibold">Version {version.version_number}</h3>
                            <LifecycleBadge evidence={version} />
                            {version.id === record.evidence.id && <Badge>Selected</Badge>}
                          </div>
                          <p className="mt-2 whitespace-pre-wrap text-sm">{version.version_reason}</p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            Collected {formatDateTime(version.collected_at)} · valid until{' '}
                            {formatDate(version.expires_at ?? version.valid_until)}
                          </p>
                        </div>
                        {canExport && version.file_name && (
                          <Button asChild size="sm" variant="outline">
                            <a
                              href={evidenceDownloadHref(controlId, version.id)}
                              download={safeEvidenceFilename(version.file_name)}
                              referrerPolicy="no-referrer"
                              aria-label={`Download version ${version.version_number} of ${version.title}`}
                            >
                              <Download aria-hidden="true" className="mr-2 h-4 w-4" />
                              Download
                            </a>
                          </Button>
                        )}
                      </div>
                    </article>
                  ))
                )}
              </TabsContent>

              <TabsContent value="reviews" className="space-y-3 pt-3">
                {record.reviews.length === 0 ? (
                  <EmptyTimeline icon={UserRound} message="No immutable review decisions yet." />
                ) : (
                  record.reviews.map((review) => (
                    <article key={review.id} className="rounded-md border p-4">
                      <div className="flex flex-wrap items-center gap-2">
                        {review.decision === 'accepted' ? (
                          <CheckCircle2 aria-hidden="true" className="h-4 w-4 text-emerald-600" />
                        ) : (
                          <AlertTriangle aria-hidden="true" className="h-4 w-4 text-destructive" />
                        )}
                        <h3 className="font-semibold">{evidenceStatusLabel(review.decision)}</h3>
                        <span className="text-xs text-muted-foreground">
                          {formatDateTime(review.created_at)}
                        </span>
                      </div>
                      {review.comment && (
                        <p className="mt-2 whitespace-pre-wrap text-sm">{review.comment}</p>
                      )}
                      <p className="mt-2 break-all text-xs text-muted-foreground">
                        Reviewer identity: {review.reviewer_id}
                      </p>
                    </article>
                  ))
                )}
              </TabsContent>

              <TabsContent value="custody" className="space-y-3 pt-3">
                {record.custody_events.length === 0 ? (
                  <EmptyTimeline icon={History} message="No custody events were returned." />
                ) : (
                  <ol className="space-y-3">
                    {record.custody_events.map((event) => (
                      <li key={event.id} className="relative rounded-md border p-4 pl-12">
                        <span className="absolute left-4 top-4 flex h-6 w-6 items-center justify-center rounded-full bg-muted text-xs font-semibold">
                          {event.sequence}
                        </span>
                        <div className="flex flex-wrap items-center gap-2">
                          {event.event_type === 'integrity_failed' ? (
                            <ShieldAlert aria-hidden="true" className="h-4 w-4 text-destructive" />
                          ) : (
                            <Clock3 aria-hidden="true" className="h-4 w-4 text-muted-foreground" />
                          )}
                          <h3 className="font-semibold">
                            {evidenceStatusLabel(event.event_type)}
                          </h3>
                          <span className="text-xs text-muted-foreground">
                            {formatDateTime(event.created_at)}
                          </span>
                        </div>
                        <p className="mt-1 text-sm">{event.reason}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          Actor: {event.actor_type}
                          {event.actor_user_id ? ` · ${event.actor_user_id}` : ''}
                        </p>
                      </li>
                    ))}
                  </ol>
                )}
                {latestVerification && (
                  <p className="text-xs text-muted-foreground">
                    Latest object check: {evidenceStatusLabel(latestVerification.event_type)} at{' '}
                    {formatDateTime(latestVerification.created_at)}.
                  </p>
                )}
              </TabsContent>
            </Tabs>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function EmptyTimeline({
  icon: Icon,
  message,
}: {
  icon: typeof History;
  message: string;
}) {
  return (
    <div className="rounded-md border border-dashed py-8 text-center text-sm text-muted-foreground">
      <Icon aria-hidden="true" className="mx-auto mb-2 h-7 w-7" />
      {message}
    </div>
  );
}
