'use client';

import { useQuery, useQueryClient } from '@tanstack/react-query';
import api from './api';
import { isCanonicalOrganizationId } from './organization-profile';
import { normalizePermissionMap } from './navigation';
import { useLayoutEffect } from 'react';

export const organizationProfileKeys = {
  permissions: (tenantId: string, actorId: string) => ['access', 'my-permissions', 'organization-profile', tenantId, actorId] as const,
  profile: (tenantId: string, actorId: string, canConfigure = false) => ['settings', 'organization-profile', tenantId, actorId, canConfigure ? 'configure' : 'read'] as const,
};

export function useOrganizationProfileAccess(tenantId: string, actorId: string) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: organizationProfileKeys.permissions(tenantId, actorId),
    queryFn: () => api.access.myPermissions(),
    enabled: isCanonicalOrganizationId(tenantId) && isCanonicalOrganizationId(actorId),
    gcTime: 0, staleTime: 30_000, retry: false,
    refetchInterval: 30_000, refetchIntervalInBackground: false,
  });
  useLayoutEffect(() => () => {
    const key = organizationProfileKeys.permissions(tenantId, actorId);
    void client.cancelQueries({ queryKey: key, exact: true });
    client.removeQueries({ queryKey: key, exact: true });
  }, [actorId, client, tenantId]);
  const permissions = normalizePermissionMap(query.data) ?? {};
  return { ...query, canRead: !query.isError && permissions.settings?.includes('read') === true,
    canConfigure: !query.isError && permissions.settings?.includes('configure') === true };
}

export function useOrganizationProfile(tenantId: string, actorId: string, canConfigure: boolean) {
  const client = useQueryClient();
  const key = organizationProfileKeys.profile(tenantId, actorId, canConfigure);
  const query = useQuery({
    queryKey: key, queryFn: ({ signal }) => api.settings.getOrg(signal),
    gcTime: 0, staleTime: 30_000, retry: false, refetchOnWindowFocus: false,
  });
  useLayoutEffect(() => () => {
    void client.cancelQueries({ queryKey: organizationProfileKeys.profile(tenantId, actorId, canConfigure), exact: true });
    client.removeQueries({ queryKey: organizationProfileKeys.profile(tenantId, actorId, canConfigure), exact: true });
  }, [actorId, canConfigure, client, tenantId]);
  return query;
}
