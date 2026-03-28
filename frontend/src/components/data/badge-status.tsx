import React from 'react';

import { cn, getStatusColor } from '@/lib/utils';

interface BadgeStatusProps {
  status: string;
  className?: string;
}

function formatStatus(status: string): string {
  return status
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

export function BadgeStatus({ status, className }: BadgeStatusProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold',
        getStatusColor(status),
        className
      )}
    >
      {formatStatus(status)}
    </span>
  );
}
