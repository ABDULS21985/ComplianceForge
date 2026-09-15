'use client';

import { Activity, ChevronLeft, ChevronRight, FileClock, Pencil, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmAction } from '@/components/settings/confirm-action';
import { format } from 'date-fns';
import { formatApiError } from '@/lib/enterprise-settings';
import type { Integration } from '@/types/enterprise-settings';
import { IntegrationEditorDialog } from '@/components/integrations/integration-editor-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from 'sonner';
import { useState } from 'react';

const LOG_PAGE_SIZE = 20;

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (status === 'active' || status === 'healthy' || status === 'completed') return 'default';
  if (status === 'error' || status === 'failed' || status === 'unhealthy') return 'destructive';
  return 'secondary';
}

function formatDate(value: string | null): string {
  if (!value) return 'Never';
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : format(date, 'PPp');
}

function SyncLogDialog({ integration, onOpenChange }: { integration: Integration; onOpenChange: (open: boolean) => void }) {
  const [page, setPage] = useState(1);
  const logsQuery = useQuery({
    queryKey: ['integrations', integration.id, 'logs', page, LOG_PAGE_SIZE],
    queryFn: () => api.integrations.logs(integration.id, { page, page_size: LOG_PAGE_SIZE }),
    staleTime: 10_000,
  });
  const pagination = logsQuery.data?.pagination;
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader><DialogTitle>Sync history · {integration.name}</DialogTitle><DialogDescription>Server-recorded synchronization attempts. A “started” record is not proof of completion.</DialogDescription></DialogHeader>
        {logsQuery.isLoading && <div role="status" aria-label="Loading synchronization history" className="space-y-3">{[0, 1, 2].map((item) => <Skeleton key={item} className="h-24" />)}</div>}
        {logsQuery.isError && <Card><CardContent className="space-y-3 py-8 text-center"><p role="alert">{formatApiError(logsQuery.error, 'Sync history could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void logsQuery.refetch()}>Try again</Button></CardContent></Card>}
        {logsQuery.isSuccess && logsQuery.data.data.length === 0 && <p className="py-10 text-center text-sm text-muted-foreground">No synchronization attempts have been recorded.</p>}
        <ol className="space-y-3">
          {logsQuery.data?.data.map((log) => (
            <li key={log.id} className="rounded-md border p-4">
              <div className="flex flex-wrap items-center justify-between gap-2"><div className="flex items-center gap-2"><Badge variant={statusVariant(log.status)}>{log.status}</Badge><span className="text-sm font-medium">{log.sync_type} sync</span></div><time className="text-xs text-muted-foreground">{formatDate(log.created_at)}</time></div>
              <dl className="mt-3 grid grid-cols-2 gap-2 text-sm sm:grid-cols-4"><div><dt className="text-muted-foreground">Processed</dt><dd>{log.records_processed}</dd></div><div><dt className="text-muted-foreground">Created</dt><dd>{log.records_created}</dd></div><div><dt className="text-muted-foreground">Updated</dt><dd>{log.records_updated}</dd></div><div><dt className="text-muted-foreground">Failed</dt><dd>{log.records_failed}</dd></div></dl>
              {log.duration_ms !== null && <p className="mt-2 text-xs text-muted-foreground">Duration: {log.duration_ms} ms</p>}
              {log.error_message && <p role="alert" className="mt-2 text-sm text-destructive">{log.error_message}</p>}
            </li>
          ))}
        </ol>
        {pagination && pagination.total_pages > 1 && <nav aria-label="Sync history pages" className="flex items-center justify-between"><span className="text-sm text-muted-foreground">Page {pagination.page} of {pagination.total_pages}</span><div className="flex gap-2"><Button type="button" size="sm" variant="outline" disabled={page === 1} onClick={() => setPage((value) => Math.max(1, value - 1))}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button><Button type="button" size="sm" variant="outline" disabled={page >= pagination.total_pages} onClick={() => setPage((value) => value + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button></div></nav>}
      </DialogContent>
    </Dialog>
  );
}

export function ConfiguredIntegrationsPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Integration | null>(null);
  const [logsFor, setLogsFor] = useState<Integration | null>(null);
  const integrationsQuery = useQuery({
    queryKey: ['integrations'],
    queryFn: () => api.integrations.list(),
    staleTime: 30_000,
  });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['integrations'] });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.integrations.remove(id),
    onSuccess: () => { void invalidate(); toast.success('Integration removed'); },
    onError: (error) => toast.error(formatApiError(error, 'The integration could not be deleted.')),
  });
  const testMutation = useMutation({
    mutationFn: (id: string) => api.integrations.test(id),
    onSuccess: (response) => { void invalidate(); toast.success(`${response.message}: ${response.status}`); },
    onError: (error) => toast.error(formatApiError(error, 'Stored configuration could not be validated.')),
  });
  const syncMutation = useMutation({
    mutationFn: (id: string) => api.integrations.sync(id, { sync_type: 'full' }),
    onSuccess: (response) => {
      void invalidate();
      void queryClient.invalidateQueries({ queryKey: ['integrations', response.data.integration_id, 'logs'] });
      toast.success('Sync request accepted and recorded. Check history for status.');
    },
    onError: (error) => toast.error(formatApiError(error, 'The sync request could not be recorded.')),
  });

  const integrations = integrationsQuery.data?.data ?? [];
  return (
    <section aria-labelledby="configured-integrations-heading" className="space-y-5">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start"><div><h2 id="configured-integrations-heading" className="text-xl font-semibold">Configured integrations</h2><p className="text-sm text-muted-foreground">Manage encrypted configuration, health state, synchronization requests, and history.</p></div><Button type="button" variant="outline" size="sm" onClick={() => void integrationsQuery.refetch()} disabled={integrationsQuery.isFetching}><RefreshCw aria-hidden="true" className={`mr-2 h-4 w-4 ${integrationsQuery.isFetching ? 'animate-spin' : ''}`} />Refresh</Button></div>
      <p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground"><ShieldCheck aria-hidden="true" className="mr-2 inline h-4 w-4" />“Validate configuration” currently verifies that stored encrypted configuration can be opened; it does not certify remote-provider connectivity.</p>
      {integrationsQuery.isLoading && <div role="status" aria-label="Loading integrations" className="space-y-3">{[0, 1, 2].map((item) => <Skeleton key={item} className="h-40" />)}</div>}
      {integrationsQuery.isError && <Card><CardContent className="space-y-3 py-8 text-center"><p role="alert">{formatApiError(integrationsQuery.error, 'Integrations could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void integrationsQuery.refetch()}>Try again</Button></CardContent></Card>}
      {integrationsQuery.isSuccess && integrations.length === 0 && <Card><CardContent className="py-12 text-center"><Activity aria-hidden="true" className="mx-auto mb-3 h-8 w-8 text-muted-foreground" /><p className="font-medium">No integrations configured</p><p className="text-sm text-muted-foreground">Choose a connector from the catalog to begin.</p></CardContent></Card>}
      <div className="grid gap-4">
        {integrations.map((integration) => (
          <Card key={integration.id}>
            <CardContent className="space-y-4 py-5">
              <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h3 className="text-lg font-semibold">{integration.name}</h3><Badge variant="outline">{integration.integration_type}</Badge><Badge variant={statusVariant(integration.status)}>{integration.status}</Badge><Badge variant={statusVariant(integration.health_status)}>Health: {integration.health_status}</Badge></div>{integration.description && <p className="mt-2 text-sm text-muted-foreground">{integration.description}</p>}</div>{canConfigure && <div className="flex flex-wrap gap-2"><ConfirmAction title={`Validate ${integration.name}?`} description="The current API validates that the encrypted configuration can be decrypted. A future connector implementation may contact the configured provider." actionLabel="Validate" onConfirm={() => testMutation.mutate(integration.id)} pending={testMutation.isPending && testMutation.variables === integration.id}><ShieldCheck aria-hidden="true" className="mr-2 h-4 w-4" />Validate configuration</ConfirmAction><ConfirmAction title={`Request a full sync for ${integration.name}?`} description="This records a full synchronization request. A started record does not guarantee that a worker has completed synchronization." actionLabel="Request sync" onConfirm={() => syncMutation.mutate(integration.id)} pending={syncMutation.isPending && syncMutation.variables === integration.id} disabled={integration.status !== 'active'}><RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />Sync</ConfirmAction></div>}</div>
              <dl className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4"><div><dt className="text-muted-foreground">Last health check</dt><dd>{formatDate(integration.last_health_check_at)}</dd></div><div><dt className="text-muted-foreground">Last sync</dt><dd>{formatDate(integration.last_sync_at)}</dd></div><div><dt className="text-muted-foreground">Schedule</dt><dd>{integration.sync_frequency_minutes ? `Every ${integration.sync_frequency_minutes} minutes` : 'Manual only'}</dd></div><div><dt className="text-muted-foreground">Errors</dt><dd>{integration.error_count}</dd></div></dl>
              {integration.last_error_message && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{integration.last_error_message}</p>}
              {integration.capabilities.length > 0 && <div className="flex flex-wrap gap-1">{integration.capabilities.map((capability) => <Badge key={capability} variant="secondary" className="font-normal">{capability}</Badge>)}</div>}
              <div className="flex flex-wrap gap-2 border-t pt-4"><Button type="button" size="sm" variant="outline" onClick={() => setLogsFor(integration)}><FileClock aria-hidden="true" className="mr-2 h-4 w-4" />History</Button>{canConfigure && <><Button type="button" size="sm" variant="outline" onClick={() => setEditing(integration)}><Pencil aria-hidden="true" className="mr-2 h-4 w-4" />Edit</Button><ConfirmAction title={`Remove ${integration.name}?`} description="The integration is removed from active views, but the API retains its encrypted record and synchronization history for audit purposes." actionLabel="Remove integration" onConfirm={() => deleteMutation.mutate(integration.id)} pending={deleteMutation.isPending && deleteMutation.variables === integration.id} triggerVariant="destructive"><Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />Remove</ConfirmAction></>}</div>
            </CardContent>
          </Card>
        ))}
      </div>
      {editing && <IntegrationEditorDialog key={editing.id} integration={editing} open onOpenChange={(open) => !open && setEditing(null)} />}
      {logsFor && <SyncLogDialog key={logsFor.id} integration={logsFor} onOpenChange={(open) => !open && setLogsFor(null)} />}
    </section>
  );
}
