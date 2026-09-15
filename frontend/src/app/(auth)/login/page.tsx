'use client';

import * as React from 'react';

import { AUTH_REDIRECT_QUERY_PARAM, getSafePostAuthRedirect, ROUTES } from '@/lib/routes';
import { Fingerprint, KeyRound, Loader2, MailCheck } from 'lucide-react';
import {
  formatIdentityMethod,
  getPasskeyCredential,
  isAuthenticatedResponse,
  isPendingEmailVerificationResponse,
  isPendingMFAResponse,
} from '@/lib/identity';
import type { IdentityMethod, IdentityMFAChallenge } from '@/types/identity';
import { loginIdentitySchema, type LoginIdentityValues } from '@/lib/identity-validation';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { formatApiError } from '@/lib/enterprise-settings';
import { Input } from '@/components/ui/input';
import Link from 'next/link';
import { useAuthStore } from '@/store/auth-store';
import { useForm } from 'react-hook-form';
import type { User } from '@/types';
import { useRouter } from 'next/navigation';
import { useWebAuthnSupport } from '@/hooks/use-webauthn-support';
import { zodResolver } from '@hookform/resolvers/zod';

export default function LoginPage() {
  const router = useRouter();
  const setAuth = useAuthStore((state) => state.setAuth);
  const [apiError, setApiError] = React.useState('');
  const [challenge, setChallenge] = React.useState<IdentityMFAChallenge | null>(null);
  const [pendingUser, setPendingUser] = React.useState<User | null>(null);
  const [verificationRequired, setVerificationRequired] = React.useState(false);
  const passkeyAvailable = useWebAuthnSupport();
  const [passkeyPending, setPasskeyPending] = React.useState(false);
  const form = useForm<LoginIdentityValues>({
    resolver: zodResolver(loginIdentitySchema),
    defaultValues: { email: '', organization_id: '', password: '' },
  });

  function complete(user: User) {
    setAuth(user);
    const params = new URLSearchParams(window.location.search);
    router.replace(getSafePostAuthRedirect(params.get(AUTH_REDIRECT_QUERY_PARAM)));
  }

  async function submit(values: LoginIdentityValues) {
    setApiError('');
    try {
      const response = await api.auth.login(values);
      form.resetField('password');
      if (isPendingMFAResponse(response)) {
        setPendingUser(response.user);
        setChallenge(response.mfa_challenge);
        return;
      }
      if (isPendingEmailVerificationResponse(response)) {
        setPendingUser(response.user);
        setVerificationRequired(true);
        return;
      }
      if (isAuthenticatedResponse(response)) complete(response.user);
    } catch (caught) {
      setApiError(formatApiError(caught, 'Sign-in failed. Check your credentials and try again.'));
    }
  }

  async function signInWithPasskey() {
    const valid = await form.trigger(['organization_id', 'email']);
    if (!valid) return;
    const values = form.getValues();
    setPasskeyPending(true);
    setApiError('');
    try {
      const ceremony = await api.auth.beginPasskeyAuthentication({
        organization_id: values.organization_id,
        email: values.email,
        purpose: 'login',
      });
      const credential = await getPasskeyCredential(ceremony.options);
      const response = await api.auth.verifyPasskeyAuthentication({
        challenge_token: ceremony.challenge_token,
        credential,
      });
      complete(response.user);
    } catch (caught) {
      setApiError(formatApiError(caught, 'Passkey sign-in could not be completed.'));
    } finally {
      setPasskeyPending(false);
    }
  }

  if (challenge && pendingUser) {
    return (
      <LoginMFAChallenge
        challenge={challenge}
        onCancel={() => {
          setChallenge(null);
          setPendingUser(null);
        }}
        onComplete={(user) => complete(user)}
      />
    );
  }
  if (verificationRequired) {
    return (
      <div className="space-y-5 text-center">
        <MailCheck aria-hidden="true" className="mx-auto h-12 w-12 text-primary" />
        <div>
          <h1 className="text-lg font-semibold">Verify your email</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            This account must be verified before sign-in. Use the link sent during registration or
            request a fresh one.
          </p>
        </div>
        <Button asChild className="w-full">
          <Link href={ROUTES.auth.verifyEmail}>Request verification</Link>
        </Button>
        <Button
          type="button"
          variant="ghost"
          className="w-full"
          onClick={() => {
            setVerificationRequired(false);
            setPendingUser(null);
          }}
        >
          Back to sign in
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="space-y-2 text-center">
        <h1 className="text-lg font-semibold">Welcome back</h1>
        <p className="text-sm text-muted-foreground">Sign in to your organization securely.</p>
      </div>
      {apiError && (
        <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {apiError}
        </p>
      )}
      <form onSubmit={form.handleSubmit(submit)} className="space-y-4" noValidate>
        <Field label="Organization ID" error={form.formState.errors.organization_id?.message}>
          <Input
            autoComplete="organization"
            spellCheck={false}
            {...form.register('organization_id')}
          />
        </Field>
        <Field label="Email address" error={form.formState.errors.email?.message}>
          <Input type="email" autoComplete="email" {...form.register('email')} />
        </Field>
        <Field
          label="Password"
          error={form.formState.errors.password?.message}
          trailing={
            <Link
              href={ROUTES.auth.forgotPassword}
              className="text-xs text-primary hover:underline"
            >
              Forgot password?
            </Link>
          }
        >
          <Input type="password" autoComplete="current-password" {...form.register('password')} />
        </Field>
        <Button
          type="submit"
          className="w-full"
          disabled={form.formState.isSubmitting || passkeyPending}
        >
          {form.formState.isSubmitting && (
            <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
          )}
          Sign in with password
        </Button>
      </form>
      <div className="relative">
        <div className="absolute inset-0 flex items-center">
          <span className="w-full border-t" />
        </div>
        <div className="relative flex justify-center text-xs uppercase">
          <span className="bg-card px-2 text-muted-foreground">or</span>
        </div>
      </div>
      <Button
        type="button"
        variant="outline"
        className="w-full"
        disabled={!passkeyAvailable || passkeyPending || form.formState.isSubmitting}
        onClick={() => void signInWithPasskey()}
      >
        {passkeyPending ? (
          <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <Fingerprint aria-hidden="true" className="mr-2 h-4 w-4" />
        )}
        Sign in with passkey
      </Button>
      {!passkeyAvailable && (
        <p className="text-center text-xs text-muted-foreground">
          Passkey sign-in is unavailable in this browser or context.
        </p>
      )}
    </div>
  );
}

function LoginMFAChallenge({
  challenge,
  onCancel,
  onComplete,
}: {
  challenge: IdentityMFAChallenge;
  onCancel: () => void;
  onComplete: (user: User) => void;
}) {
  const methods = challenge.methods;
  const [method, setMethod] = React.useState<Exclude<IdentityMethod, 'password'>>(
    methods[0] ?? 'totp',
  );
  const [code, setCode] = React.useState('');
  const [error, setError] = React.useState('');
  const [pending, setPending] = React.useState(false);
  const passkeyAvailable = useWebAuthnSupport();

  async function verify() {
    setPending(true);
    setError('');
    try {
      const credential =
        method === 'passkey'
          ? await getPasskeyCredential(challenge.passkey_options ?? {})
          : undefined;
      const response = await api.auth.verifyMFA({
        challenge_token: challenge.challenge_token,
        method,
        code: method === 'passkey' ? undefined : code,
        credential,
      });
      setCode('');
      onComplete(response.user);
    } catch (caught) {
      setError(formatApiError(caught, 'The verification proof was not accepted.'));
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="space-y-5">
      <div className="text-center">
        <KeyRound aria-hidden="true" className="mx-auto h-10 w-10 text-primary" />
        <h1 className="mt-3 text-lg font-semibold">Additional verification required</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          This challenge expires {new Date(challenge.expires_at).toLocaleString()}.
        </p>
      </div>
      {error && (
        <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </p>
      )}
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
            <option key={item} value={item} disabled={item === 'passkey' && !passkeyAvailable}>
              {formatIdentityMethod(item)}
            </option>
          ))}
        </select>
      </label>
      {method !== 'passkey' && (
        <Field label={method === 'totp' ? 'Six-digit authenticator code' : 'Recovery code'}>
          <Input
            autoFocus
            inputMode={method === 'totp' ? 'numeric' : 'text'}
            autoComplete="one-time-code"
            value={code}
            onChange={(event) => setCode(event.target.value)}
          />
        </Field>
      )}
      {method === 'passkey' && !passkeyAvailable && (
        <p role="alert" className="text-sm text-destructive">
          Passkeys are not available in this browser. Choose another method.
        </p>
      )}
      <Button
        className="w-full"
        type="button"
        disabled={
          pending ||
          (method !== 'passkey' && code.trim().length === 0) ||
          (method === 'passkey' && !passkeyAvailable)
        }
        onClick={() => void verify()}
      >
        {pending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
        {method === 'passkey' ? 'Use passkey' : 'Verify and sign in'}
      </Button>
      <Button
        className="w-full"
        type="button"
        variant="ghost"
        disabled={pending}
        onClick={onCancel}
      >
        Cancel and start again
      </Button>
    </div>
  );
}

function Field({
  children,
  error,
  label,
  trailing,
}: {
  children: React.ReactNode;
  error?: string;
  label: string;
  trailing?: React.ReactNode;
}) {
  return (
    <label className="block space-y-2">
      <span className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{label}</span>
        {trailing}
      </span>
      {children}
      {error && (
        <span role="alert" className="block text-xs text-destructive">
          {error}
        </span>
      )}
    </label>
  );
}
