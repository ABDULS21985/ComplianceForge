import type { NextRequest } from 'next/server';

import { proxyPortalRequest } from '@/lib/server/portal-bff';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

interface RouteContext {
  params: Promise<{ path: string[] }>;
}

async function handleRequest(request: NextRequest, context: RouteContext) {
  const { path } = await context.params;
  return proxyPortalRequest(request, path);
}

export const GET = handleRequest;
export const POST = handleRequest;
