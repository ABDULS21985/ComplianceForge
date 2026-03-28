'use client';

import * as React from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  ShieldAlert,
  Clock,
  Bell,
  AlertTriangle,
  CheckCircle2,
  Loader2,
  Database,
  FileWarning,
  Send,
} from 'lucide-react';

import {
  cn,
  formatDate,
  formatDateTime,
  formatRelativeTime,
  getStatusColor,
  getRiskLevelColor,
} from '@/lib/utils';
import {
  useIncident,
  useNotifyDPA,
  useNis2EarlyWarning,
  useUpdateIncidentStatus,
} from '@/lib/api-hooks';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function IncidentDetailPage() {
  const params = useParams();
  const id = params.id as string;

  const { data: incident, isLoading, isError, error } = useIncident(id);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const inc = incident as any;

  const notifyDPA = useNotifyDPA();
  const nis2EarlyWarning = useNis2EarlyWarning();
  const updateStatus = useUpdateIncidentStatus();

  // Live countdown for GDPR deadline
  const [now, setNow] = React.useState(Date.now());
  React.useEffect(() => {
    const interval = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(interval);
  }, []);

  // Loading
  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-4 w-96" />
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
        </div>
      </div>
    );
  }

  // Error
  if (isError || !inc) {
    return (
      <div className="space-y-4">
        <Link href="/incidents" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="mr-1 h-4 w-4" /> Back to Incidents
        </Link>
        <div className="text-center py-12 text-destructive">
          {isError
            ? `Failed to load incident: ${(error as Error)?.message ?? 'Unknown error'}`
            : 'Incident not found'}
        </div>
      </div>
    );
  }

  const isBreach = inc.is_data_breach as boolean;
  const isNis2 = inc.is_nis2_reportable as boolean;
  const dpaNotifiedAt = inc.dpa_notified_at as string | undefined;
  const notificationDeadline = inc.notification_deadline as string | undefined;
  const status = inc.status as string;

  // Countdown calculation
  const deadlineMs = notificationDeadline ? new Date(notificationDeadline).getTime() : null;
  const remainingMs = deadlineMs ? Math.max(0, deadlineMs - now) : null;
  const remainingHours = remainingMs !== null ? remainingMs / (1000 * 60 * 60) : null;
  const remainingMinutes = remainingMs !== null ? Math.floor((remainingMs % (1000 * 60 * 60)) / (1000 * 60)) : null;
  const remainingSeconds = remainingMs !== null ? Math.floor((remainingMs % (1000 * 60)) / 1000) : null;

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link href="/incidents" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="mr-1 h-4 w-4" /> Back to Incidents
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <span className="font-mono text-sm text-muted-foreground">
              {inc.incident_ref as string}
            </span>
            <Badge className={getRiskLevelColor(inc.severity as string)}>
              {inc.severity as string}
            </Badge>
            <Badge className={getStatusColor(status)}>
              {status.replace('_', ' ')}
            </Badge>
            {isBreach && (
              <Badge className="bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400">
                <ShieldAlert className="mr-1 h-3 w-3" />
                Data Breach
              </Badge>
            )}
            {isNis2 && (
              <Badge className="bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400">
                NIS2 Reportable
              </Badge>
            )}
          </div>
          <h1 className="text-3xl font-bold tracking-tight">{inc.title as string}</h1>
        </div>
        <div className="flex items-center gap-2">
          {status !== 'resolved' && status !== 'closed' && (
            <Button
              variant="outline"
              onClick={() => updateStatus.mutate({ id, data: { status: 'resolved' } })}
              disabled={updateStatus.isPending}
            >
              {updateStatus.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Resolve Incident
            </Button>
          )}
          {status === 'resolved' && (
            <Button
              variant="outline"
              onClick={() => updateStatus.mutate({ id, data: { status: 'closed' } })}
              disabled={updateStatus.isPending}
            >
              {updateStatus.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Close Incident
            </Button>
          )}
        </div>
      </div>

      {/* ============================================================= */}
      {/* GDPR Breach Notification Panel                                 */}
      {/* ============================================================= */}
      {isBreach && (
        <div
          className={cn(
            'rounded-lg border-2 p-5',
            dpaNotifiedAt
              ? 'border-green-400 bg-green-50 dark:border-green-700 dark:bg-green-950/20'
              : 'border-red-500 bg-red-50 dark:border-red-700 dark:bg-red-950/30'
          )}
        >
          <div className="flex items-center gap-2 mb-4">
            <ShieldAlert
              className={cn(
                'h-5 w-5',
                dpaNotifiedAt
                  ? 'text-green-600 dark:text-green-400'
                  : 'text-red-600 dark:text-red-400'
              )}
            />
            <h2
              className={cn(
                'text-lg font-bold',
                dpaNotifiedAt
                  ? 'text-green-700 dark:text-green-400'
                  : 'text-red-700 dark:text-red-400'
              )}
            >
              GDPR Article 33 — Data Breach Notification
            </h2>
          </div>

          <div className="grid gap-6 md:grid-cols-2">
            {/* Left: Deadline & Countdown */}
            <div className="space-y-4">
              {/* Live Countdown */}
              {!dpaNotifiedAt && remainingMs !== null && (
                <div className="space-y-2">
                  <p className="text-sm font-medium text-muted-foreground">Time Remaining for DPA Notification</p>
                  {remainingMs === 0 ? (
                    <div className="text-3xl font-bold text-red-700 dark:text-red-400">
                      DEADLINE PASSED
                    </div>
                  ) : (
                    <div className="flex items-baseline gap-1">
                      <span
                        className={cn(
                          'text-4xl font-bold tabular-nums',
                          (remainingHours ?? 0) < 12
                            ? 'text-red-700 dark:text-red-400'
                            : (remainingHours ?? 0) < 24
                              ? 'text-orange-600 dark:text-orange-400'
                              : 'text-yellow-600 dark:text-yellow-400'
                        )}
                      >
                        {Math.floor(remainingHours ?? 0)}h {remainingMinutes}m {remainingSeconds}s
                      </span>
                    </div>
                  )}
                  <p className="text-xs text-muted-foreground">
                    Deadline: {formatDateTime(notificationDeadline)}
                  </p>
                </div>
              )}

              {/* Notification Status */}
              <div className="space-y-2">
                <p className="text-sm font-medium text-muted-foreground">DPA Notification Status</p>
                {dpaNotifiedAt ? (
                  <div className="flex items-center gap-2 text-green-700 dark:text-green-400">
                    <CheckCircle2 className="h-5 w-5" />
                    <span className="font-semibold">
                      Notified at {formatDateTime(dpaNotifiedAt)}
                    </span>
                  </div>
                ) : (
                  <div className="flex items-center gap-2 text-red-700 dark:text-red-400">
                    <AlertTriangle className="h-5 w-5" />
                    <span className="font-bold">Not notified</span>
                  </div>
                )}
              </div>

              {/* Notify button */}
              {!dpaNotifiedAt && (
                <Button
                  variant="destructive"
                  size="lg"
                  onClick={() => notifyDPA.mutate({ id })}
                  disabled={notifyDPA.isPending}
                  className="w-full"
                >
                  {notifyDPA.isPending ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Bell className="mr-2 h-4 w-4" />
                  )}
                  Record DPA Notification
                </Button>
              )}
            </div>

            {/* Right: Breach details */}
            <div className="space-y-4">
              <div>
                <p className="text-sm font-medium text-muted-foreground">Data Subjects Affected</p>
                <p className="text-2xl font-bold">
                  {((inc.data_subjects_affected as number) ?? 0).toLocaleString()}
                </p>
              </div>
              {(inc.data_categories as string[])?.length > 0 && (
                <div>
                  <p className="text-sm font-medium text-muted-foreground mb-2">Data Categories</p>
                  <div className="flex flex-wrap gap-1.5">
                    {(inc.data_categories as string[]).map((cat) => (
                      <Badge
                        key={cat}
                        variant="outline"
                        className="border-red-300 text-red-700 dark:border-red-600 dark:text-red-400"
                      >
                        {cat}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ============================================================= */}
      {/* NIS2 Section                                                   */}
      {/* ============================================================= */}
      {isNis2 && (
        <Card className="border-purple-300 dark:border-purple-700">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-purple-700 dark:text-purple-400">
              <FileWarning className="h-5 w-5" />
              NIS2 Directive — Incident Reporting
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Early Warning Status */}
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Early Warning Status</p>
                <p className="text-sm text-muted-foreground">
                  {inc.nis2_early_warning_at
                    ? `Submitted at ${formatDateTime(inc.nis2_early_warning_at as string)}`
                    : 'Not submitted yet'}
                </p>
              </div>
              {!inc.nis2_early_warning_at && (
                <Button
                  variant="outline"
                  onClick={() => nis2EarlyWarning.mutate({ id })}
                  disabled={nis2EarlyWarning.isPending}
                  className="border-purple-300 text-purple-700 hover:bg-purple-50 dark:border-purple-600 dark:text-purple-400 dark:hover:bg-purple-950/30"
                >
                  {nis2EarlyWarning.isPending ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Send className="mr-2 h-4 w-4" />
                  )}
                  Submit Early Warning
                </Button>
              )}
            </div>

            <Separator />

            {/* NIS2 Timeline */}
            <div>
              <p className="text-sm font-semibold mb-3">NIS2 Reporting Timeline</p>
              <div className="relative space-y-0">
                {[
                  {
                    label: 'Early Warning',
                    deadline: '24 hours',
                    description: 'Initial notification to CSIRT/competent authority',
                    done: !!inc.nis2_early_warning_at,
                    doneAt: inc.nis2_early_warning_at as string,
                  },
                  {
                    label: 'Incident Notification',
                    deadline: '72 hours',
                    description: 'Detailed incident notification with initial assessment',
                    done: !!inc.nis2_notification_at,
                    doneAt: inc.nis2_notification_at as string,
                  },
                  {
                    label: 'Final Report',
                    deadline: '1 month',
                    description: 'Comprehensive final report including root cause analysis',
                    done: !!inc.nis2_final_report_at,
                    doneAt: inc.nis2_final_report_at as string,
                  },
                ].map((step, idx) => (
                  <div key={step.label} className="flex gap-4 pb-4">
                    <div className="flex flex-col items-center">
                      <div
                        className={cn(
                          'flex h-8 w-8 items-center justify-center rounded-full border-2 text-xs font-bold',
                          step.done
                            ? 'border-green-500 bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-400'
                            : 'border-purple-300 bg-purple-50 text-purple-700 dark:bg-purple-950 dark:text-purple-400'
                        )}
                      >
                        {step.done ? (
                          <CheckCircle2 className="h-4 w-4" />
                        ) : (
                          idx + 1
                        )}
                      </div>
                      {idx < 2 && (
                        <div className="w-0.5 flex-1 bg-border mt-1" />
                      )}
                    </div>
                    <div className="flex-1 pb-2">
                      <div className="flex items-center gap-2">
                        <p className="font-semibold text-sm">{step.label}</p>
                        <Badge variant="outline" className="text-xs">
                          {step.deadline}
                        </Badge>
                      </div>
                      <p className="text-sm text-muted-foreground">{step.description}</p>
                      {step.done && step.doneAt && (
                        <p className="text-xs text-green-600 dark:text-green-400 mt-1">
                          Completed: {formatDateTime(step.doneAt)}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ============================================================= */}
      {/* Incident Details                                               */}
      {/* ============================================================= */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Incident Details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <p className="text-sm font-medium text-muted-foreground">Description</p>
              <p className="mt-1 text-sm whitespace-pre-wrap">
                {(inc.description as string) || 'No description provided.'}
              </p>
            </div>
            {inc.root_cause && (
              <div>
                <p className="text-sm font-medium text-muted-foreground">Root Cause</p>
                <p className="mt-1 text-sm whitespace-pre-wrap">{String(inc.root_cause)}</p>
              </div>
            )}
            {inc.impact && (
              <div>
                <p className="text-sm font-medium text-muted-foreground">Impact</p>
                <p className="mt-1 text-sm whitespace-pre-wrap">{String(inc.impact)}</p>
              </div>
            )}
            {inc.lessons_learned && (
              <div>
                <p className="text-sm font-medium text-muted-foreground">Lessons Learned</p>
                <p className="mt-1 text-sm whitespace-pre-wrap">{String(inc.lessons_learned)}</p>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Metadata</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <MetaRow label="Incident Type" value={inc.incident_type as string} />
            <MetaRow label="Category" value={inc.category as string} />
            <MetaRow label="Reported" value={formatDateTime(inc.reported_at as string ?? inc.created_at as string)} />
            {inc.resolved_at && <MetaRow label="Resolved" value={formatDateTime(inc.resolved_at as string)} />}
            {inc.closed_at && <MetaRow label="Closed" value={formatDateTime(inc.closed_at as string)} />}
            <MetaRow
              label="Reporter"
              value={
                (inc.reporter as Record<string, string>)
                  ? `${(inc.reporter as Record<string, string>).first_name} ${(inc.reporter as Record<string, string>).last_name}`
                  : '—'
              }
            />
            <MetaRow
              label="Assignee"
              value={
                (inc.assignee as Record<string, string>)
                  ? `${(inc.assignee as Record<string, string>).first_name} ${(inc.assignee as Record<string, string>).last_name}`
                  : '—'
              }
            />
          </CardContent>
        </Card>
      </div>

      {/* ============================================================= */}
      {/* Timeline                                                       */}
      {/* ============================================================= */}
      {(inc.timeline as Record<string, unknown>[])?.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Timeline</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
              {(inc.timeline as any[]).map((entry: any, idx: number) => (
                <div key={idx} className="flex gap-4">
                  <div className="flex flex-col items-center">
                    <div className="flex h-6 w-6 items-center justify-center rounded-full border bg-muted">
                      <Clock className="h-3 w-3 text-muted-foreground" />
                    </div>
                    {idx < (inc.timeline as Record<string, unknown>[]).length - 1 && (
                      <div className="w-0.5 flex-1 bg-border mt-1" />
                    )}
                  </div>
                  <div className="flex-1 pb-4">
                    <div className="flex items-center gap-2">
                      <p className="font-medium text-sm">{String(entry.action ?? (entry as Record<string, unknown>).event ?? "")}</p>
                      {entry.status && (
                        <Badge className={getStatusColor(entry.status as string)} >
                          {(entry.status as string).replace('_', ' ')}
                        </Badge>
                      )}
                    </div>
                    {entry.notes && (
                      <p className="text-sm text-muted-foreground mt-0.5">{String(entry.notes)}</p>
                    )}
                    <p className="text-xs text-muted-foreground mt-1">
                      {formatDateTime(entry.created_at as string ?? entry.timestamp as string)}
                      {entry.user && (
                        <span>
                          {' '}by {(entry.user as Record<string, string>)?.first_name}{' '}
                          {(entry.user as Record<string, string>)?.last_name}
                        </span>
                      )}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Metadata row helper
// ---------------------------------------------------------------------------

function MetaRow({ label, value }: { label: string; value: string | undefined | null }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value || '—'}</span>
    </div>
  );
}
