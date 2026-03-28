import { create } from "zustand";
import type { User } from "@/types";
import { getToken, setToken, setRefreshToken, clearToken } from "@/lib/auth";

interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  setAuth: (token: string, refreshToken: string, user: User) => void;
  setUser: (user: User) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: getToken(),
  isAuthenticated: !!getToken(),

  setAuth: (token, refreshToken, user) => {
    setToken(token);
    setRefreshToken(refreshToken);
    set({ token, user, isAuthenticated: true });
  },

  setUser: (user) => {
    set({ user });
  },

  logout: () => {
    clearToken();
    set({ token: null, user: null, isAuthenticated: false });
  },
}));
