import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import { CSRF_ERROR_HEADER, CSRF_TOKEN_HEADER } from './auth-constants';
import { ORGANIZATION_PROFILE_ROUTE, PROFILE_KEYS, PROFILE_UPDATE_KEYS } from './organization-profile';
import type { OrganizationProfile, OrganizationProfileEnvelope, OrganizationProfileMeta, OrganizationProfileUpdateInput } from '@/types/organization-profile';
import api from './api';
import { fileURLToPath } from 'node:url';
import { organizationProfileFixture } from '@/test/organization-profile-fixture';
import { readFileSync } from 'node:fs';
import { resetCsrfToken } from './csrf-client';

describe('canonical organisation profile API/Go contract', () => {
  beforeEach(resetCsrfToken); afterEach(() => { resetCsrfToken(); vi.unstubAllGlobals(); });
  it('uses cancellable typed GET with the canonical data/meta projection envelope', async () => {
    const fixture = organizationProfileFixture(); const fetcher = vi.fn().mockResolvedValue(Response.json(fixture)); vi.stubGlobal('fetch', fetcher);
    const controller = new AbortController();
    expectTypeOf(api.settings.getOrg).returns.toEqualTypeOf<Promise<OrganizationProfileEnvelope>>();
    await expect(api.settings.getOrg(controller.signal)).resolves.toEqual(fixture);
    expect(fetcher.mock.calls[0]).toMatchObject([`/api/bff${ORGANIZATION_PROFILE_ROUTE}`, { method: 'GET', signal: controller.signal, credentials: 'same-origin' }]);
  });
  it('uses typed presence-only PUT with version/reason, CSRF, no bearer and no redirect replay', async () => {
    const fetcher = vi.fn().mockImplementation((url: string) => Promise.resolve(url === '/api/auth/csrf' ? Response.json({ csrf_token: 'profile-proof' }) : Response.json(organizationProfileFixture({ version: 5 }))));
    vi.stubGlobal('fetch', fetcher); const controller = new AbortController();
    expectTypeOf(api.settings.updateOrg).parameter(0).toEqualTypeOf<OrganizationProfileUpdateInput>();
    await api.settings.updateOrg({ expected_version: 4, reason: 'Reviewed rename', name: 'Changed name' }, controller.signal);
    const [url, init] = fetcher.mock.calls[1]; expect(url).toBe(`/api/bff${ORGANIZATION_PROFILE_ROUTE}`);
    expect(init).toMatchObject({ method: 'PUT', signal: controller.signal, redirect: 'error', credentials: 'same-origin' });
    expect(JSON.parse(init.body)).toEqual({ expected_version: 4, reason: 'Reviewed rename', name: 'Changed name' });
    expect(new Headers(init.headers).get(CSRF_TOKEN_HEADER)).toBe('profile-proof'); expect(new Headers(init.headers).has('authorization')).toBe(false);
  });
  it.each([409, 503, 403])('does not silently resubmit reviewed PUT after %s', async (status) => {
    const fetcher = vi.fn().mockImplementation((url: string) => Promise.resolve(url === '/api/auth/csrf' ? Response.json({ csrf_token: 'proof' }) : Response.json({ error_code: 'organization_profile_conflict' }, { status, headers: status === 403 ? { [CSRF_ERROR_HEADER]: '1' } : {} })));
    vi.stubGlobal('fetch', fetcher);
    await expect(api.settings.updateOrg({ expected_version: 4, reason: 'Reviewed rename', name: 'Changed' })).rejects.toMatchObject({ status });
    expect(fetcher.mock.calls.filter(([url]) => url !== '/api/auth/csrf')).toHaveLength(1);
  });
  it('matches exact current Go DTO aliases without editing shared OpenAPI artifacts', () => {
    const source = readFileSync(fileURLToPath(new URL('../../../internal/models/organization_profile.go', import.meta.url)), 'utf8');
    const fields = (type: string) => [...(source.match(new RegExp(`type ${type} struct \\{([\\s\\S]*?)\\n\\}`))?.[1] ?? '').matchAll(/json:"([a-z_]+)(?:,omitempty)?"/g)].map((match) => match[1]).sort();
    const profile: readonly (keyof OrganizationProfile)[] = PROFILE_KEYS;
    const input: readonly (keyof OrganizationProfileUpdateInput)[] = PROFILE_UPDATE_KEYS;
    const meta: readonly (keyof OrganizationProfileMeta)[] = ['schema_version', 'scope', 'editable'];
    expect(fields('OrganizationProfile')).toEqual([...profile].sort());
    expect(fields('OrganizationProfileUpdateInput')).toEqual([...input].sort());
    expect(fields('OrganizationProfileMeta')).toEqual([...meta].sort());
    expect(fields('OrganizationProfileResponse')).toEqual(['data', 'meta']);
  });
});
