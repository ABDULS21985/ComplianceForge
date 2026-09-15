import type { PaginationMeta } from '@/types/enterprise-settings';
import type { User } from '@/types';

export type IdentityMethod = 'password' | 'totp' | 'recovery_code' | 'passkey';
export type IdentityMFAAuthenticationMethod = Exclude<IdentityMethod, 'password'>;
export type IdentityConfigurableMethod = Extract<IdentityMethod, 'totp' | 'passkey'>;

export interface IdentityAcceptanceResult {
  user_id: string;
  organization_id: string;
  email_verified_at: string;
}

export interface IdentityEmailRequest {
  organization_id: string;
  email: string;
}

export interface IdentityInvitationAcceptInput {
  token: string;
  password: string;
  first_name?: string;
  last_name?: string;
}

export interface IdentityPasswordResetInput {
  token: string;
  new_password: string;
}

export interface IdentityMFAChallenge {
  challenge_token: string;
  methods: IdentityMFAAuthenticationMethod[];
  passkey_options?: Record<string, unknown>;
  expires_at: string;
}

export interface IdentityMFAProofInput {
  challenge_token: string;
  method: IdentityMFAAuthenticationMethod;
  code?: string;
  credential?: Record<string, unknown>;
}

/** Exact upstream shape. Access and refresh credentials are server-only. */
export interface IdentityTokenResponse {
  access_token?: string;
  refresh_token?: string;
  expires_at?: string;
  user?: User;
  mfa_required?: boolean;
  mfa_challenge?: IdentityMFAChallenge;
  email_verification_required?: boolean;
}

export interface AuthenticatedSessionResponse {
  expires_at: string;
  user: User;
}

export interface PendingMFAResponse {
  user: User;
  mfa_required: true;
  mfa_challenge: IdentityMFAChallenge;
}

export interface PendingEmailVerificationResponse {
  user: User;
  email_verification_required: true;
}

export type BrowserAuthenticationResponse =
  AuthenticatedSessionResponse | PendingMFAResponse | PendingEmailVerificationResponse;

export interface IdentityPolicy {
  organization_id: string;
  require_mfa: boolean;
  require_mfa_for_admins: boolean;
  allowed_methods: IdentityConfigurableMethod[];
  enrollment_grace_hours: number;
  authentication_challenge_minutes: number;
  step_up_ttl_minutes: number;
  version: number;
  updated_by: string;
  update_reason: string;
  created_at: string;
  updated_at: string;
}

export interface IdentityPolicyPatch {
  expected_version: number;
  require_mfa?: boolean;
  require_mfa_for_admins?: boolean;
  allowed_methods?: IdentityConfigurableMethod[];
  enrollment_grace_hours?: number;
  authentication_challenge_minutes?: number;
  step_up_ttl_minutes?: number;
  reason: string;
}

export interface IdentitySession {
  id: string;
  user_id: string;
  organization_id: string;
  ip_address?: string;
  user_agent?: string;
  device_name?: string;
  authentication_method: IdentityMethod;
  mfa_verified_at?: string;
  expires_at: string;
  last_seen_at: string;
  revoked_at?: string;
  revoke_reason?: string;
  version: number;
  created_at: string;
  current: boolean;
}

export interface IdentitySessionRevokeInput {
  expected_version: number;
  reason: string;
}

export interface IdentityGlobalSignOutInput {
  except_current: boolean;
  reason: string;
}

export interface IdentityGlobalSignOutResult {
  revoked_sessions: number;
}

export interface IdentityMFAFactor {
  id: string;
  organization_id: string;
  user_id: string;
  method: 'totp';
  display_name?: string;
  is_primary: boolean;
  is_verified: boolean;
  verified_at?: string;
  last_used_at?: string;
  disabled_at?: string;
  recovery_codes_remaining: number;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface IdentityTOTPEnrollmentInput {
  display_name?: string;
  reason: string;
}

export interface IdentityTOTPEnrollment {
  factor_id: string;
  secret: string;
  otpauth_uri: string;
  expires_at: string;
  expected_version: number;
}

export interface IdentityTOTPVerifyInput {
  factor_id: string;
  expected_version: number;
  code: string;
  reason: string;
}

export interface IdentityTOTPVerifyResult {
  factor: IdentityMFAFactor;
  recovery_codes: string[];
}

export interface IdentityMFADisableInput {
  expected_version: number;
  reason: string;
}

export interface IdentityRecoveryRegenerateInput {
  reason: string;
}

export interface IdentityRecoveryCodesResult {
  recovery_codes: string[];
}

export interface IdentityPasskey {
  id: string;
  organization_id: string;
  user_id: string;
  device_name: string;
  transports: string[];
  sign_count: number;
  clone_warning: boolean;
  backup_eligible: boolean;
  backup_state: boolean;
  last_used_at?: string;
  removed_at?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface IdentityPasskeyRegistrationBeginInput {
  device_name: string;
}

export interface IdentityPasskeyCeremony {
  challenge_token: string;
  options: Record<string, unknown>;
  expires_at: string;
}

export interface IdentityPasskeyRegistrationFinishInput {
  challenge_token: string;
  device_name: string;
  credential: Record<string, unknown>;
}

export interface IdentityPasskeyAuthenticationBeginInput {
  organization_id: string;
  email: string;
  purpose?: 'login';
}

export interface IdentityPasskeyAuthenticationFinishInput {
  challenge_token: string;
  credential: Record<string, unknown>;
}

export interface IdentityPasskeyRemoveInput {
  expected_version: number;
  reason: string;
}

export interface IdentityStepUpBeginInput {
  purpose: string;
}

export interface IdentityStepUpGrant {
  id: string;
  organization_id: string;
  user_id: string;
  session_id: string;
  purpose: string;
  authentication_method: IdentityMFAAuthenticationMethod;
  expires_at: string;
  grant_token: string;
}

export interface IdentityInvitationIssueInput {
  expires_in_hours?: number;
  reason: string;
}

export interface IdentityInvitation {
  id: string;
  organization_id: string;
  user_id: string;
  email: string;
  expires_at: string;
  accepted_at?: string;
  revoked_at?: string;
  created_by: string;
  created_at: string;
  delivery_queued: boolean;
}

export interface IdentityAdminMFAResetInput {
  reason: string;
  exemption_hours?: number;
}

export interface IdentitySecurityEvent {
  id: string;
  organization_id: string;
  user_id?: string;
  actor_user_id?: string;
  session_id?: string;
  event_type: string;
  reason: string;
  request_id?: string;
  ip_address?: string;
  user_agent?: string;
  details: unknown;
  created_at: string;
}

export interface IdentitySecurityEventList {
  data: IdentitySecurityEvent[];
  pagination: PaginationMeta;
}
