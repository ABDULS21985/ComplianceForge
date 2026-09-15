'use client';

import * as React from 'react';
import api, { type ApiError } from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { CheckCircle2, Edit3, Globe2, Loader2, RefreshCw } from 'lucide-react';
import {
  dataGovernanceKeys,
  formatGovernanceDate,
  formatGovernanceError,
  isGovernanceConflict,
  parseGovernanceObject,
} from '@/lib/data-governance';
import {
  GovernanceErrorState,
  GovernancePanelSkeleton,
  GovernanceWarning,
} from './governance-states';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import type { DataGovernancePolicy } from '@/types/data-governance';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';

interface PolicyDraft {
  allowedRegions: string;
  archiveAfterDays: string;
  crossBorderMode: DataGovernancePolicy['cross_border_transfer_mode'];
  defaultRetentionDays: string;
  deletionGraceDays: string;
  dispositionMode: DataGovernancePolicy['disposition_approval_mode'];
  legalHoldEnabled: boolean;
  metadata: string;
  policyStatement: string;
  primaryRegion: string;
  reason: string;
  requireProcessorConfirmation: boolean;
}

function draftFromPolicy(policy?: DataGovernancePolicy): PolicyDraft {
  return {
    allowedRegions: policy?.allowed_regions.join(', ') ?? '',
    archiveAfterDays: policy?.default_archive_after_days?.toString() ?? '',
    crossBorderMode: policy?.cross_border_transfer_mode ?? 'approved_regions',
    defaultRetentionDays: policy?.default_retention_days.toString() ?? '2555',
    deletionGraceDays: policy?.deletion_grace_days.toString() ?? '30',
    dispositionMode: policy?.disposition_approval_mode ?? 'single',
    legalHoldEnabled: policy?.legal_hold_enabled ?? true,
    metadata: JSON.stringify(policy?.metadata ?? {}, null, 2),
    policyStatement: policy?.policy_statement ?? '',
    primaryRegion: policy?.primary_region ?? '',
    reason: '',
    requireProcessorConfirmation: policy?.require_processor_confirmation ?? true,
  };
}

export function PolicyPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = React.useState(false);
  const [draft, setDraft] = React.useState<PolicyDraft>(() => draftFromPolicy());
  const [validation, setValidation] = React.useState('');
  const query = useQuery({
    queryKey: dataGovernanceKeys.policy,
    queryFn: () => api.dataGovernance.getPolicy(),
    retry: false,
  });
  const notConfigured = (query.error as ApiError | null)?.status === 404;
  const policy = query.data;

  const mutation = useMutation({
    mutationFn: () => {
      const regions = [
        ...new Set(
          draft.allowedRegions
            .split(',')
            .map((item) => item.trim().toLowerCase())
            .filter(Boolean),
        ),
      ];
      const primary = draft.primaryRegion.trim().toLowerCase();
      const retentionDays = Number(draft.defaultRetentionDays);
      const archiveDays = draft.archiveAfterDays ? Number(draft.archiveAfterDays) : undefined;
      const graceDays = Number(draft.deletionGraceDays);
      const reason = draft.reason.trim();
      if (!/^[a-z][a-z0-9-]{1,31}$/.test(primary) || !regions.includes(primary)) {
        throw new Error('Primary region must be valid and included in allowed regions.');
      }
      if (regions.length < 1 || regions.length > 50)
        throw new Error('Enter between 1 and 50 allowed regions.');
      if (!Number.isInteger(retentionDays) || retentionDays < 1 || retentionDays > 36500)
        throw new Error('Default retention must be between 1 and 36,500 days.');
      if (
        archiveDays !== undefined &&
        (!Number.isInteger(archiveDays) || archiveDays < 1 || archiveDays >= retentionDays)
      )
        throw new Error('Archive timing must be at least 1 day and earlier than retention expiry.');
      if (!Number.isInteger(graceDays) || graceDays < 0 || graceDays > 365)
        throw new Error('Deletion grace must be between 0 and 365 days.');
      if (reason.length < 3)
        throw new Error('Give a reason of at least 3 characters for the audit history.');
      return api.dataGovernance.savePolicy({
        primary_region: primary,
        allowed_regions: regions,
        cross_border_transfer_mode: draft.crossBorderMode,
        default_retention_days: retentionDays,
        default_archive_after_days: archiveDays,
        deletion_grace_days: graceDays,
        disposition_approval_mode: draft.dispositionMode,
        require_processor_confirmation: draft.requireProcessorConfirmation,
        legal_hold_enabled: draft.legalHoldEnabled,
        policy_statement: draft.policyStatement.trim(),
        metadata: parseGovernanceObject(draft.metadata, 'Policy metadata'),
        expected_version: policy?.version,
        reason,
      });
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(dataGovernanceKeys.policy, saved);
      setDraft(draftFromPolicy(saved));
      setEditing(false);
      setValidation('');
    },
    onError: (error) =>
      setValidation(formatGovernanceError(error, 'The policy could not be saved.')),
  });

  function beginEditing() {
    setDraft(draftFromPolicy(policy));
    setValidation('');
    setEditing(true);
  }

  if (query.isLoading) return <GovernancePanelSkeleton label="Loading data governance policy" />;
  if (query.isError && !notConfigured) {
    return (
      <GovernanceErrorState
        message={formatGovernanceError(query.error, 'The policy could not be loaded.')}
        onRetry={() => void query.refetch()}
      />
    );
  }

  if (!policy && !editing) {
    return (
      <Card>
        <CardContent className="space-y-4 py-12 text-center">
          <Globe2 aria-hidden="true" className="mx-auto h-10 w-10 text-muted-foreground" />
          <div>
            <h2 className="text-lg font-semibold">No lifecycle policy configured</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Define residency, default retention, deletion safeguards, and legal-hold behavior
              before activating schedules.
            </p>
          </div>
          {canConfigure && (
            <Button type="button" onClick={beginEditing}>
              Configure policy
            </Button>
          )}
        </CardContent>
      </Card>
    );
  }

  if (editing) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>
            {policy ? 'Edit policy and residency' : 'Configure policy and residency'}
          </CardTitle>
          <CardDescription>
            Changes are version checked and written to the tamper-evident history.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-6"
            onSubmit={(event) => {
              event.preventDefault();
              setValidation('');
              mutation.mutate();
            }}
          >
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="Primary region" htmlFor="policy-primary">
                <Input
                  id="policy-primary"
                  required
                  placeholder="eu-west-1"
                  value={draft.primaryRegion}
                  onChange={(event) =>
                    setDraft({ ...draft, primaryRegion: event.target.value.toLowerCase() })
                  }
                />
              </Field>
              <Field label="Allowed regions (comma separated)" htmlFor="policy-regions">
                <Input
                  id="policy-regions"
                  required
                  placeholder="eu-west-1, eu-central-1"
                  value={draft.allowedRegions}
                  onChange={(event) =>
                    setDraft({ ...draft, allowedRegions: event.target.value.toLowerCase() })
                  }
                />
              </Field>
              <Field label="Cross-border transfer mode" htmlFor="policy-transfer">
                <Select
                  value={draft.crossBorderMode}
                  onValueChange={(value) =>
                    setDraft({ ...draft, crossBorderMode: value as PolicyDraft['crossBorderMode'] })
                  }
                >
                  <SelectTrigger id="policy-transfer">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="prohibited">Prohibited</SelectItem>
                    <SelectItem value="approved_regions">Approved regions</SelectItem>
                    <SelectItem value="contractual_safeguards">Contractual safeguards</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Disposition approval" htmlFor="policy-approval">
                <Select
                  value={draft.dispositionMode}
                  onValueChange={(value) =>
                    setDraft({ ...draft, dispositionMode: value as PolicyDraft['dispositionMode'] })
                  }
                >
                  <SelectTrigger id="policy-approval">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">None</SelectItem>
                    <SelectItem value="single">Single approver</SelectItem>
                    <SelectItem value="dual">Dual approval</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Default retention (days)" htmlFor="policy-retention">
                <Input
                  id="policy-retention"
                  type="number"
                  min={1}
                  max={36500}
                  required
                  value={draft.defaultRetentionDays}
                  onChange={(event) =>
                    setDraft({ ...draft, defaultRetentionDays: event.target.value })
                  }
                />
              </Field>
              <Field label="Archive after (days, optional)" htmlFor="policy-archive">
                <Input
                  id="policy-archive"
                  type="number"
                  min={1}
                  value={draft.archiveAfterDays}
                  onChange={(event) => setDraft({ ...draft, archiveAfterDays: event.target.value })}
                />
              </Field>
              <Field label="Deletion grace (days)" htmlFor="policy-grace">
                <Input
                  id="policy-grace"
                  type="number"
                  min={0}
                  max={365}
                  required
                  value={draft.deletionGraceDays}
                  onChange={(event) =>
                    setDraft({ ...draft, deletionGraceDays: event.target.value })
                  }
                />
              </Field>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <ToggleField
                label="Require processor confirmation"
                description="Require downstream processors to confirm deletion."
                checked={draft.requireProcessorConfirmation}
                onCheckedChange={(checked) =>
                  setDraft({ ...draft, requireProcessorConfirmation: checked })
                }
              />
              <ToggleField
                label="Enable legal holds"
                description="Permit authorized holds to suspend disposition."
                checked={draft.legalHoldEnabled}
                onCheckedChange={(checked) => setDraft({ ...draft, legalHoldEnabled: checked })}
              />
            </div>
            <Field label="Policy statement" htmlFor="policy-statement">
              <Textarea
                id="policy-statement"
                rows={5}
                maxLength={20000}
                value={draft.policyStatement}
                onChange={(event) => setDraft({ ...draft, policyStatement: event.target.value })}
              />
            </Field>
            <Field label="Policy metadata (JSON object)" htmlFor="policy-metadata">
              <Textarea
                id="policy-metadata"
                className="font-mono text-xs"
                rows={5}
                value={draft.metadata}
                onChange={(event) => setDraft({ ...draft, metadata: event.target.value })}
              />
            </Field>
            <Field label="Reason for change" htmlFor="policy-reason">
              <Textarea
                id="policy-reason"
                required
                minLength={3}
                maxLength={2000}
                aria-invalid={Boolean(validation)}
                value={draft.reason}
                onChange={(event) => setDraft({ ...draft, reason: event.target.value })}
              />
            </Field>
            {validation && (
              <div role="alert" className="text-sm text-destructive">
                {validation}
              </div>
            )}
            {isGovernanceConflict(mutation.error) && (
              <GovernanceWarning>
                This policy changed after you opened it. Reload the current version before applying
                your changes.
              </GovernanceWarning>
            )}
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={mutation.isPending}
                onClick={() => {
                  setEditing(false);
                  setValidation('');
                }}
              >
                Cancel
              </Button>
              {isGovernanceConflict(mutation.error) && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setEditing(false);
                    void query.refetch();
                  }}
                >
                  <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
                  Reload current policy
                </Button>
              )}
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending && (
                  <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
                )}
                Save policy
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(20rem,0.7fr)]">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>Residency and transfer policy</CardTitle>
              <CardDescription>
                Version {policy?.version} · Updated {formatGovernanceDate(policy?.updated_at)}
              </CardDescription>
            </div>
            {canConfigure && (
              <Button type="button" size="sm" variant="outline" onClick={beginEditing}>
                <Edit3 aria-hidden="true" className="mr-2 h-4 w-4" />
                Edit policy
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          <dl className="grid gap-4 sm:grid-cols-2">
            <Detail label="Primary region" value={policy?.primary_region ?? '—'} />
            <Detail label="Allowed regions" value={policy?.allowed_regions.join(', ') ?? '—'} />
            <Detail
              label="Cross-border mode"
              value={humanize(policy?.cross_border_transfer_mode)}
            />
            <Detail
              label="Disposition approval"
              value={humanize(policy?.disposition_approval_mode)}
            />
          </dl>
          {policy?.policy_statement ? (
            <div>
              <h3 className="text-sm font-medium">Policy statement</h3>
              <p className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">
                {policy.policy_statement}
              </p>
            </div>
          ) : null}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Default lifecycle controls</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Detail label="Retention" value={`${policy?.default_retention_days ?? 0} days`} />
          <Detail
            label="Archive eligibility"
            value={
              policy?.default_archive_after_days
                ? `${policy.default_archive_after_days} days`
                : 'Not configured'
            }
          />
          <Detail label="Deletion grace" value={`${policy?.deletion_grace_days ?? 0} days`} />
          <ControlState
            enabled={Boolean(policy?.require_processor_confirmation)}
            label="Processor deletion confirmation"
          />
          <ControlState enabled={Boolean(policy?.legal_hold_enabled)} label="Legal holds" />
        </CardContent>
      </Card>
    </div>
  );
}

function Field({
  children,
  htmlFor,
  label,
}: {
  children: React.ReactNode;
  htmlFor: string;
  label: string;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
    </div>
  );
}
function ToggleField({
  checked,
  description,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  description: string;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  const id = React.useId();
  return (
    <div className="flex items-start justify-between gap-4 rounded-md border p-4">
      <div>
        <Label htmlFor={id}>{label}</Label>
        <p className="mt-1 text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}
function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words text-sm font-medium">{value}</dd>
    </div>
  );
}
function ControlState({ enabled, label }: { enabled: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <CheckCircle2
        aria-hidden="true"
        className={`h-4 w-4 ${enabled ? 'text-emerald-600' : 'text-muted-foreground'}`}
      />
      <span>
        {label}: <strong>{enabled ? 'Enabled' : 'Disabled'}</strong>
      </span>
    </div>
  );
}
function humanize(value?: string): string {
  return value ? value.replace(/_/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) : '—';
}
