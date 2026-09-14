'use client';

import * as React from 'react';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import type { Asset, AssetStatus } from '@/types/asset';
import { formatAssetError, humanizeAssetToken } from '@/lib/asset';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { useUpdateAsset } from '@/lib/api-hooks';

export function AssetStatusDialog({ asset, target, onOpenChange, onRefresh }: { asset: Asset; target: AssetStatus | null; onOpenChange: (open: boolean) => void; onRefresh: () => void }) {
  const mutation = useUpdateAsset(asset.id);
  const [confirmation, setConfirmation] = React.useState('');
  if (!target) return null;
  const decommissioning = target === 'decommissioned';
  const confirmed = !decommissioning || confirmation === asset.asset_ref;
  async function submit() {
    if (!confirmed || !target) return;
    try {
      await mutation.mutateAsync({ expected_version: asset.version, status: target });
      setConfirmation('');
      onOpenChange(false);
    } catch {
      // Mutation error remains visible.
    }
  }
  return <AlertDialog open onOpenChange={(open) => { if (!mutation.isPending && !open) { setConfirmation(''); onOpenChange(false); } }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{humanizeAssetToken(target)} asset?</AlertDialogTitle><AlertDialogDescription>{decommissioning ? 'Decommissioning is irreversible: the asset becomes immutable but its history remains available.' : `Change this asset from ${humanizeAssetToken(asset.status)} to ${humanizeAssetToken(target)}. The versioned change is recorded in history.`}</AlertDialogDescription></AlertDialogHeader>{decommissioning && <div className="space-y-2"><Label htmlFor="asset-status-confirmation">Type <span className="font-mono font-semibold">{asset.asset_ref}</span> to confirm</Label><Input id="asset-status-confirmation" autoComplete="off" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></div>}{mutation.error && <div role="alert" className="space-y-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive"><p>{formatAssetError(mutation.error, 'The asset status could not be changed.')}</p><Button type="button" size="sm" variant="outline" onClick={onRefresh}>Reload latest version</Button></div>}<AlertDialogFooter><AlertDialogCancel disabled={mutation.isPending}>Keep current status</AlertDialogCancel><Button type="button" variant={decommissioning ? 'destructive' : 'default'} disabled={!confirmed || mutation.isPending} onClick={() => void submit()}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}{decommissioning ? 'Decommission asset' : `Mark ${target}`}</Button></AlertDialogFooter></AlertDialogContent></AlertDialog>;
}
