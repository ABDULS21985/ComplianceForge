'use client';

import { useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  AlertTriangle,
  ArrowLeft,
  Calendar,
  DollarSign,
  Edit2,
  Loader2,
  Shield,
  User,
  Zap,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Progress } from '@/components/ui/progress';
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
  useRisk,
  useUpdateRisk,
  useUsers,
  useControlImplementation,
} from '@/lib/api-hooks';
import {
  cn,
  formatCurrency,
  formatDate,
  getRiskLevelColor,
  getRiskScoreColor,
  getStatusColor,
} from '@/lib/utils';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface RiskData {
  id: string;
  risk_ref: string;
  title: string;
  description: string;
  status: string;
  risk_category_id: string;
  category_name?: string;
  risk_source: string;
  owner_user_id: string;
  owner?: { id: string; first_name: string; last_name: string };
  owner_name?: string;
  inherent_likelihood: number;
  inherent_impact: number;
  inherent_risk_score: number;
  inherent_risk_level: string;
  residual_likelihood: number;
  residual_impact: number;
  residual_risk_score: number;
  residual_risk_level: string;
  financial_impact_eur?: number;
  risk_velocity?: string;
  review_frequency?: string;
  next_review_date?: string;
  tags?: string[];
  risk_treatments?: RiskTreatment[];
  linked_control_ids?: string[];
  history?: HistoryEntry[];
  created_at?: string;
  updated_at?: string;
}

interface RiskTreatment {
  id: string;
  title: string;
  description?: string;
  treatment_type: string;
  status: string;
  progress?: number;
  due_date?: string;
  assigned_to?: string;
  assigned_user_name?: string;
}

interface HistoryEntry {
  id: string;
  action: string;
  field?: string;
  old_value?: string;
  new_value?: string;
  user_name?: string;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Zod schema for editing
// ---------------------------------------------------------------------------

const updateRiskSchema = z.object({
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

type UpdateRiskFormData = z.infer<typeof updateRiskSchema>;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

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
  1: 'Rare', 2: 'Unlikely', 3: 'Possible', 4: 'Likely', 5: 'Almost Certain',
};

const IMPACT_LABELS: Record<number, string> = {
  1: 'Negligible', 2: 'Minor', 3: 'Moderate', 4: 'Major', 5: 'Catastrophic',
};

function scoreToLevel(score: number): string {
  if (score >= 20) return 'critical';
  if (score >= 12) return 'high';
  if (score >= 6) return 'medium';
  if (score >= 2) return 'low';
  return 'very_low';
}

// ---------------------------------------------------------------------------
// Slider component
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
        <span className="text-sm font-semibold">{value} - {labels[value]}</span>
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
// Impact Matrix Visualization
// ---------------------------------------------------------------------------

function ImpactMatrix({
  inherentL,
  inherentI,
  residualL,
  residualI,
}: {
  inherentL: number;
  inherentI: number;
  residualL: number;
  residualI: number;
}) {
  function getCellBg(l: number, i: number): string {
    const score = l * i;
    if (score >= 20) return 'bg-red-200 dark:bg-red-900/40';
    if (score >= 12) return 'bg-orange-200 dark:bg-orange-900/40';
    if (score >= 6) return 'bg-yellow-200 dark:bg-yellow-900/40';
    return 'bg-green-200 dark:bg-green-900/40';
  }

  return (
    <div className="overflow-x-auto">
      <table className="text-xs">
        <thead>
          <tr>
            <th className="w-16" />
            {[1, 2, 3, 4, 5].map((i) => (
              <th key={i} className="px-2 py-1 text-center font-medium text-muted-foreground">
                I{i}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {[5, 4, 3, 2, 1].map((l) => (
            <tr key={l}>
              <td className="px-2 py-1 text-right font-medium text-muted-foreground">L{l}</td>
              {[1, 2, 3, 4, 5].map((i) => {
                const isInherent = l === inherentL && i === inherentI;
                const isResidual = l === residualL && i === residualI;
                return (
                  <td key={i} className="px-1 py-1">
                    <div
                      className={cn(
                        'w-10 h-10 rounded flex items-center justify-center text-[10px] font-semibold relative',
                        getCellBg(l, i),
                        (isInherent || isResidual) && 'ring-2 ring-offset-1'
                      )}
                    >
                      {l * i}
                      {isInherent && (
                        <span className="absolute -top-1 -right-1 w-3 h-3 rounded-full bg-red-500 border border-white" title="Inherent" />
                      )}
                      {isResidual && (
                        <span className="absolute -bottom-1 -right-1 w-3 h-3 rounded-full bg-blue-500 border border-white" title="Residual" />
                      )}
                    </div>
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
      <div className="flex items-center gap-4 mt-2 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <span className="w-3 h-3 rounded-full bg-red-500" /> Inherent
        </span>
        <span className="flex items-center gap-1">
          <span className="w-3 h-3 rounded-full bg-blue-500" /> Residual
        </span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Linked Control Row
// ---------------------------------------------------------------------------

function LinkedControl({ controlId }: { controlId: string }) {
  const { data, isLoading } = useControlImplementation(controlId);
  const ctrl = data as Record<string, unknown> | undefined;

  if (isLoading) return <Skeleton className="h-10 w-full" />;
  if (!ctrl) return null;

  return (
    <tr className="border-b last:border-b-0">
      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
        {(ctrl.control_ref as string) ?? controlId.slice(0, 8)}
      </td>
      <td className="px-4 py-3">
        <Link href={`/controls/${controlId}`} className="text-primary hover:underline">
          {(ctrl.title as string) ?? 'Untitled Control'}
        </Link>
      </td>
      <td className="px-4 py-3">
        <Badge className={cn(getStatusColor((ctrl.status as string) ?? 'planned'), 'text-xs')}>
          {((ctrl.status as string) ?? 'planned').replace(/_/g, ' ')}
        </Badge>
      </td>
      <td className="px-4 py-3 text-muted-foreground">
        {((ctrl.maturity_level as number) ?? 0)}/5
      </td>
    </tr>
  );
}

// ---------------------------------------------------------------------------
// Edit Risk Sheet
// ---------------------------------------------------------------------------

function EditRiskForm({ risk, onClose }: { risk: RiskData; onClose: () => void }) {
  const updateRisk = useUpdateRisk();
  const { data: usersData } = useUsers({ page_size: 100 });
  const users = (usersData as { items?: Array<{ id: string; first_name: string; last_name: string }> })?.items ?? [];

  const {
    register,
    handleSubmit,
    control,
    watch,
    formState: { errors },
  } = useForm<UpdateRiskFormData>({
    resolver: zodResolver(updateRiskSchema),
    defaultValues: {
      title: risk.title,
      description: risk.description,
      risk_category_id: risk.risk_category_id,
      risk_source: risk.risk_source,
      owner_user_id: risk.owner_user_id,
      inherent_likelihood: risk.inherent_likelihood,
      inherent_impact: risk.inherent_impact,
      residual_likelihood: risk.residual_likelihood,
      residual_impact: risk.residual_impact,
      financial_impact_eur: risk.financial_impact_eur ?? undefined,
      risk_velocity: risk.risk_velocity ?? '',
      review_frequency: risk.review_frequency ?? '',
      tags: risk.tags?.join(', ') ?? '',
    },
  });

  const inherentL = watch('inherent_likelihood');
  const inherentI = watch('inherent_impact');
  const residualL = watch('residual_likelihood');
  const residualI = watch('residual_impact');

  const inherentScore = inherentL * inherentI;
  const residualScore = residualL * residualI;

  function ScoreBadge({ score }: { score: number }) {
    const level = scoreToLevel(score);
    return (
      <Badge className={cn(getRiskLevelColor(level), 'font-bold')}>
        {score} - {level.replace('_', ' ').toUpperCase()}
      </Badge>
    );
  }

  const onSubmit = (formData: UpdateRiskFormData) => {
    const payload = {
      ...formData,
      tags: formData.tags ? formData.tags.split(',').map((t) => t.trim()).filter(Boolean) : [],
    };
    updateRisk.mutate(
      { id: risk.id, data: payload },
      { onSuccess: () => onClose() }
    );
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-5 overflow-y-auto max-h-[calc(100vh-10rem)] pr-1">
      <div className="space-y-1.5">
        <Label htmlFor="title">Title *</Label>
        <Input id="title" {...register('title')} />
        {errors.title && <p className="text-xs text-destructive">{errors.title.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="description">Description *</Label>
        <Textarea id="description" rows={3} {...register('description')} />
        {errors.description && <p className="text-xs text-destructive">{errors.description.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label>Category *</Label>
        <Controller
          name="risk_category_id"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select category" /></SelectTrigger>
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

      <div className="space-y-1.5">
        <Label>Risk Source *</Label>
        <Controller
          name="risk_source"
          control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select source" /></SelectTrigger>
              <SelectContent>
                {RISK_SOURCES.map((s) => (
                  <SelectItem key={s} value={s}>{s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
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
      </div>

      <div className="rounded-lg border p-4 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="text-sm font-semibold">Inherent Risk</h4>
          <ScoreBadge score={inherentScore} />
        </div>
        <Controller name="inherent_likelihood" control={control}
          render={({ field }) => <ScoreSlider value={field.value} onChange={field.onChange} labels={LIKELIHOOD_LABELS} label="Likelihood" />} />
        <Controller name="inherent_impact" control={control}
          render={({ field }) => <ScoreSlider value={field.value} onChange={field.onChange} labels={IMPACT_LABELS} label="Impact" />} />
      </div>

      <div className="rounded-lg border p-4 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="text-sm font-semibold">Residual Risk</h4>
          <ScoreBadge score={residualScore} />
        </div>
        <Controller name="residual_likelihood" control={control}
          render={({ field }) => <ScoreSlider value={field.value} onChange={field.onChange} labels={LIKELIHOOD_LABELS} label="Likelihood" />} />
        <Controller name="residual_impact" control={control}
          render={({ field }) => <ScoreSlider value={field.value} onChange={field.onChange} labels={IMPACT_LABELS} label="Impact" />} />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="financial_impact_eur">Financial Impact (EUR)</Label>
        <Input id="financial_impact_eur" type="number" min={0} {...register('financial_impact_eur', { valueAsNumber: true })} />
      </div>

      <div className="space-y-1.5">
        <Label>Risk Velocity</Label>
        <Controller name="risk_velocity" control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select velocity" /></SelectTrigger>
              <SelectContent>
                {RISK_VELOCITIES.map((v) => (
                  <SelectItem key={v} value={v}>{v.charAt(0).toUpperCase() + v.slice(1)}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )} />
      </div>

      <div className="space-y-1.5">
        <Label>Review Frequency</Label>
        <Controller name="review_frequency" control={control}
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger><SelectValue placeholder="Select frequency" /></SelectTrigger>
              <SelectContent>
                {REVIEW_FREQUENCIES.map((f) => (
                  <SelectItem key={f} value={f}>{f.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )} />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="tags">Tags (comma-separated)</Label>
        <Input id="tags" {...register('tags')} />
      </div>

      <SheetFooter>
        <SheetClose asChild>
          <Button type="button" variant="outline">Cancel</Button>
        </SheetClose>
        <Button type="submit" disabled={updateRisk.isPending}>
          {updateRisk.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Save Changes
        </Button>
      </SheetFooter>
    </form>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function RiskDetailPage() {
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;

  const { data, isLoading, error } = useRisk(id);
  const [editOpen, setEditOpen] = useState(false);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <div className="grid gap-4 md:grid-cols-5">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-24" />
          ))}
        </div>
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-muted-foreground">
        <AlertTriangle className="h-12 w-12 mb-4 text-destructive" />
        <p className="text-lg font-medium">Risk not found</p>
        <p className="text-sm mt-1">The requested risk could not be loaded.</p>
        <Button variant="outline" className="mt-4" onClick={() => router.push('/risks')}>
          <ArrowLeft className="mr-2 h-4 w-4" /> Back to Register
        </Button>
      </div>
    );
  }

  const risk = data as RiskData;
  const ownerName = risk.owner
    ? `${risk.owner.first_name} ${risk.owner.last_name}`
    : risk.owner_name ?? '---';
  const treatments = risk.risk_treatments ?? [];
  const linkedControlIds = risk.linked_control_ids ?? [];
  const history = risk.history ?? [];

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link
        href="/risks"
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="mr-1 h-4 w-4" /> Back to Risk Register
      </Link>

      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <span className="font-mono text-sm text-muted-foreground">{risk.risk_ref}</span>
            <Badge className={cn(getStatusColor(risk.status), 'text-xs')}>
              {risk.status.replace(/_/g, ' ')}
            </Badge>
            <Badge className={cn(getRiskLevelColor(risk.residual_risk_level), 'text-xs')}>
              {risk.residual_risk_level?.replace('_', ' ')}
            </Badge>
          </div>
          <h1 className="text-2xl font-bold tracking-tight">{risk.title}</h1>
        </div>
        <Button onClick={() => setEditOpen(true)}>
          <Edit2 className="mr-2 h-4 w-4" /> Edit Risk
        </Button>
      </div>

      {/* Summary cards */}
      <div className="grid gap-4 md:grid-cols-5">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Inherent Score</CardTitle>
          </CardHeader>
          <CardContent>
            <p className={cn('text-2xl font-bold', getRiskScoreColor(risk.inherent_risk_score))}>
              {risk.inherent_risk_score}
            </p>
            <p className="text-xs text-muted-foreground">
              L{risk.inherent_likelihood} x I{risk.inherent_impact}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Residual Score</CardTitle>
          </CardHeader>
          <CardContent>
            <p className={cn('text-2xl font-bold', getRiskScoreColor(risk.residual_risk_score))}>
              {risk.residual_risk_score}
            </p>
            <p className="text-xs text-muted-foreground">
              L{risk.residual_likelihood} x I{risk.residual_impact}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1">
              <DollarSign className="h-3.5 w-3.5" /> Financial Impact
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">
              {formatCurrency(risk.financial_impact_eur)}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1">
              <Calendar className="h-3.5 w-3.5" /> Next Review
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-lg font-semibold">{formatDate(risk.next_review_date)}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-1">
              <User className="h-3.5 w-3.5" /> Owner
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-lg font-semibold">{ownerName}</p>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="treatments">Treatments ({treatments.length})</TabsTrigger>
          <TabsTrigger value="controls">Controls ({linkedControlIds.length})</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
        </TabsList>

        {/* Overview */}
        <TabsContent value="overview" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Description</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm leading-relaxed whitespace-pre-wrap">{risk.description}</p>
            </CardContent>
          </Card>

          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Details</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Source</span>
                  <span className="font-medium">{risk.risk_source?.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()) ?? '---'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Velocity</span>
                  <span className="font-medium flex items-center gap-1">
                    <Zap className="h-3 w-3" />
                    {risk.risk_velocity?.charAt(0).toUpperCase()}{risk.risk_velocity?.slice(1) ?? '---'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Category</span>
                  <span className="font-medium">{risk.category_name ?? risk.risk_category_id}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Review Frequency</span>
                  <span className="font-medium">{risk.review_frequency?.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()) ?? '---'}</span>
                </div>
                {risk.tags && risk.tags.length > 0 && (
                  <div className="flex justify-between items-start">
                    <span className="text-muted-foreground">Tags</span>
                    <div className="flex flex-wrap gap-1 justify-end">
                      {risk.tags.map((tag) => (
                        <Badge key={tag} variant="outline" className="text-xs">{tag}</Badge>
                      ))}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Impact Matrix</CardTitle>
              </CardHeader>
              <CardContent>
                <ImpactMatrix
                  inherentL={risk.inherent_likelihood}
                  inherentI={risk.inherent_impact}
                  residualL={risk.residual_likelihood}
                  residualI={risk.residual_impact}
                />
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* Treatments */}
        <TabsContent value="treatments">
          <Card>
            <CardContent className="p-0">
              {treatments.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <Shield className="h-8 w-8 mb-2 opacity-40" />
                  <p className="font-medium">No treatments defined</p>
                  <p className="text-sm mt-1">Add treatment plans to mitigate this risk.</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">Title</th>
                        <th className="px-4 py-3 text-left font-medium">Type</th>
                        <th className="px-4 py-3 text-left font-medium">Status</th>
                        <th className="px-4 py-3 text-left font-medium">Progress</th>
                        <th className="px-4 py-3 text-left font-medium">Due Date</th>
                        <th className="px-4 py-3 text-left font-medium">Assigned To</th>
                      </tr>
                    </thead>
                    <tbody>
                      {treatments.map((t) => (
                        <tr key={t.id} className="border-b last:border-b-0">
                          <td className="px-4 py-3 font-medium">{t.title}</td>
                          <td className="px-4 py-3">
                            <Badge variant="outline" className="text-xs">
                              {t.treatment_type?.replace(/_/g, ' ')}
                            </Badge>
                          </td>
                          <td className="px-4 py-3">
                            <Badge className={cn(getStatusColor(t.status), 'text-xs')}>
                              {t.status.replace(/_/g, ' ')}
                            </Badge>
                          </td>
                          <td className="px-4 py-3 min-w-[120px]">
                            <div className="flex items-center gap-2">
                              <Progress value={t.progress ?? 0} className="h-2 flex-1" />
                              <span className="text-xs text-muted-foreground w-8 text-right">
                                {t.progress ?? 0}%
                              </span>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-muted-foreground">{formatDate(t.due_date)}</td>
                          <td className="px-4 py-3 text-muted-foreground">{t.assigned_user_name ?? t.assigned_to ?? '---'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Controls */}
        <TabsContent value="controls">
          <Card>
            <CardContent className="p-0">
              {linkedControlIds.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <Shield className="h-8 w-8 mb-2 opacity-40" />
                  <p className="font-medium">No linked controls</p>
                  <p className="text-sm mt-1">Link controls to this risk for traceability.</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">Ref</th>
                        <th className="px-4 py-3 text-left font-medium">Title</th>
                        <th className="px-4 py-3 text-left font-medium">Status</th>
                        <th className="px-4 py-3 text-left font-medium">Maturity</th>
                      </tr>
                    </thead>
                    <tbody>
                      {linkedControlIds.map((cid) => (
                        <LinkedControl key={cid} controlId={cid} />
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* History */}
        <TabsContent value="history">
          <Card>
            <CardContent className="p-0">
              {history.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <p className="font-medium">No history available</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">Date</th>
                        <th className="px-4 py-3 text-left font-medium">Action</th>
                        <th className="px-4 py-3 text-left font-medium">Field</th>
                        <th className="px-4 py-3 text-left font-medium">Old Value</th>
                        <th className="px-4 py-3 text-left font-medium">New Value</th>
                        <th className="px-4 py-3 text-left font-medium">User</th>
                      </tr>
                    </thead>
                    <tbody>
                      {history.map((h) => (
                        <tr key={h.id} className="border-b last:border-b-0">
                          <td className="px-4 py-3 text-muted-foreground">{formatDate(h.created_at)}</td>
                          <td className="px-4 py-3">
                            <Badge variant="outline" className="text-xs">{h.action}</Badge>
                          </td>
                          <td className="px-4 py-3 text-muted-foreground">{h.field ?? '---'}</td>
                          <td className="px-4 py-3 text-muted-foreground">{h.old_value ?? '---'}</td>
                          <td className="px-4 py-3">{h.new_value ?? '---'}</td>
                          <td className="px-4 py-3 text-muted-foreground">{h.user_name ?? '---'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Edit Sheet */}
      <Sheet open={editOpen} onOpenChange={setEditOpen}>
        <SheetContent className="sm:max-w-lg overflow-y-auto">
          <SheetHeader>
            <SheetTitle>Edit Risk</SheetTitle>
            <SheetDescription>Update risk assessment details.</SheetDescription>
          </SheetHeader>
          <div className="mt-6">
            <EditRiskForm risk={risk} onClose={() => setEditOpen(false)} />
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
