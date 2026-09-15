'use client';

import * as React from 'react';
import { AlertTriangle, CheckCircle2, Gauge, Infinity as InfinityIcon, RefreshCw } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  entitlementMetrics,
  featureFlagKeys,
  formatEntitlementAmount,
  formatFeatureFlagError,
  humanizeFeatureToken,
} from '@/lib/feature-flags';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useMutation, useQuery } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import Link from 'next/link';
import { Skeleton } from '@/components/ui/skeleton';

const GIBIBYTE = 1024 ** 3;

export function EntitlementsPanel() {
  const query = useQuery({
    queryKey: featureFlagKeys.entitlements,
    queryFn: () => api.featureFlags.entitlements(),
    staleTime: 60_000,
  });
  const [selectedMetric, setSelectedMetric] = React.useState('');
  const [requested, setRequested] = React.useState('1');
  const [validationError, setValidationError] = React.useState('');
  const metrics = query.data ? entitlementMetrics(query.data) : [];
  const effectiveMetric = selectedMetric || metrics[0]?.metric || '';
  const check = useMutation({
    mutationFn: ({ metric, amount }: { amount: number; metric: string }) =>
      api.featureFlags.checkLimit(metric, amount),
  });

  async function checkCapacity(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = Number(requested);
    const amount = effectiveMetric === 'storage_bytes' ? Math.round(value * GIBIBYTE) : Math.trunc(value);
    if (!effectiveMetric || !Number.isFinite(value) || value <= 0 || amount < 1) {
      setValidationError('Enter a positive amount to check.');
      return;
    }
    setValidationError('');
    await check.mutateAsync({ metric: effectiveMetric, amount }).catch(() => undefined);
  }

  if (query.isLoading) return <div role="status" aria-label="Loading subscription entitlements" className="space-y-4"><Skeleton className="h-36" /><div className="grid gap-4 md:grid-cols-2"><Skeleton className="h-64" /><Skeleton className="h-64" /></div></div>;
  if (query.isError || !query.data) return <Card><CardContent className="space-y-3 py-12 text-center"><RefreshCw aria-hidden="true" className="mx-auto h-9 w-9 text-muted-foreground" /><p role="alert">{formatFeatureFlagError(query.error, 'Subscription entitlements could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void query.refetch()}>Try again</Button></CardContent></Card>;

  const snapshot = query.data;
  const enabledFeatures = Object.entries(snapshot.features).filter(([, enabled]) => enabled);
  const unavailableFeatures = Object.entries(snapshot.features).filter(([, enabled]) => !enabled);
  const constrained = metrics.some((metric) => metric.limit > 0 && metric.usage >= metric.limit * 0.8);
  return (
    <div className="space-y-5">
      <Card>
        <CardHeader className="flex-row flex-wrap items-start justify-between gap-4">
          <div><CardTitle>{snapshot.plan_name || humanizeFeatureToken(snapshot.tier)} plan</CardTitle><CardDescription>Authoritative server snapshot evaluated {formatDateTime(snapshot.evaluated_at)}.</CardDescription></div>
          <div className="flex flex-wrap gap-2"><Badge variant="outline">{humanizeFeatureToken(snapshot.subscription_status || 'tier fallback')}</Badge><Badge>{humanizeFeatureToken(snapshot.tier)}</Badge></div>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-3">
          <Summary label="Entitlement source" value={humanizeFeatureToken(snapshot.source)} />
          <Summary label="Enabled plan features" value={String(enabledFeatures.length)} />
          <Summary label="Metered resources" value={String(metrics.length)} />
        </CardContent>
      </Card>

      {constrained && <Card className="border-amber-500/40 bg-amber-500/5"><CardContent className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center sm:justify-between"><div className="flex gap-2 text-sm"><AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" /><span>One or more resources have reached 80% of the current plan allowance.</span></div><Button asChild size="sm" variant="outline"><Link href="/settings/subscription">Review plan options</Link></Button></CardContent></Card>}

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader><CardTitle className="text-lg">Usage and limits</CardTitle><CardDescription>A zero limit is contractually unlimited. Server checks remain authoritative during creation.</CardDescription></CardHeader>
          <CardContent className="space-y-5">
            {metrics.map((metric) => {
              const unlimited = metric.limit === 0;
              const percent = unlimited ? 0 : Math.min(100, (metric.usage / metric.limit) * 100);
              return <div key={metric.metric} className="space-y-2"><div className="flex items-center justify-between gap-3 text-sm"><span className="font-medium">{humanizeFeatureToken(metric.metric)}</span><span className="text-muted-foreground">{formatEntitlementAmount(metric.metric, metric.usage)} / {unlimited ? 'Unlimited' : formatEntitlementAmount(metric.metric, metric.limit)}</span></div>{unlimited ? <div className="flex items-center gap-2 text-xs text-emerald-700"><InfinityIcon aria-hidden="true" className="h-4 w-4" />No plan ceiling</div> : <div role="progressbar" aria-label={`${humanizeFeatureToken(metric.metric)} usage`} aria-valuemin={0} aria-valuemax={metric.limit} aria-valuenow={Math.min(metric.usage, metric.limit)} className="h-2 overflow-hidden rounded-full bg-muted"><div className={`h-full ${percent >= 100 ? 'bg-destructive' : percent >= 80 ? 'bg-amber-500' : 'bg-primary'}`} style={{ width: `${percent}%` }} /></div>}</div>;
            })}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-lg">Capacity preflight</CardTitle><CardDescription>Advisory only. The server repeats each quota check transactionally when creating a resource.</CardDescription></CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={(event) => void checkCapacity(event)} noValidate>
              <div className="space-y-2"><Label htmlFor="entitlement-metric">Resource metric</Label><Select value={effectiveMetric} onValueChange={(value) => { setSelectedMetric(value); setRequested('1'); check.reset(); }}><SelectTrigger id="entitlement-metric"><SelectValue placeholder="Choose a metric" /></SelectTrigger><SelectContent>{metrics.map((metric) => <SelectItem key={metric.metric} value={metric.metric}>{humanizeFeatureToken(metric.metric)}</SelectItem>)}</SelectContent></Select></div>
              <div className="space-y-2"><Label htmlFor="entitlement-requested">Additional {effectiveMetric === 'storage_bytes' ? 'GiB' : 'units'}</Label><Input id="entitlement-requested" type="number" min={effectiveMetric === 'storage_bytes' ? 0.1 : 1} step={effectiveMetric === 'storage_bytes' ? 0.1 : 1} value={requested} onChange={(event) => setRequested(event.target.value)} /></div>
              {(validationError || check.error) && <p role="alert" className="text-sm text-destructive">{validationError || formatFeatureFlagError(check.error, 'Capacity could not be checked.')}</p>}
              {check.data && <div role="status" className={`rounded-md p-3 text-sm ${check.data.allowed ? 'bg-emerald-500/10 text-emerald-800 dark:text-emerald-200' : 'bg-amber-500/10 text-amber-900 dark:text-amber-100'}`}><div className="flex gap-2">{check.data.allowed ? <CheckCircle2 aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" /> : <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />}<span>{check.data.allowed ? 'Capacity is available.' : 'This request exceeds the current allowance.'} {check.data.limit === 0 ? 'The resource is unlimited.' : `${formatEntitlementAmount(check.data.metric, check.data.remaining)} remains before this request.`}</span></div></div>}
              <Button type="submit" disabled={check.isPending || !effectiveMetric}><Gauge aria-hidden="true" className="mr-2 h-4 w-4" />{check.isPending ? 'Checking…' : 'Check capacity'}</Button>
            </form>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-lg">Plan features</CardTitle><CardDescription>Plan features are evaluated before tenant overrides and cannot be bypassed.</CardDescription></CardHeader>
        <CardContent className="grid gap-5 md:grid-cols-2">
          <section aria-labelledby="included-features"><h3 id="included-features" className="font-medium text-emerald-700">Included ({enabledFeatures.length})</h3><ul className="mt-3 flex flex-wrap gap-2">{enabledFeatures.length === 0 ? <li className="text-sm text-muted-foreground">No feature claims published.</li> : enabledFeatures.map(([feature]) => <li key={feature}><Badge variant="secondary">{humanizeFeatureToken(feature)}</Badge></li>)}</ul></section>
          <section aria-labelledby="unavailable-features"><h3 id="unavailable-features" className="font-medium">Not included ({unavailableFeatures.length})</h3><ul className="mt-3 flex flex-wrap gap-2">{unavailableFeatures.length === 0 ? <li className="text-sm text-muted-foreground">All published plan features are included.</li> : unavailableFeatures.map(([feature]) => <li key={feature}><Badge variant="outline">{humanizeFeatureToken(feature)}</Badge></li>)}</ul></section>
        </CardContent>
      </Card>
    </div>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  return <div className="rounded-md bg-muted p-3"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 font-semibold">{value}</p></div>;
}

function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
