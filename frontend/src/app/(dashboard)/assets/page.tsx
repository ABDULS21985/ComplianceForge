'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { AlertTriangle, Boxes, Building2, ChevronLeft, ChevronRight, Database, FilterX, Globe2, LockKeyhole, Monitor, Network, Plus, Search, Server, ShieldCheck, UsersRound } from 'lucide-react';

import { AssetEditorDialog } from '@/components/assets/asset-editor-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { useAssetPermissions } from '@/hooks/use-asset-permissions';
import { useAssets, useAssetStats } from '@/lib/api-hooks';
import { assetPersonName, formatAssetError, humanizeAssetToken } from '@/lib/asset';
import { cn, formatDateTime, getRiskLevelColor, getStatusColor } from '@/lib/utils';
import type { Asset, AssetClassification, AssetCriticality, AssetStatus, AssetType } from '@/types/asset';

const ASSET_TYPES: AssetType[] = ['hardware', 'software', 'data', 'service', 'network', 'people', 'facility'];
const CRITICALITIES: AssetCriticality[] = ['critical', 'high', 'medium', 'low'];
const CLASSIFICATIONS: AssetClassification[] = ['public', 'internal', 'confidential', 'restricted'];
const STATUSES: AssetStatus[] = ['active', 'inactive', 'decommissioned'];
const PAGE_SIZES = [10, 20, 50, 100] as const;
const TYPE_ICONS: Record<AssetType, React.ElementType> = { hardware: Monitor, software: Server, data: Database, service: Globe2, network: Network, people: UsersRound, facility: Building2 };
const CLASSIFICATION_STYLES: Record<AssetClassification, string> = { public: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200', internal: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200', confidential: 'bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200', restricted: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200' };

export default function AssetsPage() {
  const router = useRouter();
  const access = useAssetPermissions();
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState<number>(20);
  const [searchDraft, setSearchDraft] = React.useState('');
  const [search, setSearch] = React.useState('');
  const [tag, setTag] = React.useState('');
  const [assetType, setAssetType] = React.useState<AssetType | 'all'>('all');
  const [criticality, setCriticality] = React.useState<AssetCriticality | 'all'>('all');
  const [classification, setClassification] = React.useState<AssetClassification | 'all'>('all');
  const [status, setStatus] = React.useState<AssetStatus | 'all'>('all');
  const [personalData, setPersonalData] = React.useState<'all' | 'true' | 'false'>('all');
  const [sortBy, setSortBy] = React.useState<'updated_at' | 'created_at' | 'name' | 'asset_ref' | 'asset_type' | 'criticality'>('updated_at');
  const [sortDir, setSortDir] = React.useState<'asc' | 'desc'>('desc');
  const [createOpen, setCreateOpen] = React.useState(false);

  const assetsQuery = useAssets({ page, page_size: pageSize, search: search || undefined, tag: tag || undefined, asset_type: assetType === 'all' ? undefined : assetType, criticality: criticality === 'all' ? undefined : criticality, classification: classification === 'all' ? undefined : classification, status: status === 'all' ? undefined : status, processes_personal_data: personalData === 'all' ? undefined : personalData === 'true', sort_by: sortBy, sort_dir: sortDir }, { enabled: access.canRead });
  const statsQuery = useAssetStats({ enabled: access.canRead });
  const assets = assetsQuery.data?.items ?? [];
  const totalPages = Math.max(assetsQuery.data?.total_pages ?? 0, 1);

  React.useEffect(() => { if (!assetsQuery.data || page <= totalPages) return; const timer = window.setTimeout(() => setPage(totalPages), 0); return () => window.clearTimeout(timer); }, [assetsQuery.data, page, totalPages]);

  if (access.isLoading || !access.user) return <AssetListSkeleton />;
  if (access.isError) return <AccessState title="Asset access could not be verified" description="The permission service is unavailable, so inventory data was not loaded." action={<Button onClick={() => void access.retry()}>Try again</Button>} />;
  if (!access.canRead) return <AccessState title="Asset inventory unavailable" description="Your role does not grant read access to assets." />;

  const filtersActive = Boolean(search || tag || assetType !== 'all' || criticality !== 'all' || classification !== 'all' || status !== 'all' || personalData !== 'all');
  function applySearch(event: React.FormEvent) { event.preventDefault(); setSearch(searchDraft.trim()); setTag(tag.trim().toLowerCase()); setPage(1); }
  function resetFilters() { setSearchDraft(''); setSearch(''); setTag(''); setAssetType('all'); setCriticality('all'); setClassification('all'); setStatus('all'); setPersonalData('all'); setPage(1); }

  return <div className="space-y-6">
    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div><h1 className="text-3xl font-bold tracking-tight">Asset inventory</h1><p className="mt-1 text-muted-foreground">Maintain ownership, sensitivity, criticality, and lifecycle evidence for enterprise assets.</p></div>{access.canCreate && <Button className="w-full sm:w-auto" onClick={() => setCreateOpen(true)}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Register asset</Button>}</div>

    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="Asset portfolio metrics"><SummaryCard label="Total assets" value={statsQuery.data?.total} loading={statsQuery.isLoading} icon={<Boxes aria-hidden="true" className="h-4 w-4 text-blue-600" />} /><SummaryCard label="Active" value={statsQuery.data?.active} loading={statsQuery.isLoading} icon={<ShieldCheck aria-hidden="true" className="h-4 w-4 text-emerald-600" />} /><SummaryCard label="Critical" value={statsQuery.data?.critical} loading={statsQuery.isLoading} highlight={Boolean(statsQuery.data?.critical)} icon={<AlertTriangle aria-hidden="true" className="h-4 w-4 text-red-600" />} /><SummaryCard label="Processes personal data" value={statsQuery.data?.personal_data} loading={statsQuery.isLoading} highlight={Boolean(statsQuery.data?.personal_data)} icon={<Database aria-hidden="true" className="h-4 w-4 text-purple-600" />} /></div>
    {statsQuery.isError && <div role="alert" className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200">Asset statistics are temporarily unavailable.</div>}

    <Card><CardHeader className="pb-4"><CardTitle className="text-base">Find assets</CardTitle></CardHeader><CardContent><form role="search" className="grid gap-3 md:grid-cols-2 xl:grid-cols-8" onSubmit={applySearch}>
      <div className="space-y-2 md:col-span-2"><Label htmlFor="asset-search">Reference, name, category, or description</Label><div className="relative"><Search aria-hidden="true" className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" /><Input id="asset-search" className="pl-9" maxLength={200} value={searchDraft} onChange={(event) => setSearchDraft(event.target.value)} /></div></div>
      <FilterSelect id="asset-type-filter" label="Type" value={assetType} options={ASSET_TYPES} onValueChange={(value) => { setAssetType(value as AssetType | 'all'); setPage(1); }} />
      <FilterSelect id="asset-criticality-filter" label="Criticality" value={criticality} options={CRITICALITIES} onValueChange={(value) => { setCriticality(value as AssetCriticality | 'all'); setPage(1); }} />
      <FilterSelect id="asset-classification-filter" label="Classification" value={classification} options={CLASSIFICATIONS} onValueChange={(value) => { setClassification(value as AssetClassification | 'all'); setPage(1); }} />
      <FilterSelect id="asset-status-filter" label="Status" value={status} options={STATUSES} onValueChange={(value) => { setStatus(value as AssetStatus | 'all'); setPage(1); }} />
      <FilterSelect id="asset-privacy-filter" label="Personal data" value={personalData} options={['true', 'false']} labels={{ true: 'Processes data', false: 'Does not process' }} onValueChange={(value) => { setPersonalData(value as typeof personalData); setPage(1); }} />
      <div className="space-y-2"><Label htmlFor="asset-tag-filter">Tag</Label><Input id="asset-tag-filter" maxLength={64} value={tag} onChange={(event) => setTag(event.target.value)} /></div>
      <div className="flex items-end gap-2 md:col-span-2"><Button type="submit">Search</Button>{filtersActive && <Button type="button" variant="outline" onClick={resetFilters}><FilterX aria-hidden="true" className="mr-2 h-4 w-4" />Reset</Button>}</div>
    </form><div className="mt-4 flex flex-wrap items-end gap-3 border-t pt-4"><FilterSelect id="asset-sort" label="Sort by" value={sortBy} options={['updated_at', 'created_at', 'name', 'asset_ref', 'asset_type', 'criticality']} includeAll={false} onValueChange={(value) => { setSortBy(value as typeof sortBy); setPage(1); }} /><FilterSelect id="asset-sort-direction" label="Direction" value={sortDir} options={['desc', 'asc']} labels={{ desc: 'Descending', asc: 'Ascending' }} includeAll={false} onValueChange={(value) => { setSortDir(value as 'asc' | 'desc'); setPage(1); }} /></div></CardContent></Card>

    <Card><CardContent className="p-0">{assetsQuery.isLoading ? <div className="space-y-3 p-6" aria-label="Loading assets">{Array.from({ length: 5 }).map((_, index) => <Skeleton key={index} className="h-14" />)}</div> : assetsQuery.isError ? <ListError message={formatAssetError(assetsQuery.error, 'Assets could not be loaded.')} retry={() => void assetsQuery.refetch()} /> : assets.length === 0 ? <EmptyState filtered={filtersActive} create={access.canCreate ? () => setCreateOpen(true) : undefined} /> : <><AssetTable assets={assets} /><AssetCards assets={assets} /></>}
      {assetsQuery.data && assetsQuery.data.total > 0 && <div className="flex flex-col gap-3 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-center gap-2"><Label htmlFor="asset-page-size" className="text-sm text-muted-foreground">Rows</Label><Select value={String(pageSize)} onValueChange={(value) => { setPageSize(Number(value)); setPage(1); }}><SelectTrigger id="asset-page-size" className="w-20"><SelectValue /></SelectTrigger><SelectContent>{PAGE_SIZES.map((value) => <SelectItem key={value} value={String(value)}>{value}</SelectItem>)}</SelectContent></Select><p className="text-sm text-muted-foreground">Page {page} of {totalPages} · {assetsQuery.data.total} total</p></div><div className="flex gap-2"><Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button><Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button></div></div>}
    </CardContent></Card>
    {access.canCreate && <AssetEditorDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={(asset) => router.push(`/assets/${asset.id}`)} />}
  </div>;
}

function AssetTable({ assets }: { assets: Asset[] }) { return <div className="hidden overflow-x-auto md:block"><table className="w-full text-sm"><caption className="sr-only">Asset search results</caption><thead><tr className="border-b bg-muted/50"><th scope="col" className="px-4 py-3 text-left font-medium">Asset</th><th scope="col" className="px-4 py-3 text-left font-medium">Type</th><th scope="col" className="px-4 py-3 text-left font-medium">Criticality</th><th scope="col" className="px-4 py-3 text-left font-medium">Classification</th><th scope="col" className="px-4 py-3 text-left font-medium">Owner</th><th scope="col" className="px-4 py-3 text-left font-medium">Status</th><th scope="col" className="px-4 py-3 text-left font-medium">Updated</th></tr></thead><tbody>{assets.map((asset) => { const Icon = TYPE_ICONS[asset.asset_type]; return <tr key={asset.id} className="border-b last:border-0 hover:bg-muted/40"><td className="max-w-sm px-4 py-3"><Link href={`/assets/${asset.id}`} className="font-medium text-primary hover:underline">{asset.name}</Link><span className="mt-1 block font-mono text-xs text-muted-foreground">{asset.asset_ref}</span></td><td className="px-4 py-3"><span className="inline-flex items-center gap-2"><Icon aria-hidden="true" className="h-4 w-4" />{humanizeAssetToken(asset.asset_type)}</span></td><td className="px-4 py-3"><Badge className={getRiskLevelColor(asset.criticality)}>{humanizeAssetToken(asset.criticality)}</Badge></td><td className="px-4 py-3"><Badge className={CLASSIFICATION_STYLES[asset.classification]}>{humanizeAssetToken(asset.classification)}</Badge></td><td className="px-4 py-3">{assetPersonName(asset.owner, asset.owner_user_id)}</td><td className="px-4 py-3"><Badge className={getStatusColor(asset.status)}>{humanizeAssetToken(asset.status)}</Badge></td><td className="whitespace-nowrap px-4 py-3">{formatDateTime(asset.updated_at)}</td></tr>; })}</tbody></table></div>; }
function AssetCards({ assets }: { assets: Asset[] }) { return <ul className="divide-y md:hidden">{assets.map((asset) => { const Icon = TYPE_ICONS[asset.asset_type]; return <li key={asset.id}><Link href={`/assets/${asset.id}`} className="block space-y-3 p-4 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"><div className="flex items-start gap-3"><Icon aria-hidden="true" className="mt-1 h-5 w-5 shrink-0" /><div className="min-w-0"><p className="font-medium">{asset.name}</p><p className="font-mono text-xs text-muted-foreground">{asset.asset_ref}</p></div></div><div className="flex flex-wrap gap-2"><Badge className={getRiskLevelColor(asset.criticality)}>{humanizeAssetToken(asset.criticality)}</Badge><Badge className={CLASSIFICATION_STYLES[asset.classification]}>{humanizeAssetToken(asset.classification)}</Badge><Badge className={getStatusColor(asset.status)}>{humanizeAssetToken(asset.status)}</Badge>{asset.processes_personal_data && <Badge variant="outline">Personal data</Badge>}</div><p className="text-xs text-muted-foreground">Owner: {assetPersonName(asset.owner, asset.owner_user_id)}</p></Link></li>; })}</ul>; }
function FilterSelect({ id, label, value, options, labels = {}, includeAll = true, onValueChange }: { id: string; label: string; value: string; options: readonly string[]; labels?: Record<string, string>; includeAll?: boolean; onValueChange: (value: string) => void }) { return <div className="space-y-2"><Label htmlFor={id}>{label}</Label><Select value={value} onValueChange={onValueChange}><SelectTrigger id={id}><SelectValue /></SelectTrigger><SelectContent>{includeAll && <SelectItem value="all">All</SelectItem>}{options.map((option) => <SelectItem key={option} value={option}>{labels[option] ?? humanizeAssetToken(option)}</SelectItem>)}</SelectContent></Select></div>; }
function SummaryCard({ label, value, loading, highlight, icon }: { label: string; value?: number; loading: boolean; highlight?: boolean; icon: React.ReactNode }) { return <Card className={cn(highlight && 'border-orange-400')}><CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2"><CardTitle className="text-sm font-medium">{label}</CardTitle>{icon}</CardHeader><CardContent>{loading ? <Skeleton className="h-8 w-16" /> : <p className={cn('text-2xl font-bold', highlight && 'text-orange-700 dark:text-orange-300')}>{value ?? '—'}</p>}</CardContent></Card>; }
function ListError({ message, retry }: { message: string; retry: () => void }) { return <div role="alert" className="flex flex-col items-center gap-3 p-10 text-center"><AlertTriangle aria-hidden="true" className="h-9 w-9 text-destructive" /><p>{message}</p><Button variant="outline" onClick={retry}>Retry</Button></div>; }
function EmptyState({ filtered, create }: { filtered: boolean; create?: () => void }) { return <div className="flex flex-col items-center gap-3 p-12 text-center"><Boxes aria-hidden="true" className="h-10 w-10 text-muted-foreground" /><h2 className="text-lg font-medium">{filtered ? 'No assets match these filters' : 'No assets registered'}</h2><p className="text-sm text-muted-foreground">{filtered ? 'Adjust or reset the filters to broaden the result set.' : 'Register an asset to establish the inventory.'}</p>{create && !filtered && <Button onClick={create}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Register asset</Button>}</div>; }
function AccessState({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) { return <Card><CardContent role="alert" className="flex flex-col items-center gap-3 py-12 text-center"><LockKeyhole aria-hidden="true" className="h-9 w-9" /><h1 className="text-xl font-semibold">{title}</h1><p className="max-w-lg text-sm text-muted-foreground">{description}</p>{action}</CardContent></Card>; }
function AssetListSkeleton() { return <div className="space-y-5" aria-label="Loading asset inventory"><Skeleton className="h-10 w-72" /><div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-28" />)}</div><Skeleton className="h-80" /></div>; }
