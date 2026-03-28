'use client';

import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  Bell, Mail, MessageSquare, Webhook, Plus, Trash2, TestTube, Settings2,
  Clock, Shield, AlertTriangle, FileText, ClipboardCheck, Building2, Activity,
} from 'lucide-react';

import { api } from '@/lib/api';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription,
} from '@/components/ui/dialog';
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';

// ===========================================================================
// Preferences Tab
// ===========================================================================

function PreferencesTab() {
  const queryClient = useQueryClient();
  const { data: prefs, isLoading } = useQuery({
    queryKey: ['notification-preferences'],
    queryFn: () => api.notifications.getPreferences(),
  });

  const updatePrefs = useMutation({
    mutationFn: (data: any) => api.notifications.updatePreferences(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-preferences'] });
      toast.success('Notification preferences updated');
    },
    onError: () => toast.error('Failed to update preferences'),
  });

  const [localPrefs, setLocalPrefs] = React.useState<any>(null);

  React.useEffect(() => {
    if (prefs) setLocalPrefs(prefs);
  }, [prefs]);

  if (isLoading || !localPrefs) {
    return (
      <div className="space-y-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-20 w-full" />
        ))}
      </div>
    );
  }

  const handleToggle = (field: string, value: boolean) => {
    const updated = { ...localPrefs, [field]: value };
    setLocalPrefs(updated);
    updatePrefs.mutate(updated);
  };

  const handleSelect = (field: string, value: string) => {
    const updated = { ...localPrefs, [field]: value };
    setLocalPrefs(updated);
    updatePrefs.mutate(updated);
  };

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-semibold">Notification Channels</h3>
        <p className="text-sm text-muted-foreground">Choose how you want to receive notifications.</p>
      </div>

      <div className="grid gap-4">
        <Card>
          <CardContent className="flex items-center justify-between py-4">
            <div className="flex items-center gap-3">
              <Mail className="h-5 w-5 text-blue-600" />
              <div>
                <p className="font-medium">Email Notifications</p>
                <p className="text-sm text-muted-foreground">Receive notifications via email</p>
              </div>
            </div>
            <Switch
              checked={localPrefs.email_enabled ?? true}
              onCheckedChange={(v) => handleToggle('email_enabled', v)}
            />
          </CardContent>
        </Card>

        <Card>
          <CardContent className="flex items-center justify-between py-4">
            <div className="flex items-center gap-3">
              <Bell className="h-5 w-5 text-indigo-600" />
              <div>
                <p className="font-medium">In-App Notifications</p>
                <p className="text-sm text-muted-foreground">Show notifications in the app bell</p>
              </div>
            </div>
            <Switch
              checked={localPrefs.in_app_enabled ?? true}
              onCheckedChange={(v) => handleToggle('in_app_enabled', v)}
            />
          </CardContent>
        </Card>

        <Card>
          <CardContent className="flex items-center justify-between py-4">
            <div className="flex items-center gap-3">
              <MessageSquare className="h-5 w-5 text-purple-600" />
              <div>
                <p className="font-medium">Slack Notifications</p>
                <p className="text-sm text-muted-foreground">Receive notifications in Slack</p>
              </div>
            </div>
            <Switch
              checked={localPrefs.slack_enabled ?? false}
              onCheckedChange={(v) => handleToggle('slack_enabled', v)}
            />
          </CardContent>
        </Card>
      </div>

      <div>
        <h3 className="text-lg font-semibold">Delivery Settings</h3>
        <p className="text-sm text-muted-foreground">Configure when and how often you receive notifications.</p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardContent className="py-4 space-y-3">
            <Label>Digest Frequency</Label>
            <Select
              value={localPrefs.digest_frequency ?? 'immediate'}
              onValueChange={(v) => handleSelect('digest_frequency', v)}
            >
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="immediate">Immediate</SelectItem>
                <SelectItem value="hourly">Hourly Digest</SelectItem>
                <SelectItem value="daily">Daily Digest</SelectItem>
                <SelectItem value="weekly">Weekly Digest</SelectItem>
              </SelectContent>
            </Select>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="py-4 space-y-3">
            <Label>Quiet Hours</Label>
            <div className="flex items-center gap-2">
              <Input
                type="time"
                value={localPrefs.quiet_hours_start ?? '22:00'}
                onChange={(e) => handleSelect('quiet_hours_start', e.target.value)}
                className="w-28"
              />
              <span className="text-sm text-muted-foreground">to</span>
              <Input
                type="time"
                value={localPrefs.quiet_hours_end ?? '07:00'}
                onChange={(e) => handleSelect('quiet_hours_end', e.target.value)}
                className="w-28"
              />
            </div>
            <p className="text-xs text-muted-foreground">No notifications during these hours (except regulatory alerts)</p>
          </CardContent>
        </Card>
      </div>

      <Card className="border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/30">
        <CardContent className="flex items-start gap-3 py-4">
          <Shield className="h-5 w-5 text-amber-600 mt-0.5" />
          <div>
            <p className="font-medium text-amber-800 dark:text-amber-400">Regulatory Notifications Cannot Be Silenced</p>
            <p className="text-sm text-amber-700 dark:text-amber-500">
              GDPR breach deadline alerts, NIS2 early warning alerts, and DSR deadline notifications
              are always delivered regardless of your preference settings. This is a regulatory requirement.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// ===========================================================================
// Rules & Channels Tab (Admin)
// ===========================================================================

function RulesTab() {
  const queryClient = useQueryClient();
  const [channelDialogOpen, setChannelDialogOpen] = useState(false);
  const [ruleDialogOpen, setRuleDialogOpen] = useState(false);

  const { data: channels, isLoading: channelsLoading } = useQuery({
    queryKey: ['notification-channels'],
    queryFn: () => api.settings ? (api as any).settings?.notificationChannels?.() : api.get('/settings/notification-channels'),
  });

  const { data: rulesData, isLoading: rulesLoading } = useQuery({
    queryKey: ['notification-rules'],
    queryFn: () => api.get('/settings/notification-rules'),
  });

  const rules: any[] = Array.isArray(rulesData) ? rulesData : (rulesData as any)?.data ?? [];
  const channelList: any[] = Array.isArray(channels) ? channels : (channels as any)?.data ?? [];

  const testChannel = useMutation({
    mutationFn: (id: string) => api.post(`/settings/notification-channels/${id}/test`),
    onSuccess: () => toast.success('Test notification sent'),
    onError: () => toast.error('Test failed'),
  });

  const deleteRule = useMutation({
    mutationFn: (id: string) => api.delete(`/settings/notification-rules/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-rules'] });
      toast.success('Rule deleted');
    },
    onError: () => toast.error('Failed to delete rule'),
  });

  const channelIcon = (type: string) => {
    switch (type) {
      case 'email': return <Mail className="h-4 w-4" />;
      case 'slack': return <MessageSquare className="h-4 w-4" />;
      case 'webhook': return <Webhook className="h-4 w-4" />;
      case 'in_app': return <Bell className="h-4 w-4" />;
      default: return <Settings2 className="h-4 w-4" />;
    }
  };

  return (
    <div className="space-y-8">
      {/* Channels Section */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold">Notification Channels</h3>
            <p className="text-sm text-muted-foreground">Configure delivery channels for your organization.</p>
          </div>
          <Button onClick={() => setChannelDialogOpen(true)} size="sm">
            <Plus className="mr-1 h-4 w-4" /> Add Channel
          </Button>
        </div>

        {channelsLoading ? (
          <div className="space-y-3">{Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-16" />)}</div>
        ) : channelList.length === 0 ? (
          <Card><CardContent className="py-8 text-center text-muted-foreground">No channels configured yet.</CardContent></Card>
        ) : (
          <div className="grid gap-3">
            {channelList.map((ch: any) => (
              <Card key={ch.id}>
                <CardContent className="flex items-center justify-between py-3">
                  <div className="flex items-center gap-3">
                    {channelIcon(ch.channel_type)}
                    <div>
                      <p className="font-medium">{ch.name}</p>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline">{ch.channel_type}</Badge>
                        {ch.is_default && <Badge variant="secondary">Default</Badge>}
                        <Badge variant={ch.is_active ? 'default' : 'secondary'}>
                          {ch.is_active ? 'Active' : 'Inactive'}
                        </Badge>
                      </div>
                    </div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => testChannel.mutate(ch.id)}
                    disabled={testChannel.isPending}
                  >
                    <TestTube className="mr-1 h-3 w-3" />
                    Test
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      {/* Rules Section */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold">Notification Rules</h3>
            <p className="text-sm text-muted-foreground">Define which events trigger notifications and to whom.</p>
          </div>
          <Button onClick={() => setRuleDialogOpen(true)} size="sm">
            <Plus className="mr-1 h-4 w-4" /> Add Rule
          </Button>
        </div>

        {rulesLoading ? (
          <div className="space-y-3">{Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-16" />)}</div>
        ) : rules.length === 0 ? (
          <Card><CardContent className="py-8 text-center text-muted-foreground">No custom rules. System defaults are active.</CardContent></Card>
        ) : (
          <div className="grid gap-3">
            {rules.map((rule: any) => (
              <Card key={rule.id}>
                <CardContent className="flex items-center justify-between py-3">
                  <div>
                    <p className="font-medium">{rule.name}</p>
                    <div className="flex items-center gap-2 mt-1">
                      <Badge variant="outline">{rule.event_type}</Badge>
                      <Badge variant="secondary">{rule.recipient_type}</Badge>
                      {rule.severity_filter?.length > 0 && (
                        <Badge variant="secondary">
                          {rule.severity_filter.join(', ')}
                        </Badge>
                      )}
                      <Badge variant={rule.is_active ? 'default' : 'secondary'}>
                        {rule.is_active ? 'Active' : 'Inactive'}
                      </Badge>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => deleteRule.mutate(rule.id)}
                    disabled={deleteRule.isPending}
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// ===========================================================================
// Main Page
// ===========================================================================

export default function NotificationSettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Notification Settings</h1>
        <p className="text-muted-foreground">
          Configure how and when you receive compliance notifications.
        </p>
      </div>

      <Tabs defaultValue="preferences">
        <TabsList>
          <TabsTrigger value="preferences">My Preferences</TabsTrigger>
          <TabsTrigger value="rules">Rules & Channels</TabsTrigger>
        </TabsList>

        <TabsContent value="preferences" className="mt-6">
          <PreferencesTab />
        </TabsContent>

        <TabsContent value="rules" className="mt-6">
          <RulesTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
