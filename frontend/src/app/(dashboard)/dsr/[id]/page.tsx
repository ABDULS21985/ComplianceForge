'use client';

import * as React from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import {
  ArrowLeft,
  CheckCircle2,
  Clock,
  Loader2,
  ShieldCheck,
  XCircle,
  CalendarPlus,
  Send,
  UserCheck,
  AlertTriangle,
  FileText,
} from 'lucide-react';

import { cn, formatDate, formatDateTime } from '@/lib/utils';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import api from '@/lib/api';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';
import { Progress } from '@/components/ui/progress';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getDaysRemaining(deadline: string | undefined | null): number | null {
  if (!deadline) return null;
  const diff = new Date(deadline).getTime() - Date.now();
  return Math.ceil(diff / (1000 * 60 * 60 * 24));
}

function getSLAProgress(received: string | undefined | null, deadline: string | undefined | null): number {
  if (!received || !deadline) return 0;
  const start = new Date(received).getTime();
  const end = new Date(deadline).getTime();
  const now = Date.now();
  const totalDuration = end - start;
  if (totalDuration <= 0) return 100;
  const elapsed = now - start;
  return Math.min(100, Math.max(0, (elapsed / totalDuration) * 100));
}

function getTypeColor(type: string): string {
  switch (type) {
    case 'access': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'rectification': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'erasure': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'restriction': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
    case 'portability': return 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400';
    case 'objection': return 'bg-pink-100 text-pink-800 dark:bg-pink-900/30 dark:text-pink-400';
    case 'automated_decision': return 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/30 dark:text-indigo-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function getRequestStatusColor(status: string): string {
  switch (status) {
    case 'completed': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'in_progress': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'pending': case 'received': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'overdue': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'rejected': return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
    case 'extended': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function getTaskStatusColor(status: string): string {
  switch (status) {
    case 'completed': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'in_progress': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'pending': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'skipped': return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function DSRDetailPage() {
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;
  const qc = useQueryClient();

  const [extendOpen, setExtendOpen] = React.useState(false);
  const [extendReason, setExtendReason] = React.useState('');
  const [completeOpen, setCompleteOpen] = React.useState(false);
  const [responseMethod, setResponseMethod] = React.useState('email');
  const [rejectOpen, setRejectOpen] = React.useState(false);
  const [rejectReason, setRejectReason] = React.useState('');
  const [rejectBasis, setRejectBasis] = React.useState('');

  // Fetch request
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['dsr', id],
    queryFn: () => api.dsr.get(id),
    enabled: !!id,
  });

  const request = (data ?? {}) as any;
  const tasks: Record<string, unknown>[] = (request.tasks as any[]) ?? [];
  const auditTrail: Record<string, unknown>[] = (request.audit_trail as any[]) ?? [];
  const status = request.status as string;
  const daysLeft = getDaysRemaining(request.deadline as string);
  const isOverdue = daysLeft !== null && daysLeft < 0;
  const slaProgress = getSLAProgress(request.received_date as string, request.deadline as string);

  // Mutations
  const verifyIdentity = useMutation({
    mutationFn: (payload: unknown) => api.dsr.verifyIdentity(id, payload),
    onSuccess: () => {
      toast.success('Identity verified.');
      qc.invalidateQueries({ queryKey: ['dsr', id] });
    },
    onError: () => toast.error('Failed to verify identity.'),
  });

  const extendDeadline = useMutation({
    mutationFn: (payload: unknown) => api.dsr.extend(id, payload),
    onSuccess: () => {
      toast.success('Deadline extended.');
      setExtendOpen(false);
      setExtendReason('');
      qc.invalidateQueries({ queryKey: ['dsr', id] });
    },
    onError: () => toast.error('Failed to extend deadline.'),
  });

  const completeRequest = useMutation({
    mutationFn: (payload: unknown) => api.dsr.complete(id, payload),
    onSuccess: () => {
      toast.success('Request completed.');
      setCompleteOpen(false);
      qc.invalidateQueries({ queryKey: ['dsr'] });
    },
    onError: () => toast.error('Failed to complete request.'),
  });

  const rejectRequest = useMutation({
    mutationFn: (payload: unknown) => api.dsr.reject(id, payload),
    onSuccess: () => {
      toast.success('Request rejected.');
      setRejectOpen(false);
      qc.invalidateQueries({ queryKey: ['dsr'] });
    },
    onError: () => toast.error('Failed to reject request.'),
  });

  const completeTask = useMutation({
    mutationFn: ({ taskId }: { taskId: string }) =>
      api.dsr.updateTask(id, taskId, { status: 'completed' }),
    onSuccess: () => {
      toast.success('Task completed.');
      qc.invalidateQueries({ queryKey: ['dsr', id] });
    },
    onError: () => toast.error('Failed to complete task.'),
  });

  // Loading state
  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  // Error state
  if (isError) {
    return (
      <div className="space-y-6">
        <Link href="/dsr">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" /> Back to DSR
          </Button>
        </Link>
        <div className="p-6 text-center text-destructive">
          Failed to load DSR request: {(error as Error)?.message ?? 'Unknown error'}
        </div>
      </div>
    );
  }

  // Empty / not found
  if (!request.id) {
    return (
      <div className="space-y-6">
        <Link href="/dsr">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" /> Back to DSR
          </Button>
        </Link>
        <div className="p-12 text-center text-muted-foreground">
          <FileText className="mx-auto mb-3 h-10 w-10" />
          <p className="text-lg font-medium">Request not found</p>
        </div>
      </div>
    );
  }

  const isTerminal = status === 'completed' || status === 'rejected';

  return (
    <div className="space-y-6">
      {/* Back + Header */}
      <div className="flex items-center gap-4">
        <Link href="/dsr">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" /> Back
          </Button>
        </Link>
      </div>

      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">
              {request.request_ref as string}
            </h1>
            <Badge className={getTypeColor(request.request_type as string)}>
              {(request.request_type as string)?.replace('_', ' ')}
            </Badge>
            <Badge className={getRequestStatusColor(status)}>
              {status?.replace('_', ' ')}
            </Badge>
            {request.priority && (
              <Badge variant="outline">{request.priority as string}</Badge>
            )}
          </div>
          <p className="mt-1 text-muted-foreground">
            {request.description as string}
          </p>
        </div>
      </div>

      {/* SLA Countdown */}
      <Card className={cn(
        isOverdue && !isTerminal && 'border-red-300 dark:border-red-700',
      )}>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Clock className="h-4 w-4" />
            SLA Countdown
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between mb-2">
            <div>
              <span className="text-sm text-muted-foreground">Deadline: </span>
              <span className="font-medium">{formatDate(request.deadline as string)}</span>
            </div>
            <div>
              {isTerminal ? (
                <span className="text-sm text-muted-foreground">
                  {status === 'completed' ? 'Completed' : 'Rejected'}
                </span>
              ) : daysLeft !== null ? (
                <span className={cn(
                  'text-lg font-bold',
                  isOverdue && 'text-red-600 dark:text-red-400',
                  !isOverdue && daysLeft < 7 && 'text-amber-600 dark:text-amber-400',
                  !isOverdue && daysLeft >= 7 && 'text-green-600 dark:text-green-400',
                )}>
                  {isOverdue ? `${Math.abs(daysLeft)} days overdue` : `${daysLeft} days remaining`}
                </span>
              ) : (
                <span className="text-muted-foreground">--</span>
              )}
            </div>
          </div>
          <Progress
            value={isTerminal ? 100 : slaProgress}
            className={cn(
              'h-2',
              isOverdue && !isTerminal && '[&>div]:bg-red-500',
              !isOverdue && slaProgress > 70 && !isTerminal && '[&>div]:bg-amber-500',
            )}
          />
          <div className="flex justify-between mt-1 text-xs text-muted-foreground">
            <span>Received: {formatDate(request.received_date as string)}</span>
            <span>Deadline: {formatDate(request.deadline as string)}</span>
          </div>
        </CardContent>
      </Card>

      {/* Action Buttons */}
      {!isTerminal && (
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            onClick={() => verifyIdentity.mutate({ verified: true })}
            disabled={verifyIdentity.isPending}
          >
            {verifyIdentity.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <ShieldCheck className="mr-2 h-4 w-4" />
            )}
            Verify Identity
          </Button>

          <Button
            variant="outline"
            onClick={() => setExtendOpen(true)}
          >
            <CalendarPlus className="mr-2 h-4 w-4" />
            Extend Deadline
          </Button>

          <Button
            onClick={() => setCompleteOpen(true)}
          >
            <CheckCircle2 className="mr-2 h-4 w-4" />
            Complete Request
          </Button>

          <Button
            variant="destructive"
            onClick={() => setRejectOpen(true)}
          >
            <XCircle className="mr-2 h-4 w-4" />
            Reject
          </Button>
        </div>
      )}

      {/* Task Checklist */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Task Checklist</CardTitle>
        </CardHeader>
        <CardContent>
          {tasks.length === 0 ? (
            <p className="text-sm text-muted-foreground">No tasks defined for this request.</p>
          ) : (
            <div className="space-y-3">
              {tasks.map((task: any, idx) => {
                const taskStatus = task.status as string;
                const isComplete = taskStatus === 'completed';
                return (
                  <div
                    key={(task.id as string) ?? idx}
                    className={cn(
                      'flex items-center justify-between rounded-md border p-3',
                      isComplete && 'bg-muted/50',
                    )}
                  >
                    <div className="flex items-center gap-3">
                      <div className={cn(
                        'flex h-6 w-6 items-center justify-center rounded-full border text-xs font-bold',
                        isComplete
                          ? 'border-green-500 bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                          : 'border-muted-foreground/30',
                      )}>
                        {isComplete ? (
                          <CheckCircle2 className="h-4 w-4" />
                        ) : (
                          idx + 1
                        )}
                      </div>
                      <div>
                        <p className={cn('text-sm font-medium', isComplete && 'line-through text-muted-foreground')}>
                          {task.title as string}
                        </p>
                        <div className="flex items-center gap-2 mt-0.5">
                          {task.assignee_name && (
                            <span className="text-xs text-muted-foreground">
                              {task.assignee_name as string}
                            </span>
                          )}
                          <Badge className={cn('text-[10px] px-1.5 py-0', getTaskStatusColor(taskStatus))}>
                            {taskStatus?.replace('_', ' ')}
                          </Badge>
                          {task.completed_at && (
                            <span className="text-xs text-muted-foreground">
                              {formatDate(task.completed_at as string)}
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                    {!isComplete && !isTerminal && (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={completeTask.isPending}
                        onClick={() => completeTask.mutate({ taskId: task.id as string })}
                      >
                        {completeTask.isPending ? (
                          <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                        ) : (
                          <CheckCircle2 className="mr-1 h-3 w-3" />
                        )}
                        Complete
                      </Button>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Audit Trail */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Audit Trail</CardTitle>
        </CardHeader>
        <CardContent>
          {auditTrail.length === 0 ? (
            <p className="text-sm text-muted-foreground">No audit events recorded.</p>
          ) : (
            <div className="relative border-l-2 border-muted-foreground/20 ml-3 space-y-4">
              {auditTrail.map((event: any, idx: number) => (
                <div key={idx} className="relative pl-6">
                  <div className="absolute left-[-5px] top-1.5 h-2 w-2 rounded-full bg-muted-foreground/60" />
                  <div className="flex items-baseline gap-2">
                    <span className="text-sm font-medium">
                      {event.action as string}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {formatDateTime(event.created_at as string ?? event.timestamp as string)}
                    </span>
                  </div>
                  {event.user_name && (
                    <p className="text-xs text-muted-foreground">by {event.user_name as string}</p>
                  )}
                  {event.details && (
                    <p className="text-xs text-muted-foreground mt-0.5">{event.details as string}</p>
                  )}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Extend Deadline Dialog */}
      <Dialog open={extendOpen} onOpenChange={setExtendOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Extend Deadline</DialogTitle>
            <DialogDescription>
              Under GDPR, the deadline can be extended by up to 2 months for complex requests. Provide the reason for extension.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <Label htmlFor="extend-reason">Reason for Extension *</Label>
            <Textarea
              id="extend-reason"
              value={extendReason}
              onChange={(e) => setExtendReason(e.target.value)}
              placeholder="e.g. Complex request involving multiple data systems..."
              rows={3}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExtendOpen(false)}>Cancel</Button>
            <Button
              disabled={!extendReason.trim() || extendDeadline.isPending}
              onClick={() => extendDeadline.mutate({ reason: extendReason })}
            >
              {extendDeadline.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Extend Deadline
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Complete Request Dialog */}
      <Dialog open={completeOpen} onOpenChange={setCompleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Complete Request</DialogTitle>
            <DialogDescription>
              Mark this DSR as completed. Select the response delivery method.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <Label>Response Method</Label>
            <Select value={responseMethod} onValueChange={setResponseMethod}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="email">Email</SelectItem>
                <SelectItem value="post">Post</SelectItem>
                <SelectItem value="portal">Self-Service Portal</SelectItem>
                <SelectItem value="in_person">In Person</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCompleteOpen(false)}>Cancel</Button>
            <Button
              disabled={completeRequest.isPending}
              onClick={() => completeRequest.mutate({ response_method: responseMethod })}
            >
              {completeRequest.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Complete Request
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reject Request Dialog */}
      <Dialog open={rejectOpen} onOpenChange={setRejectOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reject Request</DialogTitle>
            <DialogDescription>
              Provide a reason and legal basis for rejecting this data subject request.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="reject-reason">Reason *</Label>
              <Textarea
                id="reject-reason"
                value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                placeholder="Reason for rejection..."
                rows={3}
              />
            </div>
            <div className="space-y-2">
              <Label>Legal Basis *</Label>
              <Select value={rejectBasis} onValueChange={setRejectBasis}>
                <SelectTrigger>
                  <SelectValue placeholder="Select legal basis" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="manifestly_unfounded">Manifestly Unfounded (Art. 12(5))</SelectItem>
                  <SelectItem value="excessive">Excessive / Repetitive (Art. 12(5))</SelectItem>
                  <SelectItem value="identity_unverified">Identity Not Verified</SelectItem>
                  <SelectItem value="legal_obligation">Legal Obligation Exemption</SelectItem>
                  <SelectItem value="public_interest">Public Interest Exemption</SelectItem>
                  <SelectItem value="legal_claims">Legal Claims Exemption</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRejectOpen(false)}>Cancel</Button>
            <Button
              variant="destructive"
              disabled={!rejectReason.trim() || !rejectBasis || rejectRequest.isPending}
              onClick={() => rejectRequest.mutate({ reason: rejectReason, legal_basis: rejectBasis })}
            >
              {rejectRequest.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Reject Request
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
