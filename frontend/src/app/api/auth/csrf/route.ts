import type { NextRequest } from 'next/server';
import { NextResponse } from 'next/server';

import { sessionCookiePolicy } from '@/lib/request-security';
import { createCsrfToken, setCsrfCookie } from '@/lib/server/session-security';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

export function GET(request: NextRequest): NextResponse {
  const fetchSite = request.headers.get('sec-fetch-site');
  if (fetchSite && fetchSite !== 'same-origin') {
    return NextResponse.json(
      { code: 'CROSS_ORIGIN_REQUEST', message: 'Cross-origin request rejected' },
      { status: 403, headers: { 'Cache-Control': 'no-store' } },
    );
  }

  const cookieName = sessionCookiePolicy(request).csrf;
  const existing = request.cookies.get(cookieName)?.value;
  const csrfToken =
    existing && /^[A-Za-z0-9_-]{43}$/.test(existing)
      ? existing
      : createCsrfToken();
  const response = NextResponse.json(
    { csrf_token: csrfToken },
    {
      headers: {
        'Cache-Control': 'no-store',
        'Referrer-Policy': 'same-origin',
        'X-Content-Type-Options': 'nosniff',
      },
    },
  );
  if (csrfToken !== existing) setCsrfCookie(request, response, csrfToken);
  return response;
}
