export const STEP_UP_TOKEN_HEADER = 'X-Step-Up-Token';

export const PUBLIC_IDENTITY_ROUTES = {
  acceptInvitation: '/api/auth/invitations/accept',
  requestEmailVerification: '/api/auth/email-verification/request',
  confirmEmailVerification: '/api/auth/email-verification/confirm',
  forgotPassword: '/api/auth/password/forgot',
  resetPassword: '/api/auth/password/reset',
  verifyLoginMFA: '/api/auth/mfa/verify',
  beginPasskeyAuthentication: '/api/auth/passkeys/authentication/options',
  verifyPasskeyAuthentication: '/api/auth/passkeys/authentication/verify',
} as const;

export const IDENTITY_ROUTES = {
  policy: '/identity/policy',
  sessions: '/identity/sessions',
  revokeSession: (sessionId: string) => `/identity/sessions/${encodeURIComponent(sessionId)}`,
  globalSignOut: '/identity/sessions/sign-out',
  factors: '/identity/mfa/factors',
  totpEnrollment: '/identity/mfa/totp/enrollment',
  totpVerification: '/identity/mfa/totp/verification',
  disableFactor: (factorId: string) => `/identity/mfa/factors/${encodeURIComponent(factorId)}`,
  recoveryCodes: (factorId: string) =>
    `/identity/mfa/factors/${encodeURIComponent(factorId)}/recovery-codes`,
  beginStepUp: '/identity/step-up/challenges',
  verifyStepUp: '/identity/step-up/verify',
  passkeys: '/identity/passkeys',
  beginPasskeyRegistration: '/identity/passkeys/registration/options',
  verifyPasskeyRegistration: '/identity/passkeys/registration/verify',
  removePasskey: (passkeyId: string) => `/identity/passkeys/${encodeURIComponent(passkeyId)}`,
  history: '/identity/history',
  issueInvitation: (userId: string) => `/directory/users/${encodeURIComponent(userId)}/invitation`,
  adminResetMFA: (userId: string) => `/directory/users/${encodeURIComponent(userId)}/mfa/reset`,
} as const;

export const identityKeys = {
  sessions: ['identity', 'sessions'] as const,
  factors: ['identity', 'mfa', 'factors'] as const,
  passkeys: ['identity', 'passkeys'] as const,
  policy: ['identity', 'policy'] as const,
  history: (userId: string, page: number) => ['identity', 'history', userId, page] as const,
};

export function isAuthenticatedResponse(value: unknown): value is {
  expires_at: string;
  user: import('@/types').User;
} {
  if (!value || typeof value !== 'object') return false;
  const record = value as Record<string, unknown>;
  return typeof record.expires_at === 'string' && Boolean(record.user);
}

export function isPendingMFAResponse(value: unknown): value is {
  user: import('@/types').User;
  mfa_required: true;
  mfa_challenge: import('@/types/identity').IdentityMFAChallenge;
} {
  if (!value || typeof value !== 'object') return false;
  const record = value as Record<string, unknown>;
  return record.mfa_required === true && Boolean(record.user) && Boolean(record.mfa_challenge);
}

export function isPendingEmailVerificationResponse(value: unknown): value is {
  user: import('@/types').User;
  email_verification_required: true;
} {
  if (!value || typeof value !== 'object') return false;
  const record = value as Record<string, unknown>;
  return record.email_verification_required === true && Boolean(record.user);
}

export function identityReasonValid(value: string): boolean {
  const trimmed = value.trim();
  return trimmed.length >= 3 && trimmed.length <= 1000 && trimmed === value;
}

export function formatIdentityMethod(method: string): string {
  if (method === 'totp') return 'Authenticator app';
  if (method === 'recovery_code') return 'Recovery code';
  if (method === 'passkey') return 'Passkey';
  if (method === 'password') return 'Password';
  return method.replace(/[_-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function webAuthnSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.PublicKeyCredential !== 'undefined' &&
    typeof navigator.credentials?.create === 'function' &&
    typeof navigator.credentials?.get === 'function'
  );
}

function decodeBase64Url(value: string): ArrayBuffer {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=');
  const binary = atob(padded);
  return Uint8Array.from(binary, (character) => character.charCodeAt(0)).buffer;
}

function encodeBase64Url(value: ArrayBuffer): string {
  const bytes = new Uint8Array(value);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function credentialDescriptor(value: unknown): PublicKeyCredentialDescriptor {
  const descriptor = value as { id: string; transports?: AuthenticatorTransport[]; type?: string };
  return {
    id: decodeBase64Url(descriptor.id),
    transports: descriptor.transports,
    type: 'public-key',
  };
}

function publicKeyOptions(value: Record<string, unknown>, registration: boolean) {
  const wrapper = value.publicKey as Record<string, unknown> | undefined;
  if (!wrapper || typeof wrapper.challenge !== 'string') {
    throw new Error('The server returned invalid passkey options.');
  }
  const options: Record<string, unknown> = {
    ...wrapper,
    challenge: decodeBase64Url(wrapper.challenge),
  };
  if (registration) {
    const user = wrapper.user as { id?: string } | undefined;
    if (!user || typeof user.id !== 'string') {
      throw new Error('The server returned invalid passkey user options.');
    }
    options.user = { ...user, id: decodeBase64Url(user.id) };
    if (Array.isArray(wrapper.excludeCredentials)) {
      options.excludeCredentials = wrapper.excludeCredentials.map(credentialDescriptor);
    }
  } else if (Array.isArray(wrapper.allowCredentials)) {
    options.allowCredentials = wrapper.allowCredentials.map(credentialDescriptor);
  }
  return options;
}

export async function createPasskeyCredential(
  options: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  if (!webAuthnSupported()) throw new Error('Passkeys are not supported by this browser.');
  const credential = (await navigator.credentials.create({
    publicKey: publicKeyOptions(options, true) as unknown as PublicKeyCredentialCreationOptions,
  })) as PublicKeyCredential | null;
  if (!credential) throw new Error('Passkey registration was cancelled.');
  return serializePublicKeyCredential(credential);
}

export async function getPasskeyCredential(
  options: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  if (!webAuthnSupported()) throw new Error('Passkeys are not supported by this browser.');
  const credential = (await navigator.credentials.get({
    publicKey: publicKeyOptions(options, false) as unknown as PublicKeyCredentialRequestOptions,
  })) as PublicKeyCredential | null;
  if (!credential) throw new Error('Passkey authentication was cancelled.');
  return serializePublicKeyCredential(credential);
}

export function serializePublicKeyCredential(
  credential: PublicKeyCredential,
): Record<string, unknown> {
  const response = credential.response;
  const base = {
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults(),
    id: credential.id,
    rawId: encodeBase64Url(credential.rawId),
    type: credential.type,
  };
  if ('attestationObject' in response) {
    const attestation = response as AuthenticatorAttestationResponse;
    return {
      ...base,
      response: {
        attestationObject: encodeBase64Url(attestation.attestationObject),
        clientDataJSON: encodeBase64Url(attestation.clientDataJSON),
        transports: attestation.getTransports?.() ?? [],
      },
    };
  }
  const assertion = response as AuthenticatorAssertionResponse;
  return {
    ...base,
    response: {
      authenticatorData: encodeBase64Url(assertion.authenticatorData),
      clientDataJSON: encodeBase64Url(assertion.clientDataJSON),
      signature: encodeBase64Url(assertion.signature),
      userHandle: assertion.userHandle ? encodeBase64Url(assertion.userHandle) : null,
    },
  };
}
