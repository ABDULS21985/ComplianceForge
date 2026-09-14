'use client';

import * as React from 'react';
import {
  accessAdminKeys,
  formatAccessError,
  groupPermissions,
  humanizeAccessToken,
  isAccessConflict,
  permissionKey,
  permissionSelection,
  removedPermissions,
  roleHasPermission,
} from '@/lib/access-admin';
import { AlertTriangle, Check, Loader2, ShieldCheck } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type {
  ManagedRole,
  ManagedRoleCreateInput,
  ManagedRoleImpact,
  ManagedRolePatch,
  PermissionGrant,
} from '@/types/access-admin';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

interface RoleEditorDialogProps {
  catalogue: PermissionGrant[];
  onOpenChange: (open: boolean) => void;
  onRefresh?: () => void;
  onSaved: (role: ManagedRole) => void;
  open: boolean;
  role?: ManagedRole;
}

interface Draft {
  description: string;
  name: string;
  selected: Set<string>;
  slug: string;
}

interface DraftErrors {
  description?: string;
  name?: string;
  permissions?: string;
  slug?: string;
}

function initialDraft(role?: ManagedRole): Draft {
  return {
    description: role?.description ?? '',
    name: role?.name ?? '',
    selected: new Set((role?.permissions ?? []).map(permissionKey)),
    slug: role?.slug ?? '',
  };
}

function validateDraft(draft: Draft, isEditing: boolean): DraftErrors {
  const errors: DraftErrors = {};
  const name = draft.name.trim();
  const slug = draft.slug.trim();
  if (name.length < 2 || name.length > 100) errors.name = 'Name must contain 2–100 characters.';
  if ((isEditing || slug) && (slug.length < 2 || !/^[a-z0-9](?:[a-z0-9-]{0,98}[a-z0-9])?$/.test(slug))) {
    errors.slug = 'Slug must contain 2–100 lowercase letters, numbers, or single hyphens.';
  }
  if (draft.description.trim().length > 2000) errors.description = 'Description must not exceed 2,000 characters.';
  if (draft.selected.size > 128) errors.permissions = 'A role can contain at most 128 permissions.';
  return errors;
}

function buildPermissions(catalogue: PermissionGrant[], selected: Set<string>): PermissionGrant[] {
  return permissionSelection(catalogue.filter((permission) => selected.has(permissionKey(permission))));
}

export function RoleEditorDialog({
  catalogue,
  onOpenChange,
  onRefresh,
  onSaved,
  open,
  role,
}: RoleEditorDialogProps) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = React.useState(() => initialDraft(role));
  const [errors, setErrors] = React.useState<DraftErrors>({});
  const [impact, setImpact] = React.useState<ManagedRoleImpact | null>(null);
  const [pendingPatch, setPendingPatch] = React.useState<ManagedRolePatch | null>(null);
  const grouped = React.useMemo(() => groupPermissions(catalogue), [catalogue]);

  const saveMutation = useMutation({
    mutationFn: async (input: ManagedRoleCreateInput | ManagedRolePatch) => {
      if (role) return api.access.updateRole(role.id, input as ManagedRolePatch);
      return api.access.createRole(input as ManagedRoleCreateInput);
    },
    onSuccess: async (saved) => {
      await queryClient.invalidateQueries({ queryKey: accessAdminKeys.all });
      toast.success(role ? 'Role updated.' : 'Custom role created.');
      onSaved(saved);
      onOpenChange(false);
    },
  });
  const impactMutation = useMutation({
    mutationFn: (permissions: PermissionGrant[]) =>
      api.access.previewRoleImpact(role?.id ?? '', permissions),
  });
  const isPending = saveMutation.isPending || impactMutation.isPending;
  const mutationError = saveMutation.error ?? impactMutation.error;

  function togglePermission(permission: PermissionGrant) {
    const key = permissionKey(permission);
    setDraft((current) => {
      const selected = new Set(current.selected);
      if (selected.has(key)) selected.delete(key);
      else selected.add(key);
      return { ...current, selected };
    });
    setImpact(null);
    setPendingPatch(null);
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (role?.is_system_role) return;
    const nextErrors = validateDraft(draft, Boolean(role));
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;
    const permissions = buildPermissions(catalogue, draft.selected);
    if (!role) {
      const input: ManagedRoleCreateInput = {
        name: draft.name.trim(),
        permissions,
      };
      if (draft.slug.trim()) input.slug = draft.slug.trim();
      if (draft.description.trim()) input.description = draft.description.trim();
      await saveMutation.mutateAsync(input).catch(() => undefined);
      return;
    }
    const patch: ManagedRolePatch = {
      expected_version: role.version,
      name: draft.name.trim(),
      permissions,
      slug: draft.slug.trim(),
    };
    if (draft.description.trim()) patch.description = draft.description.trim();
    else if (role.description) patch.clear_description = true;
    const removed = removedPermissions(role.permissions, permissions);
    if (removed.length > 0 && !pendingPatch) {
      try {
        const preview = await impactMutation.mutateAsync(permissions);
        setImpact(preview);
        setPendingPatch(patch);
      } catch {
        setImpact(null);
        setPendingPatch(null);
      }
      return;
    }
    await saveMutation.mutateAsync(pendingPatch ?? patch).catch(() => undefined);
  }

  const removesAdministrativeGrant = Boolean(
    role &&
    roleHasPermission(role, 'settings', 'configure') &&
    !draft.selected.has('settings:configure')
  );
  return (
    <Dialog open={open} onOpenChange={(next) => !isPending && onOpenChange(next)}>
      <DialogContent className="max-h-[92vh] max-w-5xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{role ? `Edit ${role.name}` : 'Create custom role'}</DialogTitle>
          <DialogDescription>
            Define a least-privilege role from the server permission catalogue. Permission removals require an impact review before saving.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-5" onSubmit={(event) => void submit(event)} noValidate>
          {mutationError && (
            <div role="alert" className="space-y-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <p>{formatAccessError(mutationError, 'The role could not be saved.')}</p>
              {role && isAccessConflict(mutationError) && onRefresh && (
                <Button type="button" size="sm" variant="outline" onClick={onRefresh}>Reload latest role</Button>
              )}
            </div>
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            <Field error={errors.name} htmlFor="role-name" label="Role name" required>
              <Input
                id="role-name"
                aria-describedby={errors.name ? 'role-name-error' : undefined}
                aria-invalid={Boolean(errors.name)}
                autoFocus
                maxLength={100}
                value={draft.name}
                onChange={(event) => { setDraft((current) => ({ ...current, name: event.target.value })); setImpact(null); setPendingPatch(null); }}
              />
            </Field>
            <Field error={errors.slug} htmlFor="role-slug" label="Slug">
              <Input
                id="role-slug"
                aria-describedby={errors.slug ? 'role-slug-error' : 'role-slug-help'}
                aria-invalid={Boolean(errors.slug)}
                maxLength={100}
                placeholder="generated-from-name"
                value={draft.slug}
                onChange={(event) => { setDraft((current) => ({ ...current, slug: event.target.value.toLowerCase() })); setImpact(null); setPendingPatch(null); }}
              />
              <p id="role-slug-help" className="text-xs text-muted-foreground">Leave blank when creating to generate it from the name.</p>
            </Field>
          </div>
          <Field error={errors.description} htmlFor="role-description" label="Description">
            <Textarea
              id="role-description"
              aria-describedby={errors.description ? 'role-description-error' : undefined}
              aria-invalid={Boolean(errors.description)}
              maxLength={2000}
              rows={3}
              value={draft.description}
              onChange={(event) => { setDraft((current) => ({ ...current, description: event.target.value })); setImpact(null); setPendingPatch(null); }}
            />
          </Field>
          <fieldset className="space-y-4">
            <legend className="text-sm font-semibold">Permission matrix ({draft.selected.size} selected)</legend>
            <p className="text-sm text-muted-foreground">Every checkbox maps to an exact resource and action enforced by the API.</p>
            {errors.permissions && <p role="alert" className="text-sm text-destructive">{errors.permissions}</p>}
            <div className="grid gap-4 lg:grid-cols-2">
              {grouped.map((group) => (
                <section key={group.resource} className="rounded-md border p-4" aria-labelledby={`permission-${group.resource}`}>
                  <h3 id={`permission-${group.resource}`} className="font-medium">{humanizeAccessToken(group.resource)}</h3>
                  <div className="mt-3 grid gap-2 sm:grid-cols-2">
                    {group.permissions.map((permission) => {
                      const key = permissionKey(permission);
                      return (
                        <label key={key} className="flex min-w-0 items-start gap-2 rounded-md p-2 hover:bg-muted">
                          <input
                            type="checkbox"
                            className="mt-0.5 h-4 w-4 shrink-0"
                            checked={draft.selected.has(key)}
                            onChange={() => togglePermission(permission)}
                          />
                          <span className="min-w-0 text-sm">
                            <span className="block font-medium">{humanizeAccessToken(permission.action)}</span>
                            {permission.description && <span className="block text-xs text-muted-foreground">{permission.description}</span>}
                          </span>
                        </label>
                      );
                    })}
                  </div>
                </section>
              ))}
            </div>
          </fieldset>
          {impact && pendingPatch && (
            <section role="alert" className="space-y-3 rounded-md border border-amber-500/50 bg-amber-500/10 p-4">
              <div className="flex gap-2">
                <AlertTriangle aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-amber-700" />
                <div>
                  <h3 className="font-semibold">Review permission impact</h3>
                  <p className="text-sm text-muted-foreground">
                    {impact.assigned_users} assigned {impact.assigned_users === 1 ? 'user' : 'users'} will move from {impact.current_permissions} to {impact.proposed_permissions} permissions.
                  </p>
                </div>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <ImpactList label="Added" permissions={impact.added} />
                <ImpactList label="Removed" permissions={impact.removed} />
              </div>
              {removesAdministrativeGrant && (
                <p className="text-sm font-medium text-amber-900 dark:text-amber-200">
                  This removes settings administration. The API will reject the change if it would leave the organization without an administrator.
                </p>
              )}
              <p className="text-sm font-medium">Submit again to confirm these permission removals.</p>
            </section>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={isPending} onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button type="submit" disabled={isPending || catalogue.length === 0}>
              {isPending ? <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" /> : impact ? <Check aria-hidden="true" className="mr-2 h-4 w-4" /> : <ShieldCheck aria-hidden="true" className="mr-2 h-4 w-4" />}
              {impact ? 'Confirm and save' : role ? 'Review and save' : 'Create role'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  children,
  error,
  htmlFor,
  label,
  required,
}: {
  children: React.ReactNode;
  error?: string;
  htmlFor: string;
  label: string;
  required?: boolean;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}{required ? ' *' : ''}</Label>
      {children}
      {error && <p id={`${htmlFor}-error`} className="text-sm text-destructive">{error}</p>}
    </div>
  );
}

function ImpactList({ label, permissions }: { label: string; permissions: PermissionGrant[] }) {
  return (
    <div className="rounded-md bg-background/70 p-3">
      <p className="text-sm font-medium">{label} ({permissions.length})</p>
      {permissions.length === 0 ? <p className="mt-1 text-xs text-muted-foreground">None</p> : (
        <ul className="mt-1 space-y-1 text-xs">
          {permissions.map((permission) => <li key={permissionKey(permission)} className="font-mono">{permissionKey(permission)}</li>)}
        </ul>
      )}
    </div>
  );
}
