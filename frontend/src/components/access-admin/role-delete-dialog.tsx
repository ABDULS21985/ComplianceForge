'use client';

import * as React from 'react';
import {
  accessAdminKeys,
  formatAccessError,
  isAccessConflict,
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
import { useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import type { ManagedRole } from '@/types/access-admin';
import { toast } from 'sonner';

export function RoleDeleteDialog({
  onDeleted,
  onOpenChange,
  onRefresh,
  open,
  role,
}: {
  onDeleted: () => void;
  onOpenChange: (open: boolean) => void;
  onRefresh: () => void;
  open: boolean;
  role: ManagedRole;
}) {
  const queryClient = useQueryClient();
  const [confirmation, setConfirmation] = React.useState('');
  const mutation = useMutation({
    mutationFn: () => api.access.deleteRole(role.id, role.version),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: accessAdminKeys.all });
      toast.success('Custom role deleted.');
      onDeleted();
      onOpenChange(false);
    },
  });
  const isBlocked = role.is_system_role || role.assigned_users > 0;

  async function remove() {
    if (confirmation !== role.name || isBlocked) return;
    await mutation.mutateAsync().catch(() => undefined);
  }

  return (
    <AlertDialog open={open} onOpenChange={(next) => {
      if (mutation.isPending) return;
      if (!next) setConfirmation('');
      onOpenChange(next);
    }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete custom role?</AlertDialogTitle>
          <AlertDialogDescription>
            This soft-deletes the role using version {role.version}. Its append-only change history remains available to authorized auditors.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {role.is_system_role && <p role="alert" className="rounded-md bg-amber-500/10 p-3 text-sm">System roles are immutable. Clone this role if you need a tenant-owned variant.</p>}
        {!role.is_system_role && role.assigned_users > 0 && <p role="alert" className="rounded-md bg-amber-500/10 p-3 text-sm">Remove or transfer all {role.assigned_users} user assignments before deleting this role.</p>}
        {!isBlocked && (
          <div className="space-y-2">
            <Label htmlFor="role-delete-confirmation">Type <span className="font-semibold">{role.name}</span> to confirm</Label>
            <Input id="role-delete-confirmation" autoComplete="off" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />
          </div>
        )}
        {mutation.error && (
          <div role="alert" className="space-y-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            <p>{formatAccessError(mutation.error, 'The role could not be deleted.')}</p>
            {isAccessConflict(mutation.error) && <Button type="button" size="sm" variant="outline" onClick={onRefresh}>Reload latest role</Button>}
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={mutation.isPending}>Keep role</AlertDialogCancel>
          {!isBlocked && (
            <Button type="button" variant="destructive" disabled={mutation.isPending || confirmation !== role.name} onClick={() => void remove()}>
              {mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
              Delete role
            </Button>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
