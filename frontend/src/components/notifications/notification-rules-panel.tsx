'use client';

import { Card, CardContent } from '@/components/ui/card';
import { ChevronLeft, ChevronRight, Loader2, Pencil, Plus, PowerOff } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  formatApiError,
  isNotificationToken,
  isUuid,
  parseCommaList,
  parseJSONObject,
} from '@/lib/enterprise-settings';
import { type FormEvent, useState } from 'react';
import type {
  NotificationRecipientType,
  NotificationRule,
  NotificationRuleInput,
  NotificationSeverity,
} from '@/types/enterprise-settings';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmAction } from '@/components/settings/confirm-action';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

const PAGE_SIZE = 20;
const SEVERITIES: readonly NotificationSeverity[] = ['low', 'medium', 'high', 'critical'];
const ID_RECIPIENT_TYPES: readonly NotificationRecipientType[] = ['user', 'custom', 'role'];

interface RuleDraft {
  id?: string;
  name: string;
  eventType: string;
  severities: NotificationSeverity[];
  conditionsText: string;
  channelIds: string[];
  recipientType: NotificationRecipientType;
  recipientIdsText: string;
  templateId: string;
  active: boolean;
  cooldownMinutes: string;
}

const EMPTY_RULE: RuleDraft = {
  name: '',
  eventType: '*',
  severities: [],
  conditionsText: '{}',
  channelIds: [],
  recipientType: 'owner',
  recipientIdsText: '',
  templateId: '',
  active: true,
  cooldownMinutes: '0',
};

function validateRule(draft: RuleDraft): string {
  if (!draft.name.trim()) return 'Name is required.';
  if (draft.name.trim().length > 200) return 'Name must not exceed 200 characters.';
  if (!isNotificationToken(draft.eventType.trim())) return 'Event type must be * or a lowercase event token.';
  if (draft.channelIds.length === 0) return 'Select at least one active delivery channel.';
  const cooldown = Number(draft.cooldownMinutes);
  if (!Number.isInteger(cooldown) || cooldown < 0 || cooldown > 525_600) return 'Cooldown must be a whole number between 0 and 525600.';
  try {
    const conditions = parseJSONObject(draft.conditionsText, 'Conditions');
    if (Object.keys(conditions).length > 25) return 'Conditions can contain at most 25 top-level entries.';
    if (Object.keys(conditions).some((key) => !isNotificationToken(key) || key === '*')) return 'Condition keys must be lowercase event tokens.';
  } catch (error) {
    return error instanceof Error ? error.message : 'Conditions must be valid JSON.';
  }
  const recipientIds = parseCommaList(draft.recipientIdsText);
  if (ID_RECIPIENT_TYPES.includes(draft.recipientType)) {
    if (recipientIds.length === 0) return 'Enter at least one recipient UUID for this recipient type.';
    if (recipientIds.some((id) => !isUuid(id))) return 'Every recipient ID must be a valid UUID.';
  } else if (recipientIds.length > 0) {
    return `${draft.recipientType} recipients are resolved from the event and cannot include IDs.`;
  }
  return '';
}

function toInput(draft: RuleDraft): NotificationRuleInput {
  return {
    name: draft.name.trim(),
    event_type: draft.eventType.trim().toLowerCase(),
    severity_filter: draft.severities,
    conditions: parseJSONObject(draft.conditionsText, 'Conditions'),
    channel_ids: draft.channelIds,
    recipient_type: draft.recipientType,
    recipient_ids: ID_RECIPIENT_TYPES.includes(draft.recipientType) ? parseCommaList(draft.recipientIdsText) : [],
    template_id: draft.templateId || null,
    is_active: draft.active,
    cooldown_minutes: Number(draft.cooldownMinutes),
  };
}

export function NotificationRulesPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [editorOpen, setEditorOpen] = useState(false);
  const [draft, setDraft] = useState<RuleDraft>(EMPTY_RULE);
  const [validationError, setValidationError] = useState('');

  const rulesQuery = useQuery({
    queryKey: ['notification-admin', 'rules', page, PAGE_SIZE],
    queryFn: () => api.notificationAdmin.listRules({ page, page_size: PAGE_SIZE }),
    staleTime: 30_000,
  });
  const channelsQuery = useQuery({
    queryKey: ['notification-admin', 'channels'],
    queryFn: () => api.notificationAdmin.listChannels(),
    staleTime: 30_000,
  });
  const templatesQuery = useQuery({
    queryKey: ['notification-admin', 'templates', 'selector'],
    queryFn: async () => {
      const first = await api.notificationAdmin.listTemplates({ page: 1, page_size: 100 });
      if (first.pagination.total_pages <= 1) return first;
      const remaining = await Promise.all(
        Array.from({ length: first.pagination.total_pages - 1 }, (_, index) =>
          api.notificationAdmin.listTemplates({ page: index + 2, page_size: 100 })
        )
      );
      return { ...first, data: [first.data, ...remaining.map((response) => response.data)].flat() };
    },
    staleTime: 30_000,
  });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['notification-admin', 'rules'] });
  const saveMutation = useMutation({
    mutationFn: ({ id, input }: { id?: string; input: NotificationRuleInput }) =>
      id ? api.notificationAdmin.updateRule(id, input) : api.notificationAdmin.createRule(input),
    onSuccess: (_, variables) => {
      void invalidate();
      setEditorOpen(false);
      toast.success(variables.id ? 'Notification rule updated' : 'Notification rule created');
    },
    onError: (error) => toast.error(formatApiError(error, 'The notification rule could not be saved.')),
  });
  const disableMutation = useMutation({
    mutationFn: (id: string) => api.notificationAdmin.deleteRule(id),
    onSuccess: () => {
      void invalidate();
      toast.success('Notification rule disabled');
    },
    onError: (error) => toast.error(formatApiError(error, 'The notification rule could not be disabled.')),
  });

  function edit(rule?: NotificationRule) {
    setValidationError('');
    setDraft(rule ? {
      id: rule.id,
      name: rule.name,
      eventType: rule.event_type,
      severities: rule.severity_filter,
      conditionsText: JSON.stringify(rule.conditions ?? {}, null, 2),
      channelIds: rule.channel_ids,
      recipientType: rule.recipient_type,
      recipientIdsText: rule.recipient_ids.join(', '),
      templateId: rule.template_id ?? '',
      active: rule.is_active,
      cooldownMinutes: String(rule.cooldown_minutes),
    } : EMPTY_RULE);
    setEditorOpen(true);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const error = validateRule(draft);
    setValidationError(error);
    if (error) return;
    saveMutation.mutate({ id: draft.id, input: toInput(draft) });
  }

  const pagination = rulesQuery.data?.pagination;
  const channels = channelsQuery.data?.data ?? [];
  const templates = templatesQuery.data?.data ?? [];

  return (
    <section aria-labelledby="notification-rules-heading" className="space-y-4">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
        <div><h2 id="notification-rules-heading" className="text-xl font-semibold">Routing rules</h2><p className="text-sm text-muted-foreground">Match events, resolve recipients, choose channels, and apply cooldowns.</p></div>
        {canConfigure && <Button type="button" size="sm" onClick={() => edit()} disabled={channels.filter((channel) => channel.is_active).length === 0}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Add rule</Button>}
      </div>
      {canConfigure && channelsQuery.isSuccess && channels.filter((channel) => channel.is_active).length === 0 && <p role="status" className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200">Create an active delivery channel before adding a rule.</p>}

      {rulesQuery.isLoading && <div role="status" aria-label="Loading rules" className="space-y-3">{[0, 1, 2].map((item) => <Skeleton key={item} className="h-28 w-full" />)}</div>}
      {rulesQuery.isError && <Card><CardContent className="space-y-3 py-8 text-center"><p role="alert">{formatApiError(rulesQuery.error, 'Notification rules could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void rulesQuery.refetch()}>Try again</Button></CardContent></Card>}
      {rulesQuery.isSuccess && rulesQuery.data.data.length === 0 && <Card><CardContent className="py-10 text-center text-sm text-muted-foreground">No tenant routing rules are configured.</CardContent></Card>}

      <div className="grid gap-3">
        {rulesQuery.data?.data.map((rule) => (
          <Card key={rule.id}>
            <CardContent className="flex flex-col justify-between gap-4 py-4 md:flex-row md:items-center">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2"><h3 className="font-medium">{rule.name}</h3><Badge variant="outline">{rule.event_type}</Badge><Badge variant={rule.is_active ? 'default' : 'secondary'}>{rule.is_active ? 'Active' : 'Inactive'}</Badge></div>
                <p className="mt-1 text-sm text-muted-foreground">{rule.recipient_type} recipients · {rule.channel_ids.length} channel{rule.channel_ids.length === 1 ? '' : 's'} · {rule.cooldown_minutes} minute cooldown</p>
                {rule.severity_filter.length > 0 && <p className="mt-1 text-xs text-muted-foreground">Severity: {rule.severity_filter.join(', ')}</p>}
              </div>
              {canConfigure && <div className="flex gap-2"><Button type="button" size="sm" variant="outline" onClick={() => edit(rule)}><Pencil aria-hidden="true" className="mr-2 h-4 w-4" />Edit</Button>{rule.is_active && <ConfirmAction title={`Disable ${rule.name}?`} description="The rule remains in audit history but stops routing new notifications. You can reactivate it by editing the rule." actionLabel="Disable rule" onConfirm={() => disableMutation.mutate(rule.id)} pending={disableMutation.isPending && disableMutation.variables === rule.id} triggerVariant="destructive"><PowerOff aria-hidden="true" className="mr-2 h-4 w-4" />Disable</ConfirmAction>}</div>}
            </CardContent>
          </Card>
        ))}
      </div>

      {pagination && pagination.total_pages > 1 && <nav aria-label="Rule pages" className="flex items-center justify-between gap-3"><p className="text-sm text-muted-foreground">Page {pagination.page} of {pagination.total_pages}</p><div className="flex gap-2"><Button type="button" size="sm" variant="outline" disabled={page === 1} onClick={() => setPage((value) => Math.max(1, value - 1))}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button><Button type="button" size="sm" variant="outline" disabled={page >= pagination.total_pages} onClick={() => setPage((value) => value + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button></div></nav>}

      <Dialog open={editorOpen} onOpenChange={(open) => !saveMutation.isPending && setEditorOpen(open)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <form onSubmit={submit} className="space-y-5">
            <DialogHeader><DialogTitle>{draft.id ? 'Edit routing rule' : 'Add routing rule'}</DialogTitle><DialogDescription>Rules are evaluated against organization events. Use * to match every event type.</DialogDescription></DialogHeader>
            <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="rule-name">Name</Label><Input id="rule-name" required maxLength={200} value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} /></div><div className="space-y-2"><Label htmlFor="rule-event">Event type</Label><Input id="rule-event" required value={draft.eventType} onChange={(event) => setDraft((current) => ({ ...current, eventType: event.target.value.toLowerCase() }))} /></div></div>
            <fieldset className="space-y-2"><legend className="text-sm font-medium">Severity filter</legend><div className="flex flex-wrap gap-4">{SEVERITIES.map((severity) => <label key={severity} className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 rounded border-input" checked={draft.severities.includes(severity)} onChange={(event) => setDraft((current) => ({ ...current, severities: event.target.checked ? [...current.severities, severity] : current.severities.filter((item) => item !== severity) }))} />{severity}</label>)}</div><p className="text-xs text-muted-foreground">No selections matches all severities.</p></fieldset>
            <fieldset className="space-y-2"><legend className="text-sm font-medium">Delivery channels</legend>{channelsQuery.isLoading ? <p role="status" className="text-sm text-muted-foreground">Loading channels…</p> : <div className="grid gap-2 sm:grid-cols-2">{channels.map((channel) => <label key={channel.id} className="flex items-center gap-2 rounded-md border p-2 text-sm"><input type="checkbox" className="h-4 w-4" disabled={!channel.is_active} checked={draft.channelIds.includes(channel.id)} onChange={(event) => setDraft((current) => ({ ...current, channelIds: event.target.checked ? [...current.channelIds, channel.id] : current.channelIds.filter((id) => id !== channel.id) }))} /><span>{channel.name}</span>{!channel.is_active && <span className="text-xs text-muted-foreground">inactive</span>}</label>)}</div>}</fieldset>
            <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="recipient-type">Recipient resolution</Label><Select value={draft.recipientType} onValueChange={(recipientType: NotificationRecipientType) => setDraft((current) => ({ ...current, recipientType, recipientIdsText: ID_RECIPIENT_TYPES.includes(recipientType) ? current.recipientIdsText : '' }))}><SelectTrigger id="recipient-type"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="owner">Entity owner</SelectItem><SelectItem value="assignee">Entity assignee</SelectItem><SelectItem value="dpo">Data protection officer</SelectItem><SelectItem value="ciso">CISO</SelectItem><SelectItem value="role">Role IDs</SelectItem><SelectItem value="user">User IDs</SelectItem><SelectItem value="custom">Custom user IDs</SelectItem></SelectContent></Select></div><div className="space-y-2"><Label htmlFor="rule-template">Template</Label><Select value={draft.templateId || '__default__'} onValueChange={(value) => setDraft((current) => ({ ...current, templateId: value === '__default__' ? '' : value }))}><SelectTrigger id="rule-template"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="__default__">Default event template</SelectItem>{templates.map((template) => <SelectItem key={template.id} value={template.id}>{template.name}</SelectItem>)}</SelectContent></Select></div></div>
            {ID_RECIPIENT_TYPES.includes(draft.recipientType) && <div className="space-y-2"><Label htmlFor="recipient-ids">Recipient UUIDs</Label><Textarea id="recipient-ids" required value={draft.recipientIdsText} onChange={(event) => setDraft((current) => ({ ...current, recipientIdsText: event.target.value }))} placeholder="UUID, UUID" /><p className="text-xs text-muted-foreground">Comma-separated IDs. The current API does not expose an eligible recipient directory for this editor.</p></div>}
            <div className="space-y-2"><Label htmlFor="rule-conditions">Conditions JSON</Label><Textarea id="rule-conditions" className="font-mono text-xs" rows={5} value={draft.conditionsText} onChange={(event) => setDraft((current) => ({ ...current, conditionsText: event.target.value }))} /><p className="text-xs text-muted-foreground">Use an empty object for no additional conditions.</p></div>
            <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="rule-cooldown">Cooldown (minutes)</Label><Input id="rule-cooldown" type="number" min={0} max={525600} step={1} required value={draft.cooldownMinutes} onChange={(event) => setDraft((current) => ({ ...current, cooldownMinutes: event.target.value }))} /></div><div className="flex items-center justify-between gap-3 rounded-md border p-3"><div><Label htmlFor="rule-active">Active</Label><p className="text-xs text-muted-foreground">Route matching future events.</p></div><Switch id="rule-active" checked={draft.active} onCheckedChange={(active) => setDraft((current) => ({ ...current, active }))} /></div></div>
            {validationError && <p role="alert" className="text-sm text-destructive">{validationError}</p>}
            <DialogFooter><Button type="button" variant="outline" disabled={saveMutation.isPending} onClick={() => setEditorOpen(false)}>Cancel</Button><Button type="submit" disabled={saveMutation.isPending || channelsQuery.isLoading}>{saveMutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Save rule</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  );
}
