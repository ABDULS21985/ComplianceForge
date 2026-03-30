'use client';

import { useState, useEffect } from 'react';
import { useSearchParams } from 'next/navigation';
import { Suspense } from 'react';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BoardPortalData {
  member_name: string;
  organization_name: string;
  compliance_score: number;
  risk_appetite_score: number;
  key_alerts: {
    id: string;
    message: string;
    severity: 'critical' | 'high' | 'medium' | 'low';
    date: string;
  }[];
  pending_decisions: {
    id: string;
    title: string;
    type: string;
    meeting_date: string;
    status: string;
  }[];
  board_packs: {
    id: string;
    name: string;
    meeting_date: string;
    format: string;
    size_kb: number;
  }[];
  decision_follow_ups: {
    id: string;
    title: string;
    decision: string;
    owner: string;
    status: 'not_started' | 'in_progress' | 'completed' | 'overdue';
    due_date?: string;
  }[];
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function GaugeChart({ value, label }: { value: number; label: string }) {
  const pct = Math.min(Math.max(value, 0), 100);
  const color = pct >= 75 ? '#059669' : pct >= 50 ? '#D97706' : '#DC2626';

  return (
    <div className="flex flex-col items-center">
      <svg width="140" height="100" viewBox="0 0 140 100">
        <path
          d="M 20 85 A 50 50 0 1 1 120 85"
          fill="none"
          stroke="#E5E7EB"
          strokeWidth="12"
          strokeLinecap="round"
        />
        <path
          d="M 20 85 A 50 50 0 1 1 120 85"
          fill="none"
          stroke={color}
          strokeWidth="12"
          strokeLinecap="round"
          strokeDasharray={`${Math.PI * 50 * 0.75}`}
          strokeDashoffset={`${Math.PI * 50 * 0.75 * (1 - pct / 100)}`}
        />
        <text x="70" y="75" textAnchor="middle" fill="#111827" fontSize="24" fontWeight="bold">
          {pct}%
        </text>
      </svg>
      <span className="text-sm font-medium text-gray-600 -mt-2">{label}</span>
    </div>
  );
}

function AlertCard({ alert }: { alert: { id: string; message: string; severity: string; date: string } }) {
  const sevColor: Record<string, string> = {
    critical: 'border-l-red-600 bg-red-50',
    high: 'border-l-orange-500 bg-orange-50',
    medium: 'border-l-amber-500 bg-amber-50',
    low: 'border-l-blue-400 bg-blue-50',
  };
  return (
    <div className={`border-l-4 rounded-r p-4 ${sevColor[alert.severity] ?? sevColor.low}`}>
      <p className="text-sm text-gray-800">{alert.message}</p>
      <p className="text-xs text-gray-500 mt-1">{new Date(alert.date).toLocaleDateString()}</p>
    </div>
  );
}

function FollowUpStatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    not_started: 'bg-gray-100 text-gray-600',
    in_progress: 'bg-blue-100 text-blue-700',
    completed: 'bg-green-100 text-green-700',
    overdue: 'bg-red-100 text-red-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[status] ?? 'bg-gray-100 text-gray-700'}`}>
      {status.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Inner Component (uses useSearchParams)
// ---------------------------------------------------------------------------

function BoardPortalInner() {
  const searchParams = useSearchParams();
  const token = searchParams.get('token');

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [data, setData] = useState<BoardPortalData | null>(null);

  useEffect(() => {
    if (!token) {
      setError('Invalid or missing access token. Please use the link provided in your board pack email.');
      setLoading(false);
      return;
    }

    async function fetchPortalData() {
      try {
        const res = await fetch(
          `${process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080/api/v1'}/board-portal?token=${token}`
        );
        if (!res.ok) {
          throw new Error(res.status === 401 ? 'Token expired or invalid' : 'Failed to load portal');
        }
        const json = await res.json();
        setData(json);
      } catch (err: any) {
        setError(err.message ?? 'Failed to load board portal');
      } finally {
        setLoading(false);
      }
    }

    fetchPortalData();
  }, [token]);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="text-center">
          <div className="w-8 h-8 border-4 border-slate-600 border-t-transparent rounded-full animate-spin mx-auto" />
          <p className="mt-4 text-sm text-gray-500">Loading board portal...</p>
        </div>
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="bg-white rounded-lg shadow-lg p-8 max-w-md text-center">
          <div className="w-12 h-12 bg-red-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <span className="text-red-600 text-xl font-bold">!</span>
          </div>
          <h1 className="text-lg font-semibold text-gray-900">Access Error</h1>
          <p className="text-sm text-gray-500 mt-2">{error || 'Unable to load board portal data'}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-50">
      {/* Header */}
      <header className="bg-slate-900 text-white">
        <div className="max-w-6xl mx-auto px-8 py-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-xl font-bold">{data.organization_name}</h1>
              <p className="text-sm text-slate-300 mt-1">Executive Board Portal</p>
            </div>
            <div className="text-right">
              <p className="text-sm text-slate-300">Welcome,</p>
              <p className="font-medium">{data.member_name}</p>
            </div>
          </div>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-8 py-8 space-y-8">
        {/* Gauges Row */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="bg-white rounded-xl shadow-sm border p-6 flex justify-center">
            <GaugeChart value={data.compliance_score} label="Compliance Posture" />
          </div>
          <div className="bg-white rounded-xl shadow-sm border p-6 flex justify-center">
            <GaugeChart value={data.risk_appetite_score} label="Risk Appetite Utilization" />
          </div>
        </div>

        {/* Key Alerts */}
        {data.key_alerts.length > 0 && (
          <div className="bg-white rounded-xl shadow-sm border p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">Key Alerts</h2>
            <div className="space-y-3">
              {data.key_alerts.map((alert) => (
                <AlertCard key={alert.id} alert={alert} />
              ))}
            </div>
          </div>
        )}

        {/* Pending Decisions */}
        {data.pending_decisions.length > 0 && (
          <div className="bg-white rounded-xl shadow-sm border p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">
              Pending Decisions ({data.pending_decisions.length})
            </h2>
            <div className="space-y-3">
              {data.pending_decisions.map((d) => (
                <div key={d.id} className="flex items-center justify-between border-b pb-3 last:border-0">
                  <div>
                    <p className="text-sm font-medium text-gray-900">{d.title}</p>
                    <p className="text-xs text-gray-500">
                      {d.type} | Meeting: {new Date(d.meeting_date).toLocaleDateString()}
                    </p>
                  </div>
                  <span className="text-xs bg-yellow-100 text-yellow-700 px-2 py-0.5 rounded-full font-medium">
                    Pending
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Board Packs */}
        {data.board_packs.length > 0 && (
          <div className="bg-white rounded-xl shadow-sm border p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">Board Packs</h2>
            <div className="space-y-2">
              {data.board_packs.map((pack) => (
                <div key={pack.id} className="flex items-center justify-between border-b pb-3 last:border-0">
                  <div>
                    <p className="text-sm font-medium text-gray-900">{pack.name}</p>
                    <p className="text-xs text-gray-500">
                      Meeting: {new Date(pack.meeting_date).toLocaleDateString()} |{' '}
                      <span className="uppercase font-mono">{pack.format}</span> |{' '}
                      {(pack.size_kb / 1024).toFixed(1)} MB
                    </p>
                  </div>
                  <button className="px-3 py-1.5 text-xs font-medium rounded bg-slate-800 text-white hover:bg-slate-700">
                    Download
                  </button>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Decision Follow-ups */}
        {data.decision_follow_ups.length > 0 && (
          <div className="bg-white rounded-xl shadow-sm border p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">Decision Follow-up Status</h2>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left">
                    <th className="pb-3 font-semibold text-gray-700">Decision</th>
                    <th className="pb-3 font-semibold text-gray-700">Owner</th>
                    <th className="pb-3 font-semibold text-gray-700">Status</th>
                    <th className="pb-3 font-semibold text-gray-700">Due</th>
                  </tr>
                </thead>
                <tbody>
                  {data.decision_follow_ups.map((fu) => (
                    <tr key={fu.id} className="border-b last:border-0">
                      <td className="py-3">
                        <p className="font-medium text-gray-900">{fu.title}</p>
                        <p className="text-xs text-gray-500 line-clamp-1">{fu.decision}</p>
                      </td>
                      <td className="py-3 text-gray-600">{fu.owner}</td>
                      <td className="py-3">
                        <FollowUpStatusBadge status={fu.status} />
                      </td>
                      <td className="py-3">
                        {fu.due_date ? (
                          <span
                            className={`text-xs ${
                              fu.status === 'overdue' ? 'text-red-600 font-semibold' : 'text-gray-500'
                            }`}
                          >
                            {new Date(fu.due_date).toLocaleDateString()}
                          </span>
                        ) : (
                          <span className="text-xs text-gray-400">--</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="text-center text-xs text-gray-400 py-4">
          This portal is read-only. For questions, contact your compliance team.
        </div>
      </main>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page (Suspense wrapper for useSearchParams)
// ---------------------------------------------------------------------------

export default function BoardPortalPage() {
  return (
    <Suspense
      fallback={
        <div className="min-h-screen flex items-center justify-center bg-slate-50">
          <div className="w-8 h-8 border-4 border-slate-600 border-t-transparent rounded-full animate-spin" />
        </div>
      }
    >
      <BoardPortalInner />
    </Suspense>
  );
}
