'use client';

import { ResourceState, StaleDataNotice } from '@/components/data/resource-state';
import { useCallback, useEffect, useRef, useState } from 'react';
import api from '@/lib/api';
import Image from 'next/image';
import { isForbiddenResourceError } from '@/lib/resource-errors';
import Link from 'next/link';

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

interface ActivityListResponse {
  items?: ActivityItem[];
  data?: ActivityItem[];
  total?: number;
  pagination?: { total_items?: number };
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
  const [forbidden, setForbidden] = useState(false);
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
      setForbidden(false);
      try {
        const params: Record<string, unknown> = { page: p, page_size: 30 };
        if (filters.entity_type) params.entity_type = filters.entity_type;
        if (filters.action) params.action = filters.action;
        if (filters.user) params.user_name = filters.user;

        const data = await api.activity.list(params) as ActivityItem[] | ActivityListResponse;
        const fetched = Array.isArray(data) ? data : data.items ?? data.data ?? [];
        const total = Array.isArray(data)
          ? data.length
          : data.total ?? data.pagination?.total_items ?? fetched.length;

        if (append) {
          setItems((prev) => [...prev, ...fetched]);
        } else {
          setItems(fetched);
        }
        setHasMore(p * 30 < total);
      } catch (cause: unknown) {
        if (isForbiddenResourceError(cause)) {
          setItems([]);
          setForbidden(true);
        }
        setError('Failed to load activity feed.');
      } finally {
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [filters]
  );

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setPage(1);
      void fetchActivities(1, false);
    }, 0);
    return () => window.clearTimeout(timer);
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
    } catch {
      setError('Activity was loaded, but the read state could not be updated.');
    }
  };

  const unreadCount = items.filter((i) => !i.is_read).length;

  const entityLink = (item: ActivityItem) => {
    const base = item.entity_type === 'control' ? 'frameworks' : item.entity_type + 's';
    return `/${base}/${item.entity_id}`;
  };

  if (loading) {
    return (
      <ResourceState kind="loading" loadingLayout="table" title="Loading activity feed" />
    );
  }

  if (error && items.length === 0) {
    if (forbidden) return <ResourceState kind="forbidden" title="Activity feed access unavailable" />;
    return (
      <ResourceState
        kind="error"
        title="Activity feed could not be loaded"
        description={error}
        onRetry={() => void fetchActivities(1)}
      />
    );
  }

  return (
    <div className="p-6 space-y-6">
      {error && items.length > 0 && (
        <StaleDataNotice
          title="Activity may be out of date"
          onRefresh={() => void fetchActivities(1)}
        />
      )}
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
            type="button"
            onClick={markAllRead}
            className="min-h-11 rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            Mark all read
          </button>
        )}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 bg-white border border-gray-200 rounded-xl p-4">
        <label htmlFor="activity-entity-type" className="sr-only">Filter by entity type</label>
        <select
          id="activity-entity-type"
          aria-label="Filter by entity type"
          value={filters.entity_type}
          onChange={(e) => setFilters((f) => ({ ...f, entity_type: e.target.value }))}
          className="min-h-11 rounded-lg border border-gray-300 px-3 py-2 text-sm"
        >
          <option value="">All Entity Types</option>
          {ENTITY_TYPE_OPTIONS.map((t) => (
            <option key={t} value={t} className="capitalize">{t}</option>
          ))}
        </select>
        <label htmlFor="activity-action" className="sr-only">Filter by action</label>
        <select
          id="activity-action"
          aria-label="Filter by action"
          value={filters.action}
          onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value }))}
          className="min-h-11 rounded-lg border border-gray-300 px-3 py-2 text-sm"
        >
          <option value="">All Actions</option>
          {ACTION_OPTIONS.map((a) => (
            <option key={a} value={a} className="capitalize">{a}</option>
          ))}
        </select>
        <label htmlFor="activity-user" className="sr-only">Filter by user</label>
        <input
          id="activity-user"
          aria-label="Filter by user"
          type="text"
          value={filters.user}
          onChange={(e) => setFilters((f) => ({ ...f, user: e.target.value }))}
          placeholder="Filter by user..."
          className="min-h-11 w-48 rounded-lg border border-gray-300 px-3 py-2 text-sm"
        />
      </div>

      {/* Timeline */}
      {items.length === 0 ? (
        <ResourceState
          kind="empty"
          title="No activity found"
          description="No events match the current filters."
        />
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
                          <Image
                            unoptimized
                            src={item.user_avatar}
                            alt=""
                            width={32}
                            height={32}
                            className="h-8 w-8 rounded-full"
                          />
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
                          <Link
                            href={entityLink(item)}
                            className="mt-0.5 inline-flex min-h-11 items-center gap-1 text-xs font-medium text-indigo-700 hover:text-indigo-800"
                          >
                            {item.entity_ref && (
                              <span className="font-mono bg-indigo-50 px-1 py-0.5 rounded">{item.entity_ref}</span>
                            )}
                            {item.entity_title && <span className="truncate max-w-[200px]">{item.entity_title}</span>}
                          </Link>
                        )}
                      </div>

                      {/* Time & unread indicator */}
                      <div className="flex items-center gap-2 flex-shrink-0">
                        {!item.is_read && (
                          <span className="inline-flex items-center gap-1 text-xs font-medium text-indigo-700">
                            <span aria-hidden="true" className="h-2 w-2 rounded-full bg-indigo-600" />
                            <span className="sr-only">Unread</span>
                          </span>
                        )}
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
            <div role="status" aria-live="polite" className="flex justify-center py-4">
              <div className="flex items-center gap-2 text-sm text-gray-500">
                <svg aria-hidden="true" className="w-4 h-4 animate-spin motion-reduce:animate-none" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Loading more...
              </div>
            </div>
          )}

          {hasMore && !loadingMore && (
            <div className="flex justify-center py-4">
              <button
                type="button"
                onClick={() => {
                  const nextPage = page + 1;
                  setPage(nextPage);
                  void fetchActivities(nextPage, true);
                }}
                className="min-h-11 rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
              >
                Load more activity
              </button>
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
