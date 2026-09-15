'use client';

import * as React from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';

export function GovernanceReasonDialog({
  actionLabel,
  danger = false,
  description,
  error,
  onOpenChange,
  onSubmit,
  open,
  pending,
  title,
}: {
  actionLabel: string;
  danger?: boolean;
  description: string;
  error?: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: (reason: string) => void;
  open: boolean;
  pending?: boolean;
  title: string;
}) {
  const [reason, setReason] = React.useState('');
  const [validation, setValidation] = React.useState('');

  function submit(event: React.FormEvent) {
    event.preventDefault();
    const normalized = reason.trim();
    if (normalized.length < 3) {
      setValidation('Give a reason of at least 3 characters for the audit history.');
      return;
    }
    setValidation('');
    onSubmit(normalized);
  }

  const errorId = React.useId();
  return (
    <Dialog open={open} onOpenChange={(next) => !pending && onOpenChange(next)}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-5">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor={`${errorId}-reason`}>Reason *</Label>
            <Textarea
              id={`${errorId}-reason`}
              aria-describedby={validation || error ? errorId : undefined}
              aria-invalid={Boolean(validation || error)}
              autoFocus
              maxLength={2000}
              rows={4}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              placeholder="Explain why this governed change is required"
            />
            {(validation || error) && (
              <p id={errorId} role="alert" className="text-sm text-destructive">
                {validation || error}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" variant={danger ? 'destructive' : 'default'} disabled={pending}>
              {pending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
              {actionLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
