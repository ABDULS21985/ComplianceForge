'use client';

import { useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { z } from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  AlertCircle,
  Building2,
  Users,
  ShieldCheck,
  ScrollText,
  Search,
  Plus,
  Edit,
  UserX,
  ChevronLeft,
  ChevronRight,
  Clock,
  Activity,
  Globe,
  Lock,
} from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';

import { cn } from '@/lib/utils';
import { formatDateTime, getStatusColor } from '@/lib/utils';
import {
  useOrganization,
  useUpdateOrganization,
  useUsers,
  useCreateUser,
  useDeactivateUser,
  useRoles,
  useAuditLog,
} from '@/lib/api-hooks';
import type { Organization, User as UserType, Role, AuditLogEntry } from '@/types';
import type { PaginatedResponse } from '@/lib/api';

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

const updateOrgSchema = z.object({
  name: z.string().min(1, 'Organisation name is required').max(200),
  legal_name: z.string().max(200).optional(),
  industry: z.string().max(100).optional(),
  country_code: z.string().max(10).optional(),
  timezone: z.string().max(50).optional(),
  employee_count_range: z.string().max(50).optional(),
});

type UpdateOrgValues = z.infer<typeof updateOrgSchema>;

// ---------------------------------------------------------------------------
// Organisation Tab
// ---------------------------------------------------------------------------

function OrganisationTab() {
  const [editing, setEditing] = useState(false);
  const orgQuery = useOrganization();
  const updateOrg = useUpdateOrganization();
  const org = orgQuery.data as Organization | undefined;

  const form = useForm<UpdateOrgValues>({
    resolver: zodResolver(updateOrgSchema),
    values: {
      name: org?.name ?? '',
      legal_name: org?.legal_name ?? '',
      industry: org?.industry ?? '',
      country_code: org?.country_code ?? '',
      timezone: org?.timezone ?? '',
      employee_count_range: org?.employee_count_range ?? '',
    },
  });

  const onSubmit = (values: UpdateOrgValues) => {
    updateOrg.mutate(values, {
      onSuccess: () => setEditing(false),
    });
  };

  if (orgQuery.isLoading) {
    return (
      <Card>
        <CardContent className="p-6">
          <div className="animate-pulse space-y-4">
            <div className="h-6 w-48 rounded bg-muted" />
            <div className="h-4 w-full rounded bg-muted" />
            <div className="h-4 w-3/4 rounded bg-muted" />
          </div>
        </CardContent>
      </Card>
    );
  }

  if (orgQuery.error) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 p-6 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>Failed to load organisation details.</span>
        </CardContent>
      </Card>
    );
  }

  if (!org) return null;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle className="text-lg">Organisation Details</CardTitle>
          <CardDescription>Manage your organisation profile and settings.</CardDescription>
        </div>
        {!editing && (
          <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
            <Edit className="mr-2 h-4 w-4" />
            Edit
          </Button>
        )}
      </CardHeader>
      <CardContent>
        {editing ? (
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="org-name">Organisation Name *</Label>
                <Input id="org-name" {...form.register('name')} />
                {form.formState.errors.name && (
                  <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="org-legal">Legal Name</Label>
                <Input id="org-legal" {...form.register('legal_name')} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="org-industry">Industry</Label>
                <Input id="org-industry" {...form.register('industry')} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="org-country">Country Code</Label>
                <Input id="org-country" {...form.register('country_code')} placeholder="GB" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="org-tz">Timezone</Label>
                <Input id="org-tz" {...form.register('timezone')} placeholder="Europe/London" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="org-emp">Employee Count Range</Label>
                <Input id="org-emp" {...form.register('employee_count_range')} placeholder="50-249" />
              </div>
            </div>
            <div className="flex gap-3 pt-2">
              <Button type="submit" disabled={updateOrg.isPending}>
                {updateOrg.isPending ? 'Saving...' : 'Save Changes'}
              </Button>
              <Button type="button" variant="outline" onClick={() => setEditing(false)}>
                Cancel
              </Button>
            </div>
          </form>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <InfoItem icon={Building2} label="Name" value={org.name} />
            <InfoItem icon={Building2} label="Legal Name" value={org.legal_name} />
            <InfoItem icon={Globe} label="Industry" value={org.industry} />
            <InfoItem icon={Globe} label="Country" value={org.country_code} />
            <InfoItem icon={Clock} label="Timezone" value={org.timezone} />
            <InfoItem icon={Users} label="Employee Range" value={org.employee_count_range} />
            <InfoItem icon={ShieldCheck} label="Tier" value={org.tier} />
            <InfoItem icon={Activity} label="Status" value={org.status} />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function InfoItem({
  icon: Icon,
  label,
  value,
}: {
  icon: React.ElementType;
  label: string;
  value?: string | null;
}) {
  return (
    <div className="flex items-start gap-3 py-2">
      <Icon className="mt-0.5 h-4 w-4 flex-shrink-0 text-muted-foreground" />
      <div>
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <p className="mt-0.5 text-sm">{value || '—'}</p>
      </div>
    </div>
  );
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
      {usersQuery.isLoading ? (
        <Card>
          <CardContent className="p-6">
            <div className="animate-pulse space-y-4">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="flex gap-4">
                  <div className="h-4 w-1/4 rounded bg-muted" />
                  <div className="h-4 w-1/4 rounded bg-muted" />
                  <div className="h-4 w-1/6 rounded bg-muted" />
                  <div className="h-4 w-1/6 rounded bg-muted" />
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      ) : usersQuery.error ? (
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>Failed to load users.</span>
          </CardContent>
        </Card>
      ) : !usersData?.items?.length ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <Users className="h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">No users found</h3>
          </CardContent>
        </Card>
      ) : (
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
                  {usersData.items.map((user) => (
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

            {usersData.total_pages > 1 && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Page {usersData.page} of {usersData.total_pages}
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
                    disabled={page >= usersData.total_pages}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Roles Tab
// ---------------------------------------------------------------------------

function RolesTab() {
  const rolesQuery = useRoles();
  const roles = rolesQuery.data as Role[] | undefined;

  if (rolesQuery.isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <CardContent className="p-6">
              <div className="animate-pulse space-y-3">
                <div className="h-5 w-32 rounded bg-muted" />
                <div className="h-4 w-full rounded bg-muted" />
                <div className="h-3 w-20 rounded bg-muted" />
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  if (rolesQuery.error) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 p-6 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>Failed to load roles.</span>
        </CardContent>
      </Card>
    );
  }

  if (!roles?.length) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center justify-center p-12 text-center">
          <ShieldCheck className="h-12 w-12 text-muted-foreground/50" />
          <h3 className="mt-4 text-lg font-semibold">No roles configured</h3>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
      {roles.map((role) => (
        <Card key={role.id}>
          <CardContent className="p-6">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-2">
                <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                  <ShieldCheck className="h-5 w-5 text-primary" />
                </div>
                <div>
                  <h3 className="font-semibold">{role.name}</h3>
                  {role.is_system_role && (
                    <Badge variant="secondary" className="mt-0.5 text-xs">
                      <Lock className="mr-1 h-3 w-3" />
                      System
                    </Badge>
                  )}
                </div>
              </div>
            </div>
            {role.description && (
              <p className="mt-3 text-sm text-muted-foreground">{role.description}</p>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
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

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Lock className="h-4 w-4" />
        <span>Immutable audit trail &mdash; ISO 27001 A.8.15 compliant</span>
      </div>

      {auditLogQuery.isLoading ? (
        <Card>
          <CardContent className="p-6">
            <div className="animate-pulse space-y-4">
              {Array.from({ length: 8 }).map((_, i) => (
                <div key={i} className="flex gap-4">
                  <div className="h-4 w-1/6 rounded bg-muted" />
                  <div className="h-4 w-1/6 rounded bg-muted" />
                  <div className="h-4 w-1/6 rounded bg-muted" />
                  <div className="h-4 w-1/4 rounded bg-muted" />
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      ) : auditLogQuery.error ? (
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>Failed to load audit log.</span>
          </CardContent>
        </Card>
      ) : !auditData?.items?.length ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <ScrollText className="h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">No audit entries</h3>
          </CardContent>
        </Card>
      ) : (
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
                  {auditData.items.map((entry) => {
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

            {auditData.total_pages > 1 && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Page {auditData.page} of {auditData.total_pages}
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
                    disabled={page >= auditData.total_pages}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}
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
        <TabsList>
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

        <TabsContent value="audit-log">
          <AuditLogTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
