'use client';

import { AlertTriangle, LockKeyhole, RefreshCw } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import type { ReactNode } from 'react';
import { Skeleton } from '@/components/ui/skeleton';

export function GovernanceErrorState({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <Card>
      <CardContent className="space-y-3 py-12 text-center">
        <RefreshCw aria-hidden="true" className="mx-auto h-9 w-9 text-muted-foreground" />
        <p role="alert">{message}</p>
        <Button type="button" variant="outline" onClick={onRetry}>
          Try again
        </Button>
      </CardContent>
    </Card>
  );
}

export function GovernancePageSkeleton() {
  return (
    <div role="status" aria-label="Loading data lifecycle governance" className="space-y-4">
      <Skeleton className="h-24" />
      <Skeleton className="h-12" />
      <div className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-72" />
        <Skeleton className="h-72" />
      </div>
    </div>
  );
}

export function GovernancePanelSkeleton({
  label = 'Loading governance records',
}: {
  label?: string;
}) {
  return (
    <div role="status" aria-label={label} className="space-y-3">
      <Skeleton className="h-12" />
      {Array.from({ length: 4 }).map((_, index) => (
        <Skeleton key={index} className="h-20" />
      ))}
    </div>
  );
}

export function ReadOnlyNotice() {
  return (
    <Card>
      <CardContent className="flex gap-3 py-4 text-sm text-muted-foreground">
        <LockKeyhole aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
        You have read-only settings access. Governance mutation controls are hidden.
      </CardContent>
    </Card>
  );
}

export function GovernanceWarning({ children }: { children: ReactNode }) {
  return (
    <div role="status" className="flex gap-2 rounded-md bg-amber-500/10 p-3 text-sm">
      <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" />
      <span>{children}</span>
    </div>
  );
}
