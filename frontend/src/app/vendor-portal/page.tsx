'use client';

import {
  buildPortalApiUrl,
  cleanPortalUrl,
  getPortalEntryToken,
  PORTAL_API_ROUTES,
} from '@/lib/portal-routes';
import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchWithCsrf } from '@/lib/csrf-client';
import { ResourceState } from '@/components/data/resource-state';
import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Question {
  id: string;
  section_id: string;
  text: string;
  description?: string;
  type: 'yes_no' | 'single_choice' | 'multi_select' | 'text' | 'file_upload';
  required: boolean;
  options?: string[];
  order: number;
}

interface Section {
  id: string;
  title: string;
  description?: string;
  order: number;
  questions: Question[];
}

interface QuestionnaireData {
  id: string;
  name: string;
  description: string;
  vendor_name: string;
  organization_name: string;
  due_date: string;
  sections: Section[];
}

interface AnswerMap {
  [questionId: string]: {
    value: string | string[];
    files?: File[];
  };
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function ProgressBar({ current, total }: { current: number; total: number }) {
  const pct = total > 0 ? Math.round((current / total) * 100) : 0;
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-xs text-gray-500">
        <span>
          {current} of {total} answered
        </span>
        <span>{pct}%</span>
      </div>
      <div
        aria-label="Questionnaire completion"
        aria-valuemax={total}
        aria-valuemin={0}
        aria-valuenow={current}
        className="h-2 w-full rounded-full bg-gray-200"
        role="progressbar"
      >
        <div
          className="h-2 rounded-full bg-blue-600 transition-all motion-reduce:transition-none"
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

function QuestionInput({
  question,
  answer,
  onChange,
}: {
  question: Question;
  answer?: { value: string | string[]; files?: File[] };
  onChange: (value: string | string[], files?: File[]) => void;
}) {
  const currentValue = answer?.value ?? (question.type === 'multi_select' ? [] : '');

  switch (question.type) {
    case 'yes_no':
      return (
        <div className="flex gap-3">
          {['Yes', 'No'].map((opt) => (
            <button
              key={opt}
              type="button"
              aria-pressed={currentValue === opt}
              onClick={() => onChange(opt)}
              className={`min-h-11 rounded border px-4 py-2 text-sm font-medium transition-colors motion-reduce:transition-none ${
                currentValue === opt
                  ? 'bg-blue-600 text-white border-blue-600'
                  : 'bg-white text-gray-700 border-gray-300 hover:border-blue-400'
              }`}
            >
              {opt}
            </button>
          ))}
          <button
            type="button"
            aria-pressed={currentValue === 'N/A'}
            onClick={() => onChange('N/A')}
            className={`min-h-11 rounded border px-4 py-2 text-sm font-medium transition-colors motion-reduce:transition-none ${
              currentValue === 'N/A'
                ? 'bg-gray-600 text-white border-gray-600'
                : 'bg-white text-gray-700 border-gray-300 hover:border-gray-400'
            }`}
          >
            N/A
          </button>
        </div>
      );

    case 'single_choice':
      return (
        <div className="space-y-2">
          {(question.options ?? []).map((opt) => (
            <label key={opt} className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="radio"
                name={question.id}
                checked={currentValue === opt}
                onChange={() => onChange(opt)}
                className="text-blue-600"
              />
              {opt}
            </label>
          ))}
        </div>
      );

    case 'multi_select':
      return (
        <div className="space-y-2">
          {(question.options ?? []).map((opt) => {
            const selected = Array.isArray(currentValue) && currentValue.includes(opt);
            return (
              <label key={opt} className="flex items-center gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={selected}
                  onChange={() => {
                    const arr = Array.isArray(currentValue) ? [...currentValue] : [];
                    if (selected) {
                      onChange(arr.filter((v) => v !== opt));
                    } else {
                      onChange([...arr, opt]);
                    }
                  }}
                  className="rounded text-blue-600"
                />
                {opt}
              </label>
            );
          })}
        </div>
      );

    case 'text':
      return (
        <textarea
          aria-label={question.text}
          value={typeof currentValue === 'string' ? currentValue : ''}
          onChange={(e) => onChange(e.target.value)}
          rows={4}
          className="w-full border rounded px-3 py-2 text-sm"
          placeholder="Enter your response..."
        />
      );

    case 'file_upload':
      return (
        <div className="space-y-2">
          <textarea
            aria-label={`${question.text} description`}
            value={typeof currentValue === 'string' ? currentValue : ''}
            onChange={(e) => onChange(e.target.value, answer?.files)}
            rows={2}
            className="w-full border rounded px-3 py-2 text-sm"
            placeholder="Optional description..."
          />
          <div className="border-2 border-dashed border-gray-300 rounded-lg p-4 text-center">
            <input
              aria-label={`${question.text} supporting evidence`}
              type="file"
              multiple
              onChange={(e) => {
                const files = Array.from(e.target.files ?? []);
                onChange(typeof currentValue === 'string' ? currentValue : '', files);
              }}
              className="text-sm text-gray-600"
            />
            <p className="text-xs text-gray-400 mt-1">Upload supporting evidence</p>
          </div>
          {answer?.files && answer.files.length > 0 && (
            <div className="space-y-1">
              {answer.files.map((f, i) => (
                <div key={i} className="text-xs text-gray-500 flex items-center gap-1">
                  <span>Attached: {f.name}</span>
                  <span className="text-gray-400">({(f.size / 1024).toFixed(1)} KB)</span>
                </div>
              ))}
            </div>
          )}
        </div>
      );

    default:
      return null;
  }
}

// ---------------------------------------------------------------------------
// Inner Page (uses useSearchParams)
// ---------------------------------------------------------------------------

function VendorPortalInner() {
  const searchParams = useSearchParams();
  const token = searchParams.get('token');
  const initialized = useRef(false);
  const inviteTokenRef = useRef<string | null>(null);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [questionnaire, setQuestionnaire] = useState<QuestionnaireData | null>(null);
  const [answers, setAnswers] = useState<AnswerMap>({});
  const [activeSectionIdx, setActiveSectionIdx] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [saving, setSaving] = useState(false);
  const [lastSaved, setLastSaved] = useState<string | null>(null);
  const [retryKey, setRetryKey] = useState(0);

  // Token validation & questionnaire fetch
  useEffect(() => {
    if (!initialized.current) {
      initialized.current = true;
      inviteTokenRef.current = getPortalEntryToken(token, window.location.hash);
    }
    if (inviteTokenRef.current) {
      window.history.replaceState(
        window.history.state,
        '',
        cleanPortalUrl(window.location.href),
      );
    }

    async function fetchQuestionnaire() {
      setLoading(true);
      setError('');
      try {
        const res = inviteTokenRef.current
          ? await fetchWithCsrf(buildPortalApiUrl(PORTAL_API_ROUTES.vendorSession), {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ token: inviteTokenRef.current }),
            })
          : await fetch(buildPortalApiUrl(PORTAL_API_ROUTES.vendorQuestionnaire));
        if (!res.ok) {
          throw new Error(
            [401, 404].includes(res.status)
              ? 'Invitation expired or invalid. Please reopen the link from your email.'
              : 'Failed to load questionnaire',
          );
        }
        const data = await res.json();
        inviteTokenRef.current = null;
        setQuestionnaire(data);
        if (data.saved_answers) {
          setAnswers(data.saved_answers);
        }
      } catch (cause: unknown) {
        setError(cause instanceof Error ? cause.message : 'Failed to load questionnaire');
      } finally {
        setLoading(false);
      }
    }

    fetchQuestionnaire();
  }, [retryKey, token]);

  // Auto-save
  const autoSave = useCallback(async () => {
    if (!questionnaire || Object.keys(answers).length === 0) return true;
    setSaving(true);
    try {
      const response = await fetchWithCsrf(
        buildPortalApiUrl(PORTAL_API_ROUTES.vendorSave),
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ answers }),
        }
      );
      if (!response.ok) throw new Error('Unable to save responses');
      setError('');
      setLastSaved(new Date().toLocaleTimeString());
      return true;
    } catch {
      setError('Responses could not be saved. Check your connection and try again.');
      return false;
    } finally {
      setSaving(false);
    }
  }, [questionnaire, answers]);

  useEffect(() => {
    const interval = setInterval(autoSave, 30000); // auto-save every 30s
    return () => clearInterval(interval);
  }, [autoSave]);

  const handleAnswerChange = (questionId: string, value: string | string[], files?: File[]) => {
    setAnswers((prev) => ({
      ...prev,
      [questionId]: { value, files: files ?? prev[questionId]?.files },
    }));
  };

  const handleSaveAndContinue = async () => {
    await autoSave();
  };

  const handleSubmit = async () => {
    if (!questionnaire) return;
    setSubmitting(true);
    try {
      if (!(await autoSave())) throw new Error('Unable to save responses before submission');
      const res = await fetchWithCsrf(
        buildPortalApiUrl(PORTAL_API_ROUTES.vendorSubmit),
        {
          method: 'POST',
        }
      );
      if (!res.ok) throw new Error('Submission failed');
      setSubmitted(true);
    } catch (cause: unknown) {
      setError(cause instanceof Error ? cause.message : 'Submission failed');
    } finally {
      setSubmitting(false);
    }
  };

  // Compute progress
  const allQuestions = questionnaire?.sections.flatMap((s) => s.questions) ?? [];
  const answeredCount = allQuestions.filter((q) => {
    const a = answers[q.id];
    if (!a) return false;
    if (Array.isArray(a.value)) return a.value.length > 0;
    return a.value !== '';
  }).length;

  const activeSection = questionnaire?.sections[activeSectionIdx];

  // Loading
  if (loading) {
    return (
      <main id="main-content" className="min-h-screen bg-gray-50 p-6">
        <h1 className="sr-only">Vendor assessment</h1>
        <ResourceState kind="loading" loadingLayout="detail" title="Loading questionnaire" />
      </main>
    );
  }

  // Error
  if (error && !questionnaire) {
    return (
      <main id="main-content" className="flex min-h-screen items-center bg-gray-50 p-6">
        <ResourceState
          headingLevel={1}
          kind="error"
          title="Vendor assessment unavailable"
          description={error}
          onRetry={() => setRetryKey((value) => value + 1)}
        />
      </main>
    );
  }

  // Submitted
  if (submitted) {
    return (
      <main id="main-content" className="flex min-h-screen items-center justify-center bg-gray-50 p-6">
        <div
          aria-labelledby="assessment-submitted-title"
          className="max-w-md rounded-lg bg-white p-8 text-center shadow-lg"
          role="status"
        >
          <div className="w-12 h-12 bg-green-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <span className="text-green-600 text-2xl font-bold">{'\u2713'}</span>
          </div>
          <h1 id="assessment-submitted-title" className="text-lg font-semibold text-gray-900">Assessment Submitted</h1>
          <p className="text-sm text-gray-500 mt-2">
            Thank you for completing the assessment. Your responses have been submitted to{' '}
            {questionnaire?.organization_name ?? 'the requesting organization'}.
          </p>
        </div>
      </main>
    );
  }

  if (allQuestions.length === 0) {
    return (
      <main id="main-content" className="flex min-h-screen items-center bg-gray-50 p-6">
        <ResourceState
          headingLevel={1}
          kind="empty"
          title="No assessment questions are available"
          description="The requesting organization has not published questions for this assessment. Contact them before submitting a response."
        />
      </main>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <header className="bg-white border-b shadow-sm">
        <div className="max-w-4xl mx-auto px-6 py-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h1 data-route-heading className="text-lg font-bold text-gray-900">{questionnaire?.name}</h1>
              <p className="text-sm text-gray-500">
                For: {questionnaire?.organization_name} | Vendor: {questionnaire?.vendor_name}
              </p>
            </div>
            <div className="text-right">
              <p className="text-xs text-gray-400">
                Due: {questionnaire?.due_date ? new Date(questionnaire.due_date).toLocaleDateString() : '--'}
              </p>
              <div aria-live="polite" role="status">
                {lastSaved && (
                  <p className="text-xs text-green-700">Saved at {lastSaved}</p>
                )}
                {saving && <p className="text-xs text-blue-700">Saving responses…</p>}
              </div>
            </div>
          </div>
          <div className="mt-3">
            <ProgressBar current={answeredCount} total={allQuestions.length} />
          </div>
        </div>
      </header>

      <div className="max-w-4xl mx-auto px-6 py-6 flex gap-6">
        {/* Section Nav */}
        <nav aria-label="Questionnaire sections" className="w-64 flex-shrink-0 hidden lg:block">
          <div className="sticky top-6 space-y-1">
            {questionnaire?.sections.map((sec, idx) => {
              const sectionAnswered = sec.questions.filter((q) => {
                const a = answers[q.id];
                return a && (Array.isArray(a.value) ? a.value.length > 0 : a.value !== '');
              }).length;
              return (
                <button
                  key={sec.id}
                  type="button"
                  aria-current={activeSectionIdx === idx ? 'step' : undefined}
                  onClick={() => setActiveSectionIdx(idx)}
                  className={`min-h-11 w-full rounded px-3 py-2 text-left text-sm transition-colors motion-reduce:transition-none ${
                    activeSectionIdx === idx
                      ? 'bg-blue-50 text-blue-700 font-medium'
                      : 'text-gray-600 hover:bg-gray-100'
                  }`}
                >
                  <span className="block truncate">{sec.title}</span>
                  <span className="text-xs text-gray-400">
                    {sectionAnswered}/{sec.questions.length}
                  </span>
                </button>
              );
            })}
          </div>
        </nav>

        {/* Questions */}
        <main id="main-content" className="flex-1 space-y-6">
          {activeSection && (
            <>
              <div>
                <h2 className="text-lg font-semibold text-gray-900">{activeSection.title}</h2>
                {activeSection.description && (
                  <p className="text-sm text-gray-500 mt-1">{activeSection.description}</p>
                )}
              </div>

              {activeSection.questions
                .sort((a, b) => a.order - b.order)
                .map((q, qi) => (
                  <fieldset key={q.id} className="space-y-3 rounded-lg border bg-white p-5">
                    <legend className="flex items-start gap-2 text-left text-sm font-medium text-gray-900">
                      <span aria-hidden="true" className="mt-0.5 rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500">
                        {qi + 1}
                      </span>
                      <span className="flex-1">
                          {q.text}
                          {q.required && (
                            <>
                              <span aria-hidden="true" className="ml-1 text-red-600">*</span>
                              <span className="sr-only"> (required)</span>
                            </>
                          )}
                      </span>
                    </legend>
                    {q.description && (
                      <p className="text-xs text-gray-500">{q.description}</p>
                    )}
                    <QuestionInput
                      question={q}
                      answer={answers[q.id]}
                      onChange={(value, files) => handleAnswerChange(q.id, value, files)}
                    />
                  </fieldset>
                ))}

              {/* Section Navigation */}
              <div className="flex items-center justify-between pt-4">
                <button
                  type="button"
                  onClick={() => setActiveSectionIdx(Math.max(0, activeSectionIdx - 1))}
                  disabled={activeSectionIdx === 0}
                  className="min-h-11 rounded border px-4 py-2 text-sm font-medium hover:bg-gray-50 disabled:opacity-50"
                >
                  Previous Section
                </button>
                {activeSectionIdx < (questionnaire?.sections.length ?? 1) - 1 ? (
                  <button
                    type="button"
                    onClick={() => setActiveSectionIdx(activeSectionIdx + 1)}
                    className="min-h-11 rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
                  >
                    Next Section
                  </button>
                ) : null}
              </div>
            </>
          )}

          {/* Mobile section selector */}
          <div className="lg:hidden">
            <select
              aria-label="Assessment section"
              value={activeSectionIdx}
              onChange={(e) => setActiveSectionIdx(Number(e.target.value))}
              className="min-h-11 w-full rounded border px-3 py-2 text-sm"
            >
              {questionnaire?.sections.map((sec, idx) => (
                <option key={sec.id} value={idx}>
                  {sec.title}
                </option>
              ))}
            </select>
          </div>

          {/* Action Buttons */}
          <div className="flex items-center justify-between border-t pt-6">
            <button
              type="button"
              onClick={handleSaveAndContinue}
              disabled={saving}
              className="min-h-11 rounded border px-4 py-2 text-sm font-medium hover:bg-gray-50 disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save & Continue Later'}
            </button>
            <button
              type="button"
              onClick={handleSubmit}
              disabled={submitting}
              className="min-h-11 rounded bg-green-700 px-6 py-2 text-sm font-medium text-white hover:bg-green-800 disabled:opacity-50"
            >
              {submitting ? 'Submitting...' : 'Submit Assessment'}
            </button>
          </div>

          {error && (
            <div role="alert" className="rounded border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              {error}
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page (wrapped in Suspense for useSearchParams)
// ---------------------------------------------------------------------------

export default function VendorPortalPage() {
  return (
    <Suspense
      fallback={
        <main id="main-content" className="min-h-screen bg-gray-50 p-6">
          <h1 className="sr-only">Vendor assessment</h1>
          <ResourceState kind="loading" loadingLayout="detail" title="Loading questionnaire" />
        </main>
      }
    >
      <VendorPortalInner />
    </Suspense>
  );
}
