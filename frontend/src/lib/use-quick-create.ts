'use client';

import { useCallback, useEffect, useState } from 'react';

import {
  isQuickCreateRequest,
  QUICK_CREATE_QUERY_PARAM,
  type QuickCreateResource,
} from '@/lib/routes';

/** Opens an existing create dialog when its canonical quick-create URL loads. */
export function useQuickCreate(resource: QuickCreateResource) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (isQuickCreateRequest(params, resource)) {
      setOpen(true);
    }
  }, [resource]);

  const onOpenChange = useCallback(
    (nextOpen: boolean) => {
      setOpen(nextOpen);

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
