'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface VendorAssessment {
  id: string;
  vendor_id: string;
  vendor_name: string;
  questionnaire_id: string;
  questionnaire_name: string;
  status: 'sent' | 'in_progress' | 'submitted' | 'completed' | 'overdue';
  score?: number;
  max_score?: number;
  score_pct?: number;
  due_date: string;
  submitted_date?: string;
  completed_date?: string;
  contact_email: string;
  critical_findings: number;
  created_at: string;
}

interface AssessmentStats {
  sent: number;
  in_progress: number;
  submitted: number;
  completed: number;
  overdue: number;
  critical_findings_total: number;
  score_distribution: { range: string; count: number }[];
}

interface Questionnaire {
  id: string;
  name: string;
  description: string;
  question_count: number;
}

interface VendorOption {
  id: string;
  name: string;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function SummaryCard({ label, value, color }: { label: string; value: number; color: string }) {
  const colorMap: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 border-blue-200',
    amber: 'bg-amber-50 text-amber-700 border-amber-200',
    green: 'bg-green-50 text-green-700 border-green-200',
    red: 'bg-red-50 text-red-700 border-red-200',
    purple: 'bg-purple-50 text-purple-700 border-purple-200',
  };
  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.blue}`}>
      <p className="text-xs font-medium uppercase tracking-wide opacity-70">{label}</p>
      <p className="mt-1 text-2xl font-bold">{value}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    sent: 'bg-blue-100 text-blue-700',
    in_progress: 'bg-amber-100 text-amber-700',
    submitted: 'bg-purple-100 text-purple-700',
    completed: 'bg-green-100 text-green-700',
    overdue: 'bg-red-100 text-red-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[status] ?? 'bg-gray-100 text-gray-700'}`}>
      {status.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

function ScoreBar({ pct }: { pct: number | undefined }) {
  if (pct === undefined) return <span className="text-xs text-gray-400">--</span>;
  return (
    <div className="flex items-center gap-2">
      <div className="w-20 h-2 bg-gray-200 rounded-full">
        <div
          className={`h-2 rounded-full ${pct >= 80 ? 'bg-green-500' : pct >= 60 ? 'bg-amber-500' : 'bg-red-500'}`}
          style={{ width: `${Math.min(pct, 100)}%` }}
        />
      </div>
      <span className="text-xs text-gray-600">{pct}%</span>
    </div>
  );
}

function ScoreDistributionChart({ data }: { data: { range: string; count: number }[] }) {
  const max = Math.max(...data.map((d) => d.count), 1);
  return (
    <div className="space-y-2">
      {data.map((d) => (
        <div key={d.range} className="flex items-center gap-2 text-sm">
          <span className="w-20 text-xs text-gray-500 text-right">{d.range}</span>
          <div className="flex-1 h-5 bg-gray-100 rounded">
            <div
              className="h-5 bg-blue-500 rounded text-xs text-white flex items-center justify-end pr-1"
              style={{ width: `${(d.count / max) * 100}%`, minWidth: d.count > 0 ? '20px' : '0' }}
            >
              {d.count > 0 ? d.count : ''}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Send Assessment Form
// ---------------------------------------------------------------------------

function SendAssessmentForm({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({
    vendor_id: '',
    questionnaire_id: '',
    due_date: '',
    contact_email: '',
  });

  const { data: vendorsData } = useQuery({
    queryKey: ['vendors-options'],
    queryFn: () => api.vendors.list({ page_size: 200 }),
  });

  const { data: questionnairesData } = useQuery({
    queryKey: ['questionnaires-options'],
    queryFn: () => api.questionnaires.list(),
  });

  const vendorsList: any[] = Array.isArray(vendorsData) ? vendorsData : (vendorsData as any)?.items ?? [];
  const vendors: VendorOption[] = vendorsList.map((v: any) => ({
    id: v.id,
    name: v.name ?? v.company_name ?? v.id,
  }));

  const questionnaires: Questionnaire[] = questionnairesData?.items ?? questionnairesData ?? [];

  const sendMutation = useMutation({
    mutationFn: (data: typeof form) => api.vendorAssessments.send(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['vendor-assessments'] });
      onClose();
    },
  });

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md">
        <div className="border-b p-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">Send Assessment</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl">&times;</button>
        </div>
        <div className="p-4 space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Vendor</label>
            <select
              value={form.vendor_id}
              onChange={(e) => setForm({ ...form, vendor_id: e.target.value })}
              className="w-full border rounded px-3 py-2 text-sm"
            >
              <option value="">Select vendor...</option>
              {vendors.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Questionnaire</label>
            <select
              value={form.questionnaire_id}
              onChange={(e) => setForm({ ...form, questionnaire_id: e.target.value })}
              className="w-full border rounded px-3 py-2 text-sm"
            >
              <option value="">Select questionnaire...</option>
              {questionnaires.map((q) => (
                <option key={q.id} value={q.id}>
                  {q.name} ({q.question_count} questions)
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Due Date</label>
            <input
              type="date"
              value={form.due_date}
              onChange={(e) => setForm({ ...form, due_date: e.target.value })}
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Contact Email</label>
            <input
              type="email"
              value={form.contact_email}
              onChange={(e) => setForm({ ...form, contact_email: e.target.value })}
              className="w-full border rounded px-3 py-2 text-sm"
              placeholder="vendor-contact@company.com"
            />
          </div>
        </div>
        <div className="border-t p-4 flex justify-end gap-3">
          <button onClick={onClose} className="px-4 py-2 text-sm font-medium rounded border hover:bg-gray-50">
            Cancel
          </button>
          <button
            onClick={() => sendMutation.mutate(form)}
            disabled={sendMutation.isPending || !form.vendor_id || !form.questionnaire_id}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {sendMutation.isPending ? 'Sending...' : 'Send Assessment'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Compare Modal
// ---------------------------------------------------------------------------

function VendorCompareModal({ assessments, onClose }: { assessments: VendorAssessment[]; onClose: () => void }) {
  const completed = assessments.filter((a) => a.status === 'completed' && a.score_pct !== undefined);
  const sorted = [...completed].sort((a, b) => (b.score_pct ?? 0) - (a.score_pct ?? 0));

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-lg max-h-[80vh] overflow-auto">
        <div className="border-b p-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">Vendor Comparison</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl">&times;</button>
        </div>
        <div className="p-4 space-y-3">
          {sorted.length === 0 ? (
            <p className="text-sm text-gray-500 text-center py-8">No completed assessments to compare</p>
          ) : (
            sorted.map((a) => (
              <div key={a.id} className="flex items-center gap-3 border-b pb-2 last:border-0">
                <div className="flex-1">
                  <p className="text-sm font-medium">{a.vendor_name}</p>
                  <p className="text-xs text-gray-500">{a.questionnaire_name}</p>
                </div>
                <ScoreBar pct={a.score_pct} />
                {a.critical_findings > 0 && (
                  <span className="text-xs bg-red-100 text-red-700 px-2 py-0.5 rounded-full">
                    {a.critical_findings} critical
                  </span>
                )}
              </div>
            ))
          )}
        </div>
        <div className="border-t p-4 flex justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm font-medium rounded border hover:bg-gray-50">
            Close
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function VendorAssessmentsPage() {
  const [showSendForm, setShowSendForm] = useState(false);
  const [showCompare, setShowCompare] = useState(false);

  const { data: statsData } = useQuery<AssessmentStats>({
    queryKey: ['vendor-assessment-stats'],
    queryFn: () => api.vendorAssessments.stats(),
  });

  const { data: assessmentsData, isLoading } = useQuery({
    queryKey: ['vendor-assessments'],
    queryFn: () => api.vendorAssessments.list(),
  });

  const assessments: VendorAssessment[] = assessmentsData?.items ?? assessmentsData ?? [];

  const stats = statsData ?? {
    sent: assessments.filter((a) => a.status === 'sent').length,
    in_progress: assessments.filter((a) => a.status === 'in_progress').length,
    submitted: assessments.filter((a) => a.status === 'submitted').length,
    completed: assessments.filter((a) => a.status === 'completed').length,
    overdue: assessments.filter((a) => a.status === 'overdue').length,
    critical_findings_total: assessments.reduce((s, a) => s + a.critical_findings, 0),
    score_distribution: [],
  };

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Third-Party Risk Assessments</h1>
        <div className="flex gap-2">
          <button
            onClick={() => setShowCompare(true)}
            className="px-4 py-2 text-sm font-medium rounded border hover:bg-gray-50"
          >
            Compare Vendors
          </button>
          <button
            onClick={() => setShowSendForm(true)}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
          >
            Send Assessment
          </button>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <SummaryCard label="Sent" value={stats.sent} color="blue" />
        <SummaryCard label="In Progress" value={stats.in_progress} color="amber" />
        <SummaryCard label="Submitted" value={stats.submitted} color="purple" />
        <SummaryCard label="Completed" value={stats.completed} color="green" />
        <SummaryCard label="Overdue" value={stats.overdue} color="red" />
      </div>

      {/* Score Distribution & Critical Alert */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {stats.score_distribution.length > 0 && (
          <div className="border rounded-lg p-4 bg-white">
            <h2 className="text-sm font-semibold text-gray-700 mb-3">Score Distribution</h2>
            <ScoreDistributionChart data={stats.score_distribution} />
          </div>
        )}
        {stats.critical_findings_total > 0 && (
          <div className="border border-red-200 rounded-lg p-4 bg-red-50">
            <h2 className="text-sm font-semibold text-red-700 mb-2">Critical Findings Alert</h2>
            <p className="text-2xl font-bold text-red-700">{stats.critical_findings_total}</p>
            <p className="text-xs text-red-600 mt-1">
              Critical findings across all vendor assessments requiring immediate attention
            </p>
          </div>
        )}
      </div>

      {/* Assessment List */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-16 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      ) : assessments.length === 0 ? (
        <div className="text-center py-16 text-gray-500">
          <p className="text-lg font-medium">No assessments yet</p>
          <p className="text-sm mt-1">Send an assessment to a vendor to get started</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-3 font-semibold text-gray-700">Vendor</th>
                <th className="pb-3 font-semibold text-gray-700">Questionnaire</th>
                <th className="pb-3 font-semibold text-gray-700">Status</th>
                <th className="pb-3 font-semibold text-gray-700">Score</th>
                <th className="pb-3 font-semibold text-gray-700">Due Date</th>
                <th className="pb-3 font-semibold text-gray-700">Critical</th>
              </tr>
            </thead>
            <tbody>
              {assessments.map((a) => (
                <tr key={a.id} className="border-b hover:bg-gray-50">
                  <td className="py-3">
                    <p className="font-medium text-gray-900">{a.vendor_name}</p>
                    <p className="text-xs text-gray-500">{a.contact_email}</p>
                  </td>
                  <td className="py-3 text-gray-600">{a.questionnaire_name}</td>
                  <td className="py-3">
                    <StatusBadge status={a.status} />
                  </td>
                  <td className="py-3">
                    <ScoreBar pct={a.score_pct} />
                  </td>
                  <td className="py-3">
                    <span
                      className={`text-sm ${
                        new Date(a.due_date) < new Date() && a.status !== 'completed'
                          ? 'text-red-600 font-semibold'
                          : 'text-gray-600'
                      }`}
                    >
                      {new Date(a.due_date).toLocaleDateString()}
                    </span>
                  </td>
                  <td className="py-3">
                    {a.critical_findings > 0 ? (
                      <span className="text-xs bg-red-100 text-red-700 px-2 py-0.5 rounded-full font-medium">
                        {a.critical_findings}
                      </span>
                    ) : (
                      <span className="text-xs text-gray-400">0</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showSendForm && <SendAssessmentForm onClose={() => setShowSendForm(false)} />}
      {showCompare && <VendorCompareModal assessments={assessments} onClose={() => setShowCompare(false)} />}
    </div>
  );
}
