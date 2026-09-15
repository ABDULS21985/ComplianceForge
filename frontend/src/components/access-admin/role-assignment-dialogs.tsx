'use client';

import * as React from 'react';
import {
  accessAdminKeys,
  formatAccessError,
  roleHasPermission,
} from '@/lib/access-admin';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Loader2, UserMinus, UserPlus } from 'lucide-react';
import type {
  ManagedRole,
  ManagedRoleAssignment,
  ManagedRoleAssignmentInput,
  ManagedRoleUnassignmentInput,
} from '@/types/access-admin';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import type { DirectoryUser } from '@/types/directory';
import { DirectoryUserPicker } from '@/components/access-admin/directory-user-picker';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

export function RoleAssignDialog({
  onOpenChange,
  open,
  role,
}: {
  onOpenChange: (open: boolean) => void;
  open: boolean;
  role: ManagedRole;
}) {
  const queryClient = useQueryClient();
  const [selectedUser, setSelectedUser] = React.useState<DirectoryUser | null>(null);
  const [reason, setReason] = React.useState('');
  const [validationError, setValidationError] = React.useState('');
  const mutation = useMutation({
    mutationFn: (input: ManagedRoleAssignmentInput) => api.access.assignRole(role.id, input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: accessAdminKeys.all });
      toast.success('Role assigned.');
      onOpenChange(false);
    },
  });

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedUser) {
      setValidationError('Select an active user from the directory.');
      return;
    }
    if (reason.trim().length < 3 || reason.trim().length > 1000) {
      setValidationError('Reason must contain 3–1,000 characters.');
      return;
    }
    setValidationError('');
    await mutation.mutateAsync({ user_id: selectedUser.id, reason: reason.trim() }).catch(() => undefined);
  }

  const error = validationError || (mutation.error ? formatAccessError(mutation.error, 'The role could not be assigned.') : '');
  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Assign {role.name}</DialogTitle>
          <DialogDescription>
            Assign this role to an active user in your organization. A reason is permanently recorded in role history.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={(event) => void submit(event)} noValidate>
          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          <div role="group" aria-labelledby="role-assignment-user-label" className="space-y-2">
            <Label id="role-assignment-user-label">User *</Label>
            <DirectoryUserPicker
              disabled={mutation.isPending}
              value={selectedUser}
              onChange={(user) => {
                setSelectedUser(user);
                if (user) setValidationError('');
              }}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="role-assignment-reason">Business reason *</Label>
            <Textarea id="role-assignment-reason" maxLength={1000} rows={3} value={reason} onChange={(event) => setReason(event.target.value)} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" /> : <UserPlus aria-hidden="true" className="mr-2 h-4 w-4" />}
              Assign role
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function RoleUnassignDialog({
  assignment,
  onOpenChange,
  open,
  role,
}: {
  assignment: ManagedRoleAssignment;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  role: ManagedRole;
}) {
  const queryClient = useQueryClient();
  const [reason, setReason] = React.useState('');
  const [validationError, setValidationError] = React.useState('');
  const mutation = useMutation({
    mutationFn: (input: ManagedRoleUnassignmentInput) =>
      api.access.unassignRole(role.id, assignment.user_id, input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: accessAdminKeys.all });
      toast.success('Role assignment removed.');
      onOpenChange(false);
    },
  });
  const grantsAdministration = roleHasPermission(role, 'settings', 'configure');

  async function remove() {
    if (reason.trim().length < 3 || reason.trim().length > 1000) {
      setValidationError('Reason must contain 3–1,000 characters.');
      return;
    }
    setValidationError('');
    await mutation.mutateAsync({ reason: reason.trim() }).catch(() => undefined);
  }

  const error = validationError || (mutation.error ? formatAccessError(mutation.error, 'The assignment could not be removed.') : '');
  return (
    <AlertDialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove role assignment?</AlertDialogTitle>
          <AlertDialogDescription>
            {assignment.email} will lose all permissions granted by {role.name}. The removal and reason are permanently recorded.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {grantsAdministration && (
          <p className="rounded-md bg-amber-500/10 p-3 text-sm text-amber-950 dark:text-amber-100">
            This role grants settings administration. Assign another administrator first if this is the organization’s final administrative grant; the API prevents lockout.
          </p>
        )}
        {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
        <div className="space-y-2">
          <Label htmlFor="role-unassignment-reason">Business reason *</Label>
          <Textarea id="role-unassignment-reason" autoFocus maxLength={1000} rows={3} value={reason} onChange={(event) => setReason(event.target.value)} />
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={mutation.isPending}>Keep assignment</AlertDialogCancel>
          <Button type="button" variant="destructive" disabled={mutation.isPending || reason.trim().length < 3} onClick={() => void remove()}>
            {mutation.isPending ? <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" /> : <UserMinus aria-hidden="true" className="mr-2 h-4 w-4" />}
            Remove assignment
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
