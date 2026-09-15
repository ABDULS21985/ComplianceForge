import { describe, expect, it } from 'vitest';
import { IDENTITY_ROUTES, PUBLIC_IDENTITY_ROUTES } from '@/lib/identity';
import type {
  IdentityInvitation,
  IdentityMFAFactor,
  IdentityPasskey,
  IdentityPolicy,
  IdentitySecurityEvent,
  IdentitySession,
} from '@/types/identity';
import { fileURLToPath } from 'node:url';
import { readFileSync } from 'node:fs';

interface OpenAPIOperation {
  parameters?: Array<{ $ref?: string; in?: string; name?: string }>;
}

interface OpenAPISchema {
  properties?: Record<string, { maximum?: number; minimum?: number }>;
  required?: string[];
}

interface OpenAPIDocument {
  components: { schemas: Record<string, OpenAPISchema> };
  paths: Record<string, Record<string, OpenAPIOperation>>;
}

type ExactStringKeys<T, Keys extends readonly string[]> = Exclude<
  Extract<keyof T, string>,
  Keys[number]
> extends never
  ? Exclude<Keys[number], Extract<keyof T, string>> extends never
    ? true
    : false
  : false;
type Assert<T extends true> = T;

const policyKeys = [
  'organization_id',
  'require_mfa',
  'require_mfa_for_admins',
  'allowed_methods',
  'enrollment_grace_hours',
  'authentication_challenge_minutes',
  'step_up_ttl_minutes',
  'version',
  'updated_by',
  'update_reason',
  'created_at',
  'updated_at',
] as const;
const sessionKeys = [
  'id',
  'user_id',
  'organization_id',
  'ip_address',
  'user_agent',
  'device_name',
  'authentication_method',
  'mfa_verified_at',
  'expires_at',
  'last_seen_at',
  'revoked_at',
  'revoke_reason',
  'version',
  'created_at',
  'current',
] as const;
const factorKeys = [
  'id',
  'organization_id',
  'user_id',
  'method',
  'display_name',
  'is_primary',
  'is_verified',
  'verified_at',
  'last_used_at',
  'disabled_at',
  'recovery_codes_remaining',
  'version',
  'created_at',
  'updated_at',
] as const;
const passkeyKeys = [
  'id',
  'organization_id',
  'user_id',
  'device_name',
  'transports',
  'sign_count',
  'clone_warning',
  'backup_eligible',
  'backup_state',
  'last_used_at',
  'removed_at',
  'version',
  'created_at',
  'updated_at',
] as const;
const invitationKeys = [
  'id',
  'organization_id',
  'user_id',
  'email',
  'expires_at',
  'accepted_at',
  'revoked_at',
  'created_by',
  'created_at',
  'delivery_queued',
] as const;
const eventKeys = [
  'id',
  'organization_id',
  'user_id',
  'actor_user_id',
  'session_id',
  'event_type',
  'reason',
  'request_id',
  'ip_address',
  'user_agent',
  'details',
  'created_at',
] as const;

const assertions: [
  Assert<ExactStringKeys<IdentityPolicy, typeof policyKeys>>,
  Assert<ExactStringKeys<IdentitySession, typeof sessionKeys>>,
  Assert<ExactStringKeys<IdentityMFAFactor, typeof factorKeys>>,
  Assert<ExactStringKeys<IdentityPasskey, typeof passkeyKeys>>,
  Assert<ExactStringKeys<IdentityInvitation, typeof invitationKeys>>,
  Assert<ExactStringKeys<IdentitySecurityEvent, typeof eventKeys>>,
] = [true, true, true, true, true, true];
void assertions;

const contractPath = fileURLToPath(new URL('../../../api/openapi/openapi.json', import.meta.url));
const contract = JSON.parse(readFileSync(contractPath, 'utf8')) as OpenAPIDocument;
const apiPath = (path: string) => `/api/v1${path}`;

function expectSchemaKeys(name: string, expected: readonly string[]): void {
  expect(Object.keys(contract.components.schemas[name]?.properties ?? {}).sort()).toEqual(
    [...expected].sort(),
  );
}

function operation(method: string, path: string): OpenAPIOperation {
  const found = contract.paths[path]?.[method.toLowerCase()];
  expect(found, `${method} ${path}`).toBeDefined();
  return found;
}

function upstreamPublic(path: string): string {
  return `/api/v1${path.replace(/^\/api/, '')}`;
}

describe('identity lifecycle OpenAPI drift contract', () => {
  it('keeps the principal identity DTOs aligned exactly', () => {
    expectSchemaKeys('IdentityPolicy', policyKeys);
    expectSchemaKeys('IdentitySession', sessionKeys);
    expectSchemaKeys('IdentityMFAFactor', factorKeys);
    expectSchemaKeys('IdentityPasskey', passkeyKeys);
    expectSchemaKeys('IdentityInvitation', invitationKeys);
    expectSchemaKeys('IdentitySecurityEvent', eventKeys);
    expect(
      contract.components.schemas.IdentityPolicyPatch?.properties
        ?.authentication_challenge_minutes?.maximum,
    ).toBe(15);
    expect(
      contract.components.schemas.IdentityInvitationIssueInput?.properties?.expires_in_hours
        ?.maximum,
    ).toBe(720);
  });

  it('documents all eight public identity operations', () => {
    for (const path of Object.values(PUBLIC_IDENTITY_ROUTES)) {
      operation('POST', upstreamPublic(path));
    }
  });

  it('documents all protected self-service and policy operations', () => {
    const factorId = 'contract-factor';
    const passkeyId = 'contract-passkey';
    const sessionId = 'contract-session';
    const replaceParams = (path: string) =>
      apiPath(path)
        .replace(factorId, '{factorID}')
        .replace(passkeyId, '{passkeyID}')
        .replace(sessionId, '{sessionID}');

    for (const method of ['GET', 'PUT']) operation(method, apiPath(IDENTITY_ROUTES.policy));
    operation('GET', apiPath(IDENTITY_ROUTES.sessions));
    operation('DELETE', replaceParams(IDENTITY_ROUTES.revokeSession(sessionId)));
    operation('POST', apiPath(IDENTITY_ROUTES.globalSignOut));
    operation('GET', apiPath(IDENTITY_ROUTES.factors));
    operation('POST', apiPath(IDENTITY_ROUTES.totpEnrollment));
    operation('POST', apiPath(IDENTITY_ROUTES.totpVerification));
    operation('DELETE', replaceParams(IDENTITY_ROUTES.disableFactor(factorId)));
    operation('POST', replaceParams(IDENTITY_ROUTES.recoveryCodes(factorId)));
    operation('POST', apiPath(IDENTITY_ROUTES.beginStepUp));
    operation('POST', apiPath(IDENTITY_ROUTES.verifyStepUp));
    operation('GET', apiPath(IDENTITY_ROUTES.passkeys));
    operation('POST', apiPath(IDENTITY_ROUTES.beginPasskeyRegistration));
    operation('POST', apiPath(IDENTITY_ROUTES.verifyPasskeyRegistration));
    operation('DELETE', replaceParams(IDENTITY_ROUTES.removePasskey(passkeyId)));
    operation('GET', apiPath(IDENTITY_ROUTES.history));
  });

  it('documents the administrator invitation and step-up-protected MFA reset', () => {
    const userId = 'contract-user';
    const replaceUser = (path: string) => apiPath(path).replace(userId, '{id}');
    operation('POST', replaceUser(IDENTITY_ROUTES.issueInvitation(userId)));
    const reset = operation('POST', replaceUser(IDENTITY_ROUTES.adminResetMFA(userId)));

    expect(reset.parameters).toContainEqual({
      $ref: '#/components/parameters/StepUpToken',
    });
  });

  it('requires the documented step-up header on every sensitive self-service mutation', () => {
    const protectedOperations = [
      operation('DELETE', '/api/v1/identity/mfa/factors/{factorID}'),
      operation('POST', '/api/v1/identity/mfa/factors/{factorID}/recovery-codes'),
      operation('DELETE', '/api/v1/identity/passkeys/{passkeyID}'),
    ];

    for (const protectedOperation of protectedOperations) {
      expect(protectedOperation.parameters).toContainEqual({
        $ref: '#/components/parameters/StepUpToken',
      });
    }
  });
});
