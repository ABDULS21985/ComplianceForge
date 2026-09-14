import { redirect } from 'next/navigation';

import { QUICK_CREATE_ROUTES } from '@/lib/routes';

export default function LegacyNewRiskPage() {
  redirect(QUICK_CREATE_ROUTES.risk);
}
