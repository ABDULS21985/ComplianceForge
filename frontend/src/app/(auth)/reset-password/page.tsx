'use client';

import * as React from 'react';

import { CheckCircle2, KeyRound, Loader2 } from 'lucide-react';
import { newPasswordSchema, type NewPasswordValues } from '@/lib/identity-validation';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { formatApiError } from '@/lib/enterprise-settings';
import { Input } from '@/components/ui/input';
import Link from 'next/link';
import { ROUTES } from '@/lib/routes';
import { useEphemeralQueryToken } from '@/components/identity/use-ephemeral-token';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';

export default function ResetPasswordPage() {
  const token = useEphemeralQueryToken();
  const [error, setError] = React.useState('');
  const [complete, setComplete] = React.useState(false);
  const form = useForm<NewPasswordValues>({
    resolver: zodResolver(newPasswordSchema),
    defaultValues: { confirm_password: '', password: '' },
  });

  async function submit(values: NewPasswordValues) {
    if (!token) return;
    setError('');
    try {
      await api.auth.resetPassword({ token, new_password: values.password });
      form.reset();
      setComplete(true);
    } catch (caught) {
      setError(formatApiError(caught, 'The password could not be reset.'));
    }
  }

  if (!token) {
    return (
      <Outcome
        icon={KeyRound}
        title="Reset link unavailable"
        description="This link is missing its one-time reset credential. Request a new password reset email."
        href={ROUTES.auth.forgotPassword}
        action="Request a new link"
      />
    );
  }
  if (complete) {
    return (
      <Outcome
        icon={CheckCircle2}
        title="Password reset complete"
        description="Your other sessions were revoked. Sign in again with your new password."
        href={ROUTES.auth.login}
        action="Sign in"
      />
    );
  }
  return (
    <div className="space-y-6">
      <div className="space-y-2 text-center">
        <h1 className="text-lg font-semibold">Choose a new password</h1>
        <p className="text-sm text-muted-foreground">
          The one-time reset credential has been removed from the address bar and is held only for
          this page.
        </p>
      </div>
      {error && (
        <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </p>
      )}
      <form className="space-y-4" onSubmit={form.handleSubmit(submit)} noValidate>
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
          Reset password
        </Button>
      </form>
    </div>
  );
}

function Outcome({
  action,
  description,
  href,
  icon: Icon,
  title,
}: {
  action: string;
  description: string;
  href: string;
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
        <Link href={href}>{action}</Link>
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
