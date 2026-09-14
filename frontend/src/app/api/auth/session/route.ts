import type { NextRequest } from 'next/server';

import { handleSession } from '@/lib/server/session-bff';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

export function GET(request: NextRequest) {
  return handleSession(request);
}
