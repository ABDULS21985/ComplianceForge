'use client';

import * as React from 'react';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { formatIdentityMethod, getPasskeyCredential } from '@/lib/identity';
import type { IdentityMethod, IdentityMFAChallenge } from '@/types/identity';
import { Loader2, ShieldCheck } from 'lucide-react';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { formatApiError } from '@/lib/enterprise-settings';
import { Input } from '@/components/ui/input';
import { useWebAuthnSupport } from '@/hooks/use-webauthn-support';

interface StepUpDialogProps {
  description: string;
  onGranted: (grantToken: string) => Promise<void>;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  purpose: string;
  title: string;
}

export function StepUpDialog({
  description,
  onGranted,
  onOpenChange,
  open,
  purpose,
  title,
}: StepUpDialogProps) {
  const [challenge, setChallenge] = React.useState<IdentityMFAChallenge | null>(null);
  const [method, setMethod] = React.useState<Exclude<IdentityMethod, 'password'>>('totp');
  const [code, setCode] = React.useState('');
  const [error, setError] = React.useState('');
  const [pending, setPending] = React.useState(true);

  React.useEffect(() => {
    if (!open) return;
    let active = true;
    api.identity
      .beginStepUp({ purpose })
      .then((response) => {
        if (!active) return;
        setChallenge(response);
        const available = response.methods[0];
        if (available) setMethod(available);
        setPending(false);
      })
      .catch((caught) => {
        if (!active) return;
        setError(formatApiError(caught, 'Step-up authentication could not be started.'));
        setPending(false);
      });
    return () => {
      active = false;
    };
  }, [open, purpose]);

  function close() {
    setChallenge(null);
    setCode('');
    setError('');
    setPending(true);
    onOpenChange(false);
  }

  async function verify() {
    if (!challenge) return;
    setPending(true);
    setError('');
    try {
      const credential =
        method === 'passkey'
          ? await getPasskeyCredential(challenge.passkey_options ?? {})
          : undefined;
      const grant = await api.identity.verifyStepUp({
        challenge_token: challenge.challenge_token,
        method,
        code: method === 'passkey' ? undefined : code,
        credential,
      });
      setChallenge(null);
      setCode('');
      await onGranted(grant.grant_token);
      close();
    } catch (caught) {
      setError(formatApiError(caught, 'Verification or the protected action failed.'));
      setPending(false);
    }
  }

  const methods = challenge?.methods ?? [];
  const passkeysAvailable = useWebAuthnSupport();
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheck aria-hidden="true" className="h-5 w-5" />
            {title}
          </DialogTitle>
          <DialogDescription>
            {description} This proof is single-use and is never stored in the browser.
          </DialogDescription>
        </DialogHeader>
        {error && (
          <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </p>
        )}
        {!challenge && pending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
            Preparing verification methods…
          </p>
        )}
        {challenge && (
          <div className="space-y-4">
            <label className="block space-y-2">
              <span className="text-sm font-medium">Verification method</span>
              <select
                className="h-10 w-full rounded-md border bg-background px-3 text-sm"
                value={method}
                onChange={(event) => {
                  setMethod(event.target.value as Exclude<IdentityMethod, 'password'>);
                  setCode('');
                }}
              >
                {methods.map((item) => (
                  <option
                    key={item}
                    value={item}
                    disabled={item === 'passkey' && !passkeysAvailable}
                  >
                    {formatIdentityMethod(item)}
                  </option>
                ))}
              </select>
            </label>
            {method !== 'passkey' && (
              <label className="block space-y-2">
                <span className="text-sm font-medium">
                  {method === 'totp' ? 'Six-digit authenticator code' : 'Recovery code'}
                </span>
                <Input
                  autoFocus
                  autoComplete="one-time-code"
                  inputMode={method === 'totp' ? 'numeric' : 'text'}
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                />
              </label>
            )}
            <p className="text-xs text-muted-foreground">
              Challenge expires {new Date(challenge.expires_at).toLocaleString()}.
            </p>
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" disabled={pending} onClick={close}>
            Cancel
          </Button>
          <Button
            type="button"
            disabled={
              !challenge ||
              pending ||
              (method !== 'passkey' && !code.trim()) ||
              (method === 'passkey' && !passkeysAvailable)
            }
            onClick={() => void verify()}
          >
            {pending && challenge && (
              <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
            )}
            Verify and continue
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
