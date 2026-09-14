'use client';

import * as React from 'react';
import type { BreachAssessmentStatus, Incident } from '@/types/incident';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { formatIncidentError, localDateTimeToIso, toLocalDateTimeInput } from '@/lib/incident';
import { Loader2, ShieldAlert } from 'lucide-react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useAssessIncidentBreach } from '@/lib/api-hooks';

interface BreachAssessmentDialogProps {
  incident: Incident;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type AssessmentDecision = Exclude<BreachAssessmentStatus, 'pending'>;

export function BreachAssessmentDialog({ incident, open, onOpenChange }: BreachAssessmentDialogProps) {
  const mutation = useAssessIncidentBreach(incident.id);
  const [decision, setDecision] = React.useState<AssessmentDecision>(
    incident.breach_assessment_status === 'notifiable' ? 'notifiable' : 'not_notifiable',
  );
  const [reason, setReason] = React.useState(incident.breach_assessment_reason ?? '');
  const [isDataBreach, setIsDataBreach] = React.useState(incident.is_data_breach);
  const [awarenessAt, setAwarenessAt] = React.useState(toLocalDateTimeInput(incident.breach_awareness_at));
  const [subjects, setSubjects] = React.useState(incident.data_subjects_affected?.toString() ?? '');
  const [records, setRecords] = React.useState(incident.records_affected?.toString() ?? '');
  const [categories, setCategories] = React.useState(incident.data_categories.join(', '));
  const [specialCategoryData, setSpecialCategoryData] = React.useState(incident.special_category_data);
  const [crossBorder, setCrossBorder] = React.useState(incident.cross_border);
  const [nature, setNature] = React.useState(incident.breach_nature);
  const [consequences, setConsequences] = React.useState(incident.likely_consequences);
  const [mitigation, setMitigation] = React.useState(incident.mitigation_measures);
  const [validationError, setValidationError] = React.useState<string | null>(null);

  function handleOpenChange(next: boolean) {
    if (!mutation.isPending) onOpenChange(next);
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const parsedCategories = categories.split(',').map((value) => value.trim()).filter(Boolean);
    const awarenessIso = localDateTimeToIso(awarenessAt);
    if (reason.trim().length < 3) {
      setValidationError('Document an assessment reason of at least 3 characters.');
      return;
    }
    if (decision === 'notifiable' && (!isDataBreach || !awarenessIso)) {
      setValidationError('A notifiable decision requires a personal-data breach and awareness time.');
      return;
    }
    if (parsedCategories.length > 20) {
      setValidationError('Provide no more than 20 data categories.');
      return;
    }
    setValidationError(null);
    try {
      await mutation.mutateAsync({
        version: incident.version,
        status: decision,
        reason: reason.trim(),
        is_data_breach: isDataBreach,
        ...(awarenessIso ? { awareness_at: awarenessIso } : {}),
        ...(subjects ? { data_subjects_affected: Number(subjects) } : {}),
        ...(records ? { records_affected: Number(records) } : {}),
        data_categories: parsedCategories,
        special_category_data: specialCategoryData,
        cross_border: crossBorder,
        breach_nature: nature.trim(),
        likely_consequences: consequences.trim(),
        mitigation_measures: mitigation.trim(),
      });
      onOpenChange(false);
    } catch {
      // Mutation error remains visible.
    }
  }

  const error = validationError || (mutation.error
    ? formatIncidentError(mutation.error, 'The breach assessment could not be recorded.')
    : null);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2"><ShieldAlert aria-hidden="true" className="h-5 w-5" /> GDPR breach assessment</DialogTitle>
          <DialogDescription>
            A notifiable decision starts the 72-hour Article 33 deadline from the recorded awareness time. Once DPA notification is recorded, this assessment is immutable.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="breach-decision">Assessment decision *</Label>
              <Select value={decision} onValueChange={(value) => { const next = value as AssessmentDecision; setDecision(next); if (next === 'notifiable') setIsDataBreach(true); }}>
                <SelectTrigger id="breach-decision"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="not_notifiable">Not notifiable</SelectItem>
                  <SelectItem value="notifiable">Notifiable to DPA</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="breach-awareness">Awareness time{decision === 'notifiable' ? ' *' : ''}</Label>
              <Input id="breach-awareness" type="datetime-local" max={toLocalDateTimeInput(new Date().toISOString())} value={awarenessAt} onChange={(event) => setAwarenessAt(event.target.value)} />
            </div>
          </div>
          <label className="flex items-start gap-3 rounded-md border p-3">
            <input type="checkbox" className="mt-1 h-4 w-4" checked={isDataBreach} disabled={decision === 'notifiable'} onChange={(event) => setIsDataBreach(event.target.checked)} />
            <span><span className="block text-sm font-medium">Personal-data breach</span><span className="block text-xs text-muted-foreground">Personal data confidentiality, integrity, or availability was affected.</span></span>
          </label>
          <div className="space-y-2">
            <Label htmlFor="breach-reason">Decision rationale *</Label>
            <Textarea id="breach-reason" rows={3} maxLength={4000} value={reason} onChange={(event) => setReason(event.target.value)} />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="breach-subjects">Data subjects affected</Label><Input id="breach-subjects" type="number" min={0} value={subjects} onChange={(event) => setSubjects(event.target.value)} /></div>
            <div className="space-y-2"><Label htmlFor="breach-records">Records affected</Label><Input id="breach-records" type="number" min={0} value={records} onChange={(event) => setRecords(event.target.value)} /></div>
          </div>
          <div className="space-y-2"><Label htmlFor="breach-categories">Data categories</Label><Input id="breach-categories" value={categories} onChange={(event) => setCategories(event.target.value)} placeholder="Names, email addresses, credentials" /><p className="text-xs text-muted-foreground">Comma-separated, up to 20 categories.</p></div>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" className="h-4 w-4" checked={specialCategoryData} onChange={(event) => setSpecialCategoryData(event.target.checked)} />Special-category data affected</label>
            <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" className="h-4 w-4" checked={crossBorder} onChange={(event) => setCrossBorder(event.target.checked)} />Cross-border processing</label>
          </div>
          <div className="space-y-2"><Label htmlFor="breach-nature">Nature of breach</Label><Textarea id="breach-nature" rows={2} value={nature} onChange={(event) => setNature(event.target.value)} /></div>
          <div className="space-y-2"><Label htmlFor="breach-consequences">Likely consequences</Label><Textarea id="breach-consequences" rows={2} value={consequences} onChange={(event) => setConsequences(event.target.value)} /></div>
          <div className="space-y-2"><Label htmlFor="breach-mitigation">Mitigation measures</Label><Textarea id="breach-mitigation" rows={2} value={mitigation} onChange={(event) => setMitigation(event.target.value)} /></div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button type="submit" disabled={mutation.isPending}>{mutation.isPending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}Record assessment</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
