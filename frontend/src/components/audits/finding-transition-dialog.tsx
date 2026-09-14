'use client';

import * as React from 'react';
import type { AuditFinding, FindingStatus } from '@/types/audit';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { formatAuditError, humanizeAuditToken } from '@/lib/audit';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';
import { useUpdateAuditFinding } from '@/lib/api-hooks';

interface FindingTransitionDialogProps {
  auditId: string;
  finding: AuditFinding | null;
  targetStatus: FindingStatus | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function FindingTransitionDialog({
  auditId,
  finding,
  targetStatus,
  open,
  onOpenChange,
}: FindingTransitionDialogProps) {
  const updateFinding = useUpdateAuditFinding(auditId);
  const [acceptedRiskReason, setAcceptedRiskReason] = React.useState(
    () => finding?.accepted_risk_reason ?? '',
  );

  if (!finding || !targetStatus) return null;
  const needsReason = targetStatus === 'accepted';
  const reasonMissing = needsReason && !acceptedRiskReason.trim();

  async function submit() {
    if (!finding || !targetStatus || reasonMissing) return;
    try {
      await updateFinding.mutateAsync({
        findingId: finding.id,
        data: {
          status: targetStatus,
          ...(targetStatus === 'accepted'
            ? { accepted_risk_reason: acceptedRiskReason.trim() }
            : {}),
        },
      });
      onOpenChange(false);
    } catch {
      // Mutation state renders the backend error without dismissing the dialog.
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !updateFinding.isPending && onOpenChange(next)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Move finding to {humanizeAuditToken(targetStatus)}</DialogTitle>
          <DialogDescription>
            Change {finding.finding_ref} from {humanizeAuditToken(finding.status)} to{' '}
            {humanizeAuditToken(targetStatus)}. This transition is recorded in the audit trail.
          </DialogDescription>
        </DialogHeader>

        {needsReason && (
          <div className="space-y-2">
            <Label htmlFor="accepted-risk-reason">Risk acceptance rationale *</Label>
            <Textarea
              id="accepted-risk-reason"
              autoFocus
              rows={4}
              value={acceptedRiskReason}
              aria-invalid={reasonMissing}
              onChange={(event) => setAcceptedRiskReason(event.target.value)}
              placeholder="Document who accepted the risk and why remediation is not being pursued."
            />
            {reasonMissing && (
              <p className="text-sm text-destructive">
                A rationale is required to accept this risk.
              </p>
            )}
          </div>
        )}

        {updateFinding.error && (
          <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {formatAuditError(updateFinding.error, 'Failed to update finding status.')}
          </p>
        )}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={updateFinding.isPending}
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={updateFinding.isPending || reasonMissing}
            onClick={() => void submit()}
          >
            {updateFinding.isPending && (
              <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
            )}
            Confirm transition
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
