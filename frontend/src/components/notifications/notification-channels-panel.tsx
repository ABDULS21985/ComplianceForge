'use client';

import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Bell, Loader2, Mail, MessageSquare, Pencil, Plus, Send, Trash2, Webhook } from 'lucide-react';
import { toast } from 'sonner';

import api from '@/lib/api';
import { describeChannelConfiguration, formatApiError } from '@/lib/enterprise-settings';
import type {
  NotificationChannel,
  NotificationChannelInput,
  NotificationChannelType,
} from '@/types/enterprise-settings';
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';

interface ChannelDraft {
  id?: string;
  name: string;
  channelType: NotificationChannelType;
  active: boolean;
  endpoint: string;
  secret: string;
}

const EMPTY_CHANNEL: ChannelDraft = {
  name: '',
  channelType: 'in_app',
  active: true,
  endpoint: '',
  secret: '',
};

function ChannelIcon({ type }: { type: NotificationChannelType }) {
  const Icon = type === 'email' ? Mail : type === 'in_app' ? Bell : type === 'slack' ? MessageSquare : Webhook;
  return <Icon aria-hidden="true" className="h-5 w-5" />;
}

function validateChannel(draft: ChannelDraft): string {
  if (!draft.name.trim()) return 'Name is required.';
  if (draft.name.trim().length > 200) return 'Name must not exceed 200 characters.';
  if (draft.channelType === 'webhook') {
    try {
      const endpoint = new URL(draft.endpoint);
      if (endpoint.protocol !== 'https:') return 'Webhook URL must use HTTPS.';
    } catch {
      return 'Enter a valid absolute webhook URL.';
    }
    if (draft.secret.length < 32 || draft.secret.length > 512) {
      return 'Webhook secret must contain between 32 and 512 characters.';
    }
  }
  if (draft.channelType === 'slack') {
    try {
      const endpoint = new URL(draft.endpoint);
      if (endpoint.protocol !== 'https:' || endpoint.hostname !== 'hooks.slack.com') {
        return 'Slack webhook URL must use hooks.slack.com over HTTPS.';
      }
    } catch {
      return 'Enter a valid Slack webhook URL.';
    }
  }
  return '';
}

function toInput(draft: ChannelDraft): NotificationChannelInput {
  const config: Record<string, unknown> = {};
  if (draft.channelType === 'webhook') {
    config.url = draft.endpoint.trim();
    config.secret = draft.secret;
  } else if (draft.channelType === 'slack') {
    config.webhook_url = draft.endpoint.trim();
  }
  return {
    name: draft.name.trim(),
    channel_type: draft.channelType,
    config,
    is_active: draft.active,
  };
}

export function NotificationChannelsPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [editorOpen, setEditorOpen] = useState(false);
  const [draft, setDraft] = useState<ChannelDraft>(EMPTY_CHANNEL);
  const [validationError, setValidationError] = useState('');

  const channelsQuery = useQuery({
    queryKey: ['notification-admin', 'channels'],
    queryFn: () => api.notificationAdmin.listChannels(),
    staleTime: 30_000,
  });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['notification-admin'] });

  const saveMutation = useMutation({
    mutationFn: ({ id, input }: { id?: string; input: NotificationChannelInput }) =>
      id
        ? api.notificationAdmin.updateChannel(id, input)
        : api.notificationAdmin.createChannel(input),
    onSuccess: (_, variables) => {
      void invalidate();
      setEditorOpen(false);
      setDraft(EMPTY_CHANNEL);
      toast.success(variables.id ? 'Notification channel updated' : 'Notification channel created');
    },
    onError: (error) => toast.error(formatApiError(error, 'The notification channel could not be saved.')),
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.notificationAdmin.deleteChannel(id),
    onSuccess: () => {
      void invalidate();
      toast.success('Notification channel deleted');
    },
    onError: (error) => toast.error(formatApiError(error, 'The notification channel could not be deleted.')),
  });
  const testMutation = useMutation({
    mutationFn: (id: string) => api.notificationAdmin.testChannel(id),
    onSuccess: (response) => toast.success(response.message),
    onError: (error) => toast.error(formatApiError(error, 'The test notification could not be sent.')),
  });

  function edit(channel?: NotificationChannel) {
    setValidationError('');
    setDraft(channel ? {
      id: channel.id,
      name: channel.name,
      channelType: channel.channel_type,
      active: channel.is_active,
      endpoint: '',
      secret: '',
    } : EMPTY_CHANNEL);
    setEditorOpen(true);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const error = validateChannel(draft);
    setValidationError(error);
    if (error) return;
    saveMutation.mutate({ id: draft.id, input: toInput(draft) });
  }

  return (
    <section aria-labelledby="notification-channels-heading" className="space-y-4">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
        <div>
          <h2 id="notification-channels-heading" className="text-xl font-semibold">Delivery channels</h2>
          <p className="text-sm text-muted-foreground">Credentials are encrypted by the API and only redacted status is returned.</p>
        </div>
        {canConfigure && <Button type="button" size="sm" onClick={() => edit()}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Add channel</Button>}
      </div>

      {channelsQuery.isLoading && <div role="status" aria-label="Loading channels" className="space-y-3">{[0, 1, 2].map((item) => <Skeleton key={item} className="h-24 w-full" />)}</div>}
      {channelsQuery.isError && (
        <Card><CardContent className="space-y-3 py-8 text-center"><p role="alert">{formatApiError(channelsQuery.error, 'Notification channels could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void channelsQuery.refetch()}>Try again</Button></CardContent></Card>
      )}
      {channelsQuery.isSuccess && channelsQuery.data.data.length === 0 && (
        <Card><CardContent className="py-10 text-center text-sm text-muted-foreground">No delivery channels are configured.</CardContent></Card>
      )}
      <div className="grid gap-3">
        {channelsQuery.data?.data.map((channel) => (
          <Card key={channel.id}>
            <CardContent className="flex flex-col justify-between gap-4 py-4 md:flex-row md:items-center">
              <div className="flex min-w-0 gap-3">
                <span className="mt-1 text-muted-foreground"><ChannelIcon type={channel.channel_type} /></span>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-medium">{channel.name}</h3>
                    <Badge variant="outline">{channel.channel_type.replace('_', ' ')}</Badge>
                    <Badge variant={channel.is_active ? 'default' : 'secondary'}>{channel.is_active ? 'Active' : 'Inactive'}</Badge>
                  </div>
                  <p className="mt-1 break-all text-sm text-muted-foreground">{describeChannelConfiguration(channel)}</p>
                </div>
              </div>
              {canConfigure && (
                <div className="flex flex-wrap gap-2 md:justify-end">
                  <ConfirmAction
                    title={`Send a test through ${channel.name}?`}
                    description="This performs a real delivery using the stored encrypted configuration and may contact an external recipient or endpoint."
                    actionLabel="Send test"
                    onConfirm={() => testMutation.mutate(channel.id)}
                    pending={testMutation.isPending && testMutation.variables === channel.id}
                    disabled={!channel.is_active}
                  ><Send aria-hidden="true" className="mr-2 h-4 w-4" />Test</ConfirmAction>
                  <Button type="button" size="sm" variant="outline" onClick={() => edit(channel)}><Pencil aria-hidden="true" className="mr-2 h-4 w-4" />Edit</Button>
                  <ConfirmAction
                    title={`Delete ${channel.name}?`}
                    description="The encrypted configuration will be erased. Channels referenced by active rules cannot be deleted. This cannot be undone."
                    actionLabel="Delete channel"
                    onConfirm={() => deleteMutation.mutate(channel.id)}
                    pending={deleteMutation.isPending && deleteMutation.variables === channel.id}
                    triggerVariant="destructive"
                  ><Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />Delete</ConfirmAction>
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      <Dialog open={editorOpen} onOpenChange={(open) => !saveMutation.isPending && setEditorOpen(open)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
          <form onSubmit={submit} className="space-y-5">
            <DialogHeader>
              <DialogTitle>{draft.id ? 'Edit notification channel' : 'Add notification channel'}</DialogTitle>
              <DialogDescription>
                {draft.id
                  ? 'For credential-backed channels, enter the complete replacement configuration. Existing secrets are never returned to this form.'
                  : 'The configuration is sent through the secure BFF and encrypted by the API.'}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-2">
              <Label htmlFor="channel-name">Name</Label>
              <Input id="channel-name" maxLength={200} required value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="channel-type">Channel type</Label>
              <Select value={draft.channelType} onValueChange={(channelType: NotificationChannelType) => setDraft((current) => ({ ...current, channelType, endpoint: '', secret: '' }))} disabled={Boolean(draft.id)}>
                <SelectTrigger id="channel-type"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="in_app">In app</SelectItem>
                  <SelectItem value="email">Platform email</SelectItem>
                  <SelectItem value="slack">Slack webhook</SelectItem>
                  <SelectItem value="webhook">Signed webhook</SelectItem>
                </SelectContent>
              </Select>
              {draft.id && <p className="text-xs text-muted-foreground">Create a new channel to change its type.</p>}
            </div>
            {draft.channelType === 'slack' && (
              <div className="space-y-2">
                <Label htmlFor="slack-webhook">Slack webhook URL</Label>
                <Input id="slack-webhook" type="password" autoComplete="new-password" required placeholder="https://hooks.slack.com/services/…" value={draft.endpoint} onChange={(event) => setDraft((current) => ({ ...current, endpoint: event.target.value }))} />
              </div>
            )}
            {draft.channelType === 'webhook' && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="webhook-url">Public HTTPS endpoint</Label>
                  <Input id="webhook-url" type="url" required placeholder="https://hooks.example.com/compliance" value={draft.endpoint} onChange={(event) => setDraft((current) => ({ ...current, endpoint: event.target.value }))} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="webhook-secret">Signing secret</Label>
                  <Input id="webhook-secret" type="password" autoComplete="new-password" minLength={32} maxLength={512} required value={draft.secret} onChange={(event) => setDraft((current) => ({ ...current, secret: event.target.value }))} />
                  <p className="text-xs text-muted-foreground">32–512 characters. It will not be shown again.</p>
                </div>
              </>
            )}
            <div className="flex items-center justify-between gap-4 rounded-md border p-3">
              <div><Label htmlFor="channel-active">Active</Label><p className="text-xs text-muted-foreground">Only active channels can deliver or be selected by new rules.</p></div>
              <Switch id="channel-active" checked={draft.active} onCheckedChange={(active) => setDraft((current) => ({ ...current, active }))} />
            </div>
            {validationError && <p role="alert" className="text-sm text-destructive">{validationError}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditorOpen(false)} disabled={saveMutation.isPending}>Cancel</Button>
              <Button type="submit" disabled={saveMutation.isPending}>{saveMutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Save channel</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  );
}
