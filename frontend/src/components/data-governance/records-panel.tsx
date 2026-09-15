'use client';

import * as React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  CheckCircle2,
  Clock3,
  FileSearch,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  ShieldAlert,
  XCircle,
} from 'lucide-react';
import {
  DATA_CLASSIFICATIONS,
  dataGovernanceKeys,
  formatGovernanceDate,
  formatGovernanceError,
  GOVERNANCE_RECORD_TYPES,
  humanizeGovernanceToken,
  isGovernanceConflict,
  localDateTimeInput,
  toISOStringOrUndefined,
} from '@/lib/data-governance';
import type {
  DataClassification,
  GovernanceRecordType,
  RecordRetentionAssignment,
  RetentionDecision,
  RetentionException,
} from '@/types/data-governance';
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
import { Textarea } from '@/components/ui/textarea';

interface RecordLookup {
  id: string;
  type: GovernanceRecordType;
}

export function RecordsPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [recordType, setRecordType] = React.useState<GovernanceRecordType>('asset');
  const [recordId, setRecordId] = React.useState('');
  const [recordValidation, setRecordValidation] = React.useState('');
  const [lookup, setLookup] = React.useState<RecordLookup | null>(null);
  const [assignmentInput, setAssignmentInput] = React.useState('');
  const [assignmentValidation, setAssignmentValidation] = React.useState('');
  const [assignmentId, setAssignmentId] = React.useState('');
  const [createOpen, setCreateOpen] = React.useState(false);
  const dispositionQuery = useQuery({
    queryKey: dataGovernanceKeys.disposition(lookup?.type ?? '', lookup?.id ?? ''),
    queryFn: () => api.dataGovernance.getDisposition(lookup!.type, lookup!.id),
    enabled: Boolean(lookup),
    retry: false,
  });
  const assignmentQuery = useQuery({
    queryKey: dataGovernanceKeys.assignment(assignmentId),
    queryFn: () => api.dataGovernance.getAssignment(assignmentId),
    enabled: Boolean(assignmentId),
    retry: false,
  });
  const assignment = assignmentQuery.data ?? dispositionQuery.data?.assignment;

  function evaluateRecord(event: React.FormEvent) {
    event.preventDefault();
    const id = recordId.trim();
    if (!isUuid(id)) {
      setRecordValidation('Enter the record’s UUID.');
      return;
    }
    setRecordValidation('');
    setAssignmentId('');
    setLookup({ id, type: recordType });
  }

  function loadAssignment(event: React.FormEvent) {
    event.preventDefault();
    const id = assignmentInput.trim();
    if (!isUuid(id)) {
      setAssignmentValidation('Enter a valid retention assignment UUID.');
      return;
    }
    setAssignmentValidation('');
    setLookup(null);
    setAssignmentId(id);
  }

  function refreshAssignment() {
    if (assignment?.id)
      void queryClient.invalidateQueries({
        queryKey: dataGovernanceKeys.assignment(assignment.id),
      });
    if (lookup)
      void queryClient.invalidateQueries({
        queryKey: dataGovernanceKeys.disposition(lookup.type, lookup.id),
      });
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Evaluate a record</CardTitle>
            <CardDescription>
              Check whether disposition is allowed and whether retention or a legal hold blocks it.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="grid gap-3 sm:grid-cols-[12rem_minmax(0,1fr)_auto]"
              onSubmit={evaluateRecord}
            >
              <Select
                value={recordType}
                onValueChange={(value) => setRecordType(value as GovernanceRecordType)}
              >
                <SelectTrigger aria-label="Record type">
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
              <div>
                <Input
                  aria-label="Record UUID"
                  aria-invalid={Boolean(recordValidation)}
                  placeholder="Record UUID"
                  value={recordId}
                  onChange={(event) => setRecordId(event.target.value)}
                />
                {recordValidation && (
                  <p role="alert" className="mt-1 text-xs text-destructive">
                    {recordValidation}
                  </p>
                )}
              </div>
              <Button type="submit">
                <Search aria-hidden="true" className="mr-2 h-4 w-4" />
                Evaluate
              </Button>
            </form>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Open an assignment</CardTitle>
            <CardDescription>
              Use a known assignment reference to review disposition and manage exceptions.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto]"
              onSubmit={loadAssignment}
            >
              <div>
                <Input
                  aria-label="Retention assignment UUID"
                  aria-invalid={Boolean(assignmentValidation)}
                  placeholder="Assignment UUID"
                  value={assignmentInput}
                  onChange={(event) => setAssignmentInput(event.target.value)}
                />
                {assignmentValidation && (
                  <p role="alert" className="mt-1 text-xs text-destructive">
                    {assignmentValidation}
                  </p>
                )}
              </div>
              <Button type="submit" variant="outline">
                <FileSearch aria-hidden="true" className="mr-2 h-4 w-4" />
                Open
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>

      {!lookup && !assignmentId ? (
        <Card>
          <EmptyState
            icon={FileSearch}
            title="Choose a governed record"
            description="Record assignments are intentionally lookup-driven because the API does not expose an organization-wide assignment collection."
          />
        </Card>
      ) : dispositionQuery.isLoading || assignmentQuery.isLoading ? (
        <GovernancePanelSkeleton label="Loading record disposition" />
      ) : dispositionQuery.isError ? (
        <GovernanceErrorState
          message={formatGovernanceError(
            dispositionQuery.error,
            'The disposition decision could not be loaded.',
          )}
          onRetry={() => void dispositionQuery.refetch()}
        />
      ) : assignmentQuery.isError ? (
        <GovernanceErrorState
          message={formatGovernanceError(
            assignmentQuery.error,
            'The retention assignment could not be loaded.',
          )}
          onRetry={() => void assignmentQuery.refetch()}
        />
      ) : (
        <div className="space-y-4">
          {dispositionQuery.data && (
            <DispositionSummary
              decision={dispositionQuery.data}
              canConfigure={canConfigure}
              onCreateAssignment={() => setCreateOpen(true)}
            />
          )}
          {assignment ? (
            <AssignmentWorkspace
              assignment={assignment}
              canConfigure={canConfigure}
              onRefresh={refreshAssignment}
            />
          ) : dispositionQuery.data && canConfigure ? (
            <Card>
              <CardContent className="flex flex-col gap-3 py-6 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <h3 className="font-semibold">No retention assignment</h3>
                  <p className="text-sm text-muted-foreground">
                    Attach an active schedule to bring this record under governed retention.
                  </p>
                </div>
                <Button type="button" onClick={() => setCreateOpen(true)}>
                  <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
                  Assign schedule
                </Button>
              </CardContent>
            </Card>
          ) : null}
        </div>
      )}

      {createOpen && lookup && (
        <CreateAssignmentDialog
          open
          record={lookup}
          onOpenChange={setCreateOpen}
          onCreated={(created) => {
            setAssignmentId(created.id);
            setAssignmentInput(created.id);
            setLookup(null);
          }}
        />
      )}
    </div>
  );
}

function DispositionSummary({
  canConfigure,
  decision,
  onCreateAssignment,
}: {
  canConfigure: boolean;
  decision: Awaited<ReturnType<typeof api.dataGovernance.getDisposition>>;
  onCreateAssignment: () => void;
}) {
  const Icon = decision.allowed ? CheckCircle2 : ShieldAlert;
  return (
    <Card className={decision.allowed ? 'border-emerald-500/30' : 'border-amber-500/40'}>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex gap-3">
            <Icon
              aria-hidden="true"
              className={`mt-0.5 h-6 w-6 ${decision.allowed ? 'text-emerald-600' : 'text-amber-700'}`}
            />
            <div>
              <CardTitle>
                {decision.allowed ? 'Disposition permitted' : 'Disposition blocked'}
              </CardTitle>
              <CardDescription className="mt-1">{decision.reason}</CardDescription>
            </div>
          </div>
          <Badge variant={decision.allowed ? 'default' : 'secondary'}>
            {humanizeGovernanceToken(decision.record_type)} · {decision.record_id.slice(0, 8)}…
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-xs text-muted-foreground">
          Evaluated {formatGovernanceDate(decision.evaluated_at)}
        </p>
        {decision.active_legal_holds.length > 0 && (
          <div>
            <h3 className="text-sm font-medium">Active legal holds</h3>
            <div className="mt-2 space-y-2">
              {decision.active_legal_holds.map((record) => (
                <div key={record.id} className="rounded-md border p-3 text-sm">
                  <span className="font-medium">Hold {record.hold_id}</span>
                  <span className="block text-muted-foreground">{record.reason}</span>
                </div>
              ))}
            </div>
          </div>
        )}
        {!decision.assignment && canConfigure && (
          <Button type="button" size="sm" onClick={onCreateAssignment}>
            <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
            Assign retention schedule
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

function AssignmentWorkspace({
  assignment,
  canConfigure,
  onRefresh,
}: {
  assignment: RecordRetentionAssignment;
  canConfigure: boolean;
  onRefresh: () => void;
}) {
  const overdue =
    assignment.state !== 'disposed' && new Date(assignment.disposition_due_at) < new Date();
  const queryClient = useQueryClient();
  const [review, setReview] = React.useState<RetentionDecision | null>(null);
  const [reviewError, setReviewError] = React.useState('');
  const [exceptionOpen, setExceptionOpen] = React.useState(false);
  const [deciding, setDeciding] = React.useState<{
    decision: RetentionDecision;
    exception: RetentionException;
  } | null>(null);
  const [decisionError, setDecisionError] = React.useState('');
  const exceptionsQuery = useQuery({
    queryKey: dataGovernanceKeys.exceptions(assignment.id),
    queryFn: () => api.dataGovernance.listExceptions(assignment.id),
    retry: false,
  });
  const reviewMutation = useMutation({
    mutationFn: ({ decision, reason }: { decision: RetentionDecision; reason: string }) =>
      api.dataGovernance.reviewAssignment(assignment.id, {
        expected_version: assignment.version,
        decision,
        reason,
      }),
    onSuccess: () => {
      setReview(null);
      setReviewError('');
      onRefresh();
    },
    onError: (error) =>
      setReviewError(formatGovernanceError(error, 'The disposition review could not be saved.')),
  });
  const decisionMutation = useMutation({
    mutationFn: ({
      decision,
      exception,
      reason,
    }: {
      decision: RetentionDecision;
      exception: RetentionException;
      reason: string;
    }) =>
      api.dataGovernance.decideException(exception.id, {
        expected_version: exception.version,
        decision,
        reason,
      }),
    onSuccess: async () => {
      setDeciding(null);
      setDecisionError('');
      await queryClient.invalidateQueries({
        queryKey: dataGovernanceKeys.exceptions(assignment.id),
      });
      onRefresh();
    },
    onError: (error) =>
      setDecisionError(formatGovernanceError(error, 'The exception decision could not be saved.')),
  });
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>Retention assignment</CardTitle>
            <CardDescription className="break-all">
              {assignment.id} · version {assignment.version}
            </CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            <Badge>{humanizeGovernanceToken(assignment.state)}</Badge>
            <Badge variant="outline">
              Review: {humanizeGovernanceToken(assignment.review_status)}
            </Badge>
            {overdue && <Badge variant="destructive">Disposition overdue</Badge>}
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Detail
            label="Record"
            value={`${humanizeGovernanceToken(assignment.record_type)} · ${assignment.record_id}`}
          />
          <Detail
            label="Disposition due"
            value={formatGovernanceDate(assignment.disposition_due_at)}
          />
          <Detail label="Action" value={humanizeGovernanceToken(assignment.disposition_action)} />
          <Detail label="Started" value={formatGovernanceDate(assignment.retention_started_at)} />
          <Detail
            label="Archive eligible"
            value={formatGovernanceDate(assignment.archive_eligible_at)}
          />
          <Detail label="Source" value={humanizeGovernanceToken(assignment.source)} />
          <Detail
            label="Classification"
            value={humanizeGovernanceToken(assignment.data_classification ?? 'any')}
          />
          <Detail label="Jurisdiction" value={assignment.jurisdiction || 'Any'} />
        </dl>
        {assignment.review_reason && (
          <div className="rounded-md bg-muted p-3 text-sm">
            <span className="font-medium">Review rationale: </span>
            {assignment.review_reason}
          </div>
        )}
        {canConfigure && (
          <div className="flex flex-wrap gap-2 border-t pt-4">
            {assignment.review_required && assignment.review_status === 'pending' && (
              <>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => {
                    setReviewError('');
                    setReview('approve');
                  }}
                >
                  <CheckCircle2 aria-hidden="true" className="mr-2 h-4 w-4" />
                  Approve disposition
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setReviewError('');
                    setReview('reject');
                  }}
                >
                  <XCircle aria-hidden="true" className="mr-2 h-4 w-4" />
                  Reject
                </Button>
              </>
            )}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setExceptionOpen(true)}
            >
              <Clock3 aria-hidden="true" className="mr-2 h-4 w-4" />
              Request exception
            </Button>
          </div>
        )}
        <div className="border-t pt-5">
          <h3 className="font-semibold">Retention exceptions</h3>
          {exceptionsQuery.isLoading ? (
            <div role="status" className="mt-3 text-sm text-muted-foreground">
              Loading exceptions…
            </div>
          ) : exceptionsQuery.isError ? (
            <div className="mt-3 space-y-2">
              <p role="alert" className="text-sm text-destructive">
                {formatGovernanceError(exceptionsQuery.error, 'Exceptions could not be loaded.')}
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void exceptionsQuery.refetch()}
              >
                <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
                Retry
              </Button>
            </div>
          ) : (exceptionsQuery.data?.data.length ?? 0) === 0 ? (
            <p className="mt-2 text-sm text-muted-foreground">No exceptions have been requested.</p>
          ) : (
            <div className="mt-3 space-y-2">
              {exceptionsQuery.data?.data.map((exception) => (
                <div
                  key={exception.id}
                  className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-start sm:justify-between"
                >
                  <div>
                    <div className="flex flex-wrap gap-2">
                      <Badge variant={exception.status === 'pending' ? 'secondary' : 'outline'}>
                        {humanizeGovernanceToken(exception.status)}
                      </Badge>
                      <span className="text-sm font-medium">
                        Until {formatGovernanceDate(exception.requested_until)}
                      </span>
                    </div>
                    <p className="mt-1 text-sm text-muted-foreground">{exception.reason}</p>
                    {exception.decision_reason && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        Decision: {exception.decision_reason}
                      </p>
                    )}
                  </div>
                  {canConfigure && exception.status === 'pending' && (
                    <div className="flex shrink-0 gap-2">
                      <Button
                        type="button"
                        size="sm"
                        onClick={() => {
                          setDecisionError('');
                          setDeciding({ decision: 'approve', exception });
                        }}
                      >
                        Approve
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setDecisionError('');
                          setDeciding({ decision: 'reject', exception });
                        }}
                      >
                        Reject
                      </Button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
        {isGovernanceConflict(reviewMutation.error) ||
        isGovernanceConflict(decisionMutation.error) ? (
          <GovernanceWarning>
            The assignment or exception changed. Reload it before retrying the versioned action.
          </GovernanceWarning>
        ) : null}
      </CardContent>
      {review && (
        <GovernanceReasonDialog
          open
          title={`${humanizeGovernanceToken(review)} disposition?`}
          description={`This versioned decision will be recorded against assignment ${assignment.id}.`}
          actionLabel={`${humanizeGovernanceToken(review)} disposition`}
          danger={review === 'reject'}
          pending={reviewMutation.isPending}
          error={reviewError}
          onOpenChange={(open) => !open && setReview(null)}
          onSubmit={(reason) => reviewMutation.mutate({ decision: review, reason })}
        />
      )}
      {exceptionOpen && (
        <ExceptionRequestDialog
          open
          assignmentId={assignment.id}
          onOpenChange={setExceptionOpen}
          onSaved={() => {
            void queryClient.invalidateQueries({
              queryKey: dataGovernanceKeys.exceptions(assignment.id),
            });
            onRefresh();
          }}
        />
      )}
      {deciding && (
        <GovernanceReasonDialog
          open
          title={`${humanizeGovernanceToken(deciding.decision)} exception?`}
          description={`Decide the requested extension through ${formatGovernanceDate(deciding.exception.requested_until)}.`}
          actionLabel={`${humanizeGovernanceToken(deciding.decision)} exception`}
          danger={deciding.decision === 'reject'}
          pending={decisionMutation.isPending}
          error={decisionError}
          onOpenChange={(open) => !open && setDeciding(null)}
          onSubmit={(reason) => decisionMutation.mutate({ ...deciding, reason })}
        />
      )}
    </Card>
  );
}

function CreateAssignmentDialog({
  onCreated,
  onOpenChange,
  open,
  record,
}: {
  onCreated: (assignment: RecordRetentionAssignment) => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  record: RecordLookup;
}) {
  const [scheduleId, setScheduleId] = React.useState('');
  const [classification, setClassification] = React.useState<DataClassification | 'none'>('none');
  const [jurisdiction, setJurisdiction] = React.useState('');
  const [startedAt, setStartedAt] = React.useState(() =>
    localDateTimeInput(new Date().toISOString()),
  );
  const [reason, setReason] = React.useState('');
  const [validation, setValidation] = React.useState('');
  const schedulesQuery = useQuery({
    queryKey: dataGovernanceKeys.schedules({ status: 'active', page_size: 100 }),
    queryFn: () =>
      api.dataGovernance.listSchedules({
        status: 'active',
        page: 1,
        page_size: 100,
        sort_by: 'priority',
        sort_direction: 'asc',
      }),
  });
  const mutation = useMutation({
    mutationFn: () => {
      const start = toISOStringOrUndefined(startedAt);
      if (!scheduleId) throw new Error('Choose an active retention schedule.');
      if (!start || new Date(start) > new Date(Date.now() + 5 * 60_000))
        throw new Error('Retention start must be valid and cannot be in the future.');
      if (reason.trim().length < 3)
        throw new Error('Give a reason of at least 3 characters for the audit history.');
      return api.dataGovernance.createAssignment({
        schedule_id: scheduleId,
        record_type: record.type,
        record_id: record.id,
        data_classification: classification === 'none' ? undefined : classification,
        jurisdiction: jurisdiction.trim().toUpperCase() || undefined,
        retention_started_at: start,
        source: 'manual',
        reason: reason.trim(),
      });
    },
    onSuccess: (created) => {
      onCreated(created);
      onOpenChange(false);
    },
    onError: (error) =>
      setValidation(formatGovernanceError(error, 'The retention assignment could not be created.')),
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
            <DialogTitle>Assign retention schedule</DialogTitle>
            <DialogDescription>
              Govern {humanizeGovernanceToken(record.type)} record {record.id} with an active
              schedule.
            </DialogDescription>
          </DialogHeader>
          {schedulesQuery.isError ? (
            <GovernanceErrorState
              message={formatGovernanceError(
                schedulesQuery.error,
                'Active schedules could not be loaded.',
              )}
              onRetry={() => void schedulesQuery.refetch()}
            />
          ) : (
            <>
              <Field label="Active schedule" htmlFor="assignment-schedule">
                <Select
                  value={scheduleId}
                  onValueChange={setScheduleId}
                  disabled={schedulesQuery.isLoading}
                >
                  <SelectTrigger id="assignment-schedule">
                    <SelectValue
                      placeholder={
                        schedulesQuery.isLoading ? 'Loading schedules…' : 'Choose a schedule'
                      }
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {schedulesQuery.data?.data.map((schedule) => (
                      <SelectItem key={schedule.id} value={schedule.id}>
                        {schedule.name} · {schedule.retention_days} days
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {!schedulesQuery.isLoading && schedulesQuery.data?.data.length === 0 && (
                  <p className="text-xs text-amber-700">
                    No active schedule is available. Activate one in Retention schedules first.
                  </p>
                )}
              </Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Classification" htmlFor="assignment-classification">
                  <Select
                    value={classification}
                    onValueChange={(value) => setClassification(value as typeof classification)}
                  >
                    <SelectTrigger id="assignment-classification">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">Schedule/default</SelectItem>
                      {DATA_CLASSIFICATIONS.map((value) => (
                        <SelectItem key={value} value={value}>
                          {humanizeGovernanceToken(value)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field label="Jurisdiction" htmlFor="assignment-jurisdiction">
                  <Input
                    id="assignment-jurisdiction"
                    maxLength={16}
                    placeholder="EU"
                    value={jurisdiction}
                    onChange={(event) => setJurisdiction(event.target.value.toUpperCase())}
                  />
                </Field>
              </div>
              <Field label="Retention started at" htmlFor="assignment-start">
                <Input
                  id="assignment-start"
                  type="datetime-local"
                  required
                  value={startedAt}
                  onChange={(event) => setStartedAt(event.target.value)}
                />
              </Field>
              <Field label="Reason" htmlFor="assignment-reason">
                <Textarea
                  id="assignment-reason"
                  required
                  minLength={3}
                  maxLength={2000}
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
                <Button
                  type="submit"
                  disabled={mutation.isPending || schedulesQuery.data?.data.length === 0}
                >
                  {mutation.isPending && (
                    <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
                  )}
                  Create assignment
                </Button>
              </DialogFooter>
            </>
          )}
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ExceptionRequestDialog({
  assignmentId,
  onOpenChange,
  onSaved,
  open,
}: {
  assignmentId: string;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
  open: boolean;
}) {
  const [until, setUntil] = React.useState('');
  const [reason, setReason] = React.useState('');
  const [validation, setValidation] = React.useState('');
  const mutation = useMutation({
    mutationFn: () => {
      const requestedUntil = toISOStringOrUndefined(until);
      if (!requestedUntil || new Date(requestedUntil) <= new Date())
        throw new Error('Choose a future exception end date.');
      if (reason.trim().length < 3) throw new Error('Give a reason of at least 3 characters.');
      return api.dataGovernance.requestException(assignmentId, {
        requested_until: requestedUntil,
        reason: reason.trim(),
      });
    },
    onSuccess: () => {
      onSaved();
      onOpenChange(false);
    },
    onError: (error) =>
      setValidation(
        formatGovernanceError(error, 'The retention exception could not be requested.'),
      ),
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
            <DialogTitle>Request retention exception</DialogTitle>
            <DialogDescription>
              Temporarily extend retention. Only one pending request may exist for this assignment.
            </DialogDescription>
          </DialogHeader>
          <Field label="Requested until" htmlFor="exception-until">
            <Input
              id="exception-until"
              type="datetime-local"
              required
              value={until}
              onChange={(event) => setUntil(event.target.value)}
            />
          </Field>
          <Field label="Reason" htmlFor="exception-reason">
            <Textarea
              id="exception-reason"
              required
              minLength={3}
              maxLength={2000}
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
              Request exception
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
      <dd className="mt-1 break-all text-sm font-medium">{value}</dd>
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
