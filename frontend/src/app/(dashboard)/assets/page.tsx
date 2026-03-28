'use client';

import { useState } from 'react';
import Link from 'next/link';
import { z } from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  Server,
  AlertCircle,
  AlertTriangle,
  Plus,
  ExternalLink,
  ChevronLeft,
  ChevronRight,
  Shield,
  Database,
  Monitor,
  Globe,
  Users,
  Building,
  Network,
} from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { getRiskLevelColor, getStatusColor } from '@/lib/utils';
import {
  useAssets,
  useAssetStats,
  useCreateAsset,
} from '@/lib/api-hooks';
import type { Asset, AssetStats, AssetType, AssetCriticality, Classification } from '@/types';
import type { PaginatedResponse } from '@/lib/api';

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

const registerAssetSchema = z.object({
  name: z.string().min(1, 'Asset name is required').max(200),
  asset_type: z.enum(['hardware', 'software', 'data', 'service', 'network', 'people', 'facility']),
  category: z.string().max(100).optional(),
  description: z.string().max(1000).optional(),
  criticality: z.enum(['critical', 'high', 'medium', 'low']),
  owner_user_id: z.string().optional(),
  location: z.string().max(200).optional(),
  classification: z.enum(['public', 'internal', 'confidential', 'restricted']),
  processes_personal_data: z.boolean().default(false),
  linked_vendor_id: z.string().optional(),
  tags: z.array(z.string()).default([]),
});

type RegisterAssetValues = z.infer<typeof registerAssetSchema>;

const ASSET_TYPE_ICONS: Record<string, React.ElementType> = {
  hardware: Monitor,
  software: Server,
  data: Database,
  service: Globe,
  network: Network,
  people: Users,
  facility: Building,
};

const CLASSIFICATION_COLORS: Record<string, string> = {
  public: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
  internal: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
  confidential: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
  restricted: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
};

const ASSET_TYPE_COLORS: Record<string, string> = {
  hardware: 'bg-slate-100 text-slate-800 dark:bg-slate-900/30 dark:text-slate-400',
  software: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
  data: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400',
  service: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/30 dark:text-cyan-400',
  network: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/30 dark:text-indigo-400',
  people: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400',
  facility: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-400',
};

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatCard({
  title,
  value,
  subtitle,
  icon: Icon,
  className,
}: {
  title: string;
  value: string | number;
  subtitle?: string;
  icon: React.ElementType;
  className?: string;
}) {
  return (
    <Card className={className}>
      <CardContent className="p-6">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          <Icon className="h-5 w-5 text-muted-foreground" />
        </div>
        <div className="mt-2">
          <p className="text-3xl font-bold">{value}</p>
          {subtitle && (
            <p className="mt-1 text-xs text-muted-foreground">{subtitle}</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function StatCardSkeleton() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="animate-pulse space-y-3">
          <div className="h-4 w-24 rounded bg-muted" />
          <div className="h-8 w-16 rounded bg-muted" />
          <div className="h-3 w-32 rounded bg-muted" />
        </div>
      </CardContent>
    </Card>
  );
}

function TableSkeleton() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-5 w-40 rounded bg-muted" />
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="flex gap-4">
              <div className="h-4 w-1/6 rounded bg-muted" />
              <div className="h-4 w-1/5 rounded bg-muted" />
              <div className="h-4 w-1/6 rounded bg-muted" />
              <div className="h-4 w-1/6 rounded bg-muted" />
              <div className="h-4 w-1/6 rounded bg-muted" />
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Register Asset Sheet
// ---------------------------------------------------------------------------

function RegisterAssetSheet() {
  const [open, setOpen] = useState(false);
  const createAsset = useCreateAsset();

  const form = useForm<RegisterAssetValues>({
    resolver: zodResolver(registerAssetSchema),
    defaultValues: {
      name: '',
      asset_type: 'software',
      category: '',
      description: '',
      criticality: 'medium',
      owner_user_id: '',
      location: '',
      classification: 'internal',
      processes_personal_data: false,
      linked_vendor_id: '',
      tags: [],
    },
  });

  const processesPersonalData = form.watch('processes_personal_data');
  const [tagInput, setTagInput] = useState('');

  const onSubmit = (values: RegisterAssetValues) => {
    createAsset.mutate(values, {
      onSuccess: () => {
        setOpen(false);
        form.reset();
      },
    });
  };

  const addTag = () => {
    const tag = tagInput.trim();
    if (tag && !form.getValues('tags').includes(tag)) {
      form.setValue('tags', [...form.getValues('tags'), tag]);
      setTagInput('');
    }
  };

  const removeTag = (tag: string) => {
    form.setValue('tags', form.getValues('tags').filter((t) => t !== tag));
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          Register Asset
        </Button>
      </SheetTrigger>
      <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>Register New Asset</SheetTitle>
          <SheetDescription>
            Add a new asset to the inventory for tracking, classification, and compliance.
          </SheetDescription>
        </SheetHeader>
        <form onSubmit={form.handleSubmit(onSubmit)} className="mt-6 space-y-5">
          {/* Name */}
          <div className="space-y-2">
            <Label htmlFor="name">Asset Name *</Label>
            <Input id="name" {...form.register('name')} placeholder="Production Database Server" />
            {form.formState.errors.name && (
              <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
            )}
          </div>

          {/* Asset Type */}
          <div className="space-y-2">
            <Label>Asset Type *</Label>
            <Select
              value={form.watch('asset_type')}
              onValueChange={(v) => form.setValue('asset_type', v as AssetType)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="hardware">Hardware</SelectItem>
                <SelectItem value="software">Software</SelectItem>
                <SelectItem value="data">Data</SelectItem>
                <SelectItem value="service">Service</SelectItem>
                <SelectItem value="network">Network</SelectItem>
                <SelectItem value="people">People</SelectItem>
                <SelectItem value="facility">Facility</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Category */}
          <div className="space-y-2">
            <Label htmlFor="category">Category</Label>
            <Input id="category" {...form.register('category')} placeholder="e.g., Database, Endpoint, SaaS" />
          </div>

          {/* Description */}
          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Textarea
              id="description"
              {...form.register('description')}
              placeholder="Describe the asset..."
              rows={3}
            />
          </div>

          {/* Criticality */}
          <div className="space-y-2">
            <Label>Criticality *</Label>
            <Select
              value={form.watch('criticality')}
              onValueChange={(v) => form.setValue('criticality', v as AssetCriticality)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select criticality" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="critical">Critical</SelectItem>
                <SelectItem value="high">High</SelectItem>
                <SelectItem value="medium">Medium</SelectItem>
                <SelectItem value="low">Low</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Owner */}
          <div className="space-y-2">
            <Label htmlFor="owner_user_id">Owner User ID</Label>
            <Input id="owner_user_id" {...form.register('owner_user_id')} placeholder="User ID" />
          </div>

          {/* Location */}
          <div className="space-y-2">
            <Label htmlFor="location">Location</Label>
            <Input id="location" {...form.register('location')} placeholder="e.g., AWS eu-west-2, London DC1" />
          </div>

          {/* Classification */}
          <div className="space-y-2">
            <Label>Classification *</Label>
            <Select
              value={form.watch('classification')}
              onValueChange={(v) => form.setValue('classification', v as Classification)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select classification" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="public">Public</SelectItem>
                <SelectItem value="internal">Internal</SelectItem>
                <SelectItem value="confidential">Confidential</SelectItem>
                <SelectItem value="restricted">Restricted</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <Separator />

          {/* Personal Data Switch */}
          <div className="flex items-center justify-between">
            <div>
              <Label htmlFor="processes_personal_data">Processes Personal Data</Label>
              <p className="text-xs text-muted-foreground">
                Does this asset process or store personal data?
              </p>
            </div>
            <button
              type="button"
              role="switch"
              aria-checked={processesPersonalData}
              onClick={() => form.setValue('processes_personal_data', !processesPersonalData)}
              className={cn(
                'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors',
                processesPersonalData ? 'bg-primary' : 'bg-muted'
              )}
            >
              <span
                className={cn(
                  'pointer-events-none block h-5 w-5 rounded-full bg-background shadow-lg ring-0 transition-transform',
                  processesPersonalData ? 'translate-x-5' : 'translate-x-0'
                )}
              />
            </button>
          </div>

          {/* GDPR ROPA Notice */}
          {processesPersonalData && (
            <div className="rounded-lg border border-yellow-300 bg-yellow-50 p-4 dark:border-yellow-700 dark:bg-yellow-950/40">
              <div className="flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-yellow-600 dark:text-yellow-400" />
                <div>
                  <p className="text-sm font-medium text-yellow-800 dark:text-yellow-300">
                    GDPR Article 30 &mdash; Record of Processing Activities
                  </p>
                  <p className="mt-1 text-xs text-yellow-700 dark:text-yellow-400">
                    This asset will be flagged for inclusion in the Record of Processing Activities (ROPA)
                    per GDPR Article 30. Ensure processing purposes and legal bases are documented.
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Linked Vendor */}
          <div className="space-y-2">
            <Label htmlFor="linked_vendor_id">Linked Vendor ID (optional)</Label>
            <Input
              id="linked_vendor_id"
              {...form.register('linked_vendor_id')}
              placeholder="Vendor ID if externally managed"
            />
          </div>

          <Separator />

          {/* Tags */}
          <div className="space-y-2">
            <Label>Tags</Label>
            <div className="flex gap-2">
              <Input
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                placeholder="Add a tag"
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    addTag();
                  }
                }}
              />
              <Button type="button" variant="outline" onClick={addTag}>
                Add
              </Button>
            </div>
            {form.watch('tags').length > 0 && (
              <div className="flex flex-wrap gap-1">
                {form.watch('tags').map((tag) => (
                  <Badge
                    key={tag}
                    variant="secondary"
                    className="cursor-pointer"
                    onClick={() => removeTag(tag)}
                  >
                    {tag} &times;
                  </Badge>
                ))}
              </div>
            )}
          </div>

          {/* Submit */}
          <div className="flex gap-3 pt-4">
            <Button type="submit" disabled={createAsset.isPending} className="flex-1">
              {createAsset.isPending ? 'Registering...' : 'Register Asset'}
            </Button>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function AssetsPage() {
  const [page, setPage] = useState(1);
  const pageSize = 20;

  const assets = useAssets({ page, page_size: pageSize });
  const stats = useAssetStats();

  const assetsData = assets.data as PaginatedResponse<Asset> | undefined;
  const statsData = stats.data as AssetStats | undefined;

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Asset Inventory</h1>
          <p className="text-muted-foreground">
            Track and classify all organisational assets for risk management and compliance.
          </p>
        </div>
        <RegisterAssetSheet />
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {stats.isLoading ? (
          Array.from({ length: 4 }).map((_, i) => <StatCardSkeleton key={i} />)
        ) : stats.error ? (
          <Card className="col-span-full">
            <CardContent className="flex items-center gap-2 p-6 text-destructive">
              <AlertCircle className="h-5 w-5" />
              <span>Failed to load asset statistics.</span>
            </CardContent>
          </Card>
        ) : statsData ? (
          <>
            <StatCard
              title="Total Assets"
              value={statsData.total}
              subtitle="All registered assets"
              icon={Server}
            />
            <StatCard
              title="Critical Assets"
              value={statsData.critical}
              subtitle="Highest criticality"
              icon={AlertCircle}
              className="border-l-4 border-l-red-500"
            />
            <StatCard
              title="Personal Data Assets"
              value={statsData.personal_data}
              subtitle="GDPR Article 30 scope"
              icon={Shield}
              className="border-l-4 border-l-yellow-500"
            />
            <Card>
              <CardContent className="p-6">
                <p className="text-sm font-medium text-muted-foreground">By Type</p>
                <div className="mt-3 space-y-2">
                  {statsData.by_type && Object.entries(statsData.by_type).length > 0 ? (
                    Object.entries(statsData.by_type)
                      .sort(([, a], [, b]) => b - a)
                      .slice(0, 4)
                      .map(([type, count]) => (
                        <div key={type} className="flex items-center justify-between text-sm">
                          <span className="capitalize text-muted-foreground">{type}</span>
                          <span className="font-medium">{count}</span>
                        </div>
                      ))
                  ) : (
                    <p className="text-xs text-muted-foreground">No data</p>
                  )}
                </div>
              </CardContent>
            </Card>
          </>
        ) : null}
      </div>

      {/* Data Table */}
      {assets.isLoading ? (
        <TableSkeleton />
      ) : assets.error ? (
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>Failed to load assets. Please try again.</span>
          </CardContent>
        </Card>
      ) : !assetsData?.items?.length ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <Server className="h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">No assets registered</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              Get started by registering your first asset.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">
              Asset Register ({assetsData.total} total)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left">
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Ref</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Name</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Type</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Criticality</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Classification</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Personal Data</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Owner</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Location</th>
                    <th className="pb-3 font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {assetsData.items.map((asset) => {
                    const TypeIcon = ASSET_TYPE_ICONS[asset.asset_type] ?? Server;
                    return (
                      <tr key={asset.id} className="border-b last:border-0 hover:bg-muted/50">
                        <td className="py-3 pr-4 font-mono text-xs text-muted-foreground">
                          {asset.asset_ref}
                        </td>
                        <td className="py-3 pr-4 font-medium">{asset.name}</td>
                        <td className="py-3 pr-4">
                          <Badge className={cn('capitalize', ASSET_TYPE_COLORS[asset.asset_type] ?? '')}>
                            <TypeIcon className="mr-1 h-3 w-3" />
                            {asset.asset_type}
                          </Badge>
                        </td>
                        <td className="py-3 pr-4">
                          <Badge className={cn('capitalize', getRiskLevelColor(asset.criticality))}>
                            {asset.criticality}
                          </Badge>
                        </td>
                        <td className="py-3 pr-4">
                          <Badge className={cn('capitalize', CLASSIFICATION_COLORS[asset.classification] ?? '')}>
                            {asset.classification}
                          </Badge>
                        </td>
                        <td className="py-3 pr-4">
                          {asset.processes_personal_data ? (
                            <div className="flex items-center gap-1 text-yellow-600 dark:text-yellow-400">
                              <AlertTriangle className="h-4 w-4" />
                              <span className="text-xs font-medium">Yes</span>
                            </div>
                          ) : (
                            <span className="text-muted-foreground">No</span>
                          )}
                        </td>
                        <td className="py-3 pr-4 text-muted-foreground">
                          {asset.owner
                            ? `${asset.owner.first_name} ${asset.owner.last_name}`
                            : '—'}
                        </td>
                        <td className="py-3 pr-4 text-muted-foreground">
                          {asset.location ?? '—'}
                        </td>
                        <td className="py-3">
                          <Link href={`/assets/${asset.id}`}>
                            <Button variant="ghost" size="sm">
                              <ExternalLink className="h-4 w-4" />
                            </Button>
                          </Link>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {assetsData.total_pages > 1 && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Page {assetsData.page} of {assetsData.total_pages}
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    <ChevronLeft className="h-4 w-4" />
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => p + 1)}
                    disabled={page >= assetsData.total_pages}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
