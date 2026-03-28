'use client';

import React from 'react';
import { TrendingUp, TrendingDown, Minus, type LucideIcon } from 'lucide-react';

import { cn } from '@/lib/utils';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

interface StatCardProps {
  title: string;
  value: string | number;
  subtitle?: string;
  icon?: LucideIcon;
  trend?: 'up' | 'down' | 'neutral';
  color?: 'default' | 'green' | 'yellow' | 'red' | 'blue';
  loading?: boolean;
}

const COLOR_MAP: Record<string, string> = {
  default: 'text-muted-foreground',
  green: 'text-green-600 dark:text-green-400',
  yellow: 'text-yellow-600 dark:text-yellow-400',
  red: 'text-red-600 dark:text-red-400',
  blue: 'text-blue-600 dark:text-blue-400',
};

const ICON_BG_MAP: Record<string, string> = {
  default: 'bg-muted',
  green: 'bg-green-100 dark:bg-green-900/30',
  yellow: 'bg-yellow-100 dark:bg-yellow-900/30',
  red: 'bg-red-100 dark:bg-red-900/30',
  blue: 'bg-blue-100 dark:bg-blue-900/30',
};

const TREND_MAP: Record<string, { icon: LucideIcon; className: string }> = {
  up: { icon: TrendingUp, className: 'text-green-600 dark:text-green-400' },
  down: { icon: TrendingDown, className: 'text-red-600 dark:text-red-400' },
  neutral: { icon: Minus, className: 'text-muted-foreground' },
};

export function StatCard({
  title,
  value,
  subtitle,
  icon: Icon,
  trend,
  color = 'default',
  loading = false,
}: StatCardProps) {
  if (loading) {
    return (
      <Card>
        <CardContent className="p-6">
          <div className="flex items-center justify-between">
            <Skeleton className="h-10 w-10 rounded-lg" />
            <Skeleton className="h-4 w-16" />
          </div>
          <Skeleton className="mt-4 h-8 w-24" />
          <Skeleton className="mt-2 h-4 w-32" />
        </CardContent>
      </Card>
    );
  }

  const TrendIcon = trend ? TREND_MAP[trend] : undefined;

  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex items-center justify-between">
          {Icon && (
            <div
              className={cn(
                'flex h-10 w-10 items-center justify-center rounded-lg',
                ICON_BG_MAP[color]
              )}
            >
              <Icon className={cn('h-5 w-5', COLOR_MAP[color])} />
            </div>
          )}
          {TrendIcon && (
            <div className="flex items-center gap-1">
              <TrendIcon.icon className={cn('h-4 w-4', TrendIcon.className)} />
            </div>
          )}
        </div>
        <div className="mt-4">
          <p className="text-3xl font-bold tracking-tight">{value}</p>
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
        </div>
        {subtitle && (
          <p className={cn('mt-1 text-xs', trend ? TREND_MAP[trend].className : 'text-muted-foreground')}>
            {subtitle}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
