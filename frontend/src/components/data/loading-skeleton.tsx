import React from 'react';

import { cn } from '@/lib/utils';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent, CardHeader } from '@/components/ui/card';

// ---- Table Skeleton ----
interface TableSkeletonProps {
  rows?: number;
  cols?: number;
  className?: string;
}

export function TableSkeleton({ rows = 5, cols = 4, className }: TableSkeletonProps) {
  return (
    <div className={cn('w-full', className)}>
      {/* Header row */}
      <div className="flex gap-4 border-b pb-3">
        {Array.from({ length: cols }).map((_, c) => (
          <Skeleton key={`header-${c}`} className="h-4 flex-1" />
        ))}
      </div>
      {/* Data rows */}
      {Array.from({ length: rows }).map((_, r) => (
        <div key={`row-${r}`} className="flex gap-4 border-b py-4">
          {Array.from({ length: cols }).map((_, c) => (
            <Skeleton
              key={`cell-${r}-${c}`}
              className={cn('h-4 flex-1', c === 0 && 'max-w-[200px]')}
            />
          ))}
        </div>
      ))}
    </div>
  );
}

// ---- Card Grid Skeleton ----
interface CardGridSkeletonProps {
  count?: number;
  className?: string;
}

export function CardGridSkeleton({ count = 6, className }: CardGridSkeletonProps) {
  return (
    <div className={cn('grid gap-4 sm:grid-cols-2 lg:grid-cols-3', className)}>
      {Array.from({ length: count }).map((_, i) => (
        <Card key={i}>
          <CardHeader>
            <Skeleton className="h-5 w-3/4" />
            <Skeleton className="h-4 w-1/2" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-4 w-full" />
            <Skeleton className="mt-2 h-4 w-2/3" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

// ---- Stat Card Skeleton ----
interface StatCardSkeletonProps {
  count?: number;
  className?: string;
}

export function StatCardSkeleton({ count = 4, className }: StatCardSkeletonProps) {
  return (
    <div className={cn('grid gap-4 sm:grid-cols-2 lg:grid-cols-4', className)}>
      {Array.from({ length: count }).map((_, i) => (
        <Card key={i}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <Skeleton className="h-10 w-10 rounded-lg" />
              <Skeleton className="h-4 w-16" />
            </div>
            <Skeleton className="mt-4 h-8 w-24" />
            <Skeleton className="mt-2 h-4 w-32" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

// ---- Detail Page Skeleton ----
interface DetailPageSkeletonProps {
  className?: string;
}

export function DetailPageSkeleton({ className }: DetailPageSkeletonProps) {
  return (
    <div className={cn('space-y-6', className)}>
      {/* Title area */}
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-4 w-40" />
        </div>
        <div className="flex gap-2">
          <Skeleton className="h-10 w-24" />
          <Skeleton className="h-10 w-24" />
        </div>
      </div>

      {/* Stats row */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <CardContent className="p-6">
              <Skeleton className="h-4 w-20" />
              <Skeleton className="mt-2 h-6 w-16" />
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Content area */}
      <Card>
        <CardHeader>
          <Skeleton className="h-6 w-48" />
        </CardHeader>
        <CardContent className="space-y-4">
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-1/2" />
        </CardContent>
      </Card>
    </div>
  );
}
