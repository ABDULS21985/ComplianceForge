'use client';

import api from '@/lib/api';
import { hasIncidentPermission } from '@/lib/incident';
import { normalizePermissionMap } from '@/lib/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';

export function useIncidentPermissions() {
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
    canCreate: hasIncidentPermission(permissions, user, 'create'),
    canRead: hasIncidentPermission(permissions, user, 'read'),
    canUpdate: hasIncidentPermission(permissions, user, 'update'),
    canDelete: hasIncidentPermission(permissions, user, 'delete'),
    canApprove: hasIncidentPermission(permissions, user, 'approve'),
    canAssign: hasIncidentPermission(permissions, user, 'assign'),
    isLoading: query.isLoading,
    isError: query.isError,
    retry: query.refetch,
  };
}
