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
  formatIncidentError,
  localDateTimeToIso,
  toLocalDateTimeInput,
} from '@/lib/incident';
import type { Incident, IncidentCreateInput, IncidentPatch } from '@/types/incident';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useCreateIncident, useUpdateIncident } from '@/lib/api-hooks';
import { useForm, useWatch } from 'react-hook-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';

const optionalUuid = z.union([z.literal(''), z.string().uuid('Enter a valid UUID.')]);

const incidentEditorSchema = z.object({
  title: z.string().trim().min(1, 'Title is required.').max(200),
  description: z.string().trim().min(1, 'Description is required.').max(20_000),
  category: z.string().trim().min(1, 'Category is required.').max(100),
  severity: z.enum(['critical', 'high', 'medium', 'low']),
  detected_at: z.string(),
  occurred_at: z.string(),
  related_asset_id: optionalUuid,
  followup_date: z.string(),
  retention_until: z.string(),
  root_cause: z.string().max(20_000),
  impact: z.string().max(20_000),
  lessons_learned: z.string().max(20_000),
  legal_hold: z.boolean(),
});

type IncidentEditorValues = z.infer<typeof incidentEditorSchema>;

function defaults(incident?: Incident): IncidentEditorValues {
  return {
    title: incident?.title ?? '',
    description: incident?.description ?? '',
    category: incident?.category ?? '',
    severity: incident?.severity ?? 'medium',
    detected_at: toLocalDateTimeInput(incident?.detected_at),
    occurred_at: toLocalDateTimeInput(incident?.occurred_at),
    related_asset_id: incident?.related_asset_id ?? '',
    followup_date: incident?.followup_date?.slice(0, 10) ?? '',
    retention_until: toLocalDateTimeInput(incident?.retention_until),
    root_cause: incident?.root_cause ?? '',
    impact: incident?.impact ?? '',
    lessons_learned: incident?.lessons_learned ?? '',
    legal_hold: incident?.legal_hold ?? false,
  };
}

interface IncidentEditorDialogProps {
  incident?: Incident;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (incident: Incident) => void;
}

export function IncidentEditorDialog({
  incident,
  open,
  onOpenChange,
  onCreated,
}: IncidentEditorDialogProps) {
  const createIncident = useCreateIncident();
  const updateIncident = useUpdateIncident(incident?.id ?? '');
  const form = useForm<IncidentEditorValues>({
    resolver: zodResolver(incidentEditorSchema),
    defaultValues: defaults(incident),
  });
  const severity = useWatch({ control: form.control, name: 'severity' });
  const isEditing = Boolean(incident);
  const mutation = isEditing ? updateIncident : createIncident;

  React.useEffect(() => {
    if (open) form.reset(defaults(incident));
  }, [form, incident, open]);

  async function submit(values: IncidentEditorValues) {
    try {
      if (incident) {
        const patch: IncidentPatch = {
          version: incident.version,
          title: values.title.trim(),
          description: values.description.trim(),
          category: values.category.trim(),
          root_cause: values.root_cause.trim(),
          impact: values.impact.trim(),
          lessons_learned: values.lessons_learned.trim(),
          legal_hold: values.legal_hold,
        };
        const detectedAt = localDateTimeToIso(values.detected_at);
        if (detectedAt) patch.detected_at = detectedAt;
        const occurredAt = localDateTimeToIso(values.occurred_at);
        if (occurredAt) patch.occurred_at = occurredAt;
        else if (incident.occurred_at) patch.clear_occurred_at = true;
        if (values.related_asset_id) patch.related_asset_id = values.related_asset_id;
        else if (incident.related_asset_id) patch.clear_related_asset_id = true;
        if (values.followup_date) patch.followup_date = values.followup_date;
        else if (incident.followup_date) patch.clear_followup_date = true;
        const retentionUntil = localDateTimeToIso(values.retention_until);
        if (retentionUntil) patch.retention_until = retentionUntil;
        await updateIncident.mutateAsync(patch);
      } else {
        const input: IncidentCreateInput = {
          title: values.title.trim(),
          description: values.description.trim(),
          category: values.category.trim(),
          severity: values.severity,
        };
        const detectedAt = localDateTimeToIso(values.detected_at);
        const occurredAt = localDateTimeToIso(values.occurred_at);
        const retentionUntil = localDateTimeToIso(values.retention_until);
        if (detectedAt) input.detected_at = detectedAt;
        if (occurredAt) input.occurred_at = occurredAt;
        if (values.related_asset_id) input.related_asset_id = values.related_asset_id;
        if (values.followup_date) input.followup_date = values.followup_date;
        if (retentionUntil) input.retention_until = retentionUntil;
        const created = await createIncident.mutateAsync(input);
        onCreated?.(created);
      }
      onOpenChange(false);
    } catch {
      // Keep the form open; mutation state renders the server error.
    }
  }

  const error = mutation.error
    ? formatIncidentError(mutation.error, `Failed to ${isEditing ? 'update' : 'report'} incident.`)
    : null;

  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit incident record' : 'Report an incident'}</DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Severity changes use the separately audited escalation action.'
              : 'Create the initial record, then complete the breach assessment when personal data may be involved.'}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={form.handleSubmit(submit)} noValidate>
          {error && (
            <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </p>
          )}
          <Field label="Title" htmlFor="incident-title" required error={form.formState.errors.title?.message}>
            <Input id="incident-title" autoFocus aria-invalid={Boolean(form.formState.errors.title)} {...form.register('title')} />
          </Field>
          <Field label="Description" htmlFor="incident-description" required error={form.formState.errors.description?.message}>
            <Textarea id="incident-description" rows={4} aria-invalid={Boolean(form.formState.errors.description)} {...form.register('description')} />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Category" htmlFor="incident-category" required error={form.formState.errors.category?.message}>
              <Input id="incident-category" placeholder="e.g. unauthorized access" aria-invalid={Boolean(form.formState.errors.category)} {...form.register('category')} />
            </Field>
            <Field label="Severity" htmlFor="incident-severity" required>
              <Select
                disabled={isEditing}
                value={severity}
                onValueChange={(value) => form.setValue('severity', value as IncidentEditorValues['severity'])}
              >
                <SelectTrigger id="incident-severity"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {(['low', 'medium', 'high', 'critical'] as const).map((value) => (
                    <SelectItem key={value} value={value}>{value[0].toUpperCase() + value.slice(1)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Detected at" htmlFor="incident-detected"><Input id="incident-detected" type="datetime-local" {...form.register('detected_at')} /></Field>
            <Field label="Occurred at" htmlFor="incident-occurred"><Input id="incident-occurred" type="datetime-local" {...form.register('occurred_at')} /></Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Related asset UUID" htmlFor="incident-asset" error={form.formState.errors.related_asset_id?.message}>
              <Input id="incident-asset" aria-invalid={Boolean(form.formState.errors.related_asset_id)} {...form.register('related_asset_id')} />
            </Field>
            <Field label="Follow-up date" htmlFor="incident-followup"><Input id="incident-followup" type="date" {...form.register('followup_date')} /></Field>
          </div>
          {isEditing && (
            <>
              <Field label="Impact" htmlFor="incident-impact"><Textarea id="incident-impact" rows={3} {...form.register('impact')} /></Field>
              <Field label="Root cause" htmlFor="incident-root-cause"><Textarea id="incident-root-cause" rows={3} {...form.register('root_cause')} /></Field>
              <Field label="Lessons learned" htmlFor="incident-lessons"><Textarea id="incident-lessons" rows={3} {...form.register('lessons_learned')} /></Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Retain until" htmlFor="incident-retention"><Input id="incident-retention" type="datetime-local" {...form.register('retention_until')} /></Field>
                <div className="flex items-center gap-2 pt-8">
                  <input id="incident-legal-hold" type="checkbox" className="h-4 w-4" {...form.register('legal_hold')} />
                  <Label htmlFor="incident-legal-hold">Legal hold</Label>
                </div>
              </div>
            </>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
              {isEditing ? 'Save incident' : 'Report incident'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, htmlFor, required, error, children }: { label: string; htmlFor: string; required?: boolean; error?: string; children: React.ReactNode }) {
  const errorId = `${htmlFor}-error`;
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}{required ? ' *' : ''}</Label>
      {React.isValidElement(children)
        ? React.cloneElement(children as React.ReactElement<{ 'aria-describedby'?: string }>, error ? { 'aria-describedby': errorId } : {})
        : children}
      {error && <p id={errorId} className="text-sm text-destructive">{error}</p>}
    </div>
  );
}
