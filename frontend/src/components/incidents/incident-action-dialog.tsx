'use client';

import * as React from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  formatIncidentError,
  humanizeIncidentToken,
  incidentEscalationOptions,
} from '@/lib/incident';
import type { Incident, IncidentSeverity, IncidentStatus } from '@/types/incident';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  useCloseIncident,
  useEscalateIncident,
  useIncidentReasonAction,
  useIncidentTransition,
} from '@/lib/api-hooks';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Loader2 } from 'lucide-react';
import { Textarea } from '@/components/ui/textarea';

export type IncidentAction =
  | { kind: 'transition'; status: IncidentStatus }
  | { kind: 'cancel' | 'reopen' | 'close' }
  | { kind: 'escalate' };

interface IncidentActionDialogProps {
  incident: Incident;
  action: IncidentAction | null;
  onOpenChange: (open: boolean) => void;
}

export function IncidentActionDialog({ incident, action, onOpenChange }: IncidentActionDialogProps) {
  const transition = useIncidentTransition(incident.id);
  const reasonAction = useIncidentReasonAction(incident.id);
  const close = useCloseIncident(incident.id);
  const escalate = useEscalateIncident(incident.id);
  const [reason, setReason] = React.useState('');
  const [lessons, setLessons] = React.useState(incident.lessons_learned);
  const escalationOptions = incidentEscalationOptions(incident.severity);
  const [severity, setSeverity] = React.useState<IncidentSeverity>(
    escalationOptions[0] ?? incident.severity,
  );
  const [validationError, setValidationError] = React.useState<string | null>(null);
  const mutation = action?.kind === 'transition'
    ? transition
    : action?.kind === 'escalate'
      ? escalate
      : action?.kind === 'close'
        ? close
        : reasonAction;

  function handleOpenChange(open: boolean) {
    if (mutation.isPending) return;
    if (!open) {
      setReason('');
      setLessons(incident.lessons_learned);
      setSeverity(escalationOptions[0] ?? incident.severity);
      setValidationError(null);
    }
    onOpenChange(open);
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!action || mutation.isPending) return;
    const trimmedReason = reason.trim();
    if (trimmedReason.length < 3) {
      setValidationError('Enter a reason of at least 3 characters for the audit trail.');
      return;
    }
    if (action.kind === 'close' && lessons.trim().length < 1) {
      setValidationError('Lessons learned are required before closure.');
      return;
    }
    setValidationError(null);
    try {
      if (action.kind === 'transition') {
        await transition.mutateAsync({
          status: action.status,
          reason: trimmedReason,
          version: incident.version,
        });
      } else if (action.kind === 'escalate') {
        await escalate.mutateAsync({ severity, reason: trimmedReason, version: incident.version });
      } else if (action.kind === 'close') {
        await close.mutateAsync({
          version: incident.version,
          reason: trimmedReason,
          lessonsLearned: lessons.trim(),
        });
      } else {
        await reasonAction.mutateAsync({
          action: action.kind,
          data: { version: incident.version, reason: trimmedReason },
        });
      }
      handleOpenChange(false);
    } catch {
      // Keep the dialog open and show the mutation error.
    }
  }

  if (!action) return null;
  const presentation = actionPresentation(action);
  const error = validationError || (mutation.error
    ? formatIncidentError(mutation.error, 'The incident action could not be completed.')
    : null);

  return (
    <Dialog open onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{presentation.title}</DialogTitle>
          <DialogDescription>{presentation.description}</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          {action.kind === 'escalate' && (
            <div className="space-y-2">
              <Label htmlFor="incident-escalation-severity">New severity</Label>
              <Select value={severity} onValueChange={(value) => setSeverity(value as IncidentSeverity)}>
                <SelectTrigger id="incident-escalation-severity"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {escalationOptions.map((value) => (
                    <SelectItem key={value} value={value}>{humanizeIncidentToken(value)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          {action.kind === 'close' && (
            <div className="space-y-2">
              <Label htmlFor="incident-close-lessons">Lessons learned *</Label>
              <Textarea id="incident-close-lessons" rows={4} value={lessons} onChange={(event) => setLessons(event.target.value)} />
              <p className="text-xs text-muted-foreground">Saved to the record before the approved close action.</p>
            </div>
          )}
          <div className="space-y-2">
            <Label htmlFor="incident-action-reason">Reason *</Label>
            <Textarea
              id="incident-action-reason"
              autoFocus={action.kind !== 'close'}
              rows={3}
              maxLength={4000}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              aria-invalid={Boolean(validationError)}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => handleOpenChange(false)}>Cancel</Button>
            <Button type="submit" variant={action.kind === 'cancel' ? 'destructive' : 'default'} disabled={mutation.isPending || (action.kind === 'escalate' && escalationOptions.length === 0)}>
              {mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
              {presentation.confirmLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function actionPresentation(action: IncidentAction) {
  if (action.kind === 'transition') {
    const target = humanizeIncidentToken(action.status);
    return {
      title: `Move incident to ${target}?`,
      description: 'This versioned lifecycle event will be added to the immutable incident timeline.',
      confirmLabel: `Move to ${target}`,
    };
  }
  const values = {
    cancel: {
      title: 'Cancel incident?',
      description: 'Cancellation is a terminal state. A reason is mandatory and the record remains auditable.',
      confirmLabel: 'Cancel incident',
    },
    reopen: {
      title: 'Reopen incident?',
      description: 'Closed incidents reopen to Investigating; cancelled incidents reopen to Reported.',
      confirmLabel: 'Reopen incident',
    },
    close: {
      title: 'Close incident?',
      description: 'Notifiable breaches must have a recorded DPA notification. Closure also requires lessons learned.',
      confirmLabel: 'Close incident',
    },
    escalate: {
      title: 'Escalate severity?',
      description: 'Severity can only increase. This action is captured in the incident timeline.',
      confirmLabel: 'Escalate severity',
    },
  } as const;
  return values[action.kind];
}
