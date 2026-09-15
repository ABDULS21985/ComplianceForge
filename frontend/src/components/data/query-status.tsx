'use client';

import { AlertTriangle, LockKeyhole, RefreshCw } from 'lucide-react';
import { useCallback, useSyncExternalStore } from 'react';
import { Button } from '@/components/ui/button';
import { resourceErrorStatus } from '@/lib/resource-errors';
import { useOnlineStatus } from '@/hooks/use-online-status';
import { useQueryClient } from '@tanstack/react-query';

interface QueryStatusSnapshot {
  failed: number;
  fetching: number;
  forbidden: number;
  stale: number;
}

const EMPTY_SNAPSHOT = JSON.stringify({
  failed: 0,
  fetching: 0,
  forbidden: 0,
  stale: 0,
} satisfies QueryStatusSnapshot);

/**
 * Announces active query work and provides a safe recovery path for legacy pages.
 * It deliberately reports counts only and never renders server error details.
 */
export function QueryStatus() {
  const online = useOnlineStatus();
  const queryClient = useQueryClient();
  const queryCache = queryClient.getQueryCache();

  const subscribe = useCallback(
    (onStoreChange: () => void) => queryCache.subscribe(onStoreChange),
    [queryCache],
  );
  const getSnapshot = useCallback(() => {
    const activeQueries = queryCache
      .getAll()
      .filter((query) => query.getObserversCount() > 0);
    const failures = activeQueries.filter(
      (query) => query.state.status === 'error',
    );

    return JSON.stringify({
      failed: failures.length,
      fetching: activeQueries.filter(
        (query) => query.state.fetchStatus === 'fetching',
      ).length,
      forbidden: failures.filter((query) => resourceErrorStatus(query.state.error) === 403)
        .length,
      stale: failures.filter((query) => query.state.data !== undefined).length,
    } satisfies QueryStatusSnapshot);
  }, [queryCache]);
  const serialized = useSyncExternalStore(
    subscribe,
    getSnapshot,
    () => EMPTY_SNAPSHOT,
  );
  const snapshot = JSON.parse(serialized) as QueryStatusSnapshot;

  const retryFailedQueries = () => {
    void queryClient.refetchQueries({
      predicate: (query) =>
        query.getObserversCount() > 0 &&
        query.state.status === 'error' &&
        resourceErrorStatus(query.state.error) !== 403,
      type: 'active',
    });
  };

  const onlyForbidden =
    snapshot.failed > 0 && snapshot.failed === snapshot.forbidden;

  return (
    <>
      <div aria-atomic="true" aria-live="polite" className="sr-only" role="status">
        {snapshot.fetching > 0
          ? `Loading ${snapshot.fetching} information ${snapshot.fetching === 1 ? 'source' : 'sources'}.`
          : ''}
      </div>
      {snapshot.failed > 0 && online && (
        <div
          aria-labelledby="global-query-error-title"
          className="relative z-[90] mx-4 my-4 flex max-w-2xl flex-col gap-3 rounded-lg border border-destructive/40 bg-background p-4 shadow-lg sm:mx-auto sm:flex-row sm:items-center sm:justify-between"
          role="alert"
        >
          <div className="flex items-start gap-3">
            {onlyForbidden ? (
              <LockKeyhole aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0" />
            ) : (
              <AlertTriangle aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0" />
            )}
            <div>
              <p id="global-query-error-title" className="font-medium">
                {onlyForbidden
                  ? 'Some information is restricted'
                  : snapshot.stale > 0
                    ? 'Some saved information may be out of date'
                    : 'Some page information could not be loaded'}
              </p>
              <p className="text-sm text-muted-foreground">
                {onlyForbidden
                  ? 'Your current permissions do not allow this information to be displayed.'
                  : 'No diagnostic details were exposed. Retry the active page requests.'}
              </p>
            </div>
          </div>
          {!onlyForbidden && (
            <Button type="button" variant="outline" onClick={retryFailedQueries}>
              <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
              Retry page data
            </Button>
          )}
        </div>
      )}
    </>
  );
}
