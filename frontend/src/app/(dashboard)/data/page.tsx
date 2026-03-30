'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ProcessingActivity {
  id: string;
  reference: string;
  name: string;
  description: string;
  purpose: string;
  legal_basis: 'consent' | 'contract' | 'legal_obligation' | 'vital_interests' | 'public_task' | 'legitimate_interests';
  data_subjects: string[];
  data_categories: string[];
  special_categories: boolean;
  special_category_types?: string[];
  recipients: string[];
  international_transfers: boolean;
  transfer_countries?: string[];
  transfer_safeguards?: string;
  retention_period: string;
  dpia_required: boolean;
  dpia_status?: 'not_started' | 'in_progress' | 'completed' | 'not_required';
  automated_decision_making: boolean;
  data_controller: string;
  data_processor?: string;
  technical_measures: string[];
  organizational_measures: string[];
  last_reviewed?: string;
  status: 'active' | 'inactive' | 'under_review';
  created_at: string;
}

interface ROPADashboard {
  total_activities: number;
  by_legal_basis: { basis: string; count: number }[];
  special_categories_count: number;
  international_transfers_count: number;
  dpia_status: { status: string; count: number }[];
  overdue_reviews: number;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function SummaryCard({ label, value, color }: { label: string; value: number | string; color: string }) {
  const colorMap: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 border-blue-200',
    green: 'bg-green-50 text-green-700 border-green-200',
    amber: 'bg-amber-50 text-amber-700 border-amber-200',
    red: 'bg-red-50 text-red-700 border-red-200',
    purple: 'bg-purple-50 text-purple-700 border-purple-200',
    indigo: 'bg-indigo-50 text-indigo-700 border-indigo-200',
  };
  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.blue}`}>
      <p className="text-xs font-medium uppercase tracking-wide opacity-70">{label}</p>
      <p className="mt-1 text-2xl font-bold">{value}</p>
    </div>
  );
}

function LegalBasisBadge({ basis }: { basis: string }) {
  const map: Record<string, string> = {
    consent: 'bg-blue-100 text-blue-700',
    contract: 'bg-green-100 text-green-700',
    legal_obligation: 'bg-purple-100 text-purple-700',
    vital_interests: 'bg-red-100 text-red-700',
    public_task: 'bg-amber-100 text-amber-700',
    legitimate_interests: 'bg-indigo-100 text-indigo-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[basis] ?? 'bg-gray-100 text-gray-700'}`}>
      {basis.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

function DPIABadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    not_started: 'bg-gray-100 text-gray-700',
    in_progress: 'bg-blue-100 text-blue-700',
    completed: 'bg-green-100 text-green-700',
    not_required: 'bg-gray-50 text-gray-500',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[status] ?? 'bg-gray-100 text-gray-700'}`}>
      {status.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

function PieChart({ data }: { data: { basis: string; count: number }[] }) {
  const total = data.reduce((s, d) => s + d.count, 0);
  if (total === 0) return <div className="flex items-center justify-center h-40 text-sm text-gray-400">No data</div>;

  const colors = ['#1A56DB', '#059669', '#7C3AED', '#DC2626', '#D97706', '#4F46E5', '#0891B2'];
  let cumulativePct = 0;
  const segments = data.map((d, i) => {
    const pct = (d.count / total) * 100;
    const start = cumulativePct;
    cumulativePct += pct;
    return { ...d, pct, start, color: colors[i % colors.length] };
  });
  const gradientParts = segments.map((s) => `${s.color} ${s.start}% ${s.start + s.pct}%`).join(', ');

  return (
    <div className="flex items-center gap-4">
      <div
        className="w-32 h-32 rounded-full flex-shrink-0"
        style={{
          background: `conic-gradient(${gradientParts})`,
        }}
      />
      <div className="space-y-1.5">
        {segments.map((s) => (
          <div key={s.basis} className="flex items-center gap-2 text-xs">
            <span className="w-3 h-3 rounded-full flex-shrink-0" style={{ background: s.color }} />
            <span className="capitalize">{s.basis.replace(/_/g, ' ')}</span>
            <span className="text-gray-500">{s.count}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Add Processing Activity Form (Article 30)
// ---------------------------------------------------------------------------

function AddProcessingActivityForm({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({
    name: '',
    description: '',
    purpose: '',
    legal_basis: 'legitimate_interests',
    data_subjects: '',
    data_categories: '',
    special_categories: false,
    special_category_types: '',
    recipients: '',
    international_transfers: false,
    transfer_countries: '',
    transfer_safeguards: '',
    retention_period: '',
    dpia_required: false,
    automated_decision_making: false,
    data_controller: '',
    data_processor: '',
    technical_measures: '',
    organizational_measures: '',
  });

  const createMutation = useMutation({
    mutationFn: (data: any) => api.data.createActivity(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['processing-activities'] });
      onClose();
    },
  });

  const handleSubmit = () => {
    createMutation.mutate({
      ...form,
      data_subjects: form.data_subjects.split(',').map((s) => s.trim()).filter(Boolean),
      data_categories: form.data_categories.split(',').map((s) => s.trim()).filter(Boolean),
      special_category_types: form.special_category_types.split(',').map((s) => s.trim()).filter(Boolean),
      recipients: form.recipients.split(',').map((s) => s.trim()).filter(Boolean),
      transfer_countries: form.transfer_countries.split(',').map((s) => s.trim()).filter(Boolean),
      technical_measures: form.technical_measures.split(',').map((s) => s.trim()).filter(Boolean),
      organizational_measures: form.organizational_measures.split(',').map((s) => s.trim()).filter(Boolean),
    });
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[90vh] overflow-auto">
        <div className="border-b p-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">Add Processing Activity (Article 30)</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl">&times;</button>
        </div>
        <div className="p-4 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Activity Name *</label>
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Purpose *</label>
              <input value={form.purpose} onChange={(e) => setForm({ ...form, purpose: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Description</label>
            <textarea value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} rows={2} className="w-full border rounded px-3 py-2 text-sm" />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Legal Basis *</label>
              <select value={form.legal_basis} onChange={(e) => setForm({ ...form, legal_basis: e.target.value })} className="w-full border rounded px-3 py-2 text-sm">
                <option value="consent">Consent</option>
                <option value="contract">Contract</option>
                <option value="legal_obligation">Legal Obligation</option>
                <option value="vital_interests">Vital Interests</option>
                <option value="public_task">Public Task</option>
                <option value="legitimate_interests">Legitimate Interests</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Retention Period</label>
              <input value={form.retention_period} onChange={(e) => setForm({ ...form, retention_period: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="e.g., 6 years" />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Data Subjects (comma-separated)</label>
            <input value={form.data_subjects} onChange={(e) => setForm({ ...form, data_subjects: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="Employees, Customers, Suppliers..." />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Data Categories (comma-separated)</label>
            <input value={form.data_categories} onChange={(e) => setForm({ ...form, data_categories: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="Name, Email, Address, Financial..." />
          </div>
          <div className="flex items-center gap-4">
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.special_categories} onChange={(e) => setForm({ ...form, special_categories: e.target.checked })} className="rounded" />
              Special Category Data
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.international_transfers} onChange={(e) => setForm({ ...form, international_transfers: e.target.checked })} className="rounded" />
              International Transfers
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.dpia_required} onChange={(e) => setForm({ ...form, dpia_required: e.target.checked })} className="rounded" />
              DPIA Required
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.automated_decision_making} onChange={(e) => setForm({ ...form, automated_decision_making: e.target.checked })} className="rounded" />
              Automated Decisions
            </label>
          </div>
          {form.special_categories && (
            <div>
              <label className="block text-sm font-medium mb-1">Special Category Types</label>
              <input value={form.special_category_types} onChange={(e) => setForm({ ...form, special_category_types: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="Health, Biometric, Race..." />
            </div>
          )}
          {form.international_transfers && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium mb-1">Transfer Countries</label>
                <input value={form.transfer_countries} onChange={(e) => setForm({ ...form, transfer_countries: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="US, IN..." />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Transfer Safeguards</label>
                <input value={form.transfer_safeguards} onChange={(e) => setForm({ ...form, transfer_safeguards: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="SCCs, Adequacy decision..." />
              </div>
            </div>
          )}
          <div>
            <label className="block text-sm font-medium mb-1">Recipients (comma-separated)</label>
            <input value={form.recipients} onChange={(e) => setForm({ ...form, recipients: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Data Controller *</label>
              <input value={form.data_controller} onChange={(e) => setForm({ ...form, data_controller: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Data Processor</label>
              <input value={form.data_processor} onChange={(e) => setForm({ ...form, data_processor: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Technical Measures (comma-separated)</label>
            <input value={form.technical_measures} onChange={(e) => setForm({ ...form, technical_measures: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="Encryption, Access controls, Logging..." />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Organizational Measures (comma-separated)</label>
            <input value={form.organizational_measures} onChange={(e) => setForm({ ...form, organizational_measures: e.target.value })} className="w-full border rounded px-3 py-2 text-sm" placeholder="Training, Policies, Audits..." />
          </div>
        </div>
        <div className="border-t p-4 flex justify-end gap-3">
          <button onClick={onClose} className="px-4 py-2 text-sm font-medium rounded border hover:bg-gray-50">Cancel</button>
          <button
            onClick={handleSubmit}
            disabled={createMutation.isPending || !form.name || !form.purpose || !form.legal_basis}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {createMutation.isPending ? 'Creating...' : 'Create Activity'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function DataGovernancePage() {
  const [showAddForm, setShowAddForm] = useState(false);
  const [exportFormat, setExportFormat] = useState('');

  const { data: dashboardData } = useQuery<ROPADashboard>({
    queryKey: ['ropa-dashboard'],
    queryFn: () => api.data.dashboard(),
  });

  const { data: activitiesData, isLoading } = useQuery({
    queryKey: ['processing-activities'],
    queryFn: () => api.data.listActivities(),
  });

  const exportMutation = useMutation({
    mutationFn: (format: string) => api.data.exportROPA({ format }),
  });

  const activities: ProcessingActivity[] = activitiesData?.items ?? activitiesData ?? [];

  const dash = dashboardData ?? {
    total_activities: activities.length,
    by_legal_basis: [],
    special_categories_count: activities.filter((a) => a.special_categories).length,
    international_transfers_count: activities.filter((a) => a.international_transfers).length,
    dpia_status: [],
    overdue_reviews: 0,
  };

  const handleExport = (format: string) => {
    setExportFormat(format);
    exportMutation.mutate(format);
  };

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Data Governance</h1>
        <div className="flex gap-2">
          <div className="relative">
            <select
              value={exportFormat}
              onChange={(e) => {
                if (e.target.value) handleExport(e.target.value);
              }}
              className="border rounded px-3 py-2 text-sm"
            >
              <option value="">Export ROPA...</option>
              <option value="pdf">Export as PDF</option>
              <option value="xlsx">Export as XLSX</option>
            </select>
          </div>
          <button
            onClick={() => setShowAddForm(true)}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
          >
            Add Processing Activity
          </button>
        </div>
      </div>

      {/* ROPA Dashboard */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-6 gap-4">
        <SummaryCard label="Total Activities" value={dash.total_activities} color="blue" />
        <SummaryCard label="Special Categories" value={dash.special_categories_count} color="purple" />
        <SummaryCard label="Int'l Transfers" value={dash.international_transfers_count} color="indigo" />
        <SummaryCard label="Overdue Reviews" value={dash.overdue_reviews} color="red" />
        <div className="col-span-1 sm:col-span-2 border rounded-lg p-4 bg-white">
          <h3 className="text-xs font-medium text-gray-500 uppercase mb-2">By Legal Basis</h3>
          {dash.by_legal_basis.length > 0 ? (
            <PieChart data={dash.by_legal_basis} />
          ) : (
            <div className="text-sm text-gray-400 text-center py-4">No data</div>
          )}
        </div>
      </div>

      {/* DPIA Status Breakdown */}
      {dash.dpia_status.length > 0 && (
        <div className="border rounded-lg p-4 bg-white">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">DPIA Status</h3>
          <div className="flex gap-6">
            {dash.dpia_status.map((d) => (
              <div key={d.status} className="text-center">
                <p className="text-xl font-bold text-gray-900">{d.count}</p>
                <p className="text-xs text-gray-500 capitalize">{d.status.replace(/_/g, ' ')}</p>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Processing Activity List */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-16 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      ) : activities.length === 0 ? (
        <div className="text-center py-16 text-gray-500">
          <p className="text-lg font-medium">No processing activities found</p>
          <p className="text-sm mt-1">Add a processing activity to build your ROPA</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-3 font-semibold text-gray-700">Ref</th>
                <th className="pb-3 font-semibold text-gray-700">Name</th>
                <th className="pb-3 font-semibold text-gray-700">Purpose</th>
                <th className="pb-3 font-semibold text-gray-700">Legal Basis</th>
                <th className="pb-3 font-semibold text-gray-700">Subjects</th>
                <th className="pb-3 font-semibold text-gray-700">Special Cat.</th>
                <th className="pb-3 font-semibold text-gray-700">Transfers</th>
                <th className="pb-3 font-semibold text-gray-700">DPIA</th>
              </tr>
            </thead>
            <tbody>
              {activities.map((act) => (
                <tr key={act.id} className="border-b hover:bg-gray-50">
                  <td className="py-3 font-mono text-xs text-gray-600">{act.reference}</td>
                  <td className="py-3">
                    <p className="font-medium text-gray-900">{act.name}</p>
                  </td>
                  <td className="py-3 text-gray-600 max-w-[200px] truncate">{act.purpose}</td>
                  <td className="py-3">
                    <LegalBasisBadge basis={act.legal_basis} />
                  </td>
                  <td className="py-3 text-gray-600 text-xs">{act.data_subjects.join(', ')}</td>
                  <td className="py-3">
                    {act.special_categories ? (
                      <span className="text-xs bg-red-100 text-red-700 px-2 py-0.5 rounded-full font-medium">Yes</span>
                    ) : (
                      <span className="text-xs text-gray-400">No</span>
                    )}
                  </td>
                  <td className="py-3">
                    {act.international_transfers ? (
                      <span className="text-xs bg-amber-100 text-amber-700 px-2 py-0.5 rounded-full font-medium">Yes</span>
                    ) : (
                      <span className="text-xs text-gray-400">No</span>
                    )}
                  </td>
                  <td className="py-3">
                    {act.dpia_status ? <DPIABadge status={act.dpia_status} /> : <span className="text-xs text-gray-400">N/A</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showAddForm && <AddProcessingActivityForm onClose={() => setShowAddForm(false)} />}
    </div>
  );
}
