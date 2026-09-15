'use client';

import {
  getEntityRoute,
  type GlobalSearchResult,
  normalizeGlobalSearchResponse,
} from '@/lib/navigation';
import { ResourceState, StaleDataNotice } from '@/components/data/resource-state';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import api from '@/lib/api';
import { isForbiddenResourceError } from '@/lib/resource-errors';
import Link from 'next/link';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Facets {
  entity_type: string;
  framework: string;
  status: string;
  severity: string;
  date_from: string;
  date_to: string;
}

const EMPTY_FACETS: Facets = {
  entity_type: '',
  framework: '',
  status: '',
  severity: '',
  date_from: '',
  date_to: '',
};

const ENTITY_TYPES = [
  { value: 'framework', label: 'Frameworks', icon: '🛡' },
  { value: 'control', label: 'Controls', icon: '🔧' },
  { value: 'risk', label: 'Risks', icon: '⚠' },
  { value: 'policy', label: 'Policies', icon: '📄' },
  { value: 'audit', label: 'Audits', icon: '📋' },
  { value: 'incident', label: 'Incidents', icon: '🚨' },
  { value: 'vendor', label: 'Vendors', icon: '🏢' },
  { value: 'asset', label: 'Assets', icon: '🖥' },
  { value: 'evidence', label: 'Evidence', icon: '📁' },
];

const ENTITY_ICON_MAP: Record<string, string> = Object.fromEntries(ENTITY_TYPES.map((t) => [t.value, t.icon]));

const STATUS_OPTIONS = ['active', 'draft', 'published', 'archived', 'open', 'closed', 'in_progress', 'completed'];
const SEVERITY_OPTIONS = ['critical', 'high', 'medium', 'low'];

const STATUS_STYLES: Record<string, string> = {
  active: 'bg-green-100 text-green-700',
  draft: 'bg-gray-100 text-gray-600',
  published: 'bg-blue-100 text-blue-700',
  archived: 'bg-gray-100 text-gray-500',
  open: 'bg-yellow-100 text-yellow-700',
  closed: 'bg-gray-100 text-gray-500',
  in_progress: 'bg-blue-100 text-blue-700',
  completed: 'bg-green-100 text-green-700',
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function SearchPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const initialQuery = searchParams.get('q') ?? '';

  const [query, setQuery] = useState(initialQuery);
  const [results, setResults] = useState<GlobalSearchResult[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [page, setPage] = useState(1);
  const [queryTime, setQueryTime] = useState<number | null>(null);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [facets, setFacets] = useState<Facets>(EMPTY_FACETS);

  const inputRef = useRef<HTMLInputElement>(null);

  const doSearch = useCallback(
    async (q: string, p: number, f: Facets) => {
      if (!q.trim()) {
        setResults([]);
        setTotal(0);
        setQueryTime(null);
        setSuggestions([]);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const params: Record<string, unknown> = {
          q: q.trim(),
          page: p,
          page_size: 20,
        };
        if (f.entity_type) params.entity_type = f.entity_type;
        if (f.framework) params.framework = f.framework;
        if (f.status) params.status = f.status;
        if (f.severity) params.severity = f.severity;
        if (f.date_from) params.date_from = f.date_from;
        if (f.date_to) params.date_to = f.date_to;

        const data = normalizeGlobalSearchResponse(
          await api.search.query(params)
        );
        setResults(data.items);
        setTotal(data.total);
        setTotalPages(data.total_pages);
        setQueryTime(data.query_time_ms ?? null);
        setSuggestions(data.suggestions ?? []);
      } catch (cause: unknown) {
        if (isForbiddenResourceError(cause)) {
          setResults([]);
          setForbidden(true);
        }
        setError('Search failed. Please try again.');
      } finally {
        setLoading(false);
      }
    },
    []
  );

  // Keep URL-driven searches synchronized without a synchronous effect update.
  useEffect(() => {
    if (!initialQuery) return;
    const timer = window.setTimeout(
      () => void doSearch(initialQuery, 1, EMPTY_FACETS),
      0,
    );
    return () => window.clearTimeout(timer);
  }, [doSearch, initialQuery]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(1);
    doSearch(query, 1, facets);
    // Update URL
    const params = new URLSearchParams();
    if (query) params.set('q', query);
    router.replace(`/search?${params.toString()}`);
  };

  const handleFacetChange = (key: keyof Facets, value: string) => {
    const updated = { ...facets, [key]: value };
    setFacets(updated);
    setPage(1);
    if (query.trim()) doSearch(query, 1, updated);
  };

  const handlePageChange = (newPage: number) => {
    setPage(newPage);
    doSearch(query, newPage, facets);
    window.scrollTo({
      top: 0,
      behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches
        ? 'auto'
        : 'smooth',
    });
  };

  const clearFilters = () => {
    const cleared: Facets = { ...EMPTY_FACETS };
    setFacets(cleared);
    if (query.trim()) doSearch(query, 1, cleared);
  };

  const hasActiveFilters = Object.values(facets).some(Boolean);

  const highlightSnippet = (snippet: string) => {
    // Simple <em> highlight replacement
    const parts = snippet.split(/(<em>|<\/em>)/);
    let inEm = false;
    return parts.map((part, i) => {
      if (part === '<em>') { inEm = true; return null; }
      if (part === '</em>') { inEm = false; return null; }
      return inEm ? (
        <mark key={i} className="bg-yellow-200 text-yellow-900 px-0.5 rounded">{part}</mark>
      ) : (
        <span key={i}>{part}</span>
      );
    });
  };

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Search ComplianceForge</h1>
        <p className="mt-1 text-sm text-gray-500">
          Find governance records available to your current permissions.
        </p>
      </div>
      {/* Search bar */}
      <form onSubmit={handleSubmit}>
        <div className="relative">
          <svg className="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            ref={inputRef}
            aria-label="Search all compliance data"
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search across all compliance data..."
            className="min-h-11 w-full rounded-xl border border-gray-300 bg-white py-3.5 pl-12 pr-24 text-base shadow-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500"
          />
          <button
            type="submit"
            className="absolute right-2 top-1/2 min-h-11 -translate-y-1/2 rounded-lg bg-indigo-700 px-5 py-2 text-sm font-medium text-white transition-colors motion-reduce:transition-none hover:bg-indigo-800"
          >
            Search
          </button>
        </div>
      </form>

      {error && results.length > 0 && (
        <StaleDataNotice
          title="Showing results from the previous successful search"
          onRefresh={() => void doSearch(query, page, facets)}
        />
      )}

      {/* Results meta */}
      {query.trim() && !loading && !error && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-gray-500">
            {total > 0 ? (
              <>
                <span className="font-semibold text-gray-700">{total.toLocaleString()}</span> results for{' '}
                <span className="font-semibold text-gray-700">&ldquo;{query}&rdquo;</span>
                {queryTime !== null && <span className="ml-1">({queryTime}ms)</span>}
              </>
            ) : (
              <>No results found for <span className="font-semibold text-gray-700">&ldquo;{query}&rdquo;</span></>
            )}
          </p>
          {hasActiveFilters && (
            <button
              type="button"
              onClick={clearFilters}
              className="min-h-11 rounded-md px-2 text-sm font-medium text-indigo-700 hover:text-indigo-800"
            >
              Clear all filters
            </button>
          )}
        </div>
      )}

      <div className="flex flex-col lg:flex-row gap-6">
        {/* Faceted filters sidebar */}
        <div className="w-full lg:w-64 flex-shrink-0 space-y-4">
          <div className="bg-white border border-gray-200 rounded-xl p-4 space-y-4">
            <h3 className="font-semibold text-sm text-gray-900">Filters</h3>

            {/* Entity type */}
            <div>
              <label htmlFor="search-entity-type" className="text-xs font-medium text-gray-600 mb-1.5 block">Entity Type</label>
              <select
                id="search-entity-type"
                value={facets.entity_type}
                onChange={(e) => handleFacetChange('entity_type', e.target.value)}
                className="min-h-11 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
              >
                <option value="">All Types</option>
                {ENTITY_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
            </div>

            {/* Framework */}
            <div>
              <label htmlFor="search-framework" className="text-xs font-medium text-gray-600 mb-1.5 block">Framework</label>
              <input
                id="search-framework"
                type="text"
                value={facets.framework}
                onChange={(e) => handleFacetChange('framework', e.target.value)}
                placeholder="e.g. ISO27001"
                className="min-h-11 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
              />
            </div>

            {/* Status */}
            <div>
              <label htmlFor="search-status" className="text-xs font-medium text-gray-600 mb-1.5 block">Status</label>
              <select
                id="search-status"
                value={facets.status}
                onChange={(e) => handleFacetChange('status', e.target.value)}
                className="min-h-11 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
              >
                <option value="">All</option>
                {STATUS_OPTIONS.map((s) => (
                  <option key={s} value={s}>{s.replace('_', ' ')}</option>
                ))}
              </select>
            </div>

            {/* Severity */}
            <div>
              <label htmlFor="search-severity" className="text-xs font-medium text-gray-600 mb-1.5 block">Severity</label>
              <select
                id="search-severity"
                value={facets.severity}
                onChange={(e) => handleFacetChange('severity', e.target.value)}
                className="min-h-11 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
              >
                <option value="">All</option>
                {SEVERITY_OPTIONS.map((s) => (
                  <option key={s} value={s} className="capitalize">{s}</option>
                ))}
              </select>
            </div>

            {/* Date range */}
            <fieldset>
              <legend className="text-xs font-medium text-gray-600 mb-1.5 block">Date Range</legend>
              <div className="space-y-1.5">
                <label htmlFor="search-date-from" className="sr-only">Updated from</label>
                <input
                  id="search-date-from"
                  aria-label="Updated from"
                  type="date"
                  value={facets.date_from}
                  onChange={(e) => handleFacetChange('date_from', e.target.value)}
                  className="min-h-11 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                />
                <label htmlFor="search-date-to" className="sr-only">Updated through</label>
                <input
                  id="search-date-to"
                  aria-label="Updated through"
                  type="date"
                  value={facets.date_to}
                  onChange={(e) => handleFacetChange('date_to', e.target.value)}
                  className="min-h-11 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                />
              </div>
            </fieldset>
          </div>
        </div>

        {/* Results list */}
        <div className="flex-1">
          {forbidden ? (
            <ResourceState kind="forbidden" surface="plain" title="Search access unavailable" />
          ) : loading && results.length === 0 ? (
            <ResourceState
              surface="plain"
              kind="loading"
              loadingLayout="cards"
              title="Searching governance records"
            />
          ) : error && results.length === 0 ? (
            <ResourceState
              surface="plain"
              kind="error"
              title="Search could not be completed"
              description={error}
              onRetry={() => void doSearch(query, page, facets)}
            />
          ) : results.length === 0 && query.trim() ? (
            <ResourceState
              surface="plain"
              kind="empty"
              title="No results found"
              description="Try different keywords or adjust your filters."
              action={suggestions.length > 0 ? (
                <div className="mt-4">
                  <p className="text-sm text-gray-500 mb-2">Did you mean:</p>
                  <div className="flex flex-wrap justify-center gap-2">
                    {suggestions.map((s) => (
                      <button
                        key={s}
                        type="button"
                        onClick={() => { setQuery(s); doSearch(s, 1, facets); }}
                        className="min-h-11 rounded-full bg-indigo-50 px-3 py-2 text-sm font-medium text-indigo-700 hover:bg-indigo-100 hover:text-indigo-800"
                      >
                        {s}
                      </button>
                    ))}
                  </div>
                </div>
              ) : undefined}
            />
          ) : results.length > 0 ? (
            <>
              <div className="space-y-3">
                {results.map((r) => (
                  <Link
                    key={r.id}
                    href={getEntityRoute(r)}
                    className="block bg-white border border-gray-200 rounded-xl p-4 hover:shadow-md hover:border-indigo-200 transition-all"
                  >
                    <div className="flex items-start gap-3">
                      <span aria-hidden="true" className="text-xl flex-shrink-0 mt-0.5">{ENTITY_ICON_MAP[r.entity_type] ?? '📎'}</span>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          {r.entity_ref && (
                            <span className="text-xs font-mono text-indigo-600 bg-indigo-50 px-1.5 py-0.5 rounded">{r.entity_ref}</span>
                          )}
                          <span className="text-xs px-2 py-0.5 rounded-full bg-gray-100 text-gray-600 capitalize">
                            {r.entity_type.replace('_', ' ')}
                          </span>
                          {r.status && (
                            <span className={`text-xs px-2 py-0.5 rounded-full capitalize ${STATUS_STYLES[r.status] ?? 'bg-gray-100 text-gray-600'}`}>
                              {r.status.replace('_', ' ')}
                            </span>
                          )}
                          {r.framework && (
                            <span className="text-xs px-2 py-0.5 rounded-full bg-purple-50 text-purple-700">{r.framework}</span>
                          )}
                        </div>
                        <h4 className="text-sm font-semibold text-gray-900 mt-1">{r.title}</h4>
                        {r.snippet && (
                          <p className="text-sm text-gray-600 mt-1 line-clamp-2">{highlightSnippet(r.snippet)}</p>
                        )}
                        {r.updated_at && (
                          <p className="text-xs text-gray-400 mt-1.5">
                            Updated {new Date(r.updated_at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })}
                          </p>
                        )}
                      </div>
                    </div>
                  </Link>
                ))}
              </div>

              {/* Pagination */}
              {totalPages > 1 && (
                <div className="flex items-center justify-center gap-1 mt-6">
                  <button
                    type="button"
                    onClick={() => handlePageChange(page - 1)}
                    disabled={page <= 1}
                    className="min-h-11 rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    Previous
                  </button>
                  {Array.from({ length: Math.min(totalPages, 7) }, (_, i) => {
                    let pageNum: number;
                    if (totalPages <= 7) {
                      pageNum = i + 1;
                    } else if (page <= 4) {
                      pageNum = i + 1;
                    } else if (page >= totalPages - 3) {
                      pageNum = totalPages - 6 + i;
                    } else {
                      pageNum = page - 3 + i;
                    }
                    return (
                      <button
                        key={pageNum}
                        type="button"
                        aria-current={pageNum === page ? 'page' : undefined}
                        aria-label={`Page ${pageNum}`}
                        onClick={() => handlePageChange(pageNum)}
                        className={`h-11 w-11 text-sm rounded-lg ${
                          pageNum === page
                            ? 'bg-indigo-600 text-white'
                            : 'border border-gray-300 text-gray-600 hover:bg-gray-50'
                        }`}
                      >
                        {pageNum}
                      </button>
                    );
                  })}
                  <button
                    type="button"
                    onClick={() => handlePageChange(page + 1)}
                    disabled={page >= totalPages}
                    className="min-h-11 rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    Next
                  </button>
                </div>
              )}
            </>
          ) : (
            /* Initial state - no query yet */
            <ResourceState
              surface="plain"
              kind="empty"
              title="Start a search"
              description="Search across frameworks, controls, risks, policies, audits, incidents, vendors, and more."
            />
          )}
        </div>
      </div>
    </div>
  );
}
