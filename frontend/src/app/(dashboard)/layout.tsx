'use client';

import api, { type ApiError } from '@/lib/api';
import { DATA_GOVERNANCE_CAPABILITY, dataGovernanceKeys } from '@/lib/data-governance';
import { useEffect, useSyncExternalStore } from 'react';
import { normalizePermissionMap } from '@/lib/navigation';
import { ResourceState } from '@/components/data/resource-state';
import { ROUTES } from '@/lib/routes';
import { Sidebar } from '@/components/layout/sidebar';
import { Topbar } from '@/components/layout/topbar';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const { user, isAuthenticated: storeIsAuth, isLoggingOut, clearAuth, logout, setAuth } = useAuthStore();
  const mounted = useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false,
  );

  const sessionQuery = useQuery({
    queryKey: ['auth', 'session'],
    queryFn: () => api.auth.me(),
    enabled: mounted && !storeIsAuth && !isLoggingOut,
    retry: false,
    staleTime: 5 * 60_000,
  });

  useEffect(() => {
    if (sessionQuery.data) setAuth(sessionQuery.data);
  }, [sessionQuery.data, setAuth]);

  useEffect(() => {
    const error = sessionQuery.error as ApiError | null;
    if (error?.status === 401) {
      clearAuth();
      router.replace(ROUTES.auth.login);
    }
  }, [clearAuth, router, sessionQuery.error]);

  const sessionReady = !isLoggingOut && (storeIsAuth || sessionQuery.isSuccess);
  const sessionUser = user ?? sessionQuery.data ?? null;

  const permissionsQuery = useQuery({
    queryKey: ['access', 'my-permissions', user?.id],
    queryFn: () => api.access.myPermissions(),
    enabled: mounted && sessionReady,
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const permissions = permissionsQuery.isPending
    ? {}
    : normalizePermissionMap(permissionsQuery.data);
  const dataLifecycleQuery = useQuery({
    queryKey: dataGovernanceKeys.capability,
    queryFn: () => api.featureFlags.evaluate(DATA_GOVERNANCE_CAPABILITY),
    enabled: mounted && sessionReady,
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const enabledCapabilities = dataLifecycleQuery.data?.enabled
    ? [DATA_GOVERNANCE_CAPABILITY]
    : [];

  if (!mounted || !sessionReady) {
    if (mounted && sessionQuery.isError && !isLoggingOut) {
      return (
        <main id="main-content" className="flex min-h-screen items-center justify-center p-6">
          <ResourceState
            headingLevel={1}
            kind="error"
            title="Unable to verify your session"
            description="The authentication service is temporarily unavailable. Your session has not been discarded."
            onRetry={() => void sessionQuery.refetch()}
          />
        </main>
      );
    }

    return (
      <main id="main-content" className="flex min-h-screen items-center justify-center">
        <h1 className="sr-only">{isLoggingOut ? 'Signing out' : 'Loading application'}</h1>
        <ResourceState kind="loading" loadingLayout="inline" title={isLoggingOut ? 'Signing out' : 'Loading application'} />
      </main>
    );
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <a
        href="#main-content"
        className="sr-only z-[100] rounded-md bg-background px-4 py-2 focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:ring-2 focus:ring-ring"
      >
        Skip to main content
      </a>
      <Sidebar enabledCapabilities={enabledCapabilities} permissions={permissions} user={sessionUser} />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Topbar enabledCapabilities={enabledCapabilities} permissions={permissions} user={sessionUser} onLogout={logout} />
        <main
          id="main-content"
          tabIndex={-1}
          className="flex-1 overflow-y-auto p-4 outline-none md:p-6"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
