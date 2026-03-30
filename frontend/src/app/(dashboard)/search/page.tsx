'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SearchResult {
  id: string;
  entity_type: string;
  entity_ref?: string;
  title: string;
  snippet?: string;
  status?: string;
  severity?: string;
  framework?: string;
  updated_at?: string;
  score?: number;
}

interface SearchResponse {
  items: SearchResult[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
  query_time_ms?: number;
  suggestions?: string[];
}

interface Facets {
  entity_type: string;
  framework: string;
  status: string;
  severity: string;
  date_from: string;
  date_to: string;
}

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
  const [results, setResults] = useState<SearchResult[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [page, setPage] = useState(1);
  const [queryTime, setQueryTime] = useState<number | null>(null);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [facets, setFacets] = useState<Facets>({
    entity_type: '',
    framework: '',
    status: '',
    severity: '',
    date_from: '',
    date_to: '',
  });

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

        const data = (await api.search.query(params)) as SearchResponse;
        setResults(data.items ?? []);
        setTotal(data.total ?? 0);
        setTotalPages(data.total_pages ?? 0);
        setQueryTime(data.query_time_ms ?? null);
        setSuggestions(data.suggestions ?? []);
      } catch {
        setError('Search failed. Please try again.');
      } finally {
        setLoading(false);
      }
    },
    []
  );

  // Run search on mount if query present
  useEffect(() => {
    if (initialQuery) doSearch(initialQuery, 1, facets);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const clearFilters = () => {
    const cleared: Facets = { entity_type: '', framework: '', status: '', severity: '', date_from: '', date_to: '' };
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
      {/* Search bar */}
      <form onSubmit={handleSubmit}>
        <div className="relative">
          <svg className="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search across all compliance data..."
            className="w-full pl-12 pr-24 py-3.5 text-base border border-gray-300 rounded-xl bg-white shadow-sm focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none"
          />
          <button
            type="submit"
            className="absolute right-2 top-1/2 -translate-y-1/2 px-5 py-2 bg-indigo-600 text-white text-sm font-medium rounded-lg hover:bg-indigo-700 transition-colors"
          >
            Search
          </button>
        </div>
      </form>

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
            <button onClick={clearFilters} className="text-sm text-indigo-600 hover:text-indigo-700 font-medium">
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
              <label className="text-xs font-medium text-gray-600 mb-1.5 block">Entity Type</label>
              <select
                value={facets.entity_type}
                onChange={(e) => handleFacetChange('entity_type', e.target.value)}
                className="w-full text-sm border border-gray-300 rounded-lg px-3 py-2"
              >
                <option value="">All Types</option>
                {ENTITY_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
            </div>

            {/* Framework */}
            <div>
              <label className="text-xs font-medium text-gray-600 mb-1.5 block">Framework</label>
              <input
                type="text"
                value={facets.framework}
                onChange={(e) => handleFacetChange('framework', e.target.value)}
                placeholder="e.g. ISO27001"
                className="w-full text-sm border border-gray-300 rounded-lg px-3 py-2"
              />
            </div>

            {/* Status */}
            <div>
              <label className="text-xs font-medium text-gray-600 mb-1.5 block">Status</label>
              <select
                value={facets.status}
                onChange={(e) => handleFacetChange('status', e.target.value)}
                className="w-full text-sm border border-gray-300 rounded-lg px-3 py-2"
              >
                <option value="">All</option>
                {STATUS_OPTIONS.map((s) => (
                  <option key={s} value={s}>{s.replace('_', ' ')}</option>
                ))}
              </select>
            </div>

            {/* Severity */}
            <div>
              <label className="text-xs font-medium text-gray-600 mb-1.5 block">Severity</label>
              <select
                value={facets.severity}
                onChange={(e) => handleFacetChange('severity', e.target.value)}
                className="w-full text-sm border border-gray-300 rounded-lg px-3 py-2"
              >
                <option value="">All</option>
                {SEVERITY_OPTIONS.map((s) => (
                  <option key={s} value={s} className="capitalize">{s}</option>
                ))}
              </select>
            </div>

            {/* Date range */}
            <div>
              <label className="text-xs font-medium text-gray-600 mb-1.5 block">Date Range</label>
              <div className="space-y-1.5">
                <input
                  type="date"
                  value={facets.date_from}
                  onChange={(e) => handleFacetChange('date_from', e.target.value)}
                  className="w-full text-sm border border-gray-300 rounded-lg px-3 py-2"
                />
                <input
                  type="date"
                  value={facets.date_to}
                  onChange={(e) => handleFacetChange('date_to', e.target.value)}
                  className="w-full text-sm border border-gray-300 rounded-lg px-3 py-2"
                />
              </div>
            </div>
          </div>
        </div>

        {/* Results list */}
        <div className="flex-1">
          {loading ? (
            <div className="space-y-3">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="bg-white border border-gray-200 rounded-xl p-4 animate-pulse">
                  <div className="h-4 bg-gray-200 rounded w-3/4 mb-2" />
                  <div className="h-3 bg-gray-100 rounded w-full mb-1" />
                  <div className="h-3 bg-gray-100 rounded w-1/2" />
                </div>
              ))}
            </div>
          ) : error ? (
            <div className="bg-red-50 border border-red-200 text-red-700 p-4 rounded-xl">
              <p className="font-semibold">Search Error</p>
              <p className="text-sm mt-1">{error}</p>
            </div>
          ) : results.length === 0 && query.trim() ? (
            <div className="bg-white border border-gray-200 rounded-xl p-8 text-center">
              <svg className="mx-auto w-12 h-12 text-gray-300 mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <h3 className="text-lg font-semibold text-gray-700">No results found</h3>
              <p className="text-sm text-gray-500 mt-1">Try different keywords or adjust your filters.</p>
              {suggestions.length > 0 && (
                <div className="mt-4">
                  <p className="text-sm text-gray-500 mb-2">Did you mean:</p>
                  <div className="flex flex-wrap justify-center gap-2">
                    {suggestions.map((s) => (
                      <button
                        key={s}
                        onClick={() => { setQuery(s); doSearch(s, 1, facets); }}
                        className="text-sm text-indigo-600 hover:text-indigo-700 font-medium bg-indigo-50 px-3 py-1 rounded-full hover:bg-indigo-100"
                      >
                        {s}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : results.length > 0 ? (
            <>
              <div className="space-y-3">
                {results.map((r) => (
                  <a
                    key={r.id}
                    href={`/${r.entity_type === 'control' ? 'frameworks' : r.entity_type + 's'}/${r.id}`}
                    className="block bg-white border border-gray-200 rounded-xl p-4 hover:shadow-md hover:border-indigo-200 transition-all"
                  >
                    <div className="flex items-start gap-3">
                      <span className="text-xl flex-shrink-0 mt-0.5">{ENTITY_ICON_MAP[r.entity_type] ?? '📎'}</span>
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
                  </a>
                ))}
              </div>

              {/* Pagination */}
              {totalPages > 1 && (
                <div className="flex items-center justify-center gap-1 mt-6">
                  <button
                    onClick={() => handlePageChange(page - 1)}
                    disabled={page <= 1}
                    className="px-3 py-2 text-sm rounded-lg border border-gray-300 text-gray-600 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
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
                        onClick={() => handlePageChange(pageNum)}
                        className={`w-9 h-9 text-sm rounded-lg ${
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
                    onClick={() => handlePageChange(page + 1)}
                    disabled={page >= totalPages}
                    className="px-3 py-2 text-sm rounded-lg border border-gray-300 text-gray-600 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
                  >
                    Next
                  </button>
                </div>
              )}
            </>
          ) : (
            /* Initial state - no query yet */
            <div className="bg-white border border-gray-200 rounded-xl p-12 text-center">
              <svg className="mx-auto w-16 h-16 text-gray-200 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <h3 className="text-lg font-semibold text-gray-700">Search ComplianceForge</h3>
              <p className="text-sm text-gray-500 mt-1">
                Search across frameworks, controls, risks, policies, audits, incidents, vendors, and more.
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
