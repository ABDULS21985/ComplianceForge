'use client';

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ChevronLeft, ChevronRight, History } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  featureFlagKeys,
  formatFeatureFlagError,
  humanizeFeatureToken,
} from '@/lib/feature-flags';
import api from '@/lib/api';
import { Skeleton } from '@/components/ui/skeleton';
import type { FeatureFlagEvaluation } from '@/types/feature-flag';

const PAGE_SIZE = 20;

export function FeatureFlagHistoryDialog({
  evaluation,
  onOpenChange,
  open,
}: {
  evaluation: FeatureFlagEvaluation;
  onOpenChange: (open: boolean) => void;
  open: boolean;
}) {
  const [page, setPage] = React.useState(1);
  const query = useQuery({
    queryKey: featureFlagKeys.history(evaluation.capability.key, page),
    queryFn: () => api.featureFlags.history(evaluation.capability.key, { page, page_size: PAGE_SIZE }),
    enabled: open,
  });
  const pagination = query.data?.pagination;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader><DialogTitle>{evaluation.capability.display_name} history</DialogTitle><DialogDescription>Immutable tenant override events, newest first. Catalogue changes are managed through deployment governance.</DialogDescription></DialogHeader>
        {query.isLoading ? <div role="status" aria-label="Loading feature flag history" className="space-y-3">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-24" />)}</div> : query.isError ? (
          <div className="space-y-3 py-8 text-center"><p role="alert" className="text-sm text-destructive">{formatFeatureFlagError(query.error, 'History could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void query.refetch()}>Try again</Button></div>
        ) : (query.data?.data.length ?? 0) === 0 ? (
          <div className="flex flex-col items-center py-10 text-center"><History aria-hidden="true" className="h-9 w-9 text-muted-foreground" /><p className="mt-3 font-semibold">No tenant override events</p><p className="mt-1 text-sm text-muted-foreground">The catalogue default has never been overridden for this organization.</p></div>
        ) : (
          <ol className="relative ml-2 space-y-6 border-l pl-6">
            {query.data?.data.map((event) => (
              <li key={event.id} className="relative">
                <span aria-hidden="true" className="absolute -left-[1.93rem] top-1 h-3 w-3 rounded-full border-2 border-background bg-primary" />
                <div className="flex flex-wrap items-center gap-2"><Badge variant="outline">{humanizeFeatureToken(event.event_type)}</Badge><span className="text-xs text-muted-foreground">Override v{event.override_version}</span></div>
                <p className="mt-2 text-sm">{event.reason}</p>
                <p className="mt-1 break-all text-xs text-muted-foreground">Actor: {event.actor_user_id} · {formatDateTime(event.created_at)}</p>
                {event.request_id && <p className="mt-1 break-all font-mono text-xs text-muted-foreground">Request: {event.request_id}</p>}
              </li>
            ))}
          </ol>
        )}
        <DialogFooter className="items-center sm:justify-between">
          <span className="text-sm text-muted-foreground">Page {pagination?.page ?? page} of {Math.max(1, pagination?.total_pages ?? 1)}</span>
          <div className="flex gap-2"><Button type="button" size="sm" variant="outline" disabled={page <= 1 || query.isFetching} onClick={() => setPage((current) => Math.max(1, current - 1))}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button><Button type="button" size="sm" variant="outline" disabled={!pagination || page >= pagination.total_pages || query.isFetching} onClick={() => setPage((current) => current + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button></div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
