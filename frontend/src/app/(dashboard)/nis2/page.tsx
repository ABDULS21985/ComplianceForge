'use client';

import * as React from 'react';
import {
  ShieldCheck,
  CheckCircle2,
  Clock,
  XCircle,
  Loader2,
  GraduationCap,
  Building2,
  AlertTriangle,
  Link2,
  ChevronDown,
  ChevronUp,
} from 'lucide-react';

import { cn, formatDate } from '@/lib/utils';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import api from '@/lib/api';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getEntityTypeColor(type: string): string {
  switch (type) {
    case 'essential': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'important': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function getMeasureStatusColor(status: string): string {
  switch (status) {
    case 'verified': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'implemented': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'in_progress': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'not_started': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function PhaseIndicator({ done, label }: { done: boolean | null; label: string }) {
  return (
    <div className="flex items-center gap-1 text-xs">
      {done === true ? (
        <CheckCircle2 className="h-3.5 w-3.5 text-green-500" />
      ) : done === false ? (
        <XCircle className="h-3.5 w-3.5 text-red-500" />
      ) : (
        <Clock className="h-3.5 w-3.5 text-amber-500" />
      )}
      <span>{label}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function NIS2Page() {
  const qc = useQueryClient();
  const [expandedMeasure, setExpandedMeasure] = React.useState<string | null>(null);
  const [updateStatusId, setUpdateStatusId] = React.useState<string | null>(null);
  const [newStatus, setNewStatus] = React.useState('');
  const [trainingOpen, setTrainingOpen] = React.useState(false);
  const [trainingForm, setTrainingForm] = React.useState({
    board_member_id: '',
    board_member_name: '',
    training_type: '',
    training_date: new Date().toISOString().split('T')[0],
    notes: '',
  });

  // Fetch assessment
  const { data: assessmentData, isLoading: assessLoading, isError: assessError } = useQuery({
    queryKey: ['nis2', 'assessment'],
    queryFn: () => api.nis2.getAssessment(),
  });
  const assessment = (assessmentData ?? {}) as any;

  // Fetch measures
  const { data: measuresData, isLoading: measuresLoading } = useQuery({
    queryKey: ['nis2', 'measures'],
    queryFn: () => api.nis2.getMeasures(),
  });
  const measures: Record<string, unknown>[] =
    (Array.isArray(measuresData) ? measuresData : (measuresData as any)?.items) as any[] ?? [];

  // Fetch incident reports
  const { data: incidentsData, isLoading: incidentsLoading } = useQuery({
    queryKey: ['nis2', 'incidents'],
    queryFn: () => api.nis2.listIncidentReports(),
  });
  const incidents: Record<string, unknown>[] =
    (Array.isArray(incidentsData) ? incidentsData : (incidentsData as any)?.items) as any[] ?? [];

  // Fetch management accountability
  const { data: managementData, isLoading: mgmtLoading } = useQuery({
    queryKey: ['nis2', 'management'],
    queryFn: () => api.nis2.getManagement(),
  });
  const management: Record<string, unknown>[] =
    (Array.isArray(managementData) ? managementData : (managementData as any)?.items) as any[] ?? [];

  // Mutations
  const updateMeasure = useMutation({
    mutationFn: ({ measureId, data }: { measureId: string; data: unknown }) =>
      api.nis2.updateMeasure(measureId, data),
    onSuccess: () => {
      toast.success('Measure status updated.');
      setUpdateStatusId(null);
      setNewStatus('');
      qc.invalidateQueries({ queryKey: ['nis2', 'measures'] });
    },
    onError: () => toast.error('Failed to update measure.'),
  });

  const recordTraining = useMutation({
    mutationFn: (data: unknown) => api.nis2.recordTraining(data),
    onSuccess: () => {
      toast.success('Training recorded.');
      setTrainingOpen(false);
      setTrainingForm({
        board_member_id: '',
        board_member_name: '',
        training_type: '',
        training_date: new Date().toISOString().split('T')[0],
        notes: '',
      });
      qc.invalidateQueries({ queryKey: ['nis2', 'management'] });
    },
    onError: () => toast.error('Failed to record training.'),
  });

  const createAssessment = useMutation({
    mutationFn: (data: unknown) => api.nis2.createAssessment(data),
    onSuccess: () => {
      toast.success('Entity assessment created.');
      qc.invalidateQueries({ queryKey: ['nis2', 'assessment'] });
    },
    onError: () => toast.error('Failed to create assessment.'),
  });

  const hasAssessment = !!assessment.entity_type;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">NIS2 Compliance</h1>
        <p className="text-muted-foreground">
          EU NIS2 Directive compliance management. Entity assessment, security measures, incident reporting, and management accountability.
        </p>
      </div>

      {/* Entity Assessment */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Building2 className="h-5 w-5" />
            Entity Assessment
          </CardTitle>
        </CardHeader>
        <CardContent>
          {assessLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-4 w-64" />
            </div>
          ) : assessError ? (
            <p className="text-sm text-destructive">Failed to load assessment.</p>
          ) : !hasAssessment ? (
            <div className="text-center py-6">
              <ShieldCheck className="mx-auto mb-3 h-10 w-10 text-muted-foreground" />
              <p className="text-muted-foreground mb-4">No entity assessment found. Assess your organization against NIS2 criteria.</p>
              <Button onClick={() => createAssessment.mutate({
                entity_type: 'important',
                sector: 'digital_infrastructure',
                member_state: 'IE',
              })}>
                {createAssessment.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Assess Entity
              </Button>
            </div>
          ) : (
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div>
                <p className="text-xs text-muted-foreground">Entity Type</p>
                <Badge className={cn('mt-1', getEntityTypeColor(assessment.entity_type as string))}>
                  {(assessment.entity_type as string)?.replace('_', ' ')}
                </Badge>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Sector</p>
                <p className="font-medium capitalize">{(assessment.sector as string)?.replace('_', ' ') ?? '—'}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Member State</p>
                <p className="font-medium">{assessment.member_state as string ?? '—'}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Competent Authority</p>
                <p className="font-medium">{assessment.competent_authority as string ?? '—'}</p>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Security Measures (Article 21) */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5" />
            Security Measures (Article 21)
          </CardTitle>
        </CardHeader>
        <CardContent>
          {measuresLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-16 w-full" />
              ))}
            </div>
          ) : measures.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">
              No security measures configured. Measures will appear once entity assessment is complete.
            </p>
          ) : (
            <div className="grid gap-3 md:grid-cols-2">
              {measures.map((measure: any) => {
                const mId = measure.id as string;
                const isExpanded = expandedMeasure === mId;
                return (
                  <div
                    key={mId}
                    className="rounded-lg border p-4 transition-colors hover:bg-muted/30"
                  >
                    <div className="flex items-start justify-between">
                      <div className="flex-1">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs text-muted-foreground">
                            {measure.measure_code as string}
                          </span>
                          <Badge className={getMeasureStatusColor(measure.status as string)}>
                            {(measure.status as string)?.replace('_', ' ')}
                          </Badge>
                        </div>
                        <p className="font-medium mt-1 text-sm">{measure.title as string}</p>
                        <div className="flex items-center gap-3 mt-2 text-xs text-muted-foreground">
                          {measure.owner_name && (
                            <span>Owner: {measure.owner_name as string}</span>
                          )}
                          {typeof measure.linked_controls_count === 'number' && (
                            <span className="flex items-center gap-1">
                              <Link2 className="h-3 w-3" />
                              {measure.linked_controls_count as number} ISO 27001 controls
                            </span>
                          )}
                        </div>
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setExpandedMeasure(isExpanded ? null : mId)}
                      >
                        {isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                      </Button>
                    </div>

                    {isExpanded && (
                      <div className="mt-3 pt-3 border-t space-y-2">
                        {measure.description && (
                          <p className="text-sm text-muted-foreground">{measure.description as string}</p>
                        )}
                        <div className="flex items-center gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              setUpdateStatusId(mId);
                              setNewStatus(measure.status as string);
                            }}
                          >
                            Update Status
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Incident Reports */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5" />
            NIS2 Incident Reports
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {incidentsLoading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : incidents.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              <AlertTriangle className="mx-auto mb-3 h-8 w-8" />
              <p className="text-sm">No NIS2 incident reports.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Ref</th>
                    <th className="px-4 py-3 text-left font-medium">Title</th>
                    <th className="px-4 py-3 text-left font-medium">Date</th>
                    <th className="px-4 py-3 text-center font-medium">
                      <div className="text-xs">Early Warning</div>
                      <div className="text-[10px] text-muted-foreground">24h</div>
                    </th>
                    <th className="px-4 py-3 text-center font-medium">
                      <div className="text-xs">Notification</div>
                      <div className="text-[10px] text-muted-foreground">72h</div>
                    </th>
                    <th className="px-4 py-3 text-center font-medium">
                      <div className="text-xs">Final Report</div>
                      <div className="text-[10px] text-muted-foreground">1 month</div>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {incidents.map((inc) => (
                    <tr key={inc.id as string} className="border-b hover:bg-muted/50">
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                        {inc.incident_ref as string ?? inc.ref as string}
                      </td>
                      <td className="px-4 py-3 font-medium">{inc.title as string}</td>
                      <td className="px-4 py-3">{formatDate(inc.incident_date as string ?? inc.created_at as string)}</td>
                      <td className="px-4 py-3 text-center">
                        <PhaseIndicator
                          done={inc.early_warning_submitted as boolean | null}
                          label=""
                        />
                      </td>
                      <td className="px-4 py-3 text-center">
                        <PhaseIndicator
                          done={inc.notification_submitted as boolean | null}
                          label=""
                        />
                      </td>
                      <td className="px-4 py-3 text-center">
                        <PhaseIndicator
                          done={inc.final_report_submitted as boolean | null}
                          label=""
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>

              {/* Visual Timeline Legend */}
              <div className="px-4 py-3 border-t bg-muted/30">
                <div className="flex items-center gap-6 text-xs text-muted-foreground">
                  <span className="font-medium">Reporting Timeline:</span>
                  <div className="flex items-center gap-1">
                    <div className="h-1 w-8 bg-red-400 rounded" />
                    <span>24h Early Warning</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <div className="h-1 w-12 bg-amber-400 rounded" />
                    <span>72h Notification</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <div className="h-1 w-16 bg-green-400 rounded" />
                    <span>1 Month Final Report</span>
                  </div>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Management Accountability */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="flex items-center gap-2">
            <GraduationCap className="h-5 w-5" />
            Management Accountability
          </CardTitle>
          <Button variant="outline" size="sm" onClick={() => setTrainingOpen(true)}>
            Record Training
          </Button>
        </CardHeader>
        <CardContent className="p-0">
          {mgmtLoading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : management.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              <GraduationCap className="mx-auto mb-3 h-8 w-8" />
              <p className="text-sm">No board members recorded. Add management accountability records.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Name</th>
                    <th className="px-4 py-3 text-left font-medium">Role</th>
                    <th className="px-4 py-3 text-center font-medium">Training Status</th>
                    <th className="px-4 py-3 text-center font-medium">Approval Status</th>
                    <th className="px-4 py-3 text-left font-medium">Last Training</th>
                    <th className="px-4 py-3 text-left font-medium">Next Training Due</th>
                  </tr>
                </thead>
                <tbody>
                  {management.map((member: any) => {
                    const trainingComplete = member.training_completed as boolean;
                    const approvalDone = member.approval_given as boolean;
                    return (
                      <tr key={member.id as string} className="border-b hover:bg-muted/50">
                        <td className="px-4 py-3 font-medium">{member.name as string}</td>
                        <td className="px-4 py-3 text-muted-foreground">{member.role as string ?? '—'}</td>
                        <td className="px-4 py-3 text-center">
                          {trainingComplete ? (
                            <CheckCircle2 className="mx-auto h-5 w-5 text-green-500" />
                          ) : (
                            <XCircle className="mx-auto h-5 w-5 text-red-500" />
                          )}
                        </td>
                        <td className="px-4 py-3 text-center">
                          {approvalDone ? (
                            <CheckCircle2 className="mx-auto h-5 w-5 text-green-500" />
                          ) : (
                            <XCircle className="mx-auto h-5 w-5 text-red-500" />
                          )}
                        </td>
                        <td className="px-4 py-3">{formatDate(member.last_training_date as string)}</td>
                        <td className="px-4 py-3">
                          {member.next_training_due ? (
                            <span className={cn(
                              'font-medium',
                              new Date(member.next_training_due as string) < new Date() && 'text-red-600 dark:text-red-400',
                            )}>
                              {formatDate(member.next_training_due as string)}
                            </span>
                          ) : '—'}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Update Measure Status Dialog */}
      <Dialog open={!!updateStatusId} onOpenChange={(v) => { if (!v) setUpdateStatusId(null); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Update Measure Status</DialogTitle>
            <DialogDescription>Change the implementation status of this security measure.</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <Label>New Status</Label>
            <Select value={newStatus} onValueChange={setNewStatus}>
              <SelectTrigger>
                <SelectValue placeholder="Select status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="not_started">Not Started</SelectItem>
                <SelectItem value="in_progress">In Progress</SelectItem>
                <SelectItem value="implemented">Implemented</SelectItem>
                <SelectItem value="verified">Verified</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setUpdateStatusId(null)}>Cancel</Button>
            <Button
              disabled={!newStatus || updateMeasure.isPending}
              onClick={() => {
                if (updateStatusId) {
                  updateMeasure.mutate({ measureId: updateStatusId, data: { status: newStatus } });
                }
              }}
            >
              {updateMeasure.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Update
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Record Training Dialog */}
      <Dialog open={trainingOpen} onOpenChange={setTrainingOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Record Training</DialogTitle>
            <DialogDescription>
              Record NIS2 cybersecurity training for a board member or senior management.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="train-name">Board Member Name *</Label>
              <Input
                id="train-name"
                value={trainingForm.board_member_name}
                onChange={(e) => setTrainingForm((f) => ({ ...f, board_member_name: e.target.value }))}
                placeholder="Full name"
              />
            </div>
            <div className="space-y-2">
              <Label>Training Type *</Label>
              <Select
                value={trainingForm.training_type}
                onValueChange={(v) => setTrainingForm((f) => ({ ...f, training_type: v }))}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="cybersecurity_awareness">Cybersecurity Awareness</SelectItem>
                  <SelectItem value="incident_response">Incident Response</SelectItem>
                  <SelectItem value="risk_management">Risk Management</SelectItem>
                  <SelectItem value="nis2_obligations">NIS2 Obligations</SelectItem>
                  <SelectItem value="supply_chain_security">Supply Chain Security</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="train-date">Training Date</Label>
              <Input
                id="train-date"
                type="date"
                value={trainingForm.training_date}
                onChange={(e) => setTrainingForm((f) => ({ ...f, training_date: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="train-notes">Notes</Label>
              <Textarea
                id="train-notes"
                value={trainingForm.notes}
                onChange={(e) => setTrainingForm((f) => ({ ...f, notes: e.target.value }))}
                placeholder="Additional notes..."
                rows={2}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setTrainingOpen(false)}>Cancel</Button>
            <Button
              disabled={!trainingForm.board_member_name || !trainingForm.training_type || recordTraining.isPending}
              onClick={() => recordTraining.mutate(trainingForm)}
            >
              {recordTraining.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Record Training
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
