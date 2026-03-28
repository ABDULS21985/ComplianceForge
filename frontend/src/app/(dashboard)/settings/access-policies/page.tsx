'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SubjectCondition {
  attribute: string;
  operator: string;
  value: string;
}

interface PolicyFormData {
  name: string;
  description: string;
  effect: 'allow' | 'deny';
  subject_conditions: SubjectCondition[];
  resource_type: string;
  resource_conditions: SubjectCondition[];
  actions: string[];
  require_mfa: boolean;
  ip_range: string;
  time_window_start: string;
  time_window_end: string;
  valid_from: string;
  valid_until: string;
  priority: number;
}

interface Policy {
  id: string;
  name: string;
  description: string;
  effect: 'allow' | 'deny';
  resource_type: string;
  actions: string[];
  priority: number;
  assignments: { id: string; type: 'user' | 'role'; name: string }[];
}

const EMPTY_CONDITION: SubjectCondition = { attribute: '', operator: 'equals', value: '' };

const ACTIONS = ['read', 'create', 'update', 'delete', 'approve', 'export'];

const OPERATORS = ['equals', 'not_equals', 'contains', 'in', 'not_in', 'greater_than', 'less_than'];

const RESOURCE_TYPES = [
  'risk', 'policy', 'framework', 'control', 'audit', 'incident', 'vendor', 'asset', 'report', 'user', 'setting',
];

const DEFAULT_FORM: PolicyFormData = {
  name: '',
  description: '',
  effect: 'allow',
  subject_conditions: [{ ...EMPTY_CONDITION }],
  resource_type: '',
  resource_conditions: [],
  actions: [],
  require_mfa: false,
  ip_range: '',
  time_window_start: '',
  time_window_end: '',
  valid_from: '',
  valid_until: '',
  priority: 100,
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AccessPoliciesPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'policies' | 'test' | 'audit'>('policies');
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [form, setForm] = useState<PolicyFormData>({ ...DEFAULT_FORM });
  const [editingId, setEditingId] = useState<string | null>(null);
  const [assignDialog, setAssignDialog] = useState<{ policyId: string; policyName: string } | null>(null);
  const [assignType, setAssignType] = useState<'user' | 'role'>('user');
  const [assignValue, setAssignValue] = useState('');

  // Test evaluation state
  const [testUser, setTestUser] = useState('');
  const [testAction, setTestAction] = useState('read');
  const [testResource, setTestResource] = useState('');
  const [testResult, setTestResult] = useState<{ allowed: boolean; reason: string } | null>(null);

  // Queries
  const { data: policies, isLoading, error } = useQuery({
    queryKey: ['access-policies'],
    queryFn: () => api.access.listPolicies(),
  });

  const { data: auditLog } = useQuery({
    queryKey: ['access-audit-log'],
    queryFn: () => api.access.auditLog(),
    enabled: activeTab === 'audit',
  });

  // Mutations
  const createPolicy = useMutation({
    mutationFn: (data: any) => api.access.createPolicy(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['access-policies'] });
      resetForm();
    },
  });

  const updatePolicy = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.access.updatePolicy(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['access-policies'] });
      resetForm();
    },
  });

  const deletePolicy = useMutation({
    mutationFn: (id: string) => api.access.deletePolicy(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['access-policies'] }),
  });

  const assignPolicy = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.access.assignPolicy(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['access-policies'] });
      setAssignDialog(null);
      setAssignValue('');
    },
  });

  const removeAssignment = useMutation({
    mutationFn: ({ policyId, assignmentId }: { policyId: string; assignmentId: string }) =>
      api.access.removeAssignment(policyId, assignmentId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['access-policies'] }),
  });

  const testEvaluate = useMutation({
    mutationFn: (data: any) => api.access.testEvaluate(data),
    onSuccess: (data: any) => setTestResult(data),
  });

  function resetForm() {
    setShowCreateForm(false);
    setEditingId(null);
    setForm({ ...DEFAULT_FORM });
  }

  function handleSubmit() {
    const payload = {
      ...form,
      subject_conditions: form.subject_conditions.filter((c) => c.attribute),
      resource_conditions: form.resource_conditions.filter((c) => c.attribute),
    };
    if (editingId) {
      updatePolicy.mutate({ id: editingId, data: payload });
    } else {
      createPolicy.mutate(payload);
    }
  }

  function editPolicy(policy: Policy) {
    setForm({
      name: policy.name,
      description: policy.description,
      effect: policy.effect,
      subject_conditions: [{ ...EMPTY_CONDITION }],
      resource_type: policy.resource_type,
      resource_conditions: [],
      actions: policy.actions,
      require_mfa: false,
      ip_range: '',
      time_window_start: '',
      time_window_end: '',
      valid_from: '',
      valid_until: '',
      priority: policy.priority,
    });
    setEditingId(policy.id);
    setShowCreateForm(true);
  }

  // Condition row helpers
  function updateCondition(
    list: SubjectCondition[],
    index: number,
    field: keyof SubjectCondition,
    value: string,
  ): SubjectCondition[] {
    return list.map((c, i) => (i === index ? { ...c, [field]: value } : c));
  }

  const policyList: Policy[] = policies?.items ?? policies ?? [];
  const auditLogItems: any[] = auditLog?.items ?? auditLog ?? [];

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="p-6 space-y-4">
        <h1 className="text-2xl font-bold">Access Policies</h1>
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-16 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-4">Access Policies</h1>
        <div className="bg-red-50 text-red-700 rounded-lg p-4">Failed to load access policies.</div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Access Policies (ABAC)</h1>
        {!showCreateForm && activeTab === 'policies' && (
          <button
            onClick={() => setShowCreateForm(true)}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
          >
            Create Policy
          </button>
        )}
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b">
        {(['policies', 'test', 'audit'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
              activeTab === tab
                ? 'border-blue-600 text-blue-600'
                : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            {tab === 'policies' ? 'Policies' : tab === 'test' ? 'Test Evaluation' : 'Audit Log'}
          </button>
        ))}
      </div>

      {/* Create/Edit Form */}
      {showCreateForm && activeTab === 'policies' && (
        <div className="border rounded-lg p-6 bg-white space-y-5">
          <h2 className="text-lg font-semibold">{editingId ? 'Edit Policy' : 'Create Policy'}</h2>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Name</label>
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Priority</label>
              <input
                type="number"
                value={form.priority}
                onChange={(e) => setForm({ ...form, priority: parseInt(e.target.value) || 0 })}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Description</label>
            <textarea
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              rows={2}
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>

          {/* Effect toggle */}
          <div>
            <label className="block text-sm font-medium mb-2">Effect</label>
            <div className="flex gap-2">
              <button
                onClick={() => setForm({ ...form, effect: 'allow' })}
                className={`px-4 py-2 text-sm font-medium rounded ${
                  form.effect === 'allow' ? 'bg-green-600 text-white' : 'bg-gray-100 text-gray-700'
                }`}
              >
                Allow
              </button>
              <button
                onClick={() => setForm({ ...form, effect: 'deny' })}
                className={`px-4 py-2 text-sm font-medium rounded ${
                  form.effect === 'deny' ? 'bg-red-600 text-white' : 'bg-gray-100 text-gray-700'
                }`}
              >
                Deny
              </button>
            </div>
          </div>

          {/* Subject Conditions */}
          <div>
            <label className="block text-sm font-medium mb-2">Subject Conditions</label>
            {form.subject_conditions.map((cond, idx) => (
              <div key={idx} className="flex gap-2 mb-2">
                <input
                  type="text"
                  placeholder="Attribute (e.g. department)"
                  value={cond.attribute}
                  onChange={(e) =>
                    setForm({ ...form, subject_conditions: updateCondition(form.subject_conditions, idx, 'attribute', e.target.value) })
                  }
                  className="flex-1 border rounded px-3 py-2 text-sm"
                />
                <select
                  value={cond.operator}
                  onChange={(e) =>
                    setForm({ ...form, subject_conditions: updateCondition(form.subject_conditions, idx, 'operator', e.target.value) })
                  }
                  className="border rounded px-3 py-2 text-sm"
                >
                  {OPERATORS.map((op) => (
                    <option key={op} value={op}>{op.replace(/_/g, ' ')}</option>
                  ))}
                </select>
                <input
                  type="text"
                  placeholder="Value"
                  value={cond.value}
                  onChange={(e) =>
                    setForm({ ...form, subject_conditions: updateCondition(form.subject_conditions, idx, 'value', e.target.value) })
                  }
                  className="flex-1 border rounded px-3 py-2 text-sm"
                />
                <button
                  onClick={() =>
                    setForm({ ...form, subject_conditions: form.subject_conditions.filter((_, i) => i !== idx) })
                  }
                  className="px-2 text-red-500 hover:text-red-700"
                >
                  x
                </button>
              </div>
            ))}
            <button
              onClick={() => setForm({ ...form, subject_conditions: [...form.subject_conditions, { ...EMPTY_CONDITION }] })}
              className="text-sm text-blue-600 hover:text-blue-800"
            >
              + Add condition
            </button>
          </div>

          {/* Resource Type */}
          <div>
            <label className="block text-sm font-medium mb-1">Resource Type</label>
            <select
              value={form.resource_type}
              onChange={(e) => setForm({ ...form, resource_type: e.target.value })}
              className="w-full border rounded px-3 py-2 text-sm"
            >
              <option value="">Select resource type</option>
              {RESOURCE_TYPES.map((rt) => (
                <option key={rt} value={rt}>{rt.charAt(0).toUpperCase() + rt.slice(1)}</option>
              ))}
            </select>
          </div>

          {/* Resource Conditions */}
          <div>
            <label className="block text-sm font-medium mb-2">Resource Conditions</label>
            {form.resource_conditions.map((cond, idx) => (
              <div key={idx} className="flex gap-2 mb-2">
                <input
                  type="text"
                  placeholder="Attribute"
                  value={cond.attribute}
                  onChange={(e) =>
                    setForm({ ...form, resource_conditions: updateCondition(form.resource_conditions, idx, 'attribute', e.target.value) })
                  }
                  className="flex-1 border rounded px-3 py-2 text-sm"
                />
                <select
                  value={cond.operator}
                  onChange={(e) =>
                    setForm({ ...form, resource_conditions: updateCondition(form.resource_conditions, idx, 'operator', e.target.value) })
                  }
                  className="border rounded px-3 py-2 text-sm"
                >
                  {OPERATORS.map((op) => (
                    <option key={op} value={op}>{op.replace(/_/g, ' ')}</option>
                  ))}
                </select>
                <input
                  type="text"
                  placeholder="Value"
                  value={cond.value}
                  onChange={(e) =>
                    setForm({ ...form, resource_conditions: updateCondition(form.resource_conditions, idx, 'value', e.target.value) })
                  }
                  className="flex-1 border rounded px-3 py-2 text-sm"
                />
                <button
                  onClick={() =>
                    setForm({ ...form, resource_conditions: form.resource_conditions.filter((_, i) => i !== idx) })
                  }
                  className="px-2 text-red-500 hover:text-red-700"
                >
                  x
                </button>
              </div>
            ))}
            <button
              onClick={() => setForm({ ...form, resource_conditions: [...form.resource_conditions, { ...EMPTY_CONDITION }] })}
              className="text-sm text-blue-600 hover:text-blue-800"
            >
              + Add condition
            </button>
          </div>

          {/* Actions */}
          <div>
            <label className="block text-sm font-medium mb-2">Actions</label>
            <div className="flex flex-wrap gap-3">
              {ACTIONS.map((action) => (
                <label key={action} className="flex items-center gap-1.5 text-sm">
                  <input
                    type="checkbox"
                    checked={form.actions.includes(action)}
                    onChange={(e) => {
                      if (e.target.checked) {
                        setForm({ ...form, actions: [...form.actions, action] });
                      } else {
                        setForm({ ...form, actions: form.actions.filter((a) => a !== action) });
                      }
                    }}
                    className="rounded"
                  />
                  {action}
                </label>
              ))}
            </div>
          </div>

          {/* Environment Conditions */}
          <div className="space-y-3">
            <label className="block text-sm font-medium">Environment Conditions</label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.require_mfa}
                onChange={(e) => setForm({ ...form, require_mfa: e.target.checked })}
                className="rounded"
              />
              Require MFA
            </label>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Allowed IP Range (CIDR)</label>
              <input
                type="text"
                value={form.ip_range}
                onChange={(e) => setForm({ ...form, ip_range: e.target.value })}
                className="w-full border rounded px-3 py-2 text-sm"
                placeholder="e.g. 10.0.0.0/8"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-xs text-gray-500 mb-1">Time Window Start</label>
                <input
                  type="time"
                  value={form.time_window_start}
                  onChange={(e) => setForm({ ...form, time_window_start: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-500 mb-1">Time Window End</label>
                <input
                  type="time"
                  value={form.time_window_end}
                  onChange={(e) => setForm({ ...form, time_window_end: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
            </div>
          </div>

          {/* Validity Period */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Valid From</label>
              <input
                type="date"
                value={form.valid_from}
                onChange={(e) => setForm({ ...form, valid_from: e.target.value })}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Valid Until</label>
              <input
                type="date"
                value={form.valid_until}
                onChange={(e) => setForm({ ...form, valid_until: e.target.value })}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
          </div>

          {/* Form buttons */}
          <div className="flex gap-2 pt-2">
            <button
              onClick={handleSubmit}
              disabled={!form.name || !form.resource_type || form.actions.length === 0 || createPolicy.isPending || updatePolicy.isPending}
              className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {createPolicy.isPending || updatePolicy.isPending ? 'Saving...' : editingId ? 'Update Policy' : 'Create Policy'}
            </button>
            <button onClick={resetForm} className="px-4 py-2 text-sm font-medium rounded border border-gray-300 hover:bg-gray-50">
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Policy List */}
      {activeTab === 'policies' && !showCreateForm && (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium">Effect</th>
                <th className="text-left px-4 py-2 font-medium">Name</th>
                <th className="text-left px-4 py-2 font-medium">Resource</th>
                <th className="text-left px-4 py-2 font-medium">Actions</th>
                <th className="text-left px-4 py-2 font-medium">Assigned To</th>
                <th className="text-left px-4 py-2 font-medium">Priority</th>
                <th className="text-right px-4 py-2 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {policyList.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-gray-400">No access policies configured yet.</td>
                </tr>
              )}
              {policyList.map((policy) => (
                <tr key={policy.id} className="border-b last:border-0">
                  <td className="px-4 py-3">
                    <span
                      className={`text-xs font-bold px-2 py-1 rounded ${
                        policy.effect === 'allow' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                      }`}
                    >
                      {policy.effect.toUpperCase()}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-medium">{policy.name}</td>
                  <td className="px-4 py-3 capitalize">{policy.resource_type}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {policy.actions.map((a) => (
                        <span key={a} className="text-xs bg-gray-100 px-1.5 py-0.5 rounded">{a}</span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {(policy.assignments ?? []).map((a) => (
                        <span key={a.id} className="text-xs bg-blue-50 text-blue-700 px-1.5 py-0.5 rounded flex items-center gap-1">
                          {a.name}
                          <button
                            onClick={() => removeAssignment.mutate({ policyId: policy.id, assignmentId: a.id })}
                            className="text-blue-400 hover:text-red-500"
                          >
                            x
                          </button>
                        </span>
                      ))}
                      <button
                        onClick={() => setAssignDialog({ policyId: policy.id, policyName: policy.name })}
                        className="text-xs text-blue-600 hover:text-blue-800"
                      >
                        + Assign
                      </button>
                    </div>
                  </td>
                  <td className="px-4 py-3">{policy.priority}</td>
                  <td className="px-4 py-3 text-right space-x-2">
                    <button onClick={() => editPolicy(policy)} className="text-blue-600 hover:text-blue-800 text-xs font-medium">
                      Edit
                    </button>
                    <button
                      onClick={() => {
                        if (confirm('Delete this policy?')) deletePolicy.mutate(policy.id);
                      }}
                      className="text-red-600 hover:text-red-800 text-xs font-medium"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Test Evaluation Tab */}
      {activeTab === 'test' && (
        <div className="max-w-xl space-y-4">
          <p className="text-sm text-gray-500">Simulate an access evaluation to check if a user would be allowed or denied.</p>
          <div>
            <label className="block text-sm font-medium mb-1">User (ID or email)</label>
            <input
              type="text"
              value={testUser}
              onChange={(e) => setTestUser(e.target.value)}
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Action</label>
            <select
              value={testAction}
              onChange={(e) => setTestAction(e.target.value)}
              className="w-full border rounded px-3 py-2 text-sm"
            >
              {ACTIONS.map((a) => (
                <option key={a} value={a}>{a}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Resource Type</label>
            <select
              value={testResource}
              onChange={(e) => setTestResource(e.target.value)}
              className="w-full border rounded px-3 py-2 text-sm"
            >
              <option value="">Select</option>
              {RESOURCE_TYPES.map((rt) => (
                <option key={rt} value={rt}>{rt}</option>
              ))}
            </select>
          </div>
          <button
            onClick={() =>
              testEvaluate.mutate({ user: testUser, action: testAction, resource_type: testResource })
            }
            disabled={!testUser || !testResource || testEvaluate.isPending}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {testEvaluate.isPending ? 'Evaluating...' : 'Evaluate'}
          </button>

          {testResult && (
            <div
              className={`rounded-lg p-4 ${
                testResult.allowed ? 'bg-green-50 text-green-800 border border-green-200' : 'bg-red-50 text-red-800 border border-red-200'
              }`}
            >
              <p className="font-bold text-lg">{testResult.allowed ? 'ALLOWED' : 'DENIED'}</p>
              <p className="text-sm mt-1">{testResult.reason}</p>
            </div>
          )}
        </div>
      )}

      {/* Audit Log Tab */}
      {activeTab === 'audit' && (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium">Timestamp</th>
                <th className="text-left px-4 py-2 font-medium">User</th>
                <th className="text-left px-4 py-2 font-medium">Action</th>
                <th className="text-left px-4 py-2 font-medium">Resource</th>
                <th className="text-left px-4 py-2 font-medium">Result</th>
              </tr>
            </thead>
            <tbody>
              {auditLogItems.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-12 text-center text-gray-400">No audit log entries yet.</td>
                </tr>
              )}
              {auditLogItems.map((entry: any, idx: number) => (
                <tr key={entry.id ?? idx} className="border-b last:border-0">
                  <td className="px-4 py-2 text-gray-500">{new Date(entry.timestamp).toLocaleString()}</td>
                  <td className="px-4 py-2">{entry.user_name ?? entry.user_id}</td>
                  <td className="px-4 py-2">{entry.action}</td>
                  <td className="px-4 py-2">{entry.resource_type} {entry.resource_id ? `#${entry.resource_id.slice(0, 8)}` : ''}</td>
                  <td className="px-4 py-2">
                    <span className={`text-xs font-medium px-2 py-0.5 rounded ${entry.allowed ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                      {entry.allowed ? 'Allowed' : 'Denied'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Assignment Dialog */}
      {assignDialog && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
          <div className="bg-white rounded-lg shadow-xl max-w-md w-full p-6 space-y-4">
            <h2 className="text-lg font-bold">Assign Policy: {assignDialog.policyName}</h2>
            <div>
              <label className="block text-sm font-medium mb-1">Assign to</label>
              <div className="flex gap-2 mb-3">
                <button
                  onClick={() => setAssignType('user')}
                  className={`px-3 py-1 text-sm rounded ${assignType === 'user' ? 'bg-blue-600 text-white' : 'bg-gray-100'}`}
                >
                  User
                </button>
                <button
                  onClick={() => setAssignType('role')}
                  className={`px-3 py-1 text-sm rounded ${assignType === 'role' ? 'bg-blue-600 text-white' : 'bg-gray-100'}`}
                >
                  Role
                </button>
              </div>
              <input
                type="text"
                value={assignValue}
                onChange={(e) => setAssignValue(e.target.value)}
                className="w-full border rounded px-3 py-2 text-sm"
                placeholder={assignType === 'user' ? 'User email or ID' : 'Role name or ID'}
              />
            </div>
            <div className="flex gap-2 justify-end">
              <button onClick={() => setAssignDialog(null)} className="px-4 py-2 text-sm rounded border border-gray-300 hover:bg-gray-50">
                Cancel
              </button>
              <button
                onClick={() =>
                  assignPolicy.mutate({
                    id: assignDialog.policyId,
                    data: { type: assignType, identifier: assignValue },
                  })
                }
                disabled={!assignValue.trim() || assignPolicy.isPending}
                className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {assignPolicy.isPending ? 'Assigning...' : 'Assign'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
