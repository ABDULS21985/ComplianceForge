import type { NextRequest } from 'next/server';

import { proxyPortalRequest } from '@/lib/server/portal-bff';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

interface RouteContext {
  params: { path: string[] };
}

function proxy(request: NextRequest, context: RouteContext) {
  return proxyPortalRequest(request, context.params.path);
}

export const GET = proxy;
export const POST = proxy;
