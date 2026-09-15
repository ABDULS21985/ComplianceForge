'use client';

import { useEffect, useRef } from 'react';
import { cn } from '@/lib/utils';
import { useOnlineStatus } from '@/hooks/use-online-status';
import { usePathname } from 'next/navigation';
import { WifiOff } from 'lucide-react';

function routeLabel(pathname: string) {
  const segment = pathname.split('/').filter(Boolean).at(-1);
  if (!segment) return 'Page';

  let decoded = segment;
  try {
    decoded = decodeURIComponent(segment);
  } catch {
    // Malformed URL text must not break the accessibility shell.
  }
  return decoded
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

/** Moves keyboard focus into new route content and announces client navigation. */
export function RouteFocusManager() {
  const announcementRef = useRef<HTMLDivElement>(null);
  const pathname = usePathname();
  const previousPathname = useRef(pathname);

  useEffect(() => {
    const routeChanged = previousPathname.current !== pathname;
    previousPathname.current = pathname;

    const routeRoot = document.querySelector<HTMLElement>(
      '#main-content, main, [data-route-focus-root]',
    );
    const heading =
      document.querySelector<HTMLElement>('[data-route-heading]') ??
      routeRoot?.querySelector<HTMLElement>('h1');
    const focusTarget = heading ?? routeRoot;
    const visibleTitle = heading?.textContent?.trim() || routeLabel(pathname);
    document.title = `${visibleTitle} | ComplianceForge`;

    if (!routeChanged) return;

    if (focusTarget) {
      if (!focusTarget.hasAttribute('tabindex')) focusTarget.tabIndex = -1;
      focusTarget.focus({ preventScroll: true });
    }

    if (announcementRef.current) {
      announcementRef.current.textContent = `${visibleTitle} loaded`;
    }
  }, [pathname]);

  return (
    <div
      ref={announcementRef}
      aria-atomic="true"
      aria-live="polite"
      className="sr-only"
      data-testid="route-announcer"
    />
  );
}

/** A persistent, non-colour-only connectivity warning for authenticated workflows. */
export function OfflineBanner({ floating = false }: { floating?: boolean }) {
  const online = useOnlineStatus();

  if (online) return null;

  return (
    <div
      aria-atomic="true"
      aria-live="assertive"
      className={cn(
        'flex min-h-11 items-center justify-center gap-2 border-b border-amber-600/50 bg-amber-50 px-4 py-2 text-center text-sm font-medium text-amber-950 shadow-sm dark:bg-amber-950/95 dark:text-amber-100',
        floating && 'fixed inset-x-0 top-0 z-[100]',
      )}
      role="alert"
    >
      <WifiOff aria-hidden="true" className="h-4 w-4 shrink-0" />
      You are offline. Saved content remains visible, but changes may fail until you reconnect.
    </div>
  );
}
