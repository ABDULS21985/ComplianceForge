'use client';

import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ChevronLeft, ChevronRight, FileText, Loader2, Pencil, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';

import api from '@/lib/api';
import { formatApiError, isNotificationToken, parseCommaList } from '@/lib/enterprise-settings';
import type { NotificationTemplate, NotificationTemplateInput } from '@/types/enterprise-settings';
import { ConfirmAction } from '@/components/settings/confirm-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Textarea } from '@/components/ui/textarea';

const PAGE_SIZE = 20;

interface TemplateDraft extends NotificationTemplateInput {
  id?: string;
  variablesText: string;
}

const EMPTY_TEMPLATE: TemplateDraft = {
  name: '',
  event_type: '',
  subject_template: '',
  body_text_template: '',
  body_html_template: '',
  variables: [],
  variablesText: '',
};

function validateTemplate(draft: TemplateDraft): string {
  if (!draft.name.trim()) return 'Name is required.';
  if (draft.name.trim().length > 200) return 'Name must not exceed 200 characters.';
  if (!isNotificationToken(draft.event_type.trim()) || draft.event_type.trim() === '*') {
    return 'Event type must be a lowercase event token such as policy.published.';
  }
  if (!draft.subject_template.trim()) return 'Subject template is required.';
  if (/\r|\n/.test(draft.subject_template)) return 'Subject template cannot contain line breaks.';
  if (!draft.body_text_template.trim() && !draft.body_html_template.trim()) {
    return 'Enter a text or HTML body template.';
  }
  const variables = parseCommaList(draft.variablesText);
  if (variables.some((variable) => !isNotificationToken(variable) || variable === '*')) {
    return 'Variables must be comma-separated lowercase tokens.';
  }
  return '';
}

function toInput(draft: TemplateDraft): NotificationTemplateInput {
  return {
    name: draft.name.trim(),
    event_type: draft.event_type.trim().toLowerCase(),
    subject_template: draft.subject_template,
    body_text_template: draft.body_text_template,
    body_html_template: draft.body_html_template,
    variables: parseCommaList(draft.variablesText).map((item) => item.toLowerCase()),
  };
}

export function NotificationTemplatesPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [editorOpen, setEditorOpen] = useState(false);
  const [draft, setDraft] = useState<TemplateDraft>(EMPTY_TEMPLATE);
  const [validationError, setValidationError] = useState('');

  const templatesQuery = useQuery({
    queryKey: ['notification-admin', 'templates', page, PAGE_SIZE],
    queryFn: () => api.notificationAdmin.listTemplates({ page, page_size: PAGE_SIZE }),
    staleTime: 30_000,
  });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['notification-admin', 'templates'] });
  const saveMutation = useMutation({
    mutationFn: ({ id, input }: { id?: string; input: NotificationTemplateInput }) =>
      id ? api.notificationAdmin.updateTemplate(id, input) : api.notificationAdmin.createTemplate(input),
    onSuccess: (_, variables) => {
      void invalidate();
      setEditorOpen(false);
      toast.success(variables.id ? 'Notification template updated' : 'Notification template created');
    },
    onError: (error) => toast.error(formatApiError(error, 'The notification template could not be saved.')),
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.notificationAdmin.deleteTemplate(id),
    onSuccess: () => {
      void invalidate();
      toast.success('Notification template deleted');
    },
    onError: (error) => toast.error(formatApiError(error, 'The notification template could not be deleted.')),
  });

  function edit(template?: NotificationTemplate) {
    setValidationError('');
    setDraft(template ? {
      id: template.id,
      name: template.name,
      event_type: template.event_type,
      subject_template: template.subject_template,
      body_text_template: template.body_text_template,
      body_html_template: template.body_html_template,
      variables: template.variables,
      variablesText: template.variables.join(', '),
    } : EMPTY_TEMPLATE);
    setEditorOpen(true);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const error = validateTemplate(draft);
    setValidationError(error);
    if (error) return;
    saveMutation.mutate({ id: draft.id, input: toInput(draft) });
  }

  const pagination = templatesQuery.data?.pagination;

  return (
    <section aria-labelledby="notification-templates-heading" className="space-y-4">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
        <div>
          <h2 id="notification-templates-heading" className="text-xl font-semibold">Message templates</h2>
          <p className="text-sm text-muted-foreground">System templates are visible but immutable; tenant templates can be managed here.</p>
        </div>
        {canConfigure && <Button type="button" size="sm" onClick={() => edit()}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Add template</Button>}
      </div>

      {templatesQuery.isLoading && <div role="status" aria-label="Loading templates" className="space-y-3">{[0, 1, 2].map((item) => <Skeleton key={item} className="h-28 w-full" />)}</div>}
      {templatesQuery.isError && <Card><CardContent className="space-y-3 py-8 text-center"><p role="alert">{formatApiError(templatesQuery.error, 'Notification templates could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void templatesQuery.refetch()}>Try again</Button></CardContent></Card>}
      {templatesQuery.isSuccess && templatesQuery.data.data.length === 0 && <Card><CardContent className="py-10 text-center text-sm text-muted-foreground">No notification templates are available.</CardContent></Card>}

      <div className="grid gap-3">
        {templatesQuery.data?.data.map((template) => (
          <Card key={template.id}>
            <CardContent className="flex flex-col justify-between gap-4 py-4 md:flex-row md:items-center">
              <div className="flex min-w-0 gap-3">
                <FileText aria-hidden="true" className="mt-1 h-5 w-5 shrink-0 text-muted-foreground" />
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2"><h3 className="font-medium">{template.name}</h3><Badge variant="outline">{template.event_type}</Badge>{template.is_system && <Badge variant="secondary">System</Badge>}</div>
                  <p className="mt-1 truncate text-sm text-muted-foreground">Subject: {template.subject_template}</p>
                  {template.variables.length > 0 && <p className="mt-1 text-xs text-muted-foreground">Variables: {template.variables.join(', ')}</p>}
                </div>
              </div>
              {canConfigure && !template.is_system && (
                <div className="flex gap-2">
                  <Button type="button" size="sm" variant="outline" onClick={() => edit(template)}><Pencil aria-hidden="true" className="mr-2 h-4 w-4" />Edit</Button>
                  <ConfirmAction
                    title={`Delete ${template.name}?`}
                    description="This permanently deletes the tenant template. Templates referenced by active rules cannot be deleted."
                    actionLabel="Delete template"
                    onConfirm={() => deleteMutation.mutate(template.id)}
                    pending={deleteMutation.isPending && deleteMutation.variables === template.id}
                    triggerVariant="destructive"
                  ><Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />Delete</ConfirmAction>
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      {pagination && pagination.total_pages > 1 && (
        <nav aria-label="Template pages" className="flex items-center justify-between gap-3">
          <p className="text-sm text-muted-foreground">Page {pagination.page} of {pagination.total_pages}</p>
          <div className="flex gap-2">
            <Button type="button" size="sm" variant="outline" disabled={page === 1} onClick={() => setPage((value) => Math.max(1, value - 1))}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button>
            <Button type="button" size="sm" variant="outline" disabled={page >= pagination.total_pages} onClick={() => setPage((value) => value + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button>
          </div>
        </nav>
      )}

      <Dialog open={editorOpen} onOpenChange={(open) => !saveMutation.isPending && setEditorOpen(open)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <form onSubmit={submit} className="space-y-4">
            <DialogHeader><DialogTitle>{draft.id ? 'Edit notification template' : 'Add notification template'}</DialogTitle><DialogDescription>Templates use Go template expressions. Server validation rejects invalid expressions and unsafe HTML elements or URLs.</DialogDescription></DialogHeader>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="template-name">Name</Label><Input id="template-name" required maxLength={200} value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} /></div>
              <div className="space-y-2"><Label htmlFor="template-event">Event type</Label><Input id="template-event" required placeholder="policy.published" value={draft.event_type} onChange={(event) => setDraft((current) => ({ ...current, event_type: event.target.value.toLowerCase() }))} /></div>
            </div>
            <div className="space-y-2"><Label htmlFor="template-subject">Subject template</Label><Input id="template-subject" required value={draft.subject_template} onChange={(event) => setDraft((current) => ({ ...current, subject_template: event.target.value }))} /></div>
            <div className="space-y-2"><Label htmlFor="template-variables">Declared variables</Label><Input id="template-variables" placeholder="policy_name, owner_name" value={draft.variablesText} onChange={(event) => setDraft((current) => ({ ...current, variablesText: event.target.value }))} /><p className="text-xs text-muted-foreground">Comma-separated lowercase tokens exposed by the event.</p></div>
            <div className="space-y-2"><Label htmlFor="template-text">Plain-text body</Label><Textarea id="template-text" rows={7} value={draft.body_text_template} onChange={(event) => setDraft((current) => ({ ...current, body_text_template: event.target.value }))} /></div>
            <div className="space-y-2"><Label htmlFor="template-html">HTML body</Label><Textarea id="template-html" rows={7} className="font-mono text-xs" value={draft.body_html_template} onChange={(event) => setDraft((current) => ({ ...current, body_html_template: event.target.value }))} /></div>
            {validationError && <p role="alert" className="text-sm text-destructive">{validationError}</p>}
            <DialogFooter><Button type="button" variant="outline" onClick={() => setEditorOpen(false)} disabled={saveMutation.isPending}>Cancel</Button><Button type="submit" disabled={saveMutation.isPending}>{saveMutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Save template</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  );
}
