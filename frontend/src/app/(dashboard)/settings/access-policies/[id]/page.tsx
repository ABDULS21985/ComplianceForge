'use client';

import * as React from 'react';
import {
  accessAdminKeys,
  formatAccessError,
  groupPermissions,
  humanizeAccessToken,
  permissionKey,
  roleHasPermission,
} from '@/lib/access-admin';
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Copy,
  History,
  LockKeyhole,
  Pencil,
  RefreshCw,
  ShieldCheck,
  Trash2,
  UserMinus,
  UserPlus,
  Users,
} from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import type { ManagedRoleAssignment, RoleChangeEvent } from '@/types/access-admin';
import { RoleAssignDialog, RoleUnassignDialog } from '@/components/access-admin/role-assignment-dialogs';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useParams, useRouter } from 'next/navigation';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { RoleCloneDialog } from '@/components/access-admin/role-clone-dialog';
import { RoleDeleteDialog } from '@/components/access-admin/role-delete-dialog';
import { RoleEditorDialog } from '@/components/access-admin/role-editor-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useAccessAdminPermissions } from '@/hooks/use-access-admin-permissions';
import { useQuery } from '@tanstack/react-query';

const PAGE_SIZE = 20;

export default function AccessRoleDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const roleId = params.id;
  const permission = useAccessAdminPermissions();
  const [eventPage, setEventPage] = React.useState(1);
  const [editOpen, setEditOpen] = React.useState(false);
  const [cloneOpen, setCloneOpen] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [assignOpen, setAssignOpen] = React.useState(false);
  const [unassigning, setUnassigning] = React.useState<ManagedRoleAssignment | null>(null);
  const roleQuery = useQuery({
    queryKey: accessAdminKeys.role(roleId),
    queryFn: () => api.access.getRole(roleId),
    enabled: permission.canRead,
  });
  const catalogueQuery = useQuery({
    queryKey: accessAdminKeys.permissions,
    queryFn: () => api.access.permissionCatalogue(),
    enabled: permission.canRead,
    staleTime: 10 * 60_000,
  });
  const assignmentsQuery = useQuery({
    queryKey: accessAdminKeys.assignments(roleId),
    queryFn: () => api.access.listRoleAssignments(roleId),
    enabled: permission.canRead,
  });
  const eventsQuery = useQuery({
    queryKey: accessAdminKeys.events(roleId, eventPage),
    queryFn: () => api.access.listRoleEvents(roleId, { page: eventPage, page_size: PAGE_SIZE }),
    enabled: permission.canRead,
  });
  const role = roleQuery.data;
  const catalogue = catalogueQuery.data?.data ?? [];

  if (permission.isLoading) return <LoadingState />;
  if (permission.isError) {
    return <ErrorCard title="Access could not be checked" message="Your current permissions could not be loaded." onRetry={() => void permission.retry()} />;
  }
  if (!permission.canRead) {
    return <UnavailableCard />;
  }
  if (roleQuery.isLoading) return <LoadingState />;
  if (roleQuery.isError || !role) {
    return <ErrorCard title="Role unavailable" message={formatAccessError(roleQuery.error, 'This role could not be loaded or no longer exists.')} onRetry={() => void roleQuery.refetch()} />;
  }

  const groupedPermissions = groupPermissions(role.permissions);
  const hasAdminGrant = roleHasPermission(role, 'settings', 'configure');
  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <Button asChild variant="ghost" size="sm" className="-ml-3 mb-2">
            <Link href="/settings/access-policies"><ArrowLeft aria-hidden="true" className="mr-2 h-4 w-4" />All roles</Link>
          </Button>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="break-words text-3xl font-bold tracking-tight">{role.name}</h1>
            <Badge variant={role.is_system_role ? 'secondary' : 'outline'}>{role.is_system_role ? 'System role' : 'Custom role'}</Badge>
            <Badge variant="outline">v{role.version}</Badge>
          </div>
          <p className="mt-1 break-all font-mono text-sm text-muted-foreground">{role.slug}</p>
          {role.description && <p className="mt-2 max-w-3xl text-muted-foreground">{role.description}</p>}
        </div>
        {permission.canConfigure && (
          <div className="flex flex-wrap gap-2">
            {!role.is_system_role && <Button type="button" variant="outline" onClick={() => setEditOpen(true)}><Pencil aria-hidden="true" className="mr-2 h-4 w-4" />Edit</Button>}
            <Button type="button" variant="outline" onClick={() => setCloneOpen(true)}><Copy aria-hidden="true" className="mr-2 h-4 w-4" />Clone</Button>
            {!role.is_system_role && <Button type="button" variant="destructive" onClick={() => setDeleteOpen(true)}><Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />Delete</Button>}
          </div>
        )}
      </div>

      {role.is_system_role && (
        <Card className="border-blue-500/40 bg-blue-500/5">
          <CardContent className="flex gap-3 py-4 text-sm">
            <LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-blue-700" />
            <span>System roles are managed by the platform and cannot be edited or deleted. Clone this role to customize its permissions for your organization.</span>
          </CardContent>
        </Card>
      )}
      {!permission.canConfigure && (
        <Card><CardContent className="flex gap-3 py-4 text-sm text-muted-foreground"><LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />You have read-only settings access. Role and assignment changes are hidden.</CardContent></Card>
      )}
      {hasAdminGrant && (
        <Card className="border-amber-500/40 bg-amber-500/5">
          <CardContent className="flex gap-3 py-4 text-sm">
            <ShieldCheck aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" />
            <span>This role grants settings administration. The platform prevents permission or assignment changes that would remove the organization’s final administrator.</span>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Metric label="Permissions" value={role.permissions.length} />
        <Metric label="Assigned users" value={role.assigned_users} />
        <Metric label="Created" value={formatDate(role.created_at)} />
        <Metric label="Last updated" value={formatDate(role.updated_at)} />
      </div>

      <Tabs defaultValue="permissions" className="space-y-5">
        <TabsList className="h-auto max-w-full flex-wrap justify-start">
          <TabsTrigger value="permissions">Permissions</TabsTrigger>
          <TabsTrigger value="assignments">Assignments ({role.assigned_users})</TabsTrigger>
          <TabsTrigger value="history">Change history</TabsTrigger>
        </TabsList>
        <TabsContent value="permissions">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Effective role permissions</CardTitle>
              <CardDescription>Permissions inherited through other roles are not shown here.</CardDescription>
            </CardHeader>
            <CardContent>
              {groupedPermissions.length === 0 ? (
                <EmptyState icon={ShieldCheck} title="No permissions" description="This role currently grants no capabilities." />
              ) : (
                <div className="grid gap-4 lg:grid-cols-2">
                  {groupedPermissions.map((group) => (
                    <section key={group.resource} className="rounded-md border p-4" aria-labelledby={`role-resource-${group.resource}`}>
                      <h2 id={`role-resource-${group.resource}`} className="font-semibold">{humanizeAccessToken(group.resource)}</h2>
                      <ul className="mt-3 space-y-2">
                        {group.permissions.map((item) => (
                          <li key={permissionKey(item)} className="flex items-start justify-between gap-3 text-sm">
                            <span className="font-medium">{humanizeAccessToken(item.action)}</span>
                            <span className="text-right text-xs text-muted-foreground">{item.description || permissionKey(item)}</span>
                          </li>
                        ))}
                      </ul>
                    </section>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="assignments">
          <AssignmentsPanel
            canConfigure={permission.canConfigure}
            isError={assignmentsQuery.isError}
            isLoading={assignmentsQuery.isLoading}
            items={assignmentsQuery.data?.data ?? []}
            onAssign={() => setAssignOpen(true)}
            onRetry={() => void assignmentsQuery.refetch()}
            onUnassign={setUnassigning}
          />
        </TabsContent>
        <TabsContent value="history">
          <HistoryPanel
            isError={eventsQuery.isError}
            isLoading={eventsQuery.isLoading}
            items={eventsQuery.data?.data ?? []}
            onNext={() => setEventPage((current) => current + 1)}
            onPrevious={() => setEventPage((current) => Math.max(1, current - 1))}
            onRetry={() => void eventsQuery.refetch()}
            page={eventsQuery.data?.pagination.page ?? eventPage}
            totalPages={eventsQuery.data?.pagination.total_pages ?? 0}
          />
        </TabsContent>
      </Tabs>

      {editOpen && <RoleEditorDialog catalogue={catalogue} role={role} open onOpenChange={setEditOpen} onRefresh={() => { setEditOpen(false); void roleQuery.refetch(); }} onSaved={() => void roleQuery.refetch()} />}
      {cloneOpen && <RoleCloneDialog role={role} open onOpenChange={setCloneOpen} onCloned={(cloned) => router.push(`/settings/access-policies/${cloned.id}`)} />}
      {deleteOpen && <RoleDeleteDialog role={role} open onOpenChange={setDeleteOpen} onRefresh={() => void roleQuery.refetch()} onDeleted={() => router.push('/settings/access-policies')} />}
      {assignOpen && <RoleAssignDialog role={role} open onOpenChange={setAssignOpen} />}
      {unassigning && <RoleUnassignDialog role={role} assignment={unassigning} open onOpenChange={(next) => !next && setUnassigning(null)} />}
    </div>
  );
}

function AssignmentsPanel({
  canConfigure,
  isError,
  isLoading,
  items,
  onAssign,
  onRetry,
  onUnassign,
}: {
  canConfigure: boolean;
  isError: boolean;
  isLoading: boolean;
  items: ManagedRoleAssignment[];
  onAssign: () => void;
  onRetry: () => void;
  onUnassign: (assignment: ManagedRoleAssignment) => void;
}) {
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-4">
        <div><CardTitle className="text-lg">Assigned users</CardTitle><CardDescription>Direct role grants in this organization.</CardDescription></div>
        {canConfigure && <Button type="button" onClick={onAssign}><UserPlus aria-hidden="true" className="mr-2 h-4 w-4" />Assign user</Button>}
      </CardHeader>
      <CardContent>
        {isLoading ? <RowsSkeleton /> : isError ? (
          <InlineError message="Assignments could not be loaded." onRetry={onRetry} />
        ) : items.length === 0 ? (
          <EmptyState icon={Users} title="No direct assignments" description="No users currently receive this role directly." />
        ) : (
          <div className="divide-y rounded-md border">
            {items.map((assignment) => (
              <div key={assignment.user_id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <p className="truncate font-medium">{[assignment.first_name, assignment.last_name].filter(Boolean).join(' ') || assignment.email}</p>
                  <p className="truncate text-sm text-muted-foreground">{assignment.email}</p>
                  <p className="mt-1 text-xs text-muted-foreground">Assigned {formatDateTime(assignment.assigned_at)}</p>
                </div>
                {canConfigure && <Button type="button" size="sm" variant="outline" onClick={() => onUnassign(assignment)}><UserMinus aria-hidden="true" className="mr-2 h-4 w-4" />Remove</Button>}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function HistoryPanel({
  isError,
  isLoading,
  items,
  onNext,
  onPrevious,
  onRetry,
  page,
  totalPages,
}: {
  isError: boolean;
  isLoading: boolean;
  items: RoleChangeEvent[];
  onNext: () => void;
  onPrevious: () => void;
  onRetry: () => void;
  page: number;
  totalPages: number;
}) {
  return (
    <Card>
      <CardHeader><CardTitle className="text-lg">Append-only change history</CardTitle><CardDescription>Role changes and assignment reasons, newest first.</CardDescription></CardHeader>
      <CardContent>
        {isLoading ? <RowsSkeleton /> : isError ? <InlineError message="Role history could not be loaded." onRetry={onRetry} /> : items.length === 0 ? (
          <EmptyState icon={History} title="No history recorded" description="Recorded role lifecycle events will appear here." />
        ) : (
          <ol className="relative ml-2 space-y-6 border-l pl-6">
            {items.map((event) => (
              <li key={event.id} className="relative">
                <span aria-hidden="true" className="absolute -left-[1.93rem] top-1 h-3 w-3 rounded-full border-2 border-background bg-primary" />
                <div className="flex flex-wrap items-center gap-2"><Badge variant="outline">{humanizeAccessToken(event.event_type)}</Badge><span className="text-xs text-muted-foreground">Role v{event.role_version}</span></div>
                <p className="mt-2 text-sm">{event.reason || 'No reason was required for this lifecycle event.'}</p>
                {event.target_user_id && <p className="mt-1 break-all text-xs text-muted-foreground">Target user: {event.target_user_id}</p>}
                <p className="mt-1 break-all text-xs text-muted-foreground">Actor: {event.actor_user_id} · {formatDateTime(event.created_at)}</p>
              </li>
            ))}
          </ol>
        )}
        {totalPages > 1 && (
          <div className="mt-6 flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-muted-foreground">Page {page} of {totalPages}</p>
            <div className="flex gap-2">
              <Button type="button" size="sm" variant="outline" disabled={page <= 1 || isLoading} onClick={onPrevious}><ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />Previous</Button>
              <Button type="button" size="sm" variant="outline" disabled={page >= totalPages || isLoading} onClick={onNext}>Next<ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" /></Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function EmptyState({ description, icon: Icon, title }: { description: string; icon: React.ElementType; title: string }) {
  return <div className="flex flex-col items-center py-10 text-center"><Icon aria-hidden="true" className="h-9 w-9 text-muted-foreground" /><h3 className="mt-3 font-semibold">{title}</h3><p className="mt-1 text-sm text-muted-foreground">{description}</p></div>;
}

function ErrorCard({ message, onRetry, title }: { message: string; onRetry: () => void; title: string }) {
  return <Card className="mx-auto max-w-xl"><CardContent className="space-y-4 py-12 text-center"><RefreshCw aria-hidden="true" className="mx-auto h-9 w-9 text-muted-foreground" /><h1 className="text-xl font-semibold">{title}</h1><p role="alert" className="text-sm text-muted-foreground">{message}</p><Button type="button" variant="outline" onClick={onRetry}>Try again</Button></CardContent></Card>;
}

function InlineError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div className="space-y-3 py-10 text-center"><p role="alert" className="text-sm text-destructive">{message}</p><Button type="button" size="sm" variant="outline" onClick={onRetry}>Try again</Button></div>;
}

function LoadingState() {
  return <div role="status" aria-label="Loading role" className="space-y-5"><Skeleton className="h-24 w-full" /><div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-24" />)}</div><Skeleton className="h-80 w-full" /></div>;
}

function Metric({ label, value }: { label: string; value: number | string }) {
  return <Card><CardContent className="pt-6"><p className="text-sm text-muted-foreground">{label}</p><p className="mt-1 text-2xl font-semibold">{value}</p></CardContent></Card>;
}

function RowsSkeleton() {
  return <div role="status" aria-label="Loading records" className="space-y-3">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-16 w-full" />)}</div>;
}

function UnavailableCard() {
  return <Card className="mx-auto max-w-xl"><CardContent className="flex flex-col items-center gap-3 py-12 text-center"><LockKeyhole aria-hidden="true" className="h-9 w-9 text-muted-foreground" /><h1 className="text-xl font-semibold">Access administration unavailable</h1><p className="text-sm text-muted-foreground">Your role does not grant organization settings access.</p></CardContent></Card>;
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(value));
}

function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
