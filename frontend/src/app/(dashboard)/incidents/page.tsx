'use client';

import * as React from 'react';
import Link from 'next/link';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  AlertOctagon,
  AlertTriangle,
  Plus,
  Search,
  ChevronLeft,
  ChevronRight,
  ShieldAlert,
  Clock,
  Loader2,
  Bell,
  Database,
} from 'lucide-react';

import { cn, formatDate, formatDateTime, getStatusColor, getRiskLevelColor } from '@/lib/utils';
import {
  useIncidents,
  useIncidentStats,
  useUrgentBreaches,
  useCreateIncident,
  useNotifyDPA,
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
// Validation schema
// ---------------------------------------------------------------------------

const reportIncidentSchema = z
  .object({
    title: z.string().min(1, 'Title is required').max(200),
    description: z.string().min(1, 'Description is required'),
    incident_type: z.string().min(1, 'Incident type is required'),
    severity: z.enum(['critical', 'high', 'medium', 'low'], {
      required_error: 'Select severity',
    }),
    category: z.string().min(1, 'Category is required'),
    is_data_breach: z.boolean().default(false),
    data_subjects_affected: z.number().optional(),
    data_categories: z.array(z.string()).optional(),
  })
  .refine(
    (data) => {
      if (data.is_data_breach && (!data.data_subjects_affected || data.data_subjects_affected < 1)) {
        return false;
      }
      return true;
    },
    {
      message: 'Number of affected data subjects is required for data breaches',
      path: ['data_subjects_affected'],
    }
  );

type ReportIncidentValues = z.infer<typeof reportIncidentSchema>;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DATA_CATEGORIES = [
  'Names',
  'Email addresses',
  'Phone numbers',
  'Financial data',
  'Health data',
  'Biometric data',
  'Location data',
  'National ID numbers',
  'Login credentials',
  'IP addresses',
  'Genetic data',
  'Political opinions',
  'Religious beliefs',
  'Trade union membership',
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function hoursRemaining(deadline: string | undefined | null): number | null {
  if (!deadline) return null;
  const diff = new Date(deadline).getTime() - Date.now();
  return Math.max(0, diff / (1000 * 60 * 60));
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function IncidentsPage() {
  const [page, setPage] = React.useState(1);
  const [pageSize] = React.useState(20);
  const [search, setSearch] = React.useState('');
  const [sheetOpen, setSheetOpen] = React.useState(false);

  // Urgent breaches polling
  const { data: urgentBreachesData } = useUrgentBreaches();
  const urgentBreaches: Record<string, unknown>[] =
    (Array.isArray(urgentBreachesData)
      ? urgentBreachesData
      : (urgentBreachesData as Record<string, unknown>)?.items) as Record<string, unknown>[] ?? [];

  // Stats
  const { data: statsData } = useIncidentStats();
  const stats = (statsData ?? {}) as Record<string, number>;

  // Incident list
  const { data, isLoading, isError, error } = useIncidents({
    page,
    page_size: pageSize,
    search: search || undefined,
  } as Record<string, unknown>);

  const incidents: Record<string, unknown>[] =
    (data as Record<string, unknown>)?.items as Record<string, unknown>[] ?? [];
  const total = ((data as Record<string, unknown>)?.total as number) ?? 0;
  const totalPages = ((data as Record<string, unknown>)?.total_pages as number) ?? 1;

  const notifyDPA = useNotifyDPA();

  return (
    <div className="space-y-6">
      {/* GDPR Breach Alert Banner */}
      {urgentBreaches.length > 0 && (
        <div className="rounded-lg border-2 border-red-500 bg-red-50 p-4 dark:border-red-700 dark:bg-red-950/30">
          <div className="flex items-center gap-2 mb-3">
            <ShieldAlert className="h-5 w-5 text-red-600 dark:text-red-400" />
            <h2 className="text-lg font-bold text-red-700 dark:text-red-400">
              GDPR Breach Alert — Urgent DPA Notification Required
            </h2>
          </div>
          <p className="text-sm text-red-600 dark:text-red-400 mb-4">
            The following data breaches require notification to the Data Protection Authority within 72 hours (GDPR Article 33).
          </p>
          <div className="space-y-3">
            {urgentBreaches.map((breach) => {
              const hours = hoursRemaining(breach.notification_deadline as string);
              const isUrgent = hours !== null && hours < 24;
              const isExpired = hours !== null && hours === 0;
              return (
                <div
                  key={breach.id as string}
                  className={cn(
                    'flex items-center justify-between rounded-md border p-3',
                    isExpired
                      ? 'border-red-700 bg-red-100 dark:bg-red-950'
                      : isUrgent
                        ? 'border-red-400 bg-red-50 dark:bg-red-950/50'
                        : 'border-red-300 bg-white dark:bg-red-950/20'
                  )}
                >
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-xs text-muted-foreground">
                        {breach.incident_ref as string}
                      </span>
                      <Link
                        href={`/incidents/${breach.id}`}
                        className="font-semibold text-red-800 hover:underline dark:text-red-300"
                      >
                        {breach.title as string}
                      </Link>
                    </div>
                    <div className="mt-1 flex items-center gap-4 text-sm">
                      <span className="flex items-center gap-1">
                        <Clock className="h-3.5 w-3.5" />
                        {isExpired ? (
                          <span className="font-bold text-red-700 dark:text-red-400">DEADLINE PASSED</span>
                        ) : (
                          <span className={cn('font-semibold', isUrgent && 'text-red-700 dark:text-red-400')}>
                            {hours?.toFixed(1)}h remaining
                          </span>
                        )}
                      </span>
                      <span className="flex items-center gap-1">
                        <Database className="h-3.5 w-3.5" />
                        {(breach.data_subjects_affected as number) ?? '—'} data subjects
                      </span>
                    </div>
                  </div>
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={notifyDPA.isPending}
                    onClick={() =>
                      notifyDPA.mutate({ id: breach.id as string })
                    }
                  >
                    {notifyDPA.isPending ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Bell className="mr-2 h-4 w-4" />
                    )}
                    Notify DPA
                  </Button>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Incident Management</h1>
          <p className="text-muted-foreground">
            Track, investigate, and resolve security incidents. GDPR/NIS2 compliant breach management.
          </p>
        </div>
        <Button onClick={() => setSheetOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Report Incident
        </Button>
      </div>

      {/* Summary cards */}
      <div className="grid gap-4 md:grid-cols-6">
        <SummaryCard label="Open" value={stats.open ?? 0} icon={<AlertOctagon className="h-4 w-4 text-red-500" />} />
        <SummaryCard label="Investigating" value={stats.investigating ?? 0} icon={<Search className="h-4 w-4 text-yellow-500" />} />
        <SummaryCard label="Contained" value={stats.contained ?? 0} icon={<ShieldAlert className="h-4 w-4 text-blue-500" />} />
        <SummaryCard label="Resolved" value={stats.resolved ?? 0} icon={<AlertOctagon className="h-4 w-4 text-green-500" />} />
        <SummaryCard
          label="Data Breaches"
          value={stats.data_breaches ?? 0}
          icon={<Database className="h-4 w-4 text-red-500" />}
          highlight={(stats.data_breaches ?? 0) > 0}
        />
        <SummaryCard
          label="NIS2 Reportable"
          value={stats.nis2_reportable ?? 0}
          icon={<ShieldAlert className="h-4 w-4 text-purple-500" />}
          highlight={(stats.nis2_reportable ?? 0) > 0}
        />
      </div>

      {/* Search */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search incidents..."
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
              Failed to load incidents: {(error as Error)?.message ?? 'Unknown error'}
            </div>
          ) : incidents.length === 0 ? (
            <div className="p-12 text-center text-muted-foreground">
              <AlertOctagon className="mx-auto mb-3 h-10 w-10" />
              <p className="text-lg font-medium">No incidents found</p>
              <p className="text-sm">No incidents have been reported yet.</p>
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
                    <th className="px-4 py-3 text-left font-medium">Breach</th>
                    <th className="px-4 py-3 text-left font-medium">Reported Date</th>
                    <th className="px-4 py-3 text-left font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {incidents.map((inc) => {
                    const isBreach = inc.is_data_breach as boolean;
                    const notNotified = isBreach && !inc.dpa_notified_at;
                    return (
                      <tr
                        key={inc.id as string}
                        className={cn(
                          'border-b transition-colors hover:bg-muted/50',
                          notNotified && 'border-l-4 border-l-red-500'
                        )}
                      >
                        <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                          {inc.incident_ref as string}
                        </td>
                        <td className="px-4 py-3 font-medium">
                          <Link
                            href={`/incidents/${inc.id}`}
                            className="text-primary hover:underline"
                          >
                            {inc.title as string}
                          </Link>
                        </td>
                        <td className="px-4 py-3">
                          <Badge className={getRiskLevelColor(inc.severity as string)}>
                            {inc.severity as string}
                          </Badge>
                        </td>
                        <td className="px-4 py-3">
                          <Badge className={getStatusColor(inc.status as string)}>
                            {(inc.status as string)?.replace('_', ' ')}
                          </Badge>
                        </td>
                        <td className="px-4 py-3">
                          {isBreach ? (
                            <span className="inline-flex items-center gap-1 text-red-600 dark:text-red-400">
                              <ShieldAlert className="h-4 w-4" />
                              <span className="text-xs font-medium">
                                {(inc.data_subjects_affected as number) ?? '?'} subjects
                              </span>
                            </span>
                          ) : (
                            <span className="text-muted-foreground text-xs">—</span>
                          )}
                        </td>
                        <td className="px-4 py-3">{formatDate(inc.reported_at as string ?? inc.created_at as string)}</td>
                        <td className="px-4 py-3">
                          <Link href={`/incidents/${inc.id}`}>
                            <Button variant="ghost" size="sm">
                              View
                            </Button>
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

      {/* Report Incident Sheet/Dialog */}
      <ReportIncidentSheet open={sheetOpen} onOpenChange={setSheetOpen} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Summary card
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
// Report Incident Sheet
// ---------------------------------------------------------------------------

function ReportIncidentSheet({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const createIncident = useCreateIncident();

  const form = useForm<ReportIncidentValues>({
    resolver: zodResolver(reportIncidentSchema),
    defaultValues: {
      title: '',
      description: '',
      incident_type: '',
      severity: undefined,
      category: '',
      is_data_breach: false,
      data_subjects_affected: undefined,
      data_categories: [],
    },
  });

  const isDataBreach = form.watch('is_data_breach');
  const selectedCategories = form.watch('data_categories') ?? [];

  const toggleCategory = (cat: string) => {
    const current = form.getValues('data_categories') ?? [];
    if (current.includes(cat)) {
      form.setValue(
        'data_categories',
        current.filter((c) => c !== cat)
      );
    } else {
      form.setValue('data_categories', [...current, cat]);
    }
  };

  const onSubmit = async (values: ReportIncidentValues) => {
    const payload = {
      ...values,
      data_subjects_affected: values.is_data_breach ? values.data_subjects_affected : undefined,
      data_categories: values.is_data_breach ? values.data_categories : undefined,
    };
    await createIncident.mutateAsync(payload);
    form.reset();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Report Incident</DialogTitle>
          <DialogDescription>
            Report a new security incident. Data breach incidents trigger GDPR notification workflows.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          {/* Title */}
          <div className="space-y-2">
            <Label htmlFor="inc-title">Title *</Label>
            <Input id="inc-title" {...form.register('title')} placeholder="Brief incident title" />
            {form.formState.errors.title && (
              <p className="text-sm text-destructive">{form.formState.errors.title.message}</p>
            )}
          </div>

          {/* Description */}
          <div className="space-y-2">
            <Label htmlFor="inc-desc">Description *</Label>
            <Textarea id="inc-desc" {...form.register('description')} placeholder="Describe the incident..." rows={3} />
            {form.formState.errors.description && (
              <p className="text-sm text-destructive">{form.formState.errors.description.message}</p>
            )}
          </div>

          {/* Type & Severity & Category */}
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label htmlFor="inc-type">Incident Type *</Label>
              <Input id="inc-type" {...form.register('incident_type')} placeholder="e.g. Ransomware" />
              {form.formState.errors.incident_type && (
                <p className="text-sm text-destructive">{form.formState.errors.incident_type.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label>Severity *</Label>
              <Select
                value={form.watch('severity')}
                onValueChange={(v) =>
                  form.setValue('severity', v as ReportIncidentValues['severity'], { shouldValidate: true })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="critical">Critical</SelectItem>
                  <SelectItem value="high">High</SelectItem>
                  <SelectItem value="medium">Medium</SelectItem>
                  <SelectItem value="low">Low</SelectItem>
                </SelectContent>
              </Select>
              {form.formState.errors.severity && (
                <p className="text-sm text-destructive">{form.formState.errors.severity.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="inc-cat">Category *</Label>
              <Input id="inc-cat" {...form.register('category')} placeholder="e.g. Data Loss" />
              {form.formState.errors.category && (
                <p className="text-sm text-destructive">{form.formState.errors.category.message}</p>
              )}
            </div>
          </div>

          <Separator />

          {/* Data Breach toggle */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <Label className="text-base font-semibold">Data Breach</Label>
                <p className="text-sm text-muted-foreground">
                  Does this incident involve a personal data breach?
                </p>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={isDataBreach}
                onClick={() => form.setValue('is_data_breach', !isDataBreach)}
                className={cn(
                  'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
                  isDataBreach ? 'bg-red-600' : 'bg-input'
                )}
              >
                <span
                  className={cn(
                    'pointer-events-none block h-5 w-5 rounded-full bg-background shadow-lg ring-0 transition-transform',
                    isDataBreach ? 'translate-x-5' : 'translate-x-0'
                  )}
                />
              </button>
            </div>

            {isDataBreach && (
              <div className="space-y-4 rounded-lg border-2 border-red-300 bg-red-50 p-4 dark:border-red-700 dark:bg-red-950/20">
                {/* GDPR Warning */}
                <div className="flex items-start gap-2 rounded-md bg-red-100 p-3 dark:bg-red-900/30">
                  <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-red-700 dark:text-red-400" />
                  <div>
                    <p className="text-sm font-bold text-red-800 dark:text-red-300">
                      GDPR Article 33 — 72-Hour Notification Requirement
                    </p>
                    <p className="text-sm text-red-700 dark:text-red-400">
                      Personal data breaches must be reported to the supervisory authority (DPA) within
                      72 hours of becoming aware of the breach, unless the breach is unlikely to result
                      in a risk to the rights and freedoms of natural persons.
                    </p>
                  </div>
                </div>

                {/* Data subjects affected */}
                <div className="space-y-2">
                  <Label htmlFor="inc-subjects">Data Subjects Affected *</Label>
                  <Input
                    id="inc-subjects"
                    type="number"
                    min={1}
                    {...form.register('data_subjects_affected', { valueAsNumber: true })}
                    placeholder="Number of individuals affected"
                  />
                  {form.formState.errors.data_subjects_affected && (
                    <p className="text-sm text-destructive">
                      {form.formState.errors.data_subjects_affected.message}
                    </p>
                  )}
                </div>

                {/* Data categories */}
                <div className="space-y-2">
                  <Label>Data Categories Affected</Label>
                  <div className="grid grid-cols-2 gap-2">
                    {DATA_CATEGORIES.map((cat) => {
                      const checked = selectedCategories.includes(cat);
                      return (
                        <label
                          key={cat}
                          className={cn(
                            'flex items-center gap-2 rounded-md border px-3 py-2 text-sm cursor-pointer transition-colors',
                            checked
                              ? 'border-red-400 bg-red-100 dark:border-red-600 dark:bg-red-950/40'
                              : 'hover:bg-muted'
                          )}
                        >
                          <input
                            type="checkbox"
                            checked={checked}
                            onChange={() => toggleCategory(cat)}
                            className="h-4 w-4 rounded border-gray-300 text-red-600 focus:ring-red-500"
                          />
                          {cat}
                        </label>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={createIncident.isPending}
              variant={isDataBreach ? 'destructive' : 'default'}
            >
              {createIncident.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {isDataBreach ? 'Report Data Breach' : 'Report Incident'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
