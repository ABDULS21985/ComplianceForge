'use client';

import type { AuditLogEntry, User as UserType } from '@/types';
import {
  Building2,
  ChevronLeft,
  ChevronRight,
  Gauge,
  Lock,
  Plus,
  ScrollText,
  Search,
  ShieldCheck,
  Stethoscope,
  Users,
  UserX,
} from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { formatDateTime, getStatusColor } from '@/lib/utils';
import { ResourceBoundary, StaleDataNotice } from '@/components/data/resource-state';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  useAuditLog,
  useCreateUser,
  useDeactivateUser,
  useUsers,
} from '@/lib/api-hooks';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import Link from 'next/link';
import { OrganizationProfilePanel } from '@/components/settings/organization-profile-panel';
import type { PaginatedResponse } from '@/lib/api';
import { useForm } from 'react-hook-form';
import { useSearchParams } from 'next/navigation';
import { useState } from 'react';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';

// ---------------------------------------------------------------------------
// Schemas
// ---------------------------------------------------------------------------

const createUserSchema = z.object({
  email: z.string().email('Must be a valid email'),
  first_name: z.string().min(1, 'First name is required').max(100),
  last_name: z.string().min(1, 'Last name is required').max(100),
  job_title: z.string().max(100).optional(),
  department: z.string().max(100).optional(),
  password: z.string().min(8, 'Password must be at least 8 characters'),
});

type CreateUserValues = z.infer<typeof createUserSchema>;

function OrganisationTab() {
  return <OrganizationProfilePanel />;
}

// ---------------------------------------------------------------------------
// Users Tab
// ---------------------------------------------------------------------------

function UsersTab() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [addOpen, setAddOpen] = useState(false);
  const [deactivateId, setDeactivateId] = useState<string | null>(null);

  const usersQuery = useUsers({ page, page_size: 20, search: search || undefined });
  const createUser = useCreateUser();
  const deactivateUser = useDeactivateUser();

  const usersData = usersQuery.data as PaginatedResponse<UserType> | undefined;
  const users = usersData?.items ?? [];
  const usersPage = usersData?.page ?? page;
  const usersTotalPages = usersData?.total_pages ?? 1;

  const form = useForm<CreateUserValues>({
    resolver: zodResolver(createUserSchema),
    defaultValues: {
      email: '',
      first_name: '',
      last_name: '',
      job_title: '',
      department: '',
      password: '',
    },
  });

  const onCreateUser = (values: CreateUserValues) => {
    createUser.mutate(values, {
      onSuccess: () => {
        setAddOpen(false);
        form.reset();
      },
    });
  };

  const confirmDeactivate = () => {
    if (deactivateId) {
      deactivateUser.mutate(deactivateId, {
        onSuccess: () => setDeactivateId(null),
      });
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search users..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
            className="pl-10"
          />
        </div>
        <Dialog open={addOpen} onOpenChange={setAddOpen}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="mr-2 h-4 w-4" />
              Add User
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add New User</DialogTitle>
              <DialogDescription>
                Create a new user account. They will receive login credentials.
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={form.handleSubmit(onCreateUser)} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="first_name">First Name *</Label>
                  <Input id="first_name" {...form.register('first_name')} />
                  {form.formState.errors.first_name && (
                    <p className="text-xs text-destructive">{form.formState.errors.first_name.message}</p>
                  )}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="last_name">Last Name *</Label>
                  <Input id="last_name" {...form.register('last_name')} />
                  {form.formState.errors.last_name && (
                    <p className="text-xs text-destructive">{form.formState.errors.last_name.message}</p>
                  )}
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="user-email">Email *</Label>
                <Input id="user-email" type="email" {...form.register('email')} />
                {form.formState.errors.email && (
                  <p className="text-xs text-destructive">{form.formState.errors.email.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="user-password">Password *</Label>
                <Input id="user-password" type="password" {...form.register('password')} />
                {form.formState.errors.password && (
                  <p className="text-xs text-destructive">{form.formState.errors.password.message}</p>
                )}
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="job_title">Job Title</Label>
                  <Input id="job_title" {...form.register('job_title')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="department">Department</Label>
                  <Input id="department" {...form.register('department')} />
                </div>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setAddOpen(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={createUser.isPending}>
                  {createUser.isPending ? 'Creating...' : 'Create User'}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      {Boolean(usersQuery.error) && Boolean(usersQuery.data) && (
        <StaleDataNotice
          isRefreshing={usersQuery.isFetching}
          lastUpdatedAt={usersQuery.dataUpdatedAt}
          onRefresh={() => void usersQuery.refetch()}
        />
      )}

      {/* Deactivate Confirmation Dialog */}
      <Dialog open={!!deactivateId} onOpenChange={(open) => !open && setDeactivateId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Deactivate User</DialogTitle>
            <DialogDescription>
              Are you sure you want to deactivate this user? They will lose access to the system
              immediately. This action can be reversed by an administrator.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeactivateId(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={confirmDeactivate}
              disabled={deactivateUser.isPending}
            >
              {deactivateUser.isPending ? 'Deactivating...' : 'Deactivate'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Users Table */}
      <ResourceBoundary
        isEmpty={users.length === 0}
        isError={Boolean(usersQuery.error) && !usersQuery.data}
        isLoading={usersQuery.isLoading}
        loadingLayout="table"
        loadingTitle="Loading users"
        emptyTitle={search ? 'No users match this search' : 'No users found'}
        emptyDescription={
          search
            ? 'Clear the search or use a broader name or email address.'
            : 'Add a user to begin assigning governance responsibilities.'
        }
        emptyAction={
          search ? (
            <Button type="button" variant="outline" onClick={() => setSearch('')}>
              Clear search
            </Button>
          ) : (
            <Button type="button" onClick={() => setAddOpen(true)}>
              Add user
            </Button>
          )
        }
        errorTitle="Users could not be loaded"
        errorDescription="The directory service did not return the user list. Retry without losing your search."
        onRetry={() => void usersQuery.refetch()}
        retrying={usersQuery.isFetching}
      >
        <Card>
          <CardContent className="pt-6">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left">
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Name</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Email</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Department</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Roles</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Status</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Last Login</th>
                    <th className="pb-3 font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr key={user.id} className="border-b last:border-0 hover:bg-muted/50">
                      <td className="py-3 pr-4 font-medium">
                        {user.first_name} {user.last_name}
                      </td>
                      <td className="py-3 pr-4 text-muted-foreground">{user.email}</td>
                      <td className="py-3 pr-4 text-muted-foreground">{user.department ?? '—'}</td>
                      <td className="py-3 pr-4">
                        <div className="flex flex-wrap gap-1">
                          {user.roles?.map((role) => (
                            <Badge key={role.id} variant="outline" className="text-xs">
                              {role.name}
                            </Badge>
                          )) ?? <span className="text-muted-foreground">—</span>}
                        </div>
                      </td>
                      <td className="py-3 pr-4">
                        <Badge className={getStatusColor(user.status)}>
                          {user.status}
                        </Badge>
                      </td>
                      <td className="py-3 pr-4 text-xs text-muted-foreground">
                        {formatDateTime(user.last_login_at)}
                      </td>
                      <td className="py-3">
                        {user.status === 'active' && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setDeactivateId(user.id)}
                            className="text-destructive hover:text-destructive"
                          >
                            <UserX className="h-4 w-4" />
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {usersTotalPages > 1 && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Page {usersPage} of {usersTotalPages}
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    <ChevronLeft className="h-4 w-4" />
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => p + 1)}
                    disabled={page >= usersTotalPages}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </ResourceBoundary>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Roles Tab
// ---------------------------------------------------------------------------

function RolesTab() {
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-4 p-12 text-center">
        <div className="rounded-full bg-primary/10 p-3"><ShieldCheck aria-hidden="true" className="h-8 w-8 text-primary" /></div>
        <div><h3 className="text-lg font-semibold">Enterprise role administration</h3><p className="mt-1 max-w-xl text-sm text-muted-foreground">Review the canonical permission catalogue, manage custom and system roles, assess permission impact, assign users, and inspect append-only change history.</p></div>
        <Button asChild><Link href="/settings/access-policies">Manage roles and permissions</Link></Button>
      </CardContent>
    </Card>
  );
}

function CapabilitiesTab() {
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-4 p-12 text-center">
        <div className="rounded-full bg-primary/10 p-3">
          <Gauge aria-hidden="true" className="h-8 w-8 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Capabilities and plan entitlements</h3>
          <p className="mt-1 max-w-xl text-sm text-muted-foreground">
            Review live feature evaluations, plan features and resource limits, schedule audited
            tenant overrides, and inspect immutable change history.
          </p>
        </div>
        <Button asChild>
          <Link href="/settings/capabilities">View capabilities and usage</Link>
        </Button>
      </CardContent>
    </Card>
  );
}

function DiagnosticsTab() {
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-4 p-12 text-center">
        <div className="rounded-full bg-primary/10 p-3">
          <Stethoscope aria-hidden="true" className="h-8 w-8 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Administrator diagnostics</h3>
          <p className="mt-1 max-w-xl text-sm text-muted-foreground">
            Review tenant-scoped dependencies, migrations, worker backlogs, connector health, and
            safe configuration posture without exposing secrets.
          </p>
        </div>
        <Button asChild>
          <Link href="/settings/diagnostics">Open diagnostics centre</Link>
        </Button>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Audit Log Tab
// ---------------------------------------------------------------------------

const ACTION_COLORS: Record<string, string> = {
  create: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
  update: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
  delete: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
  login: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/30 dark:text-cyan-400',
  logout: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400',
  publish: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400',
  approve: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-400',
  reject: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400',
};

function AuditLogTab() {
  const [page, setPage] = useState(1);
  const auditLogQuery = useAuditLog({ page, page_size: 25 });
  const auditData = auditLogQuery.data as PaginatedResponse<AuditLogEntry> | undefined;
  const auditEntries = auditData?.items ?? [];
  const auditPage = auditData?.page ?? page;
  const auditTotalPages = auditData?.total_pages ?? 1;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Lock className="h-4 w-4" />
        <span>Immutable audit trail &mdash; ISO 27001 A.8.15 compliant</span>
      </div>

      {Boolean(auditLogQuery.error) && Boolean(auditLogQuery.data) && (
        <StaleDataNotice
          isRefreshing={auditLogQuery.isFetching}
          lastUpdatedAt={auditLogQuery.dataUpdatedAt}
          onRefresh={() => void auditLogQuery.refetch()}
        />
      )}

      <ResourceBoundary
        isEmpty={auditEntries.length === 0}
        isError={Boolean(auditLogQuery.error) && !auditLogQuery.data}
        isLoading={auditLogQuery.isLoading}
        loadingLayout="table"
        loadingTitle="Loading audit log"
        emptyTitle="No audit entries"
        emptyDescription="Recorded administrative activity will appear in this immutable trail."
        errorTitle="Audit log could not be loaded"
        errorDescription="The service did not return audit history. Retry without leaving settings."
        onRetry={() => void auditLogQuery.refetch()}
        retrying={auditLogQuery.isFetching}
      >
        <Card>
          <CardContent className="pt-6">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left">
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Timestamp</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">User</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Action</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Entity</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Details</th>
                    <th className="pb-3 font-medium text-muted-foreground">IP Address</th>
                  </tr>
                </thead>
                <tbody>
                  {auditEntries.map((entry) => {
                    const actionBase = entry.action.split('.')[0] ?? entry.action;
                    return (
                      <tr key={entry.id} className="border-b last:border-0 hover:bg-muted/50">
                        <td className="py-3 pr-4 text-xs text-muted-foreground whitespace-nowrap">
                          {formatDateTime(entry.created_at)}
                        </td>
                        <td className="py-3 pr-4">{entry.user_name ?? 'System'}</td>
                        <td className="py-3 pr-4">
                          <Badge className={cn('capitalize', ACTION_COLORS[actionBase] ?? 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400')}>
                            {entry.action.replace(/_/g, ' ')}
                          </Badge>
                        </td>
                        <td className="py-3 pr-4 text-muted-foreground capitalize">
                          {entry.entity_type.replace(/_/g, ' ')}
                        </td>
                        <td className="py-3 pr-4 max-w-xs truncate text-xs text-muted-foreground">
                          {entry.changes
                            ? Object.keys(entry.changes).join(', ')
                            : entry.entity_id ?? '—'}
                        </td>
                        <td className="py-3 font-mono text-xs text-muted-foreground">
                          {entry.ip_address ?? '—'}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            {auditTotalPages > 1 && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Page {auditPage} of {auditTotalPages}
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    <ChevronLeft className="h-4 w-4" />
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => p + 1)}
                    disabled={page >= auditTotalPages}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </ResourceBoundary>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function SettingsPage() {
  const searchParams = useSearchParams();
  const defaultTab = searchParams.get('tab') ?? 'organisation';

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
        <p className="text-muted-foreground">
          Manage your organisation, users, roles, and view the audit trail.
        </p>
      </div>

      {/* Tabs */}
      <Tabs defaultValue={defaultTab} className="space-y-6">
        <TabsList className="h-auto max-w-full flex-wrap justify-start">
          <TabsTrigger value="organisation" className="gap-2">
            <Building2 className="h-4 w-4" />
            Organisation
          </TabsTrigger>
          <TabsTrigger value="users" className="gap-2">
            <Users className="h-4 w-4" />
            Users
          </TabsTrigger>
          <TabsTrigger value="roles" className="gap-2">
            <ShieldCheck className="h-4 w-4" />
            Roles
          </TabsTrigger>
          <TabsTrigger value="capabilities" className="gap-2">
            <Gauge className="h-4 w-4" />
            Capabilities
          </TabsTrigger>
          <TabsTrigger value="diagnostics" className="gap-2">
            <Stethoscope className="h-4 w-4" />
            Diagnostics
          </TabsTrigger>
          <TabsTrigger value="audit-log" className="gap-2">
            <ScrollText className="h-4 w-4" />
            Audit Log
          </TabsTrigger>
        </TabsList>

        <TabsContent value="organisation">
          <OrganisationTab />
        </TabsContent>

        <TabsContent value="users">
          <UsersTab />
        </TabsContent>

        <TabsContent value="roles">
          <RolesTab />
        </TabsContent>

        <TabsContent value="capabilities">
          <CapabilitiesTab />
        </TabsContent>

        <TabsContent value="diagnostics">
          <DiagnosticsTab />
        </TabsContent>

        <TabsContent value="audit-log">
          <AuditLogTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
