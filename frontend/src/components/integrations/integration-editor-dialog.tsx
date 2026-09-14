'use client';

import { useState, type FormEvent } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Loader2, ShieldCheck } from 'lucide-react';
import { toast } from 'sonner';

import api from '@/lib/api';
import { formatApiError, parseCommaList, parseJSONObject, type IntegrationCatalogEntry } from '@/lib/enterprise-settings';
import type { Integration, IntegrationInput } from '@/types/enterprise-settings';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';

interface IntegrationEditorDialogProps {
  catalogEntry?: IntegrationCatalogEntry;
  integration?: Integration;
  onOpenChange: (open: boolean) => void;
  open: boolean;
}

function validCapability(value: string): boolean {
  return /^[a-z0-9][a-z0-9_.-]*$/.test(value);
}

export function IntegrationEditorDialog({
  catalogEntry,
  integration,
  onOpenChange,
  open,
}: IntegrationEditorDialogProps) {
  const queryClient = useQueryClient();
  const editing = Boolean(integration);
  const integrationType = integration?.integration_type ?? catalogEntry?.type;
  const [name, setName] = useState(integration?.name ?? catalogEntry?.name ?? '');
  const [description, setDescription] = useState(integration?.description ?? '');
  const [syncFrequency, setSyncFrequency] = useState(String(integration?.sync_frequency_minutes ?? 0));
  const [capabilities, setCapabilities] = useState((integration?.capabilities ?? []).join(', '));
  const [configuration, setConfiguration] = useState('');
  const [validationError, setValidationError] = useState('');

  const saveMutation = useMutation({
    mutationFn: async (input: IntegrationInput) => {
      if (integration) {
        await api.integrations.update(integration.id, input);
        return;
      }
      if (!integrationType) throw new Error('Integration type is unavailable.');
      await api.integrations.create(input as IntegrationInput & {
        integration_type: Integration['integration_type'];
        name: string;
        configuration: Record<string, unknown>;
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['integrations'] });
      setConfiguration('');
      onOpenChange(false);
      toast.success(editing ? 'Integration updated' : 'Integration created');
    },
    onError: (error) => toast.error(formatApiError(error, 'The integration could not be saved.')),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setValidationError('');
    if (!integrationType) return setValidationError('Integration type is required.');
    if (!name.trim()) return setValidationError('Name is required.');
    if (name.trim().length > 200) return setValidationError('Name must not exceed 200 characters.');
    const frequency = Number(syncFrequency);
    if (!Number.isInteger(frequency) || frequency < 0 || frequency > 525_600) {
      return setValidationError('Sync frequency must be a whole number between 0 and 525600.');
    }
    const capabilityList = parseCommaList(capabilities);
    if (capabilityList.length > 100 || capabilityList.some((item) => !validCapability(item))) {
      return setValidationError('Capabilities must be at most 100 comma-separated lowercase tokens.');
    }

    const input: IntegrationInput = {
      integration_type: integrationType,
      name: name.trim(),
      description: description.trim() || null,
      sync_frequency_minutes: frequency,
      capabilities: capabilityList,
    };
    if (!editing || configuration.trim()) {
      try {
        input.configuration = parseJSONObject(configuration, 'Configuration');
      } catch (error) {
        return setValidationError(error instanceof Error ? error.message : 'Configuration must be valid JSON.');
      }
    }
    saveMutation.mutate(input);
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => !saveMutation.isPending && onOpenChange(nextOpen)}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <form onSubmit={submit} className="space-y-5">
          <DialogHeader>
            <DialogTitle>{editing ? `Edit ${integration?.name}` : `Configure ${catalogEntry?.name}`}</DialogTitle>
            <DialogDescription>
              Configuration is sent only through the secure BFF and encrypted by the API. Stored configuration is never returned to the browser.
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-200">
            <ShieldCheck aria-hidden="true" className="mr-2 inline h-4 w-4" />
            {editing ? 'Leave configuration blank to preserve the existing encrypted value.' : 'Remove secrets from your clipboard after saving.'}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="integration-name">Display name</Label><Input id="integration-name" required maxLength={200} value={name} onChange={(event) => setName(event.target.value)} /></div>
            <div className="space-y-2"><Label htmlFor="integration-frequency">Sync frequency (minutes)</Label><Input id="integration-frequency" type="number" min={0} max={525600} step={1} required value={syncFrequency} onChange={(event) => setSyncFrequency(event.target.value)} /><p className="text-xs text-muted-foreground">{editing ? 'Use a positive value to change the schedule. The API cannot reset an existing schedule to manual-only.' : '0 creates a manual-only integration.'}</p></div>
          </div>
          <div className="space-y-2"><Label htmlFor="integration-description">Description</Label><Textarea id="integration-description" maxLength={2000} value={description} onChange={(event) => setDescription(event.target.value)} />{editing && <p className="text-xs text-muted-foreground">A blank value preserves the current description because the API has no explicit clear operation.</p>}</div>
          <div className="space-y-2"><Label htmlFor="integration-capabilities">Capabilities</Label><Input id="integration-capabilities" placeholder="evidence.read, findings.write" value={capabilities} onChange={(event) => setCapabilities(event.target.value.toLowerCase())} /><p className="text-xs text-muted-foreground">Comma-separated connector capability tokens.</p></div>
          <div className="space-y-2"><Label htmlFor="integration-configuration">{editing ? 'Replacement configuration JSON (optional)' : 'Configuration JSON'}</Label><Textarea id="integration-configuration" required={!editing} rows={9} spellCheck={false} autoComplete="off" className="font-mono text-xs" placeholder={'{\n  "endpoint": "https://api.example.com",\n  "token": "…"\n}'} value={configuration} onChange={(event) => setConfiguration(event.target.value)} /><p className="text-xs text-muted-foreground">Use the field names required by the connector service. Maximum payload size is 64 KiB.</p></div>
          {validationError && <p role="alert" className="text-sm text-destructive">{validationError}</p>}
          <DialogFooter><Button type="button" variant="outline" onClick={() => { setConfiguration(''); onOpenChange(false); }} disabled={saveMutation.isPending}>Cancel</Button><Button type="submit" disabled={saveMutation.isPending}>{saveMutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Save integration</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
