import {
  buildUpstreamUrl,
  forwardedRequestHeaders,
  PayloadTooLargeError,
  proxyResponse,
  readLimitedRequestBody,
  type ServerFetch,
  serverFetchInit,
} from '@/lib/server/upstream';
import { csrfErrorResponse, jsonError, verifyCsrf } from '@/lib/server/session-security';
import { handleCredentialExchange } from '@/lib/server/session-bff';
import type { NextRequest } from 'next/server';
import { NextResponse } from 'next/server';

const MAX_PUBLIC_IDENTITY_BODY_BYTES = 256 * 1024;

const PUBLIC_IDENTITY_PATHS = new Set([
  'email-verification/confirm',
  'email-verification/request',
  'invitations/accept',
  'password/forgot',
  'password/reset',
  'passkeys/authentication/options',
]);

const SESSION_IDENTITY_PATHS = new Set(['mfa/verify', 'passkeys/authentication/verify']);

export async function handlePublicIdentityRoute(
  request: NextRequest,
  path: string[],
  fetcher: ServerFetch = fetch,
): Promise<NextResponse> {
  const operation = path.join('/');
  if (SESSION_IDENTITY_PATHS.has(operation)) {
    return handleCredentialExchange(
      request,
      operation as 'mfa/verify' | 'passkeys/authentication/verify',
      fetcher,
    );
  }
  if (!PUBLIC_IDENTITY_PATHS.has(operation)) {
    return jsonError(404, 'ROUTE_NOT_FOUND', 'Authentication route not found');
  }

  const csrf = verifyCsrf(request);
  if (!csrf.ok) return csrfErrorResponse(csrf.message);

  let body: ArrayBuffer | undefined;
  try {
    body = await readLimitedRequestBody(request, MAX_PUBLIC_IDENTITY_BODY_BYTES);
  } catch (error) {
    if (error instanceof PayloadTooLargeError) {
      return jsonError(413, 'PAYLOAD_TOO_LARGE', error.message);
    }
    return jsonError(400, 'INVALID_REQUEST_BODY', 'Unable to read request body');
  }

  let upstream: Response;
  try {
    upstream = await fetcher(
      buildUpstreamUrl(`/auth/${operation}`),
      serverFetchInit({
        method: 'POST',
        headers: forwardedRequestHeaders(request),
        body,
      }),
    );
  } catch {
    return jsonError(502, 'UPSTREAM_UNAVAILABLE', 'Identity service is unavailable');
  }
  return proxyResponse(upstream);
}
