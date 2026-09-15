import { handlePublicIdentityRoute } from '@/lib/server/identity-bff';
import type { NextRequest } from 'next/server';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

interface RouteContext {
  params: Promise<{ identity: string[] }>;
}

export async function POST(request: NextRequest, context: RouteContext) {
  const { identity } = await context.params;
  return handlePublicIdentityRoute(request, identity);
}
