'use client';

import * as React from 'react';

import api, { type ApiError } from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  History,
  KeyRound,
  Loader2,
  LockKeyhole,
  MailPlus,
  RefreshCw,
  ShieldCheck,
  UserCog,
} from 'lucide-react';
import type {
  IdentityConfigurableMethod,
  IdentityPolicy,
  IdentitySecurityEvent,
} from '@/types/identity';
import { identityKeys, identityReasonValid } from '@/lib/identity';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type { DirectoryUser } from '@/types/directory';
import { DirectoryUserPicker } from '@/components/access-admin/directory-user-picker';
import { EmptyState } from '@/components/data/empty-state';
import { formatApiError } from '@/lib/enterprise-settings';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { StepUpDialog } from '@/components/identity/step-up-dialog';
import { Textarea } from '@/components/ui/textarea';
import { useAuthStore } from '@/store/auth-store';
import { useIdentityPermissions } from '@/hooks/use-identity-permissions';

export default function IdentityAdministrationPage() {
  const permission = useIdentityPermissions();
  if (permission.isLoading) return <IdentityAdminSkeleton />;
  if (permission.isError)
    return (
      <AccessState
        icon={RefreshCw}
        title="Identity administration access could not be verified"
        description="The permission service is unavailable. Identity policy, recovery, and history were not loaded."
      >
        <Button type="button" variant="outline" onClick={() => void permission.retry()}>
          Try again
        </Button>
      </AccessState>
    );
  if (!permission.canReadAdministration && !permission.canInvite && !permission.canAdminReset)
    return (
      <AccessState
        icon={LockKeyhole}
        title="Identity administration unavailable"
        description="Your role does not grant identity policy, invitation, recovery, or history access."
      />
    );
  const defaultTab = permission.canReadAdministration ? 'policy' : 'recovery';
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Identity administration</h1>
        <p className="mt-1 max-w-4xl text-muted-foreground">
          Enforce tenant authentication policy, deliver invitations, perform safeguarded MFA
          recovery, and review the security trail.
        </p>
      </div>
      <Card>
        <CardContent className="flex gap-3 py-4 text-sm">
          <ShieldCheck aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <p>
            Administrator actions are tenant-scoped, permission checked, reasoned, and recorded. MFA
            resets additionally require a single-use step-up proof.
          </p>
        </CardContent>
      </Card>
      <Tabs defaultValue={defaultTab} className="space-y-5">
        <TabsList className="h-auto max-w-full flex-wrap justify-start">
          {permission.canReadAdministration && (
            <TabsTrigger value="policy" className="gap-2">
              <KeyRound aria-hidden="true" className="h-4 w-4" />
              Policy
            </TabsTrigger>
          )}
          {(permission.canInvite || permission.canAdminReset) && (
            <TabsTrigger value="recovery" className="gap-2">
              <UserCog aria-hidden="true" className="h-4 w-4" />
              Invitations & recovery
            </TabsTrigger>
          )}
          {permission.canReadAdministration && (
            <TabsTrigger value="history" className="gap-2">
              <History aria-hidden="true" className="h-4 w-4" />
              History
            </TabsTrigger>
          )}
        </TabsList>
        {permission.canReadAdministration && (
          <TabsContent value="policy">
            <PolicyPanel canConfigure={permission.canConfigurePolicy} />
          </TabsContent>
        )}
        {(permission.canInvite || permission.canAdminReset) && (
          <TabsContent value="recovery">
            <AdminRecoveryPanel
              canInvite={permission.canInvite}
              canReset={permission.canAdminReset}
            />
          </TabsContent>
        )}
        {permission.canReadAdministration && (
          <TabsContent value="history">
            <IdentityHistoryPanel />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}

function PolicyPanel({ canConfigure }: { canConfigure: boolean }) {
  const query = useQuery({
    queryKey: identityKeys.policy,
    queryFn: () => api.identity.getPolicy(),
  });
  if (query.isLoading)
    return <Skeleton className="h-[32rem]" aria-label="Loading identity policy" />;
  if (query.isError || !query.data)
    return (
      <RetryCard
        title="Identity policy could not be loaded"
        error={query.error}
        retry={() => void query.refetch()}
      />
    );
  return (
    <PolicyForm
      key={query.data.version}
      policy={query.data}
      canConfigure={canConfigure}
      reload={() => void query.refetch()}
    />
  );
}

function PolicyForm({
  canConfigure,
  policy,
  reload,
}: {
  canConfigure: boolean;
  policy: IdentityPolicy;
  reload: () => void;
}) {
  const queryClient = useQueryClient();
  const [requireMFA, setRequireMFA] = React.useState(policy.require_mfa);
  const [requireAdmins, setRequireAdmins] = React.useState(policy.require_mfa_for_admins);
  const [totp, setTOTP] = React.useState(policy.allowed_methods.includes('totp'));
  const [passkey, setPasskey] = React.useState(policy.allowed_methods.includes('passkey'));
  const [grace, setGrace] = React.useState(policy.enrollment_grace_hours);
  const [challenge, setChallenge] = React.useState(policy.authentication_challenge_minutes);
  const [stepUpTTL, setStepUpTTL] = React.useState(policy.step_up_ttl_minutes);
  const [reason, setReason] = React.useState('');
  const mutation = useMutation({
    mutationFn: () =>
      api.identity.updatePolicy({
        expected_version: policy.version,
        require_mfa: requireMFA,
        require_mfa_for_admins: requireAdmins,
        allowed_methods: [totp && 'totp', passkey && 'passkey'].filter(
          (item): item is IdentityConfigurableMethod => Boolean(item),
        ),
        enrollment_grace_hours: grace,
        authentication_challenge_minutes: challenge,
        step_up_ttl_minutes: stepUpTTL,
        reason,
      }),
    onSuccess: async () => {
      setReason('');
      await queryClient.invalidateQueries({ queryKey: identityKeys.policy });
    },
  });
  const valid =
    (totp || passkey) &&
    grace >= 0 &&
    grace <= 720 &&
    challenge >= 1 &&
    challenge <= 15 &&
    stepUpTTL >= 1 &&
    stepUpTTL <= 30 &&
    identityReasonValid(reason);
  const conflict = (mutation.error as ApiError | null)?.status === 409;
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Tenant authentication policy</CardTitle>
          <CardDescription>
            Version {policy.version}. Changes affect future sign-ins and sensitive operations; they
            do not reveal factor secrets.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {!canConfigure && (
            <p role="status" className="rounded-md bg-muted p-3 text-sm">
              You have read-only settings access. A settings administrator must save policy changes.
            </p>
          )}
          {mutation.isError && (
            <div role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <p>{formatApiError(mutation.error, 'Policy update failed.')}</p>
              {conflict && (
                <Button type="button" size="sm" variant="outline" className="mt-2" onClick={reload}>
                  Reload current version
                </Button>
              )}
            </div>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <Toggle
              checked={requireMFA}
              disabled={!canConfigure}
              onChange={setRequireMFA}
              title="Require MFA for everyone"
              description="All tenant users must enroll and use an allowed method."
            />
            <Toggle
              checked={requireAdmins}
              disabled={!canConfigure}
              onChange={setRequireAdmins}
              title="Require MFA for administrators"
              description="Protect administrators even when tenant-wide enforcement is off."
            />
          </div>
          <fieldset disabled={!canConfigure} className="space-y-2">
            <legend className="text-sm font-medium">Allowed methods</legend>
            <div className="grid gap-3 sm:grid-cols-2">
              <CheckOption
                checked={totp}
                onChange={setTOTP}
                label="Authenticator app"
                detail="RFC 6238 six-digit codes plus recovery codes."
              />
              <CheckOption
                checked={passkey}
                onChange={setPasskey}
                label="Passkey"
                detail="Phishing-resistant WebAuthn credentials."
              />
            </div>
            {!totp && !passkey && (
              <p role="alert" className="text-xs text-destructive">
                At least one method is required.
              </p>
            )}
          </fieldset>
          <div className="grid gap-4 sm:grid-cols-3">
            <NumberField
              label="Enrollment grace hours"
              value={grace}
              min={0}
              max={720}
              disabled={!canConfigure}
              onChange={setGrace}
            />
            <NumberField
              label="Challenge lifetime (minutes)"
              value={challenge}
              min={1}
              max={15}
              disabled={!canConfigure}
              onChange={setChallenge}
            />
            <NumberField
              label="Step-up lifetime (minutes)"
              value={stepUpTTL}
              min={1}
              max={30}
              disabled={!canConfigure}
              onChange={setStepUpTTL}
            />
          </div>
          {canConfigure && (
            <label className="block space-y-2">
              <span className="text-sm font-medium">Change reason</span>
              <Textarea
                maxLength={1000}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                placeholder="Why this policy is changing"
              />
              <span className="block text-xs text-muted-foreground">
                Required, 3–1000 characters.
              </span>
            </label>
          )}
          <Button
            type="button"
            disabled={!canConfigure || !valid || mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending && (
              <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
            )}
            Save policy
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="grid gap-3 py-4 text-sm sm:grid-cols-3">
          <div>
            <span className="text-muted-foreground">Last updated</span>
            <p>{formatDate(policy.updated_at)}</p>
          </div>
          <div>
            <span className="text-muted-foreground">Updated by</span>
            <p className="break-all font-mono text-xs">{policy.updated_by || 'Default policy'}</p>
          </div>
          <div>
            <span className="text-muted-foreground">Recorded reason</span>
            <p>{policy.update_reason || 'Platform default'}</p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function AdminRecoveryPanel({ canInvite, canReset }: { canInvite: boolean; canReset: boolean }) {
  const currentUser = useAuthStore((state) => state.user);
  const [inviteUser, setInviteUser] = React.useState<DirectoryUser | null>(null);
  const [resetUser, setResetUser] = React.useState<DirectoryUser | null>(null);
  const [inviteReason, setInviteReason] = React.useState('Issue account invitation');
  const [expires, setExpires] = React.useState(72);
  const [resetReason, setResetReason] = React.useState('Administrator-assisted MFA recovery');
  const [exemption, setExemption] = React.useState(2);
  const [stepUpOpen, setStepUpOpen] = React.useState(false);
  const [inviteMessage, setInviteMessage] = React.useState('');
  const [resetMessage, setResetMessage] = React.useState('');
  const invite = useMutation({
    mutationFn: () =>
      api.identity.issueInvitation(inviteUser!.id, {
        expires_in_hours: expires,
        reason: inviteReason,
      }),
    onSuccess: (result) => {
      setInviteMessage(
        `Invitation delivery queued for ${result.email}; expires ${formatDate(result.expires_at)}.`,
      );
      setInviteUser(null);
    },
  });
  async function resetMFA(token: string) {
    if (!resetUser) return;
    await api.identity.adminResetMFA(
      resetUser.id,
      { reason: resetReason, exemption_hours: exemption },
      token,
    );
    setResetMessage(`MFA reset completed for ${resetUser.email}.`);
    setResetUser(null);
  }
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      {canInvite && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <MailPlus aria-hidden="true" className="h-5 w-5" />
              Issue invitation
            </CardTitle>
            <CardDescription>
              Only pending-verification users are eligible. A new invitation revokes any prior
              unaccepted invitation.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {inviteMessage && (
              <p role="status" className="rounded-md bg-emerald-500/10 p-3 text-sm">
                {inviteMessage}
              </p>
            )}
            {invite.isError && (
              <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                {formatApiError(invite.error, 'Invitation could not be issued.')}
              </p>
            )}
            <DirectoryUserPicker
              status="pending_verification"
              searchLabel="Search pending users"
              selectedLabel="Invitation recipient"
              description="Only pending-verification users are shown."
              value={inviteUser}
              onChange={(user) => {
                setInviteUser(user);
                setInviteMessage('');
                invite.reset();
              }}
            />
            <NumberField
              label="Expires in hours"
              value={expires}
              min={1}
              max={720}
              disabled={invite.isPending}
              onChange={setExpires}
            />
            <label className="block space-y-2">
              <span className="text-sm font-medium">Reason</span>
              <Textarea
                maxLength={1000}
                value={inviteReason}
                onChange={(event) => setInviteReason(event.target.value)}
              />
            </label>
            <Button
              type="button"
              disabled={
                !inviteUser ||
                invite.isPending ||
                expires < 1 ||
                expires > 720 ||
                !identityReasonValid(inviteReason)
              }
              onClick={() => invite.mutate()}
            >
              {invite.isPending && (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              )}
              Queue invitation
            </Button>
          </CardContent>
        </Card>
      )}
      {canReset && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <UserCog aria-hidden="true" className="h-5 w-5" />
              Administrator MFA reset
            </CardTitle>
            <CardDescription>
              Use only after identity verification. This disables every MFA factor and passkey,
              revokes every session, and grants a bounded exemption for re-enrollment.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {resetMessage && (
              <p role="status" className="rounded-md bg-emerald-500/10 p-3 text-sm">
                {resetMessage}
              </p>
            )}
            <DirectoryUserPicker
              searchLabel="Search user for MFA recovery"
              selectedLabel="Recovery subject"
              value={resetUser}
              onChange={(user) => {
                setResetUser(user);
                setResetMessage('');
              }}
            />
            {resetUser?.id === currentUser?.id && (
              <p role="alert" className="text-sm text-destructive">
                Administrators cannot reset their own MFA. Use self-service security settings.
              </p>
            )}
            <NumberField
              label="Temporary exemption hours"
              value={exemption}
              min={1}
              max={24}
              disabled={false}
              onChange={setExemption}
            />
            <label className="block space-y-2">
              <span className="text-sm font-medium">Verified support reason</span>
              <Textarea
                maxLength={1000}
                value={resetReason}
                onChange={(event) => setResetReason(event.target.value)}
              />
            </label>
            <Button
              type="button"
              variant="destructive"
              disabled={
                !resetUser ||
                resetUser.id === currentUser?.id ||
                exemption < 1 ||
                exemption > 24 ||
                !identityReasonValid(resetReason)
              }
              onClick={() => setStepUpOpen(true)}
            >
              Verify and reset MFA
            </Button>
            {resetUser && (
              <StepUpDialog
                open={stepUpOpen}
                onOpenChange={setStepUpOpen}
                purpose={`admin_mfa_reset:${resetUser.id}`}
                title="Verify administrator MFA reset"
            description={`Confirm your identity before resetting MFA for ${resetUser.email}. Their factors, passkeys, and sessions will be revoked.`}
                onGranted={resetMFA}
              />
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function IdentityHistoryPanel() {
  const [user, setUser] = React.useState<DirectoryUser | null>(null);
  const [page, setPage] = React.useState(1);
  const query = useQuery({
    queryKey: identityKeys.history(user?.id ?? '', page),
    queryFn: () => api.identity.listHistory({ user_id: user?.id, page, page_size: 20 }),
    placeholderData: (previous) => previous,
  });
  const events = query.data?.data ?? [];
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Identity security history</CardTitle>
          <CardDescription>
            Tenant-scoped invitations, verification, password recovery, session, MFA, passkey,
            step-up, and administrator recovery events.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DirectoryUserPicker
            searchLabel="Filter history by active user"
            selectedLabel="History subject"
            value={user}
            onChange={(next) => {
              setUser(next);
              setPage(1);
            }}
          />
        </CardContent>
      </Card>
      {query.isLoading ? (
        <Skeleton className="h-80" aria-label="Loading identity history" />
      ) : query.isError ? (
        <RetryCard
          title="Identity history could not be loaded"
          error={query.error}
          retry={() => void query.refetch()}
        />
      ) : events.length === 0 ? (
        <Card>
          <EmptyState
            icon={History}
            title="No identity events found"
            description={
              user
                ? 'No events matched the selected user.'
                : 'No identity security events have been recorded for this organization.'
            }
          />
        </Card>
      ) : (
        <HistoryTable events={events} />
      )}
      {query.data && query.data.pagination.total_pages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">
            Page {query.data.pagination.page} of {query.data.pagination.total_pages}
          </p>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={page <= 1 || query.isFetching}
              onClick={() => setPage((value) => Math.max(1, value - 1))}
            >
              Previous
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={page >= query.data.pagination.total_pages || query.isFetching}
              onClick={() => setPage((value) => value + 1)}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function HistoryTable({ events }: { events: IdentitySecurityEvent[] }) {
  return (
    <Card>
      <CardContent className="p-0">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-left text-sm">
            <caption className="sr-only">Tenant identity security events</caption>
            <thead className="border-b bg-muted/40">
              <tr>
                <th scope="col" className="px-4 py-3">
                  Time
                </th>
                <th scope="col" className="px-4 py-3">
                  Event
                </th>
                <th scope="col" className="px-4 py-3">
                  Subject / actor
                </th>
                <th scope="col" className="px-4 py-3">
                  Reason
                </th>
                <th scope="col" className="px-4 py-3">
                  Request
                </th>
              </tr>
            </thead>
            <tbody>
              {events.map((event) => (
                <tr key={event.id} className="border-b last:border-0">
                  <td className="whitespace-nowrap px-4 py-3">{formatDate(event.created_at)}</td>
                  <td className="px-4 py-3">
                    <Badge variant="outline">{event.event_type.replace(/[._-]+/g, ' ')}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <span className="block break-all font-mono text-xs">
                      {event.user_id || 'Organization'}
                    </span>
                    <span className="block break-all text-xs text-muted-foreground">
                      Actor: {event.actor_user_id || 'System'}
                    </span>
                  </td>
                  <td className="max-w-sm px-4 py-3">{event.reason}</td>
                  <td className="px-4 py-3">
                    <span className="block font-mono text-xs">{event.request_id || '—'}</span>
                    <span className="block text-xs text-muted-foreground">
                      {event.ip_address || 'IP withheld'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  );
}

function Toggle({
  checked,
  description,
  disabled,
  onChange,
  title,
}: {
  checked: boolean;
  description: string;
  disabled: boolean;
  onChange: (value: boolean) => void;
  title: string;
}) {
  return (
    <label className="flex items-start gap-3 rounded-md border p-3">
      <input
        type="checkbox"
        className="mt-1 h-4 w-4"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span>
        <strong className="block text-sm">{title}</strong>
        <span className="text-xs text-muted-foreground">{description}</span>
      </span>
    </label>
  );
}
function CheckOption({
  checked,
  detail,
  label,
  onChange,
}: {
  checked: boolean;
  detail: string;
  label: string;
  onChange: (value: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-3 rounded-md border p-3">
      <input
        type="checkbox"
        className="mt-1 h-4 w-4"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span>
        <strong className="block text-sm">{label}</strong>
        <span className="text-xs text-muted-foreground">{detail}</span>
      </span>
    </label>
  );
}
function NumberField({
  disabled,
  label,
  max,
  min,
  onChange,
  value,
}: {
  disabled: boolean;
  label: string;
  max: number;
  min: number;
  onChange: (value: number) => void;
  value: number;
}) {
  return (
    <label className="block space-y-2">
      <span className="text-sm font-medium">{label}</span>
      <Input
        type="number"
        min={min}
        max={max}
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
      />
      <span className="block text-xs text-muted-foreground">
        Allowed range: {min}–{max}.
      </span>
    </label>
  );
}
function RetryCard({ error, retry, title }: { error: unknown; retry: () => void; title: string }) {
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
function IdentityAdminSkeleton() {
  return (
    <div role="status" aria-label="Loading identity administration" className="space-y-4">
      <Skeleton className="h-24" />
      <Skeleton className="h-16" />
      <Skeleton className="h-[32rem]" />
    </div>
  );
}
function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}
