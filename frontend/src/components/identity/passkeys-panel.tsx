'use client';

import * as React from 'react';

import { AlertTriangle, Fingerprint, Loader2, Plus, RefreshCw } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { createPasskeyCredential, identityKeys, identityReasonValid } from '@/lib/identity';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/data/empty-state';
import { formatApiError } from '@/lib/enterprise-settings';
import type { IdentityPasskey } from '@/types/identity';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { StepUpDialog } from '@/components/identity/step-up-dialog';
import { useWebAuthnSupport } from '@/hooks/use-webauthn-support';

export function PasskeysPanel() {
  const queryClient = useQueryClient();
  const supported = useWebAuthnSupport();
  const [registerOpen, setRegisterOpen] = React.useState(false);
  const [deviceName, setDeviceName] = React.useState('');
  const [selected, setSelected] = React.useState<IdentityPasskey | null>(null);
  const [reason, setReason] = React.useState('');
  const [stepUpOpen, setStepUpOpen] = React.useState(false);
  const [error, setError] = React.useState('');
  const [message, setMessage] = React.useState('');
  const query = useQuery({
    queryKey: identityKeys.passkeys,
    queryFn: () => api.identity.listPasskeys(),
    staleTime: 30_000,
  });
  const registration = useMutation({
    mutationFn: async () => {
      const ceremony = await api.identity.beginPasskeyRegistration({
        device_name: deviceName.trim(),
      });
      const credential = await createPasskeyCredential(ceremony.options);
      return api.identity.verifyPasskeyRegistration({
        challenge_token: ceremony.challenge_token,
        credential,
        device_name: deviceName.trim(),
      });
    },
    onSuccess: async () => {
      setRegisterOpen(false);
      setDeviceName('');
      setMessage('Passkey registered.');
      await queryClient.invalidateQueries({ queryKey: identityKeys.passkeys });
    },
  });

  async function remove(grantToken: string) {
    if (!selected) return;
    await api.identity.removePasskey(
      selected.id,
      {
        expected_version: selected.version,
        reason,
      },
      grantToken,
    );
    setSelected(null);
    setReason('');
    setMessage('Passkey removed.');
    await queryClient.invalidateQueries({ queryKey: identityKeys.passkeys });
  }

  if (query.isLoading)
    return (
      <div role="status" aria-label="Loading passkeys" className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-52" />
        <Skeleton className="h-52" />
      </div>
    );
  if (query.isError)
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
          <RefreshCw aria-hidden="true" className="h-8 w-8" />
          <h2 className="font-semibold">Passkeys could not be loaded</h2>
          <p role="alert" className="text-sm text-muted-foreground">
            {formatApiError(query.error, 'Passkeys could not be loaded.')}
          </p>
          <Button type="button" variant="outline" onClick={() => void query.refetch()}>
            Try again
          </Button>
        </CardContent>
      </Card>
    );
  const passkeys = query.data?.data ?? [];
  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-xl font-semibold">Passkeys</h2>
          <p className="text-sm text-muted-foreground">
            Use phishing-resistant authentication backed by this device, a security key, or your
            synced credential provider.
          </p>
        </div>
        <Button
          type="button"
          disabled={!supported}
          onClick={() => {
            setRegisterOpen(true);
            setMessage('');
          }}
        >
          <Plus aria-hidden="true" className="mr-2 h-4 w-4" />
          Register passkey
        </Button>
      </div>
      {!supported && (
        <p role="status" className="rounded-md bg-amber-500/10 p-3 text-sm">
          Passkey operations require a supported browser in a secure HTTPS context.
        </p>
      )}
      {message && (
        <p role="status" className="rounded-md bg-emerald-500/10 p-3 text-sm">
          {message}
        </p>
      )}
      {passkeys.length === 0 ? (
        <Card>
          <EmptyState
            icon={Fingerprint}
            title="No passkeys registered"
            description="Register a passkey to add a phishing-resistant sign-in method."
          />
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {passkeys.map((passkey) => (
            <PasskeyCard
              key={passkey.id}
              passkey={passkey}
              onRemove={() => {
                setSelected(passkey);
                setReason('');
                setError('');
              }}
            />
          ))}
        </div>
      )}
      <Dialog
        open={registerOpen}
        onOpenChange={(open) => {
          if (!registration.isPending) {
            setRegisterOpen(open);
            registration.reset();
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Register a passkey</DialogTitle>
            <DialogDescription>
              Your browser will ask you to use a device credential or security key. The ceremony
              challenge is kept only in memory.
            </DialogDescription>
          </DialogHeader>
          {registration.isError && (
            <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {formatApiError(registration.error, 'Passkey registration failed.')}
            </p>
          )}
          <label className="block space-y-2">
            <span className="text-sm font-medium">Device name</span>
            <Input
              autoFocus
              maxLength={120}
              placeholder="Work MacBook"
              value={deviceName}
              onChange={(event) => setDeviceName(event.target.value)}
            />
            <span className="block text-xs text-muted-foreground">
              Use 2–120 characters so you can recognize it later.
            </span>
          </label>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={registration.isPending}
              onClick={() => setRegisterOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              disabled={registration.isPending || deviceName.trim().length < 2 || !supported}
              onClick={() => registration.mutate()}
            >
              {registration.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              Continue in browser
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={Boolean(selected) && !stepUpOpen}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null);
            setReason('');
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove passkey</DialogTitle>
            <DialogDescription>
              This passkey will stop working immediately. Ensure another allowed sign-in method
              remains available.
            </DialogDescription>
          </DialogHeader>
          {error && (
            <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </p>
          )}
          <label className="block space-y-2">
            <span className="text-sm font-medium">Reason</span>
            <Input
              autoFocus
              maxLength={1000}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          </label>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setSelected(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={!identityReasonValid(reason)}
              onClick={() => setStepUpOpen(true)}
            >
              Continue to verification
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {selected && (
        <StepUpDialog
          open={stepUpOpen}
          onOpenChange={(open) => {
            setStepUpOpen(open);
            if (!open && selected) {
              setSelected(null);
              setReason('');
            }
          }}
          purpose="mfa_change"
          title="Verify passkey removal"
          description={`Confirm your identity before removing ${selected.device_name}.`}
          onGranted={async (token) => {
            try {
              await remove(token);
            } catch (caught) {
              setError(formatApiError(caught, 'Passkey removal failed.'));
              throw caught;
            }
          }}
        />
      )}
    </div>
  );
}

function PasskeyCard({ onRemove, passkey }: { onRemove: () => void; passkey: IdentityPasskey }) {
  const active = !passkey.removed_at;
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="flex gap-2">
            <Fingerprint aria-hidden="true" className="mt-0.5 h-5 w-5" />
            <div>
              <CardTitle className="text-base">{passkey.device_name}</CardTitle>
              <CardDescription className="mt-1">
                Registered {formatDate(passkey.created_at)}
              </CardDescription>
            </div>
          </div>
          <Badge variant={active ? 'outline' : 'secondary'}>{active ? 'Active' : 'Removed'}</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <dl className="grid grid-cols-2 gap-3">
          <div>
            <dt className="text-xs text-muted-foreground">Last used</dt>
            <dd>{passkey.last_used_at ? formatDate(passkey.last_used_at) : 'Never'}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Backup posture</dt>
            <dd>
              {passkey.backup_eligible
                ? passkey.backup_state
                  ? 'Backed up'
                  : 'Eligible, not backed up'
                : 'Device-bound'}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Transports</dt>
            <dd>{passkey.transports.join(', ') || 'Unspecified'}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Sign counter</dt>
            <dd>{passkey.sign_count}</dd>
          </div>
        </dl>
        {passkey.clone_warning && (
          <p className="flex gap-2 rounded-md bg-destructive/10 p-2 text-xs text-destructive">
            <AlertTriangle aria-hidden="true" className="h-4 w-4 shrink-0" />
            Authenticator clone warning detected. Remove this credential and review security
            history.
          </p>
        )}
        <Button type="button" size="sm" variant="destructive" disabled={!active} onClick={onRemove}>
          Remove passkey
        </Button>
      </CardContent>
    </Card>
  );
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}
