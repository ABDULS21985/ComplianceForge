'use client';

import * as React from 'react';

import { Check, Copy, ShieldAlert } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';

export function RecoveryCodesDialog({ codes, onClose }: { codes: string[]; onClose: () => void }) {
  const [acknowledged, setAcknowledged] = React.useState(false);
  const [copied, setCopied] = React.useState(false);
  const [copyError, setCopyError] = React.useState('');
  const open = codes.length > 0;

  async function copy() {
    setCopyError('');
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard unavailable');
      await navigator.clipboard.writeText(codes.join('\n'));
      setCopied(true);
    } catch {
      setCopyError('Codes could not be copied. Select and save them manually.');
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && acknowledged) onClose();
      }}
    >
      <DialogContent
        onEscapeKeyDown={(event) => {
          if (!acknowledged) event.preventDefault();
        }}
        onPointerDownOutside={(event) => {
          if (!acknowledged) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldAlert aria-hidden="true" className="h-5 w-5 text-amber-600" />
            Save your recovery codes now
          </DialogTitle>
          <DialogDescription>
            Each code works once. These codes will not be shown again, are held only in page memory,
            and will be cleared when this dialog closes.
          </DialogDescription>
        </DialogHeader>
        <ol
          aria-label="One-time recovery codes"
          className="grid grid-cols-1 gap-2 rounded-md border bg-muted/40 p-4 font-mono text-sm sm:grid-cols-2"
        >
          {codes.map((code) => (
            <li key={code}>{code}</li>
          ))}
        </ol>
        <Button type="button" variant="outline" onClick={() => void copy()}>
          {copied ? (
            <Check aria-hidden="true" className="mr-2 h-4 w-4" />
          ) : (
            <Copy aria-hidden="true" className="mr-2 h-4 w-4" />
          )}
          {copied ? 'Copied' : 'Copy codes'}
        </Button>
        {copyError && (
          <p role="alert" className="text-sm text-destructive">
            {copyError}
          </p>
        )}
        <p className="text-xs text-muted-foreground">
          Copying places these credentials on the operating-system clipboard. Move them immediately
          to an approved password manager, then clear the clipboard.
        </p>
        <label className="flex items-start gap-3 rounded-md border p-3 text-sm">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4"
            checked={acknowledged}
            onChange={(event) => setAcknowledged(event.target.checked)}
          />
          <span>I saved these codes in an approved secure location.</span>
        </label>
        <DialogFooter>
          <Button type="button" disabled={!acknowledged} onClick={onClose}>
            Clear codes and finish
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
