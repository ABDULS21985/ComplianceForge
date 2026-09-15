'use client';

import * as React from 'react';
import { AlertTriangle, Loader2, Save } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  featureEvaluationExplanation,
  featureFlagKeys,
  formatFeatureFlagError,
  isFeatureFlagConflict,
  rolloutPercent,
} from '@/lib/feature-flags';
import type { FeatureFlagEvaluation, FeatureFlagOverrideInput } from '@/types/feature-flag';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

interface EditorState {
  enabled: boolean;
  expiresAt: string;
  reason: string;
  rolloutPercent: string;
  startsAt: string;
  useCustomRollout: boolean;
  variant: string;
}

function toLocalDateTime(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return '';
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function initialState(evaluation: FeatureFlagEvaluation): EditorState {
  const override = evaluation.override;
  return {
    enabled: override?.enabled ?? evaluation.capability.default_enabled,
    expiresAt: toLocalDateTime(override?.expires_at),
    reason: '',
    rolloutPercent: String(rolloutPercent(override?.rollout_basis_points ?? evaluation.capability.rollout_basis_points)),
    startsAt: toLocalDateTime(override?.starts_at),
    useCustomRollout: override?.rollout_basis_points !== undefined,
    variant: JSON.stringify(override?.variant ?? evaluation.variant ?? {}, null, 2),
  };
}

function parseVariant(value: string): Record<string, unknown> {
  const encoded = new TextEncoder().encode(value || '{}');
  if (encoded.byteLength > 16 * 1024) throw new Error('Variant must not exceed 16 KiB.');
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || '{}');
  } catch {
    throw new Error('Variant must be valid JSON.');
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('Variant must be a JSON object.');
  }
  return parsed as Record<string, unknown>;
}

export function FeatureFlagEditorDialog({
  evaluation,
  onOpenChange,
  onReload,
  onSaved,
  open,
}: {
  evaluation: FeatureFlagEvaluation;
  onOpenChange: (open: boolean) => void;
  onReload: () => void;
  onSaved: () => void;
  open: boolean;
}) {
  const queryClient = useQueryClient();
  const [state, setState] = React.useState(() => initialState(evaluation));
  const [validationError, setValidationError] = React.useState('');
  const mutation = useMutation({
    mutationFn: (input: FeatureFlagOverrideInput) =>
      api.featureFlags.upsertOverride(evaluation.capability.key, input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: featureFlagKeys.all });
      toast.success(evaluation.override ? 'Feature flag override updated.' : 'Feature flag override created.');
      onSaved();
      onOpenChange(false);
    },
  });

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const reason = state.reason.trim();
    if (reason.length < 3 || reason.length > 1000) {
      setValidationError('Reason must contain 3–1,000 characters.');
      return;
    }
    let variant: Record<string, unknown>;
    try {
      variant = parseVariant(state.variant);
    } catch (error) {
      setValidationError(error instanceof Error ? error.message : 'Variant is invalid.');
      return;
    }
    const rollout = Number(state.rolloutPercent);
    if (state.useCustomRollout && (!Number.isFinite(rollout) || rollout < 0 || rollout > 100)) {
      setValidationError('Rollout must be between 0 and 100 percent.');
      return;
    }
    const startsAt = state.startsAt ? new Date(state.startsAt) : null;
    const expiresAt = state.expiresAt ? new Date(state.expiresAt) : null;
    if ((startsAt && !Number.isFinite(startsAt.getTime())) || (expiresAt && !Number.isFinite(expiresAt.getTime()))) {
      setValidationError('Activation dates must be valid.');
      return;
    }
    if (startsAt && expiresAt && expiresAt <= startsAt) {
      setValidationError('Expiration must be after activation.');
      return;
    }
    const input: FeatureFlagOverrideInput = { enabled: state.enabled, reason, variant };
    if (state.useCustomRollout) input.rollout_basis_points = Math.round(rollout * 100);
    if (startsAt) input.starts_at = startsAt.toISOString();
    if (expiresAt) input.expires_at = expiresAt.toISOString();
    if (evaluation.override) input.expected_version = evaluation.override.version;
    setValidationError('');
    await mutation.mutateAsync(input).catch(() => undefined);
  }

  const error = validationError || (mutation.error ? formatFeatureFlagError(mutation.error, 'The override could not be saved.') : '');
  return (
    <Dialog open={open} onOpenChange={(next) => !mutation.isPending && onOpenChange(next)}>
      <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Configure {evaluation.capability.display_name}</DialogTitle>
          <DialogDescription>Create or update the tenant override for this capability. Every change requires a business reason and is permanently audited.</DialogDescription>
        </DialogHeader>
        <form className="space-y-5" onSubmit={(event) => void submit(event)} noValidate>
          {error && (
            <div role="alert" className="space-y-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <p>{error}</p>
              {isFeatureFlagConflict(mutation.error) && <Button type="button" size="sm" variant="outline" onClick={onReload}>Reload current override</Button>}
            </div>
          )}
          <div className="rounded-md border p-4 text-sm">
            <p className="font-medium">Current evaluation: {evaluation.enabled ? 'Enabled' : 'Disabled'}</p>
            <p className="mt-1 text-muted-foreground">{featureEvaluationExplanation(evaluation)}</p>
          </div>
          {(evaluation.capability.kill_switch || !evaluation.entitled || evaluation.blocking_capability) && (
            <div className="flex gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
              <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" />
              <p>A tenant override cannot bypass the global kill switch, subscription requirements, prerequisites, or deterministic rollout assignment.</p>
            </div>
          )}
          <div className="flex items-center justify-between gap-4 rounded-md border p-4">
            <div><Label htmlFor="feature-enabled">Tenant override enabled</Label><p className="mt-1 text-xs text-muted-foreground">Controls the tenant preference when all higher-priority gates allow access.</p></div>
            <Switch id="feature-enabled" checked={state.enabled} onCheckedChange={(enabled) => setState((current) => ({ ...current, enabled }))} />
          </div>
          <label className="flex items-start gap-3 rounded-md border p-4">
            <input type="checkbox" className="mt-0.5 h-4 w-4" checked={state.useCustomRollout} onChange={(event) => setState((current) => ({ ...current, useCustomRollout: event.target.checked }))} />
            <span><span className="block text-sm font-medium">Override rollout percentage</span><span className="block text-xs text-muted-foreground">Otherwise the deployment-managed {rolloutPercent(evaluation.capability.rollout_basis_points)}% rollout applies.</span></span>
          </label>
          {state.useCustomRollout && (
            <div className="space-y-2">
              <Label htmlFor="feature-rollout">Tenant rollout percentage</Label>
              <div className="flex items-center gap-3"><input id="feature-rollout" type="range" min="0" max="100" step="0.01" className="w-full" value={state.rolloutPercent} onChange={(event) => setState((current) => ({ ...current, rolloutPercent: event.target.value }))} /><Input aria-label="Rollout percentage value" className="w-28" type="number" min="0" max="100" step="0.01" value={state.rolloutPercent} onChange={(event) => setState((current) => ({ ...current, rolloutPercent: event.target.value }))} /></div>
              <p className="text-xs text-muted-foreground">Selection is deterministic per organization; it does not randomly change between requests.</p>
            </div>
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="feature-start">Activate at</Label><Input id="feature-start" type="datetime-local" value={state.startsAt} onChange={(event) => setState((current) => ({ ...current, startsAt: event.target.value }))} /><p className="text-xs text-muted-foreground">Blank means immediately active.</p></div>
            <div className="space-y-2"><Label htmlFor="feature-expiry">Expire at</Label><Input id="feature-expiry" type="datetime-local" value={state.expiresAt} onChange={(event) => setState((current) => ({ ...current, expiresAt: event.target.value }))} /><p className="text-xs text-muted-foreground">Blank means no automatic expiry.</p></div>
          </div>
          <div className="space-y-2"><Label htmlFor="feature-variant">Variant (JSON object)</Label><Textarea id="feature-variant" className="font-mono text-xs" rows={6} spellCheck={false} value={state.variant} onChange={(event) => setState((current) => ({ ...current, variant: event.target.value }))} /><p className="text-xs text-muted-foreground">Maximum serialized size: 16 KiB. Never store credentials or secrets in variants.</p></div>
          <div className="space-y-2"><Label htmlFor="feature-reason">Business reason *</Label><Textarea id="feature-reason" autoFocus maxLength={1000} rows={3} value={state.reason} onChange={(event) => setState((current) => ({ ...current, reason: event.target.value }))} /></div>
          <DialogFooter><Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>Cancel</Button><Button type="submit" disabled={mutation.isPending}>{mutation.isPending ? <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" /> : <Save aria-hidden="true" className="mr-2 h-4 w-4" />}Save audited override</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
