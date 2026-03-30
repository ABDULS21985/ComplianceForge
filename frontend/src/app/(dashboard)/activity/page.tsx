'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ActivityItem {
  id: string;
  user_id: string;
  user_name: string;
  user_avatar?: string;
  action: string;
  entity_type: string;
  entity_id: string;
  entity_ref?: string;
  entity_title?: string;
  description: string;
  metadata?: Record<string, unknown>;
  is_read: boolean;
  created_at: string;
}

const ACTION_ICONS: Record<string, { icon: string; color: string }> = {
  created: { icon: '+', color: 'bg-green-100 text-green-600' },
  updated: { icon: '~', color: 'bg-blue-100 text-blue-600' },
  deleted: { icon: 'x', color: 'bg-red-100 text-red-600' },
  approved: { icon: 'v', color: 'bg-emerald-100 text-emerald-600' },
  rejected: { icon: '!', color: 'bg-red-100 text-red-600' },
  commented: { icon: '#', color: 'bg-purple-100 text-purple-600' },
  assigned: { icon: '@', color: 'bg-indigo-100 text-indigo-600' },
  completed: { icon: 'v', color: 'bg-green-100 text-green-600' },
  published: { icon: '^', color: 'bg-blue-100 text-blue-600' },
  archived: { icon: '-', color: 'bg-gray-100 text-gray-600' },
  submitted: { icon: '>', color: 'bg-cyan-100 text-cyan-600' },
  escalated: { icon: '!', color: 'bg-orange-100 text-orange-600' },
};

const ENTITY_TYPE_OPTIONS = [
  'framework', 'control', 'risk', 'policy', 'audit', 'incident',
  'vendor', 'asset', 'evidence', 'workflow', 'exception', 'report',
];

const ACTION_OPTIONS = [
  'created', 'updated', 'deleted', 'approved', 'rejected',
  'commented', 'assigned', 'completed', 'published', 'archived',
  'submitted', 'escalated',
];

function relativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diffSec = Math.floor((now - then) / 1000);
  if (diffSec < 60) return 'just now';
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  if (diffSec < 604800) return `${Math.floor(diffSec / 86400)}d ago`;
  return new Date(dateStr).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
}

function getInitials(name: string): string {
  return name
    .split(' ')
    .map((p) => p[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function ActivityPage() {
  const [items, setItems] = useState<ActivityItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [filters, setFilters] = useState({ entity_type: '', action: '', user: '' });

  const observerRef = useRef<IntersectionObserver | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);

  const fetchActivities = useCallback(
    async (p: number, append: boolean = false) => {
      if (p === 1) setLoading(true);
      else setLoadingMore(true);
      setError(null);
      try {
        const params: Record<string, unknown> = { page: p, page_size: 30 };
        if (filters.entity_type) params.entity_type = filters.entity_type;
        if (filters.action) params.action = filters.action;
        if (filters.user) params.user_name = filters.user;

        const data = await api.activity.list(params) as any;
        const fetched: ActivityItem[] = Array.isArray(data) ? data : data.items ?? [];
        const total = data.total ?? fetched.length;

        if (append) {
          setItems((prev) => [...prev, ...fetched]);
        } else {
          setItems(fetched);
        }
        setHasMore(p * 30 < total);
      } catch {
        setError('Failed to load activity feed.');
      } finally {
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [filters]
  );

  useEffect(() => {
    setPage(1);
    fetchActivities(1, false);
  }, [fetchActivities]);

  // Infinite scroll
  useEffect(() => {
    if (observerRef.current) observerRef.current.disconnect();

    observerRef.current = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore && !loading) {
          const nextPage = page + 1;
          setPage(nextPage);
          fetchActivities(nextPage, true);
        }
      },
      { rootMargin: '200px' }
    );

    if (sentinelRef.current) observerRef.current.observe(sentinelRef.current);

    return () => observerRef.current?.disconnect();
  }, [hasMore, loadingMore, loading, page, fetchActivities]);

  const markAllRead = async () => {
    try {
      await api.activity.markAllRead();
      setItems((prev) => prev.map((item) => ({ ...item, is_read: true })));
    } catch {}
  };

  const unreadCount = items.filter((i) => !i.is_read).length;

  const entityLink = (item: ActivityItem) => {
    const base = item.entity_type === 'control' ? 'frameworks' : item.entity_type + 's';
    return `/${base}/${item.entity_id}`;
  };

  if (loading) {
    return (
      <div className="p-6 space-y-4 animate-pulse">
        <div className="h-8 bg-gray-200 rounded w-48" />
        {[...Array(8)].map((_, i) => (
          <div key={i} className="flex gap-3">
            <div className="w-10 h-10 rounded-full bg-gray-200" />
            <div className="flex-1">
              <div className="h-4 bg-gray-200 rounded w-3/4 mb-1" />
              <div className="h-3 bg-gray-100 rounded w-1/2" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (error && items.length === 0) {
    return (
      <div className="p-6">
        <div className="bg-red-50 border border-red-200 text-red-700 p-4 rounded-xl">
          <p className="font-semibold">Error</p>
          <p className="text-sm mt-1">{error}</p>
          <button onClick={() => fetchActivities(1)} className="mt-2 text-sm font-medium underline">Retry</button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Activity Feed</h1>
          <p className="text-sm text-gray-500 mt-1">
            Track all changes and actions across your GRC platform
            {unreadCount > 0 && (
              <span className="ml-2 inline-flex items-center px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-700 rounded-full">
                {unreadCount} unread
              </span>
            )}
          </p>
        </div>
        {unreadCount > 0 && (
          <button
            onClick={markAllRead}
            className="px-4 py-2 text-sm font-medium bg-white border border-gray-300 rounded-lg hover:bg-gray-50 text-gray-700"
          >
            Mark all read
          </button>
        )}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 bg-white border border-gray-200 rounded-xl p-4">
        <select
          value={filters.entity_type}
          onChange={(e) => setFilters((f) => ({ ...f, entity_type: e.target.value }))}
          className="text-sm border border-gray-300 rounded-lg px-3 py-2"
        >
          <option value="">All Entity Types</option>
          {ENTITY_TYPE_OPTIONS.map((t) => (
            <option key={t} value={t} className="capitalize">{t}</option>
          ))}
        </select>
        <select
          value={filters.action}
          onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value }))}
          className="text-sm border border-gray-300 rounded-lg px-3 py-2"
        >
          <option value="">All Actions</option>
          {ACTION_OPTIONS.map((a) => (
            <option key={a} value={a} className="capitalize">{a}</option>
          ))}
        </select>
        <input
          type="text"
          value={filters.user}
          onChange={(e) => setFilters((f) => ({ ...f, user: e.target.value }))}
          placeholder="Filter by user..."
          className="text-sm border border-gray-300 rounded-lg px-3 py-2 w-48"
        />
      </div>

      {/* Timeline */}
      {items.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-xl p-8 text-center text-gray-500">
          No activity found with the current filters.
        </div>
      ) : (
        <div className="relative">
          {/* Vertical line */}
          <div className="absolute left-5 top-0 bottom-0 w-0.5 bg-gray-200" />

          <div className="space-y-0">
            {items.map((item) => {
              const actionStyle = ACTION_ICONS[item.action] ?? { icon: '?', color: 'bg-gray-100 text-gray-600' };

              return (
                <div
                  key={item.id}
                  className={`relative flex gap-4 py-3 pl-0 pr-4 ${!item.is_read ? 'bg-indigo-50/40 rounded-lg' : ''}`}
                >
                  {/* Dot on timeline */}
                  <div className="relative z-10 flex-shrink-0 w-10 flex items-center justify-center">
                    <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold ${actionStyle.color}`}>
                      {actionStyle.icon}
                    </div>
                  </div>

                  {/* Content */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-start gap-3">
                      {/* Avatar */}
                      <div className="flex-shrink-0">
                        {item.user_avatar ? (
                          <img src={item.user_avatar} alt={item.user_name} className="w-8 h-8 rounded-full" />
                        ) : (
                          <div className="w-8 h-8 rounded-full bg-gray-200 flex items-center justify-center text-xs font-semibold text-gray-600">
                            {getInitials(item.user_name)}
                          </div>
                        )}
                      </div>

                      <div className="flex-1 min-w-0">
                        <p className="text-sm text-gray-800">
                          <span className="font-semibold text-gray-900">{item.user_name}</span>{' '}
                          <span>{item.description}</span>
                        </p>
                        {(item.entity_ref || item.entity_title) && (
                          <a
                            href={entityLink(item)}
                            className="inline-flex items-center gap-1 mt-0.5 text-xs text-indigo-600 hover:text-indigo-700 font-medium"
                          >
                            {item.entity_ref && (
                              <span className="font-mono bg-indigo-50 px-1 py-0.5 rounded">{item.entity_ref}</span>
                            )}
                            {item.entity_title && <span className="truncate max-w-[200px]">{item.entity_title}</span>}
                          </a>
                        )}
                      </div>

                      {/* Time & unread indicator */}
                      <div className="flex items-center gap-2 flex-shrink-0">
                        {!item.is_read && <span className="w-2 h-2 rounded-full bg-indigo-500" />}
                        <span className="text-xs text-gray-400 whitespace-nowrap">{relativeTime(item.created_at)}</span>
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>

          {/* Infinite scroll sentinel */}
          <div ref={sentinelRef} className="h-4" />

          {loadingMore && (
            <div className="flex justify-center py-4">
              <div className="flex items-center gap-2 text-sm text-gray-500">
                <svg className="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Loading more...
              </div>
            </div>
          )}

          {!hasMore && items.length > 0 && (
            <p className="text-center text-xs text-gray-400 py-4">End of activity feed</p>
          )}
        </div>
      )}
    </div>
  );
}
