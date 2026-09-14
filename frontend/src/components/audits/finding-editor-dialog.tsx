'use client';

import * as React from 'react';
import type { AuditFinding, AuditFindingCreateInput, AuditFindingPatch } from '@/types/audit';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { formatAuditError, toDateInputValue } from '@/lib/audit';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useCreateFinding, useUpdateAuditFinding } from '@/lib/api-hooks';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';

const optionalUuid = z.union([z.literal(''), z.string().uuid('Enter a valid UUID.')]);

const findingEditorSchema = z.object({
  title: z.string().trim().min(1, 'Title is required.').max(200),
  description: z.string().trim().min(1, 'Description is required.').max(20_000),
  severity: z.enum(['critical', 'high', 'medium', 'low', 'informational']),
  finding_type: z.string().trim().min(1, 'Finding type is required.').max(100),
  control_id: optionalUuid,
  root_cause: z.string().trim().max(20_000),
  recommendation: z.string().trim().min(1, 'Recommendation is required.').max(20_000),
  remediation_plan: z.string().trim().max(20_000),
  responsible_user_id: z.string().trim().uuid('Enter a valid responsible user UUID.'),
  due_date: z.string().min(1, 'Due date is required.'),
});

type FindingEditorValues = z.infer<typeof findingEditorSchema>;

function findingDefaults(
  finding: AuditFinding | undefined,
  defaultResponsibleUserId: string,
): FindingEditorValues {
  return {
    title: finding?.title ?? '',
    description: finding?.description ?? '',
    severity: finding?.severity ?? 'medium',
    finding_type: finding?.finding_type ?? '',
    control_id: finding?.control_id ?? '',
    root_cause: finding?.root_cause ?? '',
    recommendation: finding?.recommendation ?? '',
    remediation_plan: finding?.remediation_plan ?? '',
    responsible_user_id: finding?.responsible_user_id ?? defaultResponsibleUserId,
    due_date: toDateInputValue(finding?.due_date),
  };
}

interface FindingEditorDialogProps {
  auditId: string;
  finding?: AuditFinding;
  defaultResponsibleUserId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function FindingEditorDialog({
  auditId,
  finding,
  defaultResponsibleUserId,
  open,
  onOpenChange,
}: FindingEditorDialogProps) {
  const createFinding = useCreateFinding(auditId);
  const updateFinding = useUpdateAuditFinding(auditId);
  const form = useForm<FindingEditorValues>({
    resolver: zodResolver(findingEditorSchema),
    defaultValues: findingDefaults(finding, defaultResponsibleUserId),
  });
  const isEditing = Boolean(finding);
  const mutation = isEditing ? updateFinding : createFinding;

  React.useEffect(() => {
    if (open) form.reset(findingDefaults(finding, defaultResponsibleUserId));
  }, [defaultResponsibleUserId, finding, form, open]);

  async function submit(values: FindingEditorValues) {
    const common = {
      title: values.title.trim(),
      description: values.description.trim(),
      severity: values.severity,
      finding_type: values.finding_type.trim(),
      root_cause: values.root_cause.trim(),
      recommendation: values.recommendation.trim(),
      remediation_plan: values.remediation_plan.trim(),
      responsible_user_id: values.responsible_user_id,
      due_date: values.due_date,
    };

    try {
      if (finding) {
        const patch: AuditFindingPatch = { ...common };
        if (values.control_id) patch.control_id = values.control_id;
        else if (finding.control_id) patch.clear_control = true;
        await updateFinding.mutateAsync({ findingId: finding.id, data: patch });
      } else {
        const input: AuditFindingCreateInput = {
          ...common,
          ...(values.control_id ? { control_id: values.control_id } : {}),
        };
        await createFinding.mutateAsync(input);
      }
      onOpenChange(false);
    } catch {
      // Mutation state renders the backend error without dismissing the form.
    }
  }

  const mutationError = mutation.error
    ? formatAuditError(mutation.error, `Failed to ${isEditing ? 'update' : 'create'} finding.`)
    : null;

  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit finding' : 'Record a finding'}</DialogTitle>
          <DialogDescription>
            Record the observation, accountable owner, recommendation, and target date.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" noValidate onSubmit={form.handleSubmit(submit)}>
          {mutationError && (
            <div
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive"
            >
              {mutationError}
            </div>
          )}

          <Field
            label="Title"
            required
            htmlFor="finding-title"
            error={form.formState.errors.title?.message}
          >
            <Input
              id="finding-title"
              autoFocus
              aria-invalid={Boolean(form.formState.errors.title)}
              {...form.register('title')}
            />
          </Field>
          <Field
            label="Description"
            required
            htmlFor="finding-description"
            error={form.formState.errors.description?.message}
          >
            <Textarea
              id="finding-description"
              rows={4}
              aria-invalid={Boolean(form.formState.errors.description)}
              {...form.register('description')}
            />
          </Field>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="Severity"
              required
              htmlFor="finding-severity"
              error={form.formState.errors.severity?.message}
            >
              <Select
                value={form.watch('severity')}
                onValueChange={(value) =>
                  form.setValue('severity', value as FindingEditorValues['severity'], {
                    shouldValidate: true,
                  })
                }
              >
                <SelectTrigger
                  id="finding-severity"
                  aria-invalid={Boolean(form.formState.errors.severity)}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="critical">Critical</SelectItem>
                  <SelectItem value="high">High</SelectItem>
                  <SelectItem value="medium">Medium</SelectItem>
                  <SelectItem value="low">Low</SelectItem>
                  <SelectItem value="informational">Informational</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field
              label="Finding type"
              required
              htmlFor="finding-type"
              error={form.formState.errors.finding_type?.message}
            >
              <Input
                id="finding-type"
                placeholder="Non-conformity, observation…"
                aria-invalid={Boolean(form.formState.errors.finding_type)}
                {...form.register('finding_type')}
              />
            </Field>
          </div>

          <Field
            label="Control UUID"
            htmlFor="finding-control"
            error={form.formState.errors.control_id?.message}
          >
            <Input
              id="finding-control"
              placeholder="Optional"
              aria-invalid={Boolean(form.formState.errors.control_id)}
              {...form.register('control_id')}
            />
          </Field>
          <Field
            label="Root cause"
            htmlFor="finding-root-cause"
            error={form.formState.errors.root_cause?.message}
          >
            <Textarea id="finding-root-cause" rows={3} {...form.register('root_cause')} />
          </Field>
          <Field
            label="Recommendation"
            required
            htmlFor="finding-recommendation"
            error={form.formState.errors.recommendation?.message}
          >
            <Textarea
              id="finding-recommendation"
              rows={3}
              aria-invalid={Boolean(form.formState.errors.recommendation)}
              {...form.register('recommendation')}
            />
          </Field>
          <Field
            label="Remediation plan"
            htmlFor="finding-remediation"
            error={form.formState.errors.remediation_plan?.message}
          >
            <Textarea id="finding-remediation" rows={3} {...form.register('remediation_plan')} />
          </Field>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="Responsible user UUID"
              required
              htmlFor="finding-owner"
              error={form.formState.errors.responsible_user_id?.message}
            >
              <Input
                id="finding-owner"
                aria-invalid={Boolean(form.formState.errors.responsible_user_id)}
                {...form.register('responsible_user_id')}
              />
              <p className="text-xs text-muted-foreground">
                Defaults to your user account. Paste another organization user ID to reassign.
              </p>
            </Field>
            <Field
              label="Due date"
              required
              htmlFor="finding-due-date"
              error={form.formState.errors.due_date?.message}
            >
              <Input
                id="finding-due-date"
                type="date"
                aria-invalid={Boolean(form.formState.errors.due_date)}
                {...form.register('due_date')}
              />
            </Field>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              {isEditing ? 'Save changes' : 'Add finding'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  children,
  error,
  htmlFor,
  label,
  required,
}: {
  children: React.ReactNode;
  error?: string;
  htmlFor: string;
  label: string;
  required?: boolean;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>
        {label}
        {required ? ' *' : ''}
      </Label>
      {children}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
