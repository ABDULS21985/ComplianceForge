import {
  buildUpstreamUrl,
  forwardedRequestHeaders,
  PayloadTooLargeError,
  proxyResponse,
  readLimitedRequestBody,
  type ServerFetch,
  serverFetchInit,
} from './upstream';
import { csrfErrorResponse, jsonError, verifyCsrf } from './session-security';
import type { NextRequest } from 'next/server';
import { NextResponse } from 'next/server';
import { portalCookiePolicy } from '@/lib/request-security';

const MAX_PORTAL_BODY_BYTES = 5 * 1024 * 1024;
const MAX_PORTAL_TOKEN_BODY_BYTES = 16 * 1024;
const PORTAL_SESSION_MAX_AGE_SECONDS = 8 * 60 * 60;

type PortalOperation =
  | 'vendor-session'
  | 'vendor-questionnaire'
  | 'vendor-save'
  | 'vendor-submit'
  | 'board-session'
  | 'board-overview';

type PortalKind = 'board' | 'vendor';

function resolveOperation(path: string[]): PortalOperation | null {
  const key = path.join('/');
  if (key === 'vendor-portal/session') return 'vendor-session';
  if (key === 'vendor-portal/questionnaire') return 'vendor-questionnaire';
  if (key === 'vendor-portal/save') return 'vendor-save';
  if (key === 'vendor-portal/submit') return 'vendor-submit';
  if (key === 'board-portal/session') return 'board-session';
  if (key === 'board-portal') return 'board-overview';
  return null;
}

function allowedMethod(operation: PortalOperation, method: string): boolean {
  if (operation === 'vendor-questionnaire' || operation === 'board-overview') {
    return method === 'GET';
  }
  return method === 'POST';
}

function portalKind(operation: PortalOperation): PortalKind {
  return operation.startsWith('vendor-') ? 'vendor' : 'board';
}

function validPortalToken(value: unknown): string | null {
  const token = typeof value === 'string' ? value.trim() : '';
  if (!token || token.length > 4096 || /[\u0000-\u001f]/.test(token)) return null;
  return token;
}

async function exchangeToken(request: NextRequest): Promise<string | null> {
  const bytes = await readLimitedRequestBody(request, MAX_PORTAL_TOKEN_BODY_BYTES);
  const parsed: unknown = JSON.parse(new TextDecoder().decode(bytes));
  if (!parsed || typeof parsed !== 'object') return null;
  return validPortalToken((parsed as Record<string, unknown>).token);
}

function setPortalSessionCookie(
  request: NextRequest,
  response: NextResponse,
  portal: PortalKind,
  token: string,
): void {
  const policy = portalCookiePolicy(request, portal);
  response.cookies.set(policy.name, token, {
    httpOnly: true,
    maxAge: PORTAL_SESSION_MAX_AGE_SECONDS,
    path: `/api/portal/${portal}-portal`,
    sameSite: 'strict',
    secure: policy.secure,
  });
}

function clearPortalSessionCookie(
  request: NextRequest,
  response: NextResponse,
  portal: PortalKind,
): void {
  const policy = portalCookiePolicy(request, portal);
  response.cookies.set(policy.name, '', {
    expires: new Date(0),
    httpOnly: true,
    maxAge: 0,
    path: `/api/portal/${portal}-portal`,
    sameSite: 'strict',
    secure: policy.secure,
  });
}

function normalizeQuestionnaire(value: unknown): unknown {
  if (!value || typeof value !== 'object') return value;
  const questionnaire = value as Record<string, unknown>;
  const existing = Array.isArray(questionnaire.existing_responses)
    ? questionnaire.existing_responses
    : [];
  const savedAnswers: Record<string, { value: string | string[] }> = {};

  for (const item of existing) {
    if (!item || typeof item !== 'object') continue;
    const response = item as Record<string, unknown>;
    if (typeof response.question_id !== 'string') continue;
    savedAnswers[response.question_id] = {
      value: Array.isArray(response.answers)
        ? response.answers.filter((answer): answer is string => typeof answer === 'string')
        : typeof response.answer === 'string'
          ? response.answer
          : '',
    };
  }

  const sections = Array.isArray(questionnaire.sections)
    ? questionnaire.sections.map((section) => {
        if (!section || typeof section !== 'object') return section;
        const entry = section as Record<string, unknown>;
        const questions = Array.isArray(entry.questions)
          ? entry.questions.map((question) => {
              if (!question || typeof question !== 'object') return question;
              const detail = question as Record<string, unknown>;
              return {
                ...detail,
                description: detail.description ?? detail.help_text,
                type: detail.type === 'multi_choice' ? 'multi_select' : detail.type,
              };
            })
          : [];
        return { ...entry, questions };
      })
    : [];

  return {
    ...questionnaire,
    id: questionnaire.id ?? questionnaire.assessment_id,
    name: questionnaire.name ?? questionnaire.title,
    organization_name: questionnaire.organization_name ?? '',
    saved_answers: savedAnswers,
    sections,
  };
}

function normalizeBoardOverview(value: unknown): unknown {
  if (!value || typeof value !== 'object') return value;
  const overview = value as Record<string, unknown>;
  return {
    ...overview,
    organization_name: overview.organization_name ?? '',
    upcoming_meetings:
      typeof overview.upcoming_meetings === 'number' ? overview.upcoming_meetings : 0,
    unread_reports: typeof overview.unread_reports === 'number' ? overview.unread_reports : 0,
    compliance_score:
      typeof overview.compliance_score === 'number' ? overview.compliance_score : null,
    risk_appetite_score:
      typeof overview.risk_appetite_score === 'number' ? overview.risk_appetite_score : null,
    pending_decisions_count:
      typeof overview.pending_decisions === 'number' ? overview.pending_decisions : 0,
    key_alerts: Array.isArray(overview.key_alerts) ? overview.key_alerts : [],
    pending_decisions: Array.isArray(overview.pending_decisions) ? overview.pending_decisions : [],
    board_packs: Array.isArray(overview.board_packs) ? overview.board_packs : [],
    decision_follow_ups: Array.isArray(overview.decision_follow_ups)
      ? overview.decision_follow_ups
      : [],
  };
}

function normalizeAnswers(value: unknown): { responses: unknown[] } | null {
  if (!value || typeof value !== 'object') return null;
  const answers = (value as Record<string, unknown>).answers;
  if (!answers || typeof answers !== 'object' || Array.isArray(answers)) {
    return null;
  }

  const responses: Array<{
    question_id: string;
    answer?: string;
    answers?: string[];
  }> = [];
  for (const [questionId, raw] of Object.entries(answers)) {
    if (!raw || typeof raw !== 'object') continue;
    const value = (raw as Record<string, unknown>).value;
    if (typeof value === 'string') {
      responses.push({ question_id: questionId, answer: value });
    } else if (Array.isArray(value) && value.every((item) => typeof item === 'string')) {
      responses.push({ question_id: questionId, answers: value });
    }
  }

  return { responses };
}

async function compatibilityResponse(
  upstream: Response,
  operation: PortalOperation,
): Promise<NextResponse> {
  if (!upstream.ok) return proxyResponse(upstream);
  if (
    !['vendor-session', 'vendor-questionnaire', 'board-session', 'board-overview'].includes(
      operation,
    )
  ) {
    return proxyResponse(upstream);
  }

  const data: unknown = await upstream.json().catch(() => null);
  if (data === null) {
    return jsonError(
      502,
      'INVALID_UPSTREAM_RESPONSE',
      'Portal service returned an invalid response',
    );
  }

  return NextResponse.json(
    operation === 'vendor-session' || operation === 'vendor-questionnaire'
      ? normalizeQuestionnaire(data)
      : normalizeBoardOverview(data),
    {
      status: upstream.status,
      headers: {
        'Cache-Control': 'no-store',
        'X-Content-Type-Options': 'nosniff',
      },
    },
  );
}

export async function proxyPortalRequest(
  request: NextRequest,
  path: string[],
  fetcher: ServerFetch = fetch,
): Promise<NextResponse> {
  const operation = resolveOperation(path);
  if (!operation) {
    return jsonError(404, 'ROUTE_NOT_FOUND', 'Portal route not found');
  }
  if (!allowedMethod(operation, request.method)) {
    const response = jsonError(405, 'METHOD_NOT_ALLOWED', 'Method not allowed');
    response.headers.set(
      'Allow',
      operation === 'vendor-questionnaire' || operation === 'board-overview'
        ? 'GET'
        : 'POST',
    );
    return response;
  }

  if (request.method !== 'GET') {
    const csrf = verifyCsrf(request);
    if (!csrf.ok) return csrfErrorResponse(csrf.message);
  }

  const kind = portalKind(operation);
  const isSessionExchange = operation === 'vendor-session' || operation === 'board-session';
  let token: string | null;
  if (isSessionExchange) {
    try {
      token = await exchangeToken(request);
    } catch (error) {
      if (error instanceof PayloadTooLargeError) {
        return jsonError(413, 'PAYLOAD_TOO_LARGE', error.message);
      }
      return jsonError(400, 'INVALID_PORTAL_TOKEN', 'Portal token must be valid JSON');
    }
  } else {
    const cookieName = portalCookiePolicy(request, kind).name;
    token = validPortalToken(request.cookies.get(cookieName)?.value);
  }
  if (!token) {
    return jsonError(
      isSessionExchange ? 400 : 401,
      isSessionExchange ? 'INVALID_PORTAL_TOKEN' : 'PORTAL_SESSION_REQUIRED',
      isSessionExchange ? 'Portal token is required' : 'Portal session is missing or expired',
    );
  }
  const encodedToken = encodeURIComponent(token);

  let pathname: string;
  let method: string;
  let body: BodyInit | undefined;
  if (operation === 'vendor-session' || operation === 'vendor-questionnaire') {
    pathname = `/vendor-portal/${encodedToken}`;
    method = 'GET';
  } else if (operation === 'vendor-save') {
    pathname = `/vendor-portal/${encodedToken}/responses`;
    method = 'PUT';
    try {
      const bytes = await readLimitedRequestBody(request, MAX_PORTAL_BODY_BYTES);
      const parsed: unknown = JSON.parse(new TextDecoder().decode(bytes));
      const normalized = normalizeAnswers(parsed);
      if (!normalized || normalized.responses.length === 0) {
        return jsonError(400, 'INVALID_REQUEST_BODY', 'At least one answer is required');
      }
      body = JSON.stringify(normalized);
    } catch (error) {
      if (error instanceof PayloadTooLargeError) {
        return jsonError(413, 'PAYLOAD_TOO_LARGE', error.message);
      }
      return jsonError(400, 'INVALID_REQUEST_BODY', 'Request body must be valid JSON');
    }
  } else if (operation === 'vendor-submit') {
    pathname = `/vendor-portal/${encodedToken}/submit`;
    method = 'POST';
  } else {
    pathname = `/board-portal/${encodedToken}`;
    method = 'GET';
  }

  const headers = forwardedRequestHeaders(request);
  if (body) headers.set('Content-Type', 'application/json');
  else headers.delete('Content-Type');

  try {
    const upstream = await fetcher(
      buildUpstreamUrl(pathname),
      serverFetchInit({ method, headers, body }),
    );
    const response = await compatibilityResponse(upstream, operation);
    if (response.ok && isSessionExchange) {
      setPortalSessionCookie(request, response, kind, token);
    } else if ([401, 403, 404, 410].includes(response.status)) {
      clearPortalSessionCookie(request, response, kind);
    }
    return response;
  } catch {
    return jsonError(502, 'UPSTREAM_UNAVAILABLE', 'Portal service is unavailable');
  }
}
