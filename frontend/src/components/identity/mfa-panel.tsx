'use client';

import * as React from 'react';

import { AlertTriangle, KeyRound, Loader2, Plus, RefreshCw, ShieldCheck } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { formatIdentityMethod, identityKeys, identityReasonValid } from '@/lib/identity';
import type { IdentityMFAFactor, IdentityTOTPEnrollment } from '@/types/identity';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/data/empty-state';
import { formatApiError } from '@/lib/enterprise-settings';
import { Input } from '@/components/ui/input';
import { RecoveryCodesDialog } from '@/components/identity/recovery-codes-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { StepUpDialog } from '@/components/identity/step-up-dialog';

type FactorAction = { factor: IdentityMFAFactor; kind: 'disable' | 'recovery' } | null;

export function MFAPanel() {
  const queryClient = useQueryClient();
  const [enrollmentOpen, setEnrollmentOpen] = React.useState(false);
  const [action, setAction] = React.useState<FactorAction>(null);
  const [reason, setReason] = React.useState('');
  const [stepUpOpen, setStepUpOpen] = React.useState(false);
  const [codes, setCodes] = React.useState<string[]>([]);
  const [message, setMessage] = React.useState('');
  const query = useQuery({
    queryKey: identityKeys.factors,
    queryFn: () => api.identity.listFactors(),
    staleTime: 30_000,
  });

  async function protectedAction(grantToken: string) {
    if (!action) return;
    if (action.kind === 'disable') {
      await api.identity.disableFactor(
        action.factor.id,
        {
          expected_version: action.factor.version,
          reason,
        },
        grantToken,
      );
      setMessage(
        `${action.factor.display_name || formatIdentityMethod(action.factor.method)} disabled.`,
      );
    } else {
      const result = await api.identity.regenerateRecoveryCodes(
        action.factor.id,
        { reason },
        grantToken,
      );
      setCodes(result.recovery_codes);
      setMessage('Previous recovery codes were invalidated.');
    }
    setAction(null);
    setReason('');
    await queryClient.invalidateQueries({ queryKey: identityKeys.factors });
  }

  if (query.isLoading)
    return (
      <div
        role="status"
        aria-label="Loading multi-factor methods"
        className="grid gap-4 lg:grid-cols-2"
      >
        <Skeleton className="h-52" />
        <Skeleton className="h-52" />
      </div>
    );
  if (query.isError) return <ErrorCard error={query.error} retry={() => void query.refetch()} />;
  const factors = query.data?.data ?? [];
  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-xl font-semibold">Multi-factor authentication</h2>
          <p className="text-sm text-muted-foreground">
            Authenticator apps and one-time recovery codes protect password sign-in and sensitive
            changes.
          </p>
        </div>
        <Button
          type="button"
          onClick={() => {
            setEnrollmentOpen(true);
            setMessage('');
          }}
        >
          <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
          Add authenticator app
        </Button>
      </div>
      {message && (
        <p role="status" className="rounded-md bg-emerald-500/10 p-3 text-sm">
          {message}
        </p>
      )}
      {factors.length === 0 ? (
        <Card>
          <EmptyState
            icon={KeyRound}
            title="No authenticator app enrolled"
            description="Add an authenticator app to enable one-time codes and generate recovery codes."
          />
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {factors.map((factor) => (
            <FactorCard
              key={factor.id}
              factor={factor}
              onAction={(kind) => {
                setAction({ factor, kind });
                setReason('');
                setMessage('');
              }}
            />
          ))}
        </div>
      )}
      <TOTPEnrollmentDialog
        open={enrollmentOpen}
        onOpenChange={setEnrollmentOpen}
        onVerified={async (recoveryCodes) => {
          setCodes(recoveryCodes);
          setEnrollmentOpen(false);
          setMessage('Authenticator app enrolled.');
          await queryClient.invalidateQueries({ queryKey: identityKeys.factors });
        }}
      />
      <Dialog
        open={Boolean(action) && !stepUpOpen}
        onOpenChange={(open) => {
          if (!open) {
            setAction(null);
            setReason('');
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {action?.kind === 'disable'
                ? 'Disable authenticator factor'
                : 'Replace recovery codes'}
            </DialogTitle>
            <DialogDescription>
              {action?.kind === 'disable'
                ? 'You will need another allowed verified method when organization policy requires MFA. The server protects the last required factor.'
                : 'All existing recovery codes will stop working immediately. Save the replacement set before leaving.'}
            </DialogDescription>
          </DialogHeader>
          <label className="block space-y-2">
            <span className="text-sm font-medium">Reason</span>
            <Input
              autoFocus
              maxLength={1000}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
            <span className="block text-xs text-muted-foreground">
              Required for security history (3–1000 characters).
            </span>
          </label>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setAction(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              variant={action?.kind === 'disable' ? 'destructive' : 'default'}
              disabled={!identityReasonValid(reason)}
              onClick={() => setStepUpOpen(true)}
            >
              Continue to verification
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {action && (
        <StepUpDialog
          open={stepUpOpen}
          onOpenChange={(open) => {
            setStepUpOpen(open);
            if (!open && action) {
              setAction(null);
              setReason('');
            }
          }}
          purpose="mfa_change"
          title="Verify this security change"
          description={
            action.kind === 'disable'
              ? 'Confirm your identity before disabling this factor.'
              : 'Confirm your identity before replacing all recovery codes.'
          }
          onGranted={protectedAction}
        />
      )}
      <RecoveryCodesDialog key={codes.join('|')} codes={codes} onClose={() => setCodes([])} />
    </div>
  );
}

function FactorCard({
  factor,
  onAction,
}: {
  factor: IdentityMFAFactor;
  onAction: (kind: 'disable' | 'recovery') => void;
}) {
  const active = factor.is_verified && !factor.disabled_at;
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="flex gap-2">
            <ShieldCheck aria-hidden="true" className="mt-0.5 h-5 w-5" />
            <div>
              <CardTitle className="text-base">
                {factor.display_name || formatIdentityMethod(factor.method)}
              </CardTitle>
              <CardDescription className="mt-1">
                Added {formatDate(factor.created_at)}
              </CardDescription>
            </div>
          </div>
          <Badge variant={active ? 'outline' : 'secondary'}>
            {factor.disabled_at
              ? 'Disabled'
              : factor.is_verified
                ? 'Verified'
                : 'Enrollment incomplete'}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <dl className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <dt className="text-xs text-muted-foreground">Last used</dt>
            <dd>{factor.last_used_at ? formatDate(factor.last_used_at) : 'Never'}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Recovery codes</dt>
            <dd>{factor.recovery_codes_remaining}</dd>
          </div>
        </dl>
        {active && factor.recovery_codes_remaining <= 2 && (
          <p className="flex gap-2 rounded-md bg-amber-500/10 p-2 text-xs">
            <AlertTriangle aria-hidden="true" className="h-4 w-4 shrink-0" />
            Recovery codes are running low. Replace them before you need account recovery.
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          {active && factor.method === 'totp' && (
            <Button type="button" size="sm" variant="outline" onClick={() => onAction('recovery')}>
              Replace recovery codes
            </Button>
          )}
          {active && (
            <Button
              type="button"
              size="sm"
              variant="destructive"
              onClick={() => onAction('disable')}
            >
              Disable factor
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function TOTPEnrollmentDialog({
  onOpenChange,
  onVerified,
  open,
}: {
  onOpenChange: (open: boolean) => void;
  onVerified: (codes: string[]) => Promise<void>;
  open: boolean;
}) {
  const [displayName, setDisplayName] = React.useState('Authenticator app');
  const [reason, setReason] = React.useState('Add authenticator app');
  const [code, setCode] = React.useState('');
  const [enrollment, setEnrollment] = React.useState<IdentityTOTPEnrollment | null>(null);
  const [error, setError] = React.useState('');
  const [copyMessage, setCopyMessage] = React.useState('');
  const begin = useMutation({
    mutationFn: () =>
      api.identity.beginTOTPEnrollment({ display_name: displayName || undefined, reason }),
    onSuccess: setEnrollment,
    onError: (caught) => setError(formatApiError(caught, 'Enrollment could not be started.')),
  });
  const verify = useMutation({
    mutationFn: () =>
      api.identity.verifyTOTPEnrollment({
        factor_id: enrollment!.factor_id,
        expected_version: enrollment!.expected_version,
        code,
        reason,
      }),
    onSuccess: async (result) => {
      setEnrollment(null);
      setCode('');
      await onVerified(result.recovery_codes);
    },
    onError: (caught) =>
      setError(formatApiError(caught, 'The authenticator code was not accepted.')),
  });
  function close() {
    if (begin.isPending || verify.isPending) return;
    setEnrollment(null);
    setCode('');
    setError('');
    setCopyMessage('');
    onOpenChange(false);
  }
  async function copySecret() {
    if (!enrollment) return;
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard unavailable');
      await navigator.clipboard.writeText(enrollment.secret);
      setCopyMessage('Setup key copied.');
    } catch {
      setCopyMessage('Setup key could not be copied. Select it manually.');
    }
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
    >
      <DialogContent
        onPointerDownOutside={(event) => {
          if (enrollment) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>Add an authenticator app</DialogTitle>
          <DialogDescription>
            {enrollment
              ? 'Enter this secret manually in your authenticator, then verify a current six-digit code. The secret exists only in this dialog.'
              : 'Give this factor a recognizable name. The reason is recorded in your security history.'}
          </DialogDescription>
        </DialogHeader>
        {error && (
          <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </p>
        )}
        {enrollment ? (
          <div className="space-y-4">
            <div className="rounded-md border bg-muted/40 p-4">
              <p className="text-xs text-muted-foreground">Manual setup key</p>
              <p className="mt-1 break-all font-mono text-sm">{enrollment.secret}</p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="mt-3"
                onClick={() => void copySecret()}
              >
                Copy setup key
              </Button>
              {copyMessage && (
                <p role="status" className="mt-2 text-xs text-muted-foreground">
                  {copyMessage}
                </p>
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              Enrollment expires {new Date(enrollment.expires_at).toLocaleString()}.
            </p>
            <label className="block space-y-2">
              <span className="text-sm font-medium">Six-digit code</span>
              <Input
                autoFocus
                inputMode="numeric"
                autoComplete="one-time-code"
                value={code}
                onChange={(event) => setCode(event.target.value)}
              />
            </label>
          </div>
        ) : (
          <div className="space-y-4">
            <label className="block space-y-2">
              <span className="text-sm font-medium">Factor name</span>
              <Input
                autoFocus
                maxLength={120}
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
              />
            </label>
            <label className="block space-y-2">
              <span className="text-sm font-medium">Reason</span>
              <Input
                maxLength={1000}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              />
            </label>
          </div>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={begin.isPending || verify.isPending}
            onClick={close}
          >
            {enrollment ? 'Cancel enrollment' : 'Cancel'}
          </Button>
          {enrollment ? (
            <Button
              type="button"
              disabled={verify.isPending || !/^\d{6}$/.test(code)}
              onClick={() => verify.mutate()}
            >
              {verify.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              Verify and finish
            </Button>
          ) : (
            <Button
              type="button"
              disabled={
                begin.isPending || !identityReasonValid(reason) || displayName.trim().length < 2
              }
              onClick={() => {
                setError('');
                begin.mutate();
              }}
            >
              {begin.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              Generate setup key
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ErrorCard({ error, retry }: { error: unknown; retry: () => void }) {
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <RefreshCw aria-hidden="true" className="h-8 w-8" />
        <h2 className="font-semibold">MFA methods could not be loaded</h2>
        <p role="alert" className="text-sm text-muted-foreground">
          {formatApiError(error, 'MFA methods could not be loaded.')}
        </p>
        <Button type="button" variant="outline" onClick={retry}>
          Try again
        </Button>
      </CardContent>
    </Card>
  );
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}
