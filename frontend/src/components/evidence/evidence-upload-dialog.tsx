'use client';

import * as React from 'react';

import { AlertTriangle, FileCheck2, Loader2, ShieldCheck, UploadCloud, X } from 'lucide-react';
import type { ControlEvidence, EvidenceType } from '@/types/control-evidence';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  EVIDENCE_ACCEPT,
  EVIDENCE_DESCRIPTION_MAX_LENGTH,
  EVIDENCE_FILE_TYPE_LABEL,
  EVIDENCE_MAX_UPLOAD_MIB,
  EVIDENCE_METADATA_MAX_BYTES,
  EVIDENCE_TITLE_MAX_LENGTH,
  EVIDENCE_TYPES,
  EVIDENCE_VERSION_REASON_MAX_LENGTH,
  evidenceFileValidationError,
  formatEvidenceError,
  parseEvidenceMetadata,
  validityError,
  versionReasonError,
} from '@/lib/control-evidence';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';

interface EvidenceUploadDialogProps {
  controlId: string;
  onOpenChange: (open: boolean) => void;
  onUploaded: (evidence: ControlEvidence) => void;
  open: boolean;
  supersedes?: ControlEvidence | null;
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError';
}

export function EvidenceUploadDialog({
  controlId,
  onOpenChange,
  onUploaded,
  open,
  supersedes,
}: EvidenceUploadDialogProps) {
  const [description, setDescription] = React.useState(supersedes?.description ?? '');
  const [evidenceType, setEvidenceType] = React.useState<EvidenceType>(
    supersedes?.evidence_type ?? 'document',
  );
  const [error, setError] = React.useState('');
  const [file, setFile] = React.useState<File | null>(null);
  const [metadata, setMetadata] = React.useState('');
  const [pending, setPending] = React.useState(false);
  const [title, setTitle] = React.useState(supersedes?.title ?? '');
  const [validFrom, setValidFrom] = React.useState(supersedes?.valid_from?.slice(0, 10) ?? '');
  const [validUntil, setValidUntil] = React.useState(supersedes?.valid_until?.slice(0, 10) ?? '');
  const [versionReason, setVersionReason] = React.useState('');
  const abortController = React.useRef<AbortController | null>(null);
  const errorRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (error) errorRef.current?.focus();
  }, [error]);

  const reset = React.useCallback(() => {
    setDescription('');
    setEvidenceType('document');
    setError('');
    setFile(null);
    setMetadata('');
    setPending(false);
    setTitle('');
    setValidFrom('');
    setValidUntil('');
    setVersionReason('');
  }, []);

  const changeOpen = (next: boolean) => {
    if (!next) {
      abortController.current?.abort();
      abortController.current = null;
      reset();
    }
    onOpenChange(next);
  };

  const chooseFile = (candidate: File | undefined) => {
    if (!candidate) return;
    const validation = evidenceFileValidationError(candidate);
    if (validation) {
      setFile(null);
      setError(validation);
      return;
    }
    setFile(candidate);
    setError('');
    if (!title)
      setTitle(candidate.name.replace(/\.[^.]+$/, '').slice(0, EVIDENCE_TITLE_MAX_LENGTH));
  };

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedTitle = title.trim();
    if (!file) {
      setError('Choose exactly one evidence file.');
      return;
    }
    const fileError = evidenceFileValidationError(file);
    if (fileError) {
      setError(fileError);
      return;
    }
    if (!trimmedTitle) {
      setError('Evidence title is required.');
      return;
    }
    if (trimmedTitle.length > EVIDENCE_TITLE_MAX_LENGTH) {
      setError(`Evidence title must not exceed ${EVIDENCE_TITLE_MAX_LENGTH} characters.`);
      return;
    }
    if (description.length > EVIDENCE_DESCRIPTION_MAX_LENGTH) {
      setError(`Description must not exceed ${EVIDENCE_DESCRIPTION_MAX_LENGTH} characters.`);
      return;
    }
    const dateError = validityError(validFrom, validUntil);
    if (dateError) {
      setError(dateError);
      return;
    }
    if (supersedes) {
      const reasonError = versionReasonError(versionReason);
      if (reasonError) {
        setError(reasonError);
        return;
      }
    }

    let parsedMetadata: Record<string, unknown> | undefined;
    try {
      parsedMetadata = parseEvidenceMetadata(metadata);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Metadata is invalid.');
      return;
    }

    const controller = new AbortController();
    abortController.current = controller;
    setPending(true);
    setError('');
    try {
      const input = {
        file,
        title: trimmedTitle,
        evidence_type: evidenceType,
        description: description.trim() || undefined,
        valid_from: validFrom || undefined,
        valid_until: validUntil || undefined,
        metadata: parsedMetadata,
      };
      const uploaded = supersedes
        ? await api.controls.supersedeEvidence(
            controlId,
            supersedes.id,
            { ...input, version_reason: versionReason.trim() },
            controller.signal,
          )
        : await api.controls.uploadEvidence(controlId, input, controller.signal);
      onUploaded(uploaded);
      changeOpen(false);
    } catch (caught) {
      if (isAbortError(caught)) {
        setError('Upload cancelled. The file was not added to this control.');
      } else {
        setError(formatEvidenceError(caught, supersedes ? 'supersede' : 'upload'));
      }
    } finally {
      if (abortController.current === controller) abortController.current = null;
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {supersedes ? 'Upload replacement evidence version' : 'Upload control evidence'}
          </DialogTitle>
          <DialogDescription>
            {supersedes
              ? `Create version ${supersedes.version_number + 1} while retaining immutable version ${supersedes.version_number}.`
              : 'Add one file. It is quarantined, content-checked, and malware-scanned before an evidence record is created.'}
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-5" noValidate onSubmit={(event) => void submit(event)}>
          {error && (
            <div
              ref={errorRef}
              role="alert"
              tabIndex={-1}
              className="flex gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive"
            >
              <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
              <p>{error}</p>
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="evidence-file">Evidence file</Label>
            <Input
              id="evidence-file"
              type="file"
              accept={EVIDENCE_ACCEPT}
              aria-describedby="evidence-file-help"
              disabled={pending}
              onChange={(event) => {
                chooseFile(event.target.files?.[0]);
                event.target.value = '';
              }}
            />
            <p id="evidence-file-help" className="text-xs text-muted-foreground">
              {EVIDENCE_FILE_TYPE_LABEL}. One file, up to {EVIDENCE_MAX_UPLOAD_MIB} MiB.
              Macro-enabled, encrypted, active-content, and mismatched files are rejected.
            </p>
            {file && (
              <div className="flex items-center gap-3 rounded-md border bg-muted/40 p-3">
                <FileCheck2 aria-hidden="true" className="h-5 w-5 shrink-0 text-primary" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{file.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {(file.size / 1024 / 1024).toFixed(2)} MiB · one file selected
                  </p>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  disabled={pending}
                  aria-label={`Remove ${file.name}`}
                  onClick={() => setFile(null)}
                >
                  <X aria-hidden="true" className="h-4 w-4" />
                </Button>
              </div>
            )}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            {supersedes && (
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="evidence-version-reason">Version reason</Label>
                <Input
                  id="evidence-version-reason"
                  required
                  maxLength={EVIDENCE_VERSION_REASON_MAX_LENGTH}
                  value={versionReason}
                  disabled={pending}
                  placeholder="Describe why this replacement is required"
                  onChange={(event) => setVersionReason(event.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Required, 3-{EVIDENCE_VERSION_REASON_MAX_LENGTH} characters on one line. The reason
                  becomes part of immutable version history.
                </p>
              </div>
            )}
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="evidence-title">Title</Label>
              <Input
                id="evidence-title"
                required
                maxLength={EVIDENCE_TITLE_MAX_LENGTH}
                value={title}
                disabled={pending}
                onChange={(event) => setTitle(event.target.value)}
              />
              <p className="text-right text-xs text-muted-foreground">
                {title.length}/{EVIDENCE_TITLE_MAX_LENGTH}
              </p>
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="evidence-type">Evidence type</Label>
              <select
                id="evidence-type"
                value={evidenceType}
                disabled={pending}
                onChange={(event) => setEvidenceType(event.target.value as EvidenceType)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2"
              >
                {EVIDENCE_TYPES.map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="evidence-description">Description</Label>
              <Textarea
                id="evidence-description"
                rows={3}
                maxLength={EVIDENCE_DESCRIPTION_MAX_LENGTH}
                value={description}
                disabled={pending}
                onChange={(event) => setDescription(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="evidence-valid-from">Valid from</Label>
              <Input
                id="evidence-valid-from"
                type="date"
                value={validFrom}
                disabled={pending}
                onChange={(event) => setValidFrom(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="evidence-valid-until">Valid until</Label>
              <Input
                id="evidence-valid-until"
                type="date"
                min={validFrom || undefined}
                value={validUntil}
                disabled={pending}
                onChange={(event) => setValidUntil(event.target.value)}
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="evidence-metadata">Metadata (optional JSON object)</Label>
              <Textarea
                id="evidence-metadata"
                rows={4}
                spellCheck={false}
                placeholder={'{"source_system":"approved repository"}'}
                value={metadata}
                disabled={pending}
                onChange={(event) => setMetadata(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Maximum {EVIDENCE_METADATA_MAX_BYTES / 1024} KiB. Do not include passwords, tokens,
                object-store paths, or other secrets; security metadata is server managed.
              </p>
            </div>
          </div>

          <div className="flex gap-2 rounded-md border bg-muted/30 p-3 text-xs text-muted-foreground">
            <ShieldCheck aria-hidden="true" className="h-4 w-4 shrink-0 text-primary" />
            <p>
              The browser never supplies checksums, storage keys, scan verdicts, or trusted file
              metadata. The service derives and verifies them before storage.
            </p>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                if (pending) {
                  abortController.current?.abort();
                } else {
                  changeOpen(false);
                }
              }}
            >
              {pending ? 'Cancel upload' : 'Cancel'}
            </Button>
            <Button type="submit" disabled={pending}>
              {pending ? (
                <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <UploadCloud aria-hidden="true" className="mr-2 h-4 w-4" />
              )}
              {pending
                ? 'Scanning securely…'
                : supersedes
                  ? 'Upload replacement'
                  : 'Upload and scan'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
