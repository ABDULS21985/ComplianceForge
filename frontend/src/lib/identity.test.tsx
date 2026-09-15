import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createPasskeyCredential,
  getPasskeyCredential,
  identityReasonValid,
  serializePublicKeyCredential,
} from '@/lib/identity';

function bytes(...values: number[]): ArrayBuffer {
  return Uint8Array.from(values).buffer;
}

function registrationCredential(): PublicKeyCredential {
  return {
    authenticatorAttachment: 'platform',
    getClientExtensionResults: () => ({}),
    id: 'credential-id',
    rawId: bytes(7, 8, 9),
    response: {
      attestationObject: bytes(10, 11),
      clientDataJSON: bytes(12, 13),
      getAuthenticatorData: () => bytes(),
      getPublicKey: () => null,
      getPublicKeyAlgorithm: () => -7,
      getTransports: () => ['internal'],
    } as AuthenticatorAttestationResponse,
    toJSON: () => ({}),
    type: 'public-key',
  } as PublicKeyCredential;
}

function assertionCredential(): PublicKeyCredential {
  return {
    authenticatorAttachment: 'cross-platform',
    getClientExtensionResults: () => ({ appid: false }),
    id: 'assertion-id',
    rawId: bytes(1, 2),
    response: {
      authenticatorData: bytes(3, 4),
      clientDataJSON: bytes(5, 6),
      signature: bytes(7, 8),
      userHandle: bytes(9, 10),
    } as AuthenticatorAssertionResponse,
    toJSON: () => ({}),
    type: 'public-key',
  } as PublicKeyCredential;
}

describe('identity browser security helpers', () => {
  const originalCredentials = Object.getOwnPropertyDescriptor(navigator, 'credentials');

  beforeEach(() => {
    vi.stubGlobal('PublicKeyCredential', class PublicKeyCredential {});
  });

  afterEach(() => {
    if (originalCredentials) Object.defineProperty(navigator, 'credentials', originalCredentials);
    else Reflect.deleteProperty(navigator, 'credentials');
    vi.unstubAllGlobals();
  });

  it('converts server registration options and returns only base64url ceremony data', async () => {
    const create = vi.fn().mockResolvedValue(registrationCredential());
    Object.defineProperty(navigator, 'credentials', {
      configurable: true,
      value: { create, get: vi.fn() },
    });
    const storage = vi.spyOn(Storage.prototype, 'setItem');

    const serialized = await createPasskeyCredential({
      publicKey: {
        challenge: 'AQID',
        excludeCredentials: [{ id: 'BAUG', type: 'public-key' }],
        pubKeyCredParams: [{ alg: -7, type: 'public-key' }],
        rp: { id: 'app.example.test', name: 'ComplianceForge' },
        user: { displayName: 'Ada', id: 'BwgJ', name: 'ada@example.test' },
      },
    });

    const browserOptions = create.mock.calls[0][0].publicKey;
    expect([...new Uint8Array(browserOptions.challenge)]).toEqual([1, 2, 3]);
    expect([...new Uint8Array(browserOptions.user.id)]).toEqual([7, 8, 9]);
    expect([...new Uint8Array(browserOptions.excludeCredentials[0].id)]).toEqual([4, 5, 6]);
    expect(serialized).toMatchObject({
      id: 'credential-id',
      rawId: 'BwgJ',
      response: {
        attestationObject: 'Cgs',
        clientDataJSON: 'DA0',
        transports: ['internal'],
      },
    });
    expect(storage).not.toHaveBeenCalled();
  });

  it('converts authentication allowlists and serializes an assertion without storage', async () => {
    const get = vi.fn().mockResolvedValue(assertionCredential());
    Object.defineProperty(navigator, 'credentials', {
      configurable: true,
      value: { create: vi.fn(), get },
    });
    const storage = vi.spyOn(Storage.prototype, 'setItem');

    const serialized = await getPasskeyCredential({
      publicKey: {
        allowCredentials: [{ id: 'AQI', transports: ['usb'], type: 'public-key' }],
        challenge: 'AwQ',
        rpId: 'app.example.test',
      },
    });

    const browserOptions = get.mock.calls[0][0].publicKey;
    expect([...new Uint8Array(browserOptions.challenge)]).toEqual([3, 4]);
    expect([...new Uint8Array(browserOptions.allowCredentials[0].id)]).toEqual([1, 2]);
    expect(serialized).toMatchObject({
      id: 'assertion-id',
      response: {
        authenticatorData: 'AwQ',
        clientDataJSON: 'BQY',
        signature: 'Bwg',
        userHandle: 'CQo',
      },
    });
    expect(storage).not.toHaveBeenCalled();
  });

  it('rejects malformed server ceremonies and untrimmed audit reasons', async () => {
    Object.defineProperty(navigator, 'credentials', {
      configurable: true,
      value: { create: vi.fn(), get: vi.fn() },
    });

    await expect(createPasskeyCredential({ publicKey: {} })).rejects.toThrow(
      'invalid passkey options',
    );
    expect(identityReasonValid('Account recovery approved')).toBe(true);
    expect(identityReasonValid(' Account recovery approved')).toBe(false);
    expect(identityReasonValid('no')).toBe(false);
  });

  it('serializes a credential without returning browser credential objects', () => {
    const result = serializePublicKeyCredential(assertionCredential());
    expect(JSON.stringify(result)).not.toContain('[object ArrayBuffer]');
    expect(result).not.toHaveProperty('response.signature.byteLength');
  });
});
