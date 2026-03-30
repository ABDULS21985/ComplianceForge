'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Exception {
  id: string;
  reference: string;
  title: string;
  description: string;
  type: 'policy' | 'control' | 'standard' | 'regulatory';
  status: 'draft' | 'pending_approval' | 'approved' | 'rejected' | 'expired' | 'revoked';
  risk_level: 'critical' | 'high' | 'medium' | 'low';
  affected_controls: string[];
  affected_control_count: number;
  framework_id?: string;
  framework_name?: string;
  justification: string;
  compensating_controls: boolean;
  compensating_description?: string;
  requested_by: string;
  approved_by?: string;
  expiry_date: string;
  last_reviewed?: string;
  review_frequency_days: number;
  created_at: string;
}

interface ExceptionKPI {
  active_count: number;
  expiring_soon: number;
  expired: number;
  overdue_reviews: number;
  avg_age_days: number;
  risk_distribution: { level: string; count: number }[];
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function KPICard({ label, value, color }: { label: string; value: number | string; color: string }) {
  const colorMap: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 border-blue-200',
    amber: 'bg-amber-50 text-amber-700 border-amber-200',
    red: 'bg-red-50 text-red-700 border-red-200',
    purple: 'bg-purple-50 text-purple-700 border-purple-200',
    gray: 'bg-gray-50 text-gray-700 border-gray-200',
  };
  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.gray}`}>
      <p className="text-xs font-medium uppercase tracking-wide opacity-70">{label}</p>
      <p className="mt-1 text-2xl font-bold">{value}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    draft: 'bg-gray-100 text-gray-700',
    pending_approval: 'bg-yellow-100 text-yellow-700',
    approved: 'bg-green-100 text-green-700',
    rejected: 'bg-red-100 text-red-700',
    expired: 'bg-red-100 text-red-700',
    revoked: 'bg-gray-100 text-gray-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[status] ?? 'bg-gray-100 text-gray-700'}`}>
      {status.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

function RiskBadge({ level }: { level: string }) {
  const map: Record<string, string> = {
    critical: 'bg-red-600 text-white',
    high: 'bg-red-100 text-red-700',
    medium: 'bg-amber-100 text-amber-700',
    low: 'bg-green-100 text-green-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[level] ?? 'bg-gray-100 text-gray-700'}`}>
      {level.charAt(0).toUpperCase() + level.slice(1)}
    </span>
  );
}

function TypeBadge({ type }: { type: string }) {
  const map: Record<string, string> = {
    policy: 'bg-blue-100 text-blue-700',
    control: 'bg-purple-100 text-purple-700',
    standard: 'bg-indigo-100 text-indigo-700',
    regulatory: 'bg-orange-100 text-orange-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[type] ?? 'bg-gray-100 text-gray-700'}`}>
      {type.charAt(0).toUpperCase() + type.slice(1)}
    </span>
  );
}

function DonutChart({ data }: { data: { level: string; count: number }[] }) {
  const total = data.reduce((s, d) => s + d.count, 0);
  if (total === 0) {
    return <div className="flex items-center justify-center h-40 text-sm text-gray-400">No data</div>;
  }
  const colors: Record<string, string> = {
    critical: '#DC2626',
    high: '#EA580C',
    medium: '#D97706',
    low: '#059669',
  };
  let cumulativePct = 0;
  const segments = data.map((d) => {
    const pct = (d.count / total) * 100;
    const start = cumulativePct;
    cumulativePct += pct;
    return { ...d, pct, start, color: colors[d.level] ?? '#6B7280' };
  });
  const gradientParts = segments.map((s) => `${s.color} ${s.start}% ${s.start + s.pct}%`).join(', ');

  return (
    <div className="flex items-center gap-4">
      <div
        className="w-32 h-32 rounded-full flex-shrink-0"
        style={{
          background: `conic-gradient(${gradientParts})`,
          WebkitMask: 'radial-gradient(farthest-side, transparent 55%, #000 56%)',
          mask: 'radial-gradient(farthest-side, transparent 55%, #000 56%)',
        }}
      />
      <div className="space-y-1.5">
        {segments.map((s) => (
          <div key={s.level} className="flex items-center gap-2 text-xs">
            <span className="w-3 h-3 rounded-full flex-shrink-0" style={{ background: s.color }} />
            <span className="capitalize">{s.level}</span>
            <span className="text-gray-500">{s.count}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Multi-Step Exception Request Form
// ---------------------------------------------------------------------------

function ExceptionRequestForm({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [step, setStep] = useState(0);
  const [form, setForm] = useState({
    title: '',
    type: 'control' as string,
    affected_controls: [] as string[],
    control_search: '',
    justification: '',
    risk_level: 'medium' as string,
    risk_description: '',
    compensating_controls: false,
    compensating_description: '',
    start_date: '',
    expiry_date: '',
    review_frequency_days: 90,
  });

  const createMutation = useMutation({
    mutationFn: (data: typeof form) => api.exceptions.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['exceptions'] });
      onClose();
    },
  });

  const steps = [
    'Select Controls',
    'Justification',
    'Risk Assessment',
    'Compensating Controls',
    'Validity Dates',
    'Review & Submit',
  ];

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[90vh] overflow-auto">
        <div className="border-b p-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">Request Exception</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl">&times;</button>
        </div>

        {/* Step Indicator */}
        <div className="px-4 pt-4">
          <div className="flex items-center gap-1 mb-4">
            {steps.map((s, i) => (
              <div key={s} className="flex items-center gap-1 flex-1">
                <div
                  className={`w-6 h-6 rounded-full flex items-center justify-center text-xs font-bold ${
                    i <= step ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-500'
                  }`}
                >
                  {i + 1}
                </div>
                <span className="text-xs text-gray-500 hidden lg:block truncate">{s}</span>
                {i < steps.length - 1 && <div className={`flex-1 h-0.5 ${i < step ? 'bg-blue-600' : 'bg-gray-200'}`} />}
              </div>
            ))}
          </div>
        </div>

        <div className="p-4 space-y-4">
          {/* Step 0: Select Controls */}
          {step === 0 && (
            <>
              <div>
                <label className="block text-sm font-medium mb-1">Exception Title</label>
                <input
                  value={form.title}
                  onChange={(e) => setForm({ ...form, title: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                  placeholder="Brief title for this exception"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Exception Type</label>
                <select
                  value={form.type}
                  onChange={(e) => setForm({ ...form, type: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                >
                  <option value="policy">Policy</option>
                  <option value="control">Control</option>
                  <option value="standard">Standard</option>
                  <option value="regulatory">Regulatory</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Affected Controls</label>
                <input
                  value={form.control_search}
                  onChange={(e) => setForm({ ...form, control_search: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                  placeholder="Search and select controls..."
                />
                <div className="mt-2 flex flex-wrap gap-1">
                  {form.affected_controls.map((c) => (
                    <span key={c} className="text-xs bg-blue-100 text-blue-700 px-2 py-0.5 rounded-full flex items-center gap-1">
                      {c}
                      <button
                        onClick={() => setForm({ ...form, affected_controls: form.affected_controls.filter((x) => x !== c) })}
                        className="hover:text-blue-900"
                      >
                        &times;
                      </button>
                    </span>
                  ))}
                </div>
              </div>
            </>
          )}

          {/* Step 1: Justification */}
          {step === 1 && (
            <div>
              <label className="block text-sm font-medium mb-1">Business Justification</label>
              <textarea
                value={form.justification}
                onChange={(e) => setForm({ ...form, justification: e.target.value })}
                rows={6}
                className="w-full border rounded px-3 py-2 text-sm"
                placeholder="Provide a detailed justification for why this exception is required..."
              />
            </div>
          )}

          {/* Step 2: Risk Assessment */}
          {step === 2 && (
            <>
              <div>
                <label className="block text-sm font-medium mb-1">Residual Risk Level</label>
                <select
                  value={form.risk_level}
                  onChange={(e) => setForm({ ...form, risk_level: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                >
                  <option value="critical">Critical</option>
                  <option value="high">High</option>
                  <option value="medium">Medium</option>
                  <option value="low">Low</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Risk Description</label>
                <textarea
                  value={form.risk_description}
                  onChange={(e) => setForm({ ...form, risk_description: e.target.value })}
                  rows={4}
                  className="w-full border rounded px-3 py-2 text-sm"
                  placeholder="Describe the risk impact of granting this exception..."
                />
              </div>
            </>
          )}

          {/* Step 3: Compensating Controls */}
          {step === 3 && (
            <>
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={form.compensating_controls}
                  onChange={(e) => setForm({ ...form, compensating_controls: e.target.checked })}
                  className="rounded"
                  id="comp-check"
                />
                <label htmlFor="comp-check" className="text-sm font-medium">
                  Compensating controls are in place
                </label>
              </div>
              {form.compensating_controls && (
                <div>
                  <label className="block text-sm font-medium mb-1">Describe Compensating Controls</label>
                  <textarea
                    value={form.compensating_description}
                    onChange={(e) => setForm({ ...form, compensating_description: e.target.value })}
                    rows={4}
                    className="w-full border rounded px-3 py-2 text-sm"
                    placeholder="Describe the compensating controls that mitigate the risk..."
                  />
                </div>
              )}
            </>
          )}

          {/* Step 4: Validity Dates */}
          {step === 4 && (
            <>
              <div>
                <label className="block text-sm font-medium mb-1">Start Date</label>
                <input
                  type="date"
                  value={form.start_date}
                  onChange={(e) => setForm({ ...form, start_date: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Expiry Date</label>
                <input
                  type="date"
                  value={form.expiry_date}
                  onChange={(e) => setForm({ ...form, expiry_date: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Review Frequency (days)</label>
                <input
                  type="number"
                  value={form.review_frequency_days}
                  onChange={(e) => setForm({ ...form, review_frequency_days: parseInt(e.target.value) || 90 })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
            </>
          )}

          {/* Step 5: Review & Submit */}
          {step === 5 && (
            <div className="space-y-3">
              <h3 className="font-semibold text-sm">Review Your Exception Request</h3>
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <span className="text-gray-500">Title:</span>
                  <p className="font-medium">{form.title || '--'}</p>
                </div>
                <div>
                  <span className="text-gray-500">Type:</span>
                  <p className="font-medium capitalize">{form.type}</p>
                </div>
                <div>
                  <span className="text-gray-500">Risk Level:</span>
                  <p className="font-medium capitalize">{form.risk_level}</p>
                </div>
                <div>
                  <span className="text-gray-500">Compensating:</span>
                  <p className="font-medium">{form.compensating_controls ? 'Yes' : 'No'}</p>
                </div>
                <div>
                  <span className="text-gray-500">Start:</span>
                  <p className="font-medium">{form.start_date || '--'}</p>
                </div>
                <div>
                  <span className="text-gray-500">Expiry:</span>
                  <p className="font-medium">{form.expiry_date || '--'}</p>
                </div>
                <div className="col-span-2">
                  <span className="text-gray-500">Affected Controls:</span>
                  <p className="font-medium">{form.affected_controls.length} selected</p>
                </div>
                <div className="col-span-2">
                  <span className="text-gray-500">Justification:</span>
                  <p className="font-medium">{form.justification || '--'}</p>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Navigation */}
        <div className="border-t p-4 flex items-center justify-between">
          <button
            onClick={() => (step === 0 ? onClose() : setStep(step - 1))}
            className="px-4 py-2 text-sm font-medium rounded border hover:bg-gray-50"
          >
            {step === 0 ? 'Cancel' : 'Back'}
          </button>
          {step < 5 ? (
            <button
              onClick={() => setStep(step + 1)}
              className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
            >
              Next
            </button>
          ) : (
            <button
              onClick={() => createMutation.mutate(form)}
              disabled={createMutation.isPending}
              className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {createMutation.isPending ? 'Submitting...' : 'Submit Request'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function ExceptionManagementPage() {
  const [statusFilter, setStatusFilter] = useState('');
  const [riskFilter, setRiskFilter] = useState('');
  const [frameworkFilter, setFrameworkFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [showForm, setShowForm] = useState(false);

  const { data: kpiData } = useQuery<ExceptionKPI>({
    queryKey: ['exceptions-kpi'],
    queryFn: () => api.exceptions.kpi(),
  });

  const { data: exceptionsData, isLoading } = useQuery({
    queryKey: ['exceptions', statusFilter, riskFilter, frameworkFilter, typeFilter],
    queryFn: () =>
      api.exceptions.list({
        status: statusFilter || undefined,
        risk_level: riskFilter || undefined,
        framework_id: frameworkFilter || undefined,
        type: typeFilter || undefined,
      }),
  });

  const exceptions: Exception[] = exceptionsData?.items ?? exceptionsData ?? [];

  const kpi = kpiData ?? {
    active_count: exceptions.filter((e) => e.status === 'approved').length,
    expiring_soon: exceptions.filter((e) => {
      const d = new Date(e.expiry_date);
      const now = new Date();
      const diff = (d.getTime() - now.getTime()) / (1000 * 60 * 60 * 24);
      return diff > 0 && diff <= 30 && e.status === 'approved';
    }).length,
    expired: exceptions.filter((e) => e.status === 'expired').length,
    overdue_reviews: exceptions.filter((e) => {
      if (!e.last_reviewed) return true;
      const next = new Date(e.last_reviewed);
      next.setDate(next.getDate() + e.review_frequency_days);
      return next < new Date();
    }).length,
    avg_age_days: 0,
    risk_distribution: [],
  };

  function isExpiringSoon(dateStr: string) {
    const d = new Date(dateStr);
    const now = new Date();
    const diff = (d.getTime() - now.getTime()) / (1000 * 60 * 60 * 24);
    return diff <= 30 && diff > 0;
  }

  function isExpired(dateStr: string) {
    return new Date(dateStr) < new Date();
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Exception Management</h1>
        <button
          onClick={() => setShowForm(true)}
          className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
        >
          Request Exception
        </button>
      </div>

      {/* KPI Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <KPICard label="Active Exceptions" value={kpi.active_count} color="blue" />
        <KPICard label="Expiring < 30 days" value={kpi.expiring_soon} color="amber" />
        <KPICard label="Expired" value={kpi.expired} color="red" />
        <KPICard label="Overdue Reviews" value={kpi.overdue_reviews} color="red" />
        <KPICard label="Avg Age (days)" value={kpi.avg_age_days} color="gray" />
      </div>

      {/* Risk Distribution Donut */}
      {kpi.risk_distribution.length > 0 && (
        <div className="bg-white border rounded-lg p-4">
          <h2 className="text-sm font-semibold text-gray-700 mb-3">Risk Distribution</h2>
          <DonutChart data={kpi.risk_distribution} />
        </div>
      )}

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="border rounded px-3 py-2 text-sm">
          <option value="">All Statuses</option>
          <option value="draft">Draft</option>
          <option value="pending_approval">Pending Approval</option>
          <option value="approved">Approved</option>
          <option value="rejected">Rejected</option>
          <option value="expired">Expired</option>
          <option value="revoked">Revoked</option>
        </select>
        <select value={riskFilter} onChange={(e) => setRiskFilter(e.target.value)} className="border rounded px-3 py-2 text-sm">
          <option value="">All Risk Levels</option>
          <option value="critical">Critical</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
        <select value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} className="border rounded px-3 py-2 text-sm">
          <option value="">All Types</option>
          <option value="policy">Policy</option>
          <option value="control">Control</option>
          <option value="standard">Standard</option>
          <option value="regulatory">Regulatory</option>
        </select>
        <input
          value={frameworkFilter}
          onChange={(e) => setFrameworkFilter(e.target.value)}
          className="border rounded px-3 py-2 text-sm"
          placeholder="Framework filter..."
        />
      </div>

      {/* DataTable */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-16 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      ) : exceptions.length === 0 ? (
        <div className="text-center py-16 text-gray-500">
          <p className="text-lg font-medium">No exceptions found</p>
          <p className="text-sm mt-1">Request an exception to get started</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-3 font-semibold text-gray-700">Ref</th>
                <th className="pb-3 font-semibold text-gray-700">Title</th>
                <th className="pb-3 font-semibold text-gray-700">Type</th>
                <th className="pb-3 font-semibold text-gray-700">Status</th>
                <th className="pb-3 font-semibold text-gray-700">Risk Level</th>
                <th className="pb-3 font-semibold text-gray-700">Controls</th>
                <th className="pb-3 font-semibold text-gray-700">Expiry Date</th>
                <th className="pb-3 font-semibold text-gray-700">Compensating</th>
                <th className="pb-3 font-semibold text-gray-700">Last Reviewed</th>
              </tr>
            </thead>
            <tbody>
              {exceptions.map((exc) => (
                <tr key={exc.id} className="border-b hover:bg-gray-50">
                  <td className="py-3 font-mono text-xs text-gray-600">{exc.reference}</td>
                  <td className="py-3">
                    <p className="font-medium text-gray-900">{exc.title}</p>
                  </td>
                  <td className="py-3">
                    <TypeBadge type={exc.type} />
                  </td>
                  <td className="py-3">
                    <StatusBadge status={exc.status} />
                  </td>
                  <td className="py-3">
                    <RiskBadge level={exc.risk_level} />
                  </td>
                  <td className="py-3 text-gray-600">{exc.affected_control_count}</td>
                  <td className="py-3">
                    <span
                      className={`text-sm ${
                        isExpired(exc.expiry_date)
                          ? 'text-red-600 font-semibold'
                          : isExpiringSoon(exc.expiry_date)
                          ? 'text-amber-600 font-medium'
                          : 'text-gray-600'
                      }`}
                    >
                      {new Date(exc.expiry_date).toLocaleDateString()}
                    </span>
                  </td>
                  <td className="py-3">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                        exc.compensating_controls
                          ? 'bg-green-100 text-green-700'
                          : 'bg-gray-100 text-gray-500'
                      }`}
                    >
                      {exc.compensating_controls ? 'Yes' : 'No'}
                    </span>
                  </td>
                  <td className="py-3 text-gray-500 text-xs">
                    {exc.last_reviewed ? new Date(exc.last_reviewed).toLocaleDateString() : '--'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showForm && <ExceptionRequestForm onClose={() => setShowForm(false)} />}
    </div>
  );
}
