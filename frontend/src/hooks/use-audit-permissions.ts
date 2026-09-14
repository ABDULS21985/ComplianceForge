'use client';

import api from '@/lib/api';
import { hasAuditPermission } from '@/lib/audit';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export function useAuditPermissions() {
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
    user,
    canCreate: hasAuditPermission(permissions, user, 'create'),
    canRead: hasAuditPermission(permissions, user, 'read'),
    canUpdate: hasAuditPermission(permissions, user, 'update'),
    canDelete: hasAuditPermission(permissions, user, 'delete'),
    isLoading: query.isLoading,
    isError: query.isError,
    retry: query.refetch,
  };
}
