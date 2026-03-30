'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface CalendarEvent {
  id: string;
  title: string;
  date: string;
  due_date?: string;
  type: 'audit' | 'policy_review' | 'risk_assessment' | 'control_test' | 'training' | 'vendor_review' | 'compliance_deadline' | 'other';
  category: string;
  priority: 'critical' | 'high' | 'medium' | 'low';
  status: 'pending' | 'in_progress' | 'completed' | 'overdue';
  entity_ref?: string;
  entity_id?: string;
  assignee?: string;
  description?: string;
}

type ViewMode = 'month' | 'week' | 'agenda';

const PRIORITY_COLORS: Record<string, string> = {
  critical: 'bg-red-500',
  high: 'bg-orange-500',
  medium: 'bg-blue-500',
  low: 'bg-green-500',
};

const PRIORITY_TEXT: Record<string, string> = {
  critical: 'text-red-700 bg-red-50 border-red-200',
  high: 'text-orange-700 bg-orange-50 border-orange-200',
  medium: 'text-blue-700 bg-blue-50 border-blue-200',
  low: 'text-green-700 bg-green-50 border-green-200',
};

const TYPE_LABELS: Record<string, string> = {
  audit: 'Audit',
  policy_review: 'Policy Review',
  risk_assessment: 'Risk Assessment',
  control_test: 'Control Test',
  training: 'Training',
  vendor_review: 'Vendor Review',
  compliance_deadline: 'Compliance Deadline',
  other: 'Other',
};

const STATUS_STYLES: Record<string, string> = {
  pending: 'text-gray-700 bg-gray-100',
  in_progress: 'text-blue-700 bg-blue-100',
  completed: 'text-green-700 bg-green-100',
  overdue: 'text-red-700 bg-red-100',
};

const DAYS_OF_WEEK = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getDaysInMonth(year: number, month: number): Date[] {
  const days: Date[] = [];
  const firstDay = new Date(year, month, 1);
  const lastDay = new Date(year, month + 1, 0);
  const startPad = firstDay.getDay();

  for (let i = startPad - 1; i >= 0; i--) {
    days.push(new Date(year, month, -i));
  }
  for (let d = 1; d <= lastDay.getDate(); d++) {
    days.push(new Date(year, month, d));
  }
  const remaining = 7 - (days.length % 7);
  if (remaining < 7) {
    for (let i = 1; i <= remaining; i++) {
      days.push(new Date(year, month + 1, i));
    }
  }
  return days;
}

function getWeekDays(date: Date): Date[] {
  const days: Date[] = [];
  const dayOfWeek = date.getDay();
  const start = new Date(date);
  start.setDate(date.getDate() - dayOfWeek);
  for (let i = 0; i < 7; i++) {
    const d = new Date(start);
    d.setDate(start.getDate() + i);
    days.push(d);
  }
  return days;
}

function formatDate(d: Date): string {
  return d.toISOString().split('T')[0];
}

function daysUntil(dateStr: string): number {
  const now = new Date();
  now.setHours(0, 0, 0, 0);
  const target = new Date(dateStr);
  target.setHours(0, 0, 0, 0);
  return Math.ceil((target.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
}

function relativeTime(dateStr: string): string {
  const days = daysUntil(dateStr);
  if (days < 0) return `${Math.abs(days)}d overdue`;
  if (days === 0) return 'Today';
  if (days === 1) return 'Tomorrow';
  return `${days}d left`;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function CalendarPage() {
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>('month');
  const [currentDate, setCurrentDate] = useState(new Date());
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const [agendaFilter, setAgendaFilter] = useState({ type: '', priority: '', status: '' });

  // Fetch events
  const fetchEvents = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const year = currentDate.getFullYear();
      const month = currentDate.getMonth();
      const from = new Date(year, month - 1, 1).toISOString().split('T')[0];
      const to = new Date(year, month + 2, 0).toISOString().split('T')[0];
      const data = await api.calendar.list({ from_date: from, to_date: to });
      setEvents(Array.isArray(data) ? data : (data as any).items ?? []);
    } catch {
      setError('Failed to load calendar events. Please try again.');
    } finally {
      setLoading(false);
    }
  }, [currentDate]);

  useEffect(() => { fetchEvents(); }, [fetchEvents]);

  // Events by date map
  const eventsByDate = useMemo(() => {
    const map: Record<string, CalendarEvent[]> = {};
    events.forEach((ev) => {
      const key = (ev.due_date ?? ev.date).slice(0, 10);
      if (!map[key]) map[key] = [];
      map[key].push(ev);
    });
    return map;
  }, [events]);

  // Deadlines
  const now = new Date();
  const todayStr = formatDate(now);
  const next7 = events
    .filter((e) => {
      const d = daysUntil(e.due_date ?? e.date);
      return d >= 0 && d <= 7 && e.status !== 'completed';
    })
    .sort((a, b) => (a.due_date ?? a.date).localeCompare(b.due_date ?? b.date));
  const overdue = events
    .filter((e) => {
      const d = daysUntil(e.due_date ?? e.date);
      return d < 0 && e.status !== 'completed';
    })
    .sort((a, b) => (a.due_date ?? a.date).localeCompare(b.due_date ?? b.date));

  // Navigation
  const navigate = (dir: number) => {
    const d = new Date(currentDate);
    if (viewMode === 'month') d.setMonth(d.getMonth() + dir);
    else if (viewMode === 'week') d.setDate(d.getDate() + dir * 7);
    else d.setMonth(d.getMonth() + dir);
    setCurrentDate(d);
    setSelectedDay(null);
  };

  const goToday = () => {
    setCurrentDate(new Date());
    setSelectedDay(null);
  };

  // Toggle completion
  const toggleComplete = async (ev: CalendarEvent) => {
    try {
      const newStatus = ev.status === 'completed' ? 'pending' : 'completed';
      await api.calendar.update(ev.id, { status: newStatus });
      setEvents((prev) =>
        prev.map((e) => (e.id === ev.id ? { ...e, status: newStatus } : e))
      );
    } catch {
      // silently fail
    }
  };

  // Selected day events
  const selectedEvents = selectedDay ? (eventsByDate[selectedDay] ?? []) : [];

  // Agenda filtered events
  const agendaEvents = useMemo(() => {
    let filtered = [...events].sort((a, b) =>
      (a.due_date ?? a.date).localeCompare(b.due_date ?? b.date)
    );
    if (agendaFilter.type) filtered = filtered.filter((e) => e.type === agendaFilter.type);
    if (agendaFilter.priority) filtered = filtered.filter((e) => e.priority === agendaFilter.priority);
    if (agendaFilter.status) filtered = filtered.filter((e) => e.status === agendaFilter.status);
    return filtered;
  }, [events, agendaFilter]);

  // Group agenda by day
  const agendaGrouped = useMemo(() => {
    const groups: Record<string, CalendarEvent[]> = {};
    agendaEvents.forEach((ev) => {
      const key = (ev.due_date ?? ev.date).slice(0, 10);
      if (!groups[key]) groups[key] = [];
      groups[key].push(ev);
    });
    return groups;
  }, [agendaEvents]);

  // ----- Render -----

  if (loading) {
    return (
      <div className="p-6 space-y-6 animate-pulse">
        <div className="h-8 bg-gray-200 rounded w-48" />
        <div className="h-96 bg-gray-100 rounded-xl" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-red-50 border border-red-200 text-red-700 p-4 rounded-lg">
          <p className="font-semibold">Error</p>
          <p className="text-sm mt-1">{error}</p>
          <button onClick={fetchEvents} className="mt-3 text-sm font-medium text-red-700 underline">
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Compliance Calendar</h1>
          <p className="text-sm text-gray-500 mt-1">Track deadlines, reviews, and compliance activities</p>
        </div>
        <div className="flex items-center gap-2">
          {/* View tabs */}
          <div className="flex bg-gray-100 rounded-lg p-1">
            {(['month', 'week', 'agenda'] as ViewMode[]).map((v) => (
              <button
                key={v}
                onClick={() => setViewMode(v)}
                className={`px-3 py-1.5 text-sm font-medium rounded-md capitalize transition-colors ${
                  viewMode === v ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'
                }`}
              >
                {v}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="flex flex-col lg:flex-row gap-6">
        {/* Main calendar area */}
        <div className="flex-1">
          {/* Navigation */}
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <button onClick={() => navigate(-1)} className="p-2 rounded-lg hover:bg-gray-100 text-gray-600">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
              </button>
              <h2 className="text-lg font-semibold text-gray-900 min-w-[200px] text-center">
                {viewMode === 'week'
                  ? (() => {
                      const wk = getWeekDays(currentDate);
                      return `${wk[0].toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })} - ${wk[6].toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })}`;
                    })()
                  : `${MONTHS[currentDate.getMonth()]} ${currentDate.getFullYear()}`}
              </h2>
              <button onClick={() => navigate(1)} className="p-2 rounded-lg hover:bg-gray-100 text-gray-600">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" /></svg>
              </button>
              <button onClick={goToday} className="ml-2 px-3 py-1.5 text-sm bg-indigo-50 text-indigo-700 font-medium rounded-lg hover:bg-indigo-100">
                Today
              </button>
            </div>
            {/* Legend */}
            <div className="hidden md:flex items-center gap-3 text-xs text-gray-500">
              {Object.entries(PRIORITY_COLORS).map(([k, color]) => (
                <span key={k} className="flex items-center gap-1">
                  <span className={`w-2.5 h-2.5 rounded-full ${color}`} />
                  <span className="capitalize">{k}</span>
                </span>
              ))}
            </div>
          </div>

          {/* MONTH VIEW */}
          {viewMode === 'month' && (
            <div className="bg-white border border-gray-200 rounded-xl overflow-hidden">
              <div className="grid grid-cols-7">
                {DAYS_OF_WEEK.map((d) => (
                  <div key={d} className="py-2 text-center text-xs font-semibold text-gray-500 border-b border-gray-200 bg-gray-50">
                    {d}
                  </div>
                ))}
              </div>
              <div className="grid grid-cols-7">
                {getDaysInMonth(currentDate.getFullYear(), currentDate.getMonth()).map((day, idx) => {
                  const key = formatDate(day);
                  const isCurrentMonth = day.getMonth() === currentDate.getMonth();
                  const isToday = key === todayStr;
                  const dayEvents = eventsByDate[key] ?? [];
                  const isSelected = selectedDay === key;

                  return (
                    <button
                      key={idx}
                      onClick={() => setSelectedDay(isSelected ? null : key)}
                      className={`min-h-[80px] p-1.5 border-b border-r border-gray-100 text-left transition-colors hover:bg-gray-50 ${
                        !isCurrentMonth ? 'bg-gray-50/50' : ''
                      } ${isSelected ? 'ring-2 ring-indigo-500 ring-inset bg-indigo-50/30' : ''}`}
                    >
                      <div className="flex items-center justify-between">
                        <span
                          className={`text-sm font-medium w-7 h-7 flex items-center justify-center rounded-full ${
                            isToday ? 'bg-indigo-600 text-white' : isCurrentMonth ? 'text-gray-900' : 'text-gray-400'
                          }`}
                        >
                          {day.getDate()}
                        </span>
                        {dayEvents.length > 0 && (
                          <span className="text-[10px] text-gray-400 font-medium">{dayEvents.length}</span>
                        )}
                      </div>
                      {dayEvents.length > 0 && (
                        <div className="mt-1 flex flex-wrap gap-1">
                          {dayEvents.slice(0, 4).map((ev) => (
                            <span key={ev.id} className={`w-2 h-2 rounded-full ${PRIORITY_COLORS[ev.priority]}`} title={ev.title} />
                          ))}
                          {dayEvents.length > 4 && (
                            <span className="text-[9px] text-gray-400">+{dayEvents.length - 4}</span>
                          )}
                        </div>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {/* WEEK VIEW */}
          {viewMode === 'week' && (
            <div className="bg-white border border-gray-200 rounded-xl overflow-hidden">
              <div className="grid grid-cols-7">
                {getWeekDays(currentDate).map((day, idx) => {
                  const key = formatDate(day);
                  const isToday = key === todayStr;
                  const dayEvents = eventsByDate[key] ?? [];

                  return (
                    <div key={idx} className="border-r border-gray-100 last:border-r-0">
                      <div className={`py-2 px-2 text-center border-b border-gray-200 ${isToday ? 'bg-indigo-50' : 'bg-gray-50'}`}>
                        <div className="text-xs text-gray-500">{DAYS_OF_WEEK[day.getDay()]}</div>
                        <div className={`text-lg font-semibold ${isToday ? 'text-indigo-600' : 'text-gray-900'}`}>{day.getDate()}</div>
                      </div>
                      <div className="min-h-[300px] p-1.5 space-y-1">
                        {dayEvents.map((ev) => (
                          <button
                            key={ev.id}
                            onClick={() => setSelectedDay(key)}
                            className={`w-full text-left p-1.5 rounded text-xs border ${PRIORITY_TEXT[ev.priority]} hover:shadow-sm transition-shadow`}
                          >
                            <div className="font-medium truncate">{ev.title}</div>
                            <div className="text-[10px] opacity-70 mt-0.5">{TYPE_LABELS[ev.type] ?? ev.type}</div>
                          </button>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* AGENDA VIEW */}
          {viewMode === 'agenda' && (
            <div className="space-y-4">
              {/* Filters */}
              <div className="flex flex-wrap gap-3 bg-white border border-gray-200 rounded-xl p-4">
                <select
                  value={agendaFilter.type}
                  onChange={(e) => setAgendaFilter((f) => ({ ...f, type: e.target.value }))}
                  className="text-sm border border-gray-300 rounded-lg px-3 py-1.5"
                >
                  <option value="">All Types</option>
                  {Object.entries(TYPE_LABELS).map(([k, v]) => (
                    <option key={k} value={k}>{v}</option>
                  ))}
                </select>
                <select
                  value={agendaFilter.priority}
                  onChange={(e) => setAgendaFilter((f) => ({ ...f, priority: e.target.value }))}
                  className="text-sm border border-gray-300 rounded-lg px-3 py-1.5"
                >
                  <option value="">All Priorities</option>
                  <option value="critical">Critical</option>
                  <option value="high">High</option>
                  <option value="medium">Medium</option>
                  <option value="low">Low</option>
                </select>
                <select
                  value={agendaFilter.status}
                  onChange={(e) => setAgendaFilter((f) => ({ ...f, status: e.target.value }))}
                  className="text-sm border border-gray-300 rounded-lg px-3 py-1.5"
                >
                  <option value="">All Statuses</option>
                  <option value="pending">Pending</option>
                  <option value="in_progress">In Progress</option>
                  <option value="completed">Completed</option>
                  <option value="overdue">Overdue</option>
                </select>
              </div>

              {/* Grouped list */}
              {Object.keys(agendaGrouped).length === 0 ? (
                <div className="bg-white border border-gray-200 rounded-xl p-8 text-center text-gray-500">
                  No events match the current filters.
                </div>
              ) : (
                Object.entries(agendaGrouped).map(([dateKey, evs]) => {
                  const d = new Date(dateKey + 'T00:00:00');
                  const isPast = dateKey < todayStr;
                  return (
                    <div key={dateKey}>
                      <div className={`text-sm font-semibold mb-2 ${isPast ? 'text-red-600' : dateKey === todayStr ? 'text-indigo-600' : 'text-gray-700'}`}>
                        {d.toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}
                        {dateKey === todayStr && <span className="ml-2 text-xs bg-indigo-100 text-indigo-700 px-2 py-0.5 rounded-full">Today</span>}
                        {isPast && <span className="ml-2 text-xs bg-red-100 text-red-700 px-2 py-0.5 rounded-full">Past</span>}
                      </div>
                      <div className="space-y-2">
                        {evs.map((ev) => (
                          <div key={ev.id} className="bg-white border border-gray-200 rounded-lg p-3 flex items-center gap-3 hover:shadow-sm transition-shadow">
                            <input
                              type="checkbox"
                              checked={ev.status === 'completed'}
                              onChange={() => toggleComplete(ev)}
                              className="w-4 h-4 rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
                            />
                            <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${PRIORITY_COLORS[ev.priority]}`} />
                            <div className="flex-1 min-w-0">
                              <div className={`text-sm font-medium ${ev.status === 'completed' ? 'line-through text-gray-400' : 'text-gray-900'}`}>
                                {ev.title}
                              </div>
                              {ev.description && <div className="text-xs text-gray-500 mt-0.5 truncate">{ev.description}</div>}
                            </div>
                            <span className="text-xs px-2 py-0.5 rounded-full border bg-gray-50 text-gray-600 flex-shrink-0">
                              {TYPE_LABELS[ev.type] ?? ev.type}
                            </span>
                            <span className={`text-xs px-2 py-0.5 rounded-full capitalize flex-shrink-0 ${PRIORITY_TEXT[ev.priority]}`}>
                              {ev.priority}
                            </span>
                            <span className={`text-xs px-2 py-0.5 rounded-full flex-shrink-0 ${STATUS_STYLES[ev.status] ?? ''}`}>
                              {ev.status.replace('_', ' ')}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          )}
        </div>

        {/* Side panel: day detail or deadline dashboard */}
        <div className="w-full lg:w-80 space-y-4">
          {/* Selected day panel */}
          {selectedDay && (
            <div className="bg-white border border-gray-200 rounded-xl p-4">
              <div className="flex items-center justify-between mb-3">
                <h3 className="font-semibold text-gray-900">
                  {new Date(selectedDay + 'T00:00:00').toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short' })}
                </h3>
                <button onClick={() => setSelectedDay(null)} className="text-gray-400 hover:text-gray-600">
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
                </button>
              </div>
              {selectedEvents.length === 0 ? (
                <p className="text-sm text-gray-500">No events on this day.</p>
              ) : (
                <div className="space-y-2">
                  {selectedEvents.map((ev) => (
                    <div key={ev.id} className="p-2.5 rounded-lg border border-gray-100 bg-gray-50">
                      <div className="flex items-center gap-2">
                        <span className={`w-2 h-2 rounded-full ${PRIORITY_COLORS[ev.priority]}`} />
                        <span className="text-sm font-medium text-gray-900 truncate">{ev.title}</span>
                      </div>
                      <div className="mt-1 flex flex-wrap gap-1">
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-200 text-gray-600">
                          {TYPE_LABELS[ev.type] ?? ev.type}
                        </span>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded capitalize ${PRIORITY_TEXT[ev.priority]}`}>
                          {ev.priority}
                        </span>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded ${STATUS_STYLES[ev.status]}`}>
                          {ev.status.replace('_', ' ')}
                        </span>
                      </div>
                      {ev.assignee && <div className="text-[10px] text-gray-400 mt-1">Assigned: {ev.assignee}</div>}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Deadline Dashboard */}
          <div className="bg-white border border-gray-200 rounded-xl p-4">
            <h3 className="font-semibold text-gray-900 mb-3">Deadline Dashboard</h3>

            {/* Overdue */}
            {overdue.length > 0 && (
              <div className="mb-4">
                <div className="flex items-center gap-2 mb-2">
                  <span className="w-2 h-2 rounded-full bg-red-500" />
                  <span className="text-sm font-medium text-red-700">Overdue ({overdue.length})</span>
                </div>
                <div className="space-y-1.5">
                  {overdue.slice(0, 5).map((ev) => (
                    <div key={ev.id} className="flex items-center justify-between p-2 rounded-lg bg-red-50 border border-red-100">
                      <div className="min-w-0 flex-1">
                        <div className="text-xs font-medium text-red-800 truncate">{ev.title}</div>
                        <div className="text-[10px] text-red-600">{ev.entity_ref}</div>
                      </div>
                      <span className="text-xs font-bold text-red-700 flex-shrink-0 ml-2">
                        {relativeTime(ev.due_date ?? ev.date)}
                      </span>
                    </div>
                  ))}
                  {overdue.length > 5 && (
                    <p className="text-[10px] text-red-500 text-center">+{overdue.length - 5} more overdue</p>
                  )}
                </div>
              </div>
            )}

            {/* Next 7 Days */}
            <div>
              <div className="flex items-center gap-2 mb-2">
                <span className="w-2 h-2 rounded-full bg-indigo-500" />
                <span className="text-sm font-medium text-gray-700">Next 7 Days ({next7.length})</span>
              </div>
              {next7.length === 0 ? (
                <p className="text-xs text-gray-400">No upcoming deadlines.</p>
              ) : (
                <div className="space-y-1.5">
                  {next7.slice(0, 8).map((ev) => (
                    <div key={ev.id} className="flex items-center justify-between p-2 rounded-lg bg-gray-50 border border-gray-100">
                      <div className="min-w-0 flex-1">
                        <div className="text-xs font-medium text-gray-800 truncate">{ev.title}</div>
                        <div className="flex items-center gap-1 mt-0.5">
                          <span className={`w-1.5 h-1.5 rounded-full ${PRIORITY_COLORS[ev.priority]}`} />
                          <span className="text-[10px] text-gray-500">{TYPE_LABELS[ev.type] ?? ev.type}</span>
                        </div>
                      </div>
                      <span className={`text-xs font-semibold flex-shrink-0 ml-2 ${
                        daysUntil(ev.due_date ?? ev.date) <= 1 ? 'text-red-600' : daysUntil(ev.due_date ?? ev.date) <= 3 ? 'text-orange-600' : 'text-gray-600'
                      }`}>
                        {relativeTime(ev.due_date ?? ev.date)}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
