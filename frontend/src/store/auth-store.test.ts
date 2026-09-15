import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from './auth-store';
import type { User } from '@/types';

const mocks = vi.hoisted(() => ({ logout: vi.fn() }));
vi.mock('@/lib/api', () => ({ default: { auth: { logout: mocks.logout } } }));
const user: User = {
  id: 'bba53cd8-dabc-4c88-97bc-319aa1bc79ed', organization_id: '7a2423cd-bdeb-472f-a60a-1b6cfec87aa8',
  email: 'admin@example.test', first_name: 'Tenant', last_name: 'Administrator', status: 'active',
  is_super_admin: false, language: 'en', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
};

describe('synchronous logout intent', () => {
  beforeEach(() => { useAuthStore.setState({ user, isAuthenticated: true, isLoggingOut: false }); mocks.logout.mockReset(); });
  afterEach(() => useAuthStore.setState({ user: null, isAuthenticated: false, isLoggingOut: false }));

  it('clears identity immediately while preserving the server revocation attempt', async () => {
    let resolve: () => void = () => undefined;
    mocks.logout.mockReturnValue(new Promise<void>((done) => { resolve = done; }));
    const pending = useAuthStore.getState().logout();
    expect(useAuthStore.getState()).toMatchObject({ user: null, isAuthenticated: false, isLoggingOut: true });
    expect(mocks.logout).toHaveBeenCalledOnce();
    useAuthStore.getState().setAuth(user);
    useAuthStore.getState().setUser(user);
    expect(useAuthStore.getState().user).toBeNull();
    resolve(); await pending;
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
  });

  it('never restores identity when the logout transport fails', async () => {
    mocks.logout.mockRejectedValue(new Error('Transport unavailable'));
    await expect(useAuthStore.getState().logout()).rejects.toThrow('Transport unavailable');
    expect(useAuthStore.getState()).toMatchObject({ user: null, isAuthenticated: false, isLoggingOut: true });
    useAuthStore.getState().setAuth(user);
    expect(useAuthStore.getState().user).toBeNull();
  });
});
