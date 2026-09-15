'use client';

import * as React from 'react';
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import api, { type ApiError } from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog';
import {
  editableOrganizationProfile, isCanonicalOrganizationId, ORGANIZATION_CONTACT_PURPOSES, ORGANIZATION_PROFILE_LANGUAGES,
  organizationProfileChanges, organizationProfileDraft, organizationProfileError, type OrganizationProfileErrors,
  PROFILE_EDITABLE_KEYS, validateOrganizationProfileUpdate,
} from '@/lib/organization-profile';
import type {
  OrganizationProfile, OrganizationProfileEditable, OrganizationProfileProjection, OrganizationProfileUpdateInput,
} from '@/types/organization-profile';
import { organizationProfileKeys, useOrganizationProfile, useOrganizationProfileAccess } from '@/lib/organization-profile-hooks';
import { ResourceState, StaleDataNotice } from '@/components/data/resource-state';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { SESSION_EXPIRED_EVENT } from '@/lib/auth-constants';
import { useAuthStore } from '@/store/auth-store';
import { useOnlineStatus } from '@/hooks/use-online-status';
import { useQueryClient } from '@tanstack/react-query';

const TEXT_FIELDS = [
  ['name', 'Organisation name', 'Required; 255 UTF-8 bytes maximum.', 255],
  ['legal_name', 'Legal name', 'Optional; 500 UTF-8 bytes maximum.', 500],
  ['industry', 'Industry', 'Optional; 100 UTF-8 bytes maximum.', 100],
  ['country_code', 'Country code', 'Two uppercase letters, or blank. Syntax only; not an ISO registry check.', 2],
  ['timezone', 'Timezone', 'IANA identifier, such as Africa/Lagos, Europe/Berlin or UTC. Server validation is authoritative.', 50],
  ['employee_count_range', 'Employee count range', 'Informational range, such as 50-249; 20 UTF-8 bytes maximum.', 20],
] as const;
const LANGUAGES = { en: 'English', de: 'German', fr: 'French' } as const;
const LABELS: Record<typeof PROFILE_EDITABLE_KEYS[number], string> = {
  name: 'Organisation name', legal_name: 'Legal name', industry: 'Industry', country_code: 'Country code', timezone: 'Timezone',
  default_language: 'Default language', supported_languages: 'Supported languages', employee_count_range: 'Employee count range',
  fiscal_year_start_month: 'Fiscal-year start month', fiscal_year_start_day: 'Fiscal-year start day', contacts: 'Informational contacts',
};

function restoreSettingsFocus() {
  const heading = document.querySelector<HTMLElement>('#main-content h1');
  if (heading) { heading.tabIndex = -1; heading.focus({ preventScroll: true }); }
}

export function OrganizationProfilePanel() {
  const { user, isAuthenticated, isLoggingOut } = useAuthStore();
  if (!isAuthenticated || isLoggingOut || !user || user.status !== 'active' ||
    !isCanonicalOrganizationId(user.id) || !isCanonicalOrganizationId(user.organization_id)) {
    return <ResourceState kind="forbidden" title="Organisation profile access unavailable" description="An active, authenticated tenant session is required. Local editing is not available." />;
  }
  return <OrganizationProfileAccessGate key={`${user.organization_id}:${user.id}`} tenantId={user.organization_id} actorId={user.id} />;
}

function OrganizationProfileAccessGate({ tenantId, actorId }: { tenantId: string; actorId: string }) {
  const online = useOnlineStatus();
  const access = useOrganizationProfileAccess(tenantId, actorId);
  const [terminated, setTerminated] = React.useState(false);
  React.useEffect(() => {
    const end = () => setTerminated(true);
    window.addEventListener(SESSION_EXPIRED_EVENT, end);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, end);
  }, []);
  if (terminated) return <ResourceState kind="forbidden" title="Organisation profile session ended" description="Local drafts were cleared. Verify your session and permissions before reopening settings." />;
  if (!online) return <ResourceState kind="offline" title="Organisation editing paused offline" description="Local drafts and pending work were cleared. Reconnect and review the current profile before editing again." />;
  if (access.isPending) return <ResourceState kind="loading" loadingLayout="inline" title="Verifying organisation settings access" />;
  if (access.isError) return <ResourceState kind="error" title="Organisation settings access could not be verified" description="No profile fields or drafts are shown without effective permission verification." onRetry={() => void access.refetch()} retrying={access.isFetching} />;
  if (!access.canRead) return <ResourceState kind="forbidden" title="Organisation profile access unavailable" description="Effective settings:read access is required." />;
  // A configure change remounts the workflow, synchronously discarding its draft.
  return <OrganizationProfileView key={String(access.canConfigure)} tenantId={tenantId} actorId={actorId} canConfigure={access.canConfigure} onTerminate={() => setTerminated(true)} />;
}

function OrganizationProfileView({ tenantId, actorId, canConfigure, onTerminate }: {
  tenantId: string; actorId: string; canConfigure: boolean; onTerminate: () => void;
}) {
  const query = useOrganizationProfile(tenantId, actorId, canConfigure);
  const client = useQueryClient();
  const [editing, setEditing] = React.useState(false);
  const [base, setBase] = React.useState<OrganizationProfile | null>(null);
  const [draft, setDraft] = React.useState<OrganizationProfileEditable | null>(null);
  const [reason, setReason] = React.useState('');
  const [errors, setErrors] = React.useState<OrganizationProfileErrors>({});
  const [saveError, setSaveError] = React.useState('');
  const [feedback, setFeedback] = React.useState('');
  const [mustReload, setMustReload] = React.useState(false);
  const [pending, setPending] = React.useState(false);
  const [review, setReview] = React.useState<OrganizationProfileUpdateInput | null>(null);
  const [discardOpen, setDiscardOpen] = React.useState(false);
  const sequence = React.useRef(0);
  const abort = React.useRef<AbortController | null>(null);
  const wasEditing = React.useRef(false);
  const editButton = React.useRef<HTMLButtonElement>(null);
  const firstField = React.useRef<HTMLInputElement>(null);
  const errorRef = React.useRef<HTMLDivElement>(null);
  const externalErrorRef = React.useRef<HTMLDivElement>(null);

  React.useLayoutEffect(() => () => {
    sequence.current += 1; abort.current?.abort(); abort.current = null;
    if (wasEditing.current) requestAnimationFrame(restoreSettingsFocus);
  }, []);
  React.useEffect(() => {
    if (Object.keys(errors).length || (saveError && editing)) errorRef.current?.focus();
    else if (saveError) externalErrorRef.current?.focus();
  }, [editing, errors, saveError]);
  React.useEffect(() => {
    if (!editing) return;
    const unload = (event: BeforeUnloadEvent) => { event.preventDefault(); };
    window.addEventListener('beforeunload', unload);
    return () => window.removeEventListener('beforeunload', unload);
  }, [editing]);

  const clearDraft = () => {
    sequence.current += 1; abort.current?.abort(); abort.current = null;
    setPending(false); setEditing(false); wasEditing.current = false;
    setBase(null); setDraft(null); setReason(''); setReview(null); setErrors({}); setDiscardOpen(false);
  };
  const current = query.data;
  const profile = current?.data;
  const full = current ? editableOrganizationProfile(current, tenantId) : null;
  const canEdit = Boolean(canConfigure && full && !mustReload && !query.isError && !query.isFetching);
  const dirty = Boolean(base && draft && Object.keys(organizationProfileChanges(base, draft, reason)).length > 2);
  const requestClose = (open: boolean) => {
    if (open) return;
    if (dirty || reason || pending) setDiscardOpen(true);
    else clearDraft();
  };
  const startEditing = () => {
    if (!canEdit || !full) return;
    setBase(full); setDraft(organizationProfileDraft(full)); setReason(''); setErrors({}); setSaveError(''); setFeedback('');
    wasEditing.current = true; setEditing(true);
  };
  const changeText = (key: typeof TEXT_FIELDS[number][0], value: string) => {
    setDraft((previous) => previous ? { ...previous, [key]: value } : null);
  };
  const submitReview = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!base || !draft || pending || !canEdit) return;
    const input = organizationProfileChanges(base, draft, reason);
    const validation = validateOrganizationProfileUpdate(input, base);
    setErrors(validation); setSaveError('');
    if (Object.keys(validation).length === 0) {
      // Do not remove the focused error summary beneath a new modal focus scope.
      firstField.current?.focus();
      setReview(input);
    }
  };
  const save = async () => {
    if (!review || !canEdit || pending || abort.current) return;
    const input = review;
    setReview(null); setSaveError(''); setPending(true);
    const controller = new AbortController(); abort.current = controller;
    const generation = ++sequence.current;
    const isCurrent = () => generation === sequence.current && !controller.signal.aborted;
    try {
      const result = await api.settings.updateOrg(input, controller.signal);
      if (!isCurrent()) return;
      if (result.data.id !== tenantId) throw new Error('Profile scope unavailable');
      client.setQueryData(organizationProfileKeys.profile(tenantId, actorId, canConfigure), result);
      clearDraft(); setMustReload(false); setFeedback('Organisation profile saved. The change reason and changed-field names were recorded in the server audit.');
    } catch (error) {
      if (!isCurrent()) return;
      const status = (error as ApiError | null)?.status;
      if (status === 401 || status === 403) { clearDraft(); onTerminate(); return; }
      if (status === 409 || status === 503) { clearDraft(); setMustReload(true); }
      setSaveError(organizationProfileError(error));
    } finally {
      if (generation === sequence.current) { abort.current = null; setPending(false); }
    }
  };
  const reload = async () => {
    if (editing || pending) return;
    const generation = ++sequence.current;
    const result = await query.refetch();
    if (generation !== sequence.current) return;
    if (!result.isError) { setMustReload(false); setSaveError(''); setFeedback('Current authorised profile loaded. Review the values before starting a new edit.'); }
  };

  if (query.isPending) return <ResourceState kind="loading" loadingLayout="detail" title="Loading organisation profile" />;
  if (query.isError && !profile) return <ResourceState kind="error" title="Organisation profile could not be loaded" description={organizationProfileError(query.error)} onRetry={() => void query.refetch()} retrying={query.isFetching} />;
  if (!profile || Object.keys(profile).length === 0) return <ResourceState kind="empty" title="Profile fields are not available" description="This authorised projection did not provide readable profile fields. Missing values will not be converted into empty editable fields." />;
  if (profile.id !== undefined && profile.id !== tenantId) return <ResourceState kind="forbidden" title="Organisation profile scope unavailable" description="The returned profile does not match the current tenant. No fields or editing controls are shown." />;

  return (
    <div className="space-y-4">
      {query.isError && <StaleDataNotice lastUpdatedAt={query.dataUpdatedAt} isRefreshing={query.isFetching} onRefresh={() => void reload()} />}
      {saveError && !editing && <div ref={externalErrorRef} tabIndex={-1} role="alert" className="rounded-md border border-destructive p-4 text-sm">{saveError}</div>}
      {feedback && <p role="status" className="rounded-md border p-3 text-sm">{feedback}</p>}
      <Card>
        <CardHeader className="gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-1">
            <CardTitle>Organisation profile</CardTitle>
            <CardDescription>Tenant profile and informational contacts. This does not configure domains, residency, delivery, billing authority or administrator delegation.</CardDescription>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={() => void reload()} disabled={editing || query.isFetching}>Reload profile</Button>
            {canConfigure && <Button ref={editButton} type="button" variant="outline" onClick={startEditing} disabled={!canEdit}>Edit organisation profile</Button>}
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          {!canConfigure && <p className="text-sm">Read-only: effective settings:configure permission is required to edit.</p>}
          {!full && <p role="status" className="rounded-md border p-3 text-sm">This projection is restricted, incomplete or not eligible for editing. Returned values may be masked; missing fields are not assumed blank. No edit draft will be created.</p>}
          {mustReload && <p role="status" className="text-sm">Editing is paused. Reload and review a fresh authorised profile; the previous draft and reason were discarded.</p>}
          <ProfileDetails profile={profile} />
        </CardContent>
      </Card>
      <Dialog open={editing} onOpenChange={requestClose}>
        <DialogContent className="sm:max-w-3xl" aria-busy={pending}
          onOpenAutoFocus={(event) => { event.preventDefault(); firstField.current?.focus(); }}
          onCloseAutoFocus={(event) => { event.preventDefault(); editButton.current?.focus(); }}>
          <DialogHeader><DialogTitle>Edit organisation profile</DialogTitle><DialogDescription>Only changed fields are submitted. Untouched values remain unchanged. A reason and final confirmation are required; lifecycle, tier, slug and raw settings are not editable.</DialogDescription></DialogHeader>
          {draft && base && <form noValidate onSubmit={submitReview} className="space-y-5">
            <p className="text-sm">Tenant: <span className="break-all font-mono">{tenantId}</span> · Editing version {base.version}</p>
            {(Object.keys(errors).length > 0 || saveError) && <div ref={errorRef} tabIndex={-1} role="alert" className="rounded-md border border-destructive p-3 text-sm">
              {saveError || 'Review the labelled fields before saving.'}
              <ul className="mt-2 list-inside list-disc">{Object.entries(errors).map(([key, message]) => <li key={key}>{message}</li>)}</ul>
            </div>}
            <fieldset disabled={pending} className="grid min-w-0 gap-4 sm:grid-cols-2">
              <legend className="sr-only">Profile identity and locale</legend>
              {TEXT_FIELDS.map(([key, label, hint, maximum]) => <Field key={key} id={`profile-${key}`} label={label} hint={hint} error={errors[key]}>
                <Input ref={key === 'name' ? firstField : undefined} id={`profile-${key}`} value={draft[key]} maxLength={maximum} required={key === 'name'}
                  aria-invalid={Boolean(errors[key])} aria-describedby={`profile-${key}-hint${errors[key] ? ` profile-${key}-error` : ''}`}
                  onChange={(event) => changeText(key, event.target.value)} />
              </Field>)}
              <Field id="profile-default_language" label="Default language" error={errors.default_language} hint="English, German or French. Choosing a default includes it in supported languages.">
                <select id="profile-default_language" className="min-h-11 w-full rounded-md border bg-background px-3 text-sm" value={draft.default_language} aria-invalid={Boolean(errors.default_language)} aria-describedby="profile-default_language-hint"
                  onChange={(event) => {
                    const language = event.target.value;
                    setDraft((previous) => previous ? { ...previous, default_language: language,
                      supported_languages: ORGANIZATION_PROFILE_LANGUAGES.filter((candidate) => candidate === language || previous.supported_languages.includes(candidate)) } : null);
                  }}>
                  {!ORGANIZATION_PROFILE_LANGUAGES.some((language) => language === draft.default_language) && <option value={draft.default_language} disabled>Current legacy value: {draft.default_language || '(blank)'}</option>}
                  {ORGANIZATION_PROFILE_LANGUAGES.map((language) => <option key={language} value={language}>{LANGUAGES[language]}</option>)}
                </select>
              </Field>
              <fieldset aria-describedby="profile-supported_languages-hint" aria-invalid={Boolean(errors.supported_languages)} className="min-w-0 space-y-1">
                <legend className="text-sm font-medium">Supported languages</legend>
                {ORGANIZATION_PROFILE_LANGUAGES.map((language) => <label key={language} className="flex min-h-11 items-center gap-3 text-sm">
                  <input type="checkbox" checked={draft.supported_languages.includes(language)} onChange={(event) => {
                    const checked = event.target.checked;
                    setDraft((previous) => previous ? { ...previous, supported_languages: ORGANIZATION_PROFILE_LANGUAGES.filter((candidate) => candidate === language ? checked : previous.supported_languages.includes(candidate)) } : null);
                  }} />{LANGUAGES[language]}
                </label>)}
                <p id="profile-supported_languages-hint" className="text-xs text-muted-foreground">One to three; include the default language. Changing this list replaces legacy unsupported codes.</p>
                {errors.supported_languages && <p className="text-xs text-destructive">{errors.supported_languages}</p>}
              </fieldset>
              <Field id="profile-fiscal_year_start_month" label="Fiscal-year start month" hint="Recurring date; not a financial reporting configuration." error={errors.fiscal_year_start_month}>
                <Input id="profile-fiscal_year_start_month" type="number" min={1} max={12} step={1} value={draft.fiscal_year_start_month}
                  onChange={(event) => setDraft((previous) => previous ? { ...previous, fiscal_year_start_month: Number(event.target.value) } : null)} />
              </Field>
              <Field id="profile-fiscal_year_start_day" label="Fiscal-year start day" hint="Valid non-leap date; February 29 is unsupported." error={errors.fiscal_year_start_day}>
                <Input id="profile-fiscal_year_start_day" type="number" min={1} max={31} step={1} value={draft.fiscal_year_start_day} aria-invalid={Boolean(errors.fiscal_year_start_day)} aria-describedby="profile-fiscal_year_start_day-hint"
                  onChange={(event) => setDraft((previous) => previous ? { ...previous, fiscal_year_start_day: Number(event.target.value) } : null)} />
              </Field>
            </fieldset>
            <fieldset disabled={pending} aria-describedby="profile-contacts-hint" className="min-w-0 space-y-3">
              <legend className="font-medium">Informational contacts</legend>
              <p id="profile-contacts-hint" className="text-xs text-muted-foreground">At most one plain ASCII email per security, privacy, billing or support purpose. These addresses do not grant access, verify domains, configure delivery or change billing instructions.</p>
              {draft.contacts.map((contact, index) => <div key={index} className="grid gap-2 rounded-md border p-3 sm:grid-cols-[1fr_2fr_auto]">
                <Field id={`contact-purpose-${index}`} label={`Contact ${index + 1} purpose`}>
                  <select id={`contact-purpose-${index}`} className="min-h-11 w-full rounded-md border bg-background px-3 text-sm" value={contact.purpose}
                    onChange={(event) => setDraft((previous) => previous ? { ...previous, contacts: previous.contacts.map((item, position) => position === index ? { ...item, purpose: event.target.value } : item) } : null)}>
                    {ORGANIZATION_CONTACT_PURPOSES.map((purpose) => <option key={purpose} value={purpose} disabled={purpose !== contact.purpose && draft.contacts.some((item) => item.purpose === purpose)}>{purpose}</option>)}
                  </select>
                </Field>
                <Field id={`contact-email-${index}`} label={`Contact ${index + 1} email`}>
                  <Input id={`contact-email-${index}`} type="email" autoComplete="off" maxLength={254} value={contact.email} aria-invalid={Boolean(errors.contacts)}
                    onChange={(event) => setDraft((previous) => previous ? { ...previous, contacts: previous.contacts.map((item, position) => position === index ? { ...item, email: event.target.value } : item) } : null)} />
                </Field>
                <Button type="button" variant="outline" className="sm:self-end" onClick={() => setDraft((previous) => previous ? { ...previous, contacts: previous.contacts.filter((_item, position) => position !== index) } : null)} aria-label={`Remove contact ${index + 1}`}>Remove</Button>
              </div>)}
              {draft.contacts.length === 0 && <p className="text-sm">No informational contacts in this draft.</p>}
              {errors.contacts && <p className="text-xs text-destructive">{errors.contacts}</p>}
              <Button type="button" variant="outline" disabled={draft.contacts.length >= 4} onClick={() => {
                const purpose = ORGANIZATION_CONTACT_PURPOSES.find((candidate) => !draft.contacts.some((contact) => contact.purpose === candidate));
                if (purpose) setDraft((previous) => previous ? { ...previous, contacts: [...previous.contacts, { purpose, email: '' }] } : null);
              }}>Add informational contact</Button>
            </fieldset>
            <Field id="profile-reason" label="Change reason" hint="Required single-line explanation for the immutable server audit; at most 500 UTF-8 bytes. Never include credentials or secrets." error={errors.reason}>
              <Input id="profile-reason" value={reason} disabled={pending} maxLength={500} required aria-invalid={Boolean(errors.reason)} aria-describedby={`profile-reason-hint${errors.reason ? ' profile-reason-error' : ''}`} onChange={(event) => setReason(event.target.value)} />
            </Field>
            {pending && <p role="status">Saving the reviewed profile change. Cancelling is local; the server may already have committed it.</p>}
            <DialogFooter className="gap-2"><Button type="button" variant="outline" onClick={() => requestClose(false)}>Cancel edit</Button><Button type="submit" disabled={pending || !canEdit}>Review changes</Button></DialogFooter>
          </form>}
        </DialogContent>
      </Dialog>
      <AlertDialog open={Boolean(review)} onOpenChange={(open) => { if (!open) setReview(null); }}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Confirm organisation profile changes</AlertDialogTitle><AlertDialogDescription>Save only the reviewed changed fields for this tenant at version {base?.version}. The server rechecks permission and records the reason atomically. Conflicts require reload, never a blind retry.</AlertDialogDescription></AlertDialogHeader>
          <ul className="list-inside list-disc text-sm">{review && PROFILE_EDITABLE_KEYS.filter((key) => Object.hasOwn(review, key)).map((key) => <li key={key}>{LABELS[key]}</li>)}</ul>
          <p className="break-words text-sm">Reason: {review?.reason}</p>
          <AlertDialogFooter><AlertDialogCancel>Return to edit</AlertDialogCancel><AlertDialogAction onClick={() => void save()}>Confirm and save profile</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={discardOpen} onOpenChange={setDiscardOpen}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Discard local organisation draft?</AlertDialogTitle><AlertDialogDescription>Unsaved fields and the reason will be cleared. An in-flight save may already have committed on the server; reload before editing again. Nothing will be retried automatically.</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction onClick={() => {
            const wasPending = pending; clearDraft(); setSaveError(''); setFeedback('Local draft discarded. Reload the current profile before editing again.'); if (wasPending) setMustReload(true);
          }}>Discard draft</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function Field({ id, label, hint, error, children }: { id: string; label: string; hint?: string; error?: string; children: React.ReactNode }) {
  return <div className="min-w-0 space-y-2"><Label htmlFor={id}>{label}</Label>{children}{hint && <p id={`${id}-hint`} className="text-xs text-muted-foreground">{hint}</p>}{error && <p id={`${id}-error`} className="text-xs text-destructive">{error}</p>}</div>;
}

function ProfileDetails({ profile }: { profile: OrganizationProfileProjection }) {
  const values: Array<[string, string | number | undefined]> = [
    ['Name', profile.name], ['Legal name', profile.legal_name], ['Industry', profile.industry], ['Country code', profile.country_code], ['Timezone', profile.timezone],
    ['Default language', profile.default_language], ['Employee count range', profile.employee_count_range], ['Slug (read-only)', profile.slug],
    ['Status (read-only)', profile.status], ['Subscription tier (read-only)', profile.tier], ['Profile version', profile.version], ['Updated at', profile.updated_at],
  ];
  return <div className="space-y-5">
    <dl className="grid gap-4 sm:grid-cols-2">{values.filter(([, value]) => value !== undefined).map(([label, value]) => <div key={label} className="min-w-0"><dt className="text-xs font-medium text-muted-foreground">{label}</dt><dd className="mt-1 break-words text-sm">{value === '' ? 'Blank in returned projection' : value}</dd></div>)}</dl>
    {profile.supported_languages !== undefined && <div><h3 className="font-medium">Supported languages</h3><p className="text-sm">{profile.supported_languages.join(', ') || 'No values in returned projection'}</p></div>}
    {profile.fiscal_year_start_month !== undefined && profile.fiscal_year_start_day !== undefined && <div><h3 className="font-medium">Recurring fiscal-year start</h3><p className="text-sm">Month {profile.fiscal_year_start_month}, day {profile.fiscal_year_start_day}</p></div>}
    {profile.contacts !== undefined && <div><h3 className="font-medium">Informational contacts</h3>{profile.contacts.length ? <ul className="space-y-2 text-sm">{profile.contacts.map((contact, index) => <li key={index} className="break-all"><span className="font-medium">{contact.purpose}:</span> {contact.email}</li>)}</ul> : <p className="text-sm">No contacts in returned projection.</p>}</div>}
  </div>;
}
