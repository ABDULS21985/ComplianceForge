'use client';

import * as React from 'react';
import { accessAdminKeys, formatAccessError } from '@/lib/access-admin';
import { Copy, Loader2 } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { ManagedRole, ManagedRoleCloneInput } from '@/types/access-admin';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

export function RoleCloneDialog({
  onCloned,
  onOpenChange,
  open,
  role,
}: {
  onCloned: (role: ManagedRole) => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  role: ManagedRole;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = React.useState(`${role.name} copy`);
  const [slug, setSlug] = React.useState('');
  const [description, setDescription] = React.useState(role.description ?? '');
  const [validationError, setValidationError] = React.useState('');
  const mutation = useMutation({
    mutationFn: (input: ManagedRoleCloneInput) => api.access.cloneRole(role.id, input),
    onSuccess: async (cloned) => {
      await queryClient.invalidateQueries({ queryKey: accessAdminKeys.all });
      toast.success('Role cloned as a tenant-owned custom role.');
      onCloned(cloned);
      onOpenChange(false);
    },
  });

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalizedName = name.trim();
    const normalizedSlug = slug.trim();
    if (normalizedName.length < 2 || normalizedName.length > 100) {
      setValidationError('Name must contain 2–100 characters.');
      return;
    }
    if (normalizedSlug && (normalizedSlug.length < 2 || !/^[a-z0-9](?:[a-z0-9-]{0,98}[a-z0-9])?$/.test(normalizedSlug))) {
      setValidationError('Slug must contain 2–100 lowercase letters, numbers, or single hyphens.');
      return;
    }
    if (description.trim().length > 2000) {
      setValidationError('Description must not exceed 2,000 characters.');
      return;
    }
    setValidationError('');
    const input: ManagedRoleCloneInput = { name: normalizedName };
    if (normalizedSlug) input.slug = normalizedSlug;
    if (description.trim()) input.description = description.trim();
    await mutation.mutateAsync(input).catch(() => undefined);
  }

  const error = validationError || (mutation.error ? formatAccessError(mutation.error, 'The role could not be cloned.') : '');
  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Clone {role.name}</DialogTitle>
          <DialogDescription>
            Copies all {role.permissions.length} permissions into a new tenant-owned role. The source role is unchanged.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={(event) => void submit(event)} noValidate>
          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          <div className="space-y-2">
            <Label htmlFor="clone-role-name">New role name *</Label>
            <Input id="clone-role-name" autoFocus maxLength={100} value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="clone-role-slug">Slug</Label>
            <Input id="clone-role-slug" maxLength={100} placeholder="generated-from-name" value={slug} onChange={(event) => setSlug(event.target.value.toLowerCase())} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="clone-role-description">Description</Label>
            <Textarea id="clone-role-description" maxLength={2000} rows={3} value={description} onChange={(event) => setDescription(event.target.value)} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" /> : <Copy aria-hidden="true" className="mr-2 h-4 w-4" />}
              Clone role
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
