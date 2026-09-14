import { beforeEach, describe, expect, it, vi } from 'vitest';

import api from '@/lib/api';
import { purgeLegacyBrowserCredentials } from '@/lib/auth';
import { resetCsrfToken } from '@/lib/csrf-client';
import { useAuthStore } from '@/store/auth-store';

describe('browser session credential isolation', () => {
  beforeEach(() => {
    resetCsrfToken();
    useAuthStore.setState({ user: null, isAuthenticated: false });
  });

  it('never writes login credentials to localStorage or document.cookie', async () => {
    const storageSet = vi.spyOn(window.localStorage, 'setItem');
    const cookieSet = vi.spyOn(Document.prototype, 'cookie', 'set');
    const user = {
      id: 'user-1',
      organization_id: 'org-1',
      email: 'user@example.test',
      first_name: 'Ada',
      last_name: 'Lovelace',
      status: 'active',
      is_super_admin: false,
      language: 'en',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    };
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(Response.json({ csrf_token: 'csrf-token' }))
      .mockResolvedValueOnce(
        Response.json({
          expires_at: '2026-12-31T00:00:00Z',
          user,
        }),
      );

    const response = await api.auth.login({
      email: 'user@example.test',
      password: 'a-secure-password',
    });
    useAuthStore.getState().setAuth(response.user);

    expect(storageSet).not.toHaveBeenCalled();
    expect(cookieSet).not.toHaveBeenCalled();
    expect(response).not.toHaveProperty('access_token');
    expect(response).not.toHaveProperty('refresh_token');
    expect(useAuthStore.getState()).not.toHaveProperty('token');
    expect(fetchMock.mock.calls[1][0]).toBe('/api/auth/login');
    const loginRequest = fetchMock.mock.calls[1][1];
    expect(loginRequest?.credentials).toBe('same-origin');
    expect(new Headers(loginRequest?.headers).has('authorization')).toBe(false);
  });

  it('purges bearer credentials left by a pre-BFF release', () => {
    window.localStorage.setItem('cf_access_token', 'legacy-access');
    window.localStorage.setItem('cf_refresh_token', 'legacy-refresh');

    purgeLegacyBrowserCredentials();

    expect(window.localStorage.getItem('cf_access_token')).toBeNull();
    expect(window.localStorage.getItem('cf_refresh_token')).toBeNull();
  });
});
