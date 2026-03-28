'use client';

import * as React from 'react';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Loader2,
  Play,
  Eye,
  RefreshCw,
  Clock,
  TrendingUp,
  Shield,
} from 'lucide-react';

import { cn, formatDate, formatDateTime } from '@/lib/utils';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import api from '@/lib/api';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getSeverityColor(severity: string): string {
  switch (severity) {
    case 'critical': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'high': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
    case 'medium': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'low': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function getDriftTypeColor(type: string): string {
  switch (type) {
    case 'configuration': return 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400';
    case 'policy': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'compliance': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'evidence': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function getDriftStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'acknowledged': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'resolved': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function getHealthColor(health: string | undefined): { bg: string; border: string; text: string } {
  switch (health) {
    case 'healthy': return { bg: 'bg-green-500', border: 'border-green-500', text: 'text-green-600 dark:text-green-400' };
    case 'degraded': return { bg: 'bg-amber-500', border: 'border-amber-500', text: 'text-amber-600 dark:text-amber-400' };
    case 'critical': return { bg: 'bg-red-500', border: 'border-red-500', text: 'text-red-600 dark:text-red-400' };
    default: return { bg: 'bg-gray-400', border: 'border-gray-400', text: 'text-gray-600 dark:text-gray-400' };
  }
}

function getCollectionStatusColor(status: string): string {
  switch (status) {
    case 'success': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'failure': case 'error': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'running': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'pending': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function MonitoringPage() {
  const qc = useQueryClient();

  // Fetch dashboard
  const { data: dashData, isLoading: dashLoading } = useQuery({
    queryKey: ['monitoring', 'dashboard'],
    queryFn: () => api.monitoring.dashboard(),
    staleTime: 15 * 1000,
    refetchOnWindowFocus: true,
    refetchInterval: 30_000,
  });
  const dashboard = (dashData ?? {}) as any;

  // Fetch drift events
  const { data: driftData, isLoading: driftLoading, isError: driftError } = useQuery({
    queryKey: ['monitoring', 'drift'],
    queryFn: () => api.monitoring.listDrift({ status: 'active' }),
    refetchOnWindowFocus: true,
  });
  const driftEvents: Record<string, unknown>[] =
    (Array.isArray(driftData) ? driftData : (driftData as any)?.items) as any[] ?? [];

  // Fetch evidence collection configs
  const { data: configsData, isLoading: configsLoading } = useQuery({
    queryKey: ['monitoring', 'configs'],
    queryFn: () => api.monitoring.listConfigs(),
    refetchOnWindowFocus: true,
  });
  const configs: Record<string, unknown>[] =
    (Array.isArray(configsData) ? configsData : (configsData as any)?.items) as any[] ?? [];

  // Fetch monitors
  const { data: monitorsData, isLoading: monitorsLoading } = useQuery({
    queryKey: ['monitoring', 'monitors'],
    queryFn: () => api.monitoring.listMonitors(),
    refetchOnWindowFocus: true,
  });
  const monitors: Record<string, unknown>[] =
    (Array.isArray(monitorsData) ? monitorsData : (monitorsData as any)?.items) as any[] ?? [];

  // Mutations
  const acknowledgeDrift = useMutation({
    mutationFn: (driftId: string) => api.monitoring.acknowledgeDrift(driftId),
    onSuccess: () => {
      toast.success('Drift event acknowledged.');
      qc.invalidateQueries({ queryKey: ['monitoring'] });
    },
    onError: () => toast.error('Failed to acknowledge drift event.'),
  });

  const resolveDrift = useMutation({
    mutationFn: ({ id, data }: { id: string; data: unknown }) => api.monitoring.resolveDrift(id, data),
    onSuccess: () => {
      toast.success('Drift event resolved.');
      qc.invalidateQueries({ queryKey: ['monitoring'] });
    },
    onError: () => toast.error('Failed to resolve drift event.'),
  });

  const runNow = useMutation({
    mutationFn: (configId: string) => api.monitoring.runNow(configId),
    onSuccess: () => {
      toast.success('Collection triggered.');
      qc.invalidateQueries({ queryKey: ['monitoring', 'configs'] });
    },
    onError: () => toast.error('Failed to trigger collection.'),
  });

  // Dashboard stats
  const overallHealth = dashboard.overall_health as string;
  const healthColors = getHealthColor(overallHealth);
  const activeDriftCount = (dashboard.active_drift_events as number) ?? 0;
  const severityBreakdown = (dashboard.severity_breakdown ?? {}) as Record<string, number>;
  const collectionRate24h = (dashboard.collection_success_rate_24h as number) ?? 0;
  const collectionRate7d = (dashboard.collection_success_rate_7d as number) ?? 0;
  const monitorsPassing = (dashboard.monitors_passing as number) ?? 0;
  const monitorsFailing = (dashboard.monitors_failing as number) ?? 0;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Continuous Monitoring</h1>
        <p className="text-muted-foreground">
          Real-time compliance monitoring, drift detection, and automated evidence collection.
        </p>
      </div>

      {/* Overall Health + Stats Row */}
      <div className="grid gap-4 md:grid-cols-5">
        {/* Health Indicator */}
        <Card className="md:col-span-1">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Overall Health</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center justify-center py-4">
            {dashLoading ? (
              <Skeleton className="h-20 w-20 rounded-full" />
            ) : (
              <>
                <div className={cn(
                  'h-20 w-20 rounded-full border-4 flex items-center justify-center',
                  healthColors.border,
                )}>
                  <div className={cn('h-14 w-14 rounded-full', healthColors.bg)} />
                </div>
                <p className={cn('mt-2 text-sm font-bold capitalize', healthColors.text)}>
                  {overallHealth ?? 'Unknown'}
                </p>
              </>
            )}
          </CardContent>
        </Card>

        {/* Active Drift Events */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Active Drift Events</CardTitle>
            <AlertTriangle className="h-4 w-4 text-amber-500" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <>
                <div className={cn(
                  'text-2xl font-bold',
                  activeDriftCount > 0 ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400',
                )}>
                  {activeDriftCount}
                </div>
                <div className="mt-1 space-y-0.5">
                  {Object.entries(severityBreakdown).map(([sev, count]) => (
                    <div key={sev} className="flex items-center justify-between text-xs">
                      <span className="capitalize">{sev}</span>
                      <span className="font-semibold">{count}</span>
                    </div>
                  ))}
                </div>
              </>
            )}
          </CardContent>
        </Card>

        {/* Collection Success Rate */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Collection Rate</CardTitle>
            <TrendingUp className="h-4 w-4 text-blue-500" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <>
                <div className="text-2xl font-bold">{collectionRate24h.toFixed(1)}%</div>
                <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
                  <span>24h</span>
                  <Separator orientation="vertical" className="h-3" />
                  <span>7d: {collectionRate7d.toFixed(1)}%</span>
                </div>
              </>
            )}
          </CardContent>
        </Card>

        {/* Monitors Passing */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Monitors Passing</CardTitle>
            <CheckCircle2 className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <div className="text-2xl font-bold text-green-600 dark:text-green-400">
                {monitorsPassing}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Monitors Failing */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Monitors Failing</CardTitle>
            <XCircle className="h-4 w-4 text-red-500" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <div className={cn(
                'text-2xl font-bold',
                monitorsFailing > 0 ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400',
              )}>
                {monitorsFailing}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Drift Events */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5" />
            Active Drift Events
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {driftLoading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : driftError ? (
            <div className="p-6 text-center text-destructive">Failed to load drift events.</div>
          ) : driftEvents.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              <CheckCircle2 className="mx-auto mb-3 h-8 w-8 text-green-500" />
              <p className="text-sm font-medium">No active drift events</p>
              <p className="text-xs">All monitored configurations are within expected parameters.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Type</th>
                    <th className="px-4 py-3 text-left font-medium">Severity</th>
                    <th className="px-4 py-3 text-left font-medium">Entity</th>
                    <th className="px-4 py-3 text-left font-medium">Description</th>
                    <th className="px-4 py-3 text-left font-medium">Detected</th>
                    <th className="px-4 py-3 text-left font-medium">Status</th>
                    <th className="px-4 py-3 text-left font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {driftEvents.map((drift: any) => (
                    <tr key={drift.id as string} className="border-b hover:bg-muted/50">
                      <td className="px-4 py-3">
                        <Badge className={getDriftTypeColor(drift.drift_type as string)}>
                          {(drift.drift_type as string)?.replace('_', ' ')}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        <Badge className={getSeverityColor(drift.severity as string)}>
                          {drift.severity as string}
                        </Badge>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                        {drift.entity_ref as string ?? '—'}
                      </td>
                      <td className="px-4 py-3 max-w-xs truncate">{drift.description as string}</td>
                      <td className="px-4 py-3">{formatDate(drift.detected_at as string)}</td>
                      <td className="px-4 py-3">
                        <Badge className={getDriftStatusColor(drift.status as string)}>
                          {(drift.status as string)?.replace('_', ' ')}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1">
                          {(drift.status as string) === 'active' && (
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={acknowledgeDrift.isPending}
                              onClick={() => acknowledgeDrift.mutate(drift.id as string)}
                            >
                              <Eye className="mr-1 h-3 w-3" />
                              Ack
                            </Button>
                          )}
                          {((drift.status as string) === 'active' || (drift.status as string) === 'acknowledged') && (
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={resolveDrift.isPending}
                              onClick={() => resolveDrift.mutate({ id: drift.id as string, data: { resolution: 'manual' } })}
                            >
                              <CheckCircle2 className="mr-1 h-3 w-3" />
                              Resolve
                            </Button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Evidence Collection Configs */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <RefreshCw className="h-5 w-5" />
            Evidence Collection Configs
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {configsLoading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : configs.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              <RefreshCw className="mx-auto mb-3 h-8 w-8" />
              <p className="text-sm">No evidence collection configs configured.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Name</th>
                    <th className="px-4 py-3 text-left font-medium">Method</th>
                    <th className="px-4 py-3 text-left font-medium">Schedule</th>
                    <th className="px-4 py-3 text-left font-medium">Last Status</th>
                    <th className="px-4 py-3 text-left font-medium">Last Run</th>
                    <th className="px-4 py-3 text-center font-medium">Consecutive Failures</th>
                    <th className="px-4 py-3 text-left font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {configs.map((config: any) => {
                    const failures = (config.consecutive_failures as number) ?? 0;
                    return (
                      <tr key={config.id as string} className="border-b hover:bg-muted/50">
                        <td className="px-4 py-3 font-medium">{config.name as string}</td>
                        <td className="px-4 py-3 text-muted-foreground capitalize">
                          {(config.method as string)?.replace('_', ' ') ?? '—'}
                        </td>
                        <td className="px-4 py-3 font-mono text-xs">{config.schedule as string ?? '—'}</td>
                        <td className="px-4 py-3">
                          <Badge className={getCollectionStatusColor(config.last_status as string)}>
                            {(config.last_status as string)?.replace('_', ' ') ?? 'never'}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-muted-foreground">
                          {config.last_run_at ? formatDateTime(config.last_run_at as string) : '—'}
                        </td>
                        <td className="px-4 py-3 text-center">
                          <span className={cn(
                            'font-semibold',
                            failures > 0 && 'text-red-600 dark:text-red-400',
                          )}>
                            {failures}
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={runNow.isPending}
                            onClick={() => runNow.mutate(config.id as string)}
                          >
                            {runNow.isPending ? (
                              <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                            ) : (
                              <Play className="mr-1 h-3 w-3" />
                            )}
                            Run Now
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Monitor Status */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Shield className="h-5 w-5" />
            Monitor Status
          </CardTitle>
        </CardHeader>
        <CardContent>
          {monitorsLoading ? (
            <div className="grid gap-3 md:grid-cols-3">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-20 w-full" />
              ))}
            </div>
          ) : monitors.length === 0 ? (
            <div className="py-8 text-center text-muted-foreground">
              <Shield className="mx-auto mb-3 h-8 w-8" />
              <p className="text-sm">No monitors configured.</p>
            </div>
          ) : (
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
              {monitors.map((monitor: any) => {
                const isPassing = (monitor.status as string) === 'passing' || (monitor.status as string) === 'healthy';
                const failures = (monitor.consecutive_failures as number) ?? 0;
                return (
                  <div
                    key={monitor.id as string}
                    className={cn(
                      'rounded-lg border p-4 flex items-start gap-3',
                      !isPassing && 'border-red-200 dark:border-red-800',
                    )}
                  >
                    <div className={cn(
                      'mt-0.5 h-3 w-3 rounded-full shrink-0',
                      isPassing ? 'bg-green-500' : 'bg-red-500',
                    )} />
                    <div className="flex-1 min-w-0">
                      <p className="font-medium text-sm truncate">{monitor.name as string}</p>
                      <div className="flex items-center gap-2 mt-1">
                        <span className={cn(
                          'text-xs',
                          isPassing ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400',
                        )}>
                          {isPassing ? 'Passing' : 'Failing'}
                        </span>
                        {failures > 0 && (
                          <span className="text-xs text-red-600 dark:text-red-400">
                            ({failures} consecutive)
                          </span>
                        )}
                      </div>
                      {monitor.last_checked_at && (
                        <p className="text-[10px] text-muted-foreground mt-0.5">
                          Last: {formatDateTime(monitor.last_checked_at as string)}
                        </p>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
