'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import api from '@/lib/api';

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
// Simple Markdown Renderer
// ---------------------------------------------------------------------------

function renderMarkdown(md: string): string {
  let html = md
    .replace(/^### (.+)$/gm, '<h3 class="text-lg font-semibold text-gray-900 mt-6 mb-2" id="$1">$1</h3>')
    .replace(/^## (.+)$/gm, '<h2 class="text-xl font-bold text-gray-900 mt-8 mb-3" id="$1">$1</h2>')
    .replace(/^# (.+)$/gm, '<h1 class="text-2xl font-bold text-gray-900 mt-8 mb-4" id="$1">$1</h1>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code class="bg-gray-100 text-sm px-1.5 py-0.5 rounded text-indigo-700">$1</code>')
    .replace(/^- (.+)$/gm, '<li class="ml-4 list-disc text-gray-700">$1</li>')
    .replace(/^(\d+)\. (.+)$/gm, '<li class="ml-4 list-decimal text-gray-700">$2</li>')
    .replace(/\n\n/g, '</p><p class="text-gray-700 leading-relaxed mb-3">')
    .replace(/\n/g, '<br />');
  return `<p class="text-gray-700 leading-relaxed mb-3">${html}</p>`;
}

function extractHeadings(md: string): { id: string; text: string; level: number }[] {
  const headings: { id: string; text: string; level: number }[] = [];
  const regex = /^(#{1,3}) (.+)$/gm;
  let match;
  while ((match = regex.exec(md)) !== null) {
    headings.push({ id: match[2], text: match[2], level: match[1].length });
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
  const [category, setCategory] = useState<Category>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedArticle, setSelectedArticle] = useState<Article | null>(null);
  const [feedbackGiven, setFeedbackGiven] = useState<Record<string, 'helpful' | 'not_helpful'>>({});

  const fetchArticles = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params: Record<string, unknown> = {};
      if (category !== 'all') params.category = category;
      if (searchQuery.trim()) params.search = searchQuery.trim();
      const data = await api.knowledge.list(params);
      const items = Array.isArray(data) ? data : (data as any).items ?? [];
      setArticles(items);
    } catch {
      setError('Failed to load knowledge base articles.');
    } finally {
      setLoading(false);
    }
  }, [category, searchQuery]);

  useEffect(() => { fetchArticles(); }, [fetchArticles]);

  // Load recommended
  useEffect(() => {
    api.knowledge.recommended()
      .then((data: any) => setRecommended(Array.isArray(data) ? data : data.items ?? []))
      .catch(() => {});
  }, []);

  const openArticle = async (article: Article) => {
    try {
      const full = (await api.knowledge.get(article.id)) as Article;
      setSelectedArticle(full);
    } catch {
      setSelectedArticle(article);
    }
  };

  const toggleBookmark = async (article: Article) => {
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
    } catch {}
  };

  const sendFeedback = async (articleId: string, type: 'helpful' | 'not_helpful') => {
    if (feedbackGiven[articleId]) return;
    try {
      await api.knowledge.feedback(articleId, { type });
      setFeedbackGiven((prev) => ({ ...prev, [articleId]: type }));
    } catch {}
  };

  const headings = selectedArticle?.content ? extractHeadings(selectedArticle.content) : [];

  // ----- Article Detail View -----
  if (selectedArticle) {
    return (
      <div className="p-6">
        <button
          onClick={() => setSelectedArticle(null)}
          className="flex items-center gap-1.5 text-sm text-gray-600 hover:text-gray-900 mb-4"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
          Back to Knowledge Base
        </button>

        <div className="flex gap-6">
          {/* TOC sidebar */}
          {headings.length > 0 && (
            <nav className="hidden xl:block w-56 flex-shrink-0 sticky top-6 self-start">
              <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">On this page</h4>
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

              <h1 className="text-2xl font-bold text-gray-900 mb-2">{selectedArticle.title}</h1>
              <p className="text-gray-500 text-sm mb-6">{selectedArticle.summary}</p>

              {selectedArticle.author && (
                <p className="text-xs text-gray-400 mb-6">
                  By {selectedArticle.author} | Updated {new Date(selectedArticle.updated_at).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric' })}
                </p>
              )}

              {/* Rendered content */}
              {selectedArticle.content ? (
                <div
                  className="prose prose-sm max-w-none"
                  dangerouslySetInnerHTML={{ __html: renderMarkdown(selectedArticle.content) }}
                />
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
                      onClick={() => sendFeedback(selectedArticle.id, 'helpful')}
                      disabled={!!feedbackGiven[selectedArticle.id]}
                      className={`px-3 py-1.5 text-sm rounded-lg border transition-colors ${
                        feedbackGiven[selectedArticle.id] === 'helpful'
                          ? 'bg-green-100 border-green-300 text-green-700'
                          : 'border-gray-300 text-gray-600 hover:bg-green-50 hover:border-green-300'
                      }`}
                    >
                      Yes
                    </button>
                    <button
                      onClick={() => sendFeedback(selectedArticle.id, 'not_helpful')}
                      disabled={!!feedbackGiven[selectedArticle.id]}
                      className={`px-3 py-1.5 text-sm rounded-lg border transition-colors ${
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
                    onClick={() => toggleBookmark(selectedArticle)}
                    className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border transition-colors ${
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
                <h3 className="text-sm font-semibold text-gray-700 mb-3">Recommended Articles</h3>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {recommended.slice(0, 4).map((rec) => (
                    <button
                      key={rec.id}
                      onClick={() => openArticle(rec)}
                      className="text-left bg-white border border-gray-200 rounded-lg p-3 hover:shadow-sm hover:border-indigo-200 transition-all"
                    >
                      <h4 className="text-sm font-medium text-gray-900 line-clamp-1">{rec.title}</h4>
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
            onClick={() => { setCategory(cat.value); setSelectedArticle(null); }}
            className={`p-3 rounded-xl border text-left transition-all ${
              category === cat.value
                ? 'bg-indigo-50 border-indigo-300 shadow-sm'
                : 'bg-white border-gray-200 hover:border-indigo-200 hover:shadow-sm'
            }`}
          >
            <span className="text-xl">{cat.icon}</span>
            <h3 className={`text-sm font-medium mt-1 ${category === cat.value ? 'text-indigo-700' : 'text-gray-900'}`}>{cat.label}</h3>
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
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search articles..."
          className="w-full pl-10 pr-4 py-2.5 text-sm border border-gray-300 rounded-xl bg-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none"
        />
      </div>

      {/* Loading */}
      {loading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="bg-white border border-gray-200 rounded-xl p-4 animate-pulse">
              <div className="h-4 bg-gray-200 rounded w-3/4 mb-2" />
              <div className="h-3 bg-gray-100 rounded w-full mb-1" />
              <div className="h-3 bg-gray-100 rounded w-2/3" />
            </div>
          ))}
        </div>
      ) : error ? (
        <div className="bg-red-50 border border-red-200 text-red-700 p-4 rounded-xl">
          <p className="font-semibold">Error</p>
          <p className="text-sm mt-1">{error}</p>
          <button onClick={fetchArticles} className="mt-2 text-sm font-medium underline">Retry</button>
        </div>
      ) : articles.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-xl p-8 text-center">
          <p className="text-gray-500">No articles found. Try a different category or search term.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {articles.map((article) => (
            <button
              key={article.id}
              onClick={() => openArticle(article)}
              className="text-left bg-white border border-gray-200 rounded-xl p-4 hover:shadow-md hover:border-indigo-200 transition-all group"
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
              <h3 className="text-sm font-semibold text-gray-900 group-hover:text-indigo-600 line-clamp-2">{article.title}</h3>
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
