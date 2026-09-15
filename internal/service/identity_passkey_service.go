package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
)

func (s *IdentityLifecycleService) BeginPasskeyRegistration(ctx context.Context, organizationID, userID, accessToken string, input models.IdentityPasskeyRegistrationBeginInput, metadata models.IdentityRequestMetadata) (*models.IdentityPasskeyCeremony, error) {
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	if !identityUUID(organizationID) || !identityUUID(userID) || accessToken == "" ||
		utf8.RuneCountInString(input.DeviceName) < 2 || utf8.RuneCountInString(input.DeviceName) > 120 {
		return nil, ErrIdentityInvalid
	}
	session, err := s.store.ResolveSession(ctx, organizationID, userID, identityTokenHash(accessToken))
	if err != nil {
		return nil, ErrIdentityCredential
	}
	user, err := s.loadWebAuthnUser(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	options, sessionState, err := s.webauthn.BeginRegistration(*user)
	if err != nil {
		return nil, ErrIdentityCredential
	}
	return s.persistPasskeyChallenge(ctx, organizationID, userID, &session.ID, "passkey_registration", "", options, sessionState, metadata)
}

func (s *IdentityLifecycleService) FinishPasskeyRegistration(ctx context.Context, organizationID, userID, actorID, accessToken string, input models.IdentityPasskeyRegistrationFinishInput, metadata models.IdentityRequestMetadata) (*models.IdentityPasskey, error) {
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	challengeOrg, challengeHash, err := parseOpaqueIdentityCredential(input.ChallengeToken)
	if err != nil || challengeOrg != organizationID || !identityUUID(userID) || actorID != userID || accessToken == "" ||
		utf8.RuneCountInString(input.DeviceName) < 2 || utf8.RuneCountInString(input.DeviceName) > 120 ||
		len(input.Credential) == 0 || len(input.Credential) > 128*1024 {
		return nil, ErrIdentityCredential
	}
	currentSession, err := s.store.ResolveSession(ctx, organizationID, userID, identityTokenHash(accessToken))
	if err != nil {
		return nil, ErrIdentityCredential
	}
	challenge, err := s.store.GetChallenge(ctx, organizationID, challengeHash, "passkey_registration")
	if err != nil || challenge.UserID != userID || challenge.SessionID == nil || *challenge.SessionID != currentSession.ID ||
		challenge.ConsumedAt != nil || !s.now().UTC().Before(challenge.ExpiresAt) {
		return nil, ErrIdentityCredential
	}
	user, err := s.loadWebAuthnUser(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	credential, err := s.webauthn.FinishRegistration(*user, challenge.WebAuthnSession, input.Credential)
	if err != nil || credential == nil || !credential.Flags.UserPresent || !credential.Flags.UserVerified || len(credential.ID) == 0 {
		_ = s.store.FailChallenge(ctx, organizationID, challengeHash, "passkey_registration", s.now().UTC())
		return nil, ErrIdentityCredential
	}
	passkeyID := uuid.NewString()
	ciphertext, err := s.sealWebAuthnCredential(organizationID, passkeyID, credential)
	if err != nil {
		return nil, err
	}
	passkey := passkeyFromWebAuthnCredential(passkeyID, organizationID, userID, input.DeviceName, credential, ciphertext)
	result, err := s.store.CreatePasskey(ctx, organizationID, userID, actorID, passkey, challengeHash,
		s.now().UTC(), "Passkey registered by account owner", sanitizeIdentityMetadata(metadata))
	if err != nil {
		return nil, mapIdentityCredentialError(err)
	}
	return result, nil
}

func (s *IdentityLifecycleService) BeginPasskeyAuthentication(ctx context.Context, input models.IdentityPasskeyAuthenticationBeginInput, metadata models.IdentityRequestMetadata) (*models.IdentityPasskeyCeremony, error) {
	input.OrganizationID = strings.TrimSpace(input.OrganizationID)
	input.Email = normalizeIdentityServiceEmail(input.Email)
	input.Purpose = strings.ToLower(strings.TrimSpace(input.Purpose))
	if input.Purpose == "" {
		input.Purpose = "login"
	}
	if input.Purpose != "login" || !identityUUID(input.OrganizationID) || !validIdentityEmail(input.Email) {
		return s.dummyPasskeyCeremony(input.OrganizationID)
	}
	if err := s.rateLimit(ctx, "passkey_begin", input.OrganizationID+":"+input.Email+":"+metadata.IPAddress, 10); err != nil {
		return nil, err
	}
	account, err := s.store.GetAccountByEmail(ctx, input.OrganizationID, input.Email)
	if err != nil || !account.User.IsActive() || account.EmailVerifiedAt == nil {
		return s.dummyPasskeyCeremony(input.OrganizationID)
	}
	user, err := s.loadWebAuthnUser(ctx, input.OrganizationID, account.User.ID)
	if err != nil || len(user.Credentials) == 0 {
		return s.dummyPasskeyCeremony(input.OrganizationID)
	}
	options, sessionState, err := s.webauthn.BeginAuthentication(*user)
	if err != nil {
		return nil, ErrIdentityUnavailable
	}
	return s.persistPasskeyChallenge(ctx, input.OrganizationID, account.User.ID, nil,
		"passkey_authentication", "login", options, sessionState, metadata)
}

func (s *IdentityLifecycleService) FinishPasskeyAuthentication(ctx context.Context, input models.IdentityPasskeyAuthenticationFinishInput, metadata models.IdentityRequestMetadata) (*models.IdentityAuthenticationResult, error) {
	organizationID, challengeHash, err := parseOpaqueIdentityCredential(input.ChallengeToken)
	if err != nil || len(input.Credential) == 0 || len(input.Credential) > 128*1024 {
		return nil, ErrIdentityCredential
	}
	if err := s.rateLimit(ctx, "passkey_finish", challengeHash+":"+metadata.IPAddress, 10); err != nil {
		return nil, err
	}
	challenge, err := s.store.GetChallenge(ctx, organizationID, challengeHash, "passkey_authentication")
	if err != nil || challenge.ConsumedAt != nil || !s.now().UTC().Before(challenge.ExpiresAt) {
		return nil, ErrIdentityCredential
	}
	user, err := s.loadWebAuthnUser(ctx, organizationID, challenge.UserID)
	if err != nil {
		return nil, ErrIdentityCredential
	}
	credential, err := s.webauthn.FinishAuthentication(*user, challenge.WebAuthnSession, input.Credential)
	if err != nil || credential == nil || !credential.Flags.UserPresent || !credential.Flags.UserVerified {
		_ = s.store.FailChallenge(ctx, organizationID, challengeHash, "passkey_authentication", s.now().UTC())
		return nil, ErrIdentityCredential
	}
	passkey, err := s.store.GetPasskeyByCredentialID(ctx, organizationID, challenge.UserID, credential.ID)
	if err != nil {
		return nil, ErrIdentityCredential
	}
	ciphertext, err := s.sealWebAuthnCredential(organizationID, passkey.ID, credential)
	if err != nil {
		return nil, err
	}
	if credential.Authenticator.CloneWarning {
		_ = s.store.UpdatePasskeyAuthentication(ctx, organizationID, challenge.UserID, passkey.ID, ciphertext,
			credential.Authenticator.SignCount, true, credential.Flags.BackupState, challengeHash,
			"passkey_authentication", s.now().UTC(), sanitizeIdentityMetadata(metadata))
		return nil, ErrIdentityCredential
	}
	if err := s.store.UpdatePasskeyAuthentication(ctx, organizationID, challenge.UserID, passkey.ID, ciphertext,
		credential.Authenticator.SignCount, false, credential.Flags.BackupState, challengeHash,
		"passkey_authentication", s.now().UTC(), sanitizeIdentityMetadata(metadata)); err != nil {
		return nil, mapIdentityCredentialError(err)
	}
	return &models.IdentityAuthenticationResult{OrganizationID: organizationID, UserID: challenge.UserID,
		AuthenticationMethod: models.IdentityMethodPasskey, AuthenticatedAt: s.now().UTC()}, nil
}

func (s *IdentityLifecycleService) ListPasskeys(ctx context.Context, organizationID, userID string) ([]models.IdentityPasskey, error) {
	if !identityUUID(organizationID) || !identityUUID(userID) {
		return nil, ErrIdentityInvalid
	}
	items, err := s.store.ListPasskeys(ctx, organizationID, userID)
	for index := range items {
		items[index].CredentialCipher = ""
		items[index].CredentialID = nil
	}
	return items, mapIdentityStoreError(err)
}

func (s *IdentityLifecycleService) RemovePasskey(ctx context.Context, organizationID, userID, actorID, passkeyID, accessToken string, input models.IdentityPasskeyRemoveInput, metadata models.IdentityRequestMetadata) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || actorID != userID || !identityUUID(passkeyID) ||
		input.ExpectedVersion < 1 || !validIdentityReason(input.Reason) {
		return ErrIdentityInvalid
	}
	if err := s.consumeStepUp(ctx, organizationID, userID, accessToken, "mfa_change", input.StepUpToken); err != nil {
		return err
	}
	return mapIdentityStoreError(s.store.RemovePasskey(ctx, organizationID, userID, passkeyID, actorID,
		input.ExpectedVersion, input.Reason, sanitizeIdentityMetadata(metadata)))
}

func (s *IdentityLifecycleService) persistPasskeyChallenge(ctx context.Context, organizationID, userID string, sessionID *string, purpose, stepUpPurpose string, options, sessionState json.RawMessage, _ models.IdentityRequestMetadata) (*models.IdentityPasskeyCeremony, error) {
	raw, hash, err := s.newOpaqueCredential(organizationID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	challenge := &models.IdentityAuthenticationChallenge{OrganizationID: organizationID, UserID: userID,
		SessionID: sessionID, TokenHash: hash, PlainToken: raw, Purpose: purpose, AllowedMethods: []string{"passkey"},
		StepUpPurpose: stepUpPurpose, WebAuthnSession: sessionState, ExpiresAt: now.Add(5 * time.Minute), MaxAttempts: 5}
	if err := s.store.CreateChallenge(ctx, challenge); err != nil {
		return nil, mapIdentityStoreError(err)
	}
	return &models.IdentityPasskeyCeremony{ChallengeToken: raw, Options: options, ExpiresAt: challenge.ExpiresAt}, nil
}

func (s *IdentityLifecycleService) dummyPasskeyCeremony(organizationID string) (*models.IdentityPasskeyCeremony, error) {
	if !identityUUID(organizationID) {
		organizationID = uuid.Nil.String()
	}
	options, err := s.webauthn.BeginDummyAuthentication()
	if err != nil {
		return nil, ErrIdentityUnavailable
	}
	raw, _, err := s.newOpaqueCredential(organizationID)
	if err != nil {
		return nil, err
	}
	return &models.IdentityPasskeyCeremony{ChallengeToken: raw, Options: options,
		ExpiresAt: s.now().UTC().Add(5 * time.Minute)}, nil
}

func (s *IdentityLifecycleService) loadWebAuthnUser(ctx context.Context, organizationID, userID string) (*IdentityWebAuthnUser, error) {
	account, err := s.store.GetAccount(ctx, organizationID, userID)
	if err != nil || !account.User.IsActive() || account.EmailVerifiedAt == nil {
		return nil, ErrIdentityCredential
	}
	passkeys, err := s.store.ListPasskeys(ctx, organizationID, userID)
	if err != nil {
		return nil, mapIdentityStoreError(err)
	}
	credentials := make([]webauthn.Credential, 0, len(passkeys))
	for _, passkey := range passkeys {
		plaintext, err := s.protector.Open(organizationID, "passkey:"+passkey.ID, passkey.CredentialCipher)
		if err != nil {
			return nil, ErrIdentityUnavailable
		}
		var credential webauthn.Credential
		if err := json.Unmarshal(plaintext, &credential); err != nil {
			return nil, ErrIdentityUnavailable
		}
		if !equalIdentityBytes(credential.ID, passkey.CredentialID) {
			return nil, ErrIdentityUnavailable
		}
		credentials = append(credentials, credential)
	}
	parsedID, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrIdentityInvalid
	}
	displayName := strings.TrimSpace(account.User.FirstName + " " + account.User.LastName)
	if displayName == "" {
		displayName = account.User.Email
	}
	return &IdentityWebAuthnUser{ID: parsedID[:], Name: account.User.Email,
		DisplayName: displayName, Credentials: credentials}, nil
}

func (s *IdentityLifecycleService) sealWebAuthnCredential(organizationID, passkeyID string, credential *webauthn.Credential) (string, error) {
	encoded, err := json.Marshal(credential)
	if err != nil {
		return "", fmt.Errorf("encode passkey credential: %w", err)
	}
	sealed, err := s.protector.Seal(organizationID, "passkey:"+passkeyID, encoded)
	if err != nil {
		return "", fmt.Errorf("protect passkey credential: %w", err)
	}
	return sealed, nil
}

func passkeyFromWebAuthnCredential(id, organizationID, userID, name string, credential *webauthn.Credential, ciphertext string) *models.IdentityPasskey {
	transports := make([]string, len(credential.Transport))
	for index, transport := range credential.Transport {
		transports[index] = string(transport)
	}
	var aaguid *string
	if parsed, err := uuid.FromBytes(credential.Authenticator.AAGUID); err == nil && parsed != uuid.Nil {
		value := parsed.String()
		aaguid = &value
	}
	return &models.IdentityPasskey{ID: id, OrganizationID: organizationID, UserID: userID,
		CredentialID: credential.ID, CredentialCipher: ciphertext, AttestationType: credential.AttestationType,
		Transports: transports, SignCount: credential.Authenticator.SignCount,
		CloneWarning: credential.Authenticator.CloneWarning, BackupEligible: credential.Flags.BackupEligible,
		BackupState: credential.Flags.BackupState, DeviceName: name, AAGUID: aaguid}
}

func equalIdentityBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for index := range left {
		different |= left[index] ^ right[index]
	}
	return different == 0
}
