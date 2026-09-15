import { QUICK_CREATE_ROUTES } from '@/lib/routes';
import { redirect } from 'next/navigation';

export default function LegacyNewPolicyPage() {
  redirect(QUICK_CREATE_ROUTES.policy);
}
