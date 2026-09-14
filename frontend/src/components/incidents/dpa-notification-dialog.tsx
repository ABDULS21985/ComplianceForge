'use client';

import * as React from 'react';
import { Bell, Loader2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useNotifyDPA } from '@/lib/api-hooks';
import { formatIncidentError, localDateTimeToIso, toLocalDateTimeInput } from '@/lib/incident';
import type { Incident } from '@/types/incident';

export function DPANotificationDialog({ incident, open, onOpenChange }: { incident: Incident; open: boolean; onOpenChange: (open: boolean) => void }) {
  const mutation = useNotifyDPA(incident.id);
  const [notifiedAt, setNotifiedAt] = React.useState(toLocalDateTimeInput(new Date().toISOString()));
  const [reference, setReference] = React.useState('');
  const [reason, setReason] = React.useState('');
  const [validationError, setValidationError] = React.useState<string | null>(null);
  const idempotencyKey = React.useRef<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const notifiedAtIso = localDateTimeToIso(notifiedAt);
    if (!notifiedAtIso || !reference.trim() || reason.trim().length < 3) {
      setValidationError('Notification time, authority reference, and a reason of at least 3 characters are required.');
      return;
    }
    setValidationError(null);
    try {
      idempotencyKey.current ??= crypto.randomUUID();
      await mutation.mutateAsync({
        version: incident.version,
        idempotency_key: idempotencyKey.current,
        notified_at: notifiedAtIso,
        reference: reference.trim(),
        reason: reason.trim(),
      });
      idempotencyKey.current = null;
      onOpenChange(false);
    } catch {
      // Mutation error remains visible and retries keep the same idempotency key.
    }
  }

  const error = validationError || (mutation.error ? formatIncidentError(mutation.error, 'The DPA notification could not be recorded.') : null);
  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent>
        <DialogHeader><DialogTitle className="flex items-center gap-2"><Bell aria-hidden="true" className="h-5 w-5" />Record DPA notification</DialogTitle><DialogDescription>This records an external notification already submitted to the supervisory authority; it does not send one.</DialogDescription></DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          <div className="space-y-2"><Label htmlFor="dpa-notified-at">Notification time *</Label><Input id="dpa-notified-at" type="datetime-local" max={toLocalDateTimeInput(new Date().toISOString())} value={notifiedAt} onChange={(event) => setNotifiedAt(event.target.value)} /></div>
          <div className="space-y-2"><Label htmlFor="dpa-reference">Authority reference *</Label><Input id="dpa-reference" maxLength={200} value={reference} onChange={(event) => setReference(event.target.value)} /></div>
          <div className="space-y-2"><Label htmlFor="dpa-reason">Notification record notes *</Label><Textarea id="dpa-reason" rows={3} maxLength={4000} value={reason} onChange={(event) => setReason(event.target.value)} /></div>
          <DialogFooter><Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>Cancel</Button><Button type="submit" disabled={mutation.isPending}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Record notification</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
