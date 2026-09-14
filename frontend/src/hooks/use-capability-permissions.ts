'use client';

import api from '@/lib/api';
import { hasSettingsPermission } from '@/lib/enterprise-settings';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export function useCapabilityPermissions() {
  const user = useAuthStore((state) => state.user);
  const query = useQuery({
    queryKey: ['access', 'my-permissions', user?.id],
    queryFn: () => api.access.myPermissions(),
    enabled: Boolean(user),
    retry: 1,
    staleTime: 5 * 60_000,
  });
  const permissions = normalizePermissionMap(query.data);
  return {
    canConfigure: hasSettingsPermission(permissions, user, 'configure'),
    canRead: hasSettingsPermission(permissions, user, 'read'),
    isError: query.isError,
    isLoading: query.isLoading,
    retry: query.refetch,
  };
}
