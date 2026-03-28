'use client';

import { useCallback, useMemo, useState } from 'react';
import Link from 'next/link';
import { useSearchParams, useRouter } from 'next/navigation';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  AlertCircle,
  CheckCircle2,
  Clock,
  FileText,
  Loader2,
  PenLine,
  Plus,
  Search,
  X,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';

import {
  usePolicies,
  useCreatePolicy,
  useUsers,
} from '@/lib/api-hooks';
import {
  cn,
  formatDate,
  formatPercentage,
  getStatusColor,
} from '@/lib/utils';

// ---------------------------------------------------------------------------
// Zod schema
// ---------------------------------------------------------------------------

const createPolicySchema = z.object({
  title: z.string().min(3, 'Title must be at least 3 characters'),
  category_id: z.string().min(1, 'Category is required'),
  classification: z.string().min(1, 'Classification is required'),
  content_html: z.string().min(20, 'Content must be at least 20 characters'),
  summary: z.string().optional(),
  owner_user_id: z.string().min(1, 'Owner is required'),
  approver_user_id: z.string().min(1, 'Approver is required'),
  review_frequency_months: z.number().min(1).max(60),
  is_mandatory: z.boolean(),
  requires_attestation: z.boolean(),
  tags: z.string().optional(),
});

type CreatePolicyFormData = z.infer<typeof createPolicySchema>;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const POLICY_CATEGORIES = [
  { id: 'cat-infosec', name: 'Information Security' },
  { id: 'cat-privacy', name: 'Data Privacy' },
  { id: 'cat-acceptable-use', name: 'Acceptable Use' },
  { id: 'cat-access', name: 'Access Control' },
  { id: 'cat-incident', name: 'Incident Response' },
  { id: 'cat-bcdr', name: 'Business Continuity' },
  { id: 'cat-vendor', name: 'Vendor Management' },
  { id: 'cat-hr', name: 'Human Resources' },
  { id: 'cat-physical', name: 'Physical Security' },
  { id: 'cat-change', name: 'Change Management' },
];

const CLASSIFICATIONS = ['public', 'internal', 'confidential', 'restricted'] as const;

function getClassificationColor(classification: string): string {
  switch (classification) {
    case 'public': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'internal': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'confidential': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
    case 'restricted': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function isOverdue(nextReview: string | undefined | null): boolean {
  if (!nextReview) return false;
  return new Date(nextReview) < new Date();
}

// ---------------------------------------------------------------------------
// Create Policy Form
// ---------------------------------------------------------------------------

function CreatePolicyForm({ onClose }: { onClose: () => void }) {
  const createPolicy = useCreatePolicy();
  const { data: usersData } = useUsers({ page_size: 100 });
  const users = (usersData as { items?: Array<{ id: string; first_name: string; last_name: string }> })?.items ?? [];

  const {
    register,
    handleSubmit,
    control,
    watch,
    setValue,
    formState: { errors },
  } = useForm<CreatePolicyFormData>({
    resolver: zodResolver(createPolicySchema),
    defaultValues: {
      title: '',
      category_id: '',
      classification: '',
      content_html: '',
      summary: '',
      owner_user_id: '',
      approver_user_id: '',
      review_frequency_months: 12,
      is_mandatory: true,
      requires_attestation: false,
      tags: '',
    },
  });

  const isMandatory = watch('is_mandatory');
  const requiresAttestation = watch('requires_attestation');

  const onSubmit = (formData: CreatePolicyFormData) => {
    const payload = {
      ...formData,
      tags: formData.tags
        ? formData.tags.split(',').map((t) => t.trim()).filter(Boolean)
        : [],
    };
    createPolicy.mutate(payload, {
      onSuccess: () => onClose(),
    });
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-5 overflow-y-auto max-h-[calc(100vh-10rem)] pr-1">
      <div className="space-y-1.5">
        <Label htmlFor="title">Title *</Label>
        <Input id="title" placeholder="e.g. Information Security Policy" {...register('title')} />
        {errors.title && <p className="text-xs text-destructive">{errors.title.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label>Category *</Label>
        <Controller
          name="category_id"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select category" /></SelectTrigger>
              <SelectContent>
                {POLICY_CATEGORIES.map((cat) => (
                  <SelectItem key={cat.id} value={cat.id}>{cat.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {errors.category_id && <p className="text-xs text-destructive">{errors.category_id.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label>Classification *</Label>
        <Controller
          name="classification"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select classification" /></SelectTrigger>
              <SelectContent>
                {CLASSIFICATIONS.map((c) => (
                  <SelectItem key={c} value={c}>{c.charAt(0).toUpperCase() + c.slice(1)}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {errors.classification && <p className="text-xs text-destructive">{errors.classification.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="summary">Summary</Label>
        <Textarea id="summary" rows={2} placeholder="Brief summary of the policy..." {...register('summary')} />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="content_html">Content (HTML) *</Label>
        <Textarea
          id="content_html"
          rows={8}
          placeholder="<h2>Purpose</h2><p>This policy defines...</p>"
          {...register('content_html')}
        />
        {errors.content_html && <p className="text-xs text-destructive">{errors.content_html.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label>Owner *</Label>
        <Controller
          name="owner_user_id"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select owner" /></SelectTrigger>
              <SelectContent>
                {users.map((u) => (
                  <SelectItem key={u.id} value={u.id}>{u.first_name} {u.last_name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {errors.owner_user_id && <p className="text-xs text-destructive">{errors.owner_user_id.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label>Approver *</Label>
        <Controller
          name="approver_user_id"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select approver" /></SelectTrigger>
              <SelectContent>
                {users.map((u) => (
                  <SelectItem key={u.id} value={u.id}>{u.first_name} {u.last_name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {errors.approver_user_id && <p className="text-xs text-destructive">{errors.approver_user_id.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="review_frequency_months">Review Frequency (months) *</Label>
        <Input
          id="review_frequency_months"
          type="number"
          min={1}
          max={60}
          {...register('review_frequency_months', { valueAsNumber: true })}
        />
        {errors.review_frequency_months && (
          <p className="text-xs text-destructive">{errors.review_frequency_months.message}</p>
        )}
      </div>

      {/* Switches as checkboxes styled like switches */}
      <div className="flex items-center justify-between rounded-lg border p-3">
        <div>
          <Label htmlFor="is_mandatory" className="font-medium">Mandatory</Label>
          <p className="text-xs text-muted-foreground">All staff must comply with this policy</p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={isMandatory}
          onClick={() => setValue('is_mandatory', !isMandatory)}
          className={cn(
            'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            isMandatory ? 'bg-primary' : 'bg-input'
          )}
        >
          <span
            className={cn(
              'pointer-events-none block h-5 w-5 rounded-full bg-background shadow-lg ring-0 transition-transform',
              isMandatory ? 'translate-x-5' : 'translate-x-0'
            )}
          />
        </button>
      </div>

      <div className="flex items-center justify-between rounded-lg border p-3">
        <div>
          <Label htmlFor="requires_attestation" className="font-medium">Requires Attestation</Label>
          <p className="text-xs text-muted-foreground">Users must acknowledge they have read this policy</p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={requiresAttestation}
          onClick={() => setValue('requires_attestation', !requiresAttestation)}
          className={cn(
            'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            requiresAttestation ? 'bg-primary' : 'bg-input'
          )}
        >
          <span
            className={cn(
              'pointer-events-none block h-5 w-5 rounded-full bg-background shadow-lg ring-0 transition-transform',
              requiresAttestation ? 'translate-x-5' : 'translate-x-0'
            )}
          />
        </button>
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="tags">Tags (comma-separated)</Label>
        <Input id="tags" placeholder="e.g. iso27001, gdpr, mandatory" {...register('tags')} />
      </div>

      <SheetFooter>
        <SheetClose asChild>
          <Button type="button" variant="outline">Cancel</Button>
        </SheetClose>
        <Button type="submit" disabled={createPolicy.isPending}>
          {createPolicy.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Draft Policy
        </Button>
      </SheetFooter>
    </form>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function PoliciesPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const page = Number(searchParams.get('page') ?? '1');
  const pageSize = Number(searchParams.get('page_size') ?? '20');

  const [search, setSearch] = useState('');
  const [sheetOpen, setSheetOpen] = useState(false);

  const apiParams = useMemo(() => {
    const params: Record<string, unknown> = { page, page_size: pageSize };
    if (search) params.search = search;
    return params;
  }, [page, pageSize, search]);

  const { data, isLoading, error } = usePolicies(apiParams as Parameters<typeof usePolicies>[0]);

  const policiesData = data as {
    items?: Array<Record<string, unknown>>;
    total?: number;
    total_pages?: number;
  };
  const policies = policiesData?.items ?? [];
  const totalPages = policiesData?.total_pages ?? 1;

  // Compute summary stats from the current page data
  const stats = useMemo(() => {
    let published = 0;
    let draft = 0;
    let underReview = 0;
    let overdue = 0;
    let totalAttestation = 0;
    let attestationCount = 0;

    for (const p of policies) {
      const status = p.status as string;
      if (status === 'published') published++;
      else if (status === 'draft') draft++;
      else if (status === 'under_review' || status === 'pending_approval') underReview++;

      if (isOverdue(p.next_review_date as string | undefined)) overdue++;

      if (p.attestation_rate != null) {
        totalAttestation += p.attestation_rate as number;
        attestationCount++;
      }
    }

    return {
      published,
      draft,
      underReview,
      overdue,
      avgAttestation: attestationCount > 0 ? totalAttestation / attestationCount : 0,
    };
  }, [policies]);

  const updateParams = useCallback(
    (updates: Record<string, string>) => {
      const params = new URLSearchParams(searchParams.toString());
      for (const [k, v] of Object.entries(updates)) {
        if (v === '') params.delete(k);
        else params.set(k, v);
      }
      router.push(`/policies?${params.toString()}`);
    },
    [searchParams, router]
  );

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
            <FileText className="h-6 w-6 text-blue-500" />
            Policy Management
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Draft, review, publish, and track policy attestation.
          </p>
        </div>
        <Button onClick={() => setSheetOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Draft Policy
        </Button>
      </div>

      {/* Summary cards */}
      <div className="grid gap-4 md:grid-cols-5">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1">
              <CheckCircle2 className="h-3.5 w-3.5 text-green-500" /> Published
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{stats.published}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1">
              <PenLine className="h-3.5 w-3.5 text-blue-500" /> Draft
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{stats.draft}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1">
              <Clock className="h-3.5 w-3.5 text-yellow-500" /> Under Review
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{stats.underReview}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1">
              <AlertCircle className="h-3.5 w-3.5 text-red-500" /> Reviews Overdue
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className={cn('text-2xl font-bold', stats.overdue > 0 && 'text-red-500')}>
              {stats.overdue}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Avg Attestation</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{formatPercentage(stats.avgAttestation)}</p>
          </CardContent>
        </Card>
      </div>

      {/* Search */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search policies..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
          {search && (
            <button
              onClick={() => setSearch('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </div>
      </div>

      {/* Table */}
      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : error ? (
            <div className="flex items-center justify-center py-12 text-destructive">
              Failed to load policies. Please try again.
            </div>
          ) : policies.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
              <FileText className="h-10 w-10 mb-3 opacity-40" />
              <p className="font-medium">No policies found</p>
              <p className="text-sm mt-1">Draft a new policy to get started.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Ref</th>
                    <th className="px-4 py-3 text-left font-medium">Title</th>
                    <th className="px-4 py-3 text-left font-medium">Status</th>
                    <th className="px-4 py-3 text-left font-medium">Version</th>
                    <th className="px-4 py-3 text-left font-medium">Review Status</th>
                    <th className="px-4 py-3 text-left font-medium">Next Review</th>
                    <th className="px-4 py-3 text-left font-medium">Attestation</th>
                    <th className="px-4 py-3 text-left font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {policies.map((policy) => {
                    const p = policy as Record<string, unknown>;
                    const status = (p.status as string) ?? 'draft';
                    const classification = (p.classification as string) ?? 'internal';
                    const nextReview = p.next_review_date as string | undefined;
                    const overdue = isOverdue(nextReview);
                    const attestationRate = (p.attestation_rate as number) ?? 0;

                    return (
                      <tr key={p.id as string} className="border-b last:border-b-0 hover:bg-muted/30 transition-colors">
                        <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                          {(p.policy_ref as string) ?? '---'}
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <Link
                              href={`/policies/${p.id}`}
                              className="font-medium text-primary hover:underline"
                            >
                              {(p.title as string) ?? 'Untitled'}
                            </Link>
                            <Badge className={cn(getClassificationColor(classification), 'text-[10px]')}>
                              {classification}
                            </Badge>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <Badge className={cn(getStatusColor(status), 'text-xs')}>
                            {status.replace(/_/g, ' ')}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-muted-foreground">
                          {(p.version_label as string) ?? (p.version as string) ?? 'v1.0'}
                        </td>
                        <td className="px-4 py-3">
                          {overdue ? (
                            <Badge className="bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400 text-xs">
                              Overdue
                            </Badge>
                          ) : (
                            <span className="text-xs text-muted-foreground">On Track</span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-muted-foreground">
                          <span className={cn(overdue && 'text-red-500 font-medium')}>
                            {formatDate(nextReview)}
                          </span>
                        </td>
                        <td className="px-4 py-3 min-w-[140px]">
                          <div className="flex items-center gap-2">
                            <Progress value={attestationRate} className="h-2 flex-1" />
                            <span className="text-xs text-muted-foreground w-10 text-right">
                              {formatPercentage(attestationRate)}
                            </span>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <Link href={`/policies/${p.id}`}>
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
        </CardContent>
      </Card>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">
            Page {page} of {totalPages} ({policiesData?.total ?? 0} total)
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => updateParams({ page: String(page - 1) })}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => updateParams({ page: String(page + 1) })}
            >
              Next
            </Button>
          </div>
        </div>
      )}

      {/* Create Policy Sheet */}
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetContent className="sm:max-w-lg overflow-y-auto">
          <SheetHeader>
            <SheetTitle>Draft New Policy</SheetTitle>
            <SheetDescription>
              Create a new policy document. It will start in draft status.
            </SheetDescription>
          </SheetHeader>
          <div className="mt-6">
            <CreatePolicyForm onClose={() => setSheetOpen(false)} />
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
