'use client';

import * as React from 'react';

import { CheckCircle2, KeyRound, Loader2 } from 'lucide-react';
import {
  invitationAcceptanceSchema,
  type InvitationAcceptanceValues,
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

export default function AcceptInvitationPage() {
  const token = useEphemeralQueryToken();
  const [error, setError] = React.useState('');
  const [complete, setComplete] = React.useState(false);
  const form = useForm<InvitationAcceptanceValues>({
    resolver: zodResolver(invitationAcceptanceSchema),
    defaultValues: { confirm_password: '', first_name: '', last_name: '', password: '' },
  });

  async function submit(values: InvitationAcceptanceValues) {
    if (!token) return;
    setError('');
    try {
      await api.auth.acceptInvitation({
        token,
        password: values.password,
        first_name: values.first_name || undefined,
        last_name: values.last_name || undefined,
      });
      form.reset();
      setComplete(true);
    } catch (caught) {
      setError(formatApiError(caught, 'This invitation could not be accepted.'));
    }
  }

  if (!token) {
    return (
      <AuthOutcome
        icon={KeyRound}
        title="Invitation link unavailable"
        description="This link is missing its one-time invitation credential. Ask your administrator to send a new invitation."
      />
    );
  }
  if (complete) {
    return (
      <AuthOutcome
        icon={CheckCircle2}
        title="Invitation accepted"
        description="Your password is set and your email is verified. Sign in to continue."
      />
    );
  }

  return (
    <div className="space-y-6">
      <div className="space-y-2 text-center">
        <h1 className="text-lg font-semibold">Accept your invitation</h1>
        <p className="text-sm text-muted-foreground">
          Create your account password. The invitation credential has been removed from the address
          bar and is held only for this page.
        </p>
      </div>
      {error && (
        <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </p>
      )}
      <form className="space-y-4" onSubmit={form.handleSubmit(submit)} noValidate>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="First name" error={form.formState.errors.first_name?.message}>
            <Input autoComplete="given-name" {...form.register('first_name')} />
          </Field>
          <Field label="Last name" error={form.formState.errors.last_name?.message}>
            <Input autoComplete="family-name" {...form.register('last_name')} />
          </Field>
        </div>
        <Field label="New password" error={form.formState.errors.password?.message}>
          <Input type="password" autoComplete="new-password" {...form.register('password')} />
        </Field>
        <Field label="Confirm password" error={form.formState.errors.confirm_password?.message}>
          <Input
            type="password"
            autoComplete="new-password"
            {...form.register('confirm_password')}
          />
        </Field>
        <p className="text-xs text-muted-foreground">
          Use 12–72 characters with no leading or trailing spaces.
        </p>
        <Button className="w-full" type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting && (
            <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
          )}
          Accept invitation
        </Button>
      </form>
    </div>
  );
}

function AuthOutcome({
  description,
  icon: Icon,
  title,
}: {
  description: string;
  icon: typeof KeyRound;
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
