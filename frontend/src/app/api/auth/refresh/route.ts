import type { NextRequest } from 'next/server';

import { handleRefresh } from '@/lib/server/session-bff';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

export function POST(request: NextRequest) {
  return handleRefresh(request);
}
