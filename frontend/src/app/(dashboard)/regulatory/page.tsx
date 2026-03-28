'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface RegulatoryChange {
  id: string;
  title: string;
  summary: string;
  source: string;
  source_url?: string;
  published_date: string;
  effective_date?: string;
  severity: 'critical' | 'high' | 'medium' | 'low';
  status: 'new' | 'under_review' | 'assessed' | 'action_required' | 'resolved' | 'dismissed';
  frameworks: string[];
  regions: string[];
  impact_score?: number;
  assessor?: string;
}

interface RegulatorySource {
  id: string;
  name: string;
  type: string;
  region: string;
  subscribed: boolean;
}

interface DashboardStats {
  new_changes: number;
  pending_assessments: number;
  upcoming_deadlines: number;
  action_required: number;
}

// ---------------------------------------------------------------------------
// Filter Options
// ---------------------------------------------------------------------------

const SEVERITY_OPTIONS = [
  { value: '', label: 'All Severities' },
  { value: 'critical', label: 'Critical' },
  { value: 'high', label: 'High' },
  { value: 'medium', label: 'Medium' },
  { value: 'low', label: 'Low' },
];

const STATUS_OPTIONS = [
  { value: '', label: 'All Statuses' },
  { value: 'new', label: 'New' },
  { value: 'under_review', label: 'Under Review' },
  { value: 'assessed', label: 'Assessed' },
  { value: 'action_required', label: 'Action Required' },
  { value: 'resolved', label: 'Resolved' },
  { value: 'dismissed', label: 'Dismissed' },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function RegulatoryChangePage() {
  const queryClient = useQueryClient();
  const [severityFilter, setSeverityFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [sourceFilter, setSourceFilter] = useState('');
  const [frameworkFilter, setFrameworkFilter] = useState('');
  const [regionFilter, setRegionFilter] = useState('');
  const [showSources, setShowSources] = useState(false);

  // Fetch dashboard
  const { data: dashboardData } = useQuery({
    queryKey: ['regulatory-dashboard'],
    queryFn: () => api.regulatory.dashboard(),
  });

  // Fetch changes
  const { data: changesData, isLoading, error } = useQuery({
    queryKey: ['regulatory-changes', severityFilter, statusFilter, sourceFilter, frameworkFilter, regionFilter],
    queryFn: () =>
      api.regulatory.listChanges({
        severity: severityFilter || undefined,
        status: statusFilter || undefined,
        source: sourceFilter || undefined,
        framework: frameworkFilter || undefined,
        region: regionFilter || undefined,
      }),
  });

  // Fetch sources
  const { data: sourcesData } = useQuery({
    queryKey: ['regulatory-sources'],
    queryFn: () => api.regulatory.listSources(),
  });

  // Assess impact mutation
  const assessMutation = useMutation({
    mutationFn: (id: string) => api.regulatory.assessImpact(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['regulatory-changes'] });
      queryClient.invalidateQueries({ queryKey: ['regulatory-dashboard'] });
    },
  });

  // Subscribe mutation
  const subscribeMutation = useMutation({
    mutationFn: (data: { source_id: string; subscribed: boolean }) => api.regulatory.subscribe(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['regulatory-sources'] });
    },
  });

  const dashboard: DashboardStats = (dashboardData as DashboardStats) ?? {
    new_changes: 0,
    pending_assessments: 0,
    upcoming_deadlines: 0,
    action_required: 0,
  };
  const changes: RegulatoryChange[] = changesData?.items ?? changesData ?? [];
  const sources: RegulatorySource[] = sourcesData?.items ?? sourcesData ?? [];

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (error) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-4">Regulatory Change Management</h1>
        <div className="bg-red-50 text-red-700 rounded-lg p-4">
          Failed to load regulatory changes. Please try again later.
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Regulatory Change Management</h1>
        <button
          onClick={() => setShowSources(!showSources)}
          className={`px-4 py-2 text-sm font-medium rounded ${showSources ? 'bg-blue-600 text-white' : 'border border-gray-300 text-gray-700 hover:bg-gray-50'}`}
        >
          {showSources ? 'Back to Feed' : 'Manage Sources'}
        </button>
      </div>

      {/* Dashboard Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <SummaryCard label="New Changes" value={dashboard.new_changes} color="blue" />
        <SummaryCard label="Pending Assessments" value={dashboard.pending_assessments} color="amber" />
        <SummaryCard label="Upcoming Deadlines" value={dashboard.upcoming_deadlines} color="red" />
        <SummaryCard label="Action Required" value={dashboard.action_required} color="purple" />
      </div>

      {showSources ? (
        /* Sources Management */
        <div className="space-y-3">
          <h2 className="text-lg font-semibold">Subscribed Sources</h2>
          {sources.length === 0 ? (
            <p className="text-gray-500 text-sm">No sources available.</p>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              {sources.map((source) => (
                <div key={source.id} className="border rounded-lg p-4 bg-white shadow-sm flex items-center justify-between">
                  <div>
                    <p className="font-semibold text-gray-900">{source.name}</p>
                    <p className="text-xs text-gray-500">{source.type} &middot; {source.region}</p>
                  </div>
                  <button
                    onClick={() => subscribeMutation.mutate({ source_id: source.id, subscribed: !source.subscribed })}
                    className={`px-3 py-1.5 text-sm font-medium rounded ${
                      source.subscribed
                        ? 'border border-red-300 text-red-600 hover:bg-red-50'
                        : 'bg-blue-600 text-white hover:bg-blue-700'
                    }`}
                  >
                    {source.subscribed ? 'Unsubscribe' : 'Subscribe'}
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      ) : (
        <>
          {/* Filters */}
          <div className="flex flex-wrap gap-3">
            <select
              value={severityFilter}
              onChange={(e) => setSeverityFilter(e.target.value)}
              className="border rounded px-3 py-2 text-sm"
            >
              {SEVERITY_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="border rounded px-3 py-2 text-sm"
            >
              {STATUS_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
            {sources.length > 0 && (
              <select
                value={sourceFilter}
                onChange={(e) => setSourceFilter(e.target.value)}
                className="border rounded px-3 py-2 text-sm"
              >
                <option value="">All Sources</option>
                {sources.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            )}
            <select
              value={regionFilter}
              onChange={(e) => setRegionFilter(e.target.value)}
              className="border rounded px-3 py-2 text-sm"
            >
              <option value="">All Regions</option>
              <option value="eu">EU</option>
              <option value="uk">UK</option>
              <option value="us">US</option>
              <option value="global">Global</option>
            </select>
          </div>

          {/* Change Feed */}
          {isLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="h-28 rounded-lg bg-gray-100 animate-pulse" />
              ))}
            </div>
          ) : changes.length === 0 ? (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No regulatory changes found</p>
              <p className="text-sm mt-1">Adjust your filters or subscribe to more sources</p>
            </div>
          ) : (
            <div className="space-y-3">
              {changes.map((change) => (
                <div key={change.id} className="border rounded-lg p-4 bg-white shadow-sm hover:shadow-md transition-shadow">
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1 flex-wrap">
                        <SeverityBadge severity={change.severity} />
                        <ChangeStatusBadge status={change.status} />
                        <span className="text-xs text-gray-400">
                          {new Date(change.published_date).toLocaleDateString()}
                        </span>
                      </div>
                      <p className="font-semibold text-gray-900">{change.title}</p>
                      <p className="text-sm text-gray-600 mt-1 line-clamp-2">{change.summary}</p>
                      <div className="flex items-center gap-2 mt-2 flex-wrap">
                        <span className="text-xs text-gray-400">Source: {change.source}</span>
                        {change.frameworks.map((fw) => (
                          <span key={fw} className="text-xs bg-blue-50 text-blue-600 px-2 py-0.5 rounded">{fw}</span>
                        ))}
                        {change.regions?.map((r) => (
                          <span key={r} className="text-xs bg-gray-100 text-gray-500 px-2 py-0.5 rounded">{r}</span>
                        ))}
                      </div>
                      {change.effective_date && (
                        <p className="text-xs text-gray-400 mt-1">
                          Effective: {new Date(change.effective_date).toLocaleDateString()}
                        </p>
                      )}
                    </div>
                    <div className="flex flex-col gap-2 shrink-0">
                      {change.impact_score !== undefined && (
                        <div className="text-center">
                          <p className="text-xs text-gray-400">Impact</p>
                          <p className={`text-lg font-bold ${
                            change.impact_score >= 8 ? 'text-red-600'
                              : change.impact_score >= 5 ? 'text-amber-600'
                              : 'text-green-600'
                          }`}>
                            {change.impact_score}/10
                          </p>
                        </div>
                      )}
                      {(change.status === 'new' || change.status === 'under_review') && (
                        <button
                          onClick={() => assessMutation.mutate(change.id)}
                          disabled={assessMutation.isPending}
                          className="px-3 py-1.5 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                        >
                          Assess Impact
                        </button>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
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
    purple: 'bg-purple-50 text-purple-700 border-purple-200',
  };
  return (
    <div className={`rounded-lg border p-4 ${colorMap[color] ?? colorMap.blue}`}>
      <p className="text-sm font-medium opacity-80">{label}</p>
      <p className="text-3xl font-bold mt-1">{value}</p>
    </div>
  );
}

function SeverityBadge({ severity }: { severity: string }) {
  const styles: Record<string, string> = {
    critical: 'bg-red-100 text-red-700',
    high: 'bg-orange-100 text-orange-700',
    medium: 'bg-amber-100 text-amber-700',
    low: 'bg-gray-100 text-gray-600',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[severity] ?? styles.medium}`}>
      {severity}
    </span>
  );
}

function ChangeStatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    new: 'bg-blue-100 text-blue-700',
    under_review: 'bg-amber-100 text-amber-700',
    assessed: 'bg-purple-100 text-purple-700',
    action_required: 'bg-red-100 text-red-700',
    resolved: 'bg-green-100 text-green-700',
    dismissed: 'bg-gray-100 text-gray-400',
  };
  const labels: Record<string, string> = {
    new: 'New',
    under_review: 'Under Review',
    assessed: 'Assessed',
    action_required: 'Action Required',
    resolved: 'Resolved',
    dismissed: 'Dismissed',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[status] ?? styles.new}`}>
      {labels[status] ?? status}
    </span>
  );
}
