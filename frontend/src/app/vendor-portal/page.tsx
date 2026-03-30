'use client';

import { useState, useEffect, useCallback } from 'react';
import { useSearchParams } from 'next/navigation';
import { Suspense } from 'react';

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
      <div className="w-full h-2 bg-gray-200 rounded-full">
        <div
          className="h-2 rounded-full bg-blue-600 transition-all"
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
              onClick={() => onChange(opt)}
              className={`px-4 py-2 text-sm rounded border font-medium transition-colors ${
                currentValue === opt
                  ? 'bg-blue-600 text-white border-blue-600'
                  : 'bg-white text-gray-700 border-gray-300 hover:border-blue-400'
              }`}
            >
              {opt}
            </button>
          ))}
          <button
            onClick={() => onChange('N/A')}
            className={`px-4 py-2 text-sm rounded border font-medium transition-colors ${
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
            value={typeof currentValue === 'string' ? currentValue : ''}
            onChange={(e) => onChange(e.target.value, answer?.files)}
            rows={2}
            className="w-full border rounded px-3 py-2 text-sm"
            placeholder="Optional description..."
          />
          <div className="border-2 border-dashed border-gray-300 rounded-lg p-4 text-center">
            <input
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

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [questionnaire, setQuestionnaire] = useState<QuestionnaireData | null>(null);
  const [answers, setAnswers] = useState<AnswerMap>({});
  const [activeSectionIdx, setActiveSectionIdx] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [saving, setSaving] = useState(false);
  const [lastSaved, setLastSaved] = useState<string | null>(null);

  // Token validation & questionnaire fetch
  useEffect(() => {
    if (!token) {
      setError('Invalid or missing access token. Please use the link provided in your email.');
      setLoading(false);
      return;
    }

    async function fetchQuestionnaire() {
      try {
        const res = await fetch(
          `${process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080/api/v1'}/vendor-portal/questionnaire?token=${token}`
        );
        if (!res.ok) {
          throw new Error(res.status === 401 ? 'Token expired or invalid' : 'Failed to load questionnaire');
        }
        const data = await res.json();
        setQuestionnaire(data);
        if (data.saved_answers) {
          setAnswers(data.saved_answers);
        }
      } catch (err: any) {
        setError(err.message ?? 'Failed to load questionnaire');
      } finally {
        setLoading(false);
      }
    }

    fetchQuestionnaire();
  }, [token]);

  // Auto-save
  const autoSave = useCallback(async () => {
    if (!token || !questionnaire || Object.keys(answers).length === 0) return;
    setSaving(true);
    try {
      await fetch(
        `${process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080/api/v1'}/vendor-portal/save?token=${token}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ answers }),
        }
      );
      setLastSaved(new Date().toLocaleTimeString());
    } catch {
      // Silently fail auto-save
    } finally {
      setSaving(false);
    }
  }, [token, questionnaire, answers]);

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
    if (!token || !questionnaire) return;
    setSubmitting(true);
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080/api/v1'}/vendor-portal/submit?token=${token}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ answers }),
        }
      );
      if (!res.ok) throw new Error('Submission failed');
      setSubmitted(true);
    } catch (err: any) {
      setError(err.message ?? 'Submission failed');
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
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-center">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin mx-auto" />
          <p className="mt-4 text-sm text-gray-500">Loading questionnaire...</p>
        </div>
      </div>
    );
  }

  // Error
  if (error && !questionnaire) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="bg-white rounded-lg shadow-lg p-8 max-w-md text-center">
          <div className="w-12 h-12 bg-red-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <span className="text-red-600 text-xl font-bold">!</span>
          </div>
          <h1 className="text-lg font-semibold text-gray-900">Access Error</h1>
          <p className="text-sm text-gray-500 mt-2">{error}</p>
        </div>
      </div>
    );
  }

  // Submitted
  if (submitted) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="bg-white rounded-lg shadow-lg p-8 max-w-md text-center">
          <div className="w-12 h-12 bg-green-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <span className="text-green-600 text-2xl font-bold">{'\u2713'}</span>
          </div>
          <h1 className="text-lg font-semibold text-gray-900">Assessment Submitted</h1>
          <p className="text-sm text-gray-500 mt-2">
            Thank you for completing the assessment. Your responses have been submitted to{' '}
            {questionnaire?.organization_name ?? 'the requesting organization'}.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <header className="bg-white border-b shadow-sm">
        <div className="max-w-4xl mx-auto px-6 py-4">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-lg font-bold text-gray-900">{questionnaire?.name}</h1>
              <p className="text-sm text-gray-500">
                For: {questionnaire?.organization_name} | Vendor: {questionnaire?.vendor_name}
              </p>
            </div>
            <div className="text-right">
              <p className="text-xs text-gray-400">
                Due: {questionnaire?.due_date ? new Date(questionnaire.due_date).toLocaleDateString() : '--'}
              </p>
              {lastSaved && (
                <p className="text-xs text-green-600">Last saved: {lastSaved}</p>
              )}
              {saving && <p className="text-xs text-blue-600">Saving...</p>}
            </div>
          </div>
          <div className="mt-3">
            <ProgressBar current={answeredCount} total={allQuestions.length} />
          </div>
        </div>
      </header>

      <div className="max-w-4xl mx-auto px-6 py-6 flex gap-6">
        {/* Section Nav */}
        <nav className="w-64 flex-shrink-0 hidden lg:block">
          <div className="sticky top-6 space-y-1">
            {questionnaire?.sections.map((sec, idx) => {
              const sectionAnswered = sec.questions.filter((q) => {
                const a = answers[q.id];
                return a && (Array.isArray(a.value) ? a.value.length > 0 : a.value !== '');
              }).length;
              return (
                <button
                  key={sec.id}
                  onClick={() => setActiveSectionIdx(idx)}
                  className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
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
        <main className="flex-1 space-y-6">
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
                  <div key={q.id} className="bg-white border rounded-lg p-5 space-y-3">
                    <div className="flex items-start gap-2">
                      <span className="text-xs bg-gray-100 text-gray-500 px-2 py-0.5 rounded font-mono mt-0.5">
                        {qi + 1}
                      </span>
                      <div className="flex-1">
                        <p className="text-sm font-medium text-gray-900">
                          {q.text}
                          {q.required && <span className="text-red-500 ml-1">*</span>}
                        </p>
                        {q.description && (
                          <p className="text-xs text-gray-500 mt-1">{q.description}</p>
                        )}
                      </div>
                    </div>
                    <QuestionInput
                      question={q}
                      answer={answers[q.id]}
                      onChange={(value, files) => handleAnswerChange(q.id, value, files)}
                    />
                  </div>
                ))}

              {/* Section Navigation */}
              <div className="flex items-center justify-between pt-4">
                <button
                  onClick={() => setActiveSectionIdx(Math.max(0, activeSectionIdx - 1))}
                  disabled={activeSectionIdx === 0}
                  className="px-4 py-2 text-sm font-medium rounded border hover:bg-gray-50 disabled:opacity-50"
                >
                  Previous Section
                </button>
                {activeSectionIdx < (questionnaire?.sections.length ?? 1) - 1 ? (
                  <button
                    onClick={() => setActiveSectionIdx(activeSectionIdx + 1)}
                    className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
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
              value={activeSectionIdx}
              onChange={(e) => setActiveSectionIdx(Number(e.target.value))}
              className="w-full border rounded px-3 py-2 text-sm"
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
              onClick={handleSaveAndContinue}
              disabled={saving}
              className="px-4 py-2 text-sm font-medium rounded border hover:bg-gray-50 disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save & Continue Later'}
            </button>
            <button
              onClick={handleSubmit}
              disabled={submitting}
              className="px-6 py-2 text-sm font-medium rounded bg-green-600 text-white hover:bg-green-700 disabled:opacity-50"
            >
              {submitting ? 'Submitting...' : 'Submit Assessment'}
            </button>
          </div>

          {error && (
            <div className="bg-red-50 border border-red-200 rounded p-3 text-sm text-red-700">
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
        <div className="min-h-screen flex items-center justify-center bg-gray-50">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin" />
        </div>
      }
    >
      <VendorPortalInner />
    </Suspense>
  );
}
