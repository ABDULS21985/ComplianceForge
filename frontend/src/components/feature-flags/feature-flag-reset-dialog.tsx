'use client';

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
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
import {
  featureFlagKeys,
  formatFeatureFlagError,
  isFeatureFlagConflict,
} from '@/lib/feature-flags';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';
import type { FeatureFlagEvaluation } from '@/types/feature-flag';

export function FeatureFlagResetDialog({
  evaluation,
  onOpenChange,
  onReload,
  onReset,
  open,
}: {
  evaluation: FeatureFlagEvaluation;
  onOpenChange: (open: boolean) => void;
  onReload: () => void;
  onReset: () => void;
  open: boolean;
}) {
  const queryClient = useQueryClient();
  const [reason, setReason] = React.useState('');
  const [validationError, setValidationError] = React.useState('');
  const mutation = useMutation({
    mutationFn: () => api.featureFlags.resetOverride(evaluation.capability.key, {
      expected_version: evaluation.override?.version ?? 0,
      reason: reason.trim(),
    }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: featureFlagKeys.all });
      toast.success('Tenant override reset to catalogue behavior.');
      onReset();
      onOpenChange(false);
    },
  });

  async function reset() {
    if (reason.trim().length < 3 || reason.trim().length > 1000) {
      setValidationError('Reason must contain 3–1,000 characters.');
      return;
    }
    setValidationError('');
    await mutation.mutateAsync().catch(() => undefined);
  }

  const error = validationError || (mutation.error ? formatFeatureFlagError(mutation.error, 'The override could not be reset.') : '');
  return (
    <AlertDialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Reset tenant override?</AlertDialogTitle>
          <AlertDialogDescription>
            {evaluation.capability.display_name} will immediately return to deployment-managed defaults, entitlement checks, prerequisites, and rollout. The reset is permanent and audited.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error && (
          <div role="alert" className="space-y-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            <p>{error}</p>
            {isFeatureFlagConflict(mutation.error) && <Button type="button" size="sm" variant="outline" onClick={onReload}>Reload current override</Button>}
          </div>
        )}
        <div className="space-y-2"><Label htmlFor="feature-reset-reason">Business reason *</Label><Textarea id="feature-reset-reason" autoFocus maxLength={1000} rows={3} value={reason} onChange={(event) => setReason(event.target.value)} /></div>
        <AlertDialogFooter><AlertDialogCancel disabled={mutation.isPending}>Keep override</AlertDialogCancel><Button type="button" variant="destructive" disabled={mutation.isPending || reason.trim().length < 3} onClick={() => void reset()}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Reset override</Button></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
