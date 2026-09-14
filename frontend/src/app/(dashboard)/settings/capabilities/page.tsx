'use client';

import * as React from 'react';
import { AlertTriangle, Clock, History, LockKeyhole, RefreshCw, RotateCcw, Search, Settings2, ShieldCheck } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { EntitlementsPanel } from '@/components/feature-flags/entitlements-panel';
import { FeatureFlagEditorDialog } from '@/components/feature-flags/feature-flag-editor-dialog';
import { FeatureFlagHistoryDialog } from '@/components/feature-flags/feature-flag-history-dialog';
import { FeatureFlagResetDialog } from '@/components/feature-flags/feature-flag-reset-dialog';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  featureEvaluationExplanation,
  featureFlagKeys,
  formatFeatureFlagError,
  humanizeFeatureToken,
  overrideWindowState,
  rolloutPercent,
} from '@/lib/feature-flags';
import { Input } from '@/components/ui/input';
import { useCapabilityPermissions } from '@/hooks/use-capability-permissions';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import type { CapabilityMaturity, FeatureFlagEvaluation } from '@/types/feature-flag';

type CapabilityFilter = 'all' | 'enabled' | 'disabled' | 'overridden' | 'scheduled';

export default function CapabilitiesPage() {
  const permission = useCapabilityPermissions();
  const [search, setSearch] = React.useState('');
  const deferredSearch = React.useDeferredValue(search.trim().toLowerCase());
  const [status, setStatus] = React.useState<CapabilityFilter>('all');
  const [maturity, setMaturity] = React.useState<'all' | CapabilityMaturity>('all');
  const [editing, setEditing] = React.useState<FeatureFlagEvaluation | null>(null);
  const [resetting, setResetting] = React.useState<FeatureFlagEvaluation | null>(null);
  const [history, setHistory] = React.useState<FeatureFlagEvaluation | null>(null);
  const query = useQuery({
    queryKey: featureFlagKeys.capabilities,
    queryFn: () => api.featureFlags.listCapabilities(),
    enabled: permission.canRead,
    staleTime: 30_000,
  });
  const evaluations = query.data?.data ?? [];
  const filtered = evaluations.filter((evaluation) => {
    const haystack = `${evaluation.capability.display_name} ${evaluation.capability.key} ${evaluation.capability.description} ${evaluation.capability.owner_team}`.toLowerCase();
    const window = overrideWindowState(evaluation);
    return (!deferredSearch || haystack.includes(deferredSearch))
      && (maturity === 'all' || evaluation.capability.maturity === maturity)
      && (status === 'all'
        || (status === 'enabled' && evaluation.enabled)
        || (status === 'disabled' && !evaluation.enabled)
        || (status === 'overridden' && Boolean(evaluation.override))
        || (status === 'scheduled' && window === 'scheduled'));
  });

  if (permission.isLoading) return <PageSkeleton />;
  if (permission.isError) return <ErrorState message="Your settings permissions could not be loaded." onRetry={() => void permission.retry()} />;
  if (!permission.canRead) return <Card className="mx-auto max-w-xl"><CardContent className="flex flex-col items-center gap-3 py-12 text-center"><LockKeyhole aria-hidden="true" className="h-9 w-9 text-muted-foreground" /><h1 className="text-xl font-semibold">Capability settings unavailable</h1><p className="text-sm text-muted-foreground">Your role does not grant organization settings access.</p></CardContent></Card>;

  const enabledCount = evaluations.filter((item) => item.enabled).length;
  const overriddenCount = evaluations.filter((item) => item.override).length;
  const subscriptionBlocked = evaluations.filter((item) => item.evaluation_reason === 'subscription_denied').length;
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Capabilities and entitlements</h1>
        <p className="mt-1 max-w-3xl text-muted-foreground">Understand what is available, why a capability is gated, how much plan capacity remains, and which audited tenant overrides are active.</p>
      </div>
      {!permission.canConfigure && <Card><CardContent className="flex gap-3 py-4 text-sm text-muted-foreground"><LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />You have read-only settings access. Override and reset controls are hidden.</CardContent></Card>}
      <div className="grid gap-4 sm:grid-cols-3"><Metric label="Enabled now" value={enabledCount} /><Metric label="Tenant overrides" value={overriddenCount} /><Metric label="Plan upgrades needed" value={subscriptionBlocked} /></div>

      <Tabs defaultValue="catalogue" className="space-y-5">
        <TabsList className="h-auto max-w-full flex-wrap justify-start"><TabsTrigger value="catalogue">Capability catalogue</TabsTrigger><TabsTrigger value="entitlements">Plan usage and entitlements</TabsTrigger></TabsList>
        <TabsContent value="catalogue" className="space-y-4">
          <Card><CardContent className="grid gap-3 pt-6 md:grid-cols-[minmax(0,1fr)_12rem_12rem]"><div className="relative"><Search aria-hidden="true" className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="Search capabilities" className="pl-9" placeholder="Search capability, key, owner, or description" value={search} onChange={(event) => setSearch(event.target.value)} /></div><Select value={status} onValueChange={(value) => setStatus(value as CapabilityFilter)}><SelectTrigger aria-label="Filter by capability status"><SelectValue /></SelectTrigger><SelectContent>{(['all', 'enabled', 'disabled', 'overridden', 'scheduled'] as const).map((value) => <SelectItem key={value} value={value}>{humanizeFeatureToken(value)}</SelectItem>)}</SelectContent></Select><Select value={maturity} onValueChange={(value) => setMaturity(value as 'all' | CapabilityMaturity)}><SelectTrigger aria-label="Filter by maturity"><SelectValue /></SelectTrigger><SelectContent>{(['all', 'experimental', 'beta', 'general_availability', 'deprecated'] as const).map((value) => <SelectItem key={value} value={value}>{humanizeFeatureToken(value)}</SelectItem>)}</SelectContent></Select></CardContent></Card>
          {query.isLoading ? <CatalogueSkeleton /> : query.isError ? <ErrorState message={formatFeatureFlagError(query.error, 'The capability catalogue could not be loaded.')} onRetry={() => void query.refetch()} /> : filtered.length === 0 ? <Card><CardContent className="py-14 text-center"><ShieldCheck aria-hidden="true" className="mx-auto h-10 w-10 text-muted-foreground" /><h2 className="mt-3 text-lg font-semibold">No matching capabilities</h2><p className="mt-1 text-sm text-muted-foreground">Adjust the search, evaluation status, or maturity filter.</p></CardContent></Card> : (
            <div className="grid gap-4 xl:grid-cols-2">{filtered.map((evaluation) => <CapabilityCard key={evaluation.capability.key} canConfigure={permission.canConfigure} evaluation={evaluation} onEdit={() => setEditing(evaluation)} onHistory={() => setHistory(evaluation)} onReset={() => setResetting(evaluation)} />)}</div>
          )}
        </TabsContent>
        <TabsContent value="entitlements"><EntitlementsPanel /></TabsContent>
      </Tabs>

      {editing && <FeatureFlagEditorDialog evaluation={editing} open onOpenChange={(open) => !open && setEditing(null)} onReload={() => { setEditing(null); void query.refetch(); }} onSaved={() => void query.refetch()} />}
      {resetting && <FeatureFlagResetDialog evaluation={resetting} open onOpenChange={(open) => !open && setResetting(null)} onReload={() => { setResetting(null); void query.refetch(); }} onReset={() => void query.refetch()} />}
      {history && <FeatureFlagHistoryDialog evaluation={history} open onOpenChange={(open) => !open && setHistory(null)} />}
    </div>
  );
}

function CapabilityCard({ canConfigure, evaluation, onEdit, onHistory, onReset }: { canConfigure: boolean; evaluation: FeatureFlagEvaluation; onEdit: () => void; onHistory: () => void; onReset: () => void }) {
  const capability = evaluation.capability;
  const window = overrideWindowState(evaluation);
  const constrained = capability.kill_switch || !evaluation.entitled || Boolean(evaluation.blocking_capability);
  return (
    <Card className={evaluation.enabled ? 'border-emerald-500/30' : ''}>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3"><div className="min-w-0"><CardTitle className="text-lg">{capability.display_name}</CardTitle><p className="mt-1 break-all font-mono text-xs text-muted-foreground">{capability.key}</p></div><div className="flex flex-wrap gap-2"><Badge variant={evaluation.enabled ? 'default' : 'secondary'}>{evaluation.enabled ? 'Enabled' : 'Disabled'}</Badge><Badge variant="outline">{humanizeFeatureToken(capability.maturity)}</Badge>{evaluation.override && <Badge variant="outline">Override v{evaluation.override.version}</Badge>}</div></div>
        <CardDescription className="pt-2">{capability.description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className={`rounded-md p-3 text-sm ${constrained ? 'bg-amber-500/10' : 'bg-muted'}`}><div className="flex gap-2">{constrained && <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" />}<span>{featureEvaluationExplanation(evaluation)}</span></div></div>
        <dl className="grid gap-3 text-sm sm:grid-cols-2"><Detail label="Owner" value={capability.owner_team} /><Detail label="Minimum tier" value={humanizeFeatureToken(capability.minimum_tier)} /><Detail label="Effective rollout" value={`${rolloutPercent(evaluation.effective_rollout_basis_points)}%`} /><Detail label="Evaluation source" value={humanizeFeatureToken(evaluation.source)} /></dl>
        {capability.required_plan_feature && <div className="text-sm"><span className="text-muted-foreground">Plan feature: </span><code className="rounded bg-muted px-1.5 py-0.5 text-xs">{capability.required_plan_feature}</code></div>}
        {capability.prerequisites.length > 0 && <div className="text-sm"><span className="text-muted-foreground">Prerequisites: </span><span className="inline-flex flex-wrap gap-1">{capability.prerequisites.map((item) => <Badge key={item} variant="secondary" className="font-mono font-normal">{item}</Badge>)}</span></div>}
        {capability.kill_switch && <p className="flex gap-2 text-sm font-medium text-destructive"><LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />Platform kill switch is active and cannot be bypassed.</p>}
        {evaluation.override && <div className="rounded-md border p-3 text-sm"><div className="flex flex-wrap items-center gap-2"><Clock aria-hidden="true" className="h-4 w-4" /><span className="font-medium">Tenant override · {humanizeFeatureToken(window)}</span></div><p className="mt-1 text-muted-foreground">{evaluation.override.reason}</p>{evaluation.override.starts_at && <p className="mt-1 text-xs text-muted-foreground">Starts {formatDateTime(evaluation.override.starts_at)}</p>}{evaluation.override.expires_at && <p className="text-xs text-muted-foreground">Expires {formatDateTime(evaluation.override.expires_at)}</p>}</div>}
        <div className="flex flex-wrap gap-2 border-t pt-4"><Button type="button" size="sm" variant="outline" onClick={onHistory}><History aria-hidden="true" className="mr-2 h-4 w-4" />History</Button>{canConfigure && <Button type="button" size="sm" onClick={onEdit}><Settings2 aria-hidden="true" className="mr-2 h-4 w-4" />{evaluation.override ? 'Edit override' : 'Create override'}</Button>}{canConfigure && evaluation.override && <Button type="button" size="sm" variant="ghost" onClick={onReset}><RotateCcw aria-hidden="true" className="mr-2 h-4 w-4" />Reset</Button>}</div>
      </CardContent>
    </Card>
  );
}

function Detail({ label, value }: { label: string; value: string }) { return <div><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-0.5 font-medium">{value}</dd></div>; }
function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) { return <Card><CardContent className="space-y-3 py-12 text-center"><RefreshCw aria-hidden="true" className="mx-auto h-9 w-9 text-muted-foreground" /><p role="alert">{message}</p><Button type="button" variant="outline" onClick={onRetry}>Try again</Button></CardContent></Card>; }
function Metric({ label, value }: { label: string; value: number }) { return <Card><CardContent className="pt-6"><p className="text-sm text-muted-foreground">{label}</p><p className="mt-1 text-2xl font-semibold">{value}</p></CardContent></Card>; }
function PageSkeleton() { return <div role="status" aria-label="Loading capability settings" className="space-y-4"><Skeleton className="h-24" /><div className="grid gap-4 sm:grid-cols-3">{Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-24" />)}</div><CatalogueSkeleton /></div>; }
function CatalogueSkeleton() { return <div role="status" aria-label="Loading capability catalogue" className="grid gap-4 xl:grid-cols-2">{Array.from({ length: 6 }).map((_, index) => <Skeleton key={index} className="h-80" />)}</div>; }
function formatDateTime(value: string): string { return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)); }
