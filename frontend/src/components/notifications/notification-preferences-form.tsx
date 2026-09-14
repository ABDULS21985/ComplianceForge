'use client';

import { useEffect, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Bell, Clock3, Loader2, Mail, MessageSquare } from 'lucide-react';
import { toast } from 'sonner';

import api from '@/lib/api';
import {
  DEFAULT_NOTIFICATION_PREFERENCE,
  formatApiError,
  localTimeZone,
} from '@/lib/enterprise-settings';
import type {
  DigestFrequency,
  NotificationPreference,
  UpdateNotificationPreferenceInput,
} from '@/types/enterprise-settings';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';

const TIMEZONE_SUGGESTIONS = [
  'UTC',
  'Africa/Lagos',
  'America/Chicago',
  'America/Los_Angeles',
  'America/New_York',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Australia/Sydney',
  'Europe/Berlin',
  'Europe/London',
] as const;

type PreferenceDraft = Pick<
  NotificationPreference,
  | 'email_enabled'
  | 'in_app_enabled'
  | 'slack_enabled'
  | 'digest_frequency'
  | 'quiet_hours_start'
  | 'quiet_hours_end'
  | 'quiet_hours_timezone'
>;

function toDraft(preference: NotificationPreference): PreferenceDraft {
  return {
    email_enabled: preference.email_enabled,
    in_app_enabled: preference.in_app_enabled,
    slack_enabled: preference.slack_enabled,
    digest_frequency: preference.digest_frequency,
    quiet_hours_start: preference.quiet_hours_start ?? '',
    quiet_hours_end: preference.quiet_hours_end ?? '',
    quiet_hours_timezone: preference.quiet_hours_timezone ?? localTimeZone(),
  };
}

export function NotificationPreferencesForm() {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<PreferenceDraft>(() =>
    toDraft(DEFAULT_NOTIFICATION_PREFERENCE)
  );
  const [validationError, setValidationError] = useState('');

  const preferenceQuery = useQuery({
    queryKey: ['notifications', 'preferences'],
    queryFn: () => api.notifications.getPreferences(),
    staleTime: 60_000,
  });

  useEffect(() => {
    if (preferenceQuery.data?.data) setDraft(toDraft(preferenceQuery.data.data));
  }, [preferenceQuery.data]);

  const updateMutation = useMutation({
    mutationFn: (input: UpdateNotificationPreferenceInput) =>
      api.notifications.updatePreferences(input),
    onSuccess: (response) => {
      queryClient.setQueryData(['notifications', 'preferences'], response);
      setDraft(toDraft(response.data));
      toast.success('Notification preferences saved');
    },
    onError: (error) => {
      toast.error(formatApiError(error, 'Notification preferences could not be saved.'));
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setValidationError('');
    const start = draft.quiet_hours_start?.trim() ?? '';
    const end = draft.quiet_hours_end?.trim() ?? '';
    const timezone = draft.quiet_hours_timezone?.trim() ?? '';
    if (Boolean(start) !== Boolean(end)) {
      setValidationError('Enter both a quiet-hours start and end time.');
      return;
    }
    if (start && !timezone) {
      setValidationError('Choose an IANA timezone for quiet hours.');
      return;
    }

    const input: UpdateNotificationPreferenceInput = {
      email_enabled: draft.email_enabled,
      in_app_enabled: draft.in_app_enabled,
      slack_enabled: draft.slack_enabled,
      digest_frequency: draft.digest_frequency,
    };
    if (start && end) {
      input.quiet_hours_start = start;
      input.quiet_hours_end = end;
      input.quiet_hours_timezone = timezone;
    }
    updateMutation.mutate(input);
  }

  if (preferenceQuery.isLoading) {
    return (
      <div role="status" aria-label="Loading notification preferences" className="space-y-4">
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-56 w-full" />
      </div>
    );
  }

  if (preferenceQuery.isError) {
    return (
      <Card>
        <CardContent className="space-y-4 py-8 text-center">
          <p role="alert">{formatApiError(preferenceQuery.error, 'Notification preferences could not be loaded.')}</p>
          <Button type="button" variant="outline" onClick={() => void preferenceQuery.refetch()}>
            Try again
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <form onSubmit={submit} className="space-y-6">
      <fieldset className="grid gap-4 lg:grid-cols-3" disabled={updateMutation.isPending}>
        <legend className="sr-only">Delivery channels</legend>
        {([
          ['email_enabled', 'Email', 'Send messages to my email address.', Mail],
          ['in_app_enabled', 'In app', 'Show messages in my notification center.', Bell],
          ['slack_enabled', 'Slack', 'Send messages through a configured Slack channel.', MessageSquare],
        ] as const).map(([field, title, description, Icon]) => (
          <Card key={field}>
            <CardContent className="flex items-start justify-between gap-4 py-5">
              <div className="flex gap-3">
                <Icon aria-hidden="true" className="mt-0.5 h-5 w-5 text-primary" />
                <div>
                  <Label htmlFor={field} className="font-medium">{title}</Label>
                  <p className="mt-1 text-sm text-muted-foreground">{description}</p>
                </div>
              </div>
              <Switch
                id={field}
                checked={draft[field]}
                onCheckedChange={(checked) => setDraft((current) => ({ ...current, [field]: checked }))}
                aria-label={`${title} notifications`}
              />
            </CardContent>
          </Card>
        ))}
      </fieldset>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <Clock3 aria-hidden="true" className="h-5 w-5" />
            Delivery schedule
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-5 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="digest-frequency">Digest frequency</Label>
            <Select
              value={draft.digest_frequency}
              onValueChange={(value: DigestFrequency) =>
                setDraft((current) => ({ ...current, digest_frequency: value }))
              }
              disabled={updateMutation.isPending}
            >
              <SelectTrigger id="digest-frequency"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="immediate">Immediate</SelectItem>
                <SelectItem value="hourly">Hourly digest</SelectItem>
                <SelectItem value="daily">Daily digest</SelectItem>
                <SelectItem value="weekly">Weekly digest</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2 md:col-span-2">
            <p className="text-sm font-medium">Quiet hours</p>
            <p className="text-xs text-muted-foreground">
              Leave both times empty if quiet hours have never been configured. The current API cannot clear an existing schedule once saved.
            </p>
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="space-y-2">
                <Label htmlFor="quiet-start">Starts</Label>
                <Input
                  id="quiet-start"
                  type="time"
                  value={draft.quiet_hours_start ?? ''}
                  onChange={(event) => setDraft((current) => ({ ...current, quiet_hours_start: event.target.value }))}
                  disabled={updateMutation.isPending}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="quiet-end">Ends</Label>
                <Input
                  id="quiet-end"
                  type="time"
                  value={draft.quiet_hours_end ?? ''}
                  onChange={(event) => setDraft((current) => ({ ...current, quiet_hours_end: event.target.value }))}
                  disabled={updateMutation.isPending}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="quiet-timezone">Timezone</Label>
                <Input
                  id="quiet-timezone"
                  list="quiet-timezones"
                  value={draft.quiet_hours_timezone ?? ''}
                  onChange={(event) => setDraft((current) => ({ ...current, quiet_hours_timezone: event.target.value }))}
                  placeholder="Europe/London"
                  autoComplete="off"
                  disabled={updateMutation.isPending}
                />
                <datalist id="quiet-timezones">
                  {[localTimeZone(), ...TIMEZONE_SUGGESTIONS].filter((item, index, list) => list.indexOf(item) === index).map((timezone) => (
                    <option key={timezone} value={timezone} />
                  ))}
                </datalist>
              </div>
            </div>
          </div>

          {validationError && <p role="alert" className="text-sm text-destructive md:col-span-2">{validationError}</p>}
          <div className="flex items-center justify-end md:col-span-2">
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
              Save preferences
            </Button>
          </div>
        </CardContent>
      </Card>
    </form>
  );
}
