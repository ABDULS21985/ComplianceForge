import { describe, expect, it } from 'vitest';

import {
  invitationAcceptanceSchema,
  loginIdentitySchema,
  newPasswordSchema,
  organizationIdentitySchema,
} from '@/lib/identity-validation';

const organizationId = '11111111-1111-4111-8111-111111111111';

describe('public identity workflow validation', () => {
  it('requires the tenant UUID and a valid email for neutral delivery requests', () => {
    expect(
      organizationIdentitySchema.safeParse({
        organization_id: organizationId,
        email: 'person@example.test',
      }).success,
    ).toBe(true);
    expect(
      organizationIdentitySchema.safeParse({
        organization_id: 'tenant-slug',
        email: 'not-an-email',
      }).success,
    ).toBe(false);
  });

  it('does not apply the new-password policy to an existing password sign-in', () => {
    expect(
      loginIdentitySchema.safeParse({
        organization_id: organizationId,
        email: 'person@example.test',
        password: 'legacy-8',
      }).success,
    ).toBe(true);
  });

  it('enforces 12–72 trimmed matching characters for new passwords', () => {
    expect(
      newPasswordSchema.safeParse({
        password: 'a secure password',
        confirm_password: 'a secure password',
      }).success,
    ).toBe(true);
    expect(
      newPasswordSchema.safeParse({
        password: ' a secure password',
        confirm_password: ' a secure password',
      }).success,
    ).toBe(false);
    expect(
      newPasswordSchema.safeParse({ password: 'a secure password', confirm_password: 'different' })
        .success,
    ).toBe(false);
  });

  it('normalizes optional invitation profile fields without weakening password checks', () => {
    const parsed = invitationAcceptanceSchema.parse({
      first_name: ' Ada ',
      last_name: ' Lovelace ',
      password: 'account password',
      confirm_password: 'account password',
    });

    expect(parsed).toMatchObject({ first_name: 'Ada', last_name: 'Lovelace' });
  });
});
