'use client';

import api from '@/lib/api';
import { hasAccessAdministrationPermission } from '@/lib/access-admin';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export function useAccessAdminPermissions() {
  const user = useAuthStore((state) => state.user);
  const query = useQuery({
    queryKey: ['access', 'my-permissions', user?.id],
    queryFn: () => api.access.myPermissions(),
    enabled: Boolean(user),
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const permissions = normalizePermissionMap(query.data);
  return {
    canConfigure: hasAccessAdministrationPermission(permissions, user, 'configure'),
    canRead: hasAccessAdministrationPermission(permissions, user, 'read'),
    isError: query.isError,
    isLoading: query.isLoading,
    retry: query.refetch,
  };
}
