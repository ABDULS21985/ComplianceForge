'use client';

import React from 'react';
import { Bell, Search } from 'lucide-react';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Breadcrumbs } from '@/components/layout/breadcrumbs';
import { UserMenu } from '@/components/layout/user-menu';

interface TopbarProps {
  user: {
    first_name: string;
    last_name: string;
    email: string;
    avatar_url?: string;
  } | null;
  notificationCount?: number;
  onLogout: () => void;
  /** Override labels for dynamic breadcrumb segments */
  dynamicLabels?: Record<string, string>;
  className?: string;
}

export function Topbar({
  user,
  notificationCount = 0,
  onLogout,
  dynamicLabels,
  className,
}: TopbarProps) {
  return (
    <header
      className={cn(
        'sticky top-0 z-30 flex h-16 items-center gap-4 border-b bg-background px-4 md:px-6',
        className
      )}
    >
      {/* Breadcrumbs -- hidden on mobile for space */}
      <div className="hidden md:flex flex-1">
        <Breadcrumbs dynamicLabels={dynamicLabels} />
      </div>

      {/* Spacer on mobile */}
      <div className="flex-1 md:hidden" />

      <div className="flex items-center gap-2">
        {/* Global search trigger */}
        <Button
          variant="outline"
          size="sm"
          className="hidden md:flex items-center gap-2 text-muted-foreground"
          onClick={() => {
            // Placeholder: open command palette
          }}
        >
          <Search className="h-4 w-4" />
          <span className="text-sm">Search...</span>
          <kbd className="pointer-events-none ml-2 hidden h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium opacity-100 sm:flex">
            <span className="text-xs">Ctrl</span>K
          </kbd>
        </Button>

        {/* Mobile search icon */}
        <Button variant="ghost" size="icon" className="md:hidden">
          <Search className="h-5 w-5" />
          <span className="sr-only">Search</span>
        </Button>

        {/* Notifications */}
        <Button variant="ghost" size="icon" className="relative">
          <Bell className="h-5 w-5" />
          {notificationCount > 0 && (
            <Badge
              variant="destructive"
              className="absolute -right-1 -top-1 h-5 min-w-[20px] px-1 text-[10px]"
            >
              {notificationCount > 99 ? '99+' : notificationCount}
            </Badge>
          )}
          <span className="sr-only">Notifications</span>
        </Button>

        {/* User menu */}
        <UserMenu user={user} onLogout={onLogout} />
      </div>
    </header>
  );
}
