'use client';

import { Card, CardContent } from '@/components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import api from '@/lib/api';
import { APIKeysPanel } from '@/components/integrations/api-keys-panel';
import { ConfiguredIntegrationsPanel } from '@/components/integrations/configured-integrations-panel';
import { hasSettingsPermission } from '@/lib/enterprise-settings';
import { IntegrationCatalogPanel } from '@/components/integrations/integration-catalog-panel';
import { LockKeyhole } from 'lucide-react';
import { normalizePermissionMap } from '@/lib/navigation';
import { Skeleton } from '@/components/ui/skeleton';
import { SSOPanel } from '@/components/integrations/sso-panel';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export default function IntegrationsPage() {
  const user = useAuthStore((state) => state.user);
  const permissionsQuery = useQuery({
    queryKey: ['access', 'my-permissions', user?.id],
    queryFn: () => api.access.myPermissions(),
    enabled: Boolean(user),
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const permissions = normalizePermissionMap(permissionsQuery.data);
  const canRead = hasSettingsPermission(permissions, user, 'read');
  const canConfigure = hasSettingsPermission(permissions, user, 'configure');

  if (permissionsQuery.isLoading) {
    return <div role="status" aria-label="Checking integration permissions" className="space-y-5"><Skeleton className="h-20 w-full" /><Skeleton className="h-12 w-full" /><Skeleton className="h-72 w-full" /></div>;
  }

  if (!canRead) {
    return (
      <Card className="mx-auto max-w-xl">
        <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
          <LockKeyhole aria-hidden="true" className="h-9 w-9 text-muted-foreground" />
          <h1 className="text-xl font-semibold">Integration settings unavailable</h1>
          <p className="text-sm text-muted-foreground">Your role does not grant organization settings access.</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Integration hub</h1>
        <p className="mt-1 text-muted-foreground">Manage external connectors, synchronization history, SSO, and automation credentials.</p>
      </div>
      {!canConfigure && <Card><CardContent className="flex gap-3 py-4 text-sm text-muted-foreground"><LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />You have read-only settings access. Secret, lifecycle, test, and synchronization controls are hidden.</CardContent></Card>}
      <Tabs defaultValue="configured" className="space-y-6">
        <TabsList className="h-auto max-w-full flex-wrap justify-start">
          <TabsTrigger value="configured">Configured</TabsTrigger>
          <TabsTrigger value="catalog">Catalog</TabsTrigger>
          <TabsTrigger value="sso">Single sign-on</TabsTrigger>
          <TabsTrigger value="api-keys">API keys</TabsTrigger>
        </TabsList>
        <TabsContent value="configured"><ConfiguredIntegrationsPanel canConfigure={canConfigure} /></TabsContent>
        <TabsContent value="catalog"><IntegrationCatalogPanel canConfigure={canConfigure} /></TabsContent>
        <TabsContent value="sso"><SSOPanel canConfigure={canConfigure} /></TabsContent>
        <TabsContent value="api-keys"><APIKeysPanel canConfigure={canConfigure} /></TabsContent>
      </Tabs>
    </div>
  );
}
