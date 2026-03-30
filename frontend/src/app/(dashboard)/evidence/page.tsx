'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface EvidenceTemplate {
  id: string;
  name: string;
  description: string;
  framework: string;
  category: string;
  difficulty: 'easy' | 'medium' | 'hard';
  priority: 'critical' | 'high' | 'medium' | 'low';
  format: string;
  example_content?: string;
  tags: string[];
}

interface EvidenceRequirement {
  id: string;
  control_id: string;
  control_ref: string;
  control_name: string;
  template_id?: string;
  template_name?: string;
  status: 'not_started' | 'in_progress' | 'collected' | 'validated' | 'expired';
  assigned_to?: string;
  due_date?: string;
  collected_date?: string;
  validated_date?: string;
  file_count: number;
}

interface TestSuite {
  id: string;
  name: string;
  description: string;
  framework: string;
  test_count: number;
  last_run?: string;
  last_result?: 'pass' | 'partial' | 'fail';
  pass_rate?: number;
}

interface ReadinessResult {
  control_ref: string;
  control_name: string;
  result: 'pass' | 'fail' | 'warning';
  message: string;
  evidence_status: string;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function DifficultyBadge({ difficulty }: { difficulty: string }) {
  const map: Record<string, string> = {
    easy: 'bg-green-100 text-green-700',
    medium: 'bg-amber-100 text-amber-700',
    hard: 'bg-red-100 text-red-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[difficulty] ?? 'bg-gray-100 text-gray-700'}`}>
      {difficulty.charAt(0).toUpperCase() + difficulty.slice(1)}
    </span>
  );
}

function PriorityBadge({ priority }: { priority: string }) {
  const map: Record<string, string> = {
    critical: 'bg-red-600 text-white',
    high: 'bg-red-100 text-red-700',
    medium: 'bg-amber-100 text-amber-700',
    low: 'bg-green-100 text-green-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[priority] ?? 'bg-gray-100 text-gray-700'}`}>
      {priority.charAt(0).toUpperCase() + priority.slice(1)}
    </span>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    not_started: 'bg-gray-100 text-gray-700',
    in_progress: 'bg-blue-100 text-blue-700',
    collected: 'bg-amber-100 text-amber-700',
    validated: 'bg-green-100 text-green-700',
    expired: 'bg-red-100 text-red-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[status] ?? 'bg-gray-100 text-gray-700'}`}>
      {status.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

function SummaryCard({ label, value, color, isText }: { label: string; value: number | string; color: string; isText?: boolean }) {
  const colorMap: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 border-blue-200',
    green: 'bg-green-50 text-green-700 border-green-200',
    amber: 'bg-amber-50 text-amber-700 border-amber-200',
    red: 'bg-red-50 text-red-700 border-red-200',
    purple: 'bg-purple-50 text-purple-700 border-purple-200',
  };
  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.blue}`}>
      <p className="text-xs font-medium uppercase tracking-wide opacity-70">{label}</p>
      <p className={`mt-1 font-bold ${isText ? 'text-lg' : 'text-2xl'}`}>{value}</p>
    </div>
  );
}

function ResultBadge({ result }: { result: string }) {
  const map: Record<string, string> = {
    pass: 'bg-green-100 text-green-700',
    partial: 'bg-amber-100 text-amber-700',
    fail: 'bg-red-100 text-red-700',
    warning: 'bg-amber-100 text-amber-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[result] ?? 'bg-gray-100 text-gray-700'}`}>
      {result.charAt(0).toUpperCase() + result.slice(1)}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Kanban Column
// ---------------------------------------------------------------------------

function KanbanColumn({
  title,
  items,
  color,
}: {
  title: string;
  items: EvidenceRequirement[];
  color: string;
}) {
  const colorMap: Record<string, string> = {
    gray: 'border-t-gray-400',
    blue: 'border-t-blue-500',
    amber: 'border-t-amber-500',
    green: 'border-t-green-500',
  };
  return (
    <div className={`bg-gray-50 rounded-lg border border-t-4 ${colorMap[color]} min-w-[260px] flex-1`}>
      <div className="p-3 border-b">
        <h3 className="text-sm font-semibold text-gray-700">
          {title}{' '}
          <span className="text-xs font-normal text-gray-400">({items.length})</span>
        </h3>
      </div>
      <div className="p-2 space-y-2 max-h-[500px] overflow-auto">
        {items.map((item) => (
          <div key={item.id} className="bg-white rounded border p-3 text-sm shadow-sm hover:shadow">
            <p className="font-medium text-gray-900 text-xs">{item.control_ref}</p>
            <p className="text-gray-600 text-xs mt-0.5 line-clamp-2">{item.control_name}</p>
            {item.assigned_to && (
              <p className="text-xs text-gray-400 mt-1">Assigned: {item.assigned_to}</p>
            )}
            {item.due_date && (
              <p className="text-xs text-gray-400">Due: {new Date(item.due_date).toLocaleDateString()}</p>
            )}
          </div>
        ))}
        {items.length === 0 && (
          <p className="text-xs text-gray-400 text-center py-4">No items</p>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function EvidenceTemplateTestingPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'templates' | 'requirements' | 'testing'>('templates');
  const [templateSearch, setTemplateSearch] = useState('');
  const [frameworkFilter, setFrameworkFilter] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('');
  const [difficultyFilter, setDifficultyFilter] = useState('');
  const [priorityFilter, setPriorityFilter] = useState('');

  // Templates
  const { data: templatesData, isLoading: templatesLoading } = useQuery({
    queryKey: ['evidence-templates', templateSearch, frameworkFilter, categoryFilter, difficultyFilter, priorityFilter],
    queryFn: () =>
      api.evidence.listTemplates({
        search: templateSearch || undefined,
        framework: frameworkFilter || undefined,
        category: categoryFilter || undefined,
        difficulty: difficultyFilter || undefined,
        priority: priorityFilter || undefined,
      }),
    enabled: activeTab === 'templates',
  });

  // Requirements
  const { data: requirementsData } = useQuery({
    queryKey: ['evidence-requirements'],
    queryFn: () => api.evidence.listRequirements(),
    enabled: activeTab === 'requirements',
  });

  // Test Suites
  const { data: testSuitesData } = useQuery({
    queryKey: ['evidence-test-suites'],
    queryFn: () => api.evidence.listTestSuites(),
    enabled: activeTab === 'testing',
  });

  // Pre-audit check
  const preAuditMutation = useMutation({
    mutationFn: () => api.evidence.runPreAuditCheck(),
  });

  const templates: EvidenceTemplate[] = templatesData?.items ?? templatesData ?? [];
  const requirements: EvidenceRequirement[] = requirementsData?.items ?? requirementsData ?? [];
  const testSuites: TestSuite[] = testSuitesData?.items ?? testSuitesData ?? [];
  const readinessResults: ReadinessResult[] = preAuditMutation.data?.results ?? [];

  // Requirements summary
  const totalReqs = requirements.length;
  const collectedPct = totalReqs > 0 ? Math.round((requirements.filter((r) => ['collected', 'validated'].includes(r.status)).length / totalReqs) * 100) : 0;
  const validatedPct = totalReqs > 0 ? Math.round((requirements.filter((r) => r.status === 'validated').length / totalReqs) * 100) : 0;
  const expiredPct = totalReqs > 0 ? Math.round((requirements.filter((r) => r.status === 'expired').length / totalReqs) * 100) : 0;
  const gapCount = requirements.filter((r) => r.status === 'not_started').length;

  // Kanban columns
  const notStarted = requirements.filter((r) => r.status === 'not_started');
  const inProgress = requirements.filter((r) => r.status === 'in_progress');
  const collected = requirements.filter((r) => r.status === 'collected');
  const validated = requirements.filter((r) => r.status === 'validated');

  const tabs = [
    { key: 'templates' as const, label: 'Templates' },
    { key: 'requirements' as const, label: 'Requirements' },
    { key: 'testing' as const, label: 'Testing' },
  ];

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Evidence Management</h1>

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

      {/* Templates Tab */}
      {activeTab === 'templates' && (
        <div className="space-y-4">
          {/* Search & Filters */}
          <div className="flex flex-wrap gap-3">
            <input
              value={templateSearch}
              onChange={(e) => setTemplateSearch(e.target.value)}
              className="border rounded px-3 py-2 text-sm flex-1 min-w-[200px]"
              placeholder="Search templates..."
            />
            <select value={frameworkFilter} onChange={(e) => setFrameworkFilter(e.target.value)} className="border rounded px-3 py-2 text-sm">
              <option value="">All Frameworks</option>
              <option value="ISO27001">ISO 27001</option>
              <option value="SOC2">SOC 2</option>
              <option value="GDPR">GDPR</option>
              <option value="PCI_DSS">PCI DSS</option>
            </select>
            <select value={categoryFilter} onChange={(e) => setCategoryFilter(e.target.value)} className="border rounded px-3 py-2 text-sm">
              <option value="">All Categories</option>
              <option value="technical">Technical</option>
              <option value="administrative">Administrative</option>
              <option value="physical">Physical</option>
              <option value="operational">Operational</option>
            </select>
            <select value={difficultyFilter} onChange={(e) => setDifficultyFilter(e.target.value)} className="border rounded px-3 py-2 text-sm">
              <option value="">All Difficulties</option>
              <option value="easy">Easy</option>
              <option value="medium">Medium</option>
              <option value="hard">Hard</option>
            </select>
            <select value={priorityFilter} onChange={(e) => setPriorityFilter(e.target.value)} className="border rounded px-3 py-2 text-sm">
              <option value="">All Priorities</option>
              <option value="critical">Critical</option>
              <option value="high">High</option>
              <option value="medium">Medium</option>
              <option value="low">Low</option>
            </select>
          </div>

          {/* Template Cards */}
          {templatesLoading ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="h-40 rounded-lg bg-gray-100 animate-pulse" />
              ))}
            </div>
          ) : templates.length === 0 ? (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No templates found</p>
              <p className="text-sm mt-1">Adjust your filters or search terms</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {templates.map((tpl) => (
                <div key={tpl.id} className="border rounded-lg p-4 hover:shadow-md transition-shadow bg-white">
                  <div className="flex items-start justify-between">
                    <h3 className="font-medium text-gray-900 text-sm">{tpl.name}</h3>
                    <PriorityBadge priority={tpl.priority} />
                  </div>
                  <p className="text-xs text-gray-500 mt-1 line-clamp-2">{tpl.description}</p>
                  <div className="flex flex-wrap gap-1.5 mt-3">
                    <span className="text-xs bg-blue-50 text-blue-700 px-2 py-0.5 rounded">{tpl.framework}</span>
                    <span className="text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded">{tpl.category}</span>
                    <DifficultyBadge difficulty={tpl.difficulty} />
                  </div>
                  <div className="flex flex-wrap gap-1 mt-2">
                    {tpl.tags.slice(0, 3).map((tag) => (
                      <span key={tag} className="text-xs text-gray-400">
                        #{tag}
                      </span>
                    ))}
                  </div>
                  <div className="mt-3 text-xs text-gray-400">Format: {tpl.format}</div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Requirements Tab */}
      {activeTab === 'requirements' && (
        <div className="space-y-6">
          {/* Summary */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
            <SummaryCard label="Total Requirements" value={totalReqs} color="blue" />
            <SummaryCard label="Collected" value={`${collectedPct}%`} color="green" isText />
            <SummaryCard label="Validated" value={`${validatedPct}%`} color="green" isText />
            <SummaryCard label="Expired" value={`${expiredPct}%`} color="red" isText />
            <SummaryCard label="Gaps" value={gapCount} color="amber" />
          </div>

          {/* Kanban Board */}
          <div className="overflow-x-auto">
            <div className="flex gap-4 min-w-[1000px]">
              <KanbanColumn title="Not Started" items={notStarted} color="gray" />
              <KanbanColumn title="In Progress" items={inProgress} color="blue" />
              <KanbanColumn title="Collected" items={collected} color="amber" />
              <KanbanColumn title="Validated" items={validated} color="green" />
            </div>
          </div>

          {/* Gap List */}
          {gapCount > 0 && (
            <div className="border rounded-lg p-4">
              <h3 className="text-sm font-semibold text-gray-700 mb-3">Evidence Gaps ({gapCount})</h3>
              <div className="space-y-2">
                {notStarted.slice(0, 20).map((gap) => (
                  <div key={gap.id} className="flex items-center justify-between border-b pb-2 last:border-0">
                    <div>
                      <span className="text-xs font-mono text-gray-500">{gap.control_ref}</span>
                      <span className="text-sm text-gray-700 ml-2">{gap.control_name}</span>
                    </div>
                    {gap.due_date && (
                      <span className="text-xs text-gray-400">
                        Due: {new Date(gap.due_date).toLocaleDateString()}
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Testing Tab */}
      {activeTab === 'testing' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-gray-700">Test Suites</h2>
            <button
              onClick={() => preAuditMutation.mutate()}
              disabled={preAuditMutation.isPending}
              className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {preAuditMutation.isPending ? 'Running...' : 'Run Pre-Audit Check'}
            </button>
          </div>

          {/* Test Suite List */}
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left">
                  <th className="pb-3 font-semibold text-gray-700">Suite Name</th>
                  <th className="pb-3 font-semibold text-gray-700">Framework</th>
                  <th className="pb-3 font-semibold text-gray-700">Tests</th>
                  <th className="pb-3 font-semibold text-gray-700">Last Run</th>
                  <th className="pb-3 font-semibold text-gray-700">Result</th>
                  <th className="pb-3 font-semibold text-gray-700">Pass Rate</th>
                </tr>
              </thead>
              <tbody>
                {testSuites.map((suite) => (
                  <tr key={suite.id} className="border-b hover:bg-gray-50">
                    <td className="py-3">
                      <p className="font-medium text-gray-900">{suite.name}</p>
                      <p className="text-xs text-gray-500">{suite.description}</p>
                    </td>
                    <td className="py-3 text-gray-600">{suite.framework}</td>
                    <td className="py-3 text-gray-600">{suite.test_count}</td>
                    <td className="py-3 text-gray-500 text-xs">
                      {suite.last_run ? new Date(suite.last_run).toLocaleDateString() : 'Never'}
                    </td>
                    <td className="py-3">
                      {suite.last_result ? <ResultBadge result={suite.last_result} /> : <span className="text-xs text-gray-400">--</span>}
                    </td>
                    <td className="py-3">
                      {suite.pass_rate !== undefined ? (
                        <div className="flex items-center gap-2">
                          <div className="w-20 h-2 bg-gray-200 rounded-full">
                            <div
                              className={`h-2 rounded-full ${
                                suite.pass_rate >= 80 ? 'bg-green-500' : suite.pass_rate >= 50 ? 'bg-amber-500' : 'bg-red-500'
                              }`}
                              style={{ width: `${suite.pass_rate}%` }}
                            />
                          </div>
                          <span className="text-xs text-gray-500">{suite.pass_rate}%</span>
                        </div>
                      ) : (
                        <span className="text-xs text-gray-400">--</span>
                      )}
                    </td>
                  </tr>
                ))}
                {testSuites.length === 0 && (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-gray-400">
                      No test suites configured
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Readiness Report */}
          {preAuditMutation.isSuccess && readinessResults.length > 0 && (
            <div className="border rounded-lg p-4">
              <h3 className="text-sm font-semibold text-gray-700 mb-3">Pre-Audit Readiness Report</h3>
              <div className="space-y-2">
                {readinessResults.map((r, i) => (
                  <div key={i} className="flex items-center gap-3 text-sm border-b pb-2 last:border-0">
                    <span
                      className={`w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold ${
                        r.result === 'pass'
                          ? 'bg-green-100 text-green-700'
                          : r.result === 'fail'
                          ? 'bg-red-100 text-red-700'
                          : 'bg-amber-100 text-amber-700'
                      }`}
                    >
                      {r.result === 'pass' ? '\u2713' : r.result === 'fail' ? '\u2717' : '!'}
                    </span>
                    <span className="font-mono text-xs text-gray-500 w-24">{r.control_ref}</span>
                    <span className="text-gray-700 flex-1">{r.control_name}</span>
                    <span className="text-xs text-gray-500">{r.message}</span>
                    <ResultBadge result={r.result} />
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
