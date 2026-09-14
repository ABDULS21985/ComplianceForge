'use client';

import * as React from 'react';
import type { Asset, AssetClassification, AssetCreateInput, AssetCriticality, AssetPatch, AssetType } from '@/types/asset';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { formatAssetError, humanizeAssetToken, normalizeAssetTags } from '@/lib/asset';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useCreateAsset, useUpdateAsset } from '@/lib/api-hooks';
import { useForm, useWatch } from 'react-hook-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';

const optionalUuid = z.union([z.literal(''), z.string().uuid('Enter a valid UUID.')]);
const optionalIp = z.union([z.literal(''), z.string().ip('Enter a valid IPv4 or IPv6 address.')]);

const assetEditorSchema = z.object({
  name: z.string().trim().min(1, 'Name is required.').max(200),
  asset_type: z.enum(['hardware', 'software', 'data', 'service', 'network', 'people', 'facility']),
  category: z.string().trim().max(100),
  description: z.string().trim().max(10_000),
  criticality: z.enum(['critical', 'high', 'medium', 'low']),
  owner_user_id: optionalUuid,
  location: z.string().trim().max(200),
  ip_address: optionalIp,
  classification: z.enum(['public', 'internal', 'confidential', 'restricted']),
  processes_personal_data: z.boolean(),
  linked_vendor_id: optionalUuid,
  tags: z.string().refine((value) => normalizeAssetTags(value).every((tag) => tag.length <= 64), 'Each tag must be 64 characters or fewer.'),
  metadata: z.string().max(16 * 1024).refine((value) => {
    try { const parsed = JSON.parse(value || '{}'); return parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed); } catch { return false; }
  }, 'Metadata must be a valid JSON object.'),
});

type AssetEditorValues = z.infer<typeof assetEditorSchema>;

function defaults(asset?: Asset): AssetEditorValues {
  return {
    name: asset?.name ?? '', asset_type: asset?.asset_type ?? 'software', category: asset?.category ?? '',
    description: asset?.description ?? '', criticality: asset?.criticality ?? 'medium', owner_user_id: asset?.owner_user_id ?? '',
    location: asset?.location ?? '', ip_address: asset?.ip_address ?? '', classification: asset?.classification ?? 'internal',
    processes_personal_data: asset?.processes_personal_data ?? false, linked_vendor_id: asset?.linked_vendor_id ?? '',
    tags: asset?.tags.join(', ') ?? '', metadata: JSON.stringify(asset?.metadata ?? {}, null, 2),
  };
}

export function AssetEditorDialog({ asset, open, onOpenChange, onCreated }: { asset?: Asset; open: boolean; onOpenChange: (open: boolean) => void; onCreated?: (asset: Asset) => void }) {
  const create = useCreateAsset();
  const update = useUpdateAsset(asset?.id ?? '');
  const form = useForm<AssetEditorValues>({ resolver: zodResolver(assetEditorSchema), defaultValues: defaults(asset) });
  const mutation = asset ? update : create;
  const assetType = useWatch({ control: form.control, name: 'asset_type' });
  const criticality = useWatch({ control: form.control, name: 'criticality' });
  const classification = useWatch({ control: form.control, name: 'classification' });

  React.useEffect(() => { if (open) form.reset(defaults(asset)); }, [asset, form, open]);

  async function submit(values: AssetEditorValues) {
    const common = {
      name: values.name.trim(), asset_type: values.asset_type, category: values.category.trim(),
      description: values.description.trim(), criticality: values.criticality, location: values.location.trim(),
      classification: values.classification, processes_personal_data: values.processes_personal_data,
      tags: normalizeAssetTags(values.tags), metadata: JSON.parse(values.metadata || '{}') as Record<string, unknown>,
    };
    try {
      if (asset) {
        const patch: AssetPatch = { ...common, expected_version: asset.version };
        if (values.owner_user_id) patch.owner_user_id = values.owner_user_id; else if (asset.owner_user_id) patch.clear_owner = true;
        if (values.ip_address) patch.ip_address = values.ip_address; else if (asset.ip_address) patch.clear_ip_address = true;
        if (values.linked_vendor_id) patch.linked_vendor_id = values.linked_vendor_id; else if (asset.linked_vendor_id) patch.clear_linked_vendor = true;
        await update.mutateAsync(patch);
      } else {
        const input: AssetCreateInput = { ...common };
        if (values.owner_user_id) input.owner_user_id = values.owner_user_id;
        if (values.ip_address) input.ip_address = values.ip_address;
        if (values.linked_vendor_id) input.linked_vendor_id = values.linked_vendor_id;
        const created = await create.mutateAsync(input);
        onCreated?.(created);
      }
      onOpenChange(false);
    } catch {
      // Keep the dialog open and announce the server error.
    }
  }

  const error = mutation.error ? formatAssetError(mutation.error, `Failed to ${asset ? 'update' : 'register'} asset.`) : null;
  return <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}><DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto"><DialogHeader><DialogTitle>{asset ? 'Edit asset' : 'Register asset'}</DialogTitle><DialogDescription>Inventory fields feed risk, privacy, vendor, and continuity scoping. Required fields are marked with an asterisk.</DialogDescription></DialogHeader><form className="space-y-4" onSubmit={form.handleSubmit(submit)} noValidate>
    {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
    <Field label="Name" htmlFor="asset-name" required error={form.formState.errors.name?.message}><Input id="asset-name" autoFocus aria-invalid={Boolean(form.formState.errors.name)} {...form.register('name')} /></Field>
    <div className="grid gap-4 sm:grid-cols-3">
      <Field label="Asset type" htmlFor="asset-type" required><Select value={assetType} onValueChange={(value) => form.setValue('asset_type', value as AssetType)}><SelectTrigger id="asset-type"><SelectValue /></SelectTrigger><SelectContent>{(['hardware', 'software', 'data', 'service', 'network', 'people', 'facility'] as const).map((value) => <SelectItem key={value} value={value}>{humanizeAssetToken(value)}</SelectItem>)}</SelectContent></Select></Field>
      <Field label="Criticality" htmlFor="asset-criticality" required><Select value={criticality} onValueChange={(value) => form.setValue('criticality', value as AssetCriticality)}><SelectTrigger id="asset-criticality"><SelectValue /></SelectTrigger><SelectContent>{(['critical', 'high', 'medium', 'low'] as const).map((value) => <SelectItem key={value} value={value}>{humanizeAssetToken(value)}</SelectItem>)}</SelectContent></Select></Field>
      <Field label="Classification" htmlFor="asset-classification" required><Select value={classification} onValueChange={(value) => form.setValue('classification', value as AssetClassification)}><SelectTrigger id="asset-classification"><SelectValue /></SelectTrigger><SelectContent>{(['public', 'internal', 'confidential', 'restricted'] as const).map((value) => <SelectItem key={value} value={value}>{humanizeAssetToken(value)}</SelectItem>)}</SelectContent></Select></Field>
    </div>
    <Field label="Description" htmlFor="asset-description" error={form.formState.errors.description?.message}><Textarea id="asset-description" rows={3} aria-invalid={Boolean(form.formState.errors.description)} {...form.register('description')} /></Field>
    <div className="grid gap-4 sm:grid-cols-2"><Field label="Category" htmlFor="asset-category" error={form.formState.errors.category?.message}><Input id="asset-category" {...form.register('category')} /></Field><Field label="Location" htmlFor="asset-location" error={form.formState.errors.location?.message}><Input id="asset-location" {...form.register('location')} /></Field></div>
    <div className="grid gap-4 sm:grid-cols-2"><Field label="Owner user UUID" htmlFor="asset-owner" error={form.formState.errors.owner_user_id?.message}><Input id="asset-owner" aria-invalid={Boolean(form.formState.errors.owner_user_id)} {...form.register('owner_user_id')} /><p className="text-xs text-muted-foreground">Must identify an active user in this organization.</p></Field><Field label="Linked vendor UUID" htmlFor="asset-vendor" error={form.formState.errors.linked_vendor_id?.message}><Input id="asset-vendor" aria-invalid={Boolean(form.formState.errors.linked_vendor_id)} {...form.register('linked_vendor_id')} /></Field></div>
    <div className="grid gap-4 sm:grid-cols-2"><Field label="IP address" htmlFor="asset-ip" error={form.formState.errors.ip_address?.message}><Input id="asset-ip" placeholder="192.0.2.10 or 2001:db8::1" aria-invalid={Boolean(form.formState.errors.ip_address)} {...form.register('ip_address')} /></Field><Field label="Tags" htmlFor="asset-tags" error={form.formState.errors.tags?.message}><Input id="asset-tags" placeholder="production, pii, eu-west" {...form.register('tags')} /><p className="text-xs text-muted-foreground">Comma-separated; tags are normalized to lowercase.</p></Field></div>
    <label className="flex items-start gap-3 rounded-md border p-3"><input type="checkbox" className="mt-1 h-4 w-4" {...form.register('processes_personal_data')} /><span><span className="block text-sm font-medium">Processes personal data</span><span className="block text-xs text-muted-foreground">Include this asset in privacy and data-protection scoping.</span></span></label>
    <Field label="Metadata (JSON object)" htmlFor="asset-metadata" error={form.formState.errors.metadata?.message}><Textarea id="asset-metadata" className="font-mono text-xs" rows={4} spellCheck={false} aria-invalid={Boolean(form.formState.errors.metadata)} {...form.register('metadata')} /></Field>
    <DialogFooter><Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>Cancel</Button><Button type="submit" disabled={mutation.isPending}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}{asset ? 'Save asset' : 'Register asset'}</Button></DialogFooter>
  </form></DialogContent></Dialog>;
}

function Field({ label, htmlFor, required, error, children }: { label: string; htmlFor: string; required?: boolean; error?: string; children: React.ReactNode }) {
  const errorId = `${htmlFor}-error`;
  return <div className="space-y-2"><Label htmlFor={htmlFor}>{label}{required ? ' *' : ''}</Label>{children}{error && <p id={errorId} className="text-sm text-destructive">{error}</p>}</div>;
}
