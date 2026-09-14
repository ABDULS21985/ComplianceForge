import type { NextRequest } from 'next/server';

import { handleCredentialExchange } from '@/lib/server/session-bff';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

export function POST(request: NextRequest) {
  return handleCredentialExchange(request, 'login');
}
