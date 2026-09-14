'use client';

import api from '@/lib/api';
import { hasAssetPermission } from '@/lib/asset';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export function useAssetPermissions() {
  const user = useAuthStore((state) => state.user);
  const query = useQuery({ queryKey: ['access', 'my-permissions', user?.id], queryFn: () => api.access.myPermissions(), enabled: Boolean(user), staleTime: 5 * 60_000, retry: 1 });
  const permissions = normalizePermissionMap(query.data);
  return {
    user,
    canCreate: hasAssetPermission(permissions, user, 'create'),
    canRead: hasAssetPermission(permissions, user, 'read'),
    canUpdate: hasAssetPermission(permissions, user, 'update'),
    canDelete: hasAssetPermission(permissions, user, 'delete'),
    isLoading: query.isLoading,
    isError: query.isError,
    retry: query.refetch,
  };
}
