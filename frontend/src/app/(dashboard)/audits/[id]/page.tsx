'use client';

import * as React from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  ArrowLeft,
  Plus,
  AlertTriangle,
  Loader2,
  Calendar,
  User,
  FileText,
} from 'lucide-react';

import { cn, formatDate, getStatusColor, getRiskLevelColor } from '@/lib/utils';
import {
  useAudit,
  useAuditFindings,
  useCreateFinding,
  useUsers,
} from '@/lib/api-hooks';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
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

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

const createFindingSchema = z.object({
  title: z.string().min(1, 'Title is required').max(200),
  description: z.string().min(1, 'Description is required'),
  severity: z.enum(['critical', 'high', 'medium', 'low', 'informational'], {
    required_error: 'Select severity',
  }),
  finding_type: z.string().min(1, 'Finding type is required'),
  control_id: z.string().optional(),
  root_cause: z.string().optional(),
  recommendation: z.string().min(1, 'Recommendation is required'),
  responsible_user_id: z.string().min(1, 'Responsible person is required'),
  due_date: z.string().min(1, 'Due date is required'),
});

type CreateFindingValues = z.infer<typeof createFindingSchema>;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function auditTypeBadge(type: string) {
  const map: Record<string, string> = {
    internal: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
    external: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400',
    certification: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-400',
  };
  return map[type] ?? 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
}

function isOverdue(dateStr: string | undefined | null): boolean {
  if (!dateStr) return false;
  return new Date(dateStr) < new Date();
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AuditDetailPage() {
  const params = useParams();
  const id = params.id as string;

  const { data: audit, isLoading, isError, error } = useAudit(id);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const a = audit as any;

  const [findingDialogOpen, setFindingDialogOpen] = React.useState(false);
  const [findingsPage, setFindingsPage] = React.useState(1);

  const { data: findingsData, isLoading: findingsLoading } = useAuditFindings(id, {
    page: findingsPage,
    page_size: 20,
  });

  const findings: Record<string, unknown>[] =
    (findingsData as Record<string, unknown>)?.items as Record<string, unknown>[] ?? [];
  const findingsTotalPages =
    ((findingsData as Record<string, unknown>)?.total_pages as number) ?? 1;

  // Loading state
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

  // Error state
  if (isError || !a) {
    return (
      <div className="space-y-4">
        <Link href="/audits" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="mr-1 h-4 w-4" /> Back to Audits
        </Link>
        <div className="text-center py-12 text-destructive">
          {isError ? `Failed to load audit: ${(error as Error)?.message ?? 'Unknown error'}` : 'Audit not found'}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link href="/audits" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="mr-1 h-4 w-4" /> Back to Audits
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <span className="font-mono text-sm text-muted-foreground">
              {a.audit_ref as string}
            </span>
            <Badge className={auditTypeBadge(a.audit_type as string)}>
              {(a.audit_type as string)?.replace('_', ' ')}
            </Badge>
            <Badge className={getStatusColor(a.status as string)}>
              {(a.status as string)?.replace('_', ' ')}
            </Badge>
          </div>
          <h1 className="text-3xl font-bold tracking-tight">{a.title as string}</h1>
        </div>
      </div>

      {/* Details section */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Audit Details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <p className="text-sm font-medium text-muted-foreground">Description</p>
              <p className="mt-1 text-sm whitespace-pre-wrap">{(a.description as string) || 'No description provided.'}</p>
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Scope</p>
              <p className="mt-1 text-sm whitespace-pre-wrap">{(a.scope as string) || '—'}</p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Schedule & Team</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-3">
              <Calendar className="h-4 w-4 text-muted-foreground" />
              <div>
                <p className="text-sm font-medium">
                  {formatDate(a.scheduled_start_date as string)} — {formatDate(a.scheduled_end_date as string)}
                </p>
                <p className="text-xs text-muted-foreground">Scheduled Period</p>
              </div>
            </div>
            {a.actual_start_date && (
              <div className="flex items-center gap-3">
                <Calendar className="h-4 w-4 text-muted-foreground" />
                <div>
                  <p className="text-sm font-medium">
                    {formatDate(a.actual_start_date as string)} — {formatDate(a.actual_end_date as string) || 'Ongoing'}
                  </p>
                  <p className="text-xs text-muted-foreground">Actual Period</p>
                </div>
              </div>
            )}
            <div className="flex items-center gap-3">
              <User className="h-4 w-4 text-muted-foreground" />
              <div>
                <p className="text-sm font-medium">
                  {(a.lead_auditor as Record<string, string>)?.first_name}{' '}
                  {(a.lead_auditor as Record<string, string>)?.last_name ?? '—'}
                </p>
                <p className="text-xs text-muted-foreground">Lead Auditor</p>
              </div>
            </div>
            {a.framework && (
              <div className="flex items-center gap-3">
                <FileText className="h-4 w-4 text-muted-foreground" />
                <div>
                  <p className="text-sm font-medium">{(a.framework as Record<string, string>)?.name}</p>
                  <p className="text-xs text-muted-foreground">Framework</p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Separator />

      {/* Findings section */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Findings</h2>
          <Button onClick={() => setFindingDialogOpen(true)} size="sm">
            <Plus className="mr-2 h-4 w-4" />
            Add Finding
          </Button>
        </div>

        <Card>
          <CardContent className="p-0">
            {findingsLoading ? (
              <div className="p-6 space-y-3">
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : findings.length === 0 ? (
              <div className="p-12 text-center text-muted-foreground">
                <AlertTriangle className="mx-auto mb-3 h-8 w-8" />
                <p className="text-lg font-medium">No findings yet</p>
                <p className="text-sm">Add findings discovered during the audit.</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b bg-muted/50">
                      <th className="px-4 py-3 text-left font-medium">Ref</th>
                      <th className="px-4 py-3 text-left font-medium">Title</th>
                      <th className="px-4 py-3 text-left font-medium">Severity</th>
                      <th className="px-4 py-3 text-left font-medium">Status</th>
                      <th className="px-4 py-3 text-left font-medium">Responsible</th>
                      <th className="px-4 py-3 text-left font-medium">Due Date</th>
                    </tr>
                  </thead>
                  <tbody>
                    {findings.map((f) => {
                      const overdue =
                        f.status !== 'closed' &&
                        f.status !== 'resolved' &&
                        isOverdue(f.due_date as string);
                      return (
                        <tr
                          key={f.id as string}
                          className="border-b transition-colors hover:bg-muted/50"
                        >
                          <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                            {f.finding_ref as string}
                          </td>
                          <td className="px-4 py-3 font-medium">{f.title as string}</td>
                          <td className="px-4 py-3">
                            <Badge className={getRiskLevelColor(f.severity as string)}>
                              {f.severity as string}
                            </Badge>
                          </td>
                          <td className="px-4 py-3">
                            <Badge className={getStatusColor(f.status as string)}>
                              {(f.status as string)?.replace('_', ' ')}
                            </Badge>
                          </td>
                          <td className="px-4 py-3">
                            {(f.responsible_user as Record<string, string>)?.first_name}{' '}
                            {(f.responsible_user as Record<string, string>)?.last_name ?? '—'}
                          </td>
                          <td
                            className={cn(
                              'px-4 py-3',
                              overdue && 'text-red-600 font-semibold dark:text-red-400'
                            )}
                          >
                            {formatDate(f.due_date as string)}
                            {overdue && (
                              <span className="ml-1 text-xs">(overdue)</span>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}

            {/* Findings pagination */}
            {findingsTotalPages > 1 && (
              <>
                <Separator />
                <div className="flex items-center justify-end gap-2 px-4 py-3">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={findingsPage <= 1}
                    onClick={() => setFindingsPage((p) => p - 1)}
                  >
                    Previous
                  </Button>
                  <span className="text-sm text-muted-foreground">
                    Page {findingsPage} of {findingsTotalPages}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={findingsPage >= findingsTotalPages}
                    onClick={() => setFindingsPage((p) => p + 1)}
                  >
                    Next
                  </Button>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Add Finding Dialog */}
      <AddFindingDialog
        auditId={id}
        open={findingDialogOpen}
        onOpenChange={setFindingDialogOpen}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Add Finding Dialog
// ---------------------------------------------------------------------------

function AddFindingDialog({
  auditId,
  open,
  onOpenChange,
}: {
  auditId: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const createFinding = useCreateFinding(auditId);
  const { data: usersData } = useUsers({ page: 1, page_size: 100 } as Record<string, unknown>);
  const users: Record<string, unknown>[] =
    (usersData as Record<string, unknown>)?.items as Record<string, unknown>[] ?? [];

  const form = useForm<CreateFindingValues>({
    resolver: zodResolver(createFindingSchema),
    defaultValues: {
      title: '',
      description: '',
      severity: undefined,
      finding_type: '',
      control_id: '',
      root_cause: '',
      recommendation: '',
      responsible_user_id: '',
      due_date: '',
    },
  });

  const onSubmit = async (values: CreateFindingValues) => {
    const payload = {
      ...values,
      control_id: values.control_id || undefined,
      root_cause: values.root_cause || undefined,
    };
    await createFinding.mutateAsync(payload);
    form.reset();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Add Finding</DialogTitle>
          <DialogDescription>
            Record a finding discovered during this audit.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          {/* Title */}
          <div className="space-y-2">
            <Label htmlFor="f-title">Title *</Label>
            <Input id="f-title" {...form.register('title')} placeholder="Finding title" />
            {form.formState.errors.title && (
              <p className="text-sm text-destructive">{form.formState.errors.title.message}</p>
            )}
          </div>

          {/* Description */}
          <div className="space-y-2">
            <Label htmlFor="f-desc">Description *</Label>
            <Textarea id="f-desc" {...form.register('description')} placeholder="Describe the finding in detail..." rows={3} />
            {form.formState.errors.description && (
              <p className="text-sm text-destructive">{form.formState.errors.description.message}</p>
            )}
          </div>

          {/* Severity & Finding Type */}
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Severity *</Label>
              <Select
                value={form.watch('severity')}
                onValueChange={(v) =>
                  form.setValue('severity', v as CreateFindingValues['severity'], { shouldValidate: true })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select severity" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="critical">Critical</SelectItem>
                  <SelectItem value="high">High</SelectItem>
                  <SelectItem value="medium">Medium</SelectItem>
                  <SelectItem value="low">Low</SelectItem>
                  <SelectItem value="informational">Informational</SelectItem>
                </SelectContent>
              </Select>
              {form.formState.errors.severity && (
                <p className="text-sm text-destructive">{form.formState.errors.severity.message}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="f-type">Finding Type *</Label>
              <Input id="f-type" {...form.register('finding_type')} placeholder="e.g. Non-conformity, Observation" />
              {form.formState.errors.finding_type && (
                <p className="text-sm text-destructive">{form.formState.errors.finding_type.message}</p>
              )}
            </div>
          </div>

          {/* Control ID (optional search) */}
          <div className="space-y-2">
            <Label htmlFor="f-control">Control ID (optional)</Label>
            <Input id="f-control" {...form.register('control_id')} placeholder="Enter control ID or search..." />
          </div>

          {/* Root Cause */}
          <div className="space-y-2">
            <Label htmlFor="f-root">Root Cause</Label>
            <Textarea id="f-root" {...form.register('root_cause')} placeholder="What caused this issue?" rows={2} />
          </div>

          {/* Recommendation */}
          <div className="space-y-2">
            <Label htmlFor="f-rec">Recommendation *</Label>
            <Textarea id="f-rec" {...form.register('recommendation')} placeholder="Recommended corrective action..." rows={2} />
            {form.formState.errors.recommendation && (
              <p className="text-sm text-destructive">{form.formState.errors.recommendation.message}</p>
            )}
          </div>

          {/* Responsible & Due Date */}
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Responsible Person *</Label>
              <Select
                value={form.watch('responsible_user_id')}
                onValueChange={(v) => form.setValue('responsible_user_id', v, { shouldValidate: true })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select person" />
                </SelectTrigger>
                <SelectContent>
                  {users.map((u) => (
                    <SelectItem key={u.id as string} value={u.id as string}>
                      {u.first_name as string} {u.last_name as string}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {form.formState.errors.responsible_user_id && (
                <p className="text-sm text-destructive">{form.formState.errors.responsible_user_id.message}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="f-due">Due Date *</Label>
              <Input id="f-due" type="date" {...form.register('due_date')} />
              {form.formState.errors.due_date && (
                <p className="text-sm text-destructive">{form.formState.errors.due_date.message}</p>
              )}
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={createFinding.isPending}>
              {createFinding.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Add Finding
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
