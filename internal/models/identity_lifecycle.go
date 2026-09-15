package models

import (
	"encoding/json"
	"time"
)

type IdentityMethod string

const (
	IdentityMethodPassword     IdentityMethod = "password"
	IdentityMethodTOTP         IdentityMethod = "totp"
	IdentityMethodRecoveryCode IdentityMethod = "recovery_code"
	IdentityMethodPasskey      IdentityMethod = "passkey"
)

type IdentityRequestMetadata struct {
	RequestID  string `json:"-"`
	IPAddress  string `json:"-"`
	UserAgent  string `json:"-"`
	DeviceName string `json:"-"`
}

type IdentityAccountState struct {
	User             User                      `json:"user"`
	EmailVerifiedAt  *time.Time                `json:"email_verified_at,omitempty"`
	MFAExemptUntil   *time.Time                `json:"mfa_exempt_until,omitempty"`
	MFAExemptReason  string                    `json:"mfa_exempt_reason,omitempty"`
	IdentityVersion  int64                     `json:"identity_version"`
	InvitationStatus DirectoryInvitationStatus `json:"invitation_status"`
}

type IdentityAcceptanceResult struct {
	UserID          string    `json:"user_id"`
	OrganizationID  string    `json:"organization_id"`
	EmailVerifiedAt time.Time `json:"email_verified_at"`
}

type IdentityPolicy struct {
	OrganizationID              string    `json:"organization_id"`
	RequireMFA                  bool      `json:"require_mfa"`
	RequireMFAForAdmins         bool      `json:"require_mfa_for_admins"`
	AllowedMethods              []string  `json:"allowed_methods"`
	EnrollmentGraceHours        int       `json:"enrollment_grace_hours"`
	AuthenticationChallengeMins int       `json:"authentication_challenge_minutes"`
	StepUpTTLMinutes            int       `json:"step_up_ttl_minutes"`
	Version                     int64     `json:"version"`
	UpdatedBy                   string    `json:"updated_by"`
	UpdateReason                string    `json:"update_reason"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

type IdentityPolicyPatch struct {
	ExpectedVersion             int64    `json:"expected_version"`
	RequireMFA                  *bool    `json:"require_mfa,omitempty"`
	RequireMFAForAdmins         *bool    `json:"require_mfa_for_admins,omitempty"`
	AllowedMethods              []string `json:"allowed_methods,omitempty"`
	EnrollmentGraceHours        *int     `json:"enrollment_grace_hours,omitempty"`
	AuthenticationChallengeMins *int     `json:"authentication_challenge_minutes,omitempty"`
	StepUpTTLMinutes            *int     `json:"step_up_ttl_minutes,omitempty"`
	Reason                      string   `json:"reason"`
}

type IdentityInvitation struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	UserID         string     `json:"user_id"`
	Email          string     `json:"email"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	DeliveryQueued bool       `json:"delivery_queued"`
	PlainToken     string     `json:"-"`
	TokenHash      string     `json:"-"`
	DeliveryCipher string     `json:"-"`
}

type IdentityInvitationIssueInput struct {
	ExpiresInHours int    `json:"expires_in_hours,omitempty"`
	Reason         string `json:"reason"`
}

type IdentityInvitationAcceptInput struct {
	Token     string `json:"token"`
	Password  string `json:"password"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

type IdentityEmailRequest struct {
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
}

type IdentityCredentialToken struct {
	ID             string
	OrganizationID string
	UserID         string
	Email          string
	TokenHash      string
	DeliveryCipher string
	IssuedAt       time.Time
	ExpiresAt      time.Time
	Metadata       IdentityRequestMetadata
}

type IdentityTokenInput struct {
	Token string `json:"token"`
}

type IdentityPasswordResetInput struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type IdentitySession struct {
	ID                   string         `json:"id"`
	UserID               string         `json:"user_id"`
	OrganizationID       string         `json:"organization_id"`
	IPAddress            string         `json:"ip_address,omitempty"`
	UserAgent            string         `json:"user_agent,omitempty"`
	DeviceName           string         `json:"device_name,omitempty"`
	AuthenticationMethod IdentityMethod `json:"authentication_method"`
	MFAVerifiedAt        *time.Time     `json:"mfa_verified_at,omitempty"`
	ExpiresAt            time.Time      `json:"expires_at"`
	LastSeenAt           time.Time      `json:"last_seen_at"`
	RevokedAt            *time.Time     `json:"revoked_at,omitempty"`
	RevokeReason         string         `json:"revoke_reason,omitempty"`
	Version              int64          `json:"version"`
	CreatedAt            time.Time      `json:"created_at"`
	Current              bool           `json:"current"`
}

type IdentitySessionRevokeInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type IdentityGlobalSignOutInput struct {
	ExceptCurrent bool   `json:"except_current"`
	Reason        string `json:"reason"`
}

type IdentityMFAFactor struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	UserID         string         `json:"user_id"`
	Method         IdentityMethod `json:"method"`
	DisplayName    string         `json:"display_name,omitempty"`
	Primary        bool           `json:"is_primary"`
	Verified       bool           `json:"is_verified"`
	VerifiedAt     *time.Time     `json:"verified_at,omitempty"`
	LastUsedAt     *time.Time     `json:"last_used_at,omitempty"`
	DisabledAt     *time.Time     `json:"disabled_at,omitempty"`
	RecoveryCodes  int            `json:"recovery_codes_remaining"`
	Version        int64          `json:"version"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	SecretCipher   []byte         `json:"-"`
	LastTOTPStep   *int64         `json:"-"`
}

type IdentityTOTPEnrollmentInput struct {
	DisplayName string `json:"display_name,omitempty"`
	Reason      string `json:"reason"`
}

type IdentityTOTPEnrollment struct {
	FactorID        string    `json:"factor_id"`
	Secret          string    `json:"secret"`
	OTPAuthURI      string    `json:"otpauth_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	ExpectedVersion int64     `json:"expected_version"`
}

type IdentityTOTPVerifyInput struct {
	FactorID        string `json:"factor_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Code            string `json:"code"`
	Reason          string `json:"reason"`
}

type IdentityTOTPVerifyResult struct {
	Factor        IdentityMFAFactor `json:"factor"`
	RecoveryCodes []string          `json:"recovery_codes"`
}

type IdentityMFADisableInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
	StepUpToken     string `json:"-"`
}

type IdentityRecoveryRegenerateInput struct {
	Reason      string `json:"reason"`
	StepUpToken string `json:"-"`
}

type IdentityPasskey struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organization_id"`
	UserID           string     `json:"user_id"`
	DeviceName       string     `json:"device_name"`
	Transports       []string   `json:"transports"`
	SignCount        uint32     `json:"sign_count"`
	CloneWarning     bool       `json:"clone_warning"`
	BackupEligible   bool       `json:"backup_eligible"`
	BackupState      bool       `json:"backup_state"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	RemovedAt        *time.Time `json:"removed_at,omitempty"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CredentialID     []byte     `json:"-"`
	CredentialCipher string     `json:"-"`
	AttestationType  string     `json:"-"`
	AAGUID           *string    `json:"-"`
}

type IdentityPasskeyRegistrationBeginInput struct {
	DeviceName string `json:"device_name"`
}

type IdentityPasskeyCeremony struct {
	ChallengeToken string          `json:"challenge_token"`
	Options        json.RawMessage `json:"options"`
	ExpiresAt      time.Time       `json:"expires_at"`
}

type IdentityPasskeyRegistrationFinishInput struct {
	ChallengeToken string          `json:"challenge_token"`
	DeviceName     string          `json:"device_name"`
	Credential     json.RawMessage `json:"credential"`
}

type IdentityPasskeyAuthenticationBeginInput struct {
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
	Purpose        string `json:"purpose,omitempty"`
}

type IdentityPasskeyAuthenticationFinishInput struct {
	ChallengeToken string          `json:"challenge_token"`
	Credential     json.RawMessage `json:"credential"`
}

type IdentityPasskeyRemoveInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
	StepUpToken     string `json:"-"`
}

type IdentityAuthenticationChallenge struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organization_id"`
	UserID          string          `json:"user_id"`
	SessionID       *string         `json:"session_id,omitempty"`
	Purpose         string          `json:"purpose"`
	AllowedMethods  []string        `json:"allowed_methods"`
	StepUpPurpose   string          `json:"step_up_purpose,omitempty"`
	WebAuthnSession json.RawMessage `json:"-"`
	ExpiresAt       time.Time       `json:"expires_at"`
	ConsumedAt      *time.Time      `json:"consumed_at,omitempty"`
	FailedAttempts  int             `json:"failed_attempts"`
	MaxAttempts     int             `json:"max_attempts"`
	PlainToken      string          `json:"-"`
	TokenHash       string          `json:"-"`
}

type IdentityMFAChallengeResponse struct {
	ChallengeToken string          `json:"challenge_token"`
	Methods        []string        `json:"methods"`
	PasskeyOptions json.RawMessage `json:"passkey_options,omitempty"`
	ExpiresAt      time.Time       `json:"expires_at"`
}

type IdentityMFAProofInput struct {
	ChallengeToken string          `json:"challenge_token"`
	Method         string          `json:"method"`
	Code           string          `json:"code,omitempty"`
	Credential     json.RawMessage `json:"credential,omitempty"`
}

type IdentityAuthenticationResult struct {
	OrganizationID       string         `json:"organization_id"`
	UserID               string         `json:"user_id"`
	AuthenticationMethod IdentityMethod `json:"authentication_method"`
	AuthenticatedAt      time.Time      `json:"authenticated_at"`
}

type IdentityStepUpBeginInput struct {
	Purpose string `json:"purpose"`
}

type IdentityStepUpGrant struct {
	ID                   string         `json:"id"`
	OrganizationID       string         `json:"organization_id"`
	UserID               string         `json:"user_id"`
	SessionID            string         `json:"session_id"`
	Purpose              string         `json:"purpose"`
	AuthenticationMethod IdentityMethod `json:"authentication_method"`
	ExpiresAt            time.Time      `json:"expires_at"`
	PlainToken           string         `json:"grant_token,omitempty"`
}

type IdentityAdminMFAResetInput struct {
	Reason         string `json:"reason"`
	ExemptionHours int    `json:"exemption_hours,omitempty"`
	StepUpToken    string `json:"-"`
}

type IdentitySecurityEvent struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	UserID         *string         `json:"user_id,omitempty"`
	ActorUserID    *string         `json:"actor_user_id,omitempty"`
	SessionID      *string         `json:"session_id,omitempty"`
	EventType      string          `json:"event_type"`
	Reason         string          `json:"reason"`
	RequestID      string          `json:"request_id,omitempty"`
	IPAddress      string          `json:"ip_address,omitempty"`
	UserAgent      string          `json:"user_agent,omitempty"`
	Details        json.RawMessage `json:"details"`
	CreatedAt      time.Time       `json:"created_at"`
}
