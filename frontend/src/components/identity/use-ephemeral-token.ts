'use client';

import * as React from 'react';
import { useSearchParams } from 'next/navigation';

/**
 * Captures an emailed one-time credential in component memory, then removes it
 * from the current history entry. It is never copied to storage or cookies.
 */
export function useEphemeralQueryToken(parameter = 'token'): string {
  const searchParams = useSearchParams();
  const [token] = React.useState(searchParams.get(parameter)?.trim() ?? '');

  React.useEffect(() => {
    const url = new URL(window.location.href);
    if (!url.searchParams.has(parameter)) return;
    url.searchParams.delete(parameter);
    window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`);
  }, [parameter]);

  return token;
}
