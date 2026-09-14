import { create } from 'zustand';
import type { User } from '@/types';
import api from '@/lib/api';
import { purgeLegacyBrowserCredentials } from '@/lib/auth';
import { resetCsrfToken } from '@/lib/csrf-client';
import { ROUTES } from '@/lib/routes';

purgeLegacyBrowserCredentials();

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  setAuth: (user: User) => void;
  setUser: (user: User) => void;
  clearAuth: () => void;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,

  setAuth: (user) => {
    set({ user, isAuthenticated: true });
  },

  setUser: (user) => {
    set({ user });
  },

  clearAuth: () => set({ user: null, isAuthenticated: false }),

  logout: async () => {
    try {
      await api.auth.logout();
    } finally {
      resetCsrfToken();
      set({ user: null, isAuthenticated: false });
      if (typeof window !== 'undefined') {
        window.location.assign(ROUTES.auth.login);
      }
    }
  },
}));
