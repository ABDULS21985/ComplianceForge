'use client';

import * as React from 'react';

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
import { Laptop, Loader2, LogOut, RefreshCw, Smartphone } from 'lucide-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/data/empty-state';
import { formatApiError } from '@/lib/enterprise-settings';
import type { IdentitySession } from '@/types/identity';
import { Input } from '@/components/ui/input';
import { ROUTES } from '@/lib/routes';
import { Skeleton } from '@/components/ui/skeleton';
import { useAuthStore } from '@/store/auth-store';
import { useRouter } from 'next/navigation';

type SessionAction = { kind: 'revoke'; session: IdentitySession } | { kind: 'global' } | null;

export function SessionsPanel() {
  const router = useRouter();
  const clearAuth = useAuthStore((state) => state.clearAuth);
  const queryClient = useQueryClient();
  const [action, setAction] = React.useState<SessionAction>(null);
  const [reason, setReason] = React.useState('');
  const [exceptCurrent, setExceptCurrent] = React.useState(true);
  const [message, setMessage] = React.useState('');
  const query = useQuery({
    queryKey: identityKeys.sessions,
    queryFn: () => api.identity.listSessions(),
    staleTime: 30_000,
  });
  const mutation = useMutation({
    mutationFn: async () => {
      if (!action) return { signedOut: false, count: 0 };
      if (action.kind === 'revoke') {
        await api.identity.revokeSession(action.session.id, {
          expected_version: action.session.version,
          reason,
        });
        return { signedOut: action.session.current, count: 1 };
      }
      const result = await api.identity.globalSignOut({ except_current: exceptCurrent, reason });
      return { signedOut: !exceptCurrent, count: result.revoked_sessions };
    },
    onSuccess: async ({ count, signedOut }) => {
      setAction(null);
      setReason('');
      if (signedOut) {
        await api.auth.logout();
        clearAuth();
        router.replace(ROUTES.auth.login);
        return;
      }
      setMessage(`${count} session${count === 1 ? '' : 's'} revoked.`);
      await queryClient.invalidateQueries({ queryKey: identityKeys.sessions });
    },
  });

  if (query.isLoading) return <PanelSkeleton label="Loading signed-in devices" />;
  if (query.isError)
    return (
      <PanelError
        title="Sessions could not be loaded"
        error={query.error}
        retry={() => void query.refetch()}
      />
    );
  const sessions = query.data?.data ?? [];
  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-xl font-semibold">Sessions and devices</h2>
          <p className="text-sm text-muted-foreground">
            Review recent device activity and revoke access you no longer recognize.
          </p>
        </div>
        <Button
          type="button"
          variant="destructive"
          disabled={sessions.length === 0}
          onClick={() => {
            setAction({ kind: 'global' });
            setExceptCurrent(true);
            setMessage('');
          }}
        >
          Sign out sessions
        </Button>
      </div>
      {message && (
        <p role="status" className="rounded-md bg-emerald-500/10 p-3 text-sm">
          {message}
        </p>
      )}
      {sessions.length === 0 ? (
        <Card>
          <EmptyState
            icon={Laptop}
            title="No sessions reported"
            description="The identity service did not report an active or recently revoked session."
          />
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {sessions.map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              onRevoke={() => {
                setAction({ kind: 'revoke', session });
                setMessage('');
              }}
            />
          ))}
        </div>
      )}
      <Dialog
        open={Boolean(action)}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) {
            setAction(null);
            setReason('');
            mutation.reset();
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {action?.kind === 'global' ? 'Sign out sessions' : 'Revoke this session'}
            </DialogTitle>
            <DialogDescription>
              {action?.kind === 'global'
                ? 'Choose whether to keep this browser signed in. Every selected session will need to authenticate again.'
                : action?.session.current
                  ? 'This is your current session. Revoking it will immediately return you to sign in.'
                  : 'The selected device will lose access and must authenticate again.'}
            </DialogDescription>
          </DialogHeader>
          {mutation.isError && (
            <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {formatApiError(mutation.error, 'The session action failed.')}
            </p>
          )}
          {action?.kind === 'global' && (
            <label className="flex items-start gap-3 rounded-md border p-3 text-sm">
              <input
                type="checkbox"
                checked={exceptCurrent}
                onChange={(event) => setExceptCurrent(event.target.checked)}
                className="mt-0.5 h-4 w-4"
              />
              <span>
                <strong className="block">Keep this browser signed in</strong>
                <span className="text-muted-foreground">
                  Clear this to sign out every session, including this one.
                </span>
              </span>
            </label>
          )}
          <label className="block space-y-2">
            <span className="text-sm font-medium">Reason</span>
            <Input
              autoFocus
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              maxLength={1000}
            />
            <span className="block text-xs text-muted-foreground">
              Required for the identity security history (3–1000 characters).
            </span>
          </label>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => setAction(null)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={mutation.isPending || !identityReasonValid(reason)}
              onClick={() => mutation.mutate()}
            >
              {mutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              {action?.kind === 'global' ? 'Sign out selected sessions' : 'Revoke session'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function SessionCard({ onRevoke, session }: { onRevoke: () => void; session: IdentitySession }) {
  const mobile = /mobile|android|iphone/i.test(session.user_agent ?? '');
  const Icon = mobile ? Smartphone : Laptop;
  return (
    <Card className={session.current ? 'border-primary/50' : undefined}>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="flex gap-3">
            <Icon aria-hidden="true" className="mt-0.5 h-5 w-5" />
            <div>
              <CardTitle className="text-base">
                {session.device_name || deviceFromAgent(session.user_agent)}
              </CardTitle>
              <CardDescription className="mt-1">
                {session.ip_address || 'IP address withheld'}
              </CardDescription>
            </div>
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            {session.current && <Badge>Current</Badge>}
            <Badge variant={session.revoked_at ? 'secondary' : 'outline'}>
              {session.revoked_at ? 'Revoked' : 'Active'}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <dl className="grid grid-cols-2 gap-3">
          <div>
            <dt className="text-xs text-muted-foreground">Authenticated with</dt>
            <dd>{formatIdentityMethod(session.authentication_method)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Last active</dt>
            <dd>{formatDate(session.last_seen_at)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Expires</dt>
            <dd>{formatDate(session.expires_at)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">MFA verified</dt>
            <dd>{session.mfa_verified_at ? formatDate(session.mfa_verified_at) : 'No'}</dd>
          </div>
        </dl>
        {session.revoke_reason && (
          <p className="rounded-md bg-muted p-2 text-xs">
            Revocation reason: {session.revoke_reason}
          </p>
        )}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={Boolean(session.revoked_at)}
          onClick={onRevoke}
        >
          <LogOut aria-hidden="true" className="mr-2 h-4 w-4" />
          {session.current ? 'Revoke and sign out' : 'Revoke session'}
        </Button>
      </CardContent>
    </Card>
  );
}

function deviceFromAgent(agent?: string): string {
  if (!agent) return 'Unidentified device';
  if (/iphone|ipad/i.test(agent)) return 'Apple mobile device';
  if (/android/i.test(agent)) return 'Android device';
  if (/windows/i.test(agent)) return 'Windows device';
  if (/macintosh|mac os/i.test(agent)) return 'Mac device';
  if (/linux/i.test(agent)) return 'Linux device';
  return 'Browser session';
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}

function PanelSkeleton({ label }: { label: string }) {
  return (
    <div role="status" aria-label={label} className="grid gap-4 lg:grid-cols-2">
      <Skeleton className="h-56" />
      <Skeleton className="h-56" />
    </div>
  );
}

function PanelError({ error, retry, title }: { error: unknown; retry: () => void; title: string }) {
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <RefreshCw aria-hidden="true" className="h-8 w-8" />
        <h2 className="font-semibold">{title}</h2>
        <p role="alert" className="text-sm text-muted-foreground">
          {formatApiError(error, title)}
        </p>
        <Button type="button" variant="outline" onClick={retry}>
          Try again
        </Button>
      </CardContent>
    </Card>
  );
}
