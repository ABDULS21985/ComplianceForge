'use client';

import {
  getRoleSlugs,
  type NavigationContext,
  type PermissionMap,
} from '@/lib/navigation';
import { useEffect, useMemo, useState } from 'react';
import { Breadcrumbs } from '@/components/layout/breadcrumbs';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { CommandPalette } from '@/components/layout/command-palette';
import { NotificationMenu } from '@/components/layout/notification-menu';
import { Search } from 'lucide-react';
import type { User } from '@/types';
import { UserMenu } from '@/components/layout/user-menu';

interface TopbarProps {
  className?: string;
  /** Override labels for dynamic breadcrumb segments. */
  dynamicLabels?: Record<string, string>;
  enabledCapabilities?: readonly string[];
  onLogout: () => void;
  permissions?: PermissionMap;
  user: User | null;
}

export function Topbar({
  className,
  dynamicLabels,
  enabledCapabilities,
  onLogout,
  permissions,
  user,
}: TopbarProps) {
  const [commandOpen, setCommandOpen] = useState(false);
  const navigationContext = useMemo<NavigationContext>(
    () => ({
      enabledCapabilities,
      isSuperAdmin: user?.is_super_admin,
      permissions,
      roleSlugs: getRoleSlugs(user?.roles),
    }),
    [enabledCapabilities, permissions, user?.is_super_admin, user?.roles]
  );

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setCommandOpen((current) => !current);
      }
    };

    document.addEventListener('keydown', handleShortcut);
    return () => document.removeEventListener('keydown', handleShortcut);
  }, []);

  return (
    <>
      <header
        className={cn(
          'sticky top-0 z-30 flex h-16 shrink-0 items-center gap-3 border-b bg-background/95 pl-14 pr-3 backdrop-blur supports-[backdrop-filter]:bg-background/80 md:px-6',
          className
        )}
      >
        <div className="hidden min-w-0 flex-1 md:block">
          <Breadcrumbs dynamicLabels={dynamicLabels} />
        </div>
        <div className="min-w-0 flex-1 md:hidden">
          <Breadcrumbs compact dynamicLabels={dynamicLabels} />
        </div>

        <div className="flex shrink-0 items-center gap-1.5">
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-haspopup="dialog"
            aria-keyshortcuts="Control+K Meta+K"
            onClick={() => setCommandOpen(true)}
            className="hidden min-w-44 items-center justify-start gap-2 text-muted-foreground lg:flex"
          >
            <Search aria-hidden="true" className="h-4 w-4" />
            <span className="flex-1 text-left text-sm">Search</span>
            <kbd className="pointer-events-none rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
              Ctrl/⌘ K
            </kbd>
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Search and navigate"
            aria-haspopup="dialog"
            aria-keyshortcuts="Control+K Meta+K"
            onClick={() => setCommandOpen(true)}
            className="lg:hidden"
          >
            <Search aria-hidden="true" className="h-5 w-5" />
          </Button>

          <NotificationMenu />
          <UserMenu user={user} onLogout={onLogout} />
        </div>
      </header>

      <CommandPalette
        context={navigationContext}
        open={commandOpen}
        onOpenChange={setCommandOpen}
      />
    </>
  );
}
