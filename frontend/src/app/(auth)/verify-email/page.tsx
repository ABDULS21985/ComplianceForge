'use client';

import * as React from 'react';

import { CheckCircle2, Loader2, MailCheck } from 'lucide-react';
import {
  organizationIdentitySchema,
  type OrganizationIdentityValues,
} from '@/lib/identity-validation';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { formatApiError } from '@/lib/enterprise-settings';
import { Input } from '@/components/ui/input';
import Link from 'next/link';
import { ROUTES } from '@/lib/routes';
import { useEphemeralQueryToken } from '@/components/identity/use-ephemeral-token';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';

export default function VerifyEmailPage() {
  const token = useEphemeralQueryToken();
  const [error, setError] = React.useState('');
  const [outcome, setOutcome] = React.useState<'confirmed' | 'requested' | null>(null);
  const [confirming, setConfirming] = React.useState(false);
  const form = useForm<OrganizationIdentityValues>({
    resolver: zodResolver(organizationIdentitySchema),
    defaultValues: { email: '', organization_id: '' },
  });

  async function confirm() {
    setConfirming(true);
    setError('');
    try {
      await api.auth.confirmEmailVerification({ token });
      setOutcome('confirmed');
    } catch (caught) {
      setError(formatApiError(caught, 'Email verification failed.'));
    } finally {
      setConfirming(false);
    }
  }

  async function request(values: OrganizationIdentityValues) {
    setError('');
    try {
      await api.auth.requestEmailVerification(values);
      setOutcome('requested');
    } catch (caught) {
      setError(formatApiError(caught, 'A verification email could not be requested.'));
    }
  }

  if (outcome === 'confirmed') {
    return (
      <Outcome
        icon={CheckCircle2}
        title="Email verified"
        description="Your email address is verified. You can now sign in."
      />
    );
  }
  if (outcome === 'requested') {
    return (
      <Outcome
        icon={MailCheck}
        title="Check your email"
        description="If the account is eligible, a fresh verification link has been queued."
      />
    );
  }
  return (
    <div className="space-y-6">
      <div className="space-y-2 text-center">
        <h1 className="text-lg font-semibold">Verify your email</h1>
        <p className="text-sm text-muted-foreground">
          {token
            ? 'Confirm this one-time verification request. The credential has been removed from the address bar.'
            : 'Request a new verification link without revealing whether an account exists.'}
        </p>
      </div>
      {error && (
        <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </p>
      )}
      {token ? (
        <Button
          className="w-full"
          type="button"
          onClick={() => void confirm()}
          disabled={confirming}
        >
          {confirming && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Verify
          email
        </Button>
      ) : (
        <form className="space-y-4" onSubmit={form.handleSubmit(request)} noValidate>
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
          <Button className="w-full" type="submit" disabled={form.formState.isSubmitting}>
            {form.formState.isSubmitting && (
              <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
            )}
            Send verification link
          </Button>
        </form>
      )}
    </div>
  );
}

function Outcome({
  description,
  icon: Icon,
  title,
}: {
  description: string;
  icon: typeof MailCheck;
  title: string;
}) {
  return (
    <div className="space-y-5 text-center">
      <Icon aria-hidden="true" className="mx-auto h-12 w-12 text-primary" />
      <div>
        <h1 className="text-lg font-semibold">{title}</h1>
        <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      </div>
      <Button asChild variant="outline" className="w-full">
        <Link href={ROUTES.auth.login}>Go to sign in</Link>
      </Button>
    </div>
  );
}

function Field({
  children,
  error,
  label,
}: {
  children: React.ReactNode;
  error?: string;
  label: string;
}) {
  return (
    <label className="block space-y-2">
      <span className="text-sm font-medium">{label}</span>
      {children}
      {error && (
        <span role="alert" className="block text-xs text-destructive">
          {error}
        </span>
      )}
    </label>
  );
}
