import { Inbox, type LucideIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import React from 'react';

interface EmptyStateProps {
  action?: React.ReactNode;
  icon?: LucideIcon;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
}

export function EmptyState({
  action,
  icon: Icon = Inbox,
  title,
  description,
  actionLabel,
  onAction,
  className,
}: EmptyStateProps) {
  return (
    <div
      aria-live="polite"
      className={cn(
        'flex flex-col items-center justify-center py-12 text-center',
        className
      )}
      role="status"
    >
      <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted">
        <Icon aria-hidden="true" className="h-8 w-8 text-muted-foreground" />
      </div>
      <h3 className="mt-4 text-lg font-semibold">{title}</h3>
      {description && (
        <p className="mt-2 max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      )}
      {actionLabel && onAction && (
        <Button onClick={onAction} className="mt-6">
          {actionLabel}
        </Button>
      )}
      {action && <div className="mt-6">{action}</div>}
    </div>
  );
}
