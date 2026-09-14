'use client';

import * as React from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { AlertTriangle, ArrowLeft, ChevronLeft, ChevronRight, Database, History, LockKeyhole, MapPin, Pencil, Server, Shield, Tags, Trash2, UserRound } from 'lucide-react';

import { AssetDeleteDialog } from '@/components/assets/asset-delete-dialog';
import { AssetEditorDialog } from '@/components/assets/asset-editor-dialog';
import { AssetStatusDialog } from '@/components/assets/asset-status-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useAssetPermissions } from '@/hooks/use-asset-permissions';
import { useAsset, useAssetEvents } from '@/lib/api-hooks';
import { assetNextStatuses, assetPersonName, formatAssetError, humanizeAssetToken } from '@/lib/asset';
import { cn, formatDateTime, getRiskLevelColor, getStatusColor } from '@/lib/utils';
import type { AssetLifecycleEvent, AssetStatus } from '@/types/asset';

const CLASSIFICATION_STYLES = { public: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200', internal: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200', confidential: 'bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200', restricted: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200' } as const;

export default function AssetDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const access = useAssetPermissions();
  const assetQuery = useAsset(id, { enabled: access.canRead && Boolean(id) });
  const [historyPage, setHistoryPage] = React.useState(1);
  const historyQuery = useAssetEvents(id, { page: historyPage, page_size: 20 }, { enabled: access.canRead && Boolean(id) });
  const [editOpen, setEditOpen] = React.useState(false);
  const [statusTarget, setStatusTarget] = React.useState<AssetStatus | null>(null);
  const [deleteOpen, setDeleteOpen] = React.useState(false);

  if (access.isLoading || !access.user) return <DetailSkeleton />;
  if (access.isError) return <AccessState title="Asset access could not be verified" description="The permission service is unavailable, so the asset was not loaded." action={<Button onClick={() => void access.retry()}>Try again</Button>} />;
  if (!access.canRead) return <AccessState title="Asset unavailable" description="Your role does not grant read access to asset records." />;
  if (assetQuery.isLoading) return <DetailSkeleton />;
  if (assetQuery.isError || !assetQuery.data) return <LoadError error={assetQuery.error} retry={() => void assetQuery.refetch()} />;

  const asset = assetQuery.data;
  const immutable = asset.status === 'decommissioned';
  const nextStatuses = assetNextStatuses(asset.status);
  const totalHistoryPages = Math.max(historyQuery.data?.total_pages ?? 0, 1);

  return <div className="space-y-6">
    <Link href="/assets" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"><ArrowLeft aria-hidden="true" className="mr-1 h-4 w-4" />Back to assets</Link>
    <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between"><div className="min-w-0 space-y-2"><div className="flex flex-wrap items-center gap-2"><span className="font-mono text-sm text-muted-foreground">{asset.asset_ref}</span><Badge variant="outline">{humanizeAssetToken(asset.asset_type)}</Badge><Badge className={getRiskLevelColor(asset.criticality)}>{humanizeAssetToken(asset.criticality)}</Badge><Badge className={CLASSIFICATION_STYLES[asset.classification]}>{humanizeAssetToken(asset.classification)}</Badge><Badge className={getStatusColor(asset.status)}>{humanizeAssetToken(asset.status)}</Badge></div><h1 className="break-words text-3xl font-bold tracking-tight">{asset.name}</h1><p className="text-sm text-muted-foreground">Version {asset.version} · Updated {formatDateTime(asset.updated_at)}</p></div><div className="flex flex-wrap gap-2">{access.canUpdate && !immutable && <Button variant="outline" onClick={() => setEditOpen(true)}><Pencil aria-hidden="true" className="mr-2 h-4 w-4" />Edit asset</Button>}{access.canUpdate && nextStatuses.map((status) => <Button key={status} variant={status === 'decommissioned' ? 'destructive' : 'default'} onClick={() => setStatusTarget(status)}>{status === 'decommissioned' ? 'Decommission' : `Mark ${status}`}</Button>)}{access.canDelete && <Button variant="destructive" onClick={() => setDeleteOpen(true)}><Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />Delete</Button>}</div></div>
    {immutable && <div role="status" className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200"><AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />This asset is decommissioned and immutable. Its record and append-only lifecycle history remain available for audit.</div>}

    <div className="grid gap-5 lg:grid-cols-2"><Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Server aria-hidden="true" className="h-5 w-5" />Inventory profile</CardTitle></CardHeader><CardContent className="space-y-5"><Detail label="Description" value={asset.description || 'Not recorded'} /><div className="grid gap-4 sm:grid-cols-2"><Detail label="Category" value={asset.category || '—'} /><Detail label="Location" value={asset.location || '—'} icon={<MapPin aria-hidden="true" className="h-4 w-4" />} /><Detail label="IP address" value={asset.ip_address || '—'} mono /><Detail label="Created" value={formatDateTime(asset.created_at)} /></div></CardContent></Card>
      <Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Shield aria-hidden="true" className="h-5 w-5" />Ownership and data scope</CardTitle></CardHeader><CardContent className="space-y-5"><Detail label="Owner" value={assetPersonName(asset.owner, asset.owner_user_id)} icon={<UserRound aria-hidden="true" className="h-4 w-4" />} />{asset.owner?.email && <Detail label="Owner email" value={asset.owner.email} />}<Detail label="Processes personal data" value={asset.processes_personal_data ? 'Yes — include in privacy scope' : 'No'} icon={<Database aria-hidden="true" className="h-4 w-4" />} /><Detail label="Linked vendor ID" value={asset.linked_vendor_id || 'None'} mono /><Detail label="Created by" value={asset.created_by} mono /></CardContent></Card></div>

    <div className="grid gap-5 lg:grid-cols-2"><Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Tags aria-hidden="true" className="h-5 w-5" />Tags</CardTitle></CardHeader><CardContent>{asset.tags.length ? <ul className="flex flex-wrap gap-2">{asset.tags.map((tag) => <li key={tag}><Badge variant="outline">{tag}</Badge></li>)}</ul> : <p className="text-sm text-muted-foreground">No tags assigned.</p>}</CardContent></Card><Card><CardHeader><CardTitle className="text-lg">Metadata</CardTitle></CardHeader><CardContent><MetadataTable metadata={asset.metadata} /></CardContent></Card></div>

    <Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><History aria-hidden="true" className="h-5 w-5" />Lifecycle history</CardTitle></CardHeader><CardContent><AssetHistory events={historyQuery.data?.items ?? []} loading={historyQuery.isLoading} error={historyQuery.isError} retry={() => void historyQuery.refetch()} />{historyQuery.data && historyQuery.data.total > 0 && <div className="mt-5 flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between"><p className="text-sm text-muted-foreground">Page {historyPage} of {totalHistoryPages} · {historyQuery.data.total} events</p><div className="flex gap-2"><Button variant="outline" size="sm" disabled={historyPage <= 1} onClick={() => setHistoryPage((value) => value - 1)}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button><Button variant="outline" size="sm" disabled={historyPage >= totalHistoryPages} onClick={() => setHistoryPage((value) => value + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button></div></div>}</CardContent></Card>

    {access.canUpdate && !immutable && <AssetEditorDialog asset={asset} open={editOpen} onOpenChange={setEditOpen} />}
    {access.canUpdate && <AssetStatusDialog asset={asset} target={statusTarget} onOpenChange={(open) => !open && setStatusTarget(null)} onRefresh={() => { setStatusTarget(null); void assetQuery.refetch(); }} />}
    {access.canDelete && <AssetDeleteDialog asset={asset} open={deleteOpen} onOpenChange={setDeleteOpen} onRefresh={() => { setDeleteOpen(false); void assetQuery.refetch(); }} onDeleted={() => { router.push('/assets'); router.refresh(); }} />}
  </div>;
}

function AssetHistory({ events, loading, error, retry }: { events: AssetLifecycleEvent[]; loading: boolean; error: boolean; retry: () => void }) {
  if (loading) return <div className="space-y-3" aria-label="Loading asset history">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-16" />)}</div>;
  if (error) return <div role="alert" className="flex items-center justify-between gap-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive"><span>Asset history could not be loaded.</span><Button variant="outline" size="sm" onClick={retry}>Retry</Button></div>;
  if (!events.length) return <p className="py-5 text-center text-sm text-muted-foreground">No lifecycle events recorded.</p>;
  return <ol className="space-y-0">{events.map((event, index) => <li key={event.id} className="flex gap-3"><div className="flex flex-col items-center"><span className="mt-1 flex h-7 w-7 items-center justify-center rounded-full border bg-background"><History aria-hidden="true" className="h-3.5 w-3.5" /></span>{index < events.length - 1 && <span aria-hidden="true" className="w-px flex-1 bg-border" />}</div><div className="min-w-0 flex-1 pb-5"><div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between"><div><p className="font-medium">{humanizeAssetToken(event.event_type)}</p><p className="text-xs text-muted-foreground">Asset version {event.asset_version} · actor <span className="font-mono">{event.actor_user_id}</span></p></div><time dateTime={event.created_at} className="whitespace-nowrap text-xs text-muted-foreground">{formatDateTime(event.created_at)}</time></div><EventDetails details={event.details} /></div></li>)}</ol>;
}

function EventDetails({ details }: { details: Record<string, unknown> }) { const entries = Object.entries(details); if (!entries.length) return null; return <dl className="mt-2 grid gap-x-4 gap-y-1 rounded-md bg-muted/50 p-3 text-xs sm:grid-cols-2">{entries.map(([key, value]) => <div key={key} className="flex gap-2"><dt className="font-medium">{humanizeAssetToken(key)}:</dt><dd className="break-all text-muted-foreground">{typeof value === 'string' ? value : JSON.stringify(value)}</dd></div>)}</dl>; }
function MetadataTable({ metadata }: { metadata: Record<string, unknown> }) { const entries = Object.entries(metadata); if (!entries.length) return <p className="text-sm text-muted-foreground">No custom metadata.</p>; return <dl className="grid gap-3 sm:grid-cols-2">{entries.map(([key, value]) => <div key={key}><dt className="text-sm font-medium">{humanizeAssetToken(key)}</dt><dd className="mt-1 break-all text-sm text-muted-foreground">{typeof value === 'string' ? value : JSON.stringify(value)}</dd></div>)}</dl>; }
function Detail({ label, value, mono, icon }: { label: string; value: React.ReactNode; mono?: boolean; icon?: React.ReactNode }) { return <div><p className="flex items-center gap-1.5 text-sm font-medium text-muted-foreground">{icon}{label}</p><p className={cn('mt-1 break-words whitespace-pre-wrap text-sm', mono && 'font-mono text-xs')}>{value || '—'}</p></div>; }
function DetailSkeleton() { return <div className="space-y-5" aria-label="Loading asset"><Skeleton className="h-5 w-32" /><Skeleton className="h-12 w-3/4" /><div className="grid gap-5 lg:grid-cols-2"><Skeleton className="h-72" /><Skeleton className="h-72" /></div><Skeleton className="h-64" /></div>; }
function AccessState({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) { return <Card><CardContent role="alert" className="flex flex-col items-center gap-3 py-12 text-center"><LockKeyhole aria-hidden="true" className="h-9 w-9" /><h1 className="text-xl font-semibold">{title}</h1><p className="max-w-lg text-sm text-muted-foreground">{description}</p>{action}</CardContent></Card>; }
function LoadError({ error, retry }: { error: unknown; retry: () => void }) { return <div className="space-y-5"><Link href="/assets" className="inline-flex items-center text-sm text-muted-foreground"><ArrowLeft aria-hidden="true" className="mr-1 h-4 w-4" />Back to assets</Link><Card><CardContent role="alert" className="flex flex-col items-center gap-3 py-12 text-center"><AlertTriangle aria-hidden="true" className="h-9 w-9 text-destructive" /><h1 className="text-xl font-semibold">Asset could not be loaded</h1><p className="text-sm text-muted-foreground">{formatAssetError(error, 'The asset was not found or is temporarily unavailable.')}</p><Button variant="outline" onClick={retry}>Retry</Button></CardContent></Card></div>; }
