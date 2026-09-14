import type { NextRequest } from 'next/server';

import { proxyAuthenticatedRequest } from '@/lib/server/session-bff';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

interface RouteContext {
  params: { path: string[] };
}

function proxy(request: NextRequest, context: RouteContext) {
  return proxyAuthenticatedRequest(request, context.params.path);
}

export const DELETE = proxy;
export const GET = proxy;
export const HEAD = proxy;
export const PATCH = proxy;
export const POST = proxy;
export const PUT = proxy;
