'use client';

import { API_KEY_ACTIONS, API_KEY_RESOURCES, formatApiError } from '@/lib/enterprise-settings';
import type { APIKeyRecord, CreateAPIKeyInput } from '@/types/enterprise-settings';
import { Card, CardContent } from '@/components/ui/card';
import { Copy, Download, Eye, EyeOff, KeyRound, Loader2, Plus, ShieldAlert, Trash2, X } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { type FormEvent, useState } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmAction } from '@/components/settings/confirm-action';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from 'sonner';

function formatDate(value: string | null): string {
  if (!value) return 'Never';
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}

function safeFilename(value: string): string {
  const stem = value.trim().toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-|-$/g, '') || 'api-key';
  return `${stem}.txt`;
}

export function downloadAPIKey(name: string, rawKey: string): void {
  const blob = new Blob([
    `ComplianceForge API key: ${name}\n\n${rawKey}\n\nStore this credential in a secrets manager. Delete this file after import.\n`,
  ], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = safeFilename(name);
  anchor.click();
  URL.revokeObjectURL(url);
}

interface RevealedKey {
  name: string;
  rawKey: string;
}

function OneTimeKeyDialog({ value, onDiscard }: { value: RevealedKey; onDiscard: () => void }) {
  const [visible, setVisible] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value.rawKey);
      toast.success('API key copied');
    } catch {
      toast.error('Copy failed. Select the key and copy it manually.');
      setVisible(true);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && acknowledged) onDiscard(); }}>
      <DialogContent
        className="sm:max-w-2xl"
        onEscapeKeyDown={(event) => event.preventDefault()}
        onPointerDownOutside={(event) => event.preventDefault()}
      >
        <DialogHeader><DialogTitle>Store your API key now</DialogTitle><DialogDescription>This is the only response that contains the complete key. It cannot be recovered later.</DialogDescription></DialogHeader>
        <div role="alert" className="rounded-md border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950 dark:bg-amber-950/30 dark:text-amber-100"><ShieldAlert aria-hidden="true" className="mr-2 inline h-5 w-5" />Put this credential in an approved secrets manager. Clipboard history and downloaded files may expose it.</div>
        <div className="space-y-2"><Label htmlFor="revealed-api-key">API key for {value.name}</Label><div className="flex gap-2"><Input id="revealed-api-key" readOnly type={visible ? 'text' : 'password'} value={value.rawKey} className="font-mono" /><Button type="button" size="icon" variant="outline" onClick={() => setVisible((current) => !current)} aria-label={visible ? 'Hide API key' : 'Reveal API key'}>{visible ? <EyeOff aria-hidden="true" className="h-4 w-4" /> : <Eye aria-hidden="true" className="h-4 w-4" />}</Button></div></div>
        <div className="flex flex-wrap gap-2"><Button type="button" variant="outline" onClick={() => void copy()}><Copy aria-hidden="true" className="mr-2 h-4 w-4" />Copy</Button><Button type="button" variant="outline" onClick={() => downloadAPIKey(value.name, value.rawKey)}><Download aria-hidden="true" className="mr-2 h-4 w-4" />Download warning file</Button></div>
        <label className="flex items-start gap-2 rounded-md border p-3 text-sm"><input type="checkbox" className="mt-0.5 h-4 w-4" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>I have stored this key securely and understand that closing this dialog permanently removes it from this browser view.</span></label>
        <DialogFooter><Button type="button" disabled={!acknowledged} onClick={onDiscard}>Finish and hide key</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function APIKeysPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState('');
  const [rateLimit, setRateLimit] = useState('60');
  const [expiry, setExpiry] = useState('');
  const [permissionAction, setPermissionAction] = useState<(typeof API_KEY_ACTIONS)[number]>('read');
  const [permissionResource, setPermissionResource] = useState<(typeof API_KEY_RESOURCES)[number]>('controls');
  const [permissions, setPermissions] = useState<string[]>([]);
  const [validationError, setValidationError] = useState('');
  const [revealed, setRevealed] = useState<RevealedKey | null>(null);

  const keysQuery = useQuery({
    queryKey: ['integrations', 'api-keys'],
    queryFn: () => api.integrations.listAPIKeys(),
    staleTime: 30_000,
  });
  const createMutation = useMutation({
    mutationFn: (input: CreateAPIKeyInput) => api.integrations.createAPIKey(input),
    onSuccess: (response, input) => {
      void queryClient.invalidateQueries({ queryKey: ['integrations', 'api-keys'] });
      setCreateOpen(false);
      setRevealed({ name: input.name, rawKey: response.key });
      setName(''); setRateLimit('60'); setExpiry(''); setPermissions([]);
    },
    onError: (error) => toast.error(formatApiError(error, 'The API key could not be created.')),
  });
  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.integrations.revokeAPIKey(id),
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['integrations', 'api-keys'] }); toast.success('API key revoked'); },
    onError: (error) => toast.error(formatApiError(error, 'The API key could not be revoked.')),
  });

  function addPermission() {
    const permission = `${permissionAction}:${permissionResource}`;
    setPermissions((current) => current.includes(permission) ? current : [...current, permission]);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setValidationError('');
    if (!name.trim()) return setValidationError('Name is required.');
    if (name.trim().length > 200) return setValidationError('Name must not exceed 200 characters.');
    if (permissions.length === 0) return setValidationError('Grant at least one permission.');
    const rate = Number(rateLimit);
    if (!Number.isInteger(rate) || rate < 0 || rate > 10_000) return setValidationError('Rate limit must be a whole number between 0 and 10000.');
    const input: CreateAPIKeyInput = { name: name.trim(), permissions, rate_limit: rate };
    if (expiry) {
      const date = new Date(expiry);
      if (Number.isNaN(date.valueOf()) || date <= new Date()) return setValidationError('Expiry must be a valid future date and time.');
      input.expires_at = date.toISOString();
    }
    createMutation.mutate(input);
  }

  const keys = keysQuery.data?.data ?? [];
  return (
    <section aria-labelledby="api-keys-heading" className="space-y-5">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start"><div><h2 id="api-keys-heading" className="text-xl font-semibold">API keys</h2><p className="text-sm text-muted-foreground">Issue least-privilege credentials for approved automation clients.</p></div>{canConfigure && <Button type="button" size="sm" onClick={() => { setValidationError(''); setCreateOpen(true); }}><Plus aria-hidden="true" className="mr-2 h-4 w-4" />Create key</Button>}</div>
      {keysQuery.isLoading && <div role="status" aria-label="Loading API keys" className="space-y-3">{[0, 1, 2].map((item) => <Skeleton key={item} className="h-32" />)}</div>}
      {keysQuery.isError && <Card><CardContent className="space-y-3 py-8 text-center"><p role="alert">{formatApiError(keysQuery.error, 'API keys could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void keysQuery.refetch()}>Try again</Button></CardContent></Card>}
      {keysQuery.isSuccess && keys.length === 0 && <Card><CardContent className="py-12 text-center"><KeyRound aria-hidden="true" className="mx-auto mb-3 h-8 w-8 text-muted-foreground" /><p className="font-medium">No API keys</p><p className="text-sm text-muted-foreground">Create a scoped key when an automation client is ready.</p></CardContent></Card>}
      <div className="grid gap-3">
        {keys.map((key: APIKeyRecord) => {
          const expired = Boolean(key.expires_at && new Date(key.expires_at) <= new Date());
          return <Card key={key.id}><CardContent className="space-y-4 py-5"><div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start"><div><div className="flex flex-wrap items-center gap-2"><h3 className="font-medium">{key.name}</h3><Badge variant={key.is_active && !expired ? 'default' : 'secondary'}>{expired ? 'Expired' : key.is_active ? 'Active' : 'Revoked'}</Badge></div><p className="mt-1 font-mono text-sm text-muted-foreground">{key.key_prefix}…</p></div>{canConfigure && key.is_active && !expired && <ConfirmAction title={`Revoke ${key.name}?`} description="Clients using this key will immediately lose access. Revocation cannot be undone." actionLabel="Revoke key" onConfirm={() => revokeMutation.mutate(key.id)} pending={revokeMutation.isPending && revokeMutation.variables === key.id} triggerVariant="destructive"><Trash2 aria-hidden="true" className="mr-2 h-4 w-4" />Revoke</ConfirmAction>}</div><dl className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4"><div><dt className="text-muted-foreground">Rate limit</dt><dd>{key.rate_limit_per_minute}/minute</dd></div><div><dt className="text-muted-foreground">Created</dt><dd>{formatDate(key.created_at)}</dd></div><div><dt className="text-muted-foreground">Last used</dt><dd>{formatDate(key.last_used_at)}</dd></div><div><dt className="text-muted-foreground">Expires</dt><dd>{formatDate(key.expires_at)}</dd></div></dl><div className="flex flex-wrap gap-1">{key.permissions.map((permission) => <Badge key={permission} variant="outline" className="font-normal">{permission}</Badge>)}</div></CardContent></Card>;
        })}
      </div>

      <Dialog open={createOpen} onOpenChange={(open) => !createMutation.isPending && setCreateOpen(open)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <form onSubmit={submit} className="space-y-5">
            <DialogHeader><DialogTitle>Create API key</DialogTitle><DialogDescription>The full credential is returned once. Choose only the permissions this client needs.</DialogDescription></DialogHeader>
            <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="key-name">Key name</Label><Input id="key-name" required maxLength={200} value={name} onChange={(event) => setName(event.target.value)} placeholder="CI evidence collector" /></div><div className="space-y-2"><Label htmlFor="key-rate">Rate limit per minute</Label><Input id="key-rate" type="number" min={0} max={10000} step={1} required value={rateLimit} onChange={(event) => setRateLimit(event.target.value)} /></div></div>
            <div className="space-y-2"><Label htmlFor="key-expiry">Expiry (optional)</Label><Input id="key-expiry" type="datetime-local" value={expiry} onChange={(event) => setExpiry(event.target.value)} /></div>
            <fieldset className="space-y-3"><legend className="text-sm font-medium">Permissions</legend><div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto]"><Select value={permissionAction} onValueChange={(value: (typeof API_KEY_ACTIONS)[number]) => setPermissionAction(value)}><SelectTrigger aria-label="Permission action"><SelectValue /></SelectTrigger><SelectContent>{API_KEY_ACTIONS.map((action) => <SelectItem key={action} value={action}>{action}</SelectItem>)}</SelectContent></Select><Select value={permissionResource} onValueChange={(value: (typeof API_KEY_RESOURCES)[number]) => setPermissionResource(value)}><SelectTrigger aria-label="Permission resource"><SelectValue /></SelectTrigger><SelectContent>{API_KEY_RESOURCES.map((resource) => <SelectItem key={resource} value={resource}>{resource}</SelectItem>)}</SelectContent></Select><Button type="button" variant="outline" onClick={addPermission}>Add</Button></div>{permissions.length === 0 ? <p className="text-xs text-muted-foreground">No permissions selected.</p> : <ul aria-label="Selected API permissions" className="flex flex-wrap gap-2">{permissions.map((permission) => <li key={permission}><Badge variant="secondary" className="gap-1">{permission}<button type="button" aria-label={`Remove ${permission}`} onClick={() => setPermissions((current) => current.filter((item) => item !== permission))} className="rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"><X aria-hidden="true" className="h-3 w-3" /></button></Badge></li>)}</ul>}</fieldset>
            {validationError && <p role="alert" className="text-sm text-destructive">{validationError}</p>}
            <DialogFooter><Button type="button" variant="outline" disabled={createMutation.isPending} onClick={() => setCreateOpen(false)}>Cancel</Button><Button type="submit" disabled={createMutation.isPending}>{createMutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Create key</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      {revealed && <OneTimeKeyDialog value={revealed} onDiscard={() => setRevealed(null)} />}
    </section>
  );
}
