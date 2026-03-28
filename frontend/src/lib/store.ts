// ComplianceForge Zustand Stores
// Global client-side state management

import { create } from "zustand";
import { persist } from "zustand/middleware";

// ---------------------------------------------------------------------------
// Auth Store
// ---------------------------------------------------------------------------

interface User {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  roles: string[];
  org_id?: string;
  [key: string]: unknown;
}

interface AuthState {
  user: User | null;
  setUser: (user: User) => void;
  clearUser: () => void;
  isAuthenticated: () => boolean;
}

export const useAuthStore = create<AuthState>()((set, get) => ({
  user: null,
  setUser: (user) => set({ user }),
  clearUser: () => set({ user: null }),
  isAuthenticated: () => get().user !== null,
}));

// ---------------------------------------------------------------------------
// Sidebar Store (persisted to localStorage)
// ---------------------------------------------------------------------------

interface SidebarState {
  isCollapsed: boolean;
  toggle: () => void;
  setCollapsed: (collapsed: boolean) => void;
}

export const useSidebarStore = create<SidebarState>()(
  persist(
    (set, get) => ({
      isCollapsed: false,
      toggle: () => set({ isCollapsed: !get().isCollapsed }),
      setCollapsed: (collapsed) => set({ isCollapsed: collapsed }),
    }),
    {
      name: "cf-sidebar",
    }
  )
);

// ---------------------------------------------------------------------------
// Notification Store
// ---------------------------------------------------------------------------

export interface Notification {
  id: string;
  title: string;
  message: string;
  type: "info" | "success" | "warning" | "error";
  timestamp: number;
  read: boolean;
}

interface NotificationState {
  notifications: Notification[];
  add: (notification: Omit<Notification, "id" | "timestamp" | "read">) => void;
  remove: (id: string) => void;
  markAsRead: (id: string) => void;
  clear: () => void;
  unreadCount: () => number;
}

export const useNotificationStore = create<NotificationState>()((set, get) => ({
  notifications: [],

  add: (notification) =>
    set((state) => ({
      notifications: [
        {
          ...notification,
          id: crypto.randomUUID(),
          timestamp: Date.now(),
          read: false,
        },
        ...state.notifications,
      ],
    })),

  remove: (id) =>
    set((state) => ({
      notifications: state.notifications.filter((n) => n.id !== id),
    })),

  markAsRead: (id) =>
    set((state) => ({
      notifications: state.notifications.map((n) =>
        n.id === id ? { ...n, read: true } : n
      ),
    })),

  clear: () => set({ notifications: [] }),

  unreadCount: () => get().notifications.filter((n) => !n.read).length,
}));
