'use client';

import * as React from 'react';
import {
  DATA_CLASSIFICATIONS,
  formatGovernanceError,
  GOVERNANCE_RECORD_TYPES,
  humanizeGovernanceToken,
  isGovernanceConflict,
  localDateTimeInput,
  RETENTION_TRIGGERS,
  toISOStringOrUndefined,
} from '@/lib/data-governance';
import type {
  DataClassification,
  DispositionAction,
  GovernanceRecordType,
  RetentionSchedule,
  RetentionScheduleStatus,
  RetentionTrigger,
} from '@/types/data-governance';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Loader2, RefreshCw } from 'lucide-react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { GovernanceWarning } from './governance-states';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useMutation } from '@tanstack/react-query';

interface ScheduleDraft {
  archiveAfterDays: string;
  classification: DataClassification | 'none';
  description: string;
  dispositionAction: DispositionAction;
  effectiveFrom: string;
  effectiveUntil: string;
  jurisdiction: string;
  legalBasis: string;
  name: string;
  priority: string;
  reason: string;
  recordType: GovernanceRecordType;
  retentionDays: string;
  reviewRequired: boolean;
  status: RetentionScheduleStatus;
  triggerEvent: RetentionTrigger;
}

function initialDraft(schedule?: RetentionSchedule): ScheduleDraft {
  return {
    archiveAfterDays: schedule?.archive_after_days?.toString() ?? '',
    classification: schedule?.data_classification ?? 'none',
    description: schedule?.description ?? '',
    dispositionAction: schedule?.disposition_action ?? 'review',
    effectiveFrom: localDateTimeInput(schedule?.effective_from ?? new Date().toISOString()),
    effectiveUntil: localDateTimeInput(schedule?.effective_until),
    jurisdiction: schedule?.jurisdiction ?? '',
    legalBasis: schedule?.legal_basis ?? '',
    name: schedule?.name ?? '',
    priority: schedule?.priority.toString() ?? '100',
    reason: '',
    recordType: schedule?.record_type ?? 'asset',
    retentionDays: schedule?.retention_days.toString() ?? '365',
    reviewRequired: schedule?.review_required ?? true,
    status: schedule?.status === 'retired' ? 'retired' : (schedule?.status ?? 'draft'),
    triggerEvent: schedule?.trigger_event ?? 'record_created',
  };
}

export function ScheduleEditorDialog({
  onOpenChange,
  onReload,
  onSaved,
  open,
  schedule,
}: {
  onOpenChange: (open: boolean) => void;
  onReload: () => void;
  onSaved: (schedule: RetentionSchedule) => void;
  open: boolean;
  schedule?: RetentionSchedule;
}) {
  const [draft, setDraft] = React.useState<ScheduleDraft>(() => initialDraft(schedule));
  const [validation, setValidation] = React.useState('');

  const mutation = useMutation({
    mutationFn: () => {
      const retentionDays = Number(draft.retentionDays);
      const archiveDays = draft.archiveAfterDays ? Number(draft.archiveAfterDays) : undefined;
      const priority = Number(draft.priority);
      const effectiveFrom = toISOStringOrUndefined(draft.effectiveFrom);
      const effectiveUntil = toISOStringOrUndefined(draft.effectiveUntil);
      const reason = draft.reason.trim();
      if (draft.name.trim().length < 3)
        throw new Error('Schedule name must be at least 3 characters.');
      if (draft.legalBasis.trim().length < 3)
        throw new Error('Legal basis must be at least 3 characters.');
      if (!Number.isInteger(retentionDays) || retentionDays < 1 || retentionDays > 36500)
        throw new Error('Retention must be between 1 and 36,500 days.');
      if (
        archiveDays !== undefined &&
        (!Number.isInteger(archiveDays) || archiveDays < 1 || archiveDays >= retentionDays)
      )
        throw new Error('Archive timing must be earlier than retention expiry.');
      if (!Number.isInteger(priority) || priority < 1 || priority > 10000)
        throw new Error('Priority must be between 1 and 10,000.');
      if (!effectiveFrom || (draft.effectiveUntil && !effectiveUntil))
        throw new Error('Enter valid effective dates.');
      if (effectiveUntil && new Date(effectiveUntil) < new Date(effectiveFrom))
        throw new Error('Effective-until must not be before effective-from.');
      if (reason.length < 3)
        throw new Error('Give a reason of at least 3 characters for the audit history.');
      const common = {
        name: draft.name.trim(),
        description: draft.description.trim(),
        trigger_event: draft.triggerEvent,
        legal_basis: draft.legalBasis.trim(),
        retention_days: retentionDays,
        disposition_action: draft.dispositionAction,
        review_required: draft.reviewRequired,
        priority,
        status: draft.status,
        effective_from: effectiveFrom,
        reason,
      };
      if (schedule) {
        return api.dataGovernance.updateSchedule(schedule.id, {
          ...common,
          expected_version: schedule.version,
          data_classification: draft.classification === 'none' ? undefined : draft.classification,
          clear_data_classification: draft.classification === 'none',
          jurisdiction: draft.jurisdiction.trim().toUpperCase() || undefined,
          clear_jurisdiction: !draft.jurisdiction.trim(),
          archive_after_days: archiveDays,
          clear_archive_after_days: archiveDays === undefined,
          effective_until: effectiveUntil,
          clear_effective_until: effectiveUntil === undefined,
        });
      }
      return api.dataGovernance.createSchedule({
        ...common,
        record_type: draft.recordType,
        data_classification: draft.classification === 'none' ? undefined : draft.classification,
        jurisdiction: draft.jurisdiction.trim().toUpperCase() || undefined,
        archive_after_days: archiveDays,
        effective_until: effectiveUntil,
      });
    },
    onSuccess: (saved) => {
      onSaved(saved);
      onOpenChange(false);
    },
    onError: (error) =>
      setValidation(formatGovernanceError(error, 'The retention schedule could not be saved.')),
  });

  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setValidation('');
            mutation.mutate();
          }}
          className="space-y-5"
        >
          <DialogHeader>
            <DialogTitle>
              {schedule ? 'Edit retention schedule' : 'Create retention schedule'}
            </DialogTitle>
            <DialogDescription>
              Define record scope, legal basis, timing, and controlled disposition. Every save is
              audited.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Name" htmlFor="schedule-name">
              <Input
                id="schedule-name"
                required
                minLength={3}
                maxLength={200}
                value={draft.name}
                onChange={(event) => setDraft({ ...draft, name: event.target.value })}
              />
            </Field>
            <Field label="Record type" htmlFor="schedule-record-type">
              <Select
                disabled={Boolean(schedule)}
                value={draft.recordType}
                onValueChange={(value) =>
                  setDraft({ ...draft, recordType: value as GovernanceRecordType })
                }
              >
                <SelectTrigger id="schedule-record-type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {GOVERNANCE_RECORD_TYPES.map((value) => (
                    <SelectItem key={value} value={value}>
                      {humanizeGovernanceToken(value)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {schedule && (
                <p className="text-xs text-muted-foreground">
                  Record scope is immutable after creation.
                </p>
              )}
            </Field>
            <Field label="Classification" htmlFor="schedule-classification">
              <Select
                value={draft.classification}
                onValueChange={(value) =>
                  setDraft({ ...draft, classification: value as ScheduleDraft['classification'] })
                }
              >
                <SelectTrigger id="schedule-classification">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">Any classification</SelectItem>
                  {DATA_CLASSIFICATIONS.map((value) => (
                    <SelectItem key={value} value={value}>
                      {humanizeGovernanceToken(value)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Jurisdiction (optional)" htmlFor="schedule-jurisdiction">
              <Input
                id="schedule-jurisdiction"
                maxLength={16}
                placeholder="EU"
                value={draft.jurisdiction}
                onChange={(event) =>
                  setDraft({ ...draft, jurisdiction: event.target.value.toUpperCase() })
                }
              />
            </Field>
            <Field label="Trigger event" htmlFor="schedule-trigger">
              <Select
                value={draft.triggerEvent}
                onValueChange={(value) =>
                  setDraft({ ...draft, triggerEvent: value as RetentionTrigger })
                }
              >
                <SelectTrigger id="schedule-trigger">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {RETENTION_TRIGGERS.map((value) => (
                    <SelectItem key={value} value={value}>
                      {humanizeGovernanceToken(value)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Disposition action" htmlFor="schedule-action">
              <Select
                value={draft.dispositionAction}
                onValueChange={(value) =>
                  setDraft({ ...draft, dispositionAction: value as DispositionAction })
                }
              >
                <SelectTrigger id="schedule-action">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(['review', 'delete', 'anonymize', 'archive'] as const).map((value) => (
                    <SelectItem key={value} value={value}>
                      {humanizeGovernanceToken(value)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Retention (days)" htmlFor="schedule-retention">
              <Input
                id="schedule-retention"
                type="number"
                min={1}
                max={36500}
                required
                value={draft.retentionDays}
                onChange={(event) => setDraft({ ...draft, retentionDays: event.target.value })}
              />
            </Field>
            <Field label="Archive after (days)" htmlFor="schedule-archive">
              <Input
                id="schedule-archive"
                type="number"
                min={1}
                value={draft.archiveAfterDays}
                onChange={(event) => setDraft({ ...draft, archiveAfterDays: event.target.value })}
              />
            </Field>
            <Field label="Priority" htmlFor="schedule-priority">
              <Input
                id="schedule-priority"
                type="number"
                min={1}
                max={10000}
                required
                value={draft.priority}
                onChange={(event) => setDraft({ ...draft, priority: event.target.value })}
              />
            </Field>
            <Field label="Status" htmlFor="schedule-status">
              <Select
                value={draft.status}
                onValueChange={(value) =>
                  setDraft({ ...draft, status: value as RetentionScheduleStatus })
                }
              >
                <SelectTrigger id="schedule-status">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="draft">Draft</SelectItem>
                  <SelectItem value="active">Active</SelectItem>
                  {schedule?.status === 'retired' && (
                    <SelectItem value="retired">Retired</SelectItem>
                  )}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Effective from" htmlFor="schedule-from">
              <Input
                id="schedule-from"
                type="datetime-local"
                required
                value={draft.effectiveFrom}
                onChange={(event) => setDraft({ ...draft, effectiveFrom: event.target.value })}
              />
            </Field>
            <Field label="Effective until (optional)" htmlFor="schedule-until">
              <Input
                id="schedule-until"
                type="datetime-local"
                value={draft.effectiveUntil}
                onChange={(event) => setDraft({ ...draft, effectiveUntil: event.target.value })}
              />
            </Field>
          </div>
          <Field label="Description" htmlFor="schedule-description">
            <Textarea
              id="schedule-description"
              maxLength={20000}
              rows={3}
              value={draft.description}
              onChange={(event) => setDraft({ ...draft, description: event.target.value })}
            />
          </Field>
          <Field label="Legal basis" htmlFor="schedule-basis">
            <Textarea
              id="schedule-basis"
              required
              minLength={3}
              maxLength={4000}
              rows={3}
              value={draft.legalBasis}
              onChange={(event) => setDraft({ ...draft, legalBasis: event.target.value })}
            />
          </Field>
          <div className="flex items-start justify-between gap-4 rounded-md border p-4">
            <div>
              <Label htmlFor="schedule-review">Manual review required</Label>
              <p className="mt-1 text-xs text-muted-foreground">
                Require an explicit approve or reject decision before disposition.
              </p>
            </div>
            <Switch
              id="schedule-review"
              checked={draft.reviewRequired}
              onCheckedChange={(checked) => setDraft({ ...draft, reviewRequired: checked })}
            />
          </div>
          <Field label="Reason for change" htmlFor="schedule-reason">
            <Textarea
              id="schedule-reason"
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
              The schedule changed after you opened it. Reload the latest version before saving.
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
                Reload schedule
              </Button>
            )}
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              {schedule ? 'Save schedule' : 'Create schedule'}
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
