'use client';

import api from '@/lib/api';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export function useIdentityPermissions() {
  const user = useAuthStore((state) => state.user);
  const query = useQuery({
    queryKey: ['access', 'my-permissions', user?.id],
    queryFn: () => api.access.myPermissions(),
    enabled: Boolean(user),
    retry: 1,
    staleTime: 5 * 60_000,
  });
  const permissions = normalizePermissionMap(query.data);
  const allows = (resource: string, action: string) =>
    Boolean(user?.is_super_admin || permissions?.[resource]?.includes(action));
  return {
    canAdminReset: allows('users', 'update'),
    canConfigurePolicy: allows('settings', 'configure'),
    canInvite: allows('users', 'create'),
    canReadAdministration: allows('settings', 'read'),
    canSelfService: allows('users', 'read'),
    isError: query.isError,
    isLoading: Boolean(user) && query.isLoading,
    retry: query.refetch,
  };
}
