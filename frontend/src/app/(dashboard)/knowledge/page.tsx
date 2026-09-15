'use client';

import { ResourceState, StaleDataNotice } from '@/components/data/resource-state';
import { useCallback, useEffect, useRef, useState } from 'react';
import api from '@/lib/api';
import { isForbiddenResourceError } from '@/lib/resource-errors';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Article {
  id: string;
  title: string;
  summary: string;
  content?: string;
  category: string;
  frameworks: string[];
  difficulty: 'beginner' | 'intermediate' | 'advanced';
  reading_time_min: number;
  helpful_count: number;
  not_helpful_count: number;
  bookmarked?: boolean;
  updated_at: string;
  author?: string;
  tags?: string[];
}

interface ArticleListResponse {
  data?: Article[];
  items?: Article[];
}

type Category = 'all' | 'implementation_guides' | 'regulatory_guides' | 'best_practices' | 'glossary';

const CATEGORIES: { value: Category; label: string; icon: string; description: string }[] = [
  { value: 'all', label: 'All Articles', icon: '📚', description: 'Browse everything' },
  { value: 'implementation_guides', label: 'Implementation Guides', icon: '🔧', description: 'Step-by-step implementation' },
  { value: 'regulatory_guides', label: 'Regulatory Guides', icon: '📜', description: 'Regulatory requirements explained' },
  { value: 'best_practices', label: 'Best Practices', icon: '🏆', description: 'Industry best practices' },
  { value: 'glossary', label: 'Glossary', icon: '📖', description: 'Terms and definitions' },
];

const DIFFICULTY_STYLES: Record<string, string> = {
  beginner: 'bg-green-100 text-green-700',
  intermediate: 'bg-yellow-100 text-yellow-700',
  advanced: 'bg-red-100 text-red-700',
};

const FRAMEWORK_COLORS: Record<string, string> = {
  ISO27001: 'bg-blue-100 text-blue-700',
  UK_GDPR: 'bg-purple-100 text-purple-700',
  NCSC_CAF: 'bg-emerald-100 text-emerald-700',
  CYBER_ESSENTIALS: 'bg-orange-100 text-orange-700',
  NIST_800_53: 'bg-red-100 text-red-700',
  NIST_CSF_2: 'bg-sky-100 text-sky-700',
  PCI_DSS_4: 'bg-cyan-100 text-cyan-700',
};

// ---------------------------------------------------------------------------
// Safe, deliberately small Markdown renderer. Server-provided content remains
// text and is never injected as HTML.
// ---------------------------------------------------------------------------

function headingId(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, '')
    .trim()
    .replace(/\s+/g, '-');
}

function normalizeArticles(value: unknown): Article[] {
  if (Array.isArray(value)) return value as Article[];
  if (!value || typeof value !== 'object') return [];
  const response = value as ArticleListResponse;
  return response.items ?? response.data ?? [];
}

function MarkdownContent({ markdown }: { markdown: string }) {
  return (
    <div className="space-y-3 text-gray-700">
      {markdown.split(/\n{2,}/).map((block, index) => {
        const value = block.trim();
        const heading = /^(#{1,3})\s+(.+)$/.exec(value);
        if (heading) {
          const level = heading[1].length;
          const text = heading[2];
          const className = 'mt-6 font-semibold text-gray-900';
          if (level === 1) return <h2 id={headingId(text)} key={index} className={`${className} text-xl`}>{text}</h2>;
          if (level === 2) return <h3 id={headingId(text)} key={index} className={`${className} text-lg`}>{text}</h3>;
          return <h4 id={headingId(text)} key={index} className={className}>{text}</h4>;
        }

        const lines = value.split('\n');
        if (lines.every((line) => /^-\s+/.test(line))) {
          return (
            <ul key={index} className="list-disc space-y-1 pl-6">
              {lines.map((line, lineIndex) => <li key={lineIndex}>{line.replace(/^-\s+/, '')}</li>)}
            </ul>
          );
        }
        if (lines.every((line) => /^\d+\.\s+/.test(line))) {
          return (
            <ol key={index} className="list-decimal space-y-1 pl-6">
              {lines.map((line, lineIndex) => <li key={lineIndex}>{line.replace(/^\d+\.\s+/, '')}</li>)}
            </ol>
          );
        }
        return <p key={index} className="whitespace-pre-line leading-relaxed">{value}</p>;
      })}
    </div>
  );
}

function extractHeadings(md: string): { id: string; text: string; level: number }[] {
  const headings: { id: string; text: string; level: number }[] = [];
  const regex = /^(#{1,3}) (.+)$/gm;
  let match;
  while ((match = regex.exec(md)) !== null) {
    headings.push({ id: headingId(match[2]), text: match[2], level: match[1].length });
  }
  return headings;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function KnowledgeBasePage() {
  const [articles, setArticles] = useState<Article[]>([]);
  const [recommended, setRecommended] = useState<Article[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [category, setCategory] = useState<Category>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedArticle, setSelectedArticle] = useState<Article | null>(null);
  const [feedbackGiven, setFeedbackGiven] = useState<Record<string, 'helpful' | 'not_helpful'>>({});
  const [actionError, setActionError] = useState<string | null>(null);
  const articleHeadingRef = useRef<HTMLHeadingElement>(null);
  const articleTriggerRef = useRef<HTMLButtonElement | null>(null);

  const fetchArticles = useCallback(async () => {
    setLoading(true);
    setError(null);
    setForbidden(false);
    try {
      const params: Record<string, unknown> = {};
      if (category !== 'all') params.category = category;
      if (searchQuery.trim()) params.search = searchQuery.trim();
      setArticles(normalizeArticles(await api.knowledge.list(params)));
    } catch (cause: unknown) {
      if (isForbiddenResourceError(cause)) {
        setArticles([]);
        setForbidden(true);
      }
      setError('Failed to load knowledge base articles.');
    } finally {
      setLoading(false);
    }
  }, [category, searchQuery]);

  useEffect(() => {
    const timer = window.setTimeout(() => void fetchArticles(), 300);
    return () => window.clearTimeout(timer);
  }, [fetchArticles]);

  // Load recommended
  useEffect(() => {
    api.knowledge.recommended()
      .then((data: unknown) => setRecommended(normalizeArticles(data)))
      .catch(() => {});
  }, []);

  const openArticle = async (article: Article, trigger?: HTMLButtonElement) => {
    if (trigger) articleTriggerRef.current = trigger;
    try {
      const full = (await api.knowledge.get(article.id)) as Article;
      setSelectedArticle(full);
    } catch {
      setSelectedArticle(article);
    }
  };

  const toggleBookmark = async (article: Article) => {
    setActionError(null);
    try {
      if (article.bookmarked) {
        await api.knowledge.unbookmark(article.id);
      } else {
        await api.knowledge.bookmark(article.id);
      }
      const update = (a: Article) =>
        a.id === article.id ? { ...a, bookmarked: !a.bookmarked } : a;
      setArticles((prev) => prev.map(update));
      if (selectedArticle?.id === article.id) {
        setSelectedArticle((prev) => prev ? { ...prev, bookmarked: !prev.bookmarked } : prev);
      }
    } catch {
      setActionError('The bookmark could not be updated. Try again.');
    }
  };

  const sendFeedback = async (articleId: string, type: 'helpful' | 'not_helpful') => {
    if (feedbackGiven[articleId]) return;
    setActionError(null);
    try {
      await api.knowledge.feedback(articleId, { type });
      setFeedbackGiven((prev) => ({ ...prev, [articleId]: type }));
    } catch {
      setActionError('Your feedback could not be saved. Try again.');
    }
  };

  const headings = selectedArticle?.content ? extractHeadings(selectedArticle.content) : [];

  useEffect(() => {
    if (selectedArticle) articleHeadingRef.current?.focus({ preventScroll: true });
  }, [selectedArticle]);

  const closeArticle = () => {
    setSelectedArticle(null);
    window.requestAnimationFrame(() => articleTriggerRef.current?.focus());
  };

  // ----- Article Detail View -----
  if (selectedArticle) {
    return (
      <div className="p-6">
        <button
          type="button"
          onClick={closeArticle}
          className="mb-4 flex min-h-11 items-center gap-1.5 rounded-md px-2 text-sm text-gray-700 hover:text-gray-900"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
          Back to Knowledge Base
        </button>

        <div className="flex gap-6">
          {/* TOC sidebar */}
          {headings.length > 0 && (
            <nav className="hidden xl:block w-56 flex-shrink-0 sticky top-6 self-start">
              <p className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">On this page</p>
              <div className="space-y-1 border-l-2 border-gray-200">
                {headings.map((h, i) => (
                  <a
                    key={i}
                    href={`#${h.id}`}
                    className={`block text-sm text-gray-600 hover:text-indigo-600 py-0.5 ${
                      h.level === 1 ? 'pl-3 font-medium' : h.level === 2 ? 'pl-5' : 'pl-7 text-xs'
                    }`}
                  >
                    {h.text}
                  </a>
                ))}
              </div>
            </nav>
          )}

          {/* Article content */}
          <article className="flex-1 max-w-3xl">
            <div className="bg-white border border-gray-200 rounded-xl p-6 sm:p-8">
              {/* Meta */}
              <div className="flex flex-wrap items-center gap-2 mb-4">
                <span className={`text-xs px-2 py-0.5 rounded-full capitalize ${DIFFICULTY_STYLES[selectedArticle.difficulty] ?? 'bg-gray-100 text-gray-600'}`}>
                  {selectedArticle.difficulty}
                </span>
                <span className="text-xs text-gray-500">{selectedArticle.reading_time_min} min read</span>
                {selectedArticle.frameworks.map((fw) => (
                  <span key={fw} className={`text-xs px-2 py-0.5 rounded-full ${FRAMEWORK_COLORS[fw] ?? 'bg-gray-100 text-gray-600'}`}>
                    {fw.replace(/_/g, ' ')}
                  </span>
                ))}
              </div>

              <h1 ref={articleHeadingRef} tabIndex={-1} className="text-2xl font-bold text-gray-900 mb-2">{selectedArticle.title}</h1>
              <p className="text-gray-500 text-sm mb-6">{selectedArticle.summary}</p>

              {actionError && (
                <div role="alert" className="mb-6 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-800">
                  {actionError}
                </div>
              )}

              {selectedArticle.author && (
                <p className="text-xs text-gray-400 mb-6">
                  By {selectedArticle.author} | Updated {new Date(selectedArticle.updated_at).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric' })}
                </p>
              )}

              {/* Rendered content */}
              {selectedArticle.content ? (
                <MarkdownContent markdown={selectedArticle.content} />
              ) : (
                <p className="text-gray-500 italic">Full article content is not available.</p>
              )}

              {/* Actions */}
              <div className="mt-8 pt-6 border-t border-gray-200">
                <div className="flex items-center justify-between flex-wrap gap-4">
                  {/* Feedback */}
                  <div className="flex items-center gap-3">
                    <span className="text-sm text-gray-600">Was this helpful?</span>
                    <button
                      type="button"
                      onClick={() => sendFeedback(selectedArticle.id, 'helpful')}
                      disabled={!!feedbackGiven[selectedArticle.id]}
                      className={`min-h-11 px-3 py-1.5 text-sm rounded-lg border transition-colors motion-reduce:transition-none ${
                        feedbackGiven[selectedArticle.id] === 'helpful'
                          ? 'bg-green-100 border-green-300 text-green-700'
                          : 'border-gray-300 text-gray-600 hover:bg-green-50 hover:border-green-300'
                      }`}
                    >
                      Yes
                    </button>
                    <button
                      type="button"
                      onClick={() => sendFeedback(selectedArticle.id, 'not_helpful')}
                      disabled={!!feedbackGiven[selectedArticle.id]}
                      className={`min-h-11 px-3 py-1.5 text-sm rounded-lg border transition-colors motion-reduce:transition-none ${
                        feedbackGiven[selectedArticle.id] === 'not_helpful'
                          ? 'bg-red-100 border-red-300 text-red-700'
                          : 'border-gray-300 text-gray-600 hover:bg-red-50 hover:border-red-300'
                      }`}
                    >
                      No
                    </button>
                  </div>

                  {/* Bookmark */}
                  <button
                    type="button"
                    onClick={() => toggleBookmark(selectedArticle)}
                    className={`flex min-h-11 items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border transition-colors motion-reduce:transition-none ${
                      selectedArticle.bookmarked
                        ? 'bg-indigo-50 border-indigo-300 text-indigo-700'
                        : 'border-gray-300 text-gray-600 hover:bg-indigo-50'
                    }`}
                  >
                    <svg className="w-4 h-4" fill={selectedArticle.bookmarked ? 'currentColor' : 'none'} stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
                    </svg>
                    {selectedArticle.bookmarked ? 'Bookmarked' : 'Bookmark'}
                  </button>
                </div>
              </div>
            </div>

            {/* Recommended */}
            {recommended.length > 0 && (
              <div className="mt-6">
                <h2 className="text-sm font-semibold text-gray-700 mb-3">Recommended Articles</h2>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {recommended.slice(0, 4).map((rec) => (
                    <button
                      key={rec.id}
                      type="button"
                      onClick={(event) => void openArticle(rec, event.currentTarget)}
                      className="min-h-11 rounded-lg border border-gray-200 bg-white p-3 text-left transition-all motion-reduce:transition-none hover:border-indigo-200 hover:shadow-sm"
                    >
                      <h3 className="text-sm font-medium text-gray-900 line-clamp-1">{rec.title}</h3>
                      <p className="text-xs text-gray-500 mt-1 line-clamp-2">{rec.summary}</p>
                      <div className="flex items-center gap-2 mt-2">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-full capitalize ${DIFFICULTY_STYLES[rec.difficulty] ?? ''}`}>
                          {rec.difficulty}
                        </span>
                        <span className="text-[10px] text-gray-400">{rec.reading_time_min} min</span>
                      </div>
                    </button>
                  ))}
                </div>
              </div>
            )}
          </article>
        </div>
      </div>
    );
  }

  // ----- List View -----
  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Knowledge Base</h1>
          <p className="text-sm text-gray-500 mt-1">Guides, best practices, and reference materials</p>
        </div>
      </div>

      {/* Category cards */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        {CATEGORIES.map((cat) => (
          <button
            key={cat.value}
            type="button"
            aria-pressed={category === cat.value}
            onClick={() => { setCategory(cat.value); setSelectedArticle(null); }}
            className={`min-h-11 p-3 rounded-xl border text-left transition-all motion-reduce:transition-none ${
              category === cat.value
                ? 'bg-indigo-50 border-indigo-300 shadow-sm'
                : 'bg-white border-gray-200 hover:border-indigo-200 hover:shadow-sm'
            }`}
          >
            <span aria-hidden="true" className="text-xl">{cat.icon}</span>
            <span className={`mt-1 block text-sm font-medium ${category === cat.value ? 'text-indigo-700' : 'text-gray-900'}`}>{cat.label}</span>
            <p className="text-[10px] text-gray-500 mt-0.5">{cat.description}</p>
          </button>
        ))}
      </div>

      {/* Search within KB */}
      <div className="relative max-w-md">
        <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          aria-label="Search knowledge base articles"
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search articles..."
          className="min-h-11 w-full rounded-xl border border-gray-300 bg-white py-2.5 pl-10 pr-4 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500"
        />
      </div>

      {/* Loading */}
      {error && articles.length > 0 && (
        <StaleDataNotice
          title="Showing articles from the previous successful request"
          onRefresh={() => void fetchArticles()}
        />
      )}
      {forbidden ? (
        <ResourceState kind="forbidden" title="Knowledge base access unavailable" />
      ) : loading && articles.length === 0 ? (
        <ResourceState kind="loading" loadingLayout="cards" title="Loading knowledge base" />
      ) : error && articles.length === 0 ? (
        <ResourceState
          kind="error"
          title="Knowledge base could not be loaded"
          description={error}
          onRetry={() => void fetchArticles()}
        />
      ) : articles.length === 0 ? (
        <ResourceState
          kind="empty"
          title="No articles found"
          description="Try a different category or search term."
        />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {articles.map((article) => (
            <button
              key={article.id}
              type="button"
              onClick={(event) => void openArticle(article, event.currentTarget)}
              className="group min-h-11 rounded-xl border border-gray-200 bg-white p-4 text-left transition-all motion-reduce:transition-none hover:border-indigo-200 hover:shadow-md"
            >
              <div className="flex items-center justify-between mb-2">
                <span className={`text-xs px-2 py-0.5 rounded-full capitalize ${DIFFICULTY_STYLES[article.difficulty] ?? 'bg-gray-100 text-gray-600'}`}>
                  {article.difficulty}
                </span>
                <div className="flex items-center gap-2">
                  <span className="text-xs text-gray-400">{article.reading_time_min} min</span>
                  {article.bookmarked && (
                    <svg className="w-3.5 h-3.5 text-indigo-500" fill="currentColor" viewBox="0 0 24 24">
                      <path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
                    </svg>
                  )}
                </div>
              </div>
              <h2 className="text-sm font-semibold text-gray-900 group-hover:text-indigo-600 line-clamp-2">{article.title}</h2>
              <p className="text-xs text-gray-500 mt-1.5 line-clamp-3">{article.summary}</p>
              {article.frameworks.length > 0 && (
                <div className="flex flex-wrap gap-1 mt-3">
                  {article.frameworks.slice(0, 3).map((fw) => (
                    <span key={fw} className={`text-[10px] px-1.5 py-0.5 rounded-full ${FRAMEWORK_COLORS[fw] ?? 'bg-gray-100 text-gray-600'}`}>
                      {fw.replace(/_/g, ' ')}
                    </span>
                  ))}
                  {article.frameworks.length > 3 && (
                    <span className="text-[10px] text-gray-400">+{article.frameworks.length - 3}</span>
                  )}
                </div>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
