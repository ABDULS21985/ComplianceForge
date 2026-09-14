'use client';

import * as React from 'react';
import { Loader2 } from 'lucide-react';
import { z } from 'zod';

import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useAssignIncident, useUnassignIncident } from '@/lib/api-hooks';
import { formatIncidentError, humanizeIncidentToken } from '@/lib/incident';
import type { Incident, IncidentAssignment, IncidentAssignmentRole } from '@/types/incident';

interface IncidentAssignmentDialogProps {
  incident: Incident;
  open: boolean;
  assignment?: IncidentAssignment;
  onOpenChange: (open: boolean) => void;
}

export function IncidentAssignmentDialog({ incident, open, assignment, onOpenChange }: IncidentAssignmentDialogProps) {
  const assign = useAssignIncident(incident.id);
  const unassign = useUnassignIncident(incident.id);
  const mutation = assignment ? unassign : assign;
  const [assigneeId, setAssigneeId] = React.useState('');
  const [role, setRole] = React.useState<IncidentAssignmentRole>('investigator');
  const [reason, setReason] = React.useState('');
  const [validationError, setValidationError] = React.useState<string | null>(null);

  function close() {
    if (mutation.isPending) return;
    setAssigneeId('');
    setRole('investigator');
    setReason('');
    setValidationError(null);
    onOpenChange(false);
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!assignment && !z.string().uuid().safeParse(assigneeId.trim()).success) {
      setValidationError('Enter a valid organization user UUID.');
      return;
    }
    if (reason.trim().length < 3) {
      setValidationError('Enter a reason of at least 3 characters.');
      return;
    }
    setValidationError(null);
    try {
      if (assignment) {
        await unassign.mutateAsync({
          assignmentId: assignment.id,
          data: { version: incident.version, reason: reason.trim() },
        });
      } else {
        await assign.mutateAsync({
          version: incident.version,
          assignee_id: assigneeId.trim(),
          role,
          reason: reason.trim(),
        });
      }
      close();
    } catch {
      // Mutation error remains visible.
    }
  }

  const error = validationError || (mutation.error ? formatIncidentError(mutation.error, `Failed to ${assignment ? 'unassign' : 'assign'} responder.`) : null);
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) close(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{assignment ? 'Unassign responder?' : 'Assign responder'}</DialogTitle>
          <DialogDescription>
            {assignment
              ? `End the ${humanizeIncidentToken(assignment.role)} assignment for user ${assignment.assignee_user_id}.`
              : 'Assignments are versioned and retained in the incident history. Only users in this organization are accepted.'}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          {!assignment && (
            <>
              <div className="space-y-2"><Label htmlFor="assignment-user">Organization user UUID *</Label><Input id="assignment-user" autoFocus value={assigneeId} onChange={(event) => setAssigneeId(event.target.value)} aria-invalid={Boolean(validationError && !assigneeId)} /><p className="text-xs text-muted-foreground">A dedicated assignable-user lookup is not exposed by the incident API.</p></div>
              <div className="space-y-2"><Label htmlFor="assignment-role">Response role *</Label><Select value={role} onValueChange={(value) => setRole(value as IncidentAssignmentRole)}><SelectTrigger id="assignment-role"><SelectValue /></SelectTrigger><SelectContent>{(['primary', 'investigator', 'observer'] as const).map((value) => <SelectItem key={value} value={value}>{humanizeIncidentToken(value)}</SelectItem>)}</SelectContent></Select></div>
            </>
          )}
          <div className="space-y-2"><Label htmlFor="assignment-reason">Reason *</Label><Textarea id="assignment-reason" autoFocus={Boolean(assignment)} rows={3} maxLength={2000} value={reason} onChange={(event) => setReason(event.target.value)} /></div>
          <DialogFooter><Button type="button" variant="outline" disabled={mutation.isPending} onClick={close}>Cancel</Button><Button type="submit" variant={assignment ? 'destructive' : 'default'} disabled={mutation.isPending}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}{assignment ? 'Unassign responder' : 'Assign responder'}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
