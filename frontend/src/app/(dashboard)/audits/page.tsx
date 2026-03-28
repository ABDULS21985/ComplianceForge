'use client';

import * as React from 'react';
import Link from 'next/link';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  ClipboardCheck,
  Plus,
  Search,
  ChevronLeft,
  ChevronRight,
  AlertTriangle,
  Loader2,
} from 'lucide-react';

import { cn, formatDate, getStatusColor } from '@/lib/utils';
import {
  useAudits,
  useCreateAudit,
  useUsers,
  useFrameworks,
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


// ---------------------------------------------------------------------------
// Validation schema
// ---------------------------------------------------------------------------

const createAuditSchema = z.object({
  title: z.string().min(1, 'Title is required').max(200),
  description: z.string().min(1, 'Description is required'),
  audit_type: z.enum(['internal', 'external', 'certification'], {
    required_error: 'Select an audit type',
  }),
  lead_auditor_id: z.string().min(1, 'Lead auditor is required'),
  scope: z.string().min(1, 'Scope is required'),
  scheduled_start_date: z.string().min(1, 'Start date is required'),
  scheduled_end_date: z.string().min(1, 'End date is required'),
  framework_id: z.string().optional(),
});

type CreateAuditValues = z.infer<typeof createAuditSchema>;

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

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AuditsPage() {
  const [page, setPage] = React.useState(1);
  const [pageSize] = React.useState(20);
  const [search, setSearch] = React.useState('');
  const [sheetOpen, setSheetOpen] = React.useState(false);

  const { data, isLoading, isError, error } = useAudits({
    page,
    page_size: pageSize,
    search: search || undefined,
  } as Record<string, unknown>);

  const audits: Record<string, unknown>[] = (data as Record<string, unknown>)?.items as Record<string, unknown>[] ?? [];
  const total = ((data as Record<string, unknown>)?.total as number) ?? 0;
  const totalPages = ((data as Record<string, unknown>)?.total_pages as number) ?? 1;

  // Summary counts
  const planned = audits.filter((a) => a.status === 'planned').length;
  const inProgress = audits.filter((a) => a.status === 'in_progress').length;
  const completed = audits.filter((a) => a.status === 'completed').length;
  const totalFindings = audits.reduce(
    (sum, a) => sum + (((a.findings_count as number) ?? 0)),
    0
  );
  const criticalOpen = audits.reduce(
    (sum, a) => sum + (((a.critical_findings_open as number) ?? 0)),
    0
  );

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Audit Management</h1>
          <p className="text-muted-foreground">
            Plan, execute, and track internal and external audits
          </p>
        </div>
        <Button onClick={() => setSheetOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Plan Audit
        </Button>
      </div>

      {/* Summary cards */}
      <div className="grid gap-4 md:grid-cols-5">
        <SummaryCard label="Planned" value={planned} icon={<ClipboardCheck className="h-4 w-4 text-blue-500" />} />
        <SummaryCard label="In Progress" value={inProgress} icon={<Loader2 className="h-4 w-4 text-yellow-500" />} />
        <SummaryCard label="Completed" value={completed} icon={<ClipboardCheck className="h-4 w-4 text-green-500" />} />
        <SummaryCard label="Total Findings" value={totalFindings} icon={<AlertTriangle className="h-4 w-4 text-orange-500" />} />
        <SummaryCard
          label="Critical Findings Open"
          value={criticalOpen}
          icon={<AlertTriangle className="h-4 w-4 text-red-500" />}
          highlight={criticalOpen > 0}
        />
      </div>

      {/* Search */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search audits..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
            className="pl-10"
          />
        </div>
      </div>

      {/* Table */}
      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : isError ? (
            <div className="p-6 text-center text-destructive">
              Failed to load audits: {(error as Error)?.message ?? 'Unknown error'}
            </div>
          ) : audits.length === 0 ? (
            <div className="p-12 text-center text-muted-foreground">
              <ClipboardCheck className="mx-auto mb-3 h-10 w-10" />
              <p className="text-lg font-medium">No audits found</p>
              <p className="text-sm">Create your first audit to get started.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Ref</th>
                    <th className="px-4 py-3 text-left font-medium">Title</th>
                    <th className="px-4 py-3 text-left font-medium">Type</th>
                    <th className="px-4 py-3 text-left font-medium">Status</th>
                    <th className="px-4 py-3 text-left font-medium">Lead Auditor</th>
                    <th className="px-4 py-3 text-left font-medium">Start Date</th>
                    <th className="px-4 py-3 text-left font-medium">End Date</th>
                    <th className="px-4 py-3 text-left font-medium">Findings</th>
                  </tr>
                </thead>
                <tbody>
                  {audits.map((audit) => (
                    <tr
                      key={audit.id as string}
                      className="border-b transition-colors hover:bg-muted/50"
                    >
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                        {audit.audit_ref as string}
                      </td>
                      <td className="px-4 py-3 font-medium">
                        <Link
                          href={`/audits/${audit.id}`}
                          className="text-primary hover:underline"
                        >
                          {audit.title as string}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <Badge className={auditTypeBadge(audit.audit_type as string)}>
                          {(audit.audit_type as string)?.replace('_', ' ')}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        <Badge className={getStatusColor(audit.status as string)}>
                          {(audit.status as string)?.replace('_', ' ')}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        {(audit.lead_auditor as Record<string, string>)?.first_name}{' '}
                        {(audit.lead_auditor as Record<string, string>)?.last_name ?? (audit.lead_auditor_id as string)?.slice(0, 8)}
                      </td>
                      <td className="px-4 py-3">{formatDate(audit.scheduled_start_date as string)}</td>
                      <td className="px-4 py-3">{formatDate(audit.scheduled_end_date as string)}</td>
                      <td className="px-4 py-3">
                        <span className="font-medium">{(audit.findings_count as number) ?? 0}</span>
                        {((audit.critical_findings_open as number) ?? 0) > 0 && (
                          <span className="ml-1 text-red-600 font-semibold">
                            / {audit.critical_findings_open as number} critical
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination */}
          {totalPages > 1 && (
            <>
              <div className="border-t" />
              <div className="flex items-center justify-between px-4 py-3">
                <p className="text-sm text-muted-foreground">
                  Showing page {page} of {totalPages} ({total} total)
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={page <= 1}
                    onClick={() => setPage((p) => p - 1)}
                  >
                    <ChevronLeft className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={page >= totalPages}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* Create Audit Sheet/Dialog */}
      <CreateAuditSheet open={sheetOpen} onOpenChange={setSheetOpen} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Summary card component
// ---------------------------------------------------------------------------

function SummaryCard({
  label,
  value,
  icon,
  highlight,
}: {
  label: string;
  value: number;
  icon: React.ReactNode;
  highlight?: boolean;
}) {
  return (
    <Card className={cn(highlight && 'border-red-300 dark:border-red-700')}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{label}</CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        <div className={cn('text-2xl font-bold', highlight && 'text-red-600 dark:text-red-400')}>
          {value}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Create Audit Sheet (Dialog acting as side-sheet)
// ---------------------------------------------------------------------------

function CreateAuditSheet({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const createAudit = useCreateAudit();
  const { data: usersData } = useUsers({ page: 1, page_size: 100 } as Record<string, unknown>);
  const { data: frameworksData } = useFrameworks({ page: 1, page_size: 100 });

  const users: Record<string, unknown>[] =
    (usersData as Record<string, unknown>)?.items as Record<string, unknown>[] ?? [];
  const frameworks: Record<string, unknown>[] =
    (frameworksData as Record<string, unknown>)?.items as Record<string, unknown>[] ?? [];

  const form = useForm<CreateAuditValues>({
    resolver: zodResolver(createAuditSchema),
    defaultValues: {
      title: '',
      description: '',
      audit_type: undefined,
      lead_auditor_id: '',
      scope: '',
      scheduled_start_date: '',
      scheduled_end_date: '',
      framework_id: '',
    },
  });

  const onSubmit = async (values: CreateAuditValues) => {
    const payload = {
      ...values,
      framework_id: values.framework_id || undefined,
    };
    await createAudit.mutateAsync(payload);
    form.reset();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Plan New Audit</DialogTitle>
          <DialogDescription>
            Fill in the details to schedule a new audit.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          {/* Title */}
          <div className="space-y-2">
            <Label htmlFor="title">Title *</Label>
            <Input id="title" {...form.register('title')} placeholder="e.g. ISO 27001 Annual Audit 2026" />
            {form.formState.errors.title && (
              <p className="text-sm text-destructive">{form.formState.errors.title.message}</p>
            )}
          </div>

          {/* Description */}
          <div className="space-y-2">
            <Label htmlFor="description">Description *</Label>
            <Textarea id="description" {...form.register('description')} placeholder="Describe the audit objectives..." rows={3} />
            {form.formState.errors.description && (
              <p className="text-sm text-destructive">{form.formState.errors.description.message}</p>
            )}
          </div>

          {/* Audit Type */}
          <div className="space-y-2">
            <Label>Audit Type *</Label>
            <Select
              value={form.watch('audit_type')}
              onValueChange={(v) => form.setValue('audit_type', v as CreateAuditValues['audit_type'], { shouldValidate: true })}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select audit type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="internal">Internal</SelectItem>
                <SelectItem value="external">External</SelectItem>
                <SelectItem value="certification">Certification</SelectItem>
              </SelectContent>
            </Select>
            {form.formState.errors.audit_type && (
              <p className="text-sm text-destructive">{form.formState.errors.audit_type.message}</p>
            )}
          </div>

          {/* Lead Auditor */}
          <div className="space-y-2">
            <Label>Lead Auditor *</Label>
            <Select
              value={form.watch('lead_auditor_id')}
              onValueChange={(v) => form.setValue('lead_auditor_id', v, { shouldValidate: true })}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select lead auditor" />
              </SelectTrigger>
              <SelectContent>
                {users.map((u) => (
                  <SelectItem key={u.id as string} value={u.id as string}>
                    {u.first_name as string} {u.last_name as string}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {form.formState.errors.lead_auditor_id && (
              <p className="text-sm text-destructive">{form.formState.errors.lead_auditor_id.message}</p>
            )}
          </div>

          {/* Scope */}
          <div className="space-y-2">
            <Label htmlFor="scope">Scope *</Label>
            <Textarea id="scope" {...form.register('scope')} placeholder="Define the audit scope..." rows={3} />
            {form.formState.errors.scope && (
              <p className="text-sm text-destructive">{form.formState.errors.scope.message}</p>
            )}
          </div>

          {/* Dates */}
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="scheduled_start_date">Start Date *</Label>
              <Input id="scheduled_start_date" type="date" {...form.register('scheduled_start_date')} />
              {form.formState.errors.scheduled_start_date && (
                <p className="text-sm text-destructive">{form.formState.errors.scheduled_start_date.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="scheduled_end_date">End Date *</Label>
              <Input id="scheduled_end_date" type="date" {...form.register('scheduled_end_date')} />
              {form.formState.errors.scheduled_end_date && (
                <p className="text-sm text-destructive">{form.formState.errors.scheduled_end_date.message}</p>
              )}
            </div>
          </div>

          {/* Framework (optional) */}
          <div className="space-y-2">
            <Label>Framework (optional)</Label>
            <Select
              value={form.watch('framework_id') ?? ''}
              onValueChange={(v) => form.setValue('framework_id', v === '__none__' ? '' : v)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select framework" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">None</SelectItem>
                {frameworks.map((fw) => (
                  <SelectItem key={fw.id as string} value={fw.id as string}>
                    {fw.name as string}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={createAudit.isPending}>
              {createAudit.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Audit
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
