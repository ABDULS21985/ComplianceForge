'use client';

import * as React from 'react';
import {
  Activity,
  BellRing,
  CheckCircle2,
  CircleHelp,
  CloudCog,
  Database,
  Inbox,
  Loader2,
  LockKeyhole,
  OctagonAlert,
  RefreshCw,
  ServerCog,
  ShieldCheck,
  Timer,
  TriangleAlert,
} from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import type {
  ConfigurationDiagnostic,
  DependencyDiagnostic,
  DiagnosticsSnapshot,
  DiagnosticStatus,
} from '@/types/diagnostics';
import {
  connectorRemediation,
  dependencyRemediation,
  diagnosticsFreshness,
  diagnosticsKeys,
  diagnosticsPollInterval,
  formatDiagnosticAge,
  formatDiagnosticLatency,
  humanizeDiagnosticToken,
  migrationRemediation,
  notificationRemediation,
  queueRemediation,
  statusCounts,
} from '@/lib/diagnostics';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/data/empty-state';
import { formatApiError } from '@/lib/enterprise-settings';
import { Skeleton } from '@/components/ui/skeleton';
import { SupportBundlePanel } from '@/components/diagnostics/support-bundle-panel';
import { useCapabilityPermissions } from '@/hooks/use-capability-permissions';
import { useQuery } from '@tanstack/react-query';

function subscribeEnvironment(onStoreChange: () => void) {
  document.addEventListener('visibilitychange', onStoreChange);
  window.addEventListener('online', onStoreChange);
  window.addEventListener('offline', onStoreChange);
  return () => {
    document.removeEventListener('visibilitychange', onStoreChange);
    window.removeEventListener('online', onStoreChange);
    window.removeEventListener('offline', onStoreChange);
  };
}

function getEnvironmentSnapshot() {
  return `${document.visibilityState}:${navigator.onLine ? 'online' : 'offline'}`;
}

export default function DiagnosticsPage() {
  const permission = useCapabilityPermissions();
  const environment = React.useSyncExternalStore(
    subscribeEnvironment,
    getEnvironmentSnapshot,
    () => 'hidden:offline',
  );
  const [now, setNow] = React.useState(() => new Date());
  const hidden = environment.startsWith('hidden:');
  const online = environment.endsWith(':online');

  React.useEffect(() => {
    if (hidden) return undefined;
    const timer = window.setInterval(() => setNow(new Date()), 30_000);
    return () => window.clearInterval(timer);
  }, [hidden]);

  const query = useQuery({
    queryKey: diagnosticsKeys.snapshot,
    queryFn: ({ signal }) => api.diagnostics.snapshot(signal),
    enabled: permission.canRead,
    // The shared transport already performs bounded idempotent retries. Avoid
    // multiplying those attempts at the query layer during an outage.
    retry: false,
    staleTime: 30_000,
    refetchInterval: (current) =>
      diagnosticsPollInterval(current.state.fetchFailureCount, hidden, online),
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: false,
  });
  const snapshot = query.data?.data;

  if (permission.isLoading) return <DiagnosticsSkeleton />;
  if (permission.isError) {
    return (
      <PageState
        icon={RefreshCw}
        title="Settings access could not be verified"
        description="The permission service is temporarily unavailable. No diagnostic data was loaded."
      >
        <Button type="button" variant="outline" onClick={() => void permission.retry()}>
          Try again
        </Button>
      </PageState>
    );
  }
  if (!permission.canRead) {
    return (
      <PageState
        icon={LockKeyhole}
        title="Administrator diagnostics unavailable"
        description="Your role does not grant organization settings access."
      />
    );
  }
  if (query.isLoading) return <DiagnosticsSkeleton />;
  if (query.isError && !snapshot) {
    return (
      <PageState
        icon={RefreshCw}
        title="Diagnostics temporarily unavailable"
        description={formatApiError(query.error, 'The diagnostic snapshot could not be loaded.')}
      >
        <Button
          type="button"
          variant="outline"
          disabled={!online}
          onClick={() => void query.refetch()}
        >
          Try again
        </Button>
      </PageState>
    );
  }
  if (!snapshot) return <DiagnosticsSkeleton />;

  const freshness = diagnosticsFreshness(snapshot.generated_at, now);
  const counts = statusCounts(snapshot);
  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Administrator diagnostics</h1>
          <p className="mt-1 max-w-4xl text-muted-foreground">
            Review safe operational posture for this organization without exposing infrastructure
            secrets or raw failure detail.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          disabled={query.isFetching || !online}
          onClick={() => void query.refetch()}
        >
          {query.isFetching ? (
            <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
          )}
          {query.isFetching ? 'Refreshing snapshot' : 'Refresh snapshot'}
        </Button>
      </div>

      <SafetyNotice online={online} />
      <SupportBundlePanel
        organizationId={snapshot.organization_id}
        permissions={permission.permissions}
        permissionsVerified={!permission.isLoading && !permission.isError}
      />
      {query.isError && (
        <div
          role="alert"
          className="flex gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-4"
        >
          <TriangleAlert aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0" />
          <div>
            <p className="font-medium">Refresh failed; showing the last successful snapshot</p>
            <p className="mt-1 text-sm text-muted-foreground">
              {formatApiError(query.error, 'The live diagnostic snapshot could not be refreshed.')}
            </p>
          </div>
        </div>
      )}
      {freshness.state !== 'fresh' && (
        <StaleNotice state={freshness.state} ageSeconds={freshness.ageSeconds} />
      )}

      <OverallPosture snapshot={snapshot} counts={counts} freshness={freshness} />
      <OperationalPosture snapshot={snapshot} />
      <Dependencies items={snapshot.dependencies} />
      <ConfigurationPosture items={snapshot.configuration} />
    </div>
  );
}

function SafetyNotice({ online }: { online: boolean }) {
  return (
    <Card>
      <CardContent className="grid gap-3 py-4 text-sm lg:grid-cols-2">
        <div className="flex gap-3">
          <ShieldCheck aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <p>
            Queue, inbox, notification, and connector counts are scoped only to your organization.
            They are not platform-wide totals.
          </p>
        </div>
        <div className="flex gap-3">
          <LockKeyhole aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <p>
            Raw dependency errors, endpoints, credentials, server identities, and configuration
            values are withheld. Messages and remediations are curated safe summaries.
          </p>
        </div>
        {!online && (
          <p role="status" className="flex gap-2 text-amber-700 lg:col-span-2">
            <TriangleAlert aria-hidden="true" className="h-4 w-4 shrink-0" />
            You are offline. Automatic and manual refresh are paused until connectivity returns.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function OverallPosture({
  counts,
  freshness,
  snapshot,
}: {
  counts: Record<DiagnosticStatus, number>;
  freshness: ReturnType<typeof diagnosticsFreshness>;
  snapshot: DiagnosticsSnapshot;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex flex-wrap items-center gap-2">
              Overall posture <StatusBadge status={snapshot.overall_status} />
            </CardTitle>
            <CardDescription className="mt-2">
              Generated {formatTimestamp(snapshot.generated_at)} · organization{' '}
              <span className="break-all font-mono">{snapshot.organization_id}</span>
            </CardDescription>
          </div>
          <Badge
            variant={freshness.state === 'fresh' ? 'outline' : 'secondary'}
            className="w-fit gap-1"
          >
            <Timer aria-hidden="true" className="h-3 w-3" />
            {freshness.ageSeconds === null
              ? 'Age unknown'
              : `${formatDiagnosticAge(freshness.ageSeconds)} old`}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Metric label="Healthy checks" value={counts.healthy} status="healthy" />
        <Metric label="Warnings" value={counts.warning} status="warning" />
        <Metric label="Critical checks" value={counts.critical} status="critical" />
        <Metric label="Unknown checks" value={counts.unknown} status="unknown" />
      </CardContent>
    </Card>
  );
}

function OperationalPosture({ snapshot }: { snapshot: DiagnosticsSnapshot }) {
  const migration = snapshot.migration;
  const queue = snapshot.queue;
  const notifications = snapshot.notifications;
  const connectors = snapshot.connectors;
  return (
    <section aria-labelledby="operational-heading" className="space-y-3">
      <div>
        <h2 id="operational-heading" className="text-xl font-semibold">
          Operational posture
        </h2>
        <p className="text-sm text-muted-foreground">
          Tenant-scoped work backlogs and application compatibility signals.
        </p>
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <DiagnosticCard
          icon={Database}
          title="Database migrations"
          status={migration.status}
          remediation={migrationRemediation(migration)}
        >
          <MetricGrid
            items={[
              ['Current schema', migration.current_version],
              ['Supported schema', migration.supported_version],
              ['Pending', migration.pending_count],
              ['Dirty state', migration.dirty ? 'Yes' : 'No'],
            ]}
          />
        </DiagnosticCard>
        <DiagnosticCard
          icon={Activity}
          title="Work queue"
          status={queue.status}
          remediation={queueRemediation(queue)}
        >
          <MetricGrid
            items={[
              ['Ready', queue.pending],
              ['Leased', queue.leased],
              ['Dead', queue.dead],
              ['Expired leases', queue.expired_leases],
              ['Oldest ready', formatDiagnosticAge(queue.oldest_ready_age_seconds)],
            ]}
          />
        </DiagnosticCard>
        <DiagnosticCard
          icon={Inbox}
          title="Event inbox"
          status={queue.status}
          remediation={queueRemediation(queue)}
        >
          <MetricGrid
            items={[
              ['Processing', queue.inbox_processing],
              ['Expired leases', queue.inbox_expired_leases],
              ['Queue age', formatDiagnosticAge(queue.oldest_ready_age_seconds)],
            ]}
          />
        </DiagnosticCard>
        <DiagnosticCard
          icon={BellRing}
          title="Notifications"
          status={notifications.status}
          remediation={notificationRemediation(notifications)}
        >
          <MetricGrid
            items={[
              ['Due', notifications.due],
              ['Expired leases', notifications.expired_leases],
              ['Terminal failures', notifications.terminal_failures],
              ['Oldest due', formatDiagnosticAge(notifications.oldest_due_age_seconds)],
            ]}
          />
        </DiagnosticCard>
        <DiagnosticCard
          icon={CloudCog}
          title="Connectors"
          status={connectors.status}
          remediation={connectorRemediation(connectors)}
          className="xl:col-span-2"
        >
          <MetricGrid
            items={[
              ['Configured', connectors.total],
              ['Healthy', connectors.healthy],
              ['Degraded', connectors.degraded],
              ['Unhealthy', connectors.unhealthy],
              ['Unknown', connectors.unknown],
              ['Recent failed syncs', connectors.recent_failed_syncs],
            ]}
          />
        </DiagnosticCard>
      </div>
    </section>
  );
}

function Dependencies({ items }: { items: DependencyDiagnostic[] }) {
  return (
    <section aria-labelledby="dependencies-heading" className="space-y-3">
      <div>
        <h2 id="dependencies-heading" className="text-xl font-semibold">
          Runtime dependencies
        </h2>
        <p className="text-sm text-muted-foreground">
          Bounded availability probes; raw errors and endpoint identities remain server-side.
        </p>
      </div>
      {items.length === 0 ? (
        <Card>
          <EmptyState
            icon={ServerCog}
            title="No dependency probes reported"
            description="This snapshot did not include any configured runtime probes."
          />
        </Card>
      ) : (
        <Card>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left text-sm">
                <caption className="sr-only">
                  Runtime dependency posture and safe remediation
                </caption>
                <thead className="border-b bg-muted/40">
                  <tr>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Dependency
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Status
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Latency
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Checked
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Safe detail and remediation
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((item) => (
                    <tr key={item.key} className="border-b last:border-0">
                      <th scope="row" className="px-4 py-4 align-top">
                        <span className="block font-medium">{item.name}</span>
                        <span className="mt-1 block font-mono text-xs text-muted-foreground">
                          {item.key}
                        </span>
                        <Badge variant="outline" className="mt-2">
                          {item.critical ? 'Critical dependency' : 'Optional dependency'}
                        </Badge>
                      </th>
                      <td className="px-4 py-4 align-top">
                        <StatusBadge status={item.status} />
                      </td>
                      <td className="px-4 py-4 align-top font-medium">
                        {formatDiagnosticLatency(item.latency_ms)}
                      </td>
                      <td className="px-4 py-4 align-top">{formatTimestamp(item.checked_at)}</td>
                      <td className="max-w-lg px-4 py-4 align-top">
                        <p>{item.message}</p>
                        <p className="mt-2 text-xs text-muted-foreground">
                          <strong>Next step:</strong> {dependencyRemediation(item)}
                        </p>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}
    </section>
  );
}

function ConfigurationPosture({ items }: { items: ConfigurationDiagnostic[] }) {
  const categories = [...new Set(items.map((item) => item.category))].sort();
  return (
    <section aria-labelledby="configuration-heading" className="space-y-3">
      <div>
        <h2 id="configuration-heading" className="text-xl font-semibold">
          Configuration posture
        </h2>
        <p className="text-sm text-muted-foreground">
          Boolean posture checks only. Configuration values and secrets are never returned to the
          browser.
        </p>
      </div>
      {items.length === 0 ? (
        <Card>
          <EmptyState
            icon={ServerCog}
            title="No configuration checks reported"
            description="This snapshot did not include any safe configuration posture checks."
          />
        </Card>
      ) : (
        <div className="space-y-4">
          {categories.map((category) => (
            <Card key={category}>
              <CardHeader>
                <CardTitle className="text-base">{humanizeDiagnosticToken(category)}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {items
                  .filter((item) => item.category === category)
                  .map((item) => (
                    <div key={item.key} className="rounded-md border p-4">
                      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                        <div>
                          <h3 className="font-medium">{humanizeDiagnosticToken(item.key)}</h3>
                          <p className="mt-1 text-sm text-muted-foreground">{item.message}</p>
                        </div>
                        <StatusBadge status={item.status} />
                      </div>
                      {item.remediation && item.status !== 'healthy' && (
                        <p className="mt-3 rounded-md bg-muted p-3 text-sm">
                          <strong>Remediation:</strong> {item.remediation}
                        </p>
                      )}
                    </div>
                  ))}
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </section>
  );
}

function DiagnosticCard({
  children,
  className,
  icon: Icon,
  remediation,
  status,
  title,
}: {
  children: React.ReactNode;
  className?: string;
  icon: typeof Database;
  remediation: string;
  status: DiagnosticStatus;
  title: string;
}) {
  return (
    <Card className={className}>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2">
            <Icon aria-hidden="true" className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-base">{title}</CardTitle>
          </div>
          <StatusBadge status={status} />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {children}
        <p className="rounded-md bg-muted p-3 text-sm">
          <strong>Next step:</strong> {remediation}
        </p>
      </CardContent>
    </Card>
  );
}

function StatusBadge({ status }: { status: DiagnosticStatus }) {
  const Icon =
    status === 'healthy'
      ? CheckCircle2
      : status === 'warning'
        ? TriangleAlert
        : status === 'critical'
          ? OctagonAlert
          : CircleHelp;
  return (
    <Badge
      variant={
        status === 'critical' ? 'destructive' : status === 'healthy' ? 'outline' : 'secondary'
      }
      className="w-fit gap-1"
    >
      <Icon aria-hidden="true" className="h-3.5 w-3.5" />
      {humanizeDiagnosticToken(status)}
    </Badge>
  );
}

function Metric({
  label,
  status,
  value,
}: {
  label: string;
  status: DiagnosticStatus;
  value: number;
}) {
  return (
    <div className="rounded-md border p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{label}</p>
        <StatusGlyph status={status} />
      </div>
      <p className="mt-1 text-2xl font-semibold">{value}</p>
    </div>
  );
}

function StatusGlyph({ status }: { status: DiagnosticStatus }) {
  const Icon =
    status === 'healthy'
      ? CheckCircle2
      : status === 'warning'
        ? TriangleAlert
        : status === 'critical'
          ? OctagonAlert
          : CircleHelp;
  return (
    <span>
      <Icon aria-hidden="true" className="h-4 w-4" />
      <span className="sr-only">{humanizeDiagnosticToken(status)} status</span>
    </span>
  );
}

function MetricGrid({ items }: { items: ReadonlyArray<readonly [string, number | string]> }) {
  return (
    <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {items.map(([label, value]) => (
        <div key={label}>
          <dt className="text-xs text-muted-foreground">{label}</dt>
          <dd className="mt-1 text-lg font-semibold">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function StaleNotice({
  ageSeconds,
  state,
}: {
  ageSeconds: number | null;
  state: ReturnType<typeof diagnosticsFreshness>['state'];
}) {
  const message =
    state === 'clock_skew'
      ? 'The snapshot timestamp is ahead of this browser clock. Verify workstation time before judging freshness.'
      : state === 'unknown'
        ? 'The snapshot timestamp could not be interpreted. Refresh and contact support if this persists.'
        : `This snapshot is ${ageSeconds === null ? 'an unknown age' : formatDiagnosticAge(ageSeconds) + ' old'}. Refresh before making an operational decision.`;
  return (
    <div
      role="status"
      className="flex gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-4"
    >
      <TriangleAlert aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0" />
      <div>
        <p className="font-medium">Snapshot may be stale</p>
        <p className="mt-1 text-sm text-muted-foreground">{message}</p>
      </div>
    </div>
  );
}

function DiagnosticsSkeleton() {
  return (
    <div role="status" aria-label="Loading administrator diagnostics" className="space-y-4">
      <Skeleton className="h-24" />
      <Skeleton className="h-24" />
      <div className="grid gap-4 xl:grid-cols-2">
        {Array.from({ length: 6 }).map((_, index) => (
          <Skeleton key={index} className="h-56" />
        ))}
      </div>
    </div>
  );
}

function PageState({
  children,
  description,
  icon: Icon,
  title,
}: {
  children?: React.ReactNode;
  description: string;
  icon: typeof RefreshCw;
  title: string;
}) {
  return (
    <Card className="mx-auto max-w-xl">
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <Icon aria-hidden="true" className="h-10 w-10 text-muted-foreground" />
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="text-sm text-muted-foreground">{description}</p>
        {children}
      </CardContent>
    </Card>
  );
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'medium' }).format(
    date,
  );
}
