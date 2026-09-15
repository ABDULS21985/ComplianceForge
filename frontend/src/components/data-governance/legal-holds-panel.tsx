'use client';

import * as React from 'react';
import {
  ArchiveRestore,
  ChevronLeft,
  ChevronRight,
  Edit3,
  Loader2,
  Plus,
  Scale,
  ShieldOff,
} from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  dataGovernanceKeys,
  formatGovernanceDate,
  formatGovernanceError,
  GOVERNANCE_RECORD_TYPES,
  humanizeGovernanceToken,
  isGovernanceConflict,
} from '@/lib/data-governance';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  GovernanceErrorState,
  GovernancePanelSkeleton,
  GovernanceWarning,
} from './governance-states';
import type {
  GovernanceRecordType,
  LegalHold,
  LegalHoldOutcome,
  LegalHoldRecord,
  LegalHoldStatus,
} from '@/types/data-governance';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/data/empty-state';
import { GovernanceReasonDialog } from './governance-reason-dialog';
import { Input } from '@/components/ui/input';
import { isUuid } from '@/lib/enterprise-settings';
import { Label } from '@/components/ui/label';
import { LegalHoldEditorDialog } from './legal-hold-editor-dialog';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';

export function LegalHoldsPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [page, setPage] = React.useState(1);
  const [status, setStatus] = React.useState('all');
  const [editing, setEditing] = React.useState<LegalHold | 'new' | null>(null);
  const [managing, setManaging] = React.useState<LegalHold | null>(null);
  const params = {
    page,
    page_size: 20,
    status: status === 'all' ? undefined : (status as LegalHoldStatus),
  };
  const query = useQuery({
    queryKey: dataGovernanceKeys.holds(params),
    queryFn: () => api.dataGovernance.listHolds(params),
    placeholderData: (previous) => previous,
  });

  function refresh() {
    void queryClient.invalidateQueries({ queryKey: dataGovernanceKeys.all });
  }
  function updateManaged(saved: LegalHold) {
    setManaging(saved);
    refresh();
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>Legal holds</CardTitle>
              <CardDescription>
                Preserve matter records and prevent disposition until an authorized release.
              </CardDescription>
            </div>
            {canConfigure && (
              <Button type="button" onClick={() => setEditing('new')}>
                <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
                Place hold
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <Select
            value={status}
            onValueChange={(value) => {
              setStatus(value);
              setPage(1);
            }}
          >
            <SelectTrigger aria-label="Filter legal holds by status" className="w-full sm:w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All hold statuses</SelectItem>
              {(['active', 'released', 'cancelled'] as const).map((value) => (
                <SelectItem key={value} value={value}>
                  {humanizeGovernanceToken(value)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardContent>
      </Card>
      {query.isLoading ? (
        <GovernancePanelSkeleton label="Loading legal holds" />
      ) : query.isError ? (
        <GovernanceErrorState
          message={formatGovernanceError(query.error, 'Legal holds could not be loaded.')}
          onRetry={() => void query.refetch()}
        />
      ) : (query.data?.data.length ?? 0) === 0 ? (
        <Card>
          <EmptyState
            icon={Scale}
            title="No legal holds"
            description="Place a hold or adjust the current status filter."
            actionLabel={canConfigure ? 'Place legal hold' : undefined}
            onAction={canConfigure ? () => setEditing('new') : undefined}
          />
        </Card>
      ) : (
        <div className="space-y-3">
          {query.data?.data.map((hold) => (
            <HoldCard
              key={hold.id}
              canConfigure={canConfigure}
              hold={hold}
              onEdit={() => setEditing(hold)}
              onManage={() => setManaging(hold)}
            />
          ))}
          {query.data?.pagination && (
            <div className="flex flex-col gap-3 rounded-md border bg-card p-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-muted-foreground">
                Page {query.data.pagination.page} of{' '}
                {Math.max(1, query.data.pagination.total_pages)} ·{' '}
                {query.data.pagination.total_items} holds
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={page <= 1 || query.isFetching}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  <ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />
                  Previous
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={page >= query.data.pagination.total_pages || query.isFetching}
                  onClick={() => setPage((current) => current + 1)}
                >
                  Next
                  <ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
      {editing && (
        <LegalHoldEditorDialog
          open
          hold={editing === 'new' ? undefined : editing}
          onOpenChange={(open) => !open && setEditing(null)}
          onReload={() => {
            setEditing(null);
            refresh();
          }}
          onSaved={() => refresh()}
        />
      )}
      {managing && (
        <HoldRecordsDialog
          canConfigure={canConfigure}
          hold={managing}
          open
          onHoldUpdated={updateManaged}
          onOpenChange={(open) => !open && setManaging(null)}
        />
      )}
    </div>
  );
}

function HoldCard({
  canConfigure,
  hold,
  onEdit,
  onManage,
}: {
  canConfigure: boolean;
  hold: LegalHold;
  onEdit: () => void;
  onManage: () => void;
}) {
  const reviewOverdue =
    hold.status === 'active' &&
    hold.review_due_at !== undefined &&
    new Date(hold.review_due_at) < new Date();
  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-semibold">{hold.name}</h3>
              <Badge variant={hold.status === 'active' ? 'default' : 'secondary'}>
                {humanizeGovernanceToken(hold.status)}
              </Badge>
              <Badge variant="outline">{hold.hold_ref}</Badge>
              {reviewOverdue && <Badge variant="destructive">Review overdue</Badge>}
            </div>
            <p className="text-sm text-muted-foreground">{hold.description}</p>
            <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
              <Detail label="Matter" value={hold.matter_reference || '—'} />
              <Detail label="Legal authority" value={hold.legal_authority} />
              <Detail label="Owner" value={hold.owner_user_id} />
              <Detail label="Records" value={String(hold.record_count)} />
              <Detail label="Custodians" value={String(hold.custodian_ids.length)} />
              <Detail label="Placed" value={formatGovernanceDate(hold.placed_at)} />
              <Detail label="Review due" value={formatGovernanceDate(hold.review_due_at)} />
              <Detail label="Version" value={String(hold.version)} />
            </dl>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button type="button" size="sm" variant="outline" onClick={onManage}>
              <ArchiveRestore aria-hidden="true" className="mr-2 h-4 w-4" />
              Records
            </Button>
            {canConfigure && hold.status === 'active' && (
              <Button type="button" size="sm" variant="outline" onClick={onEdit}>
                <Edit3 aria-hidden="true" className="mr-2 h-4 w-4" />
                Edit
              </Button>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function HoldRecordsDialog({
  canConfigure,
  hold,
  onHoldUpdated,
  onOpenChange,
  open,
}: {
  canConfigure: boolean;
  hold: LegalHold;
  onHoldUpdated: (hold: LegalHold) => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
}) {
  const queryClient = useQueryClient();
  const [activeOnly, setActiveOnly] = React.useState(true);
  const [addOpen, setAddOpen] = React.useState(false);
  const [releasingRecord, setReleasingRecord] = React.useState<LegalHoldRecord | null>(null);
  const [closing, setClosing] = React.useState<LegalHoldOutcome | null>(null);
  const [actionError, setActionError] = React.useState('');
  const recordsQuery = useQuery({
    queryKey: dataGovernanceKeys.holdRecords(hold.id, activeOnly),
    queryFn: () => api.dataGovernance.listHoldRecords(hold.id, activeOnly),
    retry: false,
  });
  async function refreshHoldAndRecords() {
    await queryClient.invalidateQueries({
      queryKey: ['data-governance', 'hold', hold.id, 'records'],
    });
    const current = await api.dataGovernance.getHold(hold.id);
    onHoldUpdated(current);
  }
  const releaseRecordMutation = useMutation({
    mutationFn: (reason: string) => {
      if (!releasingRecord) throw new Error('Choose a record to release.');
      return api.dataGovernance.releaseHoldRecord(hold.id, releasingRecord.id, { reason });
    },
    onSuccess: async () => {
      setReleasingRecord(null);
      setActionError('');
      await refreshHoldAndRecords();
    },
    onError: (error) =>
      setActionError(
        formatGovernanceError(error, 'The record could not be released from the hold.'),
      ),
  });
  const closeMutation = useMutation({
    mutationFn: (reason: string) => {
      if (!closing) throw new Error('Choose a hold outcome.');
      return api.dataGovernance.releaseHold(hold.id, {
        expected_version: hold.version,
        outcome: closing,
        reason,
      });
    },
    onSuccess: (saved) => {
      setClosing(null);
      setActionError('');
      onHoldUpdated(saved);
    },
    onError: (error) =>
      setActionError(formatGovernanceError(error, 'The legal hold could not be closed.')),
  });
  return (
    <Dialog
      open={open}
      onOpenChange={(next) =>
        !releaseRecordMutation.isPending && !closeMutation.isPending && onOpenChange(next)
      }
    >
      <DialogContent className="max-h-[92vh] max-w-4xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{hold.name} records</DialogTitle>
          <DialogDescription>
            {hold.hold_ref} · {humanizeGovernanceToken(hold.status)} · {hold.record_count} record
            {hold.record_count === 1 ? '' : 's'} currently reported
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            <Switch id="hold-active-records" checked={activeOnly} onCheckedChange={setActiveOnly} />
            <Label htmlFor="hold-active-records">Active records only</Label>
          </div>
          {canConfigure && hold.status === 'active' && (
            <div className="flex flex-wrap gap-2">
              <Button type="button" size="sm" onClick={() => setAddOpen(true)}>
                <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
                Add record
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setActionError('');
                  setClosing('release');
                }}
              >
                Release hold
              </Button>
              <Button
                type="button"
                size="sm"
                variant="destructive"
                onClick={() => {
                  setActionError('');
                  setClosing('cancel');
                }}
              >
                <ShieldOff aria-hidden="true" className="mr-2 h-4 w-4" />
                Cancel hold
              </Button>
            </div>
          )}
        </div>
        {Object.keys(hold.scope).length > 0 && (
          <details className="rounded-md border p-3">
            <summary className="cursor-pointer text-sm font-medium">View legal hold scope</summary>
            <pre className="mt-3 max-h-64 overflow-auto rounded bg-muted p-3 text-xs">
              {JSON.stringify(hold.scope, null, 2)}
            </pre>
          </details>
        )}
        {recordsQuery.isLoading ? (
          <GovernancePanelSkeleton label="Loading legal hold records" />
        ) : recordsQuery.isError ? (
          <GovernanceErrorState
            message={formatGovernanceError(
              recordsQuery.error,
              'Legal hold records could not be loaded.',
            )}
            onRetry={() => void recordsQuery.refetch()}
          />
        ) : (recordsQuery.data?.data.length ?? 0) === 0 ? (
          <EmptyState
            icon={ArchiveRestore}
            title="No matching hold records"
            description={
              activeOnly
                ? 'No active records are attached to this hold.'
                : 'No records have been attached to this hold.'
            }
          />
        ) : (
          <div className="space-y-2">
            {recordsQuery.data?.data.map((record) => (
              <div
                key={record.id}
                className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-start sm:justify-between"
              >
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">{humanizeGovernanceToken(record.record_type)}</Badge>
                    <span className="break-all font-mono text-xs">{record.record_id}</span>
                    {record.released_at && <Badge variant="secondary">Released</Badge>}
                  </div>
                  <p className="mt-2 text-sm text-muted-foreground">{record.reason}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Placed {formatGovernanceDate(record.placed_at)}
                    {record.released_at
                      ? ` · Released ${formatGovernanceDate(record.released_at)}`
                      : ''}
                  </p>
                </div>
                {canConfigure && hold.status === 'active' && !record.released_at && (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setActionError('');
                      setReleasingRecord(record);
                    }}
                  >
                    Release record
                  </Button>
                )}
              </div>
            ))}
          </div>
        )}
        {isGovernanceConflict(closeMutation.error) && (
          <GovernanceWarning>
            This hold changed before the close action. Close this dialog, reload the hold list, and
            retry against the latest version.
          </GovernanceWarning>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
        {addOpen && (
          <AddHoldRecordDialog
            holdId={hold.id}
            open
            onOpenChange={setAddOpen}
            onSaved={() => {
              void refreshHoldAndRecords();
            }}
          />
        )}
        {releasingRecord && (
          <GovernanceReasonDialog
            open
            title="Release record from hold?"
            description={`This removes the legal preservation block from ${humanizeGovernanceToken(releasingRecord.record_type)} ${releasingRecord.record_id}.`}
            actionLabel="Release record"
            pending={releaseRecordMutation.isPending}
            error={actionError}
            onOpenChange={(next) => !next && setReleasingRecord(null)}
            onSubmit={(reason) => releaseRecordMutation.mutate(reason)}
          />
        )}
        {closing && (
          <GovernanceReasonDialog
            open
            title={closing === 'release' ? 'Release legal hold?' : 'Cancel legal hold?'}
            description={
              closing === 'release'
                ? 'All records on this matter will no longer be protected by this hold. Confirm that legal authorization has been received.'
                : 'Cancellation closes this hold as cancelled and cannot be undone.'
            }
            actionLabel={closing === 'release' ? 'Release hold' : 'Cancel hold'}
            danger={closing === 'cancel'}
            pending={closeMutation.isPending}
            error={actionError}
            onOpenChange={(next) => !next && setClosing(null)}
            onSubmit={(reason) => closeMutation.mutate(reason)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function AddHoldRecordDialog({
  holdId,
  onOpenChange,
  onSaved,
  open,
}: {
  holdId: string;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
  open: boolean;
}) {
  const [recordType, setRecordType] = React.useState<GovernanceRecordType>('asset');
  const [recordId, setRecordId] = React.useState('');
  const [reason, setReason] = React.useState('');
  const [validation, setValidation] = React.useState('');
  const mutation = useMutation({
    mutationFn: () => {
      if (!isUuid(recordId.trim())) throw new Error('Enter a valid record UUID.');
      if (reason.trim().length < 3) throw new Error('Give a reason of at least 3 characters.');
      return api.dataGovernance.addHoldRecord(holdId, {
        record_type: recordType,
        record_id: recordId.trim(),
        reason: reason.trim(),
      });
    },
    onSuccess: () => {
      onSaved();
      onOpenChange(false);
    },
    onError: (error) =>
      setValidation(formatGovernanceError(error, 'The record could not be added to the hold.')),
  });
  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent>
        <form
          className="space-y-5"
          onSubmit={(event) => {
            event.preventDefault();
            setValidation('');
            mutation.mutate();
          }}
        >
          <DialogHeader>
            <DialogTitle>Add record to legal hold</DialogTitle>
            <DialogDescription>
              Disposition will be blocked until this record or the entire hold is released.
            </DialogDescription>
          </DialogHeader>
          <Field label="Record type" htmlFor="hold-record-type">
            <Select
              value={recordType}
              onValueChange={(value) => setRecordType(value as GovernanceRecordType)}
            >
              <SelectTrigger id="hold-record-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {GOVERNANCE_RECORD_TYPES.map((value) => (
                  <SelectItem key={value} value={value}>
                    {humanizeGovernanceToken(value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label="Record UUID" htmlFor="hold-record-id">
            <Input
              id="hold-record-id"
              required
              aria-invalid={Boolean(validation)}
              value={recordId}
              onChange={(event) => setRecordId(event.target.value)}
            />
          </Field>
          <Field label="Reason" htmlFor="hold-record-reason">
            <Textarea
              id="hold-record-reason"
              required
              minLength={3}
              maxLength={2000}
              rows={3}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          </Field>
          {validation && (
            <p role="alert" className="text-sm text-destructive">
              {validation}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              Add to hold
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 break-all font-medium">{value}</dd>
    </div>
  );
}
function Field({
  children,
  htmlFor,
  label,
}: {
  children: React.ReactNode;
  htmlFor: string;
  label: string;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
    </div>
  );
}
