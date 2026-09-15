'use client';

import * as React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Fingerprint,
  Loader2,
  RefreshCw,
  Search,
  ShieldCheck,
  ShieldX,
} from 'lucide-react';
import {
  dataGovernanceKeys,
  formatGovernanceDate,
  formatGovernanceError,
  humanizeGovernanceToken,
} from '@/lib/data-governance';
import { GovernanceErrorState, GovernancePanelSkeleton } from './governance-states';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type { DataGovernanceEventListParams } from '@/types/data-governance';
import { EmptyState } from '@/components/data/empty-state';
import { Input } from '@/components/ui/input';
import { isUuid } from '@/lib/enterprise-settings';
import { useQuery } from '@tanstack/react-query';

export function GovernanceHistoryPanel() {
  const [page, setPage] = React.useState(1);
  const [entityType, setEntityType] = React.useState('all');
  const [entityId, setEntityId] = React.useState('');
  const [appliedEntityId, setAppliedEntityId] = React.useState('');
  const [eventType, setEventType] = React.useState('');
  const [appliedEventType, setAppliedEventType] = React.useState('');
  const [filterError, setFilterError] = React.useState('');
  const params: DataGovernanceEventListParams = {
    entity_type:
      entityType === 'all'
        ? undefined
        : (entityType as DataGovernanceEventListParams['entity_type']),
    entity_id: appliedEntityId || undefined,
    event_type: appliedEventType || undefined,
    page,
    page_size: 20,
  };
  const eventsQuery = useQuery({
    queryKey: dataGovernanceKeys.events(params),
    queryFn: () => api.dataGovernance.listEvents(params),
    placeholderData: (previous) => previous,
  });
  const verificationQuery = useQuery({
    queryKey: dataGovernanceKeys.verification,
    queryFn: () => api.dataGovernance.verifyEvents(),
    enabled: false,
    retry: false,
  });
  const verification = verificationQuery.data;

  function applyFilters(event: React.FormEvent) {
    event.preventDefault();
    const normalizedEntityId = entityId.trim();
    const normalizedEventType = eventType.trim().toLowerCase();
    if (normalizedEntityId && !isUuid(normalizedEntityId)) {
      setFilterError('Entity reference must be a valid UUID.');
      return;
    }
    if (normalizedEventType.length > 40) {
      setFilterError('Event type must be 40 characters or fewer.');
      return;
    }
    setFilterError('');
    setAppliedEntityId(normalizedEntityId);
    setAppliedEventType(normalizedEventType);
    setPage(1);
  }

  return (
    <div className="space-y-4">
      <Card
        className={
          verification
            ? verification.valid
              ? 'border-emerald-500/30'
              : 'border-destructive/50'
            : ''
        }
      >
        <CardHeader>
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Fingerprint aria-hidden="true" className="h-5 w-5" />
                Tamper-evident history
              </CardTitle>
              <CardDescription>
                Verify the organization’s chained governance events and investigate every material
                change.
              </CardDescription>
            </div>
            <Button
              type="button"
              disabled={verificationQuery.isFetching}
              onClick={() => void verificationQuery.refetch()}
            >
              {verificationQuery.isFetching ? (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <ShieldCheck aria-hidden="true" className="mr-2 h-4 w-4" />
              )}
              Verify event chain
            </Button>
          </div>
        </CardHeader>
        {verificationQuery.isError && (
          <CardContent>
            <div
              role="alert"
              className="flex flex-col gap-3 rounded-md border border-destructive/30 p-4 sm:flex-row sm:items-center sm:justify-between"
            >
              <span className="text-sm text-destructive">
                {formatGovernanceError(
                  verificationQuery.error,
                  'Event-chain verification could not be completed.',
                )}
              </span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void verificationQuery.refetch()}
              >
                <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
                Retry
              </Button>
            </div>
          </CardContent>
        )}
        {verification && (
          <CardContent>
            <div
              role="status"
              className={`flex gap-3 rounded-md p-4 ${verification.valid ? 'bg-emerald-500/10' : 'bg-destructive/10'}`}
            >
              {verification.valid ? (
                <CheckCircle2
                  aria-hidden="true"
                  className="mt-0.5 h-5 w-5 shrink-0 text-emerald-700"
                />
              ) : (
                <ShieldX aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-destructive" />
              )}
              <div>
                <p className="font-medium">
                  {verification.valid ? 'Event chain verified' : 'Integrity verification failed'}
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {verification.event_count} events · last sequence {verification.last_sequence} ·
                  verified {formatGovernanceDate(verification.verified_at)}
                </p>
                {verification.failure_reason && (
                  <p className="mt-2 text-sm text-destructive">{verification.failure_reason}</p>
                )}
                <p className="mt-2 break-all font-mono text-xs text-muted-foreground">
                  Last hash: {verification.last_hash || 'None (empty chain)'}
                </p>
              </div>
            </div>
          </CardContent>
        )}
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Governance event ledger</CardTitle>
          <CardDescription>
            Filter by entity or event token. Hashes remain visible for independent evidence review.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-3 md:grid-cols-[13rem_minmax(0,1fr)_minmax(0,1fr)_auto]"
            onSubmit={applyFilters}
          >
            <Select
              value={entityType}
              onValueChange={(value) => {
                setEntityType(value);
                setPage(1);
              }}
            >
              <SelectTrigger aria-label="Filter history by entity type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All entity types</SelectItem>
                {(
                  [
                    'policy',
                    'retention_schedule',
                    'retention_assignment',
                    'retention_exception',
                    'legal_hold',
                  ] as const
                ).map((value) => (
                  <SelectItem key={value} value={value}>
                    {humanizeGovernanceToken(value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="relative">
              <Search
                aria-hidden="true"
                className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                aria-label="Filter history by entity UUID"
                aria-invalid={Boolean(filterError)}
                className="pl-9"
                placeholder="Entity UUID"
                value={entityId}
                onChange={(event) => setEntityId(event.target.value)}
              />
            </div>
            <Input
              aria-label="Filter history by event type"
              maxLength={40}
              placeholder="Event type, e.g. hold_released"
              value={eventType}
              onChange={(event) => setEventType(event.target.value.toLowerCase())}
            />
            <Button type="submit" variant="outline">
              Apply filters
            </Button>
          </form>
          {filterError && (
            <p role="alert" className="mt-2 text-sm text-destructive">
              {filterError}
            </p>
          )}
        </CardContent>
      </Card>

      {eventsQuery.isLoading ? (
        <GovernancePanelSkeleton label="Loading governance event history" />
      ) : eventsQuery.isError ? (
        <GovernanceErrorState
          message={formatGovernanceError(
            eventsQuery.error,
            'Governance history could not be loaded.',
          )}
          onRetry={() => void eventsQuery.refetch()}
        />
      ) : (eventsQuery.data?.data.length ?? 0) === 0 ? (
        <Card>
          <EmptyState
            icon={Fingerprint}
            title="No governance events"
            description="No events match the current filters."
          />
        </Card>
      ) : (
        <div className="space-y-3">
          {eventsQuery.data?.data.map((event) => (
            <Card key={event.id}>
              <CardContent className="p-4">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline">#{event.chain_sequence}</Badge>
                      <Badge>{humanizeGovernanceToken(event.event_type)}</Badge>
                      <span className="text-sm font-medium">
                        {humanizeGovernanceToken(event.entity_type)}
                      </span>
                    </div>
                    <p className="mt-2 text-sm">{event.reason}</p>
                    <dl className="mt-3 grid gap-2 text-xs text-muted-foreground sm:grid-cols-2">
                      <Detail label="Entity" value={event.entity_id} />
                      <Detail label="Actor" value={event.actor_user_id} />
                      <Detail label="Source" value={event.source} />
                      <Detail label="Created" value={formatGovernanceDate(event.created_at)} />
                      {event.request_id && <Detail label="Request" value={event.request_id} />}
                    </dl>
                  </div>
                  <div className="min-w-0 shrink-0 space-y-2 lg:max-w-sm">
                    <Hash label="Event hash" value={event.event_hash} />
                    <Hash label="Previous hash" value={event.previous_hash || 'Genesis'} />
                  </div>
                </div>
                {(event.before_state || event.after_state) && (
                  <details className="mt-4 rounded-md border p-3">
                    <summary className="cursor-pointer text-sm font-medium">
                      View audited state snapshot
                    </summary>
                    <div className="mt-3 grid gap-3 lg:grid-cols-2">
                      <Snapshot label="Before" value={event.before_state} />
                      <Snapshot label="After" value={event.after_state} />
                    </div>
                  </details>
                )}
              </CardContent>
            </Card>
          ))}
          {eventsQuery.data?.pagination && (
            <div className="flex flex-col gap-3 rounded-md border bg-card p-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-muted-foreground">
                Page {eventsQuery.data.pagination.page} of{' '}
                {Math.max(1, eventsQuery.data.pagination.total_pages)} ·{' '}
                {eventsQuery.data.pagination.total_items} events
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={page <= 1 || eventsQuery.isFetching}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  <ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />
                  Previous
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={
                    page >= eventsQuery.data.pagination.total_pages || eventsQuery.isFetching
                  }
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
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className="break-all font-mono text-foreground">{value}</dd>
    </div>
  );
}
function Hash({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <code className="mt-1 block break-all rounded bg-muted px-2 py-1 text-[11px]">{value}</code>
    </div>
  );
}

function Snapshot({ label, value }: { label: string; value?: Record<string, unknown> }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <pre className="mt-1 max-h-64 overflow-auto rounded bg-muted p-3 text-xs">
        {value ? JSON.stringify(value, null, 2) : 'No snapshot'}
      </pre>
    </div>
  );
}
