'use client';

import * as React from 'react';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { formatIncidentError } from '@/lib/incident';
import type { Incident } from '@/types/incident';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { useDeleteIncident } from '@/lib/api-hooks';

export function IncidentDeleteDialog({ incident, open, onOpenChange, onDeleted }: { incident: Incident; open: boolean; onOpenChange: (open: boolean) => void; onDeleted: () => void }) {
  const mutation = useDeleteIncident();
  const [confirmation, setConfirmation] = React.useState('');
  async function remove() {
    if (confirmation !== incident.incident_ref) return;
    try {
      await mutation.mutateAsync({ id: incident.id, version: incident.version });
      onOpenChange(false);
      onDeleted();
    } catch {
      // Mutation error remains announced in the dialog.
    }
  }
  function handleOpenChange(next: boolean) {
    if (mutation.isPending) return;
    if (!next) setConfirmation('');
    onOpenChange(next);
  }
  return <AlertDialog open={open} onOpenChange={handleOpenChange}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete incident record?</AlertDialogTitle><AlertDialogDescription>This soft-deletes the terminal incident. Legal hold and an unexpired retention period prevent deletion.</AlertDialogDescription></AlertDialogHeader><div className="space-y-2"><Label htmlFor="incident-delete-confirmation">Type <span className="font-mono font-semibold">{incident.incident_ref}</span> to confirm</Label><Input id="incident-delete-confirmation" autoComplete="off" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></div>{mutation.error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{formatIncidentError(mutation.error, 'The incident could not be deleted.')}</p>}<AlertDialogFooter><AlertDialogCancel disabled={mutation.isPending}>Keep record</AlertDialogCancel><Button type="button" variant="destructive" disabled={mutation.isPending || confirmation !== incident.incident_ref} onClick={() => void remove()}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Delete incident</Button></AlertDialogFooter></AlertDialogContent></AlertDialog>;
}
