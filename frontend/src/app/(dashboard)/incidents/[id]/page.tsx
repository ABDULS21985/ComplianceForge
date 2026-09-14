'use client';

import * as React from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { AlertTriangle, ArrowLeft, Bell, CalendarClock, CheckCircle2, ChevronLeft, ChevronRight, Clock3, History, LockKeyhole, Pencil, Plus, RotateCcw, ShieldAlert, Trash2, UserMinus, UsersRound, XCircle, Zap } from 'lucide-react';

import { BreachAssessmentDialog } from '@/components/incidents/breach-assessment-dialog';
import { DPANotificationDialog } from '@/components/incidents/dpa-notification-dialog';
import { IncidentActionDialog, type IncidentAction } from '@/components/incidents/incident-action-dialog';
import { IncidentAssignmentDialog } from '@/components/incidents/incident-assignment-dialog';
import { IncidentDeleteDialog } from '@/components/incidents/incident-delete-dialog';
import { IncidentEditorDialog } from '@/components/incidents/incident-editor-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useIncidentPermissions } from '@/hooks/use-incident-permissions';
import { useIncident, useIncidentAssignments, useIncidentTimeline } from '@/lib/api-hooks';
import { canDeleteIncident, formatIncidentError, humanizeIncidentToken, incidentDeadline, incidentEscalationOptions, incidentNextStatuses } from '@/lib/incident';
import { cn, formatDate, formatDateTime, getRiskLevelColor, getStatusColor } from '@/lib/utils';
import type { Incident, IncidentAssignment, IncidentEvent, IncidentStatus } from '@/types/incident';

const LIFECYCLE: Exclude<IncidentStatus, 'cancelled'>[] = ['reported', 'triaged', 'investigating', 'contained', 'resolved', 'closed'];

export default function IncidentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const access = useIncidentPermissions();
  const incidentQuery = useIncident(id, { enabled: access.canRead && Boolean(id) });
  const [timelinePage, setTimelinePage] = React.useState(1);
  const timelineQuery = useIncidentTimeline(id, { page: timelinePage, page_size: 20 }, { enabled: access.canRead && Boolean(id) });
  const assignmentsQuery = useIncidentAssignments(id, true, { enabled: access.canRead && Boolean(id) });
  const [editOpen, setEditOpen] = React.useState(false);
  const [breachOpen, setBreachOpen] = React.useState(false);
  const [dpaOpen, setDpaOpen] = React.useState(false);
  const [assignOpen, setAssignOpen] = React.useState(false);
  const [unassigning, setUnassigning] = React.useState<IncidentAssignment | null>(null);
  const [action, setAction] = React.useState<IncidentAction | null>(null);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [now, setNow] = React.useState(() => new Date());

  React.useEffect(() => {
    const interval = window.setInterval(() => setNow(new Date()), 60_000);
    return () => window.clearInterval(interval);
  }, []);

  if (access.isLoading || !access.user) return <DetailSkeleton />;
  if (access.isError) return <AccessState title="Incident access could not be verified" description="The permission service is unavailable, so the incident was not loaded." action={<Button onClick={() => void access.retry()}>Try again</Button>} />;
  if (!access.canRead) return <AccessState title="Incident unavailable" description="Your role does not grant read access to incident records." />;
  if (incidentQuery.isLoading) return <DetailSkeleton />;
  if (incidentQuery.isError || !incidentQuery.data) return <LoadError error={incidentQuery.error} retry={() => void incidentQuery.refetch()} />;

  const incident = incidentQuery.data;
  const terminal = incident.status === 'closed' || incident.status === 'cancelled';
  const nextStatuses = incidentNextStatuses(incident.status);
  const mayClose = access.canApprove && incident.status === 'resolved';
  const mayDelete = access.canDelete && canDeleteIncident(incident, now);
  const escalationOptions = incidentEscalationOptions(incident.severity);
  const deadline = incidentDeadline(incident, now);
  const totalTimelinePages = Math.max(timelineQuery.data?.total_pages ?? 0, 1);

  return (
    <div className="space-y-6">
      <Link href="/incidents" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"><ArrowLeft aria-hidden="true" className="mr-1 h-4 w-4" />Back to incidents</Link>

      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 space-y-2"><div className="flex flex-wrap items-center gap-2"><span className="font-mono text-sm text-muted-foreground">{incident.incident_ref}</span><Badge className={getRiskLevelColor(incident.severity)}>{humanizeIncidentToken(incident.severity)}</Badge><Badge className={getStatusColor(incident.status)}>{humanizeIncidentToken(incident.status)}</Badge>{incident.is_data_breach && <Badge variant="outline" className="border-red-400 text-red-700 dark:text-red-300"><ShieldAlert aria-hidden="true" className="mr-1 h-3 w-3" />Personal-data breach</Badge>}</div><h1 className="break-words text-3xl font-bold tracking-tight">{incident.title}</h1><p className="text-sm text-muted-foreground">Version {incident.version} · Updated {formatDateTime(incident.updated_at)}</p></div>
        <div className="flex flex-wrap gap-2">
          {access.canUpdate && !terminal && <Button variant="outline" onClick={() => setEditOpen(true)}><Pencil aria-hidden="true" className="mr-2 h-4 w-4" />Edit</Button>}
          {access.canUpdate && !terminal && escalationOptions.length > 0 && <Button variant="outline" onClick={() => setAction({ kind: 'escalate' })}><Zap aria-hidden="true" className="mr-2 h-4 w-4" />Escalate</Button>}
          {access.canUpdate && nextStatuses.map((status) => <Button key={status} onClick={() => setAction({ kind: 'transition', status })}>Move to {humanizeIncidentToken(status)}</Button>)}
          {mayClose && <Button onClick={() => setAction({ kind: 'close' })}><CheckCircle2 aria-hidden="true" className="mr-2 h-4 w-4" />Close incident</Button>}
          {access.canUpdate && !terminal && <Button variant="destructive" onClick={() => setAction({ kind: 'cancel' })}><XCircle aria-hidden="true" className="mr-2 h-4 w-4" />Cancel</Button>}
          {access.canUpdate && terminal && <Button onClick={() => setAction({ kind: 'reopen' })}><RotateCcw aria-hidden="true" className="mr-2 h-4 w-4" />Reopen</Button>}
          {mayDelete && <Button variant="destructive" onClick={() => setDeleteOpen(true)}><Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />Delete</Button>}
        </div>
      </div>

      {incident.status === 'resolved' && incident.is_breach_notifiable && !incident.dpa_notified_at && <Warning text="This incident cannot close until the supervisory-authority notification is recorded." />}
      {terminal && !mayDelete && access.canDelete && <Warning text={incident.legal_hold ? 'Deletion is blocked by legal hold.' : incident.retention_until && new Date(incident.retention_until) > now ? `Deletion is blocked until retention expires on ${formatDateTime(incident.retention_until)}.` : 'Only terminal incident records can be deleted.'} />}

      <LifecycleRail incident={incident} />

      <div className="grid gap-5 xl:grid-cols-[1.2fr_0.8fr]">
        <Card><CardHeader><CardTitle className="text-lg">Incident brief</CardTitle></CardHeader><CardContent className="space-y-5"><Detail label="Description" value={incident.description} /><div className="grid gap-4 sm:grid-cols-2"><Detail label="Category" value={incident.category} /><Detail label="Reporter ID" value={incident.reporter_id} mono /></div><div className="grid gap-4 sm:grid-cols-2"><Detail label="Detected" value={formatDateTime(incident.detected_at)} /><Detail label="Occurred" value={formatDateTime(incident.occurred_at)} /><Detail label="Reported" value={formatDateTime(incident.reported_at)} /><Detail label="Follow-up" value={formatDate(incident.followup_date)} /></div>{incident.related_asset_id && <Detail label="Related asset ID" value={incident.related_asset_id} mono />}</CardContent></Card>
        <Card><CardHeader><CardTitle className="text-lg">Investigation record</CardTitle></CardHeader><CardContent className="space-y-5"><Detail label="Impact" value={incident.impact || 'Not recorded'} /><Detail label="Root cause" value={incident.root_cause || 'Not recorded'} /><Detail label="Lessons learned" value={incident.lessons_learned || 'Not recorded'} /><div className="grid gap-4 sm:grid-cols-2"><Detail label="Retention until" value={formatDateTime(incident.retention_until)} /><Detail label="Legal hold" value={incident.legal_hold ? 'Active' : 'No'} /></div></CardContent></Card>
      </div>

      <BreachPanel incident={incident} deadline={deadline} canApprove={access.canApprove} onAssess={() => setBreachOpen(true)} onNotify={() => setDpaOpen(true)} />

      <Card><CardHeader className="flex flex-row items-center justify-between"><div><CardTitle className="flex items-center gap-2 text-lg"><UsersRound aria-hidden="true" className="h-5 w-5" />Response team</CardTitle><p className="mt-1 text-sm text-muted-foreground">Active assignments; ended assignments remain in the timeline.</p></div>{access.canAssign && !terminal && <Button size="sm" onClick={() => setAssignOpen(true)}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Assign</Button>}</CardHeader><CardContent><AssignmentList assignments={assignmentsQuery.data ?? []} loading={assignmentsQuery.isLoading} error={assignmentsQuery.isError} canUnassign={access.canAssign && !terminal} onUnassign={setUnassigning} retry={() => void assignmentsQuery.refetch()} /></CardContent></Card>

      <Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><History aria-hidden="true" className="h-5 w-5" />Immutable timeline</CardTitle></CardHeader><CardContent><Timeline events={timelineQuery.data?.items ?? []} loading={timelineQuery.isLoading} error={timelineQuery.isError} retry={() => void timelineQuery.refetch()} />{timelineQuery.data && timelineQuery.data.total > 0 && <div className="mt-5 flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between"><p className="text-sm text-muted-foreground">Page {timelinePage} of {totalTimelinePages} · {timelineQuery.data.total} events</p><div className="flex gap-2"><Button variant="outline" size="sm" disabled={timelinePage <= 1} onClick={() => setTimelinePage((value) => value - 1)}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button><Button variant="outline" size="sm" disabled={timelinePage >= totalTimelinePages} onClick={() => setTimelinePage((value) => value + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button></div></div>}</CardContent></Card>

      {access.canUpdate && <IncidentEditorDialog incident={incident} open={editOpen} onOpenChange={setEditOpen} />}
      <IncidentActionDialog incident={incident} action={action} onOpenChange={(open) => !open && setAction(null)} />
      {access.canApprove && <><BreachAssessmentDialog incident={incident} open={breachOpen} onOpenChange={setBreachOpen} /><DPANotificationDialog incident={incident} open={dpaOpen} onOpenChange={setDpaOpen} /></>}
      {access.canAssign && <><IncidentAssignmentDialog incident={incident} open={assignOpen} onOpenChange={setAssignOpen} />{unassigning && <IncidentAssignmentDialog incident={incident} assignment={unassigning} open onOpenChange={(open) => !open && setUnassigning(null)} />}</>}
      {access.canDelete && <IncidentDeleteDialog incident={incident} open={deleteOpen} onOpenChange={setDeleteOpen} onDeleted={() => { router.push('/incidents'); router.refresh(); }} />}
    </div>
  );
}

function LifecycleRail({ incident }: { incident: Incident }) {
  const timestamps: Partial<Record<Exclude<IncidentStatus, 'cancelled'>, string | undefined>> = { reported: incident.reported_at, triaged: incident.triaged_at, investigating: incident.investigation_started_at, contained: incident.contained_at, resolved: incident.resolved_at, closed: incident.closed_at };
  const currentIndex = LIFECYCLE.indexOf(incident.status as Exclude<IncidentStatus, 'cancelled'>);
  return <Card><CardHeader><CardTitle className="text-base">Response lifecycle</CardTitle></CardHeader><CardContent><ol className="grid gap-2 sm:grid-cols-3 xl:grid-cols-6">{LIFECYCLE.map((status, index) => { const complete = currentIndex >= index && incident.status !== 'cancelled'; const current = incident.status === status; return <li key={status} aria-current={current ? 'step' : undefined} className={cn('rounded-md border p-3', current && 'border-primary bg-primary/5', complete && !current && 'bg-muted/50')}><div className="flex items-center gap-2">{complete ? <CheckCircle2 aria-hidden="true" className="h-4 w-4 text-emerald-600" /> : <span aria-hidden="true" className="h-4 w-4 rounded-full border" />}<span className="text-sm font-medium">{humanizeIncidentToken(status)}</span></div><p className="mt-1 text-xs text-muted-foreground">{formatDateTime(timestamps[status])}</p></li>; })}</ol>{incident.status === 'cancelled' && <p className="mt-3 flex items-center gap-2 text-sm text-muted-foreground"><XCircle aria-hidden="true" className="h-4 w-4" />Cancelled {formatDateTime(incident.cancelled_at)} · {incident.cancellation_reason}</p>}</CardContent></Card>;
}

function BreachPanel({ incident, deadline, canApprove, onAssess, onNotify }: { incident: Incident; deadline: ReturnType<typeof incidentDeadline>; canApprove: boolean; onAssess: () => void; onNotify: () => void }) {
  return <Card className={cn(incident.is_breach_notifiable && !incident.dpa_notified_at && (deadline.state === 'overdue' ? 'border-red-600' : 'border-orange-400'))}><CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><CardTitle className="flex items-center gap-2 text-lg"><ShieldAlert aria-hidden="true" className="h-5 w-5" />GDPR breach assessment</CardTitle><p className="mt-1 text-sm text-muted-foreground">Assessment status: {humanizeIncidentToken(incident.breach_assessment_status)}</p></div><div className="flex flex-wrap gap-2">{canApprove && !incident.dpa_notified_at && <Button variant="outline" onClick={onAssess}>{incident.breach_assessment_status === 'pending' ? 'Assess breach' : 'Revise assessment'}</Button>}{canApprove && incident.is_breach_notifiable && !incident.dpa_notified_at && <Button variant="destructive" onClick={onNotify}><Bell aria-hidden="true" className="mr-2 h-4 w-4" />Record DPA notification</Button>}</div></CardHeader><CardContent className="space-y-5">
    {incident.is_breach_notifiable && <div role={deadline.state === 'overdue' ? 'alert' : 'status'} aria-live="polite" className={cn('flex items-start gap-3 rounded-md border p-4', deadline.state === 'overdue' ? 'border-red-500 bg-red-50 text-red-950 dark:bg-red-950/30 dark:text-red-100' : deadline.state === 'notified' ? 'border-emerald-400 bg-emerald-50 text-emerald-950 dark:bg-emerald-950/30 dark:text-emerald-100' : 'border-orange-400 bg-orange-50 text-orange-950 dark:bg-orange-950/30 dark:text-orange-100')}><CalendarClock aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0" /><div><p className="font-semibold">{deadline.state === 'notified' ? `DPA notified ${formatDateTime(incident.dpa_notified_at)}` : deadline.state === 'overdue' ? '72-hour notification deadline has passed' : `${Math.max(0, deadline.hoursRemaining ?? 0).toFixed(1)} hours remaining`}</p><p className="text-sm">Awareness {formatDateTime(incident.breach_awareness_at)} · Deadline {formatDateTime(incident.notification_deadline)}</p>{incident.dpa_notification_reference && <p className="mt-1 text-sm">Authority reference: {incident.dpa_notification_reference}</p>}</div></div>}
    {incident.breach_assessment_status === 'pending' ? <p className="text-sm text-muted-foreground">No decision has been recorded. Assess whether personal data was involved and whether notification is required.</p> : <div className="grid gap-5 lg:grid-cols-2"><Detail label="Assessment rationale" value={incident.breach_assessment_reason ?? '—'} /><div className="grid grid-cols-2 gap-4"><Detail label="Data subjects" value={incident.data_subjects_affected?.toLocaleString() ?? '—'} /><Detail label="Records" value={incident.records_affected?.toLocaleString() ?? '—'} /><Detail label="Special-category data" value={incident.special_category_data ? 'Yes' : 'No'} /><Detail label="Cross-border" value={incident.cross_border ? 'Yes' : 'No'} /></div><Detail label="Nature" value={incident.breach_nature || 'Not recorded'} /><Detail label="Likely consequences" value={incident.likely_consequences || 'Not recorded'} /><Detail label="Mitigation measures" value={incident.mitigation_measures || 'Not recorded'} /><Detail label="Data categories" value={incident.data_categories.length ? incident.data_categories.join(', ') : 'None recorded'} /></div>}
  </CardContent></Card>;
}

function AssignmentList({ assignments, loading, error, canUnassign, onUnassign, retry }: { assignments: IncidentAssignment[]; loading: boolean; error: boolean; canUnassign: boolean; onUnassign: (assignment: IncidentAssignment) => void; retry: () => void }) {
  if (loading) return <div className="space-y-2" aria-label="Loading assignments"><Skeleton className="h-12" /><Skeleton className="h-12" /></div>;
  if (error) return <div role="alert" className="flex items-center justify-between gap-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive"><span>Assignments could not be loaded.</span><Button variant="outline" size="sm" onClick={retry}>Retry</Button></div>;
  if (!assignments.length) return <p className="py-5 text-center text-sm text-muted-foreground">No active responders assigned.</p>;
  return <ul className="divide-y">{assignments.map((assignment) => <li key={assignment.id} className="flex flex-col gap-3 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between"><div><p className="font-mono text-sm">{assignment.assignee_user_id}</p><p className="text-xs text-muted-foreground">{humanizeIncidentToken(assignment.role)} · assigned {formatDateTime(assignment.assigned_at)}</p><p className="mt-1 text-sm">{assignment.reason}</p></div>{canUnassign && <Button variant="outline" size="sm" onClick={() => onUnassign(assignment)}><UserMinus aria-hidden="true" className="mr-2 h-4 w-4" />Unassign</Button>}</li>)}</ul>;
}

function Timeline({ events, loading, error, retry }: { events: IncidentEvent[]; loading: boolean; error: boolean; retry: () => void }) {
  if (loading) return <div className="space-y-3" aria-label="Loading timeline">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-16" />)}</div>;
  if (error) return <div role="alert" className="flex items-center justify-between gap-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive"><span>Timeline events could not be loaded.</span><Button variant="outline" size="sm" onClick={retry}>Retry</Button></div>;
  if (!events.length) return <p className="py-5 text-center text-sm text-muted-foreground">No timeline events recorded.</p>;
  return <ol className="space-y-0">{events.map((event, index) => <li key={event.id} className="flex gap-3"><div className="flex flex-col items-center"><span className="mt-1 flex h-7 w-7 items-center justify-center rounded-full border bg-background"><Clock3 aria-hidden="true" className="h-3.5 w-3.5" /></span>{index < events.length - 1 && <span aria-hidden="true" className="w-px flex-1 bg-border" />}</div><div className="min-w-0 flex-1 pb-5"><div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between"><div><p className="font-medium">{event.summary}</p><p className="text-xs text-muted-foreground">{humanizeIncidentToken(event.event_type)} · actor <span className="font-mono">{event.actor_user_id}</span></p></div><time className="whitespace-nowrap text-xs text-muted-foreground" dateTime={event.occurred_at}>{formatDateTime(event.occurred_at)}</time></div>{event.from_status && event.to_status && <p className="mt-2 text-sm"><Badge variant="outline">{humanizeIncidentToken(event.from_status)}</Badge><span aria-hidden="true" className="mx-2">→</span><Badge variant="outline">{humanizeIncidentToken(event.to_status)}</Badge></p>}</div></li>)}</ol>;
}

function Detail({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) { return <div><p className="text-sm font-medium text-muted-foreground">{label}</p><p className={cn('mt-1 break-words whitespace-pre-wrap text-sm', mono && 'font-mono text-xs')}>{value || '—'}</p></div>; }
function Warning({ text }: { text: string }) { return <div role="status" className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200"><AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />{text}</div>; }
function DetailSkeleton() { return <div className="space-y-5" aria-label="Loading incident"><Skeleton className="h-5 w-36" /><Skeleton className="h-12 w-3/4" /><Skeleton className="h-32" /><div className="grid gap-5 lg:grid-cols-2"><Skeleton className="h-72" /><Skeleton className="h-72" /></div></div>; }
function AccessState({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) { return <Card><CardContent role="alert" className="flex flex-col items-center gap-3 py-12 text-center"><LockKeyhole aria-hidden="true" className="h-9 w-9" /><h1 className="text-xl font-semibold">{title}</h1><p className="max-w-lg text-sm text-muted-foreground">{description}</p>{action}</CardContent></Card>; }
function LoadError({ error, retry }: { error: unknown; retry: () => void }) { return <div className="space-y-5"><Link href="/incidents" className="inline-flex items-center text-sm text-muted-foreground"><ArrowLeft aria-hidden="true" className="mr-1 h-4 w-4" />Back to incidents</Link><Card><CardContent role="alert" className="flex flex-col items-center gap-3 py-12 text-center"><AlertTriangle aria-hidden="true" className="h-9 w-9 text-destructive" /><h1 className="text-xl font-semibold">Incident could not be loaded</h1><p className="text-sm text-muted-foreground">{formatIncidentError(error, 'The incident was not found or is temporarily unavailable.')}</p><Button variant="outline" onClick={retry}>Retry</Button></CardContent></Card></div>; }
