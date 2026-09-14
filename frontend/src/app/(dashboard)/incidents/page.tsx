'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { AlertOctagon, AlertTriangle, BellRing, ChevronLeft, ChevronRight, Clock3, FilterX, Loader2, LockKeyhole, Plus, Search, ShieldAlert } from 'lucide-react';

import { IncidentEditorDialog } from '@/components/incidents/incident-editor-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { useIncidentPermissions } from '@/hooks/use-incident-permissions';
import { useIncidents, useIncidentStats, useUrgentBreaches } from '@/lib/api-hooks';
import { formatIncidentError, humanizeIncidentToken, incidentDeadline } from '@/lib/incident';
import { useQuickCreate } from '@/lib/use-quick-create';
import { cn, formatDateTime, getRiskLevelColor, getStatusColor } from '@/lib/utils';
import type { Incident, IncidentSeverity, IncidentStatus } from '@/types/incident';

const STATUSES: IncidentStatus[] = ['reported', 'triaged', 'investigating', 'contained', 'resolved', 'closed', 'cancelled'];
const SEVERITIES: IncidentSeverity[] = ['critical', 'high', 'medium', 'low'];
const PAGE_SIZES = [10, 20, 50] as const;

export default function IncidentsPage() {
  const router = useRouter();
  const access = useIncidentPermissions();
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState<number>(20);
  const [searchDraft, setSearchDraft] = React.useState('');
  const [search, setSearch] = React.useState('');
  const [category, setCategory] = React.useState('');
  const [status, setStatus] = React.useState<IncidentStatus | 'all'>('all');
  const [severity, setSeverity] = React.useState<IncidentSeverity | 'all'>('all');
  const [breach, setBreach] = React.useState<'all' | 'true' | 'false'>('all');
  const [sort, setSort] = React.useState<'reported_at' | 'updated_at' | 'severity' | 'notification_deadline'>('reported_at');
  const [direction, setDirection] = React.useState<'asc' | 'desc'>('desc');
  const [createOpen, setCreateOpen] = useQuickCreate('incident');

  const listQuery = useIncidents({
    page, page_size: pageSize, search: search || undefined, category: category || undefined,
    status: status === 'all' ? undefined : status,
    severity: severity === 'all' ? undefined : severity,
    breach_notifiable: breach === 'all' ? undefined : breach === 'true', sort, direction,
  }, { enabled: access.canRead });
  const statsQuery = useIncidentStats({ enabled: access.canRead });
  const breachesQuery = useUrgentBreaches({ horizon_hours: 168, limit: 20 }, { enabled: access.canRead, retry: 1 });
  const incidents = listQuery.data?.items ?? [];
  const totalPages = Math.max(listQuery.data?.total_pages ?? 0, 1);

  React.useEffect(() => {
    if (!listQuery.data || page <= totalPages) return;
    const timer = window.setTimeout(() => setPage(totalPages), 0);
    return () => window.clearTimeout(timer);
  }, [listQuery.data, page, totalPages]);

  if (access.isLoading || !access.user) return <IncidentListSkeleton />;
  if (access.isError) return <AccessState title="Incident access could not be verified" description="The permission service is unavailable, so incident data was not loaded." action={<Button onClick={() => void access.retry()}>Try again</Button>} />;
  if (!access.canRead) return <AccessState title="Incident management unavailable" description="Your role does not grant read access to incident records." />;

  const filtersActive = Boolean(search || category || status !== 'all' || severity !== 'all' || breach !== 'all');
  function applySearch(event: React.FormEvent) { event.preventDefault(); setSearch(searchDraft.trim()); setPage(1); }
  function resetFilters() { setSearchDraft(''); setSearch(''); setCategory(''); setStatus('all'); setSeverity('all'); setBreach('all'); setPage(1); }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div><h1 className="text-3xl font-bold tracking-tight">Incident management</h1><p className="mt-1 text-muted-foreground">Coordinate response, preserve the investigation trail, and manage GDPR Article 33 deadlines.</p></div>
        {access.canCreate && <Button className="w-full sm:w-auto" onClick={() => setCreateOpen(true)}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Report incident</Button>}
      </div>

      {breachesQuery.isError && <InlineState role="alert" icon={<AlertTriangle aria-hidden="true" className="h-4 w-4" />} text="Breach deadlines could not be refreshed. Incident records remain available below." />}
      {(breachesQuery.data?.length ?? 0) > 0 && <BreachDeadlinePanel incidents={breachesQuery.data ?? []} />}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5" aria-label="Incident portfolio metrics">
        <SummaryCard label="Total" value={statsQuery.data?.total} loading={statsQuery.isLoading} icon={<AlertOctagon aria-hidden="true" className="h-4 w-4 text-blue-600" />} />
        <SummaryCard label="Active" value={statsQuery.data?.active} loading={statsQuery.isLoading} icon={<Loader2 aria-hidden="true" className="h-4 w-4 text-amber-600" />} />
        <SummaryCard label="Urgent breaches" value={statsQuery.data?.urgent_breaches} loading={statsQuery.isLoading} highlight={Boolean(statsQuery.data?.urgent_breaches)} icon={<Clock3 aria-hidden="true" className="h-4 w-4 text-orange-600" />} />
        <SummaryCard label="Overdue breaches" value={statsQuery.data?.overdue_breaches} loading={statsQuery.isLoading} highlight={Boolean(statsQuery.data?.overdue_breaches)} icon={<ShieldAlert aria-hidden="true" className="h-4 w-4 text-red-600" />} />
        <SummaryCard label="Avg. resolution" value={statsQuery.data ? `${statsQuery.data.average_resolution_hours.toFixed(1)}h` : undefined} loading={statsQuery.isLoading} icon={<BellRing aria-hidden="true" className="h-4 w-4 text-emerald-600" />} />
      </div>
      {statsQuery.isError && <InlineState role="alert" icon={<AlertTriangle aria-hidden="true" className="h-4 w-4" />} text="Portfolio statistics are temporarily unavailable." />}

      <Card><CardHeader className="pb-4"><CardTitle className="text-base">Find incidents</CardTitle></CardHeader><CardContent>
        <form role="search" className="grid gap-3 md:grid-cols-2 xl:grid-cols-7" onSubmit={applySearch}>
          <div className="space-y-2 md:col-span-2"><Label htmlFor="incident-search">Reference, title, or description</Label><div className="relative"><Search aria-hidden="true" className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" /><Input id="incident-search" className="pl-9" maxLength={500} value={searchDraft} onChange={(event) => setSearchDraft(event.target.value)} /></div></div>
          <FilterSelect id="incident-status" label="Status" value={status} onValueChange={(value) => { setStatus(value as IncidentStatus | 'all'); setPage(1); }} options={STATUSES} />
          <FilterSelect id="incident-severity" label="Severity" value={severity} onValueChange={(value) => { setSeverity(value as IncidentSeverity | 'all'); setPage(1); }} options={SEVERITIES} />
          <FilterSelect id="incident-breach" label="DPA decision" value={breach} onValueChange={(value) => { setBreach(value as typeof breach); setPage(1); }} options={['true', 'false']} labels={{ true: 'Notifiable', false: 'Not notifiable' }} />
          <div className="space-y-2"><Label htmlFor="incident-category">Category</Label><Input id="incident-category" maxLength={100} value={category} onChange={(event) => { setCategory(event.target.value); setPage(1); }} /></div>
          <div className="flex items-end gap-2"><Button type="submit">Search</Button>{filtersActive && <Button type="button" variant="outline" onClick={resetFilters}><FilterX aria-hidden="true" className="mr-2 h-4 w-4" />Reset</Button>}</div>
        </form>
        <div className="mt-4 flex flex-wrap items-end gap-3 border-t pt-4">
          <FilterSelect id="incident-sort" label="Sort by" value={sort} onValueChange={(value) => { setSort(value as typeof sort); setPage(1); }} options={['reported_at', 'updated_at', 'severity', 'notification_deadline']} includeAll={false} />
          <FilterSelect id="incident-direction" label="Direction" value={direction} onValueChange={(value) => { setDirection(value as 'asc' | 'desc'); setPage(1); }} options={['desc', 'asc']} labels={{ desc: 'Newest / highest', asc: 'Oldest / lowest' }} includeAll={false} />
        </div>
      </CardContent></Card>

      <Card><CardContent className="p-0">
        {listQuery.isLoading ? <div className="space-y-3 p-6" aria-label="Loading incidents">{Array.from({ length: 5 }).map((_, index) => <Skeleton key={index} className="h-14 w-full" />)}</div>
          : listQuery.isError ? <ListError message={formatIncidentError(listQuery.error, 'Incidents could not be loaded.')} retry={() => void listQuery.refetch()} />
            : incidents.length === 0 ? <EmptyState filtered={filtersActive} create={access.canCreate ? () => setCreateOpen(true) : undefined} />
              : <><IncidentTable incidents={incidents} /><IncidentCards incidents={incidents} /></>}
        {listQuery.data && listQuery.data.total > 0 && <div className="flex flex-col gap-3 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-center gap-2"><Label htmlFor="incident-page-size" className="text-sm text-muted-foreground">Rows</Label><Select value={String(pageSize)} onValueChange={(value) => { setPageSize(Number(value)); setPage(1); }}><SelectTrigger id="incident-page-size" className="w-20"><SelectValue /></SelectTrigger><SelectContent>{PAGE_SIZES.map((value) => <SelectItem key={value} value={String(value)}>{value}</SelectItem>)}</SelectContent></Select><p className="text-sm text-muted-foreground">Page {page} of {totalPages} · {listQuery.data.total} total</p></div><div className="flex gap-2"><Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button><Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button></div></div>}
      </CardContent></Card>

      {access.canCreate && <IncidentEditorDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={(incident) => router.push(`/incidents/${incident.id}`)} />}
    </div>
  );
}

function BreachDeadlinePanel({ incidents }: { incidents: Incident[] }) {
  return <section aria-labelledby="breach-deadlines-heading" className="rounded-lg border-2 border-red-500 bg-red-50 p-4 dark:border-red-800 dark:bg-red-950/30"><div className="flex gap-3"><ShieldAlert aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-red-700 dark:text-red-300" /><div className="min-w-0 flex-1"><h2 id="breach-deadlines-heading" className="font-semibold text-red-950 dark:text-red-100">GDPR notification deadlines require attention</h2><p className="mt-1 text-sm text-red-800 dark:text-red-200">These assessed, notifiable breaches fall within the next seven days or are overdue. Open a record to enter the authority reference and notification evidence.</p><ul className="mt-3 grid gap-2 lg:grid-cols-2">{incidents.map((incident) => { const deadline = incidentDeadline(incident); return <li key={incident.id}><Link href={`/incidents/${incident.id}`} className="flex min-h-12 items-center justify-between gap-3 rounded-md border border-red-300 bg-background px-3 py-2 text-sm hover:bg-red-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:border-red-800 dark:hover:bg-red-950"><span className="min-w-0"><span className="block truncate font-medium">{incident.incident_ref} · {incident.title}</span><span className="block text-xs text-muted-foreground">Deadline {formatDateTime(incident.notification_deadline)}</span></span><DeadlineBadge state={deadline.state} hours={deadline.hoursRemaining} /></Link></li>; })}</ul></div></div></section>;
}

function IncidentTable({ incidents }: { incidents: Incident[] }) {
  return <div className="hidden overflow-x-auto md:block"><table className="w-full text-sm"><caption className="sr-only">Incident search results</caption><thead><tr className="border-b bg-muted/50"><th scope="col" className="px-4 py-3 text-left font-medium">Incident</th><th scope="col" className="px-4 py-3 text-left font-medium">Severity</th><th scope="col" className="px-4 py-3 text-left font-medium">Status</th><th scope="col" className="px-4 py-3 text-left font-medium">Breach assessment</th><th scope="col" className="px-4 py-3 text-left font-medium">Reported</th></tr></thead><tbody>{incidents.map((incident) => { const deadline = incidentDeadline(incident); return <tr key={incident.id} className="border-b last:border-0 hover:bg-muted/40"><td className="max-w-md px-4 py-3"><Link className="font-medium text-primary hover:underline" href={`/incidents/${incident.id}`}>{incident.title}</Link><span className="mt-1 block font-mono text-xs text-muted-foreground">{incident.incident_ref}</span></td><td className="px-4 py-3"><Badge className={getRiskLevelColor(incident.severity)}>{humanizeIncidentToken(incident.severity)}</Badge></td><td className="px-4 py-3"><Badge className={getStatusColor(incident.status)}>{humanizeIncidentToken(incident.status)}</Badge></td><td className="px-4 py-3">{incident.is_breach_notifiable ? <DeadlineBadge state={deadline.state} hours={deadline.hoursRemaining} /> : humanizeIncidentToken(incident.breach_assessment_status)}</td><td className="whitespace-nowrap px-4 py-3">{formatDateTime(incident.reported_at)}</td></tr>; })}</tbody></table></div>;
}

function IncidentCards({ incidents }: { incidents: Incident[] }) {
  return <ul className="divide-y md:hidden">{incidents.map((incident) => { const deadline = incidentDeadline(incident); return <li key={incident.id}><Link href={`/incidents/${incident.id}`} className="block space-y-3 p-4 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"><div><p className="font-medium">{incident.title}</p><p className="font-mono text-xs text-muted-foreground">{incident.incident_ref}</p></div><div className="flex flex-wrap gap-2"><Badge className={getRiskLevelColor(incident.severity)}>{humanizeIncidentToken(incident.severity)}</Badge><Badge className={getStatusColor(incident.status)}>{humanizeIncidentToken(incident.status)}</Badge>{incident.is_breach_notifiable && <DeadlineBadge state={deadline.state} hours={deadline.hoursRemaining} />}</div><p className="text-xs text-muted-foreground">Reported {formatDateTime(incident.reported_at)}</p></Link></li>; })}</ul>;
}

function DeadlineBadge({ state, hours }: { state: string; hours?: number }) {
  const label = state === 'notified' ? 'DPA notified' : state === 'overdue' ? 'DPA overdue' : state === 'urgent' || state === 'upcoming' ? `${Math.max(0, hours ?? 0).toFixed(1)}h left` : 'Assessment pending';
  return <Badge className={cn(state === 'notified' && 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200', state === 'overdue' && 'bg-red-700 text-white', state === 'urgent' && 'bg-orange-600 text-white')} variant={['notified', 'overdue', 'urgent'].includes(state) ? 'default' : 'outline'}>{label}</Badge>;
}

function FilterSelect({ id, label, value, options, labels = {}, includeAll = true, onValueChange }: { id: string; label: string; value: string; options: readonly string[]; labels?: Record<string, string>; includeAll?: boolean; onValueChange: (value: string) => void }) { return <div className="space-y-2"><Label htmlFor={id}>{label}</Label><Select value={value} onValueChange={onValueChange}><SelectTrigger id={id}><SelectValue /></SelectTrigger><SelectContent>{includeAll && <SelectItem value="all">All</SelectItem>}{options.map((option) => <SelectItem key={option} value={option}>{labels[option] ?? humanizeIncidentToken(option)}</SelectItem>)}</SelectContent></Select></div>; }
function SummaryCard({ label, value, loading, highlight, icon }: { label: string; value?: number | string; loading: boolean; highlight?: boolean; icon: React.ReactNode }) { return <Card className={cn(highlight && 'border-red-400')}><CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2"><CardTitle className="text-sm font-medium">{label}</CardTitle>{icon}</CardHeader><CardContent>{loading ? <Skeleton className="h-8 w-16" /> : <p className={cn('text-2xl font-bold', highlight && 'text-red-700 dark:text-red-300')}>{value ?? '—'}</p>}</CardContent></Card>; }
function InlineState({ role, icon, text }: { role: 'alert' | 'status'; icon: React.ReactNode; text: string }) { return <div role={role} className="flex items-center gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200">{icon}{text}</div>; }
function ListError({ message, retry }: { message: string; retry: () => void }) { return <div role="alert" className="flex flex-col items-center gap-3 p-10 text-center"><AlertTriangle aria-hidden="true" className="h-9 w-9 text-destructive" /><p>{message}</p><Button variant="outline" onClick={retry}>Retry</Button></div>; }
function EmptyState({ filtered, create }: { filtered: boolean; create?: () => void }) { return <div className="flex flex-col items-center gap-3 p-12 text-center"><AlertOctagon aria-hidden="true" className="h-10 w-10 text-muted-foreground" /><h2 className="text-lg font-medium">{filtered ? 'No incidents match these filters' : 'No incidents reported'}</h2><p className="text-sm text-muted-foreground">{filtered ? 'Adjust or reset the filters to broaden the result set.' : 'Report an incident to begin the response workflow.'}</p>{create && !filtered && <Button onClick={create}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Report incident</Button>}</div>; }
function AccessState({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) { return <Card><CardContent role="alert" className="flex flex-col items-center gap-3 py-12 text-center"><LockKeyhole aria-hidden="true" className="h-9 w-9" /><h1 className="text-xl font-semibold">{title}</h1><p className="max-w-lg text-sm text-muted-foreground">{description}</p>{action}</CardContent></Card>; }
function IncidentListSkeleton() { return <div className="space-y-5" aria-label="Checking incident permissions"><Skeleton className="h-10 w-72" /><div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">{Array.from({ length: 5 }).map((_, index) => <Skeleton key={index} className="h-28" />)}</div><Skeleton className="h-80" /></div>; }
