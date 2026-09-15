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
import {
  formatGovernanceError,
  isGovernanceConflict,
  localDateTimeInput,
  parseGovernanceObject,
  toISOStringOrUndefined,
} from '@/lib/data-governance';
import { Loader2, RefreshCw, UserPlus, X } from 'lucide-react';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type { DirectoryUser } from '@/types/directory';
import { DirectoryUserPicker } from '@/components/access-admin/directory-user-picker';
import { GovernanceWarning } from './governance-states';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import type { LegalHold } from '@/types/data-governance';
import { Textarea } from '@/components/ui/textarea';
import { useMutation } from '@tanstack/react-query';

interface HoldDraft {
  description: string;
  legalAuthority: string;
  matterReference: string;
  name: string;
  reason: string;
  reviewDueAt: string;
  scope: string;
}

function initialDraft(hold?: LegalHold): HoldDraft {
  return {
    description: hold?.description ?? '',
    legalAuthority: hold?.legal_authority ?? '',
    matterReference: hold?.matter_reference ?? '',
    name: hold?.name ?? '',
    reason: '',
    reviewDueAt: localDateTimeInput(hold?.review_due_at),
    scope: JSON.stringify(hold?.scope ?? {}, null, 2),
  };
}

export function LegalHoldEditorDialog({
  hold,
  onOpenChange,
  onReload,
  onSaved,
  open,
}: {
  hold?: LegalHold;
  onOpenChange: (open: boolean) => void;
  onReload: () => void;
  onSaved: (hold: LegalHold) => void;
  open: boolean;
}) {
  const [draft, setDraft] = React.useState(() => initialDraft(hold));
  const [owner, setOwner] = React.useState<DirectoryUser | null>(null);
  const [changeOwner, setChangeOwner] = React.useState(!hold);
  const [custodianCandidate, setCustodianCandidate] = React.useState<DirectoryUser | null>(null);
  const [custodians, setCustodians] = React.useState<DirectoryUser[]>([]);
  const [validation, setValidation] = React.useState('');

  const mutation = useMutation({
    mutationFn: () => {
      const reason = draft.reason.trim();
      const reviewDueAt = toISOStringOrUndefined(draft.reviewDueAt);
      if (draft.name.trim().length < 3) throw new Error('Hold name must be at least 3 characters.');
      if (draft.description.trim().length < 3)
        throw new Error('Description must be at least 3 characters.');
      if (draft.legalAuthority.trim().length < 3)
        throw new Error('Legal authority must be at least 3 characters.');
      if (!hold && !owner) throw new Error('Choose an active owner for the legal hold.');
      if (draft.reviewDueAt && (!reviewDueAt || new Date(reviewDueAt) <= new Date()))
        throw new Error('Review due date must be in the future.');
      if (reason.length < 3)
        throw new Error('Give a reason of at least 3 characters for the audit history.');
      const scope = parseGovernanceObject(draft.scope, 'Legal hold scope');
      if (hold) {
        return api.dataGovernance.updateHold(hold.id, {
          expected_version: hold.version,
          name: draft.name.trim(),
          matter_reference: draft.matterReference.trim() || undefined,
          clear_matter_reference: !draft.matterReference.trim(),
          description: draft.description.trim(),
          legal_authority: draft.legalAuthority.trim(),
          scope,
          owner_user_id: owner?.id,
          review_due_at: reviewDueAt,
          clear_review_due_at: reviewDueAt === undefined,
          reason,
        });
      }
      return api.dataGovernance.createHold({
        name: draft.name.trim(),
        matter_reference: draft.matterReference.trim() || undefined,
        description: draft.description.trim(),
        legal_authority: draft.legalAuthority.trim(),
        scope,
        owner_user_id: owner!.id,
        review_due_at: reviewDueAt,
        custodian_ids: custodians.map((user) => user.id),
        reason,
      });
    },
    onSuccess: (saved) => {
      onSaved(saved);
      onOpenChange(false);
    },
    onError: (error) =>
      setValidation(formatGovernanceError(error, 'The legal hold could not be saved.')),
  });

  function addCustodian(user: DirectoryUser | null) {
    setCustodianCandidate(user);
    if (!user) return;
    setCustodians((current) =>
      current.some((item) => item.id === user.id) ? current : [...current, user],
    );
    setCustodianCandidate(null);
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto">
        <form
          className="space-y-5"
          onSubmit={(event) => {
            event.preventDefault();
            setValidation('');
            mutation.mutate();
          }}
        >
          <DialogHeader>
            <DialogTitle>{hold ? 'Edit legal hold' : 'Place legal hold'}</DialogTitle>
            <DialogDescription>
              Legal holds suspend disposition for records in scope. Ownership and every change are
              audited.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Hold name" htmlFor="hold-name">
              <Input
                id="hold-name"
                required
                minLength={3}
                maxLength={200}
                value={draft.name}
                onChange={(event) => setDraft({ ...draft, name: event.target.value })}
              />
            </Field>
            <Field label="Matter reference" htmlFor="hold-matter">
              <Input
                id="hold-matter"
                maxLength={200}
                value={draft.matterReference}
                onChange={(event) => setDraft({ ...draft, matterReference: event.target.value })}
              />
            </Field>
          </div>
          <Field label="Description" htmlFor="hold-description">
            <Textarea
              id="hold-description"
              required
              minLength={3}
              maxLength={20000}
              rows={3}
              value={draft.description}
              onChange={(event) => setDraft({ ...draft, description: event.target.value })}
            />
          </Field>
          <Field label="Legal authority" htmlFor="hold-authority">
            <Textarea
              id="hold-authority"
              required
              minLength={3}
              maxLength={4000}
              rows={3}
              value={draft.legalAuthority}
              onChange={(event) => setDraft({ ...draft, legalAuthority: event.target.value })}
            />
          </Field>
          <Field label="Scope (JSON object)" htmlFor="hold-scope">
            <Textarea
              id="hold-scope"
              className="font-mono text-xs"
              rows={5}
              value={draft.scope}
              onChange={(event) => setDraft({ ...draft, scope: event.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              Record selectors and matter-specific constraints; the server stores the object as an
              audited snapshot.
            </p>
          </Field>
          <Field label="Review due date (optional)" htmlFor="hold-review">
            <Input
              id="hold-review"
              type="datetime-local"
              value={draft.reviewDueAt}
              onChange={(event) => setDraft({ ...draft, reviewDueAt: event.target.value })}
            />
          </Field>
          <div className="space-y-2">
            <Label>Hold owner *</Label>
            {hold && !changeOwner ? (
              <div className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <p className="text-sm font-medium">Current owner reference</p>
                  <p className="break-all font-mono text-xs text-muted-foreground">
                    {hold.owner_user_id}
                  </p>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setChangeOwner(true)}
                >
                  Change owner
                </Button>
              </div>
            ) : (
              <>
                <DirectoryUserPicker
                  disabled={mutation.isPending}
                  value={owner}
                  onChange={setOwner}
                  searchLabel="Search for hold owner"
                  selectedLabel="Selected hold owner"
                />
                {hold && (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setOwner(null);
                      setChangeOwner(false);
                    }}
                  >
                    Keep current owner
                  </Button>
                )}
              </>
            )}
          </div>
          {!hold ? (
            <div className="space-y-3">
              <div>
                <Label>Custodians (optional)</Label>
                <p className="mt-1 text-xs text-muted-foreground">
                  Add active users who are custodians for this matter. Custodian membership is fixed
                  at placement by the current API contract.
                </p>
              </div>
              {custodians.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {custodians.map((user) => (
                    <Badge key={user.id} variant="secondary" className="gap-1 py-1">
                      {user.first_name} {user.last_name}
                      <button
                        type="button"
                        className="rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        aria-label={`Remove custodian ${user.first_name} ${user.last_name}`}
                        onClick={() =>
                          setCustodians((current) => current.filter((item) => item.id !== user.id))
                        }
                      >
                        <X aria-hidden="true" className="h-3 w-3" />
                      </button>
                    </Badge>
                  ))}
                </div>
              )}
              <div className="rounded-md bg-muted/40 p-3">
                <p className="mb-2 flex items-center gap-2 text-sm font-medium">
                  <UserPlus aria-hidden="true" className="h-4 w-4" />
                  Add custodian
                </p>
                <DirectoryUserPicker
                  disabled={mutation.isPending}
                  value={custodianCandidate}
                  onChange={addCustodian}
                  searchLabel="Search for custodian"
                  selectedLabel="Selected custodian"
                />
              </div>
            </div>
          ) : (
            <div className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
              {hold.custodian_ids.length} custodian references are attached. The mounted API does
              not expose post-placement custodian membership changes.
            </div>
          )}
          <Field label="Reason for change" htmlFor="hold-reason">
            <Textarea
              id="hold-reason"
              required
              minLength={3}
              maxLength={2000}
              aria-invalid={Boolean(validation)}
              rows={3}
              value={draft.reason}
              onChange={(event) => setDraft({ ...draft, reason: event.target.value })}
            />
          </Field>
          {validation && (
            <p role="alert" className="text-sm text-destructive">
              {validation}
            </p>
          )}
          {isGovernanceConflict(mutation.error) && (
            <GovernanceWarning>
              The legal hold changed after you opened it. Reload the current version before saving.
            </GovernanceWarning>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            {isGovernanceConflict(mutation.error) && (
              <Button type="button" variant="outline" onClick={onReload}>
                <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
                Reload hold
              </Button>
            )}
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              {hold ? 'Save hold' : 'Place hold'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  children,
  htmlFor,
  label,
}: {
  children: React.ReactNode;
  htmlFor: string;
  label: string;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
    </div>
  );
}
