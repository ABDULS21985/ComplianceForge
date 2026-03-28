'use client';

import * as React from 'react';
import Link from 'next/link';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  UserCheck,
  Plus,
  Search,
  ChevronLeft,
  ChevronRight,
  Loader2,
  Clock,
  AlertTriangle,
  FileText,
  PieChart,
  CheckCircle2,
  XCircle,
  Timer,
} from 'lucide-react';

import { cn, formatDate } from '@/lib/utils';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import api from '@/lib/api';

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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';
import { Progress } from '@/components/ui/progress';

// ---------------------------------------------------------------------------
// Validation schema
// ---------------------------------------------------------------------------

const newRequestSchema = z.object({
  request_type: z.string().min(1, 'Request type is required'),
  data_subject_name: z.string().min(1, 'Name is required'),
  data_subject_email: z.string().email('Valid email is required'),
  data_subject_phone: z.string().optional(),
  data_subject_address: z.string().optional(),
  description: z.string().min(1, 'Description is required'),
  source: z.string().min(1, 'Source is required'),
  received_date: z.string().min(1, 'Received date is required'),
});

type NewRequestValues = z.infer<typeof newRequestSchema>;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DSR_TYPES = [
  { value: 'access', label: 'Right of Access (Art. 15)' },
  { value: 'rectification', label: 'Right to Rectification (Art. 16)' },
  { value: 'erasure', label: 'Right to Erasure (Art. 17)' },
  { value: 'restriction', label: 'Right to Restriction (Art. 18)' },
  { value: 'portability', label: 'Right to Data Portability (Art. 20)' },
  { value: 'objection', label: 'Right to Object (Art. 21)' },
  { value: 'automated_decision', label: 'Automated Decision Making (Art. 22)' },
];

const DSR_SOURCES = [
  { value: 'email', label: 'Email' },
  { value: 'web_form', label: 'Web Form' },
  { value: 'letter', label: 'Letter' },
  { value: 'phone', label: 'Phone' },
  { value: 'in_person', label: 'In Person' },
  { value: 'third_party', label: 'Third Party' },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function maskName(name: string): string {
  if (!name) return '---';
  const parts = name.split(' ');
  return parts
    .map((part) => {
      if (part.length <= 1) return part[0] + '***';
      return part[0] + '***' + ' ' + (parts.length > 1 ? '' : '');
    })
    .join('')
    .replace(/\s+/g, ' ')
    .trim();
}

function maskNameParts(name: string): string {
  if (!name) return '---';
  const parts = name.split(' ');
  if (parts.length === 1) return parts[0][0] + '***';
  return parts[0][0] + '*** ' + parts[parts.length - 1][0] + '**';
}

function getDaysRemaining(deadline: string | undefined | null): number | null {
  if (!deadline) return null;
  const diff = new Date(deadline).getTime() - Date.now();
  return Math.ceil(diff / (1000 * 60 * 60 * 24));
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

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function DSRPage() {
  const [page, setPage] = React.useState(1);
  const [pageSize] = React.useState(20);
  const [search, setSearch] = React.useState('');
  const [sheetOpen, setSheetOpen] = React.useState(false);

  // Dashboard stats
  const { data: dashboardData, isLoading: dashLoading } = useQuery({
    queryKey: ['dsr', 'dashboard'],
    queryFn: () => api.dsr.dashboard(),
    staleTime: 30 * 1000,
    refetchOnWindowFocus: true,
  });
  const dashboard = (dashboardData ?? {}) as Record<string, unknown>;

  // DSR list
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['dsr', 'list', { page, page_size: pageSize, search: search || undefined }],
    queryFn: () => api.dsr.list({ page, page_size: pageSize, search: search || undefined }),
  });

  const requests: Record<string, unknown>[] =
    (data as Record<string, unknown>)?.items as Record<string, unknown>[] ?? [];
  const total = ((data as Record<string, unknown>)?.total as number) ?? 0;
  const totalPages = ((data as Record<string, unknown>)?.total_pages as number) ?? 1;

  // Dashboard stats
  const byType = (dashboard.by_type ?? {}) as Record<string, number>;
  const byStatus = (dashboard.by_status ?? {}) as Record<string, number>;
  const slaRate = (dashboard.sla_compliance_rate as number) ?? 0;
  const avgDays = (dashboard.avg_completion_days as number) ?? 0;
  const totalRequests = (dashboard.total_requests as number) ?? 0;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">DSR Management</h1>
          <p className="text-muted-foreground">
            Manage GDPR Data Subject Requests. Track, process, and respond within regulatory deadlines.
          </p>
        </div>
        <Button onClick={() => setSheetOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          New Request
        </Button>
      </div>

      {/* Dashboard Summary */}
      <div className="grid gap-4 md:grid-cols-5">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Requests</CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <div className="text-2xl font-bold">{totalRequests}</div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">By Type (Top)</CardTitle>
            <PieChart className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-full" />
            ) : (
              <div className="space-y-1">
                {Object.entries(byType).slice(0, 3).map(([type, count]) => (
                  <div key={type} className="flex items-center justify-between text-xs">
                    <span className="capitalize">{type.replace('_', ' ')}</span>
                    <span className="font-semibold">{count}</span>
                  </div>
                ))}
                {Object.keys(byType).length === 0 && (
                  <p className="text-xs text-muted-foreground">No data</p>
                )}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">By Status</CardTitle>
            <CheckCircle2 className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-full" />
            ) : (
              <div className="space-y-1">
                {Object.entries(byStatus).slice(0, 3).map(([status, count]) => (
                  <div key={status} className="flex items-center justify-between text-xs">
                    <span className="capitalize">{status.replace('_', ' ')}</span>
                    <span className="font-semibold">{count}</span>
                  </div>
                ))}
                {Object.keys(byStatus).length === 0 && (
                  <p className="text-xs text-muted-foreground">No data</p>
                )}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">SLA Compliance</CardTitle>
            <Timer className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <>
                <div className={cn(
                  'text-2xl font-bold',
                  slaRate >= 90 ? 'text-green-600' : slaRate >= 70 ? 'text-yellow-600' : 'text-red-600'
                )}>
                  {slaRate.toFixed(1)}%
                </div>
                <Progress value={slaRate} className="mt-2 h-1.5" />
              </>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Avg. Completion</CardTitle>
            <Clock className="h-4 w-4 text-blue-500" />
          </CardHeader>
          <CardContent>
            {dashLoading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <div className="text-2xl font-bold">
                {avgDays.toFixed(1)} <span className="text-sm font-normal text-muted-foreground">days</span>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Search */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search requests..."
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
              Failed to load DSR requests: {(error as Error)?.message ?? 'Unknown error'}
            </div>
          ) : requests.length === 0 ? (
            <div className="p-12 text-center text-muted-foreground">
              <UserCheck className="mx-auto mb-3 h-10 w-10" />
              <p className="text-lg font-medium">No DSR requests found</p>
              <p className="text-sm">No data subject requests have been submitted yet.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Ref</th>
                    <th className="px-4 py-3 text-left font-medium">Type</th>
                    <th className="px-4 py-3 text-left font-medium">Subject</th>
                    <th className="px-4 py-3 text-left font-medium">Status</th>
                    <th className="px-4 py-3 text-left font-medium">Received</th>
                    <th className="px-4 py-3 text-left font-medium">Deadline</th>
                    <th className="px-4 py-3 text-left font-medium">Assigned To</th>
                    <th className="px-4 py-3 text-left font-medium">Days Left</th>
                    <th className="px-4 py-3 text-left font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {requests.map((req) => {
                    const daysLeft = getDaysRemaining(req.deadline as string);
                    const isOverdue = daysLeft !== null && daysLeft < 0;
                    const isUrgent = daysLeft !== null && daysLeft >= 0 && daysLeft < 7;
                    const status = req.status as string;

                    return (
                      <tr
                        key={req.id as string}
                        className={cn(
                          'border-b transition-colors hover:bg-muted/50',
                          isOverdue && status !== 'completed' && status !== 'rejected' && 'border-l-4 border-l-red-500'
                        )}
                      >
                        <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                          {req.request_ref as string}
                        </td>
                        <td className="px-4 py-3">
                          <Badge className={getTypeColor(req.request_type as string)}>
                            {(req.request_type as string)?.replace('_', ' ')}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 font-medium">
                          {maskNameParts(req.data_subject_name as string)}
                        </td>
                        <td className="px-4 py-3">
                          <Badge className={getRequestStatusColor(status)}>
                            {status?.replace('_', ' ')}
                          </Badge>
                        </td>
                        <td className="px-4 py-3">{formatDate(req.received_date as string)}</td>
                        <td className="px-4 py-3">
                          <span className={cn(
                            'font-medium',
                            isOverdue && status !== 'completed' && status !== 'rejected' && 'text-red-600 dark:text-red-400',
                            isUrgent && status !== 'completed' && status !== 'rejected' && 'text-amber-600 dark:text-amber-400',
                          )}>
                            {formatDate(req.deadline as string)}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-muted-foreground">
                          {(req.assigned_to_name as string) ?? '—'}
                        </td>
                        <td className="px-4 py-3">
                          {status === 'completed' || status === 'rejected' ? (
                            <span className="text-muted-foreground">—</span>
                          ) : daysLeft !== null ? (
                            <span className={cn(
                              'font-semibold',
                              isOverdue && 'text-red-600 dark:text-red-400',
                              isUrgent && !isOverdue && 'text-amber-600 dark:text-amber-400',
                              !isOverdue && !isUrgent && 'text-green-600 dark:text-green-400',
                            )}>
                              {isOverdue ? `${Math.abs(daysLeft)}d overdue` : `${daysLeft}d`}
                            </span>
                          ) : (
                            '—'
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <Link href={`/dsr/${req.id}`}>
                            <Button variant="ghost" size="sm">View</Button>
                          </Link>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination */}
          {totalPages > 1 && (
            <>
              <Separator />
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

      {/* New Request Sheet */}
      <NewRequestSheet open={sheetOpen} onOpenChange={setSheetOpen} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// New Request Sheet
// ---------------------------------------------------------------------------

function NewRequestSheet({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const qc = useQueryClient();
  const createRequest = useMutation({
    mutationFn: (data: unknown) => api.dsr.create(data),
    onSuccess: () => {
      toast.success('DSR request created.');
      qc.invalidateQueries({ queryKey: ['dsr'] });
    },
    onError: () => {
      toast.error('Failed to create DSR request.');
    },
  });

  const form = useForm<NewRequestValues>({
    resolver: zodResolver(newRequestSchema),
    defaultValues: {
      request_type: '',
      data_subject_name: '',
      data_subject_email: '',
      data_subject_phone: '',
      data_subject_address: '',
      description: '',
      source: '',
      received_date: new Date().toISOString().split('T')[0],
    },
  });

  const onSubmit = async (values: NewRequestValues) => {
    await createRequest.mutateAsync(values);
    form.reset();
    onOpenChange(false);
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="sm:max-w-lg overflow-y-auto">
        <SheetHeader>
          <SheetTitle>New Data Subject Request</SheetTitle>
          <SheetDescription>
            Record a new GDPR data subject request. A 30-day SLA deadline will be calculated automatically.
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 mt-6">
          {/* Request Type */}
          <div className="space-y-2">
            <Label>Request Type *</Label>
            <Controller
              control={form.control}
              name="request_type"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select request type" />
                  </SelectTrigger>
                  <SelectContent>
                    {DSR_TYPES.map((t) => (
                      <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            {form.formState.errors.request_type && (
              <p className="text-sm text-destructive">{form.formState.errors.request_type.message}</p>
            )}
          </div>

          {/* Data Subject Info */}
          <Separator />
          <p className="text-sm font-medium">Data Subject Information</p>

          <div className="space-y-2">
            <Label htmlFor="ds-name">Full Name *</Label>
            <Input id="ds-name" {...form.register('data_subject_name')} placeholder="John Doe" />
            {form.formState.errors.data_subject_name && (
              <p className="text-sm text-destructive">{form.formState.errors.data_subject_name.message}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="ds-email">Email *</Label>
            <Input id="ds-email" type="email" {...form.register('data_subject_email')} placeholder="john.doe@example.com" />
            {form.formState.errors.data_subject_email && (
              <p className="text-sm text-destructive">{form.formState.errors.data_subject_email.message}</p>
            )}
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="ds-phone">Phone</Label>
              <Input id="ds-phone" {...form.register('data_subject_phone')} placeholder="+44..." />
            </div>
            <div className="space-y-2">
              <Label htmlFor="ds-address">Address</Label>
              <Input id="ds-address" {...form.register('data_subject_address')} placeholder="Address" />
            </div>
          </div>

          {/* Description */}
          <div className="space-y-2">
            <Label htmlFor="ds-desc">Description *</Label>
            <Textarea
              id="ds-desc"
              {...form.register('description')}
              placeholder="Describe the request details..."
              rows={3}
            />
            {form.formState.errors.description && (
              <p className="text-sm text-destructive">{form.formState.errors.description.message}</p>
            )}
          </div>

          {/* Source */}
          <div className="space-y-2">
            <Label>Source *</Label>
            <Controller
              control={form.control}
              name="source"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue placeholder="How was the request received?" />
                  </SelectTrigger>
                  <SelectContent>
                    {DSR_SOURCES.map((s) => (
                      <SelectItem key={s.value} value={s.value}>{s.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            {form.formState.errors.source && (
              <p className="text-sm text-destructive">{form.formState.errors.source.message}</p>
            )}
          </div>

          {/* Received Date */}
          <div className="space-y-2">
            <Label htmlFor="ds-received">Received Date *</Label>
            <Input id="ds-received" type="date" {...form.register('received_date')} />
            {form.formState.errors.received_date && (
              <p className="text-sm text-destructive">{form.formState.errors.received_date.message}</p>
            )}
          </div>

          <SheetFooter className="pt-4">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={createRequest.isPending}>
              {createRequest.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Request
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
