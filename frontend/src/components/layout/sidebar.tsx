'use client';

import React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  LayoutDashboard,
  Shield,
  AlertTriangle,
  FileText,
  ClipboardCheck,
  AlertOctagon,
  Building2,
  Server,
  BarChart3,
  Settings,
  PanelLeftClose,
  PanelLeft,
  Menu,
  type LucideIcon,
} from 'lucide-react';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

import { cn } from '@/lib/utils';
import { NAV_ITEMS } from '@/lib/constants';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet';
import { Badge } from '@/components/ui/badge';

// ---- Sidebar Store ----
interface SidebarState {
  collapsed: boolean;
  toggleCollapsed: () => void;
  setCollapsed: (v: boolean) => void;
}

export const useSidebarStore = create<SidebarState>()(
  persist(
    (set) => ({
      collapsed: false,
      toggleCollapsed: () => set((s) => ({ collapsed: !s.collapsed })),
      setCollapsed: (collapsed) => set({ collapsed }),
    }),
    { name: 'sidebar-collapsed' }
  )
);

// ---- Icon map ----
const ICON_MAP: Record<string, LucideIcon> = {
  LayoutDashboard,
  Shield,
  AlertTriangle,
  FileText,
  ClipboardCheck,
  AlertOctagon,
  Building2,
  Server,
  BarChart3,
  Settings,
};

// ---- Nav Item ----
interface NavItemProps {
  item: (typeof NAV_ITEMS)[number];
  collapsed: boolean;
  badgeCounts?: Record<string, number>;
}

function NavItem({ item, collapsed, badgeCounts }: NavItemProps) {
  const pathname = usePathname();
  const Icon = ICON_MAP[item.icon] || LayoutDashboard;
  const isActive =
    pathname === item.href || pathname.startsWith(`${item.href}/`);
  const badgeCount =
    'badgeKey' in item && item.badgeKey
      ? badgeCounts?.[item.badgeKey as string]
      : undefined;

  const link = (
    <Link
      href={item.href}
      className={cn(
        'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
        'hover:bg-accent hover:text-accent-foreground',
        isActive
          ? 'bg-accent text-accent-foreground'
          : 'text-muted-foreground',
        collapsed && 'justify-center px-2'
      )}
    >
      <Icon className="h-5 w-5 shrink-0" />
      {!collapsed && (
        <>
          <span className="flex-1">{item.label}</span>
          {badgeCount != null && badgeCount > 0 && (
            <Badge variant="destructive" className="ml-auto h-5 min-w-[20px] px-1.5 text-[10px]">
              {badgeCount}
            </Badge>
          )}
        </>
      )}
    </Link>
  );

  if (collapsed) {
    return (
      <Tooltip delayDuration={0}>
        <TooltipTrigger asChild>{link}</TooltipTrigger>
        <TooltipContent side="right" className="flex items-center gap-2">
          {item.label}
          {badgeCount != null && badgeCount > 0 && (
            <Badge variant="destructive" className="h-5 min-w-[20px] px-1.5 text-[10px]">
              {badgeCount}
            </Badge>
          )}
        </TooltipContent>
      </Tooltip>
    );
  }

  return link;
}

// ---- Sidebar Content ----
interface SidebarContentProps {
  collapsed: boolean;
  badgeCounts?: Record<string, number>;
  onToggle?: () => void;
}

function SidebarContent({ collapsed, badgeCounts, onToggle }: SidebarContentProps) {
  return (
    <div className="flex h-full flex-col">
      {/* Logo */}
      <div
        className={cn(
          'flex h-16 items-center border-b px-4',
          collapsed && 'justify-center px-2'
        )}
      >
        <Link href="/dashboard" className="flex items-center gap-2">
          <Shield className="h-7 w-7 text-primary" />
          {!collapsed && (
            <span className="text-lg font-bold tracking-tight">
              ComplianceForge
            </span>
          )}
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-4">
        {NAV_ITEMS.map((item) => (
          <NavItem
            key={item.href}
            item={item}
            collapsed={collapsed}
            badgeCounts={badgeCounts}
          />
        ))}
      </nav>

      {/* Collapse toggle */}
      {onToggle && (
        <div className={cn('border-t p-3', collapsed && 'flex justify-center')}>
          <Button
            variant="ghost"
            size={collapsed ? 'icon' : 'sm'}
            onClick={onToggle}
            className={cn(!collapsed && 'w-full justify-start gap-2')}
          >
            {collapsed ? (
              <PanelLeft className="h-5 w-5" />
            ) : (
              <>
                <PanelLeftClose className="h-5 w-5" />
                <span>Collapse</span>
              </>
            )}
          </Button>
        </div>
      )}
    </div>
  );
}

// ---- Main Sidebar ----
interface SidebarProps {
  badgeCounts?: Record<string, number>;
}

export function Sidebar({ badgeCounts }: SidebarProps) {
  const { collapsed, toggleCollapsed } = useSidebarStore();

  return (
    <TooltipProvider>
      {/* Desktop / Tablet sidebar */}
      <aside
        className={cn(
          'hidden border-r bg-background transition-all duration-300 md:block',
          collapsed ? 'w-[68px]' : 'w-64'
        )}
      >
        <SidebarContent
          collapsed={collapsed}
          badgeCounts={badgeCounts}
          onToggle={toggleCollapsed}
        />
      </aside>

      {/* Mobile drawer */}
      <Sheet>
        <SheetTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="md:hidden fixed left-4 top-3 z-40"
          >
            <Menu className="h-5 w-5" />
            <span className="sr-only">Toggle navigation</span>
          </Button>
        </SheetTrigger>
        <SheetContent side="left" className="w-64 p-0">
          <SidebarContent
            collapsed={false}
            badgeCounts={badgeCounts}
          />
        </SheetContent>
      </Sheet>
    </TooltipProvider>
  );
}
