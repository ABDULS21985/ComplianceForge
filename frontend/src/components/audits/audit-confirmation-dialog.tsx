'use client';

import * as React from 'react';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { formatAuditError } from '@/lib/audit';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';

interface AuditConfirmationDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: React.ReactNode;
  confirmLabel: string;
  onConfirm: () => Promise<unknown>;
  confirmationText?: string;
  destructive?: boolean;
}

export function AuditConfirmationDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  onConfirm,
  confirmationText,
  destructive = false,
}: AuditConfirmationDialogProps) {
  const [typedValue, setTypedValue] = React.useState('');
  const [isPending, setIsPending] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const confirmationId = React.useId();

  function handleOpenChange(nextOpen: boolean) {
    if (isPending) return;
    if (!nextOpen) {
      setTypedValue('');
      setError(null);
    }
    onOpenChange(nextOpen);
  }

  const confirmed = !confirmationText || typedValue === confirmationText;

  async function confirm() {
    if (!confirmed || isPending) return;
    setError(null);
    setIsPending(true);
    try {
      await onConfirm();
      setTypedValue('');
      setError(null);
      onOpenChange(false);
    } catch (caught) {
      setError(formatAuditError(caught, 'The requested action could not be completed.'));
    } finally {
      setIsPending(false);
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-3">{description}</div>
          </AlertDialogDescription>
        </AlertDialogHeader>

        {confirmationText && (
          <div className="space-y-2">
            <Label htmlFor={confirmationId}>
              Type{' '}
              <span className="font-mono font-semibold text-foreground">{confirmationText}</span> to
              confirm
            </Label>
            <Input
              id={confirmationId}
              autoComplete="off"
              value={typedValue}
              onChange={(event) => setTypedValue(event.target.value)}
            />
          </div>
        )}
        {error && (
          <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </p>
        )}

        <AlertDialogFooter>
          <AlertDialogCancel disabled={isPending}>Keep current state</AlertDialogCancel>
          <Button
            type="button"
            variant={destructive ? 'destructive' : 'default'}
            disabled={!confirmed || isPending}
            onClick={() => void confirm()}
          >
            {isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
            {confirmLabel}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
