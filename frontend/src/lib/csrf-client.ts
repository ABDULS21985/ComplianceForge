import { CSRF_ERROR_HEADER, CSRF_TOKEN_HEADER, isStateChangingMethod } from '@/lib/auth-constants';

let csrfToken: string | null = null;
let csrfRequest: Promise<string> | null = null;

export function resetCsrfToken(): void {
  csrfToken = null;
  csrfRequest = null;
}

export async function getCsrfToken(): Promise<string> {
  if (csrfToken) return csrfToken;
  if (csrfRequest) return csrfRequest;

  csrfRequest = fetch('/api/auth/csrf', {
    cache: 'no-store',
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  })
    .then(async (response) => {
      if (!response.ok) {
        throw new Error('Unable to initialize request protection');
      }
      const payload = (await response.json()) as Record<string, unknown>;
      if (typeof payload.csrf_token !== 'string' || !payload.csrf_token) {
        throw new Error('Invalid request-protection response');
      }
      csrfToken = payload.csrf_token;
      return csrfToken;
    })
    .finally(() => {
      csrfRequest = null;
    });

  return csrfRequest;
}

export async function fetchWithCsrf(
  input: RequestInfo | URL,
  init: RequestInit = {},
): Promise<Response> {
  const method = (init.method ?? 'GET').toUpperCase();
  const headers = new Headers(init.headers);

  if (isStateChangingMethod(method)) {
    headers.set(CSRF_TOKEN_HEADER, await getCsrfToken());
  }

  const response = await fetch(input, {
    ...init,
    credentials: 'same-origin',
    headers,
  });

  if (
    isStateChangingMethod(method) &&
    response.status === 403 &&
    response.headers.get(CSRF_ERROR_HEADER) === '1'
  ) {
    resetCsrfToken();
    headers.set(CSRF_TOKEN_HEADER, await getCsrfToken());
    return fetch(input, {
      ...init,
      credentials: 'same-origin',
      headers,
    });
  }

  return response;
}
