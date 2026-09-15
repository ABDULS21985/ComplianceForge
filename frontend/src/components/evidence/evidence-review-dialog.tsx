'use client';

import * as React from 'react';

import { CheckCircle2, Loader2, ShieldCheck, XCircle } from 'lucide-react';
import type { ControlEvidence, ControlEvidenceReviewInput } from '@/types/control-evidence';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { EVIDENCE_REVIEW_COMMENT_MAX_LENGTH, formatEvidenceError } from '@/lib/control-evidence';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';

interface EvidenceReviewDialogProps {
  controlId: string;
  evidence: ControlEvidence | null;
  onOpenChange: (open: boolean) => void;
  onReviewed: (evidence: ControlEvidence) => void;
  open: boolean;
}

export function EvidenceReviewDialog({
  controlId,
  evidence,
  onOpenChange,
  onReviewed,
  open,
}: EvidenceReviewDialogProps) {
  const [comment, setComment] = React.useState('');
  const [error, setError] = React.useState('');
  const [pending, setPending] = React.useState(false);
  const [status, setStatus] = React.useState<ControlEvidenceReviewInput['status']>('accepted');
  const errorRef = React.useRef<HTMLParagraphElement>(null);

  React.useEffect(() => {
    if (error) errorRef.current?.focus();
  }, [error]);

  const changeOpen = (next: boolean) => {
    if (pending) return;
    if (!next) {
      setComment('');
      setError('');
      setStatus('accepted');
    }
    onOpenChange(next);
  };

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!evidence) return;
    const trimmedComment = comment.trim();
    if (status === 'rejected' && !trimmedComment) {
      setError('A rejection rationale is required.');
      return;
    }
    if (trimmedComment.length > EVIDENCE_REVIEW_COMMENT_MAX_LENGTH) {
      setError(`Review notes must not exceed ${EVIDENCE_REVIEW_COMMENT_MAX_LENGTH} characters.`);
      return;
    }
    setError('');
    setPending(true);
    try {
      const reviewed = await api.controls.reviewEvidence(controlId, evidence.id, {
        status,
        comment: trimmedComment || undefined,
      });
      onReviewed(reviewed);
      setComment('');
      setError('');
      setStatus('accepted');
      setPending(false);
      onOpenChange(false);
    } catch (caught) {
      setError(formatEvidenceError(caught, 'review'));
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Review evidence</DialogTitle>
          <DialogDescription>
            Record a review decision for “{evidence?.title ?? 'selected evidence'}”. The reviewer,
            timestamp, status, and notes are retained with the record.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-5" noValidate onSubmit={(event) => void submit(event)}>
          {error && (
            <p
              ref={errorRef}
              role="alert"
              tabIndex={-1}
              className="rounded-md bg-destructive/10 p-3 text-sm text-destructive"
            >
              {error}
            </p>
          )}
          <fieldset disabled={pending} className="space-y-3">
            <legend className="text-sm font-medium">Decision</legend>
            <label className="flex cursor-pointer gap-3 rounded-md border p-3 has-[:checked]:border-primary has-[:checked]:bg-primary/5">
              <input
                type="radio"
                name="evidence-review-status"
                value="accepted"
                checked={status === 'accepted'}
                onChange={() => setStatus('accepted')}
                className="mt-1"
              />
              <CheckCircle2 aria-hidden="true" className="mt-0.5 h-5 w-5 text-emerald-600" />
              <span>
                <span className="block text-sm font-medium">Accept</span>
                <span className="block text-xs text-muted-foreground">
                  The evidence is suitable for the control and stated validity period.
                </span>
              </span>
            </label>
            <label className="flex cursor-pointer gap-3 rounded-md border p-3 has-[:checked]:border-destructive has-[:checked]:bg-destructive/5">
              <input
                type="radio"
                name="evidence-review-status"
                value="rejected"
                checked={status === 'rejected'}
                onChange={() => setStatus('rejected')}
                className="mt-1"
              />
              <XCircle aria-hidden="true" className="mt-0.5 h-5 w-5 text-destructive" />
              <span>
                <span className="block text-sm font-medium">Reject</span>
                <span className="block text-xs text-muted-foreground">
                  The evidence is unsuitable. A rationale is mandatory so the owner can remediate.
                </span>
              </span>
            </label>
          </fieldset>
          <div className="space-y-2">
            <Label htmlFor="evidence-review-comment">
              Review notes {status === 'rejected' ? '(required)' : '(optional)'}
            </Label>
            <Textarea
              id="evidence-review-comment"
              required={status === 'rejected'}
              maxLength={EVIDENCE_REVIEW_COMMENT_MAX_LENGTH}
              rows={5}
              value={comment}
              disabled={pending}
              onChange={(event) => setComment(event.target.value)}
            />
            <p className="text-right text-xs text-muted-foreground">
              {comment.length}/{EVIDENCE_REVIEW_COMMENT_MAX_LENGTH}
            </p>
          </div>
          <div className="flex gap-2 rounded-md border bg-muted/30 p-3 text-xs text-muted-foreground">
            <ShieldCheck aria-hidden="true" className="h-4 w-4 shrink-0 text-primary" />
            <p>Reviewing never changes the stored file, server-derived checksum, or scan result.</p>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => changeOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant={status === 'rejected' ? 'destructive' : 'default'}
              disabled={pending}
            >
              {pending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
              Record {status === 'accepted' ? 'acceptance' : 'rejection'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
