import {
  ACCESS_ADMIN_ROUTES,
  groupPermissions,
  hasAccessAdministrationPermission,
  permissionSelection,
  removedPermissions,
  roleHasPermission,
} from '@/lib/access-admin';
import { describe, expect, it } from 'vitest';
import type { ManagedRole, PermissionGrant } from '@/types/access-admin';

const permissions: PermissionGrant[] = [
  { id: '2', resource: 'settings', action: 'configure', description: 'Configure settings' },
  { id: '1', resource: 'audits', action: 'read', description: 'Read audits' },
  { id: '3', resource: 'settings', action: 'read', description: 'Read settings' },
];

describe('access administration helpers', () => {
  it('defines the canonical role, impact, assignment, and event routes', () => {
    expect(ACCESS_ADMIN_ROUTES.role('role-1')).toBe('/access/roles/role-1');
    expect(ACCESS_ADMIN_ROUTES.impact('role-1')).toBe('/access/roles/role-1/impact-preview');
    expect(ACCESS_ADMIN_ROUTES.assignment('role-1', 'user-1')).toBe('/access/roles/role-1/assignments/user-1');
    expect(ACCESS_ADMIN_ROUTES.events('role-1')).toBe('/access/roles/role-1/events');
  });

  it('strips catalogue metadata, normalizes, de-duplicates, and sorts mutation permissions', () => {
    expect(permissionSelection([
      permissions[0],
      { resource: ' SETTINGS ', action: 'CONFIGURE' },
      permissions[1],
    ])).toEqual([
      { resource: 'audits', action: 'read' },
      { resource: 'settings', action: 'configure' },
    ]);
  });

  it('groups the catalogue and identifies removed administrative grants', () => {
    expect(groupPermissions(permissions).map((group) => group.resource)).toEqual(['audits', 'settings']);
    expect(removedPermissions(permissions, [permissions[1]])).toEqual([permissions[0], permissions[2]]);
    expect(roleHasPermission({ permissions } as ManagedRole, 'settings', 'configure')).toBe(true);
  });

  it('honors explicit settings grants before the legacy administrator fallback', () => {
    const administrator = { roles: [{ slug: 'org_admin' }] };
    expect(hasAccessAdministrationPermission(undefined, administrator as never, 'configure')).toBe(true);
    expect(hasAccessAdministrationPermission({ settings: ['read'] }, administrator as never, 'configure')).toBe(false);
    expect(hasAccessAdministrationPermission({ settings: ['read', 'configure'] }, null, 'configure')).toBe(true);
  });
});
