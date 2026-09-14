'use client';

import { useEffect, useSyncExternalStore } from 'react';
import api, { type ApiError } from '@/lib/api';
import { normalizePermissionMap } from '@/lib/navigation';
import { ROUTES } from '@/lib/routes';
import { Sidebar } from '@/components/layout/sidebar';
import { Topbar } from '@/components/layout/topbar';
import { useAuthStore } from '@/store/auth-store';
import { useQuery } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const { user, isAuthenticated: storeIsAuth, clearAuth, logout, setAuth } = useAuthStore();
  const mounted = useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false,
  );

  const sessionQuery = useQuery({
    queryKey: ['auth', 'session'],
    queryFn: () => api.auth.me(),
    enabled: mounted && !storeIsAuth,
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

  const sessionReady = storeIsAuth || sessionQuery.isSuccess;
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

  if (!mounted || !sessionReady) {
    if (mounted && sessionQuery.isError) {
      return (
        <div className="flex min-h-screen items-center justify-center p-6">
          <div className="max-w-md space-y-3 text-center">
            <h1 className="text-lg font-semibold">Unable to verify your session</h1>
            <p className="text-sm text-muted-foreground">
              The authentication service is temporarily unavailable. Your session has not been
              discarded.
            </p>
            <button
              type="button"
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground"
              onClick={() => void sessionQuery.refetch()}
            >
              Try again
            </button>
          </div>
        </div>
      );
    }

    return (
      <div
        role="status"
        aria-label="Loading application"
        className="flex min-h-screen items-center justify-center"
      >
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
      </div>
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
      <Sidebar permissions={permissions} user={sessionUser} />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Topbar permissions={permissions} user={sessionUser} onLogout={logout} />
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
