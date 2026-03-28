'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Approval {
  id: string;
  workflow_instance_id: string;
  step_name: string;
  entity_type: string;
  entity_name: string;
  entity_id: string;
  requested_by: string;
  requested_at: string;
  sla_deadline: string;
  status: 'pending' | 'approved' | 'rejected' | 'delegated';
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function timeRemaining(deadline: string): { label: string; status: 'ok' | 'at_risk' | 'overdue' } {
  const now = Date.now();
  const end = new Date(deadline).getTime();
  const diff = end - now;

  if (diff <= 0) return { label: 'Overdue', status: 'overdue' };

  const hours = Math.floor(diff / 3_600_000);
  const minutes = Math.floor((diff % 3_600_000) / 60_000);

  if (hours < 4) return { label: `${hours}h ${minutes}m left`, status: 'at_risk' };
  if (hours < 24) return { label: `${hours}h left`, status: 'at_risk' };

  const days = Math.floor(hours / 24);
  return { label: `${days}d ${hours % 24}h left`, status: 'ok' };
}

function slaColor(status: 'ok' | 'at_risk' | 'overdue') {
  if (status === 'overdue') return 'text-red-600 bg-red-50';
  if (status === 'at_risk') return 'text-amber-600 bg-amber-50';
  return 'text-green-600 bg-green-50';
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function WorkflowApprovalsPage() {
  const queryClient = useQueryClient();
  const [activeAction, setActiveAction] = useState<{ id: string; type: 'approve' | 'reject' | 'delegate' } | null>(null);
  const [comment, setComment] = useState('');
  const [delegateTo, setDelegateTo] = useState('');

  // Fetch approvals
  const { data: approvals, isLoading, error } = useQuery({
    queryKey: ['workflow-approvals'],
    queryFn: () => api.workflows.myApprovals(),
  });

  // Mutations
  const approveMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.workflows.approveStep(id, data),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['workflow-approvals'] }); resetAction(); },
  });

  const rejectMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.workflows.rejectStep(id, data),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['workflow-approvals'] }); resetAction(); },
  });

  const delegateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.workflows.delegateStep(id, data),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['workflow-approvals'] }); resetAction(); },
  });

  function resetAction() {
    setActiveAction(null);
    setComment('');
    setDelegateTo('');
  }

  function handleSubmitAction() {
    if (!activeAction) return;
    const { id, type } = activeAction;
    if (type === 'approve') {
      approveMutation.mutate({ id, data: { comment: comment || undefined } });
    } else if (type === 'reject') {
      if (!comment.trim()) return;
      rejectMutation.mutate({ id, data: { comment } });
    } else if (type === 'delegate') {
      if (!delegateTo.trim()) return;
      delegateMutation.mutate({ id, data: { delegate_to: delegateTo, comment: comment || undefined } });
    }
  }

  // Derive summary counts
  const items: Approval[] = approvals?.items ?? approvals ?? [];
  const sorted = [...items].sort(
    (a, b) => new Date(a.sla_deadline).getTime() - new Date(b.sla_deadline).getTime(),
  );

  const totalPending = sorted.length;
  const atRisk = sorted.filter((a) => timeRemaining(a.sla_deadline).status === 'at_risk').length;
  const overdue = sorted.filter((a) => timeRemaining(a.sla_deadline).status === 'overdue').length;
  const completedToday = 0; // placeholder — would come from a separate query

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="p-6 space-y-4">
        <h1 className="text-2xl font-bold">Workflow Approvals</h1>
        <div className="grid grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-24 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-20 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-4">Workflow Approvals</h1>
        <div className="bg-red-50 text-red-700 rounded-lg p-4">
          Failed to load approvals. Please try again later.
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Workflow Approvals</h1>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <SummaryCard label="Total Pending" value={totalPending} color="blue" />
        <SummaryCard label="At Risk (SLA)" value={atRisk} color="amber" />
        <SummaryCard label="Overdue" value={overdue} color="red" />
        <SummaryCard label="Completed Today" value={completedToday} color="green" />
      </div>

      {/* Empty state */}
      {sorted.length === 0 && (
        <div className="text-center py-16 text-gray-500">
          <p className="text-lg font-medium">No pending approvals</p>
          <p className="text-sm mt-1">You&apos;re all caught up!</p>
        </div>
      )}

      {/* Approval List */}
      <div className="space-y-3">
        {sorted.map((item) => {
          const sla = timeRemaining(item.sla_deadline);
          const isActive = activeAction?.id === item.id;

          return (
            <div
              key={item.id}
              className="border rounded-lg p-4 bg-white shadow-sm hover:shadow-md transition-shadow"
            >
              <div className="flex items-start justify-between gap-4">
                {/* Left: context */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-xs font-medium uppercase text-gray-400">{item.entity_type}</span>
                    <span className="text-xs text-gray-300">|</span>
                    <span className="text-xs text-gray-500">{item.step_name}</span>
                  </div>
                  <p className="font-semibold text-gray-900 truncate">{item.entity_name}</p>
                  <p className="text-sm text-gray-500 mt-0.5">
                    Requested by {item.requested_by} &middot;{' '}
                    {new Date(item.requested_at).toLocaleDateString()}
                  </p>
                </div>

                {/* Right: SLA + actions */}
                <div className="flex flex-col items-end gap-2 shrink-0">
                  <span className={`text-xs font-medium px-2 py-1 rounded-full ${slaColor(sla.status)}`}>
                    {sla.label}
                  </span>
                  <div className="flex gap-2">
                    <button
                      onClick={() => setActiveAction({ id: item.id, type: 'approve' })}
                      className="px-3 py-1 text-sm font-medium rounded bg-green-600 text-white hover:bg-green-700"
                    >
                      Approve
                    </button>
                    <button
                      onClick={() => setActiveAction({ id: item.id, type: 'reject' })}
                      className="px-3 py-1 text-sm font-medium rounded bg-red-600 text-white hover:bg-red-700"
                    >
                      Reject
                    </button>
                    <button
                      onClick={() => setActiveAction({ id: item.id, type: 'delegate' })}
                      className="px-3 py-1 text-sm font-medium rounded border border-gray-300 text-gray-700 hover:bg-gray-50"
                    >
                      Delegate
                    </button>
                  </div>
                </div>
              </div>

              {/* Expanded action panel */}
              {isActive && (
                <div className="mt-4 pt-4 border-t space-y-3">
                  {activeAction.type === 'reject' && (
                    <p className="text-sm text-red-600 font-medium">Comment is required for rejection.</p>
                  )}
                  {activeAction.type === 'delegate' && (
                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-1">Delegate to (user ID or email)</label>
                      <input
                        type="text"
                        value={delegateTo}
                        onChange={(e) => setDelegateTo(e.target.value)}
                        className="w-full border rounded px-3 py-2 text-sm"
                        placeholder="Enter user email or ID"
                      />
                    </div>
                  )}
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      Comment {activeAction.type === 'reject' ? '(required)' : '(optional)'}
                    </label>
                    <textarea
                      value={comment}
                      onChange={(e) => setComment(e.target.value)}
                      rows={2}
                      className="w-full border rounded px-3 py-2 text-sm"
                      placeholder="Add a comment..."
                    />
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={handleSubmitAction}
                      disabled={
                        (activeAction.type === 'reject' && !comment.trim()) ||
                        (activeAction.type === 'delegate' && !delegateTo.trim()) ||
                        approveMutation.isPending || rejectMutation.isPending || delegateMutation.isPending
                      }
                      className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                    >
                      {approveMutation.isPending || rejectMutation.isPending || delegateMutation.isPending
                        ? 'Submitting...'
                        : `Confirm ${activeAction.type.charAt(0).toUpperCase() + activeAction.type.slice(1)}`}
                    </button>
                    <button
                      onClick={resetAction}
                      className="px-4 py-2 text-sm font-medium rounded border border-gray-300 text-gray-700 hover:bg-gray-50"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function SummaryCard({ label, value, color }: { label: string; value: number; color: string }) {
  const colorMap: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 border-blue-200',
    amber: 'bg-amber-50 text-amber-700 border-amber-200',
    red: 'bg-red-50 text-red-700 border-red-200',
    green: 'bg-green-50 text-green-700 border-green-200',
  };

  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.blue}`}>
      <p className="text-sm font-medium opacity-80">{label}</p>
      <p className="text-3xl font-bold mt-1">{value}</p>
    </div>
  );
}
