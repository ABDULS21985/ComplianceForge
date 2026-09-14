'use client';

import { useQuery } from '@tanstack/react-query';
import { LockKeyhole } from 'lucide-react';

import api from '@/lib/api';
import { hasSettingsPermission } from '@/lib/enterprise-settings';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { NotificationPreferencesForm } from '@/components/notifications/notification-preferences-form';
import { NotificationChannelsPanel } from '@/components/notifications/notification-channels-panel';
import { NotificationRulesPanel } from '@/components/notifications/notification-rules-panel';
import { NotificationTemplatesPanel } from '@/components/notifications/notification-templates-panel';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

export default function NotificationSettingsPage() {
  const user = useAuthStore((state) => state.user);
  const permissionsQuery = useQuery({
    queryKey: ['access', 'my-permissions', user?.id],
    queryFn: () => api.access.myPermissions(),
    enabled: Boolean(user),
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const permissions = normalizePermissionMap(permissionsQuery.data);
  const canReadAdministration = hasSettingsPermission(permissions, user, 'read');
  const canConfigure = hasSettingsPermission(permissions, user, 'configure');

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Notification settings</h1>
        <p className="mt-1 text-muted-foreground">Control your delivery schedule and, with permission, organization routing.</p>
      </div>

      <Tabs defaultValue="preferences" className="space-y-6">
        <TabsList className="h-auto max-w-full flex-wrap justify-start">
          <TabsTrigger value="preferences">My preferences</TabsTrigger>
          {permissionsQuery.isLoading && <Skeleton className="mx-2 h-8 w-36" />}
          {canReadAdministration && <TabsTrigger value="channels">Channels</TabsTrigger>}
          {canReadAdministration && <TabsTrigger value="rules">Rules</TabsTrigger>}
          {canReadAdministration && <TabsTrigger value="templates">Templates</TabsTrigger>}
        </TabsList>

        <TabsContent value="preferences"><NotificationPreferencesForm /></TabsContent>
        {canReadAdministration && <TabsContent value="channels"><NotificationChannelsPanel canConfigure={canConfigure} /></TabsContent>}
        {canReadAdministration && <TabsContent value="rules"><NotificationRulesPanel canConfigure={canConfigure} /></TabsContent>}
        {canReadAdministration && <TabsContent value="templates"><NotificationTemplatesPanel canConfigure={canConfigure} /></TabsContent>}
      </Tabs>

      {!permissionsQuery.isLoading && !canReadAdministration && (
        <Card>
          <CardContent className="flex gap-3 py-4 text-sm text-muted-foreground">
            <LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
            Organization notification administration is hidden because your role does not grant settings access. Your personal preferences remain available.
          </CardContent>
        </Card>
      )}
      {canReadAdministration && !canConfigure && (
        <Card>
          <CardContent className="flex gap-3 py-4 text-sm text-muted-foreground">
            <LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
            You have read-only settings access. Creation, editing, testing, and destructive actions are disabled.
          </CardContent>
        </Card>
      )}
    </div>
  );
}
