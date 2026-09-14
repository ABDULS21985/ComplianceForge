'use client';

import { create } from 'zustand';
import { NAVIGATION_GROUPS } from '@/lib/navigation';
import { persist } from 'zustand/middleware';

const DEFAULT_EXPANDED_GROUPS = Object.fromEntries(
  NAVIGATION_GROUPS.map((group) => [group.id, Boolean(group.defaultOpen)])
);

interface NavigationState {
  collapsed: boolean;
  expandedGroups: Record<string, boolean>;
  favoriteIds: string[];
  recentIds: string[];
  toggleCollapsed: () => void;
  setCollapsed: (collapsed: boolean) => void;
  toggleGroup: (groupId: string) => void;
  setGroupExpanded: (groupId: string, expanded: boolean) => void;
  toggleFavorite: (itemId: string) => void;
  recordRecent: (itemId: string) => void;
}

export const useNavigationStore = create<NavigationState>()(
  persist(
    (set) => ({
      collapsed: false,
      expandedGroups: DEFAULT_EXPANDED_GROUPS,
      favoriteIds: [],
      recentIds: [],
      toggleCollapsed: () =>
        set((state) => ({ collapsed: !state.collapsed })),
      setCollapsed: (collapsed) => set({ collapsed }),
      toggleGroup: (groupId) =>
        set((state) => ({
          expandedGroups: {
            ...state.expandedGroups,
            [groupId]: !state.expandedGroups[groupId],
          },
        })),
      setGroupExpanded: (groupId, expanded) =>
        set((state) => ({
          expandedGroups: {
            ...state.expandedGroups,
            [groupId]: expanded,
          },
        })),
      toggleFavorite: (itemId) =>
        set((state) => ({
          favoriteIds: state.favoriteIds.includes(itemId)
            ? state.favoriteIds.filter((id) => id !== itemId)
            : [...state.favoriteIds, itemId],
        })),
      recordRecent: (itemId) =>
        set((state) => ({
          recentIds: [
            itemId,
            ...state.recentIds.filter((id) => id !== itemId),
          ].slice(0, 5),
        })),
    }),
    {
      name: 'cf-navigation',
      version: 1,
    }
  )
);

export const useSidebarStore = useNavigationStore;
