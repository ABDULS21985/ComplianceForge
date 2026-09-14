'use client';

import * as React from 'react';
import {
  AlertTriangle,
  ArrowLeft,
  CalendarDays,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  FileText,
  Flag,
  LockKeyhole,
  Pencil,
  Play,
  Plus,
  Trash2,
  UserRound,
  XCircle,
} from 'lucide-react';
import type {
  Audit,
  AuditFinding,
  AuditFindingStats,
  AuditLifecycleAction,
  FindingStatus,
} from '@/types/audit';
import {
  auditLifecycleActions,
  auditPersonName,
  findingNextStatuses,
  formatAuditError,
  humanizeAuditToken,
  isFindingOverdue,
} from '@/lib/audit';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn, formatDate, formatDateTime, getRiskLevelColor, getStatusColor } from '@/lib/utils';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  useAudit,
  useAuditFindings,
  useAuditFindingsStats,
  useAuditTransition,
  useDeleteAudit,
  useDeleteAuditFinding,
} from '@/lib/api-hooks';
import { useParams, useRouter } from 'next/navigation';
import { AuditConfirmationDialog } from '@/components/audits/audit-confirmation-dialog';
import { AuditEditorDialog } from '@/components/audits/audit-editor-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FindingEditorDialog } from '@/components/audits/finding-editor-dialog';
import { FindingTransitionDialog } from '@/components/audits/finding-transition-dialog';
import Link from 'next/link';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { useAuditPermissions } from '@/hooks/use-audit-permissions';

export default function AuditDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const access = useAuditPermissions();
  const auditQuery = useAudit(id, { enabled: access.canRead && Boolean(id) });
  const [findingsPage, setFindingsPage] = React.useState(1);
  const findingsQuery = useAuditFindings(
    id,
    { page: findingsPage, page_size: 20 },
    { enabled: access.canRead && Boolean(id) },
  );
  const statsQuery = useAuditFindingsStats(id, {
    enabled: access.canRead && Boolean(id),
  });
  const transitionAudit = useAuditTransition(id);
  const deleteAudit = useDeleteAudit();
  const deleteFinding = useDeleteAuditFinding(id);

  const [editAuditOpen, setEditAuditOpen] = React.useState(false);
  const [createFindingOpen, setCreateFindingOpen] = React.useState(false);
  const [viewingFinding, setViewingFinding] = React.useState<AuditFinding | null>(null);
  const [editingFinding, setEditingFinding] = React.useState<AuditFinding | null>(null);
  const [deletingFinding, setDeletingFinding] = React.useState<AuditFinding | null>(null);
  const [transitioningFinding, setTransitioningFinding] = React.useState<{
    finding: AuditFinding;
    status: FindingStatus;
  } | null>(null);
  const [lifecycleAction, setLifecycleAction] = React.useState<AuditLifecycleAction | null>(null);
  const [deleteAuditOpen, setDeleteAuditOpen] = React.useState(false);

  const findingsData = findingsQuery.data;
  const findings = findingsData?.items ?? [];
  const findingsTotalPages = Math.max(findingsData?.total_pages ?? 0, 1);

  React.useEffect(() => {
    if (!findingsData || findingsPage <= findingsTotalPages) return;
    const timer = window.setTimeout(() => setFindingsPage(findingsTotalPages), 0);
    return () => window.clearTimeout(timer);
  }, [findingsData, findingsPage, findingsTotalPages]);

  if (access.isLoading || !access.user)
    return <AuditDetailSkeleton label="Checking audit permissions" />;
  if (access.isError) {
    return (
      <AccessState
        alert
        title="Audit access could not be verified"
        description="The permissions service is temporarily unavailable. No audit data was loaded."
        action={<Button onClick={() => void access.retry()}>Try again</Button>}
      />
    );
  }
  if (!access.canRead) {
    return (
      <AccessState
        title="Audit unavailable"
        description="Your role does not grant read access to audit records."
      />
    );
  }
  if (auditQuery.isLoading) return <AuditDetailSkeleton label="Loading audit" />;
  if (auditQuery.isError || !auditQuery.data) {
    return (
      <div className="space-y-5">
        <BackLink />
        <Card>
          <CardContent role="alert" className="flex flex-col items-center gap-3 py-12 text-center">
            <AlertTriangle aria-hidden="true" className="h-9 w-9 text-destructive" />
            <h1 className="text-xl font-semibold">Audit could not be loaded</h1>
            <p className="max-w-xl text-sm text-muted-foreground">
              {formatAuditError(
                auditQuery.error,
                'The audit was not found or is temporarily unavailable.',
              )}
            </p>
            <Button variant="outline" onClick={() => void auditQuery.refetch()}>
              Retry
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const audit = auditQuery.data;
  const mutableAudit = !['closed', 'cancelled'].includes(audit.status);
  const mayCreateFinding = access.canCreate && ['planned', 'in_progress'].includes(audit.status);
  const mayDeleteFinding = access.canDelete && ['planned', 'in_progress'].includes(audit.status);
  const mayDeleteAudit = access.canDelete && ['planned', 'cancelled'].includes(audit.status);
  const stats = statsQuery.data;
  const closeBlockers = (stats?.open ?? 0) + (stats?.in_progress ?? 0);
  const lifecycleActions = access.canUpdate ? auditLifecycleActions(audit.status) : [];

  async function confirmAuditDeletion() {
    await deleteAudit.mutateAsync(audit.id);
    router.push('/audits');
    router.refresh();
  }

  return (
    <div className="space-y-6">
      <BackLink />

      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-sm text-muted-foreground">{audit.audit_ref}</span>
            <Badge className={getStatusColor(audit.status)}>
              {humanizeAuditToken(audit.status)}
            </Badge>
            <Badge variant="outline">{humanizeAuditToken(audit.audit_type)}</Badge>
          </div>
          <h1 className="break-words text-3xl font-bold tracking-tight">{audit.title}</h1>
          <p className="text-sm text-muted-foreground">
            Last updated {formatDateTime(audit.updated_at)}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {access.canUpdate && mutableAudit && (
            <Button variant="outline" onClick={() => setEditAuditOpen(true)}>
              <Pencil aria-hidden="true" className="mr-2 h-4 w-4" />
              Edit plan
            </Button>
          )}
          {lifecycleActions.map((action) => {
            const closeUnavailable =
              action === 'close' &&
              (statsQuery.isLoading || statsQuery.isError || closeBlockers > 0);
            return (
              <Button
                key={action}
                variant={action === 'cancel' ? 'destructive' : 'default'}
                disabled={closeUnavailable}
                title={
                  closeUnavailable ? closeReason(statsQuery.isError, closeBlockers) : undefined
                }
                onClick={() => setLifecycleAction(action)}
              >
                {lifecycleIcon(action)}
                {humanizeAuditToken(action)} audit
              </Button>
            );
          })}
          {mayDeleteAudit && (
            <Button variant="destructive" onClick={() => setDeleteAuditOpen(true)}>
              <Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />
              Delete
            </Button>
          )}
        </div>
      </div>

      {audit.status === 'completed' && (statsQuery.isError || closeBlockers > 0) && (
        <div
          role={statsQuery.isError ? 'alert' : 'status'}
          className="flex gap-3 rounded-md border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200"
        >
          <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
          {statsQuery.isError
            ? 'Finding status could not be verified, so closure is unavailable.'
            : `${closeBlockers} open or in-progress finding${closeBlockers === 1 ? '' : 's'} must be resolved or risk-accepted before closure.`}
        </div>
      )}

      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Engagement brief</CardTitle>
          </CardHeader>
          <CardContent className="space-y-5">
            <Detail label="Description" value={audit.description} />
            <Detail label="Scope" value={audit.scope} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Schedule and ownership</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="space-y-4">
              <IconDetail
                icon={<CalendarDays aria-hidden="true" className="h-4 w-4" />}
                label="Scheduled period"
                value={`${formatDate(audit.scheduled_start_date)} – ${formatDate(audit.scheduled_end_date)}`}
              />
              <IconDetail
                icon={<CalendarDays aria-hidden="true" className="h-4 w-4" />}
                label="Actual period"
                value={
                  audit.actual_start_date
                    ? `${formatDate(audit.actual_start_date)} – ${audit.actual_end_date ? formatDate(audit.actual_end_date) : 'Ongoing'}`
                    : 'Not started'
                }
              />
              <IconDetail
                icon={<UserRound aria-hidden="true" className="h-4 w-4" />}
                label="Lead auditor"
                value={auditPersonName(audit.lead_auditor, audit.lead_auditor_id)}
              />
              <IconDetail
                icon={<FileText aria-hidden="true" className="h-4 w-4" />}
                label="Framework"
                value={
                  audit.framework
                    ? `${audit.framework.code} — ${audit.framework.name}`
                    : 'No framework linked'
                }
              />
            </dl>
          </CardContent>
        </Card>
      </div>

      <Separator />

      <section aria-labelledby="finding-heading" className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 id="finding-heading" className="text-2xl font-semibold">
              Findings
            </h2>
            <p className="text-sm text-muted-foreground">
              Track observations through remediation, acceptance, and closure.
            </p>
          </div>
          {mayCreateFinding && (
            <Button onClick={() => setCreateFindingOpen(true)}>
              <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
              Add finding
            </Button>
          )}
        </div>

        <FindingStatsCards
          queryLoading={statsQuery.isLoading}
          queryError={statsQuery.isError}
          stats={stats}
          onRetry={() => void statsQuery.refetch()}
        />

        <Card>
          <CardContent className="p-0">
            {findingsQuery.isLoading ? (
              <div role="status" aria-label="Loading findings" className="space-y-3 p-6">
                {Array.from({ length: 4 }, (_, index) => (
                  <Skeleton key={index} className="h-14 w-full" />
                ))}
              </div>
            ) : findingsQuery.isError ? (
              <div role="alert" className="flex flex-col items-center gap-3 p-10 text-center">
                <AlertTriangle aria-hidden="true" className="h-8 w-8 text-destructive" />
                <p className="font-semibold">Findings could not be loaded</p>
                <p className="text-sm text-muted-foreground">Retry without leaving this audit.</p>
                <Button variant="outline" onClick={() => void findingsQuery.refetch()}>
                  Retry
                </Button>
              </div>
            ) : findings.length === 0 ? (
              <div className="flex flex-col items-center gap-3 p-12 text-center">
                <ClipboardCheck aria-hidden="true" className="h-10 w-10 text-muted-foreground" />
                <p className="text-lg font-semibold">No findings recorded</p>
                <p className="text-sm text-muted-foreground">
                  {mayCreateFinding
                    ? 'Record the first observation for this engagement.'
                    : 'This audit has no findings.'}
                </p>
                {mayCreateFinding && (
                  <Button onClick={() => setCreateFindingOpen(true)}>Add first finding</Button>
                )}
              </div>
            ) : (
              <>
                <div className="hidden overflow-x-auto lg:block">
                  <table className="w-full text-sm">
                    <caption className="sr-only">Findings for {audit.audit_ref}</caption>
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th scope="col" className="px-4 py-3 text-left font-medium">
                          Finding
                        </th>
                        <th scope="col" className="px-4 py-3 text-left font-medium">
                          Severity
                        </th>
                        <th scope="col" className="px-4 py-3 text-left font-medium">
                          Status
                        </th>
                        <th scope="col" className="px-4 py-3 text-left font-medium">
                          Responsible
                        </th>
                        <th scope="col" className="px-4 py-3 text-left font-medium">
                          Due
                        </th>
                        <th scope="col" className="px-4 py-3 text-right font-medium">
                          Actions
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {findings.map((finding) => (
                        <FindingTableRow
                          key={finding.id}
                          finding={finding}
                          audit={audit}
                          canUpdate={access.canUpdate}
                          canDelete={mayDeleteFinding}
                          onView={setViewingFinding}
                          onEdit={setEditingFinding}
                          onDelete={setDeletingFinding}
                          onTransition={(status) => setTransitioningFinding({ finding, status })}
                        />
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="divide-y lg:hidden">
                  {findings.map((finding) => (
                    <FindingCard
                      key={finding.id}
                      finding={finding}
                      audit={audit}
                      canUpdate={access.canUpdate}
                      canDelete={mayDeleteFinding}
                      onView={setViewingFinding}
                      onEdit={setEditingFinding}
                      onDelete={setDeletingFinding}
                      onTransition={(status) => setTransitioningFinding({ finding, status })}
                    />
                  ))}
                </div>
              </>
            )}

            {findingsData && findingsData.total > 0 && (
              <div className="flex flex-col gap-3 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <p aria-live="polite" className="text-sm text-muted-foreground">
                  Page {findingsPage} of {findingsTotalPages} · {findingsData.total} finding
                  {findingsData.total === 1 ? '' : 's'}
                </p>
                <div className="flex gap-2">
                  <Button
                    size="icon"
                    variant="outline"
                    aria-label="Previous findings page"
                    disabled={findingsPage <= 1 || findingsQuery.isFetching}
                    onClick={() => setFindingsPage((value) => value - 1)}
                  >
                    <ChevronLeft aria-hidden="true" className="h-4 w-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="outline"
                    aria-label="Next findings page"
                    disabled={findingsPage >= findingsTotalPages || findingsQuery.isFetching}
                    onClick={() => setFindingsPage((value) => value + 1)}
                  >
                    <ChevronRight aria-hidden="true" className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </section>

      {access.canUpdate && mutableAudit && (
        <AuditEditorDialog
          audit={audit}
          open={editAuditOpen}
          onOpenChange={setEditAuditOpen}
          defaultLeadAuditorId={access.user.id}
        />
      )}
      {mayCreateFinding && (
        <FindingEditorDialog
          auditId={id}
          open={createFindingOpen}
          onOpenChange={setCreateFindingOpen}
          defaultResponsibleUserId={access.user.id}
        />
      )}
      {access.canUpdate && editingFinding && (
        <FindingEditorDialog
          auditId={id}
          finding={editingFinding}
          open={Boolean(editingFinding)}
          onOpenChange={(open) => !open && setEditingFinding(null)}
          defaultResponsibleUserId={access.user.id}
        />
      )}
      <FindingDetailDialog
        finding={viewingFinding}
        open={Boolean(viewingFinding)}
        onOpenChange={(open) => !open && setViewingFinding(null)}
      />
      {access.canUpdate && transitioningFinding && (
        <FindingTransitionDialog
          auditId={id}
          finding={transitioningFinding.finding}
          targetStatus={transitioningFinding.status}
          open={Boolean(transitioningFinding)}
          onOpenChange={(open) => !open && setTransitioningFinding(null)}
        />
      )}
      {access.canDelete && deletingFinding && (
        <AuditConfirmationDialog
          open={Boolean(deletingFinding)}
          onOpenChange={(open) => !open && setDeletingFinding(null)}
          title="Delete finding?"
          description={
            <p>
              This removes the finding from active audit records. Findings must be retained once the
              audit is completed.
            </p>
          }
          confirmLabel="Delete finding"
          confirmationText={deletingFinding.finding_ref}
          destructive
          onConfirm={() => deleteFinding.mutateAsync(deletingFinding.id)}
        />
      )}
      {access.canUpdate && lifecycleAction && (
        <AuditConfirmationDialog
          open={Boolean(lifecycleAction)}
          onOpenChange={(open) => !open && setLifecycleAction(null)}
          title={lifecycleTitle(lifecycleAction)}
          description={<p>{lifecycleDescription(lifecycleAction)}</p>}
          confirmLabel={`${humanizeAuditToken(lifecycleAction)} audit`}
          destructive={lifecycleAction === 'cancel'}
          onConfirm={() => transitionAudit.mutateAsync(lifecycleAction)}
        />
      )}
      {mayDeleteAudit && (
        <AuditConfirmationDialog
          open={deleteAuditOpen}
          onOpenChange={setDeleteAuditOpen}
          title="Delete audit?"
          description={
            <p>This removes the audit plan from active records. Type the reference to confirm.</p>
          }
          confirmLabel="Delete audit"
          confirmationText={audit.audit_ref}
          destructive
          onConfirm={confirmAuditDeletion}
        />
      )}
    </div>
  );
}

interface FindingActionsProps {
  audit: Audit;
  finding: AuditFinding;
  canUpdate: boolean;
  canDelete: boolean;
  onView: (finding: AuditFinding) => void;
  onEdit: (finding: AuditFinding) => void;
  onDelete: (finding: AuditFinding) => void;
  onTransition: (status: FindingStatus) => void;
}

function FindingActions({
  audit,
  finding,
  canUpdate,
  canDelete,
  onView,
  onEdit,
  onDelete,
  onTransition,
}: FindingActionsProps) {
  const mutable = !['closed', 'cancelled'].includes(audit.status);
  const transitions = mutable && canUpdate ? findingNextStatuses(finding.status) : [];
  return (
    <div className="flex flex-wrap justify-end gap-1">
      <Button size="sm" variant="outline" onClick={() => onView(finding)}>
        View
      </Button>
      {mutable && canUpdate && (
        <Button
          size="icon"
          variant="ghost"
          aria-label={`Edit ${finding.finding_ref}`}
          onClick={() => onEdit(finding)}
        >
          <Pencil aria-hidden="true" className="h-4 w-4" />
        </Button>
      )}
      {transitions.length > 0 && (
        <Select value="" onValueChange={(value) => onTransition(value as FindingStatus)}>
          <SelectTrigger
            className="h-9 w-[9.5rem]"
            aria-label={`Change status for ${finding.finding_ref}`}
          >
            <SelectValue placeholder="Change status" />
          </SelectTrigger>
          <SelectContent>
            {transitions.map((status) => (
              <SelectItem key={status} value={status}>
                {humanizeAuditToken(status)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {canDelete && (
        <Button
          size="icon"
          variant="ghost"
          className="text-destructive hover:text-destructive"
          aria-label={`Delete ${finding.finding_ref}`}
          onClick={() => onDelete(finding)}
        >
          <Trash2 aria-hidden="true" className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}

function FindingTableRow(props: FindingActionsProps) {
  const { finding } = props;
  const overdue = isFindingOverdue(finding);
  return (
    <tr
      className={cn(
        'border-b last:border-0 hover:bg-muted/30',
        overdue && 'bg-red-50/60 dark:bg-red-950/10',
      )}
    >
      <td className="px-4 py-3">
        <button
          type="button"
          className="text-left font-medium text-primary hover:underline"
          onClick={() => props.onView(finding)}
        >
          {finding.title}
        </button>
        <div className="font-mono text-xs text-muted-foreground">
          {finding.finding_ref} · {finding.finding_type}
        </div>
      </td>
      <td className="px-4 py-3">
        <Badge className={getRiskLevelColor(finding.severity)}>
          {humanizeAuditToken(finding.severity)}
        </Badge>
      </td>
      <td className="px-4 py-3">
        <Badge className={getStatusColor(finding.status)}>
          {humanizeAuditToken(finding.status)}
        </Badge>
      </td>
      <td className="px-4 py-3">
        {auditPersonName(finding.responsible_user, finding.responsible_user_id)}
      </td>
      <td
        className={cn('whitespace-nowrap px-4 py-3', overdue && 'font-semibold text-destructive')}
      >
        {formatDate(finding.due_date)}
        {overdue && <span className="ml-1 text-xs">Overdue</span>}
      </td>
      <td className="px-4 py-3">
        <FindingActions {...props} />
      </td>
    </tr>
  );
}

function FindingCard(props: FindingActionsProps) {
  const { finding } = props;
  const overdue = isFindingOverdue(finding);
  return (
    <article className={cn('space-y-3 p-4', overdue && 'bg-red-50/60 dark:bg-red-950/10')}>
      <div className="flex flex-wrap gap-2">
        <Badge className={getRiskLevelColor(finding.severity)}>
          {humanizeAuditToken(finding.severity)}
        </Badge>
        <Badge className={getStatusColor(finding.status)}>
          {humanizeAuditToken(finding.status)}
        </Badge>
        {overdue && <Badge variant="destructive">Overdue</Badge>}
      </div>
      <div>
        <button
          type="button"
          className="text-left font-semibold text-primary hover:underline"
          onClick={() => props.onView(finding)}
        >
          {finding.title}
        </button>
        <p className="font-mono text-xs text-muted-foreground">{finding.finding_ref}</p>
      </div>
      <dl className="grid grid-cols-2 gap-3 text-sm">
        <div>
          <dt className="text-muted-foreground">Responsible</dt>
          <dd>{auditPersonName(finding.responsible_user, finding.responsible_user_id)}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Due</dt>
          <dd className={cn(overdue && 'font-semibold text-destructive')}>
            {formatDate(finding.due_date)}
          </dd>
        </div>
      </dl>
      <FindingActions {...props} />
    </article>
  );
}

function FindingStatsCards({
  stats,
  queryLoading,
  queryError,
  onRetry,
}: {
  stats?: AuditFindingStats;
  queryLoading: boolean;
  queryError: boolean;
  onRetry: () => void;
}) {
  if (queryLoading)
    return (
      <div
        role="status"
        aria-label="Loading finding statistics"
        className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5"
      >
        {Array.from({ length: 5 }, (_, index) => (
          <Skeleton key={index} className="h-24" />
        ))}
      </div>
    );
  if (queryError || !stats)
    return (
      <div
        role="alert"
        className="flex items-center justify-between gap-3 rounded-md border border-destructive/30 p-4 text-sm"
      >
        <span>Finding statistics are unavailable.</span>
        <Button size="sm" variant="outline" onClick={onRetry}>
          Retry
        </Button>
      </div>
    );
  const cards = [
    ['Total', stats.total, false],
    ['Open / in progress', stats.open + stats.in_progress, stats.open + stats.in_progress > 0],
    ['Resolved / closed', stats.resolved + stats.closed, false],
    ['Risk accepted', stats.accepted, false],
    ['Overdue', stats.overdue, stats.overdue > 0],
  ] as const;
  return (
    <div
      role="group"
      className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5"
      aria-label="Finding statistics"
    >
      {cards.map(([label, value, alert]) => (
        <Card key={label} className={cn(alert && 'border-destructive/40')}>
          <CardContent className="p-4">
            <p className="text-sm text-muted-foreground">{label}</p>
            <p className={cn('mt-1 text-2xl font-bold', alert && 'text-destructive')}>{value}</p>
            {label === 'Open / in progress' && (
              <p className="text-xs text-muted-foreground">
                {stats.critical_open} critical · {stats.high_open} high
              </p>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function FindingDetailDialog({
  finding,
  open,
  onOpenChange,
}: {
  finding: AuditFinding | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  if (!finding) return null;
  const overdue = isFindingOverdue(finding);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{finding.title}</DialogTitle>
          <DialogDescription>
            {finding.finding_ref} · created {formatDateTime(finding.created_at)}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap gap-2">
          <Badge className={getRiskLevelColor(finding.severity)}>
            {humanizeAuditToken(finding.severity)}
          </Badge>
          <Badge className={getStatusColor(finding.status)}>
            {humanizeAuditToken(finding.status)}
          </Badge>
          {overdue && <Badge variant="destructive">Overdue</Badge>}
        </div>
        <dl className="grid gap-4 sm:grid-cols-2">
          <Detail label="Finding type" value={finding.finding_type} />
          <Detail label="Due date" value={formatDate(finding.due_date)} />
          <Detail
            label="Responsible"
            value={auditPersonName(finding.responsible_user, finding.responsible_user_id)}
          />
          <Detail label="Control UUID" value={finding.control_id ?? 'Not linked'} />
        </dl>
        <Separator />
        <Detail label="Description" value={finding.description} />
        <Detail label="Root cause" value={finding.root_cause || 'Not documented'} />
        <Detail label="Recommendation" value={finding.recommendation} />
        <Detail label="Remediation plan" value={finding.remediation_plan || 'Not documented'} />
        {finding.accepted_risk_reason && (
          <Detail label="Risk acceptance rationale" value={finding.accepted_risk_reason} />
        )}
        {finding.resolved_at && (
          <Detail label="Resolved at" value={formatDateTime(finding.resolved_at)} />
        )}
      </DialogContent>
    </Dialog>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-sm font-medium text-muted-foreground">{label}</p>
      <p className="mt-1 whitespace-pre-wrap break-words text-sm">{value}</p>
    </div>
  );
}

function IconDetail({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="flex gap-3">
      <span className="mt-0.5 text-muted-foreground">{icon}</span>
      <div>
        <dt className="text-xs text-muted-foreground">{label}</dt>
        <dd className="text-sm font-medium">{value}</dd>
      </div>
    </div>
  );
}

function BackLink() {
  return (
    <Link
      href="/audits"
      className="inline-flex items-center rounded-sm text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <ArrowLeft aria-hidden="true" className="mr-1 h-4 w-4" />
      Back to audits
    </Link>
  );
}

function AuditDetailSkeleton({ label }: { label: string }) {
  return (
    <div role="status" aria-label={label} className="space-y-6">
      <Skeleton className="h-5 w-28" />
      <Skeleton className="h-24 w-full" />
      <div className="grid gap-5 lg:grid-cols-2">
        <Skeleton className="h-64" />
        <Skeleton className="h-64" />
      </div>
      <Skeleton className="h-80 w-full" />
    </div>
  );
}

function AccessState({
  title,
  description,
  action,
  alert = false,
}: {
  title: string;
  description: string;
  action?: React.ReactNode;
  alert?: boolean;
}) {
  return (
    <Card className="mx-auto max-w-xl">
      <CardContent
        role={alert ? 'alert' : undefined}
        className="flex flex-col items-center gap-3 py-12 text-center"
      >
        <LockKeyhole aria-hidden="true" className="h-9 w-9 text-muted-foreground" />
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="text-sm text-muted-foreground">{description}</p>
        {action}
      </CardContent>
    </Card>
  );
}

function lifecycleIcon(action: AuditLifecycleAction) {
  const className = 'mr-2 h-4 w-4';
  if (action === 'start') return <Play aria-hidden="true" className={className} />;
  if (action === 'complete') return <CheckCircle2 aria-hidden="true" className={className} />;
  if (action === 'close') return <Flag aria-hidden="true" className={className} />;
  return <XCircle aria-hidden="true" className={className} />;
}

function lifecycleTitle(action: AuditLifecycleAction): string {
  if (action === 'start') return 'Start audit execution?';
  if (action === 'complete') return 'Complete audit execution?';
  if (action === 'close') return 'Close audit?';
  return 'Cancel audit plan?';
}

function lifecycleDescription(action: AuditLifecycleAction): string {
  if (action === 'start')
    return 'The audit moves to In Progress and today becomes its actual start date.';
  if (action === 'complete')
    return 'The audit moves to Completed and today becomes its actual end date.';
  if (action === 'close')
    return 'Closure makes the audit and its findings immutable. All active findings must already be resolved or risk-accepted.';
  return 'Cancellation is only available before execution starts. The cancelled record remains available for retention and reporting.';
}

function closeReason(statsFailed: boolean, blockers: number): string {
  if (statsFailed) return 'Finding statistics are unavailable.';
  if (blockers > 0) return `${blockers} active finding${blockers === 1 ? '' : 's'} block closure.`;
  return 'Finding statistics are still loading.';
}
