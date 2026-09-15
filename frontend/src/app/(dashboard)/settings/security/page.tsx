'use client';

import { Card, CardContent } from '@/components/ui/card';
import { Fingerprint, KeyRound, Laptop, LockKeyhole, RefreshCw, Settings2 } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { MFAPanel } from '@/components/identity/mfa-panel';
import { PasskeysPanel } from '@/components/identity/passkeys-panel';
import { ROUTES } from '@/lib/routes';
import { SessionsPanel } from '@/components/identity/sessions-panel';
import { Skeleton } from '@/components/ui/skeleton';
import { useIdentityPermissions } from '@/hooks/use-identity-permissions';

export default function SecuritySettingsPage() {
  const permission = useIdentityPermissions();
  if (permission.isLoading) return <SecuritySkeleton />;
  if (permission.isError)
    return (
      <AccessState
        icon={RefreshCw}
        title="Security access could not be verified"
        description="The permission service is temporarily unavailable. No identity data was loaded."
      >
        <Button type="button" variant="outline" onClick={() => void permission.retry()}>
          Try again
        </Button>
      </AccessState>
    );
  if (!permission.canSelfService)
    return (
      <AccessState
        icon={LockKeyhole}
        title="Security settings unavailable"
        description="Your account does not currently have permission to read its own user security profile."
      />
    );
  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Security settings</h1>
          <p className="mt-1 max-w-3xl text-muted-foreground">
            Manage your signed-in devices, multi-factor methods, recovery credentials, and
            phishing-resistant passkeys.
          </p>
        </div>
        {permission.canReadAdministration && (
          <Button asChild variant="outline">
            <Link href={ROUTES.identityAdministration}>
              <Settings2 aria-hidden="true" className="mr-2 h-4 w-4" />
              Identity administration
            </Link>
          </Button>
        )}
      </div>
      <Card>
        <CardContent className="flex gap-3 py-4 text-sm">
          <KeyRound aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <p>
            Secrets, passkey challenges, step-up grants, and one-time recovery codes are held only
            while their workflow is open. They are never written to local storage.
          </p>
        </CardContent>
      </Card>
      <Tabs defaultValue="sessions" className="space-y-5">
        <TabsList className="h-auto max-w-full flex-wrap justify-start">
          <TabsTrigger value="sessions" className="gap-2">
            <Laptop aria-hidden="true" className="h-4 w-4" />
            Sessions
          </TabsTrigger>
          <TabsTrigger value="mfa" className="gap-2">
            <KeyRound aria-hidden="true" className="h-4 w-4" />
            Authenticator
          </TabsTrigger>
          <TabsTrigger value="passkeys" className="gap-2">
            <Fingerprint aria-hidden="true" className="h-4 w-4" />
            Passkeys
          </TabsTrigger>
        </TabsList>
        <TabsContent value="sessions">
          <SessionsPanel />
        </TabsContent>
        <TabsContent value="mfa">
          <MFAPanel />
        </TabsContent>
        <TabsContent value="passkeys">
          <PasskeysPanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function AccessState({
  children,
  description,
  icon: Icon,
  title,
}: {
  children?: React.ReactNode;
  description: string;
  icon: typeof LockKeyhole;
  title: string;
}) {
  return (
    <Card className="mx-auto max-w-xl">
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <Icon aria-hidden="true" className="h-10 w-10 text-muted-foreground" />
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="text-sm text-muted-foreground">{description}</p>
        {children}
      </CardContent>
    </Card>
  );
}

function SecuritySkeleton() {
  return (
    <div role="status" aria-label="Loading security settings" className="space-y-4">
      <Skeleton className="h-24" />
      <Skeleton className="h-14" />
      <div className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-56" />
        <Skeleton className="h-56" />
      </div>
    </div>
  );
}
