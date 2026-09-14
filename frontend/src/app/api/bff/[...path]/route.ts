import type { NextRequest } from 'next/server';

import { proxyAuthenticatedRequest } from '@/lib/server/session-bff';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

interface RouteContext {
  params: Promise<{ path: string[] }>;
}

async function handleRequest(request: NextRequest, context: RouteContext) {
  const { path } = await context.params;
  return proxyAuthenticatedRequest(request, path);
}

export const DELETE = handleRequest;
export const GET = handleRequest;
export const HEAD = handleRequest;
export const PATCH = handleRequest;
export const POST = handleRequest;
export const PUT = handleRequest;
