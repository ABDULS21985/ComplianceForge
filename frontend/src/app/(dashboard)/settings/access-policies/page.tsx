'use client';

import * as React from 'react';
import {
  accessAdminKeys,
  formatAccessError,
  groupPermissions,
  humanizeAccessToken,
  permissionKey,
} from '@/lib/access-admin';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  ChevronLeft,
  ChevronRight,
  Copy,
  KeyRound,
  LockKeyhole,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Users,
} from 'lucide-react';
import type { ManagedRole, ManagedRoleListParams, PermissionGrant } from '@/types/access-admin';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import Link from 'next/link';
import { RoleCloneDialog } from '@/components/access-admin/role-clone-dialog';
import { RoleEditorDialog } from '@/components/access-admin/role-editor-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useAccessAdminPermissions } from '@/hooks/use-access-admin-permissions';
import { useQuery } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';

const PAGE_SIZE = 20;

export default function AccessAdministrationPage() {
  const router = useRouter();
  const permission = useAccessAdminPermissions();
  const [page, setPage] = React.useState(1);
  const [search, setSearch] = React.useState('');
  const deferredSearch = React.useDeferredValue(search.trim());
  const [includeSystem, setIncludeSystem] = React.useState(true);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [cloning, setCloning] = React.useState<ManagedRole | null>(null);
  const roleParams: ManagedRoleListParams = {
    include_system: includeSystem,
    page,
    page_size: PAGE_SIZE,
    search: deferredSearch || undefined,
  };
  const rolesQuery = useQuery({
    queryKey: accessAdminKeys.roles(roleParams as Record<string, unknown>),
    queryFn: () => api.access.listRoles(roleParams),
    enabled: permission.canRead,
  });
  const catalogueQuery = useQuery({
    queryKey: accessAdminKeys.permissions,
    queryFn: () => api.access.permissionCatalogue(),
    enabled: permission.canRead,
    staleTime: 10 * 60_000,
  });
  const roles = rolesQuery.data?.data ?? [];
  const pagination = rolesQuery.data?.pagination;
  const catalogue = catalogueQuery.data?.data ?? [];

  if (permission.isLoading) return <PageSkeleton />;
  if (permission.isError) {
    return <PageError title="Access could not be checked" message="Your current permissions could not be loaded." onRetry={() => void permission.retry()} />;
  }
  if (!permission.canRead) {
    return (
      <Card className="mx-auto max-w-xl">
        <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
          <LockKeyhole aria-hidden="true" className="h-10 w-10 text-muted-foreground" />
          <h1 className="text-xl font-semibold">Access administration unavailable</h1>
          <p className="text-sm text-muted-foreground">Your role does not grant organization settings access.</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Access administration</h1>
          <p className="mt-1 max-w-3xl text-muted-foreground">Manage tenant roles, exact API permissions, user assignments, and an append-only change history.</p>
        </div>
        {permission.canConfigure && (
          <Button type="button" disabled={catalogueQuery.isLoading || catalogueQuery.isError} onClick={() => setCreateOpen(true)}>
            <Plus aria-hidden="true" className="mr-2 h-4 w-4" />Create custom role
          </Button>
        )}
      </div>

      {!permission.canConfigure && (
        <Card><CardContent className="flex gap-3 py-4 text-sm text-muted-foreground"><LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />You have read-only settings access. Role, permission, and assignment changes are hidden.</CardContent></Card>
      )}

      <div className="grid gap-4 sm:grid-cols-3">
        <Metric icon={ShieldCheck} label="Roles matching filters" value={pagination?.total_items ?? 0} />
        <Metric icon={KeyRound} label="Catalogue permissions" value={catalogue.length} />
        <Metric icon={Users} label="Assignments on this page" value={roles.reduce((total, role) => total + role.assigned_users, 0)} />
      </div>

      <Tabs defaultValue="roles" className="space-y-5">
        <TabsList className="h-auto max-w-full flex-wrap justify-start">
          <TabsTrigger value="roles">Roles</TabsTrigger>
          <TabsTrigger value="catalogue">Permission catalogue</TabsTrigger>
        </TabsList>
        <TabsContent value="roles" className="space-y-4">
          <Card>
            <CardContent className="flex flex-col gap-4 pt-6 md:flex-row md:items-center md:justify-between">
              <div className="relative w-full md:max-w-md">
                <Search aria-hidden="true" className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  aria-label="Search roles"
                  className="pl-9"
                  maxLength={200}
                  placeholder="Search name, slug, or description"
                  value={search}
                  onChange={(event) => { setSearch(event.target.value); setPage(1); }}
                />
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" className="h-4 w-4" checked={includeSystem} onChange={(event) => { setIncludeSystem(event.target.checked); setPage(1); }} />
                Include platform system roles
              </label>
            </CardContent>
          </Card>

          {rolesQuery.isLoading ? <RoleGridSkeleton /> : rolesQuery.isError ? (
            <PageError title="Roles could not be loaded" message={formatAccessError(rolesQuery.error, 'The role catalogue is temporarily unavailable.')} onRetry={() => void rolesQuery.refetch()} />
          ) : roles.length === 0 ? (
            <Card><CardContent className="flex flex-col items-center py-14 text-center"><ShieldCheck aria-hidden="true" className="h-10 w-10 text-muted-foreground" /><h2 className="mt-3 text-lg font-semibold">No matching roles</h2><p className="mt-1 text-sm text-muted-foreground">Adjust the search or system-role filter, or create a custom role.</p></CardContent></Card>
          ) : (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {roles.map((role) => (
                <Card key={role.id} className="flex flex-col">
                  <CardHeader className="flex-1">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <CardTitle className="truncate text-lg"><Link className="hover:underline" href={`/settings/access-policies/${role.id}`}>{role.name}</Link></CardTitle>
                        <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{role.slug}</p>
                      </div>
                      <Badge variant={role.is_system_role ? 'secondary' : 'outline'}>{role.is_system_role ? 'System' : 'Custom'}</Badge>
                    </div>
                    <CardDescription className="line-clamp-2 pt-2">{role.description || 'No description provided.'}</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <dl className="grid grid-cols-3 gap-2 text-center text-sm">
                      <div className="rounded-md bg-muted p-2"><dt className="text-xs text-muted-foreground">Permissions</dt><dd className="font-semibold">{role.permissions.length}</dd></div>
                      <div className="rounded-md bg-muted p-2"><dt className="text-xs text-muted-foreground">Users</dt><dd className="font-semibold">{role.assigned_users}</dd></div>
                      <div className="rounded-md bg-muted p-2"><dt className="text-xs text-muted-foreground">Version</dt><dd className="font-semibold">{role.version}</dd></div>
                    </dl>
                    <div className="flex flex-wrap gap-2">
                      <Button asChild size="sm" variant="outline"><Link href={`/settings/access-policies/${role.id}`}>View details</Link></Button>
                      {permission.canConfigure && <Button type="button" size="sm" variant="ghost" onClick={() => setCloning(role)}><Copy aria-hidden="true" className="mr-2 h-4 w-4" />Clone</Button>}
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {pagination && pagination.total_pages > 1 && (
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-muted-foreground">Page {pagination.page} of {pagination.total_pages} · {pagination.total_items} roles</p>
              <div className="flex gap-2">
                <Button type="button" variant="outline" size="sm" disabled={page <= 1 || rolesQuery.isFetching} onClick={() => setPage((current) => Math.max(1, current - 1))}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button>
                <Button type="button" variant="outline" size="sm" disabled={page >= pagination.total_pages || rolesQuery.isFetching} onClick={() => setPage((current) => current + 1)}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button>
              </div>
            </div>
          )}
        </TabsContent>
        <TabsContent value="catalogue">
          <PermissionCatalogue catalogue={catalogue} isError={catalogueQuery.isError} isLoading={catalogueQuery.isLoading} onRetry={() => void catalogueQuery.refetch()} />
        </TabsContent>
      </Tabs>

      {createOpen && <RoleEditorDialog catalogue={catalogue} open onOpenChange={setCreateOpen} onSaved={(created) => router.push(`/settings/access-policies/${created.id}`)} />}
      {cloning && <RoleCloneDialog role={cloning} open onOpenChange={(next) => !next && setCloning(null)} onCloned={(cloned) => router.push(`/settings/access-policies/${cloned.id}`)} />}
    </div>
  );
}

function Metric({ icon: Icon, label, value }: { icon: React.ElementType; label: string; value: number }) {
  return <Card><CardContent className="flex items-center gap-3 pt-6"><div className="rounded-md bg-primary/10 p-2"><Icon aria-hidden="true" className="h-5 w-5 text-primary" /></div><div><p className="text-sm text-muted-foreground">{label}</p><p className="text-2xl font-semibold">{value}</p></div></CardContent></Card>;
}

function PageError({ message, onRetry, title }: { message: string; onRetry: () => void; title: string }) {
  return <Card><CardContent className="flex flex-col items-center gap-3 py-12 text-center"><RefreshCw aria-hidden="true" className="h-9 w-9 text-muted-foreground" /><h2 className="text-lg font-semibold">{title}</h2><p role="alert" className="text-sm text-muted-foreground">{message}</p><Button type="button" variant="outline" onClick={onRetry}>Try again</Button></CardContent></Card>;
}

function PageSkeleton() {
  return <div role="status" aria-label="Loading access administration" className="space-y-5"><Skeleton className="h-24 w-full" /><div className="grid gap-4 sm:grid-cols-3">{Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-24" />)}</div><RoleGridSkeleton /></div>;
}

function PermissionCatalogue({ catalogue, isError, isLoading, onRetry }: { catalogue: PermissionGrant[]; isError: boolean; isLoading: boolean; onRetry: () => void }) {
  if (isLoading) return <RoleGridSkeleton />;
  if (isError) return <PageError title="Permission catalogue unavailable" message="The canonical permission catalogue could not be loaded." onRetry={onRetry} />;
  const groups = groupPermissions(catalogue);
  if (groups.length === 0) return <Card><CardContent className="py-12 text-center"><p className="font-semibold">No permissions published</p><p className="mt-1 text-sm text-muted-foreground">Role creation is disabled until the backend publishes a catalogue.</p></CardContent></Card>;
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {groups.map((group) => (
        <Card key={group.resource}>
          <CardHeader><CardTitle className="text-lg">{humanizeAccessToken(group.resource)}</CardTitle><CardDescription>{group.permissions.length} available actions</CardDescription></CardHeader>
          <CardContent><ul className="space-y-3">{group.permissions.map((item) => <li key={permissionKey(item)} className="border-t pt-3 first:border-0 first:pt-0"><div className="flex items-start justify-between gap-3"><span className="font-medium">{humanizeAccessToken(item.action)}</span><code className="rounded bg-muted px-1.5 py-0.5 text-xs">{permissionKey(item)}</code></div>{item.description && <p className="mt-1 text-sm text-muted-foreground">{item.description}</p>}</li>)}</ul></CardContent>
        </Card>
      ))}
    </div>
  );
}

function RoleGridSkeleton() {
  return <div role="status" aria-label="Loading roles" className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{Array.from({ length: 6 }).map((_, index) => <Skeleton key={index} className="h-64" />)}</div>;
}
