'use client';

import { useCallback, useMemo, useState } from 'react';
import Link from 'next/link';
import { useSearchParams, useRouter } from 'next/navigation';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Loader2,
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
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
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
  useRisks,
  useRiskHeatmap,
  useCreateRisk,
  useUsers,
} from '@/lib/api-hooks';
import {
  cn,
  getRiskLevelColor,
  getRiskScoreColor,
  getStatusColor,
} from '@/lib/utils';

// ---------------------------------------------------------------------------
// Zod schema for creating a risk
// ---------------------------------------------------------------------------

const createRiskSchema = z.object({
  title: z.string().min(3, 'Title must be at least 3 characters'),
  description: z.string().min(10, 'Description must be at least 10 characters'),
  risk_category_id: z.string().min(1, 'Category is required'),
  risk_source: z.string().min(1, 'Risk source is required'),
  owner_user_id: z.string().min(1, 'Owner is required'),
  inherent_likelihood: z.number().min(1).max(5),
  inherent_impact: z.number().min(1).max(5),
  residual_likelihood: z.number().min(1).max(5),
  residual_impact: z.number().min(1).max(5),
  financial_impact_eur: z.number().min(0).optional(),
  risk_velocity: z.string().optional(),
  review_frequency: z.string().optional(),
  tags: z.string().optional(),
});

type CreateRiskFormData = z.infer<typeof createRiskSchema>;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const RISK_LEVELS = ['all', 'critical', 'high', 'medium', 'low', 'very_low'] as const;
const RISK_STATUSES = ['all', 'open', 'in_progress', 'mitigated', 'accepted', 'closed'] as const;
const RISK_SOURCES = ['internal', 'external', 'third_party', 'regulatory', 'operational', 'strategic', 'financial', 'technology'] as const;
const RISK_VELOCITIES = ['immediate', 'fast', 'moderate', 'slow'] as const;
const REVIEW_FREQUENCIES = ['monthly', 'quarterly', 'semi_annual', 'annual'] as const;

const RISK_CATEGORIES = [
  { id: 'cat-cyber', name: 'Cyber Security' },
  { id: 'cat-data', name: 'Data Protection' },
  { id: 'cat-ops', name: 'Operational' },
  { id: 'cat-compliance', name: 'Compliance' },
  { id: 'cat-vendor', name: 'Third Party / Vendor' },
  { id: 'cat-strategic', name: 'Strategic' },
  { id: 'cat-financial', name: 'Financial' },
  { id: 'cat-legal', name: 'Legal' },
];

const LIKELIHOOD_LABELS: Record<number, string> = {
  1: 'Rare',
  2: 'Unlikely',
  3: 'Possible',
  4: 'Likely',
  5: 'Almost Certain',
};

const IMPACT_LABELS: Record<number, string> = {
  1: 'Negligible',
  2: 'Minor',
  3: 'Moderate',
  4: 'Major',
  5: 'Catastrophic',
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function scoreToLevel(score: number): string {
  if (score >= 20) return 'critical';
  if (score >= 12) return 'high';
  if (score >= 6) return 'medium';
  if (score >= 2) return 'low';
  return 'very_low';
}

function ScoreBadge({ score }: { score: number }) {
  const level = scoreToLevel(score);
  return (
    <Badge className={cn(getRiskLevelColor(level), 'font-bold')}>
      {score} - {level.replace('_', ' ').toUpperCase()}
    </Badge>
  );
}

// ---------------------------------------------------------------------------
// Risk Heatmap component
// ---------------------------------------------------------------------------

function RiskHeatmap() {
  const { data, isLoading, error } = useRiskHeatmap();
  const [mode, setMode] = useState<'inherent' | 'residual'>('residual');

  if (isLoading) {
    return (
      <div className="grid gap-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-[400px] w-full" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center py-12 text-destructive">
        Failed to load heatmap data.
      </div>
    );
  }

  const heatmapData = data as {
    cells?: Array<{
      likelihood: number;
      impact: number;
      inherent_count: number;
      residual_count: number;
      risks: Array<{ id: string; title: string; risk_ref: string }>;
    }>;
  };
  const cells = heatmapData?.cells ?? [];

  // Build a 5x5 grid lookup
  const grid: Record<string, typeof cells[0]> = {};
  for (const cell of cells) {
    grid[`${cell.likelihood}-${cell.impact}`] = cell;
  }

  function getCellColor(count: number): string {
    if (count === 0) return 'bg-muted';
    if (count >= 5) return 'bg-red-500 text-white';
    if (count >= 3) return 'bg-orange-400 text-white';
    if (count >= 1) return 'bg-yellow-300 text-yellow-900';
    return 'bg-muted';
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <span className="text-sm font-medium text-muted-foreground">View:</span>
        <Button
          variant={mode === 'inherent' ? 'default' : 'outline'}
          size="sm"
          onClick={() => setMode('inherent')}
        >
          Inherent
        </Button>
        <Button
          variant={mode === 'residual' ? 'default' : 'outline'}
          size="sm"
          onClick={() => setMode('residual')}
        >
          Residual
        </Button>
      </div>

      <div className="overflow-x-auto">
        <div className="inline-block">
          <div className="flex items-end gap-1 mb-1">
            <div className="w-24" />
            {[1, 2, 3, 4, 5].map((impact) => (
              <div
                key={impact}
                className="w-24 text-center text-xs font-medium text-muted-foreground"
              >
                {IMPACT_LABELS[impact]}
              </div>
            ))}
          </div>
          <div className="flex items-center gap-1 mb-1">
            <div className="w-24" />
            {[1, 2, 3, 4, 5].map((impact) => (
              <div
                key={impact}
                className="w-24 text-center text-xs text-muted-foreground"
              >
                Impact {impact}
              </div>
            ))}
          </div>
          {[5, 4, 3, 2, 1].map((likelihood) => (
            <div key={likelihood} className="flex items-center gap-1 mb-1">
              <div className="w-24 text-right pr-2 text-xs font-medium text-muted-foreground">
                <div>{LIKELIHOOD_LABELS[likelihood]}</div>
                <div className="text-muted-foreground/60">L{likelihood}</div>
              </div>
              {[1, 2, 3, 4, 5].map((impact) => {
                const cell = grid[`${likelihood}-${impact}`];
                const count =
                  mode === 'inherent'
                    ? (cell?.inherent_count ?? 0)
                    : (cell?.residual_count ?? 0);
                return (
                  <div
                    key={impact}
                    className={cn(
                      'w-24 h-20 rounded-md flex flex-col items-center justify-center text-sm font-semibold transition-colors cursor-default',
                      getCellColor(count)
                    )}
                    title={`L${likelihood} x I${impact} = ${likelihood * impact} | ${count} risk(s)`}
                  >
                    <span className="text-lg">{count}</span>
                    <span className="text-[10px] opacity-70">
                      Score: {likelihood * impact}
                    </span>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
      <p className="text-xs text-muted-foreground mt-2">
        Heatmap shows the number of risks at each likelihood/impact intersection ({mode} scores).
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Slider component (simple custom since the page needs it inline)
// ---------------------------------------------------------------------------

function ScoreSlider({
  value,
  onChange,
  labels,
  label,
}: {
  value: number;
  onChange: (v: number) => void;
  labels: Record<number, string>;
  label: string;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label>{label}</Label>
        <span className="text-sm font-semibold">
          {value} - {labels[value]}
        </span>
      </div>
      <input
        type="range"
        min={1}
        max={5}
        step={1}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-primary h-2 rounded-lg cursor-pointer"
      />
      <div className="flex justify-between text-[10px] text-muted-foreground px-0.5">
        {[1, 2, 3, 4, 5].map((n) => (
          <span key={n}>{n}</span>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Create Risk Sheet Form
// ---------------------------------------------------------------------------

function CreateRiskForm({ onClose }: { onClose: () => void }) {
  const createRisk = useCreateRisk();
  const { data: usersData } = useUsers({ page_size: 100 });
  const users = (usersData as { items?: Array<{ id: string; first_name: string; last_name: string }> })?.items ?? [];

  const {
    register,
    handleSubmit,
    control,
    watch,
    formState: { errors },
  } = useForm<CreateRiskFormData>({
    resolver: zodResolver(createRiskSchema),
    defaultValues: {
      title: '',
      description: '',
      risk_category_id: '',
      risk_source: '',
      owner_user_id: '',
      inherent_likelihood: 3,
      inherent_impact: 3,
      residual_likelihood: 2,
      residual_impact: 2,
      financial_impact_eur: undefined,
      risk_velocity: '',
      review_frequency: '',
      tags: '',
    },
  });

  const inherentL = watch('inherent_likelihood');
  const inherentI = watch('inherent_impact');
  const residualL = watch('residual_likelihood');
  const residualI = watch('residual_impact');

  const inherentScore = inherentL * inherentI;
  const residualScore = residualL * residualI;

  const onSubmit = (formData: CreateRiskFormData) => {
    const payload = {
      ...formData,
      tags: formData.tags
        ? formData.tags.split(',').map((t) => t.trim()).filter(Boolean)
        : [],
    };
    createRisk.mutate(payload, {
      onSuccess: () => onClose(),
    });
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-5 overflow-y-auto max-h-[calc(100vh-10rem)] pr-1">
      {/* Title */}
      <div className="space-y-1.5">
        <Label htmlFor="title">Title *</Label>
        <Input id="title" placeholder="e.g. Ransomware attack on production systems" {...register('title')} />
        {errors.title && <p className="text-xs text-destructive">{errors.title.message}</p>}
      </div>

      {/* Description */}
      <div className="space-y-1.5">
        <Label htmlFor="description">Description *</Label>
        <Textarea id="description" rows={3} placeholder="Describe the risk scenario..." {...register('description')} />
        {errors.description && <p className="text-xs text-destructive">{errors.description.message}</p>}
      </div>

      {/* Category */}
      <div className="space-y-1.5">
        <Label>Category *</Label>
        <Controller
          name="risk_category_id"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger>
                <SelectValue placeholder="Select category" />
              </SelectTrigger>
              <SelectContent>
                {RISK_CATEGORIES.map((cat) => (
                  <SelectItem key={cat.id} value={cat.id}>{cat.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {errors.risk_category_id && <p className="text-xs text-destructive">{errors.risk_category_id.message}</p>}
      </div>

      {/* Risk Source */}
      <div className="space-y-1.5">
        <Label>Risk Source *</Label>
        <Controller
          name="risk_source"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger>
                <SelectValue placeholder="Select source" />
              </SelectTrigger>
              <SelectContent>
                {RISK_SOURCES.map((s) => (
                  <SelectItem key={s} value={s}>{s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {errors.risk_source && <p className="text-xs text-destructive">{errors.risk_source.message}</p>}
      </div>

      {/* Owner */}
      <div className="space-y-1.5">
        <Label>Owner *</Label>
        <Controller
          name="owner_user_id"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger>
                <SelectValue placeholder="Select owner" />
              </SelectTrigger>
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

      {/* Inherent Scores */}
      <div className="rounded-lg border p-4 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="text-sm font-semibold">Inherent Risk</h4>
          <ScoreBadge score={inherentScore} />
        </div>
        <Controller
          name="inherent_likelihood"
          control={control}
          render={({ field }) => (
            <ScoreSlider
              value={field.value}
              onChange={field.onChange}
              labels={LIKELIHOOD_LABELS}
              label="Likelihood"
            />
          )}
        />
        <Controller
          name="inherent_impact"
          control={control}
          render={({ field }) => (
            <ScoreSlider
              value={field.value}
              onChange={field.onChange}
              labels={IMPACT_LABELS}
              label="Impact"
            />
          )}
        />
      </div>

      {/* Residual Scores */}
      <div className="rounded-lg border p-4 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="text-sm font-semibold">Residual Risk</h4>
          <ScoreBadge score={residualScore} />
        </div>
        <Controller
          name="residual_likelihood"
          control={control}
          render={({ field }) => (
            <ScoreSlider
              value={field.value}
              onChange={field.onChange}
              labels={LIKELIHOOD_LABELS}
              label="Likelihood"
            />
          )}
        />
        <Controller
          name="residual_impact"
          control={control}
          render={({ field }) => (
            <ScoreSlider
              value={field.value}
              onChange={field.onChange}
              labels={IMPACT_LABELS}
              label="Impact"
            />
          )}
        />
      </div>

      {/* Financial Impact */}
      <div className="space-y-1.5">
        <Label htmlFor="financial_impact_eur">Financial Impact (EUR)</Label>
        <Input
          id="financial_impact_eur"
          type="number"
          min={0}
          placeholder="0"
          {...register('financial_impact_eur', { valueAsNumber: true })}
        />
      </div>

      {/* Risk Velocity */}
      <div className="space-y-1.5">
        <Label>Risk Velocity</Label>
        <Controller
          name="risk_velocity"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger>
                <SelectValue placeholder="Select velocity" />
              </SelectTrigger>
              <SelectContent>
                {RISK_VELOCITIES.map((v) => (
                  <SelectItem key={v} value={v}>{v.charAt(0).toUpperCase() + v.slice(1)}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </div>

      {/* Review Frequency */}
      <div className="space-y-1.5">
        <Label>Review Frequency</Label>
        <Controller
          name="review_frequency"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger>
                <SelectValue placeholder="Select frequency" />
              </SelectTrigger>
              <SelectContent>
                {REVIEW_FREQUENCIES.map((f) => (
                  <SelectItem key={f} value={f}>{f.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </div>

      {/* Tags */}
      <div className="space-y-1.5">
        <Label htmlFor="tags">Tags (comma-separated)</Label>
        <Input id="tags" placeholder="e.g. gdpr, nis2, critical-infra" {...register('tags')} />
      </div>

      <SheetFooter>
        <SheetClose asChild>
          <Button type="button" variant="outline">Cancel</Button>
        </SheetClose>
        <Button type="submit" disabled={createRisk.isPending}>
          {createRisk.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Register Risk
        </Button>
      </SheetFooter>
    </form>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function RiskRegisterPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // URL state
  const page = Number(searchParams.get('page') ?? '1');
  const sortBy = searchParams.get('sort_by') ?? 'residual_risk_score';
  const sortDir = (searchParams.get('sort_dir') ?? 'desc') as 'asc' | 'desc';
  const statusFilter = searchParams.get('status') ?? 'all';
  const riskLevelFilter = searchParams.get('risk_level') ?? 'all';

  const [search, setSearch] = useState('');
  const [sheetOpen, setSheetOpen] = useState(false);

  // Build API params
  const apiParams = useMemo(() => {
    const params: Record<string, unknown> = {
      page,
      page_size: 20,
      sort_by: sortBy,
      sort_order: sortDir,
    };
    if (statusFilter !== 'all') params.status = statusFilter;
    if (riskLevelFilter !== 'all') params.risk_level = riskLevelFilter;
    if (search) params.search = search;
    return params;
  }, [page, sortBy, sortDir, statusFilter, riskLevelFilter, search]);

  const { data, isLoading, error } = useRisks(apiParams as Parameters<typeof useRisks>[0]);

  const risksData = data as {
    items?: Array<Record<string, unknown>>;
    total?: number;
    total_pages?: number;
    page?: number;
  };
  const risks = risksData?.items ?? [];
  const totalPages = risksData?.total_pages ?? 1;

  // URL updater
  const updateParams = useCallback(
    (updates: Record<string, string>) => {
      const params = new URLSearchParams(searchParams.toString());
      for (const [k, v] of Object.entries(updates)) {
        if (v === 'all' || v === '') {
          params.delete(k);
        } else {
          params.set(k, v);
        }
      }
      router.push(`/risks?${params.toString()}`);
    },
    [searchParams, router]
  );

  const toggleSort = (col: string) => {
    if (sortBy === col) {
      updateParams({ sort_dir: sortDir === 'asc' ? 'desc' : 'asc' });
    } else {
      updateParams({ sort_by: col, sort_dir: 'desc' });
    }
  };

  function SortIcon({ col }: { col: string }) {
    if (sortBy !== col) return null;
    return sortDir === 'asc' ? (
      <ChevronUp className="inline h-3 w-3 ml-1" />
    ) : (
      <ChevronDown className="inline h-3 w-3 ml-1" />
    );
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
            <AlertTriangle className="h-6 w-6 text-orange-500" />
            Risk Register
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Identify, assess, and monitor organisational risks.
          </p>
        </div>
        <Button onClick={() => setSheetOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Register Risk
        </Button>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="list">
        <TabsList>
          <TabsTrigger value="list">List View</TabsTrigger>
          <TabsTrigger value="heatmap">Heatmap</TabsTrigger>
        </TabsList>

        {/* ---- List View ---- */}
        <TabsContent value="list" className="space-y-4">
          {/* Filters */}
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative flex-1 max-w-sm">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search risks..."
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
            <Select
              value={riskLevelFilter}
              onValueChange={(v) => updateParams({ risk_level: v, page: '1' })}
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="Risk Level" />
              </SelectTrigger>
              <SelectContent>
                {RISK_LEVELS.map((l) => (
                  <SelectItem key={l} value={l}>
                    {l === 'all' ? 'All Levels' : l.replace('_', ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={statusFilter}
              onValueChange={(v) => updateParams({ status: v, page: '1' })}
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                {RISK_STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s === 'all' ? 'All Statuses' : s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
                  Failed to load risks. Please try again.
                </div>
              ) : risks.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
                  <AlertTriangle className="h-10 w-10 mb-3 opacity-40" />
                  <p className="font-medium">No risks found</p>
                  <p className="text-sm mt-1">Adjust your filters or register a new risk.</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">Ref</th>
                        <th
                          className="px-4 py-3 text-left font-medium cursor-pointer select-none"
                          onClick={() => toggleSort('title')}
                        >
                          Title <SortIcon col="title" />
                        </th>
                        <th className="px-4 py-3 text-left font-medium">Category</th>
                        <th
                          className="px-4 py-3 text-left font-medium cursor-pointer select-none"
                          onClick={() => toggleSort('residual_risk_score')}
                        >
                          Residual Score <SortIcon col="residual_risk_score" />
                        </th>
                        <th className="px-4 py-3 text-left font-medium">Level</th>
                        <th className="px-4 py-3 text-left font-medium">Owner</th>
                        <th className="px-4 py-3 text-left font-medium">Status</th>
                        <th className="px-4 py-3 text-left font-medium">Treatments</th>
                      </tr>
                    </thead>
                    <tbody>
                      {risks.map((risk) => {
                        const r = risk as Record<string, unknown>;
                        const score = (r.residual_risk_score as number) ?? 0;
                        const level = (r.residual_risk_level as string) ?? scoreToLevel(score);
                        const status = (r.status as string) ?? 'open';
                        const treatments = (r.risk_treatments as unknown[]) ?? [];
                        const owner = r.owner as Record<string, string> | undefined;

                        return (
                          <tr key={r.id as string} className="border-b last:border-b-0 hover:bg-muted/30 transition-colors">
                            <td className="px-4 py-3 text-muted-foreground font-mono text-xs">
                              {(r.risk_ref as string) ?? '---'}
                            </td>
                            <td className="px-4 py-3">
                              <Link
                                href={`/risks/${r.id}`}
                                className="font-medium text-primary hover:underline"
                              >
                                {(r.title as string) ?? 'Untitled'}
                              </Link>
                            </td>
                            <td className="px-4 py-3 text-muted-foreground">
                              {(r.category_name as string) ?? (r.risk_category_id as string) ?? '---'}
                            </td>
                            <td className="px-4 py-3">
                              <span className={cn('font-bold', getRiskScoreColor(score))}>
                                {score}
                              </span>
                            </td>
                            <td className="px-4 py-3">
                              <Badge className={cn(getRiskLevelColor(level), 'text-xs')}>
                                {level.replace('_', ' ')}
                              </Badge>
                            </td>
                            <td className="px-4 py-3 text-muted-foreground">
                              {owner
                                ? `${owner.first_name ?? ''} ${owner.last_name ?? ''}`.trim()
                                : (r.owner_name as string) ?? '---'}
                            </td>
                            <td className="px-4 py-3">
                              <Badge className={cn(getStatusColor(status), 'text-xs')}>
                                {status.replace(/_/g, ' ')}
                              </Badge>
                            </td>
                            <td className="px-4 py-3 text-center text-muted-foreground">
                              {treatments.length}
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
                Page {page} of {totalPages} ({risksData?.total ?? 0} total)
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
        </TabsContent>

        {/* ---- Heatmap ---- */}
        <TabsContent value="heatmap">
          <Card>
            <CardHeader>
              <CardTitle>Risk Heatmap</CardTitle>
            </CardHeader>
            <CardContent>
              <RiskHeatmap />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Create Risk Sheet */}
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetContent className="sm:max-w-lg overflow-y-auto">
          <SheetHeader>
            <SheetTitle>Register New Risk</SheetTitle>
            <SheetDescription>
              Enter risk details. Scores are auto-calculated from likelihood and impact.
            </SheetDescription>
          </SheetHeader>
          <div className="mt-6">
            <CreateRiskForm onClose={() => setSheetOpen(false)} />
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
