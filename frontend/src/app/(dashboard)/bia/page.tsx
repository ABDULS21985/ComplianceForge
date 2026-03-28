'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BIAProcess {
  id: string;
  name: string;
  description: string;
  department: string;
  owner: string;
  criticality: 'critical' | 'high' | 'medium' | 'low';
  rto_hours: number;
  rpo_hours: number;
  mtpd_hours: number;
  last_bia_date?: string;
  dependencies?: string[];
  has_bia: boolean;
}

interface SinglePointOfFailure {
  id: string;
  process_name: string;
  process_id: string;
  component: string;
  description: string;
  risk_level: 'critical' | 'high' | 'medium' | 'low';
  mitigation_status: 'unmitigated' | 'partial' | 'mitigated';
}

interface BCScenario {
  id: string;
  name: string;
  description: string;
  type: string;
  likelihood: 'high' | 'medium' | 'low';
  impact: 'critical' | 'high' | 'medium' | 'low';
  affected_processes: number;
  has_plan: boolean;
}

interface ContinuityPlan {
  id: string;
  name: string;
  scenario_id: string;
  scenario_name: string;
  status: 'draft' | 'approved' | 'active' | 'archived';
  last_tested?: string;
  owner: string;
}

interface BCExercise {
  id: string;
  name: string;
  type: 'tabletop' | 'walkthrough' | 'simulation' | 'full_scale';
  plan_id?: string;
  plan_name?: string;
  scheduled_date: string;
  status: 'scheduled' | 'in_progress' | 'completed' | 'cancelled';
  participants?: number;
  result?: 'pass' | 'partial' | 'fail';
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function BusinessImpactAnalysisPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'processes' | 'spof' | 'scenarios' | 'plans' | 'exercises'>('processes');
  const [criticalityFilter, setCriticalityFilter] = useState('');

  // Fetch processes
  const { data: processesData, isLoading: processesLoading } = useQuery({
    queryKey: ['bia-processes', criticalityFilter],
    queryFn: () => api.bia.listProcesses({ criticality: criticalityFilter || undefined }),
  });

  // Fetch SPoFs
  const { data: spofData } = useQuery({
    queryKey: ['bia-spof'],
    queryFn: () => api.bia.singlePointsOfFailure(),
    enabled: activeTab === 'spof' || activeTab === 'processes',
  });

  // Fetch scenarios
  const { data: scenariosData } = useQuery({
    queryKey: ['bc-scenarios'],
    queryFn: () => api.bia.listScenarios(),
    enabled: activeTab === 'scenarios',
  });

  // Fetch plans
  const { data: plansData } = useQuery({
    queryKey: ['bc-plans'],
    queryFn: () => api.bia.listPlans(),
    enabled: activeTab === 'plans',
  });

  // Fetch exercises
  const { data: exercisesData } = useQuery({
    queryKey: ['bc-exercises'],
    queryFn: () => api.bia.listExercises(),
    enabled: activeTab === 'exercises',
  });

  // Generate report mutation
  const reportMutation = useMutation({
    mutationFn: () => api.bia.report(),
  });

  const processes: BIAProcess[] = processesData?.items ?? processesData ?? [];
  const spofs: SinglePointOfFailure[] = spofData?.items ?? spofData ?? [];
  const scenarios: BCScenario[] = scenariosData?.items ?? scenariosData ?? [];
  const plans: ContinuityPlan[] = plansData?.items ?? plansData ?? [];
  const exercises: BCExercise[] = exercisesData?.items ?? exercisesData ?? [];

  // Summary stats
  const criticalProcesses = processes.filter((p) => p.criticality === 'critical').length;
  const processesWithoutBIA = processes.filter((p) => !p.has_bia).length;
  const spofCount = spofs.filter((s) => s.mitigation_status !== 'mitigated').length;
  const lastExercise = exercises
    .filter((e) => e.status === 'completed')
    .sort((a, b) => new Date(b.scheduled_date).getTime() - new Date(a.scheduled_date).getTime())[0];

  const tabs = [
    { key: 'processes' as const, label: 'Processes' },
    { key: 'spof' as const, label: `SPoF Alerts (${spofCount})` },
    { key: 'scenarios' as const, label: 'Scenarios' },
    { key: 'plans' as const, label: 'Continuity Plans' },
    { key: 'exercises' as const, label: 'Exercises' },
  ];

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Business Impact Analysis</h1>
        <button
          onClick={() => reportMutation.mutate()}
          disabled={reportMutation.isPending}
          className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {reportMutation.isPending ? 'Generating...' : 'Generate BIA Report'}
        </button>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <SummaryCard label="Critical Processes" value={criticalProcesses} color="red" />
        <SummaryCard label="Without BIA" value={processesWithoutBIA} color="amber" />
        <SummaryCard label="Unmitigated SPoFs" value={spofCount} color="purple" />
        <SummaryCard
          label="Last Exercise"
          value={lastExercise ? new Date(lastExercise.scheduled_date).toLocaleDateString() : 'None'}
          color="blue"
          isText
        />
      </div>

      {/* Tabs */}
      <div className="border-b">
        <div className="flex gap-0">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
                activeTab === tab.key
                  ? 'border-blue-600 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab Content */}
      {activeTab === 'processes' && (
        <div className="space-y-4">
          <div className="flex gap-3">
            <select
              value={criticalityFilter}
              onChange={(e) => setCriticalityFilter(e.target.value)}
              className="border rounded px-3 py-2 text-sm"
            >
              <option value="">All Criticalities</option>
              <option value="critical">Critical</option>
              <option value="high">High</option>
              <option value="medium">Medium</option>
              <option value="low">Low</option>
            </select>
          </div>

          {processesLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="h-20 rounded-lg bg-gray-100 animate-pulse" />
              ))}
            </div>
          ) : processes.length === 0 ? (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No processes found</p>
              <p className="text-sm mt-1">Add business processes to begin impact analysis</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left">
                    <th className="pb-3 font-semibold text-gray-700">Process</th>
                    <th className="pb-3 font-semibold text-gray-700">Department</th>
                    <th className="pb-3 font-semibold text-gray-700">Criticality</th>
                    <th className="pb-3 font-semibold text-gray-700">RTO</th>
                    <th className="pb-3 font-semibold text-gray-700">RPO</th>
                    <th className="pb-3 font-semibold text-gray-700">BIA Status</th>
                    <th className="pb-3 font-semibold text-gray-700">Last BIA</th>
                  </tr>
                </thead>
                <tbody>
                  {processes.map((proc) => (
                    <tr key={proc.id} className="border-b hover:bg-gray-50">
                      <td className="py-3">
                        <p className="font-medium text-gray-900">{proc.name}</p>
                        <p className="text-xs text-gray-500">Owner: {proc.owner}</p>
                      </td>
                      <td className="py-3 text-gray-600">{proc.department}</td>
                      <td className="py-3">
                        <CriticalityBadge criticality={proc.criticality} />
                      </td>
                      <td className="py-3">
                        <RTOBadge hours={proc.rto_hours} />
                      </td>
                      <td className="py-3">
                        <RTOBadge hours={proc.rpo_hours} />
                      </td>
                      <td className="py-3">
                        {proc.has_bia ? (
                          <span className="text-xs px-2 py-0.5 rounded-full bg-green-100 text-green-700 font-medium">Complete</span>
                        ) : (
                          <span className="text-xs px-2 py-0.5 rounded-full bg-amber-100 text-amber-700 font-medium">Pending</span>
                        )}
                      </td>
                      <td className="py-3 text-xs text-gray-500">
                        {proc.last_bia_date ? new Date(proc.last_bia_date).toLocaleDateString() : '--'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {activeTab === 'spof' && (
        <div className="space-y-3">
          {spofs.length === 0 ? (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No single points of failure identified</p>
              <p className="text-sm mt-1">Run dependency mapping to detect SPoFs</p>
            </div>
          ) : (
            spofs.map((spof) => (
              <div
                key={spof.id}
                className={`border rounded-lg p-4 bg-white shadow-sm ${
                  spof.mitigation_status === 'unmitigated' ? 'border-l-4 border-l-red-500' : ''
                }`}
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1">
                    <div className="flex items-center gap-2 mb-1">
                      <CriticalityBadge criticality={spof.risk_level} />
                      <MitigationBadge status={spof.mitigation_status} />
                    </div>
                    <p className="font-semibold text-gray-900">{spof.component}</p>
                    <p className="text-sm text-gray-600 mt-0.5">{spof.description}</p>
                    <p className="text-xs text-gray-400 mt-1">Process: {spof.process_name}</p>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {activeTab === 'scenarios' && (
        <div className="space-y-3">
          {scenarios.length === 0 ? (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No scenarios defined</p>
              <p className="text-sm mt-1">Create disruption scenarios for continuity planning</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {scenarios.map((scenario) => (
                <div key={scenario.id} className="border rounded-lg p-4 bg-white shadow-sm">
                  <div className="flex items-start justify-between gap-2 mb-2">
                    <div>
                      <p className="font-semibold text-gray-900">{scenario.name}</p>
                      <p className="text-xs text-gray-500">{scenario.type}</p>
                    </div>
                    {scenario.has_plan ? (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-green-100 text-green-700 font-medium">Has Plan</span>
                    ) : (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-red-100 text-red-700 font-medium">No Plan</span>
                    )}
                  </div>
                  <p className="text-sm text-gray-600 line-clamp-2">{scenario.description}</p>
                  <div className="flex items-center gap-3 mt-3">
                    <span className="text-xs text-gray-400">
                      Likelihood: <span className="font-medium capitalize">{scenario.likelihood}</span>
                    </span>
                    <span className="text-xs text-gray-400">
                      Impact: <span className="font-medium capitalize">{scenario.impact}</span>
                    </span>
                    <span className="text-xs text-gray-400">
                      Affected: {scenario.affected_processes} processes
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {activeTab === 'plans' && (
        <div className="space-y-3">
          {plans.length === 0 ? (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No continuity plans yet</p>
              <p className="text-sm mt-1">Create plans from your disruption scenarios</p>
            </div>
          ) : (
            plans.map((plan) => (
              <div key={plan.id} className="border rounded-lg p-4 bg-white shadow-sm">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-2 mb-1">
                      <p className="font-semibold text-gray-900">{plan.name}</p>
                      <PlanStatusBadge status={plan.status} />
                    </div>
                    <p className="text-sm text-gray-500">Scenario: {plan.scenario_name}</p>
                    <p className="text-xs text-gray-400 mt-1">
                      Owner: {plan.owner}
                      {plan.last_tested && ` | Last tested: ${new Date(plan.last_tested).toLocaleDateString()}`}
                    </p>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {activeTab === 'exercises' && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold">Exercise Calendar</h3>
          {exercises.length === 0 ? (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No exercises scheduled</p>
              <p className="text-sm mt-1">Schedule exercises to validate your continuity plans</p>
            </div>
          ) : (
            <div className="space-y-3">
              {exercises
                .sort((a, b) => new Date(a.scheduled_date).getTime() - new Date(b.scheduled_date).getTime())
                .map((exercise) => (
                  <div key={exercise.id} className="border rounded-lg p-4 bg-white shadow-sm">
                    <div className="flex items-start justify-between gap-4">
                      <div>
                        <div className="flex items-center gap-2 mb-1">
                          <p className="font-semibold text-gray-900">{exercise.name}</p>
                          <ExerciseTypeBadge type={exercise.type} />
                          <ExerciseStatusBadge status={exercise.status} />
                        </div>
                        {exercise.plan_name && (
                          <p className="text-sm text-gray-500">Plan: {exercise.plan_name}</p>
                        )}
                        <p className="text-xs text-gray-400 mt-1">
                          Scheduled: {new Date(exercise.scheduled_date).toLocaleDateString()}
                          {exercise.participants && ` | ${exercise.participants} participants`}
                        </p>
                      </div>
                      {exercise.result && (
                        <span className={`text-xs px-2 py-1 rounded-full font-medium ${
                          exercise.result === 'pass' ? 'bg-green-100 text-green-700'
                            : exercise.result === 'partial' ? 'bg-amber-100 text-amber-700'
                            : 'bg-red-100 text-red-700'
                        }`}>
                          {exercise.result === 'pass' ? 'Passed' : exercise.result === 'partial' ? 'Partial' : 'Failed'}
                        </span>
                      )}
                    </div>
                  </div>
                ))}
            </div>
          )}
        </div>
      )}

      {/* Report success notification */}
      {reportMutation.isSuccess && (
        <div className="fixed bottom-6 right-6 bg-green-600 text-white px-4 py-3 rounded-lg shadow-lg text-sm font-medium">
          BIA Report generated successfully!
        </div>
      )}
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
  };
  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.blue}`}>
      <p className="text-sm font-medium opacity-80">{label}</p>
      <p className={`${isText ? 'text-lg' : 'text-3xl'} font-bold mt-1`}>{value}</p>
    </div>
  );
}

function CriticalityBadge({ criticality }: { criticality: string }) {
  const styles: Record<string, string> = {
    critical: 'bg-red-100 text-red-700',
    high: 'bg-orange-100 text-orange-700',
    medium: 'bg-amber-100 text-amber-700',
    low: 'bg-gray-100 text-gray-600',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[criticality] ?? styles.medium}`}>
      {criticality}
    </span>
  );
}

function RTOBadge({ hours }: { hours: number }) {
  const label = hours < 1 ? `${hours * 60}m` : hours < 24 ? `${hours}h` : `${Math.round(hours / 24)}d`;
  const color = hours <= 4 ? 'text-red-600 font-semibold' : hours <= 24 ? 'text-amber-600 font-medium' : 'text-gray-600';
  return <span className={`text-sm ${color}`}>{label}</span>;
}

function MitigationBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    unmitigated: 'bg-red-100 text-red-700',
    partial: 'bg-amber-100 text-amber-700',
    mitigated: 'bg-green-100 text-green-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[status] ?? styles.unmitigated}`}>
      {status}
    </span>
  );
}

function PlanStatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    draft: 'bg-gray-100 text-gray-600',
    approved: 'bg-blue-100 text-blue-700',
    active: 'bg-green-100 text-green-700',
    archived: 'bg-gray-100 text-gray-400',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[status] ?? styles.draft}`}>
      {status}
    </span>
  );
}

function ExerciseTypeBadge({ type }: { type: string }) {
  const styles: Record<string, string> = {
    tabletop: 'bg-blue-100 text-blue-700',
    walkthrough: 'bg-purple-100 text-purple-700',
    simulation: 'bg-cyan-100 text-cyan-700',
    full_scale: 'bg-orange-100 text-orange-700',
  };
  const labels: Record<string, string> = {
    tabletop: 'Tabletop',
    walkthrough: 'Walkthrough',
    simulation: 'Simulation',
    full_scale: 'Full Scale',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[type] ?? 'bg-gray-100 text-gray-600'}`}>
      {labels[type] ?? type}
    </span>
  );
}

function ExerciseStatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    scheduled: 'bg-blue-100 text-blue-700',
    in_progress: 'bg-amber-100 text-amber-700',
    completed: 'bg-green-100 text-green-700',
    cancelled: 'bg-gray-100 text-gray-400',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[status] ?? styles.scheduled}`}>
      {status.replace('_', ' ')}
    </span>
  );
}
