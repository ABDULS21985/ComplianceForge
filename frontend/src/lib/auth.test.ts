import { describe, expect, it, vi } from 'vitest';

import { decodeJwt, isTokenAuthenticated } from '@/lib/auth';

function createToken(payload: Record<string, unknown>): string {
  const encodedPayload = Buffer.from(JSON.stringify(payload)).toString('base64url');
  return `header.${encodedPayload}.signature`;
}

describe('JWT compatibility helpers', () => {
  it('decodes a valid base64url payload', () => {
    expect(
      decodeJwt(
        createToken({
          sub: 'user-1',
          email: 'owner@example.test',
          iat: 1_700_000_000,
          exp: 1_900_000_000,
        })
      )
    ).toMatchObject({ sub: 'user-1', email: 'owner@example.test' });
  });

  it.each(['', 'not-a-jwt', 'a.b.c.d', 'a.%%%.c'])(
    'rejects malformed token %s',
    (token) => {
      expect(decodeJwt(token)).toBeNull();
    }
  );

  it('accepts only tokens whose expiry is in the future', () => {
    vi.spyOn(Date, 'now').mockReturnValue(1_800_000_000_000);

    expect(
      isTokenAuthenticated(
        createToken({
          sub: 'user-1',
          email: 'owner@example.test',
          iat: 1_700_000_000,
          exp: 1_800_000_001,
        })
      )
    ).toBe(true);
    expect(
      isTokenAuthenticated(
        createToken({
          sub: 'user-1',
          email: 'owner@example.test',
          iat: 1_700_000_000,
          exp: 1_800_000_000,
        })
      )
    ).toBe(false);
  });
});
