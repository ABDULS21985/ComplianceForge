import api from '@/lib/api';
import { create } from 'zustand';
import { purgeLegacyBrowserCredentials } from '@/lib/auth';
import { resetCsrfToken } from '@/lib/csrf-client';
import { ROUTES } from '@/lib/routes';
import type { User } from '@/types';

purgeLegacyBrowserCredentials();

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoggingOut: boolean;
  setAuth: (user: User) => void;
  setUser: (user: User) => void;
  clearAuth: () => void;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  isLoggingOut: false,

  setAuth: (user) => {
    set((state) => state.isLoggingOut ? state : { user, isAuthenticated: true });
  },

  setUser: (user) => {
    set((state) => state.isLoggingOut ? state : { user });
  },

  clearAuth: () => set({ user: null, isAuthenticated: false }),

  logout: async () => {
    // Invalidate sensitive local workflows at intent, not after an unreliable transport.
    set({ user: null, isAuthenticated: false, isLoggingOut: true });
    try {
      await api.auth.logout();
    } finally {
      resetCsrfToken();
      set({ user: null, isAuthenticated: false, isLoggingOut: true });
      if (typeof window !== 'undefined') {
        window.location.assign(ROUTES.auth.login);
      }
    }
  },
}));
