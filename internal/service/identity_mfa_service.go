package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- HMAC-SHA1 only for the existing RFC 4226/6238 TOTP profile, never general-purpose hashing.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/complianceforge/platform/internal/models"
)

const identityTOTPStepSeconds int64 = 30

func (s *IdentityLifecycleService) BeginTOTPEnrollment(ctx context.Context, organizationID, userID, actorID string, input models.IdentityTOTPEnrollmentInput, metadata models.IdentityRequestMetadata) (*models.IdentityTOTPEnrollment, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || userID != actorID ||
		input.DisplayName != "" && (utf8.RuneCountInString(input.DisplayName) < 2 || utf8.RuneCountInString(input.DisplayName) > 120) ||
		!validIdentityReason(input.Reason) {
		return nil, ErrIdentityInvalid
	}
	account, err := s.store.GetAccount(ctx, organizationID, userID)
	if err != nil || !account.User.IsActive() || account.EmailVerifiedAt == nil {
		return nil, ErrIdentityCredential
	}
	secretBytes := make([]byte, 20)
	if _, err := io.ReadFull(s.random, secretBytes); err != nil {
		return nil, fmt.Errorf("generate TOTP secret: %w", err)
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
	ciphertext, err := s.protector.Seal(organizationID, "totp:"+userID, []byte(secret))
	if err != nil {
		return nil, fmt.Errorf("protect TOTP secret: %w", err)
	}
	factor, err := s.store.CreateTOTPFactor(ctx, organizationID, userID, actorID, input.DisplayName,
		[]byte(ciphertext), input.Reason, sanitizeIdentityMetadata(metadata))
	if err != nil {
		return nil, mapIdentityStoreError(err)
	}
	now := s.now().UTC()
	label := strings.TrimSpace(account.User.Email)
	issuer := "ComplianceForge"
	query := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return &models.IdentityTOTPEnrollment{FactorID: factor.ID, Secret: secret,
		OTPAuthURI: "otpauth://totp/" + url.PathEscape(issuer+":"+label) + "?" + query.Encode(),
		ExpiresAt:  now.Add(s.enrollmentTTL), ExpectedVersion: factor.Version}, nil
}

func (s *IdentityLifecycleService) VerifyTOTPEnrollment(ctx context.Context, organizationID, userID, actorID string, input models.IdentityTOTPVerifyInput, metadata models.IdentityRequestMetadata) (*models.IdentityTOTPVerifyResult, error) {
	input.Code = normalizeIdentityCode(input.Code)
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || actorID != userID || !identityUUID(input.FactorID) ||
		input.ExpectedVersion < 1 || !validIdentityReason(input.Reason) {
		return nil, ErrIdentityInvalid
	}
	factor, err := s.store.GetMFAFactor(ctx, organizationID, userID, input.FactorID)
	if err != nil || factor.Verified || factor.DisabledAt != nil || factor.Method != models.IdentityMethodTOTP ||
		s.now().UTC().After(factor.CreatedAt.Add(s.enrollmentTTL)) {
		return nil, ErrIdentityCredential
	}
	step, valid := s.validateTOTP(factor, input.Code)
	if !valid {
		return nil, ErrIdentityCredential
	}
	codes, hashes, err := s.generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	verified, err := s.store.VerifyTOTPFactor(ctx, organizationID, userID, input.FactorID, input.ExpectedVersion,
		step, hashes, input.Reason, sanitizeIdentityMetadata(metadata))
	if err != nil {
		return nil, mapIdentityCredentialError(err)
	}
	return &models.IdentityTOTPVerifyResult{Factor: *verified, RecoveryCodes: codes}, nil
}

func (s *IdentityLifecycleService) ListMFAFactors(ctx context.Context, organizationID, userID string) ([]models.IdentityMFAFactor, error) {
	if !identityUUID(organizationID) || !identityUUID(userID) {
		return nil, ErrIdentityInvalid
	}
	items, err := s.store.ListMFAFactors(ctx, organizationID, userID)
	for index := range items {
		items[index].SecretCipher = nil
	}
	return items, mapIdentityStoreError(err)
}

func (s *IdentityLifecycleService) DisableMFAFactor(ctx context.Context, organizationID, userID, actorID, factorID, accessToken string, input models.IdentityMFADisableInput, metadata models.IdentityRequestMetadata) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || userID != actorID || !identityUUID(factorID) ||
		input.ExpectedVersion < 1 || !validIdentityReason(input.Reason) {
		return ErrIdentityInvalid
	}
	if err := s.consumeStepUp(ctx, organizationID, userID, accessToken, "mfa_change", input.StepUpToken); err != nil {
		return err
	}
	return mapIdentityStoreError(s.store.DisableMFAFactor(ctx, organizationID, userID, factorID, actorID,
		input.ExpectedVersion, input.Reason, sanitizeIdentityMetadata(metadata)))
}

func (s *IdentityLifecycleService) RegenerateRecoveryCodes(ctx context.Context, organizationID, userID, actorID, factorID, accessToken string, input models.IdentityRecoveryRegenerateInput, metadata models.IdentityRequestMetadata) ([]string, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || userID != actorID || !identityUUID(factorID) || !validIdentityReason(input.Reason) {
		return nil, ErrIdentityInvalid
	}
	if err := s.consumeStepUp(ctx, organizationID, userID, accessToken, "mfa_change", input.StepUpToken); err != nil {
		return nil, err
	}
	codes, hashes, err := s.generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := s.store.ReplaceRecoveryCodes(ctx, organizationID, userID, factorID, hashes, input.Reason,
		sanitizeIdentityMetadata(metadata)); err != nil {
		return nil, mapIdentityStoreError(err)
	}
	return codes, nil
}

func (s *IdentityLifecycleService) BeginLoginMFA(ctx context.Context, organizationID, userID string, metadata models.IdentityRequestMetadata) (*models.IdentityMFAChallengeResponse, error) {
	if !identityUUID(organizationID) || !identityUUID(userID) {
		return nil, ErrIdentityInvalid
	}
	return s.beginMFAChallenge(ctx, organizationID, userID, nil, "login", "", metadata)
}

func (s *IdentityLifecycleService) BeginStepUp(ctx context.Context, organizationID, userID, accessToken string, input models.IdentityStepUpBeginInput, metadata models.IdentityRequestMetadata) (*models.IdentityMFAChallengeResponse, error) {
	input.Purpose = strings.ToLower(strings.TrimSpace(input.Purpose))
	if !identityUUID(organizationID) || !identityUUID(userID) || accessToken == "" || !identityStepUpPurposePattern.MatchString(input.Purpose) {
		return nil, ErrIdentityInvalid
	}
	session, err := s.store.ResolveSession(ctx, organizationID, userID, identityTokenHash(accessToken))
	if err != nil {
		return nil, ErrIdentityCredential
	}
	return s.beginMFAChallenge(ctx, organizationID, userID, &session.ID, "step_up", input.Purpose, metadata)
}

func (s *IdentityLifecycleService) beginMFAChallenge(ctx context.Context, organizationID, userID string, sessionID *string, purpose, stepUpPurpose string, metadata models.IdentityRequestMetadata) (*models.IdentityMFAChallengeResponse, error) {
	account, err := s.store.GetAccount(ctx, organizationID, userID)
	if err != nil || !account.User.IsActive() || account.EmailVerifiedAt == nil {
		return nil, ErrIdentityCredential
	}
	policy, err := s.GetPolicy(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	factors, err := s.store.ListMFAFactors(ctx, organizationID, userID)
	if err != nil {
		return nil, mapIdentityStoreError(err)
	}
	passkeys, err := s.store.ListPasskeys(ctx, organizationID, userID)
	if err != nil {
		return nil, mapIdentityStoreError(err)
	}
	methods := availableIdentityMethods(policy.AllowedMethods, factors, passkeys)
	admin := account.User.IsSuperAdmin || account.User.Role == models.UserRoleAdmin
	policyRequired := policy.RequireMFA || policy.RequireMFAForAdmins && admin
	exempt := account.MFAExemptUntil != nil && s.now().UTC().Before(*account.MFAExemptUntil)
	required := purpose == "step_up" || policyRequired && !exempt || len(methods) > 0
	if !required {
		return nil, nil
	}
	if len(methods) == 0 {
		if purpose == "login" && policy.EnrollmentGraceHours > 0 &&
			s.now().UTC().Before(account.User.CreatedAt.Add(time.Duration(policy.EnrollmentGraceHours)*time.Hour)) {
			return nil, nil
		}
		return nil, ErrIdentityEnrollment
	}
	var passkeyOptions, webauthnSession []byte
	if containsIdentityMethod(methods, "passkey") {
		webauthnUser, loadErr := s.loadWebAuthnUser(ctx, organizationID, userID)
		if loadErr != nil {
			return nil, loadErr
		}
		passkeyOptions, webauthnSession, err = s.webauthn.BeginAuthentication(*webauthnUser)
		if err != nil {
			return nil, ErrIdentityUnavailable
		}
	}
	raw, hash, err := s.newOpaqueCredential(organizationID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	challengeMinutes := policy.AuthenticationChallengeMins
	if challengeMinutes < 1 {
		challengeMinutes = 5
	}
	challenge := &models.IdentityAuthenticationChallenge{OrganizationID: organizationID, UserID: userID,
		SessionID: sessionID, TokenHash: hash, PlainToken: raw, Purpose: purpose, AllowedMethods: methods,
		StepUpPurpose: stepUpPurpose, WebAuthnSession: webauthnSession,
		ExpiresAt: now.Add(time.Duration(challengeMinutes) * time.Minute), MaxAttempts: 5}
	if err := s.store.CreateChallenge(ctx, challenge); err != nil {
		return nil, mapIdentityStoreError(err)
	}
	_ = metadata // Metadata is captured on proof/audit; challenge rows contain no client secrets.
	return &models.IdentityMFAChallengeResponse{ChallengeToken: raw, Methods: methods,
		PasskeyOptions: passkeyOptions, ExpiresAt: challenge.ExpiresAt}, nil
}

func (s *IdentityLifecycleService) VerifyLoginMFA(ctx context.Context, input models.IdentityMFAProofInput, metadata models.IdentityRequestMetadata) (*models.IdentityAuthenticationResult, error) {
	result, _, err := s.verifyMFAChallenge(ctx, input, "login", metadata)
	return result, err
}

func (s *IdentityLifecycleService) VerifyStepUp(ctx context.Context, organizationID, userID, accessToken string, input models.IdentityMFAProofInput, metadata models.IdentityRequestMetadata) (*models.IdentityStepUpGrant, error) {
	challengeOrg, challengeHash, parseErr := parseOpaqueIdentityCredential(input.ChallengeToken)
	if parseErr != nil || challengeOrg != organizationID || !identityUUID(userID) || accessToken == "" {
		return nil, ErrIdentityCredential
	}
	preflight, preflightErr := s.store.GetChallenge(ctx, organizationID, challengeHash, "step_up")
	if preflightErr != nil || preflight.UserID != userID || preflight.SessionID == nil {
		return nil, ErrIdentityCredential
	}
	currentSession, sessionErr := s.store.ResolveSession(ctx, organizationID, userID, identityTokenHash(accessToken))
	if sessionErr != nil || currentSession.ID != *preflight.SessionID {
		return nil, ErrIdentityCredential
	}
	result, challenge, err := s.verifyMFAChallenge(ctx, input, "step_up", metadata)
	if err != nil {
		return nil, err
	}
	if challenge.SessionID == nil {
		return nil, ErrIdentityCredential
	}
	policy, err := s.GetPolicy(ctx, result.OrganizationID)
	if err != nil {
		return nil, err
	}
	raw, hash, err := s.newOpaqueCredential(result.OrganizationID)
	if err != nil {
		return nil, err
	}
	ttl := policy.StepUpTTLMinutes
	if ttl < 1 {
		ttl = 10
	}
	grant := &models.IdentityStepUpGrant{OrganizationID: result.OrganizationID, UserID: result.UserID,
		SessionID: *challenge.SessionID, Purpose: challenge.StepUpPurpose, AuthenticationMethod: result.AuthenticationMethod,
		ExpiresAt: s.now().UTC().Add(time.Duration(ttl) * time.Minute), PlainToken: raw}
	if err := s.store.CreateStepUpGrant(ctx, grant, hash); err != nil {
		return nil, mapIdentityStoreError(err)
	}
	return grant, nil
}

func (s *IdentityLifecycleService) verifyMFAChallenge(ctx context.Context, input models.IdentityMFAProofInput, purpose string, metadata models.IdentityRequestMetadata) (*models.IdentityAuthenticationResult, *models.IdentityAuthenticationChallenge, error) {
	organizationID, tokenHash, err := parseOpaqueIdentityCredential(input.ChallengeToken)
	input.Method = strings.ToLower(strings.TrimSpace(input.Method))
	input.Code = normalizeIdentityCode(input.Code)
	if err != nil || input.Method != "totp" && input.Method != "recovery_code" && input.Method != "passkey" {
		return nil, nil, ErrIdentityCredential
	}
	if err := s.rateLimit(ctx, "mfa_proof", tokenHash+":"+metadata.IPAddress, 10); err != nil {
		return nil, nil, err
	}
	challenge, err := s.store.GetChallenge(ctx, organizationID, tokenHash, purpose)
	if err != nil || challenge.ConsumedAt != nil || !s.now().UTC().Before(challenge.ExpiresAt) ||
		challenge.FailedAttempts >= challenge.MaxAttempts || !containsIdentityMethod(challenge.AllowedMethods, input.Method) {
		return nil, nil, ErrIdentityCredential
	}
	verified, challengeConsumed := false, false
	if input.Method == "totp" {
		factors, listErr := s.store.ListMFAFactors(ctx, organizationID, challenge.UserID)
		if listErr != nil {
			return nil, nil, mapIdentityStoreError(listErr)
		}
		for index := range factors {
			factor := &factors[index]
			if factor.Method != models.IdentityMethodTOTP || !factor.Verified || factor.DisabledAt != nil {
				continue
			}
			step, valid := s.validateTOTP(factor, input.Code)
			if !valid {
				continue
			}
			if useErr := s.store.UseTOTPFactor(ctx, organizationID, challenge.UserID, factor.ID, step, s.now().UTC()); useErr == nil {
				verified = true
				break
			}
		}
	} else if input.Method == "recovery_code" {
		hashes, listErr := s.store.ListRecoveryCodeHashes(ctx, organizationID, challenge.UserID)
		if listErr != nil {
			return nil, nil, mapIdentityStoreError(listErr)
		}
		for id, hash := range hashes {
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Code)) != nil {
				continue
			}
			if consumeErr := s.store.ConsumeRecoveryCode(ctx, organizationID, challenge.UserID, id, hash, s.now().UTC()); consumeErr == nil {
				verified = true
				break
			}
		}
	} else {
		if len(input.Credential) == 0 || len(input.Credential) > 128*1024 || len(challenge.WebAuthnSession) == 0 {
			_ = s.store.FailChallenge(ctx, organizationID, tokenHash, purpose, s.now().UTC())
			return nil, nil, ErrIdentityCredential
		}
		user, loadErr := s.loadWebAuthnUser(ctx, organizationID, challenge.UserID)
		if loadErr != nil {
			return nil, nil, ErrIdentityCredential
		}
		credential, verifyErr := s.webauthn.FinishAuthentication(*user, challenge.WebAuthnSession, input.Credential)
		if verifyErr == nil && credential != nil && credential.Flags.UserPresent && credential.Flags.UserVerified {
			passkey, getErr := s.store.GetPasskeyByCredentialID(ctx, organizationID, challenge.UserID, credential.ID)
			if getErr == nil {
				ciphertext, sealErr := s.sealWebAuthnCredential(organizationID, passkey.ID, credential)
				if sealErr == nil {
					updateErr := s.store.UpdatePasskeyAuthentication(ctx, organizationID, challenge.UserID, passkey.ID,
						ciphertext, credential.Authenticator.SignCount, credential.Authenticator.CloneWarning,
						credential.Flags.BackupState, tokenHash, purpose, s.now().UTC(), sanitizeIdentityMetadata(metadata))
					verified = updateErr == nil && !credential.Authenticator.CloneWarning
					challengeConsumed = updateErr == nil
				}
			}
		}
	}
	if !verified {
		_ = s.store.FailChallenge(ctx, organizationID, tokenHash, purpose, s.now().UTC())
		return nil, nil, ErrIdentityCredential
	}
	if !challengeConsumed {
		if err := s.store.ConsumeChallenge(ctx, organizationID, tokenHash, purpose, s.now().UTC()); err != nil {
			return nil, nil, ErrIdentityCredential
		}
	}
	result := &models.IdentityAuthenticationResult{OrganizationID: organizationID, UserID: challenge.UserID,
		AuthenticationMethod: models.IdentityMethod(input.Method), AuthenticatedAt: s.now().UTC()}
	return result, challenge, nil
}

func (s *IdentityLifecycleService) AdminResetMFA(ctx context.Context, organizationID, targetUserID, actorID, accessToken string, input models.IdentityAdminMFAResetInput, metadata models.IdentityRequestMetadata) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ExemptionHours == 0 {
		input.ExemptionHours = 2
	}
	if !identityUUID(organizationID) || !identityUUID(targetUserID) || !identityUUID(actorID) || actorID == targetUserID || !validIdentityReason(input.Reason) {
		return ErrIdentityInvalid
	}
	if input.ExemptionHours < 1 || input.ExemptionHours > 24 {
		return ErrIdentityInvalid
	}
	if err := s.consumeStepUp(ctx, organizationID, actorID, accessToken, "admin_mfa_reset:"+targetUserID, input.StepUpToken); err != nil {
		return err
	}
	return mapIdentityStoreError(s.store.AdminResetMFA(ctx, organizationID, targetUserID, actorID, input.Reason,
		s.now().UTC().Add(time.Duration(input.ExemptionHours)*time.Hour),
		sanitizeIdentityMetadata(metadata)))
}

func (s *IdentityLifecycleService) consumeStepUp(ctx context.Context, organizationID, userID, accessToken, purpose, grantToken string) error {
	grantOrg, grantHash, err := parseOpaqueIdentityCredential(grantToken)
	if err != nil || grantOrg != organizationID || accessToken == "" {
		return ErrIdentityCredential
	}
	session, err := s.store.ResolveSession(ctx, organizationID, userID, identityTokenHash(accessToken))
	if err != nil {
		return ErrIdentityCredential
	}
	if err := s.store.ConsumeStepUpGrant(ctx, organizationID, userID, session.ID, purpose, grantHash, s.now().UTC()); err != nil {
		return ErrIdentityCredential
	}
	return nil
}

func (s *IdentityLifecycleService) validateTOTP(factor *models.IdentityMFAFactor, code string) (int64, bool) {
	if factor == nil || len(code) != 6 {
		return 0, false
	}
	plaintext, err := s.protector.Open(factor.OrganizationID, "totp:"+factor.UserID, string(factor.SecretCipher))
	if err != nil {
		return 0, false
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(string(plaintext)))
	if err != nil || len(secret) < 16 {
		return 0, false
	}
	seconds := s.now().UTC().Unix()
	if seconds < 0 {
		return 0, false
	}
	current := seconds / identityTOTPStepSeconds
	for offset := int64(-1); offset <= 1; offset++ {
		step := current + offset
		if step < 0 || factor.LastTOTPStep != nil && step <= *factor.LastTOTPStep {
			continue
		}
		expected := identityTOTPCode(secret, step)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func identityTOTPCode(secret []byte, step int64) string {
	// HOTP's moving factor is an unsigned 64-bit value. Reject signed input
	// outside that domain before encoding; do not wrap a negative time step.
	if step < 0 {
		return ""
	}
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(step))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(message)
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0x0f
	value := (uint32(digest[offset])&0x7f)<<24 | uint32(digest[offset+1])<<16 |
		uint32(digest[offset+2])<<8 | uint32(digest[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}

func (s *IdentityLifecycleService) generateRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, s.recoveryCount)
	hashes := make([]string, s.recoveryCount)
	for index := range codes {
		random := make([]byte, 10)
		if _, err := io.ReadFull(s.random, random); err != nil {
			return nil, nil, fmt.Errorf("generate MFA recovery code: %w", err)
		}
		encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random)
		codes[index] = encoded[:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:]
		hash, err := bcrypt.GenerateFromPassword([]byte(normalizeIdentityCode(codes[index])), s.bcryptCost)
		if err != nil {
			return nil, nil, fmt.Errorf("hash MFA recovery code: %w", err)
		}
		hashes[index] = string(hash)
	}
	return codes, hashes, nil
}

func availableIdentityMethods(allowed []string, factors []models.IdentityMFAFactor, passkeys []models.IdentityPasskey) []string {
	allow := map[string]bool{}
	for _, method := range allowed {
		allow[method] = true
	}
	hasTOTP, hasRecovery := false, false
	for _, factor := range factors {
		if factor.Method == models.IdentityMethodTOTP && factor.Verified && factor.DisabledAt == nil {
			hasTOTP = true
			hasRecovery = hasRecovery || factor.RecoveryCodes > 0
		}
	}
	methods := []string{}
	if allow["totp"] && hasTOTP {
		methods = append(methods, "totp")
		if hasRecovery {
			methods = append(methods, "recovery_code")
		}
	}
	if allow["passkey"] && len(passkeys) > 0 {
		methods = append(methods, "passkey")
	}
	return methods
}

func containsIdentityMethod(methods []string, expected string) bool {
	for _, method := range methods {
		if method == expected {
			return true
		}
	}
	return false
}

func normalizeIdentityCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	return strings.ReplaceAll(value, "-", "")
}
