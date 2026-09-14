'use client';

import * as React from 'react';
import { Loader2 } from 'lucide-react';

import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useDeleteAsset } from '@/lib/api-hooks';
import { formatAssetError } from '@/lib/asset';
import type { Asset } from '@/types/asset';

export function AssetDeleteDialog({ asset, open, onOpenChange, onDeleted, onRefresh }: { asset: Asset; open: boolean; onOpenChange: (open: boolean) => void; onDeleted: () => void; onRefresh: () => void }) {
  const mutation = useDeleteAsset();
  const [confirmation, setConfirmation] = React.useState('');
  async function remove() {
    if (confirmation !== asset.asset_ref) return;
    try {
      await mutation.mutateAsync({ id: asset.id, expectedVersion: asset.version });
      onOpenChange(false); onDeleted();
    } catch {
      // Mutation error remains visible.
    }
  }
  return <AlertDialog open={open} onOpenChange={(next) => { if (!mutation.isPending) { if (!next) setConfirmation(''); onOpenChange(next); } }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete asset?</AlertDialogTitle><AlertDialogDescription>This soft-deletes the inventory record using its current version. Linked controls, risks, incidents, or privacy records may prevent deletion.</AlertDialogDescription></AlertDialogHeader><div className="space-y-2"><Label htmlFor="asset-delete-confirmation">Type <span className="font-mono font-semibold">{asset.asset_ref}</span> to confirm</Label><Input id="asset-delete-confirmation" autoComplete="off" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></div>{mutation.error && <div role="alert" className="space-y-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive"><p>{formatAssetError(mutation.error, 'The asset could not be deleted.')}</p><Button type="button" size="sm" variant="outline" onClick={onRefresh}>Reload latest version</Button></div>}<AlertDialogFooter><AlertDialogCancel disabled={mutation.isPending}>Keep asset</AlertDialogCancel><Button type="button" variant="destructive" disabled={mutation.isPending || confirmation !== asset.asset_ref} onClick={() => void remove()}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Delete asset</Button></AlertDialogFooter></AlertDialogContent></AlertDialog>;
}
