import { handleLogout } from '@/lib/server/session-bff';
import type { NextRequest } from 'next/server';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

export function POST(request: NextRequest) {
  return handleLogout(request);
}
