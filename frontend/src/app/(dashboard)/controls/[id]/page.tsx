'use client';

import { useState, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  ChevronLeft,
  AlertCircle,
  Upload,
  Plus,
  FileText,
  TestTube,
  CheckCircle2,
  Clock,
  XCircle,
  Save,
} from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn, getStatusColor, formatDate, formatDateTime } from '@/lib/utils';
import { MATURITY_LABELS } from '@/lib/constants';
import api from '@/lib/api';
import type {
  ControlImplementation,
  ControlEvidence,
  ControlTestResult,
} from '@/types';
import Link from 'next/link';

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

function useControlImplementation(id: string) {
  return useQuery<ControlImplementation>({
    queryKey: ['controls', id],
    queryFn: () => api.controls.get(id) as Promise<ControlImplementation>,
    enabled: !!id,
  });
}

function useUpdateControlImplementation(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<ControlImplementation>) =>
      api.controls.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['controls', id] });
    },
  });
}

function useUploadEvidence(controlId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (formData: FormData) =>
      api.controls.uploadEvidence(controlId, formData),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['controls', controlId] });
    },
  });
}

function useRecordTest(controlId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: {
      test_type: string;
      test_procedure?: string;
      result: string;
      findings?: string;
      recommendations?: string;
    }) => api.controls.recordTest(controlId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['controls', controlId] });
    },
  });
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STATUS_OPTIONS = [
  { value: 'not_applicable', label: 'Not Applicable' },
  { value: 'not_implemented', label: 'Not Implemented' },
  { value: 'planned', label: 'Planned' },
  { value: 'partial', label: 'Partial' },
  { value: 'implemented', label: 'Implemented' },
  { value: 'effective', label: 'Effective' },
] as const;

const TEST_RESULT_OPTIONS = ['pass', 'fail', 'inconclusive', 'not_tested'];

// ---------------------------------------------------------------------------
// Skeleton
// ---------------------------------------------------------------------------

function PageSkeleton() {
  return (
    <div className="space-y-6">
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="space-y-3">
        <div className="h-8 w-96 animate-pulse rounded bg-muted" />
        <div className="flex gap-2">
          <div className="h-6 w-20 animate-pulse rounded-full bg-muted" />
          <div className="h-6 w-24 animate-pulse rounded-full bg-muted" />
          <div className="h-6 w-16 animate-pulse rounded-full bg-muted" />
        </div>
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2 space-y-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="animate-pulse space-y-2">
              <div className="h-4 w-28 rounded bg-muted" />
              <div className="h-10 w-full rounded bg-muted" />
            </div>
          ))}
        </div>
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <div className="animate-pulse space-y-3">
                  <div className="h-4 w-24 rounded bg-muted" />
                  <div className="h-16 w-full rounded bg-muted" />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Evidence type badge color
// ---------------------------------------------------------------------------

function evidenceTypeBadgeClass(type: string): string {
  switch (type) {
    case 'document':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'screenshot':
      return 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400';
    case 'log':
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
    case 'config':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function reviewStatusIcon(status: string) {
  switch (status) {
    case 'approved':
      return <CheckCircle2 className="h-4 w-4 text-green-500" />;
    case 'pending':
      return <Clock className="h-4 w-4 text-yellow-500" />;
    case 'rejected':
      return <XCircle className="h-4 w-4 text-red-500" />;
    default:
      return <Clock className="h-4 w-4 text-muted-foreground" />;
  }
}

// ---------------------------------------------------------------------------
// Record Test Dialog (inline)
// ---------------------------------------------------------------------------

function RecordTestForm({
  onSubmit,
  isLoading,
  onCancel,
}: {
  onSubmit: (data: {
    test_type: string;
    test_procedure?: string;
    result: string;
    findings?: string;
    recommendations?: string;
  }) => void;
  isLoading: boolean;
  onCancel: () => void;
}) {
  const [testType, setTestType] = useState('manual');
  const [procedure, setProcedure] = useState('');
  const [result, setResult] = useState('pass');
  const [findings, setFindings] = useState('');
  const [recommendations, setRecommendations] = useState('');

  return (
    <div className="space-y-4 rounded-lg border p-4">
      <h4 className="font-semibold">Record Test Result</h4>
      <div className="space-y-3">
        <div>
          <label className="text-sm font-medium">Test Type</label>
          <select
            className="mt-1 flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            value={testType}
            onChange={(e) => setTestType(e.target.value)}
          >
            <option value="manual">Manual</option>
            <option value="automated">Automated</option>
            <option value="walkthrough">Walkthrough</option>
            <option value="inspection">Inspection</option>
          </select>
        </div>
        <div>
          <label className="text-sm font-medium">Test Procedure</label>
          <textarea
            className="mt-1 flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            value={procedure}
            onChange={(e) => setProcedure(e.target.value)}
            placeholder="Describe the test procedure..."
          />
        </div>
        <div>
          <label className="text-sm font-medium">Result</label>
          <select
            className="mt-1 flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            value={result}
            onChange={(e) => setResult(e.target.value)}
          >
            {TEST_RESULT_OPTIONS.map((r) => (
              <option key={r} value={r}>
                {r.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase())}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="text-sm font-medium">Findings</label>
          <textarea
            className="mt-1 flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            value={findings}
            onChange={(e) => setFindings(e.target.value)}
            placeholder="Any findings from the test..."
          />
        </div>
        <div>
          <label className="text-sm font-medium">Recommendations</label>
          <textarea
            className="mt-1 flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            value={recommendations}
            onChange={(e) => setRecommendations(e.target.value)}
            placeholder="Recommendations..."
          />
        </div>
      </div>
      <div className="flex gap-2">
        <Button
          onClick={() =>
            onSubmit({
              test_type: testType,
              test_procedure: procedure || undefined,
              result,
              findings: findings || undefined,
              recommendations: recommendations || undefined,
            })
          }
          disabled={isLoading}
        >
          {isLoading ? 'Saving...' : 'Save Test'}
        </Button>
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function ControlImplementationDetailPage() {
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;

  const { data: impl, isLoading, error } = useControlImplementation(id);
  const updateMutation = useUpdateControlImplementation(id);
  const uploadEvidenceMutation = useUploadEvidence(id);
  const recordTestMutation = useRecordTest(id);

  // Local form state
  const [formState, setFormState] = useState<{
    status?: string;
    maturity_level?: number;
    owner_user_id?: string;
    implementation_description?: string;
    gap_description?: string;
    remediation_plan?: string;
    remediation_due_date?: string;
    compensating_control_description?: string;
  }>({});
  const [formDirty, setFormDirty] = useState(false);
  const [showTestForm, setShowTestForm] = useState(false);

  // Sync form state when data loads
  const currentStatus = formDirty
    ? (formState.status ?? impl?.status)
    : impl?.status;
  const currentMaturity = formDirty
    ? (formState.maturity_level ?? impl?.maturity_level)
    : impl?.maturity_level;

  const updateField = useCallback(
    (field: string, value: unknown) => {
      setFormState((prev) => ({ ...prev, [field]: value }));
      setFormDirty(true);
    },
    []
  );

  const handleSave = useCallback(() => {
    if (!formDirty) return;
    updateMutation.mutate(formState as Partial<ControlImplementation>, {
      onSuccess: () => setFormDirty(false),
    });
  }, [formState, formDirty, updateMutation]);

  const handleFileUpload = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (!files?.length) return;
      const formData = new FormData();
      for (let i = 0; i < files.length; i++) {
        formData.append('files', files[i]);
      }
      uploadEvidenceMutation.mutate(formData);
      e.target.value = '';
    },
    [uploadEvidenceMutation]
  );

  const handleRecordTest = useCallback(
    (data: {
      test_type: string;
      test_procedure?: string;
      result: string;
      findings?: string;
      recommendations?: string;
    }) => {
      recordTestMutation.mutate(data, {
        onSuccess: () => setShowTestForm(false),
      });
    },
    [recordTestMutation]
  );

  // Loading
  if (isLoading) return <PageSkeleton />;

  // Error
  if (error || !impl) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" size="sm" onClick={() => router.back()}>
          <ChevronLeft className="mr-1 h-4 w-4" /> Back
        </Button>
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>
              {error
                ? 'Failed to load control implementation.'
                : 'Control not found.'}
            </span>
          </CardContent>
        </Card>
      </div>
    );
  }

  const showGapFields =
    currentStatus !== 'implemented' && currentStatus !== 'effective';

  return (
    <div className="space-y-6">
      {/* Back */}
      <Button variant="ghost" size="sm" onClick={() => router.back()}>
        <ChevronLeft className="mr-1 h-4 w-4" /> Back
      </Button>

      {/* Header */}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-bold tracking-tight">
            {impl.control?.code ?? 'Control'}{' '}
            {impl.control?.title && `\u2014 ${impl.control.title}`}
          </h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge className={cn('text-xs', getStatusColor(impl.status))}>
            {impl.status.replace(/_/g, ' ')}
          </Badge>
          <Badge variant="outline" className="text-xs">
            Maturity: {impl.maturity_level}/5 (
            {MATURITY_LABELS[impl.maturity_level] ?? 'Unknown'})
          </Badge>
          {impl.control?.framework_id && (
            <Badge variant="secondary" className="text-xs">
              Framework
            </Badge>
          )}
        </div>
      </div>

      {/* Main content */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Left column: Implementation details form */}
        <div className="lg:col-span-2 space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Implementation Details</CardTitle>
            </CardHeader>
            <CardContent className="space-y-5">
              {/* Status selector */}
              <div>
                <label className="text-sm font-medium">Status</label>
                <select
                  className="mt-1 flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  value={currentStatus ?? impl.status}
                  onChange={(e) => updateField('status', e.target.value)}
                >
                  {STATUS_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* Maturity level slider */}
              <div>
                <label className="text-sm font-medium">
                  Maturity Level: {currentMaturity ?? impl.maturity_level}/5 (
                  {MATURITY_LABELS[currentMaturity ?? impl.maturity_level ?? 0] ??
                    'Unknown'}
                  )
                </label>
                <input
                  type="range"
                  min={0}
                  max={5}
                  step={1}
                  value={currentMaturity ?? impl.maturity_level ?? 0}
                  onChange={(e) =>
                    updateField('maturity_level', parseInt(e.target.value, 10))
                  }
                  className="mt-2 w-full accent-primary"
                />
                <div className="mt-1 flex justify-between text-xs text-muted-foreground">
                  {Array.from({ length: 6 }).map((_, i) => (
                    <span key={i}>{i}</span>
                  ))}
                </div>
              </div>

              {/* Owner selector */}
              <div>
                <label className="text-sm font-medium">Owner</label>
                <Input
                  className="mt-1"
                  placeholder="Owner user ID"
                  defaultValue={impl.owner_user_id ?? ''}
                  onChange={(e) =>
                    updateField('owner_user_id', e.target.value || undefined)
                  }
                />
              </div>

              {/* Implementation description */}
              <div>
                <label className="text-sm font-medium">
                  Implementation Description
                </label>
                <textarea
                  className="mt-1 flex min-h-[100px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  defaultValue={impl.implementation_description ?? ''}
                  onChange={(e) =>
                    updateField('implementation_description', e.target.value)
                  }
                  placeholder="Describe how this control is implemented..."
                />
              </div>

              {/* Gap description (conditional) */}
              {showGapFields && (
                <div>
                  <label className="text-sm font-medium">Gap Description</label>
                  <textarea
                    className="mt-1 flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    defaultValue={impl.gap_description ?? ''}
                    onChange={(e) =>
                      updateField('gap_description', e.target.value)
                    }
                    placeholder="Describe the gap..."
                  />
                </div>
              )}

              {/* Remediation plan + due date (conditional) */}
              {showGapFields && (
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div>
                    <label className="text-sm font-medium">
                      Remediation Plan
                    </label>
                    <textarea
                      className="mt-1 flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                      defaultValue={impl.remediation_plan ?? ''}
                      onChange={(e) =>
                        updateField('remediation_plan', e.target.value)
                      }
                      placeholder="Describe the remediation plan..."
                    />
                  </div>
                  <div>
                    <label className="text-sm font-medium">
                      Remediation Due Date
                    </label>
                    <Input
                      type="date"
                      className="mt-1"
                      defaultValue={impl.remediation_due_date ?? ''}
                      onChange={(e) =>
                        updateField('remediation_due_date', e.target.value)
                      }
                    />
                  </div>
                </div>
              )}

              {/* Compensating control */}
              <div>
                <label className="text-sm font-medium">
                  Compensating Control
                </label>
                <textarea
                  className="mt-1 flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  defaultValue={
                    impl.compensating_control_description ?? ''
                  }
                  onChange={(e) =>
                    updateField(
                      'compensating_control_description',
                      e.target.value
                    )
                  }
                  placeholder="Describe any compensating controls..."
                />
              </div>

              {/* Save button */}
              <div className="flex items-center gap-3">
                <Button
                  onClick={handleSave}
                  disabled={!formDirty || updateMutation.isPending}
                >
                  <Save className="mr-2 h-4 w-4" />
                  {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
                </Button>
                {updateMutation.isSuccess && (
                  <span className="text-sm text-green-600">Saved!</span>
                )}
                {updateMutation.isError && (
                  <span className="text-sm text-destructive">
                    Failed to save. Please try again.
                  </span>
                )}
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Right column: Evidence & Testing */}
        <div className="space-y-6">
          {/* Evidence */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Evidence</CardTitle>
              <label className="cursor-pointer">
                <input
                  type="file"
                  multiple
                  className="hidden"
                  onChange={handleFileUpload}
                  accept="*/*"
                />
                <Button
                  variant="outline"
                  size="sm"
                  asChild
                  disabled={uploadEvidenceMutation.isPending}
                >
                  <span>
                    <Upload className="mr-1 h-3.5 w-3.5" />
                    {uploadEvidenceMutation.isPending
                      ? 'Uploading...'
                      : 'Upload'}
                  </span>
                </Button>
              </label>
            </CardHeader>
            <CardContent>
              {uploadEvidenceMutation.isError && (
                <p className="mb-3 text-sm text-destructive">
                  Upload failed. Please try again.
                </p>
              )}
              {!impl.evidence || impl.evidence.length === 0 ? (
                <p className="py-4 text-center text-sm text-muted-foreground">
                  No evidence uploaded yet.
                </p>
              ) : (
                <div className="space-y-3">
                  {impl.evidence.map((ev) => (
                    <div
                      key={ev.id}
                      className="flex items-start gap-3 rounded-lg border p-3"
                    >
                      <FileText className="mt-0.5 h-4 w-4 flex-shrink-0 text-muted-foreground" />
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium truncate">
                          {ev.file_name ?? ev.title}
                        </p>
                        <div className="mt-1 flex flex-wrap items-center gap-1.5">
                          <Badge
                            className={cn(
                              'text-xs',
                              evidenceTypeBadgeClass(ev.evidence_type)
                            )}
                          >
                            {ev.evidence_type}
                          </Badge>
                          <div className="flex items-center gap-1">
                            {reviewStatusIcon(ev.review_status)}
                            <span className="text-xs text-muted-foreground">
                              {ev.review_status}
                            </span>
                          </div>
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">
                          Collected {formatDate(ev.collected_at)}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Testing */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Test History</CardTitle>
              {!showTestForm && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setShowTestForm(true)}
                >
                  <Plus className="mr-1 h-3.5 w-3.5" />
                  Record Test
                </Button>
              )}
            </CardHeader>
            <CardContent>
              {showTestForm && (
                <div className="mb-4">
                  <RecordTestForm
                    onSubmit={handleRecordTest}
                    isLoading={recordTestMutation.isPending}
                    onCancel={() => setShowTestForm(false)}
                  />
                  {recordTestMutation.isError && (
                    <p className="mt-2 text-sm text-destructive">
                      Failed to record test. Please try again.
                    </p>
                  )}
                </div>
              )}

              {!impl.test_results || impl.test_results.length === 0 ? (
                <p className="py-4 text-center text-sm text-muted-foreground">
                  No tests recorded yet.
                </p>
              ) : (
                <div className="space-y-3">
                  {impl.test_results.map((test) => (
                    <div
                      key={test.id}
                      className="rounded-lg border p-3 space-y-1.5"
                    >
                      <div className="flex items-center justify-between">
                        <Badge
                          variant="outline"
                          className="text-xs capitalize"
                        >
                          {test.test_type}
                        </Badge>
                        <Badge
                          className={cn(
                            'text-xs',
                            test.result === 'pass'
                              ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                              : test.result === 'fail'
                                ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
                                : 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
                          )}
                        >
                          {test.result}
                        </Badge>
                      </div>
                      {test.findings && (
                        <p className="text-xs text-muted-foreground">
                          {test.findings}
                        </p>
                      )}
                      <p className="text-xs text-muted-foreground">
                        {formatDateTime(test.tested_at)}
                        {test.tested_by && ` by ${test.tested_by}`}
                      </p>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
