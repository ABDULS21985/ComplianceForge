'use client';

import {
  AlertTriangle,
  Inbox,
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  WifiOff,
} from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { Skeleton } from '@/components/ui/skeleton';
import { useId } from 'react';
import { useOnlineStatus } from '@/hooks/use-online-status';

export type ResourceStateKind =
  | 'empty'
  | 'error'
  | 'forbidden'
  | 'loading'
  | 'offline';

type LoadingLayout = 'cards' | 'detail' | 'inline' | 'table';

interface ResourceStateProps {
  action?: ReactNode;
  className?: string;
  description?: string;
  headingLevel?: 1 | 2 | 3;
  kind: ResourceStateKind;
  loadingLayout?: LoadingLayout;
  onRetry?: () => void;
  retryLabel?: string;
  retrying?: boolean;
  surface?: 'card' | 'plain';
  title?: string;
}

const STATE_DEFAULTS: Record<
  Exclude<ResourceStateKind, 'loading'>,
  { description: string; icon: LucideIcon; title: string }
> = {
  empty: {
    description: 'There is nothing to show yet.',
    icon: Inbox,
    title: 'No results',
  },
  error: {
    description:
      'The service did not return this information. Retry without losing your work.',
    icon: AlertTriangle,
    title: 'Information could not be loaded',
  },
  forbidden: {
    description:
      'Your current role does not grant access to this information or action.',
    icon: LockKeyhole,
    title: 'Access unavailable',
  },
  offline: {
    description:
      'Your device appears to be offline. Reconnect, then retry without losing your work.',
    icon: WifiOff,
    title: 'You are offline',
  },
};

function LoadingVisual({ layout }: { layout: LoadingLayout }) {
  if (layout === 'inline') {
    return (
      <div className="flex items-center gap-3">
        <LoaderCircle className="h-5 w-5 animate-spin motion-reduce:animate-none" />
        <Skeleton className="h-4 w-48" />
      </div>
    );
  }

  if (layout === 'cards') {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }, (_, index) => (
          <Skeleton key={index} className="h-36 w-full" />
        ))}
      </div>
    );
  }

  if (layout === 'detail') {
    return (
      <div className="space-y-5">
        <Skeleton className="h-24 w-full" />
        <div className="grid gap-4 sm:grid-cols-3">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-28 w-full" />
          ))}
        </div>
        <Skeleton className="h-72 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <Skeleton className="h-10 w-full" />
      {Array.from({ length: 5 }, (_, index) => (
        <Skeleton key={index} className="h-12 w-full" />
      ))}
    </div>
  );
}

export function ResourceState({
  action,
  className,
  description,
  headingLevel = 2,
  kind,
  loadingLayout = 'table',
  onRetry,
  retryLabel = 'Try again',
  retrying = false,
  surface = 'card',
  title,
}: ResourceStateProps) {
  const descriptionId = useId();
  const titleId = useId();

  if (kind === 'loading') {
    const loadingTitle = title ?? 'Loading information';

    return (
      <div
        aria-busy="true"
        aria-live="polite"
        aria-label={loadingTitle}
        className={cn('w-full p-6', className)}
        role="status"
      >
        <span className="sr-only">{loadingTitle}</span>
        <div aria-hidden="true">
          <LoadingVisual layout={loadingLayout} />
        </div>
      </div>
    );
  }

  const defaults = STATE_DEFAULTS[kind];
  const Icon = defaults.icon;
  const resolvedTitle = title ?? defaults.title;
  const resolvedDescription = description ?? defaults.description;
  const isUrgent = kind === 'error' || kind === 'offline';
  const Heading = headingLevel === 1 ? 'h1' : headingLevel === 3 ? 'h3' : 'h2';

  const content = (
      <div
        aria-describedby={descriptionId}
        aria-labelledby={titleId}
        className={cn(
          'flex flex-col items-center gap-3 px-6 py-10 text-center',
          surface === 'plain' && className,
        )}
        role={isUrgent ? 'alert' : 'status'}
      >
        <span className="flex h-12 w-12 items-center justify-center rounded-full border bg-muted">
          <Icon aria-hidden="true" className="h-6 w-6" />
        </span>
        <Heading id={titleId} className="text-lg font-semibold">
          {resolvedTitle}
        </Heading>
        <p id={descriptionId} className="max-w-xl text-sm text-muted-foreground">
          {resolvedDescription}
        </p>
        {(action || onRetry) && (
          <div className="mt-2 flex flex-wrap items-center justify-center gap-2">
            {onRetry && (
              <Button
                type="button"
                variant="outline"
                disabled={retrying}
                onClick={onRetry}
              >
                <RefreshCw
                  aria-hidden="true"
                  className={cn(
                    'mr-2 h-4 w-4',
                    retrying && 'animate-spin motion-reduce:animate-none',
                  )}
                />
                {retrying ? 'Retrying…' : retryLabel}
              </Button>
            )}
            {action}
          </div>
        )}
      </div>
  );

  if (surface === 'plain') return content;

  return (
    <Card className={cn('w-full', className)}>
      <CardContent className="p-0">{content}</CardContent>
    </Card>
  );
}

interface ResourceBoundaryProps {
  children: ReactNode;
  className?: string;
  emptyAction?: ReactNode;
  emptyDescription?: string;
  emptyTitle?: string;
  errorDescription?: string;
  errorTitle?: string;
  forbidden?: boolean;
  forbiddenDescription?: string;
  forbiddenTitle?: string;
  isEmpty?: boolean;
  isError?: boolean;
  isLoading?: boolean;
  loadingLayout?: LoadingLayout;
  loadingTitle?: string;
  onRetry?: () => void;
  retrying?: boolean;
  surface?: 'card' | 'plain';
}

/** Renders one mutually exclusive, accessible state before revealing resource content. */
export function ResourceBoundary({
  children,
  className,
  emptyAction,
  emptyDescription,
  emptyTitle,
  errorDescription,
  errorTitle,
  forbidden = false,
  forbiddenDescription,
  forbiddenTitle,
  isEmpty = false,
  isError = false,
  isLoading = false,
  loadingLayout,
  loadingTitle,
  onRetry,
  retrying,
  surface,
}: ResourceBoundaryProps) {
  const online = useOnlineStatus();

  if (forbidden) {
    return (
      <ResourceState
        className={className}
        description={forbiddenDescription}
        kind="forbidden"
        surface={surface}
        title={forbiddenTitle}
      />
    );
  }

  if (isLoading) {
    return (
      <ResourceState
        className={className}
        kind="loading"
        loadingLayout={loadingLayout}
        title={loadingTitle}
      />
    );
  }

  if (isError) {
    return (
      <ResourceState
        className={className}
        description={online ? errorDescription : undefined}
        kind={online ? 'error' : 'offline'}
        onRetry={onRetry}
        retrying={retrying}
        surface={surface}
        title={online ? errorTitle : undefined}
      />
    );
  }

  if (isEmpty) {
    return (
      <ResourceState
        action={emptyAction}
        className={className}
        description={emptyDescription}
        kind="empty"
        surface={surface}
        title={emptyTitle}
      />
    );
  }

  return children;
}

interface StaleDataNoticeProps {
  className?: string;
  isRefreshing?: boolean;
  lastUpdatedAt?: number;
  onRefresh?: () => void;
  title?: string;
}

export function StaleDataNotice({
  className,
  isRefreshing = false,
  lastUpdatedAt,
  onRefresh,
  title = 'Showing saved data while the service reconnects',
}: StaleDataNoticeProps) {
  const date = lastUpdatedAt ? new Date(lastUpdatedAt) : null;
  const hasValidDate = Boolean(date && !Number.isNaN(date.valueOf()));

  return (
    <div
      aria-atomic="true"
      aria-live="polite"
      className={cn(
        'flex flex-col gap-3 rounded-lg border border-amber-600/40 bg-amber-50 p-4 text-amber-950 dark:bg-amber-950/30 dark:text-amber-100 sm:flex-row sm:items-center sm:justify-between',
        className,
      )}
      role="status"
    >
      <div className="flex items-start gap-3">
        <AlertTriangle aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0" />
        <div>
          <p className="font-medium">{title}</p>
          <p className="text-sm">
            Confirm current values before making a decision.
            {hasValidDate && date ? (
              <>
                {' '}Last updated{' '}
                <time dateTime={date.toISOString()}>{date.toLocaleString()}</time>.
              </>
            ) : null}
          </p>
        </div>
      </div>
      {onRefresh && (
        <Button
          type="button"
          variant="outline"
          disabled={isRefreshing}
          onClick={onRefresh}
        >
          <RefreshCw
            aria-hidden="true"
            className={cn(
              'mr-2 h-4 w-4',
              isRefreshing && 'animate-spin motion-reduce:animate-none',
            )}
          />
          {isRefreshing ? 'Refreshing…' : 'Refresh now'}
        </Button>
      )}
    </div>
  );
}
