'use client';

import api from '@/lib/api';
import { hasControlPermission } from '@/lib/control-evidence';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export function useControlPermissions() {
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
    canApprove: hasControlPermission(permissions, user, 'approve'),
    canExport: hasControlPermission(permissions, user, 'export'),
    canRead: hasControlPermission(permissions, user, 'read'),
    canUpdate: hasControlPermission(permissions, user, 'update'),
    isError: query.isError,
    isLoading: query.isLoading,
    retry: query.refetch,
  };
}
