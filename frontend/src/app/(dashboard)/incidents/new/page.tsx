import { QUICK_CREATE_ROUTES } from '@/lib/routes';
import { redirect } from 'next/navigation';

export default function LegacyNewIncidentPage() {
  redirect(QUICK_CREATE_ROUTES.incident);
}
