'use client';

import * as React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,
} from '@/components/ui/dialog';
import { Download, FileArchive, Loader2, ShieldCheck } from 'lucide-react';
import { isCanonicalSupportUuid, supportBundleError, verifySupportBundle } from '@/lib/support-bundle';
import { SUPPORT_BUNDLE_MAX_BYTES, SUPPORT_BUNDLE_SCOPE } from '@/lib/support-bundle-contract';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import type { PermissionMap } from '@/types/access';
import { SESSION_EXPIRED_EVENT } from '@/lib/auth-constants';
import { useAuthStore } from '@/store/auth-store';
import { useOnlineStatus } from '@/hooks/use-online-status';
import type { VerifiedSupportBundle } from '@/types/support-bundle';

const CONTENTS = [
  ['manifest.json', 'Tenant UUID, generation/consent time, scope, exclusions, safe configuration fingerprint, member sizes and hashes.'],
  ['health.json', 'This tenant’s queue, inbox, notification and connector aggregates; migration state; allowlisted dependency status and latency.'],
  ['configuration.json', 'Environment enum, numeric release version, feature booleans, reviewed operational budgets and configuration-posture statuses.'],
  ['README.txt', 'Scope, integrity, consent-audit and customer-controlled sharing limitations.'],
] as const;

const EXCLUDED = [
  'Credentials and secrets', 'Dependency endpoints and server identities',
  'Origins, file paths, bucket names and KMS identifiers', 'Raw logs and errors',
  'Event payloads', 'Business records', 'User identifiers', 'Domain-record identifiers',
];

function focusRouteHeading() {
  const heading = document.querySelector<HTMLElement>('#main-content h1');
  if (heading) {
    heading.tabIndex = -1;
    heading.focus({ preventScroll: true });
  }
}

export function SupportBundlePanel({
  organizationId,
  permissions,
  permissionsVerified,
}: {
  organizationId: string;
  permissions?: PermissionMap;
  permissionsVerified: boolean;
}) {
  const { isAuthenticated, user } = useAuthStore();
  const online = useOnlineStatus();
  const identity = user ? `${user.id}:${user.organization_id}` : 'anonymous';
  const allowed = Boolean(isAuthenticated && user && permissionsVerified &&
    permissions?.settings?.includes('configure') &&
    isCanonicalSupportUuid(user.organization_id) && user.organization_id === organizationId);
  return (
    <SupportBundleCard>
      {allowed ? <SupportBundleSessionGate key={identity} organizationId={organizationId} online={online} /> :
        <p className="text-sm">Generation requires verified settings:configure access for this tenant.</p>}
    </SupportBundleCard>
  );
}

/** Keyed/mounted authorization barriers discard all workflow state synchronously. */
function SupportBundleSessionGate({ organizationId, online }: { organizationId: string; online: boolean }) {
  const [sessionTerminated, setSessionTerminated] = React.useState(false);
  React.useEffect(() => {
    const expired = () => setSessionTerminated(true);
    window.addEventListener(SESSION_EXPIRED_EVENT, expired);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, expired);
  }, []);
  if (sessionTerminated) return <p role="status">Your session ended. Prepared data and consent were cleared. Sign in again before generation.</p>;
  if (!online) return (
    <div className="space-y-2">
      <p role="status">You are offline. Prepared data and consent were cleared. Reconnect to review a fresh preview.</p>
      <Button type="button" variant="outline" disabled>Preview support bundle</Button>
    </div>
  );
  return <SupportBundleWorkflow organizationId={organizationId} />;
}

function SupportBundleWorkflow({ organizationId }: { organizationId: string }) {
  const [open, setOpen] = React.useState(false);
  const [consent, setConsent] = React.useState(false);
  const [error, setError] = React.useState('');
  const [notice, setNotice] = React.useState('');
  const [pending, setPending] = React.useState(false);
  const [downloadRequested, setDownloadRequested] = React.useState(false);
  const [artifact, setArtifact] = React.useState<VerifiedSupportBundle | null>(null);
  const wasOpen = React.useRef(false);
  const generation = React.useRef(0);
  const controller = React.useRef<AbortController | null>(null);
  const objectUrl = React.useRef<string | null>(null);
  const revokeTimer = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const previewRef = React.useRef<HTMLHeadingElement>(null);
  const readyRef = React.useRef<HTMLHeadingElement>(null);
  const errorRef = React.useRef<HTMLDivElement>(null);
  const visibleArtifact = artifact;
  const consentId = React.useId();

  const revoke = React.useCallback(() => {
    if (revokeTimer.current) clearTimeout(revokeTimer.current);
    revokeTimer.current = null;
    if (objectUrl.current) URL.revokeObjectURL(objectUrl.current);
    objectUrl.current = null;
  }, []);

  const clear = React.useCallback(() => {
    generation.current += 1;
    controller.current?.abort();
    controller.current = null;
    revoke();
    setArtifact(null);
    setConsent(false);
    setDownloadRequested(false);
    setPending(false);
    setError('');
    setNotice('');
  }, [revoke]);

  const changeOpen = (next: boolean) => {
    if (!next) clear();
    wasOpen.current = next;
    setOpen(next);
  };

  React.useLayoutEffect(() => {
    return () => {
      generation.current += 1;
      controller.current?.abort();
      controller.current = null;
      revoke();
      if (wasOpen.current) requestAnimationFrame(focusRouteHeading);
    };
  }, [revoke]);

  React.useEffect(() => {
    if (error) errorRef.current?.focus();
    else if (visibleArtifact) readyRef.current?.focus();
    else if (notice && open && !pending) previewRef.current?.focus();
  }, [error, visibleArtifact, notice, open, pending]);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!consent || pending || controller.current) return;
    const sequence = ++generation.current;
    const abort = new AbortController();
    controller.current = abort;
    // Consume consent at the start; every subsequent attempt needs a fresh action.
    setConsent(false);
    setArtifact(null);
    setError('');
    setNotice('');
    setPending(true);
    const current = () => sequence === generation.current && !abort.signal.aborted;
    try {
      const attachment = await api.diagnostics.generateSupportBundle(
        { consent: true, scope: SUPPORT_BUNDLE_SCOPE }, abort.signal,
      );
      if (!current()) return;
      const verified = await verifySupportBundle(attachment, organizationId, abort.signal);
      if (!current()) return;
      setArtifact(verified);
    } catch (caught) {
      if (current()) setError(supportBundleError(caught));
    } finally {
      if (sequence === generation.current) {
        controller.current = null;
        setPending(false);
      }
    }
  };

  const cancel = () => {
    clear();
    setNotice('Generation was cancelled locally. The server may already have recorded generation consent. No automatic retry will occur.');
    previewRef.current?.focus();
  };

  const download = () => {
    if (!visibleArtifact || downloadRequested) return;
    revoke();
    try {
      const url = URL.createObjectURL(visibleArtifact.blob);
      objectUrl.current = url;
      const link = document.createElement('a');
      link.href = url;
      link.download = visibleArtifact.filename;
      link.rel = 'noopener noreferrer';
      document.body.appendChild(link);
      try { link.click(); } finally { link.remove(); }
      revokeTimer.current = setTimeout(revoke, 1_000);
      setDownloadRequested(true);
    } catch {
      revoke();
      setError('The local download could not be prepared. No data was sent to a support provider.');
    }
  };

  return (
          <Dialog open={open} onOpenChange={changeOpen}>
            <DialogTrigger asChild>
              <Button type="button" variant="outline">
                Preview support bundle
              </Button>
            </DialogTrigger>
            <DialogContent
              aria-busy={pending}
              className="sm:max-w-3xl"
              onOpenAutoFocus={(event) => { event.preventDefault(); previewRef.current?.focus(); }}
            >
              <DialogHeader>
                <DialogTitle>Review and consent to a support bundle</DialogTitle>
                <DialogDescription>
                  One generation, one fresh consent. This preview describes the reviewed server allowlist, not a live copy of its data.
                </DialogDescription>
              </DialogHeader>
              {visibleArtifact ? (
                <div className="space-y-4">
                  <h3 ref={readyRef} tabIndex={-1} className="flex items-center gap-2 font-semibold">
                    <ShieldCheck aria-hidden="true" className="h-5 w-5 shrink-0" />
                    Bundle integrity checked
                  </h3>
                  <dl className="grid gap-3 text-sm sm:grid-cols-2">
                    <Metadata label="Tenant UUID" value={visibleArtifact.manifest.organization_id} />
                    <Metadata label="Bundle UUID" value={visibleArtifact.manifest.bundle_id} />
                    <Metadata label="Generated and consent recorded" value={visibleArtifact.manifest.generated_at} />
                    <Metadata label="Archive size" value={`${visibleArtifact.blob.size} bytes / ${SUPPORT_BUNDLE_MAX_BYTES} maximum`} />
                    <Metadata label="Archive SHA-256" value={visibleArtifact.sha256} />
                    <Metadata label="Safe configuration fingerprint" value={visibleArtifact.manifest.configuration_fingerprint} />
                  </dl>
                  <p className="text-sm">
                    Archive and member SHA-256 values match the same-origin response and manifest.
                    This detects changes; it is not a signature, proof of origin, or encryption.
                    The configuration fingerprint covers only configuration.json, never raw secrets.
                  </p>
                  <p className="text-sm text-muted-foreground">
                    The server recorded generation consent before releasing bytes. It did not record successful browser receipt or provider receipt.
                    Inspect all four members after download and share only through your approved secure support channel.
                    Local/provider access, expiry, deletion and retention are your responsibility; this feature does not manage them.
                  </p>
                  {downloadRequested && (
                    <p role="status">Download requested. Check your browser downloads; successful receipt has not been confirmed.</p>
                  )}
                  {error && <div ref={errorRef} tabIndex={-1} role="alert" className="rounded-md border border-destructive p-3 text-sm">{error}</div>}
                  <DialogFooter className="gap-2">
                    <Button type="button" variant="outline" onClick={() => {
                      clear();
                      setNotice('The prepared bundle was discarded from this page. Review and consent again for another generation.');
                    }}>Discard and prepare another</Button>
                    <Button type="button" disabled={downloadRequested} onClick={download}>
                      <Download aria-hidden="true" className="mr-2 h-4 w-4" />
                      Download local ZIP
                    </Button>
                  </DialogFooter>
                </div>
              ) : (
                <form className="space-y-4" onSubmit={(event) => void submit(event)}>
                  <h3 ref={previewRef} tabIndex={-1} className="font-semibold">Included operational metadata</h3>
                  <p className="break-all text-sm"><strong>Tenant UUID included:</strong> {organizationId}</p>
                  <p className="text-sm">Scope: health_and_posture · Redaction profile: health_posture_allowlist_v1.</p>
                  <div className="overflow-x-auto">
                    <table className="w-full text-left text-sm">
                      <caption className="sr-only">Exactly four fixed, uncompressed root-level ZIP members</caption>
                      <thead><tr><th scope="col" className="p-2">Member</th><th scope="col" className="p-2">Reviewed contents</th></tr></thead>
                      <tbody>{CONTENTS.map(([name, description]) => (
                        <tr key={name} className="border-t"><th scope="row" className="p-2 font-mono text-xs">{name}</th><td className="p-2">{description}</td></tr>
                      ))}</tbody>
                    </table>
                  </div>
                  <div>
                    <h3 className="font-semibold">Excluded data classes</h3>
                    <ul className="mt-2 grid list-inside list-disc gap-1 text-sm sm:grid-cols-2">
                      {EXCLUDED.map((entry) => <li key={entry}>{entry}</li>)}
                    </ul>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Actor and request UUIDs stay in server-side consent audit evidence, not in the ZIP.
                    Budget zero means unspecified or outside reviewed bounds. The browser verifies bounds,
                    fixed members and manifest integrity; it does not independently perform a redaction audit.
                    No raw configuration is included. This ZIP is not encrypted or signed.
                  </p>
                  {notice && <p role="status" className="text-sm">{notice}</p>}
                  {error && <div ref={errorRef} tabIndex={-1} role="alert" className="rounded-md border border-destructive p-3 text-sm">{error}</div>}
                  <label htmlFor={consentId} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3 text-sm">
                    <input
                      id={consentId}
                      type="checkbox"
                      className="mt-1 h-4 w-4 shrink-0"
                      checked={consent}
                      disabled={pending}
                      onChange={(event) => setConsent(event.target.checked)}
                    />
                    I consent to generate this organization’s health-and-posture bundle now. I understand that its tenant UUID is included,
                    consent is audited, and no support provider receives it automatically.
                  </label>
                  {pending && <p role="status" className="flex items-center gap-2"><Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />Generating and checking the bundle…</p>}
                  <DialogFooter className="gap-2">
                    <Button type="button" variant="outline" onClick={pending ? cancel : () => changeOpen(false)}>{pending ? 'Cancel generation' : 'Close preview'}</Button>
                    <Button type="submit" disabled={!consent || pending}>Generate bundle</Button>
                  </DialogFooter>
                </form>
              )}
            </DialogContent>
          </Dialog>
  );
}

function SupportBundleCard({ children }: { children: React.ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FileArchive aria-hidden="true" className="h-5 w-5 shrink-0" />
          Consented support bundle
        </CardTitle>
        <CardDescription>Prepare a local, at-most-64-KiB health-and-posture ZIP. Nothing is sent to a support provider.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Review the included tenant UUID and excluded data classes before granting fresh consent.
          Generation is audited; a downloaded copy cannot be recalled by this application.
        </p>
        {children}
      </CardContent>
    </Card>
  );
}

function Metadata({ label, value }: { label: string; value: string }) {
  return <div><dt className="font-medium">{label}</dt><dd className="mt-1 break-all font-mono text-xs">{value}</dd></div>;
}
