'use client';

import * as React from 'react';
import { ArchiveX, ChevronLeft, ChevronRight, Clock3, Edit3, Plus, Search } from 'lucide-react';
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
  GovernanceErrorState,
  GovernancePanelSkeleton,
  GovernanceWarning,
} from './governance-states';
import type { RetentionSchedule, RetentionScheduleListParams } from '@/types/data-governance';
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
import { ScheduleEditorDialog } from './schedule-editor-dialog';

export function SchedulesPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [page, setPage] = React.useState(1);
  const [search, setSearch] = React.useState('');
  const deferredSearch = React.useDeferredValue(search.trim());
  const [recordType, setRecordType] = React.useState('all');
  const [status, setStatus] = React.useState('all');
  const [editing, setEditing] = React.useState<RetentionSchedule | 'new' | null>(null);
  const [retiring, setRetiring] = React.useState<RetentionSchedule | null>(null);
  const [retireError, setRetireError] = React.useState('');

  const params: RetentionScheduleListParams = {
    page,
    page_size: 20,
    search: deferredSearch || undefined,
    record_type:
      recordType === 'all' ? undefined : (recordType as RetentionScheduleListParams['record_type']),
    status: status === 'all' ? undefined : (status as RetentionScheduleListParams['status']),
    sort_by: 'priority',
    sort_direction: 'asc',
  };
  const query = useQuery({
    queryKey: dataGovernanceKeys.schedules(params),
    queryFn: () => api.dataGovernance.listSchedules(params),
    placeholderData: (previous) => previous,
  });
  const retireMutation = useMutation({
    mutationFn: (reason: string) => {
      if (!retiring) throw new Error('Choose a schedule to retire.');
      return api.dataGovernance.retireSchedule(retiring.id, {
        expected_version: retiring.version,
        reason,
      });
    },
    onSuccess: async () => {
      setRetiring(null);
      setRetireError('');
      await queryClient.invalidateQueries({ queryKey: dataGovernanceKeys.all });
    },
    onError: (error) =>
      setRetireError(formatGovernanceError(error, 'The retention schedule could not be retired.')),
  });

  function refresh() {
    void queryClient.invalidateQueries({ queryKey: dataGovernanceKeys.all });
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>Retention schedules</CardTitle>
              <CardDescription>
                Prioritized, jurisdiction-aware rules determine each record’s lifecycle.
              </CardDescription>
            </div>
            {canConfigure && (
              <Button type="button" onClick={() => setEditing('new')}>
                <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
                New schedule
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-[minmax(0,1fr)_13rem_11rem]">
          <div className="relative">
            <Search
              aria-hidden="true"
              className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              aria-label="Search retention schedules"
              className="pl-9"
              placeholder="Search name or description"
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setPage(1);
              }}
            />
          </div>
          <Select
            value={recordType}
            onValueChange={(value) => {
              setRecordType(value);
              setPage(1);
            }}
          >
            <SelectTrigger aria-label="Filter schedules by record type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All record types</SelectItem>
              {GOVERNANCE_RECORD_TYPES.map((value) => (
                <SelectItem key={value} value={value}>
                  {humanizeGovernanceToken(value)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={status}
            onValueChange={(value) => {
              setStatus(value);
              setPage(1);
            }}
          >
            <SelectTrigger aria-label="Filter schedules by status">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All statuses</SelectItem>
              {(['draft', 'active', 'retired'] as const).map((value) => (
                <SelectItem key={value} value={value}>
                  {humanizeGovernanceToken(value)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardContent>
      </Card>

      {query.isLoading ? (
        <GovernancePanelSkeleton label="Loading retention schedules" />
      ) : query.isError ? (
        <GovernanceErrorState
          message={formatGovernanceError(query.error, 'Retention schedules could not be loaded.')}
          onRetry={() => void query.refetch()}
        />
      ) : (query.data?.data.length ?? 0) === 0 ? (
        <Card>
          <EmptyState
            icon={Clock3}
            title="No retention schedules"
            description="Create a schedule or adjust the current filters."
            actionLabel={canConfigure ? 'Create schedule' : undefined}
            onAction={canConfigure ? () => setEditing('new') : undefined}
          />
        </Card>
      ) : (
        <div className="space-y-3">
          {query.data?.data.map((schedule) => (
            <ScheduleCard
              key={schedule.id}
              canConfigure={canConfigure}
              schedule={schedule}
              onEdit={() => setEditing(schedule)}
              onRetire={() => {
                setRetireError('');
                setRetiring(schedule);
              }}
            />
          ))}
          {query.data?.pagination && (
            <div className="flex flex-col gap-3 rounded-md border bg-card p-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-muted-foreground">
                Page {query.data.pagination.page} of{' '}
                {Math.max(1, query.data.pagination.total_pages)} ·{' '}
                {query.data.pagination.total_items} schedules
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page <= 1 || query.isFetching}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  <ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />
                  Previous
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
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
        <ScheduleEditorDialog
          open
          onOpenChange={(open) => !open && setEditing(null)}
          onReload={() => {
            setEditing(null);
            refresh();
          }}
          onSaved={() => refresh()}
          schedule={editing === 'new' ? undefined : editing}
        />
      )}
      {retiring && (
        <GovernanceReasonDialog
          open
          title={`Retire ${retiring.name}?`}
          description="Retirement stops this schedule from being selected for new records. Existing assignments remain governed and the schedule is retained in history."
          actionLabel="Retire schedule"
          danger
          pending={retireMutation.isPending}
          error={retireError}
          onOpenChange={(open) => !open && setRetiring(null)}
          onSubmit={(reason) => retireMutation.mutate(reason)}
        />
      )}
      {isGovernanceConflict(retireMutation.error) && (
        <GovernanceWarning>
          The schedule changed before retirement. Reload the list and retry against the latest
          version.
        </GovernanceWarning>
      )}
    </div>
  );
}

function ScheduleCard({
  canConfigure,
  onEdit,
  onRetire,
  schedule,
}: {
  canConfigure: boolean;
  onEdit: () => void;
  onRetire: () => void;
  schedule: RetentionSchedule;
}) {
  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-semibold">{schedule.name}</h3>
              <Badge variant={schedule.status === 'active' ? 'default' : 'secondary'}>
                {humanizeGovernanceToken(schedule.status)}
              </Badge>
              <Badge variant="outline">Priority {schedule.priority}</Badge>
            </div>
            <p className="text-sm text-muted-foreground">
              {schedule.description || schedule.legal_basis}
            </p>
            <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 xl:grid-cols-4">
              <Detail
                label="Scope"
                value={`${humanizeGovernanceToken(schedule.record_type)}${schedule.data_classification ? ` · ${humanizeGovernanceToken(schedule.data_classification)}` : ''}`}
              />
              <Detail label="Trigger" value={humanizeGovernanceToken(schedule.trigger_event)} />
              <Detail label="Retention" value={`${schedule.retention_days} days`} />
              <Detail
                label="Disposition"
                value={`${humanizeGovernanceToken(schedule.disposition_action)}${schedule.review_required ? ' · review' : ''}`}
              />
              <Detail label="Jurisdiction" value={schedule.jurisdiction || 'Any'} />
              <Detail label="Legal basis" value={schedule.legal_basis} />
              <Detail label="Effective" value={formatGovernanceDate(schedule.effective_from)} />
              <Detail label="Version" value={String(schedule.version)} />
            </dl>
          </div>
          {canConfigure && (
            <div className="flex shrink-0 flex-wrap gap-2">
              <Button type="button" size="sm" variant="outline" onClick={onEdit}>
                <Edit3 aria-hidden="true" className="mr-2 h-4 w-4" />
                Edit
              </Button>
              {schedule.status !== 'retired' && (
                <Button type="button" size="sm" variant="outline" onClick={onRetire}>
                  <ArchiveX aria-hidden="true" className="mr-2 h-4 w-4" />
                  Retire
                </Button>
              )}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 break-words font-medium">{value}</dd>
    </div>
  );
}
