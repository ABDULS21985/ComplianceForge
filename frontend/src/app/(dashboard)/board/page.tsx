'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BoardMember {
  id: string;
  name: string;
  title: string;
  email: string;
  committees: string[];
  portal_access: boolean;
  last_login?: string;
  status: 'active' | 'inactive';
}

interface BoardMeeting {
  id: string;
  title: string;
  date: string;
  type: 'regular' | 'special' | 'committee' | 'agm';
  status: 'scheduled' | 'in_progress' | 'completed' | 'cancelled';
  attendees: number;
  agenda_items: number;
  minutes_available: boolean;
  board_pack_ready: boolean;
}

interface BoardDecision {
  id: string;
  meeting_id: string;
  meeting_date: string;
  title: string;
  type: 'approval' | 'directive' | 'resolution' | 'action_item';
  decision: string;
  status: 'pending' | 'approved' | 'rejected' | 'deferred';
  follow_up_status: 'not_started' | 'in_progress' | 'completed' | 'overdue';
  follow_up_owner?: string;
  follow_up_due?: string;
}

interface BoardReport {
  id: string;
  name: string;
  type: string;
  period: string;
  generated_date: string;
  format: 'pdf' | 'pptx' | 'xlsx';
  size_kb: number;
}

interface BoardDashboard {
  compliance_score: number;
  risk_appetite_score: number;
  key_alerts: { id: string; message: string; severity: 'critical' | 'high' | 'medium' | 'low'; date: string }[];
  pending_decisions: number;
  next_meeting?: string;
  open_actions: number;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function GaugeChart({ value, label, color }: { value: number; label: string; color: string }) {
  const circumference = 2 * Math.PI * 45;
  const pct = Math.min(Math.max(value, 0), 100);
  const offset = circumference - (pct / 100) * circumference * 0.75; // 270-degree arc
  const colorMap: Record<string, string> = {
    green: '#059669',
    amber: '#D97706',
    red: '#DC2626',
    blue: '#1A56DB',
  };
  const gaugeColor = pct >= 75 ? colorMap.green : pct >= 50 ? colorMap.amber : colorMap.red;

  return (
    <div className="flex flex-col items-center">
      <svg width="120" height="90" viewBox="0 0 120 90">
        <path
          d="M 15 75 A 45 45 0 1 1 105 75"
          fill="none"
          stroke="#E5E7EB"
          strokeWidth="10"
          strokeLinecap="round"
        />
        <path
          d="M 15 75 A 45 45 0 1 1 105 75"
          fill="none"
          stroke={color === 'auto' ? gaugeColor : (colorMap[color] ?? gaugeColor)}
          strokeWidth="10"
          strokeLinecap="round"
          strokeDasharray={`${circumference * 0.75}`}
          strokeDashoffset={offset}
        />
        <text x="60" y="65" textAnchor="middle" className="text-xl font-bold" fill="#111827" fontSize="20">
          {pct}%
        </text>
      </svg>
      <span className="text-xs font-medium text-gray-600 -mt-1">{label}</span>
    </div>
  );
}

function SummaryCard({ label, value, color }: { label: string; value: number | string; color: string }) {
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
      <p className="mt-1 text-2xl font-bold">{value}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    scheduled: 'bg-blue-100 text-blue-700',
    in_progress: 'bg-amber-100 text-amber-700',
    completed: 'bg-green-100 text-green-700',
    cancelled: 'bg-gray-100 text-gray-700',
    pending: 'bg-yellow-100 text-yellow-700',
    approved: 'bg-green-100 text-green-700',
    rejected: 'bg-red-100 text-red-700',
    deferred: 'bg-gray-100 text-gray-600',
    not_started: 'bg-gray-100 text-gray-600',
    overdue: 'bg-red-100 text-red-700',
    active: 'bg-green-100 text-green-700',
    inactive: 'bg-gray-100 text-gray-500',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[status] ?? 'bg-gray-100 text-gray-700'}`}>
      {status.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

function TypeBadge({ type }: { type: string }) {
  const map: Record<string, string> = {
    regular: 'bg-blue-100 text-blue-700',
    special: 'bg-purple-100 text-purple-700',
    committee: 'bg-indigo-100 text-indigo-700',
    agm: 'bg-green-100 text-green-700',
    approval: 'bg-blue-100 text-blue-700',
    directive: 'bg-purple-100 text-purple-700',
    resolution: 'bg-indigo-100 text-indigo-700',
    action_item: 'bg-amber-100 text-amber-700',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${map[type] ?? 'bg-gray-100 text-gray-700'}`}>
      {type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
    </span>
  );
}

function AlertItem({ alert }: { alert: { id: string; message: string; severity: string; date: string } }) {
  const sev: Record<string, string> = {
    critical: 'border-l-red-600 bg-red-50',
    high: 'border-l-orange-500 bg-orange-50',
    medium: 'border-l-amber-500 bg-amber-50',
    low: 'border-l-blue-400 bg-blue-50',
  };
  return (
    <div className={`border-l-4 rounded-r p-3 text-sm ${sev[alert.severity] ?? sev.low}`}>
      <p className="text-gray-800">{alert.message}</p>
      <p className="text-xs text-gray-500 mt-1">{new Date(alert.date).toLocaleDateString()}</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function BoardManagementPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'dashboard' | 'members' | 'meetings' | 'decisions' | 'reports'>('dashboard');

  const { data: dashData } = useQuery<BoardDashboard>({
    queryKey: ['board-dashboard'],
    queryFn: () => api.board.dashboard(),
  });

  const { data: membersData } = useQuery({
    queryKey: ['board-members'],
    queryFn: () => api.board.listMembers(),
    enabled: activeTab === 'members' || activeTab === 'dashboard',
  });

  const { data: meetingsData } = useQuery({
    queryKey: ['board-meetings'],
    queryFn: () => api.board.listMeetings(),
    enabled: activeTab === 'meetings' || activeTab === 'dashboard',
  });

  const { data: decisionsData } = useQuery({
    queryKey: ['board-decisions'],
    queryFn: () => api.board.listDecisions(),
    enabled: activeTab === 'decisions' || activeTab === 'dashboard',
  });

  const { data: reportsData } = useQuery({
    queryKey: ['board-reports'],
    queryFn: () => api.board.listReports(),
    enabled: activeTab === 'reports',
  });

  const generatePackMutation = useMutation({
    mutationFn: (meetingId: string) => api.board.generateBoardPack(meetingId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['board-meetings'] }),
  });

  const members: BoardMember[] = membersData?.items ?? membersData ?? [];
  const meetings: BoardMeeting[] = meetingsData?.items ?? meetingsData ?? [];
  const decisions: BoardDecision[] = decisionsData?.items ?? decisionsData ?? [];
  const reports: BoardReport[] = reportsData?.items ?? reportsData ?? [];

  const dash = dashData ?? {
    compliance_score: 0,
    risk_appetite_score: 0,
    key_alerts: [],
    pending_decisions: decisions.filter((d) => d.status === 'pending').length,
    next_meeting: meetings.filter((m) => m.status === 'scheduled').sort((a, b) => new Date(a.date).getTime() - new Date(b.date).getTime())[0]?.date,
    open_actions: decisions.filter((d) => d.follow_up_status !== 'completed').length,
  };

  const tabs = [
    { key: 'dashboard' as const, label: 'Dashboard' },
    { key: 'members' as const, label: 'Members' },
    { key: 'meetings' as const, label: 'Meetings' },
    { key: 'decisions' as const, label: 'Decisions' },
    { key: 'reports' as const, label: 'Reports' },
  ];

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Board Management</h1>

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

      {/* Dashboard Tab */}
      {activeTab === 'dashboard' && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
            <div className="border rounded-lg p-4 flex justify-center bg-white">
              <GaugeChart value={dash.compliance_score} label="Compliance Score" color="auto" />
            </div>
            <div className="border rounded-lg p-4 flex justify-center bg-white">
              <GaugeChart value={dash.risk_appetite_score} label="Risk Appetite" color="auto" />
            </div>
            <SummaryCard label="Pending Decisions" value={dash.pending_decisions} color="amber" />
            <SummaryCard label="Open Actions" value={dash.open_actions} color="red" />
          </div>

          {dash.next_meeting && (
            <div className="border rounded-lg p-4 bg-blue-50 border-blue-200">
              <p className="text-xs font-medium text-blue-600 uppercase">Next Meeting</p>
              <p className="text-lg font-bold text-blue-800 mt-1">
                {new Date(dash.next_meeting).toLocaleDateString(undefined, {
                  weekday: 'long',
                  year: 'numeric',
                  month: 'long',
                  day: 'numeric',
                })}
              </p>
            </div>
          )}

          {dash.key_alerts.length > 0 && (
            <div className="space-y-2">
              <h2 className="text-sm font-semibold text-gray-700">Key Alerts</h2>
              {dash.key_alerts.map((alert) => (
                <AlertItem key={alert.id} alert={alert} />
              ))}
            </div>
          )}
        </div>
      )}

      {/* Members Tab */}
      {activeTab === 'members' && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-3 font-semibold text-gray-700">Name</th>
                <th className="pb-3 font-semibold text-gray-700">Title</th>
                <th className="pb-3 font-semibold text-gray-700">Committees</th>
                <th className="pb-3 font-semibold text-gray-700">Portal Access</th>
                <th className="pb-3 font-semibold text-gray-700">Last Login</th>
                <th className="pb-3 font-semibold text-gray-700">Status</th>
              </tr>
            </thead>
            <tbody>
              {members.map((m) => (
                <tr key={m.id} className="border-b hover:bg-gray-50">
                  <td className="py-3">
                    <p className="font-medium text-gray-900">{m.name}</p>
                    <p className="text-xs text-gray-500">{m.email}</p>
                  </td>
                  <td className="py-3 text-gray-600">{m.title}</td>
                  <td className="py-3">
                    <div className="flex flex-wrap gap-1">
                      {m.committees.map((c) => (
                        <span key={c} className="text-xs bg-blue-50 text-blue-700 px-2 py-0.5 rounded">
                          {c}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td className="py-3">
                    {m.portal_access ? (
                      <span className="text-xs bg-green-100 text-green-700 px-2 py-0.5 rounded-full font-medium">Enabled</span>
                    ) : (
                      <span className="text-xs bg-gray-100 text-gray-500 px-2 py-0.5 rounded-full">Disabled</span>
                    )}
                  </td>
                  <td className="py-3 text-xs text-gray-500">
                    {m.last_login ? new Date(m.last_login).toLocaleDateString() : 'Never'}
                  </td>
                  <td className="py-3">
                    <StatusBadge status={m.status} />
                  </td>
                </tr>
              ))}
              {members.length === 0 && (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-gray-400">
                    No board members configured
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* Meetings Tab */}
      {activeTab === 'meetings' && (
        <div className="space-y-4">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left">
                  <th className="pb-3 font-semibold text-gray-700">Meeting</th>
                  <th className="pb-3 font-semibold text-gray-700">Date</th>
                  <th className="pb-3 font-semibold text-gray-700">Type</th>
                  <th className="pb-3 font-semibold text-gray-700">Status</th>
                  <th className="pb-3 font-semibold text-gray-700">Attendees</th>
                  <th className="pb-3 font-semibold text-gray-700">Board Pack</th>
                  <th className="pb-3 font-semibold text-gray-700">Actions</th>
                </tr>
              </thead>
              <tbody>
                {meetings.map((m) => (
                  <tr key={m.id} className="border-b hover:bg-gray-50">
                    <td className="py-3">
                      <p className="font-medium text-gray-900">{m.title}</p>
                      <p className="text-xs text-gray-500">{m.agenda_items} agenda items</p>
                    </td>
                    <td className="py-3 text-gray-600">{new Date(m.date).toLocaleDateString()}</td>
                    <td className="py-3">
                      <TypeBadge type={m.type} />
                    </td>
                    <td className="py-3">
                      <StatusBadge status={m.status} />
                    </td>
                    <td className="py-3 text-gray-600">{m.attendees}</td>
                    <td className="py-3">
                      {m.board_pack_ready ? (
                        <span className="text-xs bg-green-100 text-green-700 px-2 py-0.5 rounded-full font-medium">Ready</span>
                      ) : (
                        <span className="text-xs text-gray-400">Not ready</span>
                      )}
                    </td>
                    <td className="py-3">
                      {!m.board_pack_ready && m.status === 'scheduled' && (
                        <button
                          onClick={() => generatePackMutation.mutate(m.id)}
                          disabled={generatePackMutation.isPending}
                          className="text-xs text-blue-600 hover:text-blue-800 font-medium"
                        >
                          Generate Board Pack
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {meetings.length === 0 && (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-gray-400">
                      No meetings scheduled
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Decisions Tab */}
      {activeTab === 'decisions' && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-3 font-semibold text-gray-700">Decision</th>
                <th className="pb-3 font-semibold text-gray-700">Type</th>
                <th className="pb-3 font-semibold text-gray-700">Meeting Date</th>
                <th className="pb-3 font-semibold text-gray-700">Status</th>
                <th className="pb-3 font-semibold text-gray-700">Follow-up</th>
                <th className="pb-3 font-semibold text-gray-700">Owner</th>
                <th className="pb-3 font-semibold text-gray-700">Due</th>
              </tr>
            </thead>
            <tbody>
              {decisions.map((d) => (
                <tr key={d.id} className="border-b hover:bg-gray-50">
                  <td className="py-3">
                    <p className="font-medium text-gray-900">{d.title}</p>
                    <p className="text-xs text-gray-500 line-clamp-1">{d.decision}</p>
                  </td>
                  <td className="py-3">
                    <TypeBadge type={d.type} />
                  </td>
                  <td className="py-3 text-gray-600 text-xs">{new Date(d.meeting_date).toLocaleDateString()}</td>
                  <td className="py-3">
                    <StatusBadge status={d.status} />
                  </td>
                  <td className="py-3">
                    <StatusBadge status={d.follow_up_status} />
                  </td>
                  <td className="py-3 text-gray-600">{d.follow_up_owner ?? '--'}</td>
                  <td className="py-3">
                    {d.follow_up_due ? (
                      <span
                        className={`text-xs ${
                          new Date(d.follow_up_due) < new Date() && d.follow_up_status !== 'completed'
                            ? 'text-red-600 font-semibold'
                            : 'text-gray-500'
                        }`}
                      >
                        {new Date(d.follow_up_due).toLocaleDateString()}
                      </span>
                    ) : (
                      <span className="text-xs text-gray-400">--</span>
                    )}
                  </td>
                </tr>
              ))}
              {decisions.length === 0 && (
                <tr>
                  <td colSpan={7} className="py-8 text-center text-gray-400">
                    No decisions recorded
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* Reports Tab */}
      {activeTab === 'reports' && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-3 font-semibold text-gray-700">Report</th>
                <th className="pb-3 font-semibold text-gray-700">Type</th>
                <th className="pb-3 font-semibold text-gray-700">Period</th>
                <th className="pb-3 font-semibold text-gray-700">Generated</th>
                <th className="pb-3 font-semibold text-gray-700">Format</th>
                <th className="pb-3 font-semibold text-gray-700">Size</th>
                <th className="pb-3 font-semibold text-gray-700">Actions</th>
              </tr>
            </thead>
            <tbody>
              {reports.map((r) => (
                <tr key={r.id} className="border-b hover:bg-gray-50">
                  <td className="py-3 font-medium text-gray-900">{r.name}</td>
                  <td className="py-3 text-gray-600">{r.type}</td>
                  <td className="py-3 text-gray-600">{r.period}</td>
                  <td className="py-3 text-gray-500 text-xs">{new Date(r.generated_date).toLocaleDateString()}</td>
                  <td className="py-3">
                    <span className="text-xs bg-gray-100 text-gray-700 px-2 py-0.5 rounded uppercase font-mono">
                      {r.format}
                    </span>
                  </td>
                  <td className="py-3 text-gray-500 text-xs">{(r.size_kb / 1024).toFixed(1)} MB</td>
                  <td className="py-3">
                    <button className="text-xs text-blue-600 hover:text-blue-800 font-medium">Download</button>
                  </td>
                </tr>
              ))}
              {reports.length === 0 && (
                <tr>
                  <td colSpan={7} className="py-8 text-center text-gray-400">
                    No reports available
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
