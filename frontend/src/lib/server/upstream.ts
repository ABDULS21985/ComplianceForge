import type { NextRequest } from 'next/server';
import { NextResponse } from 'next/server';

const DEFAULT_INTERNAL_API_URL = 'http://localhost:8080/api/v1';

const REQUEST_HEADER_ALLOWLIST = [
  'accept',
  'content-type',
  'if-match',
  'if-none-match',
  'range',
  'x-request-id',
] as const;

const RESPONSE_HEADER_ALLOWLIST = [
  'accept-ranges',
  'content-disposition',
  'content-range',
  'content-type',
  'etag',
  'last-modified',
  'retry-after',
  'x-request-id',
] as const;

export type ServerFetch = (input: string | URL | Request, init?: RequestInit) => Promise<Response>;

export class PayloadTooLargeError extends Error {}

/** Read a replayable request body while enforcing the limit during streaming. */
export async function readLimitedRequestBody(
  request: NextRequest,
  maximumBytes: number,
): Promise<ArrayBuffer | undefined> {
  if (request.method === 'GET' || request.method === 'HEAD' || !request.body) {
    return undefined;
  }

  const declaredLength = Number(request.headers.get('content-length'));
  if (Number.isFinite(declaredLength) && declaredLength > maximumBytes) {
    throw new PayloadTooLargeError('Request body is too large');
  }

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let totalBytes = 0;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    totalBytes += value.byteLength;
    if (totalBytes > maximumBytes) {
      await reader.cancel().catch(() => undefined);
      throw new PayloadTooLargeError('Request body is too large');
    }
    chunks.push(value);
  }

  const combined = new Uint8Array(totalBytes);
  let offset = 0;
  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return combined.buffer;
}

export function internalApiBaseUrl(): string {
  const configured = process.env.API_INTERNAL_URL?.trim() || DEFAULT_INTERNAL_API_URL;
  const parsed = new URL(configured);

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('API_INTERNAL_URL must use http or https');
  }
  if (parsed.username || parsed.password) {
    throw new Error('API_INTERNAL_URL must not contain credentials');
  }
  if (parsed.search || parsed.hash) {
    throw new Error('API_INTERNAL_URL must not contain a query string or fragment');
  }
  if (/\/{2,}/.test(parsed.pathname)) {
    throw new Error('API_INTERNAL_URL path must not contain empty segments');
  }

  return parsed.toString().replace(/\/+$/, '');
}

export function buildUpstreamUrl(pathname: string, search = ''): string {
  if (!pathname.startsWith('/') || pathname.startsWith('//')) {
    throw new Error('Invalid upstream path');
  }

  const url = new URL(`${internalApiBaseUrl()}${pathname}`);
  url.search = search.startsWith('?') ? search.slice(1) : search;
  return url.toString();
}

export function forwardedRequestHeaders(request: NextRequest): Headers {
  const headers = new Headers();
  for (const name of REQUEST_HEADER_ALLOWLIST) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  return headers;
}

/** Convert an upstream response without ever forwarding its cookies. */
export function proxyResponse(upstream: Response): NextResponse {
  const headers = new Headers({
    'Cache-Control': 'no-store',
    'X-Content-Type-Options': 'nosniff',
  });

  for (const name of RESPONSE_HEADER_ALLOWLIST) {
    const value = upstream.headers.get(name);
    if (value) headers.set(name, value);
  }

  return new NextResponse(
    upstream.status === 204 || upstream.status === 304 ? null : upstream.body,
    {
      status: upstream.status,
      statusText: upstream.statusText,
      headers,
    },
  );
}

export function serverFetchInit(init: RequestInit): RequestInit {
  return {
    ...init,
    cache: 'no-store',
    redirect: 'manual',
  };
}
