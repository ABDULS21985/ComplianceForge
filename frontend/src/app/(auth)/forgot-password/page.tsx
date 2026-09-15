'use client';

import * as React from 'react';

import { ArrowLeft, CheckCircle2, Loader2 } from 'lucide-react';
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
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';

export default function ForgotPasswordPage() {
  const [submitted, setSubmitted] = React.useState(false);
  const [error, setError] = React.useState('');
  const form = useForm<OrganizationIdentityValues>({
    resolver: zodResolver(organizationIdentitySchema),
    defaultValues: { email: '', organization_id: '' },
  });

  async function submit(values: OrganizationIdentityValues) {
    setError('');
    try {
      await api.auth.forgotPassword(values);
      setSubmitted(true);
    } catch (caught) {
      setError(formatApiError(caught, 'Password-reset delivery is temporarily unavailable.'));
    }
  }

  if (submitted) {
    return (
      <div className="space-y-6 text-center">
        <CheckCircle2 aria-hidden="true" className="mx-auto h-12 w-12 text-primary" />
        <div className="space-y-2">
          <h1 className="text-lg font-semibold">Check your email</h1>
          <p className="text-sm text-muted-foreground">
            If the account is eligible, password-reset instructions have been queued. This neutral
            response protects account privacy.
          </p>
        </div>
        <Button asChild variant="outline" className="w-full">
          <Link href={ROUTES.auth.login}><ArrowLeft aria-hidden="true" className="mr-2 h-4 w-4" />Back to sign in</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="space-y-2 text-center">
        <h1 className="text-lg font-semibold">Reset your password</h1>
        <p className="text-sm text-muted-foreground">
          Enter the organization ID and account email supplied by your administrator.
        </p>
      </div>
      {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
      <form className="space-y-4" onSubmit={form.handleSubmit(submit)} noValidate>
        <Field label="Organization ID" error={form.formState.errors.organization_id?.message}>
          <Input autoComplete="organization" spellCheck={false} {...form.register('organization_id')} />
        </Field>
        <Field label="Email address" error={form.formState.errors.email?.message}>
          <Input type="email" autoComplete="email" {...form.register('email')} />
        </Field>
        <Button type="submit" className="w-full" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
          Send reset link
        </Button>
      </form>
      <div className="text-center">
        <Link href={ROUTES.auth.login} className="inline-flex items-center text-sm text-primary hover:underline">
          <ArrowLeft aria-hidden="true" className="mr-1 h-3 w-3" />Back to sign in
        </Link>
      </div>
    </div>
  );
}

function Field({ children, error, label }: { children: React.ReactNode; error?: string; label: string }) {
  return <label className="block space-y-2"><span className="text-sm font-medium">{label}</span>{children}{error && <span role="alert" className="block text-xs text-destructive">{error}</span>}</label>;
}
