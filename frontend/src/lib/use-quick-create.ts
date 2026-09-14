'use client';

import { useCallback, useState, useSyncExternalStore } from 'react';

import {
  isQuickCreateRequest,
  QUICK_CREATE_QUERY_PARAM,
  type QuickCreateResource,
} from '@/lib/routes';

/** Opens an existing create dialog when its canonical quick-create URL loads. */
export function useQuickCreate(resource: QuickCreateResource) {
  const requestedByUrl = useSyncExternalStore(
    () => () => undefined,
    () => isQuickCreateRequest(new URLSearchParams(window.location.search), resource),
    () => false,
  );
  const [openOverride, setOpenOverride] = useState<boolean | null>(null);
  const open = openOverride ?? requestedByUrl;

  const onOpenChange = useCallback(
    (nextOpen: boolean) => {
      setOpenOverride(nextOpen);

      if (!nextOpen) {
        const url = new URL(window.location.href);
        if (isQuickCreateRequest(url.searchParams, resource)) {
          url.searchParams.delete(QUICK_CREATE_QUERY_PARAM);
          window.history.replaceState(
            window.history.state,
            '',
            `${url.pathname}${url.search}${url.hash}`
          );
        }
      }
    },
    [resource]
  );

  return [open, onOpenChange] as const;
}
