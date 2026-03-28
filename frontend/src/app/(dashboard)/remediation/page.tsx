'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface RemediationAction {
  id: string;
  title: string;
  description: string;
  status: 'todo' | 'in_progress' | 'review' | 'done';
  priority: 'critical' | 'high' | 'medium' | 'low';
  assignee?: string;
  due_date?: string;
  ai_guidance?: string;
  control_ref?: string;
  effort_estimate?: string;
}

interface RemediationPlan {
  id: string;
  name: string;
  description: string;
  status: 'draft' | 'active' | 'completed' | 'archived';
  framework_ids: string[];
  created_at: string;
  target_date: string;
  progress: number;
  total_actions: number;
  completed_actions: number;
  actions?: RemediationAction[];
}

// ---------------------------------------------------------------------------
// Wizard Steps
// ---------------------------------------------------------------------------

type WizardStep = 'frameworks' | 'gaps' | 'generating' | 'review';

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function RemediationPlannerPage() {
  const queryClient = useQueryClient();
  const [selectedPlanId, setSelectedPlanId] = useState<string | null>(null);
  const [showWizard, setShowWizard] = useState(false);
  const [wizardStep, setWizardStep] = useState<WizardStep>('frameworks');
  const [selectedFrameworks, setSelectedFrameworks] = useState<string[]>([]);
  const [generatedPlan, setGeneratedPlan] = useState<any>(null);
  const [activeGuidanceActionId, setActiveGuidanceActionId] = useState<string | null>(null);
  const [dragItem, setDragItem] = useState<string | null>(null);

  // Fetch plans
  const { data: plansData, isLoading, error } = useQuery({
    queryKey: ['remediation-plans'],
    queryFn: () => api.remediation.listPlans(),
  });

  // Fetch selected plan detail
  const { data: planDetail } = useQuery({
    queryKey: ['remediation-plan', selectedPlanId],
    queryFn: () => api.remediation.getPlan(selectedPlanId!),
    enabled: !!selectedPlanId,
  });

  // Fetch progress for selected plan
  const { data: planProgress } = useQuery({
    queryKey: ['remediation-progress', selectedPlanId],
    queryFn: () => api.remediation.getPlanProgress(selectedPlanId!),
    enabled: !!selectedPlanId,
  });

  // Fetch frameworks for wizard
  const { data: frameworksData } = useQuery({
    queryKey: ['frameworks-list'],
    queryFn: () => api.frameworks.list({ page_size: 50 }),
    enabled: showWizard,
  });

  // Fetch gaps for selected frameworks
  const { data: gapsData } = useQuery({
    queryKey: ['compliance-gaps', selectedFrameworks],
    queryFn: () => api.compliance.gaps({ framework_id: selectedFrameworks[0] }),
    enabled: wizardStep === 'gaps' && selectedFrameworks.length > 0,
  });

  // Generate plan mutation
  const generateMutation = useMutation({
    mutationFn: (data: any) => api.remediation.generatePlan(data),
    onSuccess: (data) => {
      setGeneratedPlan(data);
      setWizardStep('review');
    },
  });

  // Approve plan mutation
  const approveMutation = useMutation({
    mutationFn: (id: string) => api.remediation.approvePlan(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['remediation-plans'] });
      setShowWizard(false);
      resetWizard();
    },
  });

  // Update action mutation (for kanban drag)
  const updateActionMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.remediation.updateAction(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['remediation-plan', selectedPlanId] });
    },
  });

  // Complete action mutation
  const completeActionMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.remediation.completeAction(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['remediation-plan', selectedPlanId] });
      queryClient.invalidateQueries({ queryKey: ['remediation-progress', selectedPlanId] });
    },
  });

  // AI guidance
  const guidanceMutation = useMutation({
    mutationFn: (data: any) => api.ai.controlGuidance(data),
  });

  function resetWizard() {
    setWizardStep('frameworks');
    setSelectedFrameworks([]);
    setGeneratedPlan(null);
  }

  function handleGeneratePlan() {
    setWizardStep('generating');
    generateMutation.mutate({ framework_ids: selectedFrameworks });
  }

  function handleDrop(actionId: string, newStatus: RemediationAction['status']) {
    updateActionMutation.mutate({ id: actionId, data: { status: newStatus } });
  }

  function handleRequestGuidance(action: RemediationAction) {
    setActiveGuidanceActionId(action.id);
    guidanceMutation.mutate({ control_ref: action.control_ref, context: action.title });
  }

  const plans: RemediationPlan[] = plansData?.items ?? plansData ?? [];

  // Derive stats
  const activePlans = plans.filter((p) => p.status === 'active').length;
  const completedPlans = plans.filter((p) => p.status === 'completed').length;
  const avgProgress = plans.length > 0
    ? Math.round(plans.reduce((sum, p) => sum + (p.progress ?? 0), 0) / plans.length)
    : 0;

  // Kanban columns
  const kanbanColumns: { key: RemediationAction['status']; label: string; color: string }[] = [
    { key: 'todo', label: 'To Do', color: 'bg-gray-100' },
    { key: 'in_progress', label: 'In Progress', color: 'bg-blue-50' },
    { key: 'review', label: 'In Review', color: 'bg-amber-50' },
    { key: 'done', label: 'Done', color: 'bg-green-50' },
  ];

  const actions: RemediationAction[] = planDetail?.actions ?? [];

  // ---------------------------------------------------------------------------
  // Loading / Error
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="p-6 space-y-4">
        <h1 className="text-2xl font-bold">AI Remediation Planner</h1>
        <div className="grid grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-24 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-20 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-4">AI Remediation Planner</h1>
        <div className="bg-red-50 text-red-700 rounded-lg p-4">
          Failed to load remediation plans. Please try again later.
        </div>
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Wizard Modal
  // ---------------------------------------------------------------------------

  if (showWizard) {
    return (
      <div className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">Generate AI Remediation Plan</h1>
          <button
            onClick={() => { setShowWizard(false); resetWizard(); }}
            className="px-4 py-2 text-sm font-medium rounded border border-gray-300 text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
        </div>

        {/* Progress Steps */}
        <div className="flex items-center gap-2">
          {(['frameworks', 'gaps', 'generating', 'review'] as WizardStep[]).map((step, idx) => (
            <div key={step} className="flex items-center gap-2">
              <div
                className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${
                  wizardStep === step
                    ? 'bg-blue-600 text-white'
                    : (['frameworks', 'gaps', 'generating', 'review'].indexOf(wizardStep) > idx
                        ? 'bg-green-100 text-green-700'
                        : 'bg-gray-100 text-gray-400')
                }`}
              >
                {idx + 1}
              </div>
              <span className={`text-sm ${wizardStep === step ? 'font-semibold text-gray-900' : 'text-gray-500'}`}>
                {step === 'frameworks' ? 'Select Frameworks' : step === 'gaps' ? 'Review Gaps' : step === 'generating' ? 'AI Generating' : 'Review Plan'}
              </span>
              {idx < 3 && <div className="w-8 h-px bg-gray-300" />}
            </div>
          ))}
        </div>

        {/* Step Content */}
        <div className="bg-white border rounded-lg p-6 min-h-[400px]">
          {wizardStep === 'frameworks' && (
            <div className="space-y-4">
              <p className="text-gray-600">Select frameworks to analyze for compliance gaps:</p>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
                {(frameworksData?.items ?? []).map((fw: any) => (
                  <label
                    key={fw.id}
                    className={`flex items-center gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                      selectedFrameworks.includes(fw.id)
                        ? 'border-blue-500 bg-blue-50'
                        : 'border-gray-200 hover:border-gray-300'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={selectedFrameworks.includes(fw.id)}
                      onChange={(e) => {
                        if (e.target.checked) setSelectedFrameworks([...selectedFrameworks, fw.id]);
                        else setSelectedFrameworks(selectedFrameworks.filter((id) => id !== fw.id));
                      }}
                      className="rounded border-gray-300 text-blue-600"
                    />
                    <span className="text-sm font-medium">{fw.name ?? fw.id}</span>
                  </label>
                ))}
              </div>
              <div className="flex justify-end">
                <button
                  onClick={() => setWizardStep('gaps')}
                  disabled={selectedFrameworks.length === 0}
                  className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                >
                  Next: Review Gaps
                </button>
              </div>
            </div>
          )}

          {wizardStep === 'gaps' && (
            <div className="space-y-4">
              <p className="text-gray-600">Identified compliance gaps that the AI will address:</p>
              <div className="space-y-2 max-h-[320px] overflow-y-auto">
                {(gapsData as any)?.gaps?.map?.((gap: any, idx: number) => (
                  <div key={idx} className="flex items-center justify-between p-3 rounded border border-gray-200">
                    <div>
                      <p className="text-sm font-medium">{gap.control_ref ?? gap.control_name ?? `Gap ${idx + 1}`}</p>
                      <p className="text-xs text-gray-500">{gap.description ?? 'No description'}</p>
                    </div>
                    <span className={`text-xs px-2 py-1 rounded-full font-medium ${
                      gap.severity === 'critical' ? 'bg-red-100 text-red-700'
                        : gap.severity === 'high' ? 'bg-orange-100 text-orange-700'
                        : 'bg-amber-100 text-amber-700'
                    }`}>
                      {gap.severity ?? 'medium'}
                    </span>
                  </div>
                )) ?? (
                  <p className="text-gray-400 text-sm">Loading gaps...</p>
                )}
              </div>
              <div className="flex justify-between">
                <button
                  onClick={() => setWizardStep('frameworks')}
                  className="px-4 py-2 text-sm font-medium rounded border border-gray-300 text-gray-700 hover:bg-gray-50"
                >
                  Back
                </button>
                <button
                  onClick={handleGeneratePlan}
                  className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
                >
                  Generate AI Plan
                </button>
              </div>
            </div>
          )}

          {wizardStep === 'generating' && (
            <div className="flex flex-col items-center justify-center h-80 space-y-4">
              <div className="w-12 h-12 border-4 border-blue-200 border-t-blue-600 rounded-full animate-spin" />
              <p className="text-lg font-medium text-gray-700">AI is generating your remediation plan...</p>
              <p className="text-sm text-gray-500">Analyzing gaps and creating actionable steps</p>
            </div>
          )}

          {wizardStep === 'review' && generatedPlan && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-lg font-semibold">{generatedPlan.name ?? 'AI-Generated Remediation Plan'}</h3>
                  <p className="text-sm text-gray-500">{generatedPlan.description}</p>
                </div>
                <span className="text-xs bg-purple-100 text-purple-700 px-2 py-1 rounded-full font-medium">AI Generated</span>
              </div>
              <div className="space-y-2 max-h-[280px] overflow-y-auto">
                {(generatedPlan.actions ?? []).map((action: any, idx: number) => (
                  <div key={idx} className="p-3 rounded border border-gray-200">
                    <div className="flex items-start justify-between">
                      <div>
                        <p className="text-sm font-medium">{action.title}</p>
                        <p className="text-xs text-gray-500 mt-1">{action.description}</p>
                      </div>
                      <span className={`text-xs px-2 py-1 rounded font-medium ${
                        action.priority === 'critical' ? 'bg-red-100 text-red-700'
                          : action.priority === 'high' ? 'bg-orange-100 text-orange-700'
                          : action.priority === 'medium' ? 'bg-amber-100 text-amber-700'
                          : 'bg-gray-100 text-gray-600'
                      }`}>
                        {action.priority}
                      </span>
                    </div>
                    {action.effort_estimate && (
                      <p className="text-xs text-gray-400 mt-1">Effort: {action.effort_estimate}</p>
                    )}
                  </div>
                ))}
              </div>
              <div className="flex justify-between">
                <button
                  onClick={() => setWizardStep('gaps')}
                  className="px-4 py-2 text-sm font-medium rounded border border-gray-300 text-gray-700 hover:bg-gray-50"
                >
                  Back
                </button>
                <button
                  onClick={() => approveMutation.mutate(generatedPlan.id)}
                  disabled={approveMutation.isPending}
                  className="px-4 py-2 text-sm font-medium rounded bg-green-600 text-white hover:bg-green-700 disabled:opacity-50"
                >
                  {approveMutation.isPending ? 'Approving...' : 'Approve & Activate Plan'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Plan Detail with Kanban
  // ---------------------------------------------------------------------------

  if (selectedPlanId && planDetail) {
    const detail: RemediationPlan = planDetail;
    const progress = planProgress ?? {};

    return (
      <div className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <button
              onClick={() => setSelectedPlanId(null)}
              className="text-sm text-blue-600 hover:text-blue-800"
            >
              &larr; Back to Plans
            </button>
            <h1 className="text-2xl font-bold">{detail.name}</h1>
            <StatusBadge status={detail.status} />
          </div>
        </div>

        {/* Progress Dashboard */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <SummaryCard label="Total Actions" value={detail.total_actions ?? actions.length} color="blue" />
          <SummaryCard label="Completed" value={detail.completed_actions ?? 0} color="green" />
          <SummaryCard
            label="Progress"
            value={`${detail.progress ?? 0}%`}
            color="purple"
          />
          <SummaryCard
            label="Target Date"
            value={detail.target_date ? new Date(detail.target_date).toLocaleDateString() : 'N/A'}
            color="gray"
            isText
          />
        </div>

        {/* Progress Bar */}
        <div className="bg-white border rounded-lg p-4">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm font-medium text-gray-700">Overall Progress</span>
            <span className="text-sm font-bold text-blue-600">{detail.progress ?? 0}%</span>
          </div>
          <div className="w-full bg-gray-200 rounded-full h-3">
            <div
              className="bg-blue-600 h-3 rounded-full transition-all duration-500"
              style={{ width: `${detail.progress ?? 0}%` }}
            />
          </div>
          {progress.by_priority && (
            <div className="grid grid-cols-4 gap-2 mt-3">
              {Object.entries(progress.by_priority as Record<string, number>).map(([priority, count]) => (
                <div key={priority} className="text-center">
                  <span className="text-xs text-gray-500 capitalize">{priority}</span>
                  <p className="text-lg font-bold">{count as number}</p>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Kanban Board */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          {kanbanColumns.map((col) => {
            const colActions = actions.filter((a) => a.status === col.key);
            return (
              <div
                key={col.key}
                className={`rounded-lg p-3 min-h-[300px] ${col.color}`}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => {
                  if (dragItem) handleDrop(dragItem, col.key);
                  setDragItem(null);
                }}
              >
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-sm font-semibold text-gray-700">{col.label}</h3>
                  <span className="text-xs bg-white rounded-full px-2 py-0.5 font-medium text-gray-600">
                    {colActions.length}
                  </span>
                </div>
                <div className="space-y-2">
                  {colActions.map((action) => (
                    <div
                      key={action.id}
                      draggable
                      onDragStart={() => setDragItem(action.id)}
                      className="bg-white rounded-lg p-3 border shadow-sm cursor-grab active:cursor-grabbing hover:shadow-md transition-shadow"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <p className="text-sm font-medium text-gray-900 flex-1">{action.title}</p>
                        <PriorityBadge priority={action.priority} />
                      </div>
                      {action.control_ref && (
                        <p className="text-xs text-gray-400 mt-1">{action.control_ref}</p>
                      )}
                      {action.assignee && (
                        <p className="text-xs text-gray-500 mt-1">Assigned: {action.assignee}</p>
                      )}
                      {action.due_date && (
                        <p className="text-xs text-gray-400 mt-1">Due: {new Date(action.due_date).toLocaleDateString()}</p>
                      )}
                      <div className="flex gap-1 mt-2">
                        <button
                          onClick={() => handleRequestGuidance(action)}
                          className="text-xs px-2 py-1 rounded bg-purple-50 text-purple-600 hover:bg-purple-100"
                        >
                          AI Guidance
                        </button>
                        {action.status !== 'done' && (
                          <button
                            onClick={() => completeActionMutation.mutate({ id: action.id, data: {} })}
                            className="text-xs px-2 py-1 rounded bg-green-50 text-green-600 hover:bg-green-100"
                          >
                            Complete
                          </button>
                        )}
                      </div>
                      {/* AI Guidance Panel */}
                      {activeGuidanceActionId === action.id && (
                        <div className="mt-2 p-2 rounded bg-purple-50 border border-purple-200">
                          <div className="flex items-center justify-between mb-1">
                            <span className="text-xs font-semibold text-purple-700">AI Guidance</span>
                            <button
                              onClick={() => setActiveGuidanceActionId(null)}
                              className="text-xs text-purple-400 hover:text-purple-600"
                            >
                              Close
                            </button>
                          </div>
                          {guidanceMutation.isPending ? (
                            <p className="text-xs text-purple-500">Generating guidance...</p>
                          ) : guidanceMutation.data ? (
                            <p className="text-xs text-purple-800 whitespace-pre-wrap">
                              {(guidanceMutation.data as any).guidance ?? JSON.stringify(guidanceMutation.data)}
                            </p>
                          ) : guidanceMutation.isError ? (
                            <p className="text-xs text-red-500">Failed to get guidance.</p>
                          ) : null}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Plan List
  // ---------------------------------------------------------------------------

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">AI Remediation Planner</h1>
        <button
          onClick={() => setShowWizard(true)}
          className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
        >
          Generate AI Plan
        </button>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <SummaryCard label="Total Plans" value={plans.length} color="blue" />
        <SummaryCard label="Active Plans" value={activePlans} color="amber" />
        <SummaryCard label="Completed" value={completedPlans} color="green" />
        <SummaryCard label="Avg. Progress" value={`${avgProgress}%`} color="purple" />
      </div>

      {/* Plan List */}
      {plans.length === 0 && (
        <div className="text-center py-16 text-gray-500">
          <p className="text-lg font-medium">No remediation plans yet</p>
          <p className="text-sm mt-1">Click &quot;Generate AI Plan&quot; to create your first plan</p>
        </div>
      )}

      <div className="space-y-3">
        {plans.map((plan) => (
          <div
            key={plan.id}
            onClick={() => setSelectedPlanId(plan.id)}
            className="border rounded-lg p-4 bg-white shadow-sm hover:shadow-md transition-shadow cursor-pointer"
          >
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <p className="font-semibold text-gray-900">{plan.name}</p>
                  <StatusBadge status={plan.status} />
                </div>
                <p className="text-sm text-gray-500">{plan.description}</p>
                <p className="text-xs text-gray-400 mt-1">
                  Target: {plan.target_date ? new Date(plan.target_date).toLocaleDateString() : 'N/A'} &middot;{' '}
                  {plan.completed_actions ?? 0}/{plan.total_actions ?? 0} actions completed
                </p>
              </div>
              <div className="flex flex-col items-end gap-2 shrink-0">
                <div className="w-32">
                  <div className="flex items-center justify-between text-xs mb-1">
                    <span className="text-gray-500">Progress</span>
                    <span className="font-medium">{plan.progress ?? 0}%</span>
                  </div>
                  <div className="w-full bg-gray-200 rounded-full h-2">
                    <div
                      className="bg-blue-600 h-2 rounded-full transition-all"
                      style={{ width: `${plan.progress ?? 0}%` }}
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function SummaryCard({ label, value, color, isText }: { label: string; value: number | string; color: string; isText?: boolean }) {
  const colorMap: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 border-blue-200',
    amber: 'bg-amber-50 text-amber-700 border-amber-200',
    red: 'bg-red-50 text-red-700 border-red-200',
    green: 'bg-green-50 text-green-700 border-green-200',
    purple: 'bg-purple-50 text-purple-700 border-purple-200',
    gray: 'bg-gray-50 text-gray-700 border-gray-200',
  };

  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.blue}`}>
      <p className="text-sm font-medium opacity-80">{label}</p>
      <p className={`${isText ? 'text-lg' : 'text-3xl'} font-bold mt-1`}>{value}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    draft: 'bg-gray-100 text-gray-600',
    active: 'bg-blue-100 text-blue-700',
    completed: 'bg-green-100 text-green-700',
    archived: 'bg-gray-100 text-gray-400',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[status] ?? styles.draft}`}>
      {status}
    </span>
  );
}

function PriorityBadge({ priority }: { priority: string }) {
  const styles: Record<string, string> = {
    critical: 'bg-red-100 text-red-700',
    high: 'bg-orange-100 text-orange-700',
    medium: 'bg-amber-100 text-amber-700',
    low: 'bg-gray-100 text-gray-600',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded font-medium ${styles[priority] ?? styles.medium}`}>
      {priority}
    </span>
  );
}
