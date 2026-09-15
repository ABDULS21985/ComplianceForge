'use client';

import * as React from 'react';
import {
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  FilterX,
  Loader2,
  Pencil,
  Plus,
  Search,
  Trash2,
} from 'lucide-react';
import type { Audit, AuditStatus, AuditType } from '@/types/audit';
import { auditPersonName, humanizeAuditToken } from '@/lib/audit';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn, formatDate, getStatusColor } from '@/lib/utils';
import { ResourceBoundary, ResourceState, StaleDataNotice } from '@/components/data/resource-state';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useAudits, useDeleteAudit } from '@/lib/api-hooks';
import { AuditConfirmationDialog } from '@/components/audits/audit-confirmation-dialog';
import { AuditEditorDialog } from '@/components/audits/audit-editor-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import Link from 'next/link';
import { useAuditPermissions } from '@/hooks/use-audit-permissions';
import { useQuickCreate } from '@/lib/use-quick-create';

const PAGE_SIZES = [10, 20, 50] as const;

const AUDIT_TYPE_STYLES: Record<AuditType, string> = {
  internal: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
  external: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
  certification: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
};

export default function AuditsPage() {
  const access = useAuditPermissions();
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState<number>(20);
  const [searchDraft, setSearchDraft] = React.useState('');
  const [search, setSearch] = React.useState('');
  const [status, setStatus] = React.useState<AuditStatus | 'all'>('all');
  const [auditType, setAuditType] = React.useState<AuditType | 'all'>('all');
  const [createOpen, setCreateOpen] = useQuickCreate('audit');
  const [editingAudit, setEditingAudit] = React.useState<Audit | null>(null);
  const [deletingAudit, setDeletingAudit] = React.useState<Audit | null>(null);
  const deleteAudit = useDeleteAudit();

  const auditsQuery = useAudits(
    {
      page,
      page_size: pageSize,
      search: search || undefined,
      status: status === 'all' ? undefined : status,
      audit_type: auditType === 'all' ? undefined : auditType,
    },
    { enabled: access.canRead },
  );
  const data = auditsQuery.data;
  const audits = data?.items ?? [];
  const totalPages = Math.max(data?.total_pages ?? 0, 1);

  React.useEffect(() => {
    if (!data || page <= totalPages) return;
    const timer = window.setTimeout(() => setPage(totalPages), 0);
    return () => window.clearTimeout(timer);
  }, [data, page, totalPages]);

  if (access.isLoading || !access.user) {
    return (
      <ResourceState
        headingLevel={1}
        kind="loading"
        loadingLayout="detail"
        title="Checking audit permissions"
      />
    );
  }

  if (access.isError) {
    return (
      <ResourceState
        headingLevel={1}
        kind="error"
        title="Audit access could not be verified"
        description="The permissions service is temporarily unavailable. No audit data was loaded."
        onRetry={() => void access.retry()}
      />
    );
  }

  if (!access.canRead) {
    return (
      <ResourceState
        headingLevel={1}
        kind="forbidden"
        title="Audit management unavailable"
        description="Your role does not grant read access to audits."
      />
    );
  }

  const plannedOnPage = audits.filter((audit) => audit.status === 'planned').length;
  const activeOnPage = audits.filter((audit) => audit.status === 'in_progress').length;
  const findingsOnPage = audits.reduce((total, audit) => total + audit.findings_count, 0);
  const urgentOnPage = audits.reduce(
    (total, audit) => total + audit.critical_findings_open + audit.high_findings_open,
    0,
  );
  const filtersActive = Boolean(search || status !== 'all' || auditType !== 'all');

  function applySearch(event: React.FormEvent) {
    event.preventDefault();
    setSearch(searchDraft.trim());
    setPage(1);
  }

  function resetFilters() {
    setSearchDraft('');
    setSearch('');
    setStatus('all');
    setAuditType('all');
    setPage(1);
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Audit management</h1>
          <p className="mt-1 text-muted-foreground">
            Plan engagements, manage execution, and resolve findings.
          </p>
        </div>
        {access.canCreate && (
          <Button className="w-full sm:w-auto" onClick={() => setCreateOpen(true)}>
            <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
            Plan audit
          </Button>
        )}
      </div>

      {auditsQuery.isError && data && (
        <StaleDataNotice
          isRefreshing={auditsQuery.isFetching}
          lastUpdatedAt={auditsQuery.dataUpdatedAt}
          onRefresh={() => void auditsQuery.refetch()}
        />
      )}

      <div
        role="group"
        className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
        aria-label="Metrics for the current results page"
      >
        <SummaryCard
          label="Planned on page"
          value={plannedOnPage}
          icon={<ClipboardCheck aria-hidden="true" className="h-4 w-4 text-blue-600" />}
        />
        <SummaryCard
          label="In progress on page"
          value={activeOnPage}
          icon={<Loader2 aria-hidden="true" className="h-4 w-4 text-amber-600" />}
        />
        <SummaryCard
          label="Findings on page"
          value={findingsOnPage}
          icon={<AlertTriangle aria-hidden="true" className="h-4 w-4 text-orange-600" />}
        />
        <SummaryCard
          label="Critical/high open"
          value={urgentOnPage}
          highlight={urgentOnPage > 0}
          icon={<AlertTriangle aria-hidden="true" className="h-4 w-4 text-red-600" />}
        />
      </div>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-base">Find audits</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            role="search"
            className="grid gap-3 lg:grid-cols-[minmax(16rem,1fr)_12rem_12rem_auto]"
            onSubmit={applySearch}
          >
            <div className="space-y-2">
              <Label htmlFor="audit-search">Reference or title</Label>
              <div className="relative">
                <Search
                  aria-hidden="true"
                  className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
                />
                <Input
                  id="audit-search"
                  maxLength={200}
                  className="pl-9"
                  value={searchDraft}
                  onChange={(event) => setSearchDraft(event.target.value)}
                  placeholder="Search up to 200 characters"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="audit-status-filter">Status</Label>
              <Select
                value={status}
                onValueChange={(value) => {
                  setStatus(value as AuditStatus | 'all');
                  setPage(1);
                }}
              >
                <SelectTrigger id="audit-status-filter">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All statuses</SelectItem>
                  {(['planned', 'in_progress', 'completed', 'closed', 'cancelled'] as const).map(
                    (value) => (
                      <SelectItem key={value} value={value}>
                        {humanizeAuditToken(value)}
                      </SelectItem>
                    ),
                  )}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="audit-type-filter">Type</Label>
              <Select
                value={auditType}
                onValueChange={(value) => {
                  setAuditType(value as AuditType | 'all');
                  setPage(1);
                }}
              >
                <SelectTrigger id="audit-type-filter">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All types</SelectItem>
                  {(['internal', 'external', 'certification'] as const).map((value) => (
                    <SelectItem key={value} value={value}>
                      {humanizeAuditToken(value)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end gap-2">
              <Button type="submit">Search</Button>
              {filtersActive && (
                <Button type="button" variant="outline" onClick={resetFilters}>
                  <FilterX aria-hidden="true" className="mr-2 h-4 w-4" />
                  Reset
                </Button>
              )}
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          <ResourceBoundary
            surface="plain"
            isEmpty={audits.length === 0}
            isError={auditsQuery.isError && !data}
            isLoading={auditsQuery.isLoading}
            loadingLayout="table"
            loadingTitle="Loading audits"
            emptyTitle={filtersActive ? 'No audits match these filters' : 'No audits have been planned'}
            emptyDescription={
              filtersActive
                ? 'Adjust or clear the filters to broaden the results.'
                : 'Create an audit plan to begin an assurance engagement.'
            }
            emptyAction={
              filtersActive ? (
                <Button variant="outline" onClick={resetFilters}>
                  Clear filters
                </Button>
              ) : access.canCreate ? (
                <Button onClick={() => setCreateOpen(true)}>Plan first audit</Button>
              ) : null
            }
            errorTitle="Audits could not be loaded"
            errorDescription="The service did not return the audit list. Retry without losing your filters."
            onRetry={() => void auditsQuery.refetch()}
            retrying={auditsQuery.isFetching}
          >
            <>
              <div className="hidden overflow-x-auto md:block">
                <table className="w-full text-sm">
                  <caption className="sr-only">Audits matching the active filters</caption>
                  <thead>
                    <tr className="border-b bg-muted/50">
                      <th scope="col" className="px-4 py-3 text-left font-medium">
                        Audit
                      </th>
                      <th scope="col" className="px-4 py-3 text-left font-medium">
                        Type
                      </th>
                      <th scope="col" className="px-4 py-3 text-left font-medium">
                        Status
                      </th>
                      <th scope="col" className="px-4 py-3 text-left font-medium">
                        Lead auditor
                      </th>
                      <th scope="col" className="px-4 py-3 text-left font-medium">
                        Schedule
                      </th>
                      <th scope="col" className="px-4 py-3 text-left font-medium">
                        Findings
                      </th>
                      <th scope="col" className="px-4 py-3 text-right font-medium">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {audits.map((audit) => (
                      <tr key={audit.id} className="border-b last:border-0 hover:bg-muted/30">
                        <td className="px-4 py-3">
                          <Link
                            href={`/audits/${audit.id}`}
                            className="font-medium text-primary hover:underline"
                          >
                            {audit.title}
                          </Link>
                          <div className="font-mono text-xs text-muted-foreground">
                            {audit.audit_ref}
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <Badge className={AUDIT_TYPE_STYLES[audit.audit_type]}>
                            {humanizeAuditToken(audit.audit_type)}
                          </Badge>
                        </td>
                        <td className="px-4 py-3">
                          <Badge className={getStatusColor(audit.status)}>
                            {humanizeAuditToken(audit.status)}
                          </Badge>
                        </td>
                        <td className="px-4 py-3">
                          {auditPersonName(audit.lead_auditor, audit.lead_auditor_id)}
                        </td>
                        <td className="whitespace-nowrap px-4 py-3">
                          {formatDate(audit.scheduled_start_date)} –{' '}
                          {formatDate(audit.scheduled_end_date)}
                        </td>
                        <td className="px-4 py-3">
                          <span
                            className={cn(
                              audit.critical_findings_open + audit.high_findings_open > 0 &&
                                'font-semibold text-destructive',
                            )}
                          >
                            {audit.findings_count} total ·{' '}
                            {audit.critical_findings_open + audit.high_findings_open} critical/high
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <AuditActions
                            audit={audit}
                            canUpdate={access.canUpdate}
                            canDelete={access.canDelete}
                            onEdit={setEditingAudit}
                            onDelete={setDeletingAudit}
                          />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div className="divide-y md:hidden">
                {audits.map((audit) => (
                  <article key={audit.id} className="space-y-3 p-4">
                    <div className="flex flex-wrap gap-2">
                      <Badge className={getStatusColor(audit.status)}>
                        {humanizeAuditToken(audit.status)}
                      </Badge>
                      <Badge className={AUDIT_TYPE_STYLES[audit.audit_type]}>
                        {humanizeAuditToken(audit.audit_type)}
                      </Badge>
                    </div>
                    <div>
                      <Link
                        href={`/audits/${audit.id}`}
                        className="font-semibold text-primary hover:underline"
                      >
                        {audit.title}
                      </Link>
                      <p className="font-mono text-xs text-muted-foreground">{audit.audit_ref}</p>
                    </div>
                    <dl className="grid grid-cols-2 gap-3 text-sm">
                      <div>
                        <dt className="text-muted-foreground">Lead</dt>
                        <dd>{auditPersonName(audit.lead_auditor, audit.lead_auditor_id)}</dd>
                      </div>
                      <div>
                        <dt className="text-muted-foreground">Findings</dt>
                        <dd>{audit.findings_count} total</dd>
                      </div>
                      <div className="col-span-2">
                        <dt className="text-muted-foreground">Scheduled</dt>
                        <dd>
                          {formatDate(audit.scheduled_start_date)} –{' '}
                          {formatDate(audit.scheduled_end_date)}
                        </dd>
                      </div>
                    </dl>
                    <AuditActions
                      audit={audit}
                      canUpdate={access.canUpdate}
                      canDelete={access.canDelete}
                      onEdit={setEditingAudit}
                      onDelete={setDeletingAudit}
                    />
                  </article>
                ))}
              </div>
            </>
          </ResourceBoundary>

          {data && data.total > 0 && (
            <div className="flex flex-col gap-3 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-muted-foreground" aria-live="polite">
                Page {page} of {totalPages} · {data.total} audit{data.total === 1 ? '' : 's'}
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <Label htmlFor="audit-page-size" className="text-sm font-normal">
                  Rows
                </Label>
                <Select
                  value={String(pageSize)}
                  onValueChange={(value) => {
                    setPageSize(Number(value));
                    setPage(1);
                  }}
                >
                  <SelectTrigger id="audit-page-size" className="w-20">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PAGE_SIZES.map((size) => (
                      <SelectItem key={size} value={String(size)}>
                        {size}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  size="icon"
                  variant="outline"
                  aria-label="Previous audits page"
                  disabled={page <= 1 || auditsQuery.isFetching}
                  onClick={() => setPage((value) => value - 1)}
                >
                  <ChevronLeft aria-hidden="true" className="h-4 w-4" />
                </Button>
                <Button
                  size="icon"
                  variant="outline"
                  aria-label="Next audits page"
                  disabled={page >= totalPages || auditsQuery.isFetching}
                  onClick={() => setPage((value) => value + 1)}
                >
                  <ChevronRight aria-hidden="true" className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {access.canCreate && (
        <AuditEditorDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          defaultLeadAuditorId={access.user.id}
        />
      )}
      {access.canUpdate && editingAudit && (
        <AuditEditorDialog
          audit={editingAudit}
          open={Boolean(editingAudit)}
          onOpenChange={(open) => !open && setEditingAudit(null)}
          defaultLeadAuditorId={access.user.id}
        />
      )}
      {access.canDelete && deletingAudit && (
        <AuditConfirmationDialog
          open={Boolean(deletingAudit)}
          onOpenChange={(open) => !open && setDeletingAudit(null)}
          title="Delete audit plan?"
          description={
            <p>
              This removes the audit plan from active records. Executed audits are retained by the
              service and cannot be deleted.
            </p>
          }
          confirmLabel="Delete audit"
          confirmationText={deletingAudit.audit_ref}
          destructive
          onConfirm={() => deleteAudit.mutateAsync(deletingAudit.id)}
        />
      )}
    </div>
  );
}

function AuditActions({
  audit,
  canUpdate,
  canDelete,
  onEdit,
  onDelete,
}: {
  audit: Audit;
  canUpdate: boolean;
  canDelete: boolean;
  onEdit: (audit: Audit) => void;
  onDelete: (audit: Audit) => void;
}) {
  const editable = !['closed', 'cancelled'].includes(audit.status);
  const deletable = ['planned', 'cancelled'].includes(audit.status);
  return (
    <div className="flex justify-end gap-1">
      <Button asChild size="sm" variant="outline">
        <Link href={`/audits/${audit.id}`}>Open</Link>
      </Button>
      {canUpdate && editable && (
        <Button
          size="icon"
          variant="ghost"
          aria-label={`Edit ${audit.audit_ref}`}
          onClick={() => onEdit(audit)}
        >
          <Pencil aria-hidden="true" className="h-4 w-4" />
        </Button>
      )}
      {canDelete && deletable && (
        <Button
          size="icon"
          variant="ghost"
          aria-label={`Delete ${audit.audit_ref}`}
          className="text-destructive hover:text-destructive"
          onClick={() => onDelete(audit)}
        >
          <Trash2 aria-hidden="true" className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}

function SummaryCard({
  label,
  value,
  icon,
  highlight = false,
}: {
  label: string;
  value: number;
  icon: React.ReactNode;
  highlight?: boolean;
}) {
  return (
    <Card className={cn(highlight && 'border-destructive/50')}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{label}</CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        <div className={cn('text-2xl font-bold', highlight && 'text-destructive')}>{value}</div>
      </CardContent>
    </Card>
  );
}
