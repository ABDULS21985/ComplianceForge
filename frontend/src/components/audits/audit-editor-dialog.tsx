'use client';

import * as React from 'react';
import type { Audit, AuditCreateInput, AuditPatch } from '@/types/audit';
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
import { useAuditFrameworkOptions, useCreateAudit, useUpdateAudit } from '@/lib/api-hooks';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';

const optionalUuid = z.union([z.literal(''), z.string().uuid('Enter a valid UUID.')]);

const auditEditorSchema = z
  .object({
    title: z.string().trim().min(1, 'Title is required.').max(200),
    description: z.string().trim().min(1, 'Description is required.').max(10_000),
    audit_type: z.enum(['internal', 'external', 'certification']),
    lead_auditor_id: z.string().trim().uuid('Enter a valid lead auditor UUID.'),
    scope: z.string().trim().min(1, 'Scope is required.').max(20_000),
    scheduled_start_date: z.string().min(1, 'Start date is required.'),
    scheduled_end_date: z.string().min(1, 'End date is required.'),
    framework_id: optionalUuid,
  })
  .refine((values) => values.scheduled_end_date >= values.scheduled_start_date, {
    message: 'End date must be on or after the start date.',
    path: ['scheduled_end_date'],
  });

type AuditEditorValues = z.infer<typeof auditEditorSchema>;

function auditDefaults(audit: Audit | undefined, defaultLeadAuditorId: string): AuditEditorValues {
  return {
    title: audit?.title ?? '',
    description: audit?.description ?? '',
    audit_type: audit?.audit_type ?? 'internal',
    lead_auditor_id: audit?.lead_auditor_id ?? defaultLeadAuditorId,
    scope: audit?.scope ?? '',
    scheduled_start_date: toDateInputValue(audit?.scheduled_start_date),
    scheduled_end_date: toDateInputValue(audit?.scheduled_end_date),
    framework_id: audit?.framework_id ?? '',
  };
}

interface AuditEditorDialogProps {
  audit?: Audit;
  defaultLeadAuditorId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AuditEditorDialog({
  audit,
  defaultLeadAuditorId,
  open,
  onOpenChange,
}: AuditEditorDialogProps) {
  const createAudit = useCreateAudit();
  const updateAudit = useUpdateAudit(audit?.id ?? '');
  const frameworksQuery = useAuditFrameworkOptions(open);
  const form = useForm<AuditEditorValues>({
    resolver: zodResolver(auditEditorSchema),
    defaultValues: auditDefaults(audit, defaultLeadAuditorId),
  });
  const isEditing = Boolean(audit);
  const mutation = isEditing ? updateAudit : createAudit;
  const startDate = form.watch('scheduled_start_date');

  React.useEffect(() => {
    if (open) form.reset(auditDefaults(audit, defaultLeadAuditorId));
  }, [audit, defaultLeadAuditorId, form, open]);

  async function submit(values: AuditEditorValues) {
    try {
      if (audit) {
        const patch: AuditPatch = {
          title: values.title.trim(),
          description: values.description.trim(),
          audit_type: values.audit_type,
          lead_auditor_id: values.lead_auditor_id,
          scope: values.scope.trim(),
          scheduled_start_date: values.scheduled_start_date,
          scheduled_end_date: values.scheduled_end_date,
        };
        if (values.framework_id) patch.framework_id = values.framework_id;
        else if (audit.framework_id) patch.clear_framework = true;
        await updateAudit.mutateAsync(patch);
      } else {
        const input: AuditCreateInput = {
          title: values.title.trim(),
          description: values.description.trim(),
          audit_type: values.audit_type,
          lead_auditor_id: values.lead_auditor_id,
          scope: values.scope.trim(),
          scheduled_start_date: values.scheduled_start_date,
          scheduled_end_date: values.scheduled_end_date,
          ...(values.framework_id ? { framework_id: values.framework_id } : {}),
        };
        await createAudit.mutateAsync(input);
      }
      onOpenChange(false);
    } catch {
      // Mutation state renders the backend error without dismissing the form.
    }
  }

  const mutationError = mutation.error
    ? formatAuditError(mutation.error, `Failed to ${isEditing ? 'update' : 'create'} audit.`)
    : null;

  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit audit plan' : 'Plan a new audit'}</DialogTitle>
          <DialogDescription>
            Dates are stored in UTC. Required fields are marked with an asterisk.
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-4" onSubmit={form.handleSubmit(submit)} noValidate>
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
            error={form.formState.errors.title?.message}
            htmlFor="audit-title"
          >
            <Input
              id="audit-title"
              autoFocus
              aria-invalid={Boolean(form.formState.errors.title)}
              {...form.register('title')}
            />
          </Field>

          <Field
            label="Description"
            required
            error={form.formState.errors.description?.message}
            htmlFor="audit-description"
          >
            <Textarea
              id="audit-description"
              rows={3}
              aria-invalid={Boolean(form.formState.errors.description)}
              {...form.register('description')}
            />
          </Field>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="Audit type"
              required
              error={form.formState.errors.audit_type?.message}
              htmlFor="audit-type"
            >
              <Select
                value={form.watch('audit_type')}
                onValueChange={(value) =>
                  form.setValue('audit_type', value as AuditEditorValues['audit_type'], {
                    shouldValidate: true,
                  })
                }
              >
                <SelectTrigger
                  id="audit-type"
                  aria-invalid={Boolean(form.formState.errors.audit_type)}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="internal">Internal</SelectItem>
                  <SelectItem value="external">External</SelectItem>
                  <SelectItem value="certification">Certification</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field
              label="Lead auditor UUID"
              required
              error={form.formState.errors.lead_auditor_id?.message}
              htmlFor="audit-lead"
            >
              <Input
                id="audit-lead"
                aria-invalid={Boolean(form.formState.errors.lead_auditor_id)}
                {...form.register('lead_auditor_id')}
              />
              <p className="text-xs text-muted-foreground">
                Defaults to your user account. Paste another organization user ID to reassign.
              </p>
            </Field>
          </div>

          <Field
            label="Scope"
            required
            error={form.formState.errors.scope?.message}
            htmlFor="audit-scope"
          >
            <Textarea
              id="audit-scope"
              rows={4}
              aria-invalid={Boolean(form.formState.errors.scope)}
              {...form.register('scope')}
            />
          </Field>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="Scheduled start"
              required
              error={form.formState.errors.scheduled_start_date?.message}
              htmlFor="audit-start"
            >
              <Input
                id="audit-start"
                type="date"
                aria-invalid={Boolean(form.formState.errors.scheduled_start_date)}
                {...form.register('scheduled_start_date')}
              />
            </Field>
            <Field
              label="Scheduled end"
              required
              error={form.formState.errors.scheduled_end_date?.message}
              htmlFor="audit-end"
            >
              <Input
                id="audit-end"
                type="date"
                min={startDate || undefined}
                aria-invalid={Boolean(form.formState.errors.scheduled_end_date)}
                {...form.register('scheduled_end_date')}
              />
            </Field>
          </div>

          <Field
            label="Framework"
            error={form.formState.errors.framework_id?.message}
            htmlFor="audit-framework"
          >
            {frameworksQuery.isSuccess ? (
              <Select
                value={form.watch('framework_id') || 'none'}
                onValueChange={(value) =>
                  form.setValue('framework_id', value === 'none' ? '' : value, {
                    shouldValidate: true,
                  })
                }
              >
                <SelectTrigger id="audit-framework">
                  <SelectValue placeholder="No framework" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">No framework</SelectItem>
                  {audit?.framework &&
                    !frameworksQuery.data.items.some((item) => item.id === audit.framework?.id) && (
                      <SelectItem value={audit.framework.id}>
                        {audit.framework.code} — {audit.framework.name}
                      </SelectItem>
                    )}
                  {frameworksQuery.data.items.map((framework) => (
                    <SelectItem key={framework.id} value={framework.id}>
                      {framework.code} — {framework.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : frameworksQuery.isError ? (
              <>
                <Input
                  id="audit-framework"
                  placeholder="Optional framework UUID"
                  aria-invalid={Boolean(form.formState.errors.framework_id)}
                  {...form.register('framework_id')}
                />
                <p role="status" className="text-xs text-muted-foreground">
                  The framework catalog is unavailable. You can leave this blank or enter a known
                  framework UUID.
                </p>
              </>
            ) : (
              <Input id="audit-framework" disabled value="Loading framework catalog…" readOnly />
            )}
          </Field>

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
              {isEditing ? 'Save changes' : 'Plan audit'}
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
