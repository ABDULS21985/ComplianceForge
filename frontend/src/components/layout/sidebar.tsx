'use client';

import {
  ChevronDown,
  History,
  Menu,
  PanelLeft,
  PanelLeftClose,
  Shield,
  Star,
} from 'lucide-react';
import {
  getNavigationItemForPath,
  getRoleSlugs,
  getVisibleNavigationGroups,
  type NavigationContext,
  type NavigationItem,
  type PermissionMap,
} from '@/lib/navigation';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import Link from 'next/link';
import { NAVIGATION_ICONS } from '@/components/layout/navigation-icons';
import { useNavigationStore } from '@/store/navigation-store';
import { usePathname } from 'next/navigation';
import type { User } from '@/types';

interface NavigationLinkProps {
  active: boolean;
  collapsed: boolean;
  favorite: boolean;
  item: NavigationItem;
  onNavigate?: () => void;
  onRecordRecent: (itemId: string) => void;
  onToggleFavorite: (itemId: string) => void;
}

function NavigationLink({
  active,
  collapsed,
  favorite,
  item,
  onNavigate,
  onRecordRecent,
  onToggleFavorite,
}: NavigationLinkProps) {
  const Icon = NAVIGATION_ICONS[item.icon];
  const activate = () => {
    onRecordRecent(item.id);
    onNavigate?.();
  };

  const link = (
    <Link
      href={item.href}
      aria-current={active ? 'page' : undefined}
      onClick={activate}
      className={cn(
        'flex min-w-0 flex-1 items-center gap-3 rounded-md px-3 py-2 text-sm font-medium outline-none transition-colors',
        'hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1',
        active
          ? 'bg-accent text-accent-foreground'
          : 'text-muted-foreground',
        collapsed && 'justify-center px-2'
      )}
    >
      <Icon aria-hidden="true" className="h-[18px] w-[18px] shrink-0" />
      {!collapsed && <span className="truncate">{item.label}</span>}
    </Link>
  );

  if (collapsed) {
    return (
      <Tooltip delayDuration={0}>
        <TooltipTrigger asChild>{link}</TooltipTrigger>
        <TooltipContent side="right">{item.label}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <div className="group flex items-center gap-1">
      {link}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={`${favorite ? 'Remove' : 'Add'} ${item.label} ${
          favorite ? 'from' : 'to'
        } favorites`}
        aria-pressed={favorite}
        onClick={() => onToggleFavorite(item.id)}
        className={cn(
          'h-8 w-8 shrink-0 focus-visible:opacity-100',
          favorite
            ? 'text-amber-500 opacity-100'
            : 'text-muted-foreground opacity-100 md:opacity-0 md:group-hover:opacity-100'
        )}
      >
        <Star aria-hidden="true" className={cn('h-4 w-4', favorite && 'fill-current')} />
      </Button>
    </div>
  );
}

interface SidebarContentProps {
  collapsed: boolean;
  context: NavigationContext;
  onNavigate?: () => void;
  onToggle?: () => void;
  pathname: string;
}

export function SidebarContent({
  collapsed,
  context,
  onNavigate,
  onToggle,
  pathname,
}: SidebarContentProps) {
  const {
    expandedGroups,
    favoriteIds,
    recentIds,
    recordRecent,
    setGroupExpanded,
    toggleFavorite,
    toggleGroup,
  } = useNavigationStore();
  const groups = useMemo(() => getVisibleNavigationGroups(context), [context]);
  const visibleItems = useMemo(
    () => groups.flatMap((group) => group.items),
    [groups]
  );
  const activeItem = getNavigationItemForPath(pathname, visibleItems);
  const activeGroup = groups.find((group) =>
    group.items.some((item) => item.id === activeItem?.id)
  );
  const activeGroupId = activeGroup?.id;

  useEffect(() => {
    if (activeGroupId) {
      setGroupExpanded(activeGroupId, true);
    }
  }, [activeGroupId, setGroupExpanded]);

  const favorites = favoriteIds
    .map((id) => visibleItems.find((item) => item.id === id))
    .filter((item): item is NavigationItem => Boolean(item));
  const recent = recentIds
    .map((id) => visibleItems.find((item) => item.id === id))
    .filter((item): item is NavigationItem => Boolean(item))
    .slice(0, 3);

  const renderLink = (item: NavigationItem, showActive = true) => (
    <NavigationLink
      key={item.id}
      active={showActive && activeItem?.id === item.id}
      collapsed={collapsed}
      favorite={favoriteIds.includes(item.id)}
      item={item}
      onNavigate={onNavigate}
      onRecordRecent={recordRecent}
      onToggleFavorite={toggleFavorite}
    />
  );

  return (
    <div className="flex h-full flex-col">
      <div
        className={cn(
          'flex h-16 shrink-0 items-center border-b px-4',
          collapsed && 'justify-center px-2'
        )}
      >
        <Link
          href="/dashboard"
          onClick={onNavigate}
          aria-label="ComplianceForge dashboard"
          className="flex min-w-0 items-center gap-2 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Shield aria-hidden="true" className="h-7 w-7 shrink-0 text-primary" />
          {!collapsed && (
            <span className="truncate text-lg font-bold tracking-tight">
              ComplianceForge
            </span>
          )}
        </Link>
      </div>

      <nav
        aria-label="Primary navigation"
        className="flex-1 overflow-y-auto overscroll-contain px-2 py-3"
      >
        {!collapsed && favorites.length > 0 && (
          <section aria-labelledby="navigation-favorites" className="mb-3">
            <div className="flex items-center px-3 pb-1.5">
              <Star aria-hidden="true" className="mr-2 h-3.5 w-3.5 text-amber-500" />
              <h2
                id="navigation-favorites"
                className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground"
              >
                Favorites
              </h2>
            </div>
            <div className="space-y-0.5">
              {favorites.map((item) => renderLink(item, false))}
            </div>
          </section>
        )}

        {!collapsed && recent.length > 0 && (
          <section aria-labelledby="navigation-recent" className="mb-3">
            <div className="flex items-center px-3 pb-1.5">
              <History aria-hidden="true" className="mr-2 h-3.5 w-3.5" />
              <h2
                id="navigation-recent"
                className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground"
              >
                Recent
              </h2>
            </div>
            <div className="space-y-0.5">
              {recent.map((item) => renderLink(item, false))}
            </div>
          </section>
        )}

        <div className="space-y-2">
          {groups.map((group) => {
            const expanded = collapsed || Boolean(expandedGroups[group.id]);
            const regionId = `navigation-group-${group.id}`;

            return (
              <section key={group.id} aria-labelledby={`${regionId}-label`}>
                {collapsed ? (
                  <div aria-hidden="true" className="mx-2 my-2 border-t" />
                ) : (
                  <button
                    id={`${regionId}-label`}
                    type="button"
                    aria-controls={regionId}
                    aria-expanded={expanded}
                    onClick={() => toggleGroup(group.id)}
                    className="flex w-full items-center rounded-md px-3 py-1.5 text-left text-[11px] font-semibold uppercase tracking-wider text-muted-foreground outline-none hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <span className="flex-1">{group.label}</span>
                    <ChevronDown
                      aria-hidden="true"
                      className={cn(
                        'h-3.5 w-3.5 transition-transform',
                        !expanded && '-rotate-90'
                      )}
                    />
                  </button>
                )}
                <div
                  id={regionId}
                  hidden={!expanded}
                  className="mt-0.5 space-y-0.5"
                >
                  {group.items.map((item) => renderLink(item))}
                </div>
              </section>
            );
          })}
        </div>
      </nav>

      {onToggle && (
        <div className={cn('shrink-0 border-t p-2', collapsed && 'flex justify-center')}>
          <Button
            type="button"
            variant="ghost"
            size={collapsed ? 'icon' : 'sm'}
            onClick={onToggle}
            aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'}
            className={cn(!collapsed && 'w-full justify-start gap-2')}
          >
            {collapsed ? (
              <PanelLeft aria-hidden="true" className="h-5 w-5" />
            ) : (
              <>
                <PanelLeftClose aria-hidden="true" className="h-5 w-5" />
                <span>Collapse navigation</span>
              </>
            )}
          </Button>
        </div>
      )}
    </div>
  );
}

interface SidebarProps {
  permissions?: PermissionMap;
  user: User | null;
}

export function Sidebar({ permissions, user }: SidebarProps) {
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const { collapsed, toggleCollapsed } = useNavigationStore();
  const context = useMemo<NavigationContext>(
    () => ({
      isSuperAdmin: user?.is_super_admin,
      permissions,
      roleSlugs: getRoleSlugs(user?.roles),
    }),
    [permissions, user?.is_super_admin, user?.roles]
  );

  return (
    <TooltipProvider>
      <aside
        aria-label="Application sidebar"
        className={cn(
          'hidden h-screen shrink-0 border-r bg-background transition-[width] duration-200 md:block',
          collapsed ? 'w-[68px]' : 'w-72'
        )}
      >
        <SidebarContent
          collapsed={collapsed}
          context={context}
          onToggle={toggleCollapsed}
          pathname={pathname}
        />
      </aside>

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Open primary navigation"
            className="fixed left-3 top-3 z-40 md:hidden"
          >
            <Menu aria-hidden="true" className="h-5 w-5" />
          </Button>
        </SheetTrigger>
        <SheetContent side="left" className="w-[min(90vw,20rem)] p-0">
          <SheetTitle className="sr-only">Primary navigation</SheetTitle>
          <SheetDescription className="sr-only">
            Navigate between ComplianceForge workspaces and domains.
          </SheetDescription>
          <SidebarContent
            collapsed={false}
            context={context}
            onNavigate={() => setMobileOpen(false)}
            pathname={pathname}
          />
        </SheetContent>
      </Sheet>
    </TooltipProvider>
  );
}
