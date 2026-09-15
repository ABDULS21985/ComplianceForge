package service

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	identityTestOrg  = "81000000-0000-0000-0000-000000000001"
	identityTestUser = "82000000-0000-0000-0000-000000000001"
)

type identityServiceStoreStub struct {
	repository.IdentityLifecycleRepository
	account            *models.IdentityAccountState
	invitation         models.IdentityInvitation
	passwordToken      models.IdentityCredentialToken
	factor             *models.IdentityMFAFactor
	recoveryHashes     []string
	verifiedStep       int64
	passwordIssueCalls int
}

func (s *identityServiceStoreStub) GetAccount(context.Context, string, string) (*models.IdentityAccountState, error) {
	if s.account == nil {
		return nil, repository.ErrIdentityNotFound
	}
	return s.account, nil
}

func (s *identityServiceStoreStub) GetPolicy(context.Context, string) (*models.IdentityPolicy, error) {
	return nil, repository.ErrIdentityNotFound
}

func (s *identityServiceStoreStub) IssueInvitation(_ context.Context, invitation models.IdentityInvitation, _ models.IdentityRequestMetadata, _ string) (*models.IdentityInvitation, error) {
	s.invitation = invitation
	copy := invitation
	copy.DeliveryQueued = true
	return &copy, nil
}

func (s *identityServiceStoreStub) IssuePasswordReset(_ context.Context, token models.IdentityCredentialToken) (bool, error) {
	s.passwordIssueCalls++
	s.passwordToken = token
	return true, nil
}

func (s *identityServiceStoreStub) CreateTOTPFactor(_ context.Context, organizationID, userID, _ string, displayName string, cipher []byte, _ string, _ models.IdentityRequestMetadata) (*models.IdentityMFAFactor, error) {
	s.factor = &models.IdentityMFAFactor{ID: "83000000-0000-0000-0000-000000000001", OrganizationID: organizationID,
		UserID: userID, Method: models.IdentityMethodTOTP, DisplayName: displayName, SecretCipher: cipher,
		Version: 1, CreatedAt: s.account.User.CreatedAt}
	return s.factor, nil
}

func (s *identityServiceStoreStub) GetMFAFactor(context.Context, string, string, string) (*models.IdentityMFAFactor, error) {
	return s.factor, nil
}

func (s *identityServiceStoreStub) VerifyTOTPFactor(_ context.Context, _, _, _ string, _ int64, step int64, hashes []string, _ string, _ models.IdentityRequestMetadata) (*models.IdentityMFAFactor, error) {
	s.verifiedStep = step
	s.recoveryHashes = append([]string(nil), hashes...)
	copy := *s.factor
	copy.Verified = true
	copy.Version++
	copy.RecoveryCodes = len(hashes)
	s.factor = &copy
	return &copy, nil
}

type identityLimiterStub struct {
	allowed bool
	err     error
}

func (l identityLimiterStub) Allow(context.Context, string, int) (bool, time.Duration, error) {
	return l.allowed, time.Minute, l.err
}

type identityWebAuthnStub struct{}

func (identityWebAuthnStub) BeginRegistration(IdentityWebAuthnUser) (json.RawMessage, json.RawMessage, error) {
	return json.RawMessage(`{"publicKey":{"challenge":"registration"}}`), json.RawMessage(`{"challenge":"registration"}`), nil
}
func (identityWebAuthnStub) FinishRegistration(IdentityWebAuthnUser, json.RawMessage, json.RawMessage) (*webauthn.Credential, error) {
	return nil, errors.New("not configured")
}
func (identityWebAuthnStub) BeginAuthentication(IdentityWebAuthnUser) (json.RawMessage, json.RawMessage, error) {
	return json.RawMessage(`{"publicKey":{"challenge":"authentication"}}`), json.RawMessage(`{"challenge":"authentication"}`), nil
}
func (identityWebAuthnStub) BeginDummyAuthentication() (json.RawMessage, error) {
	return json.RawMessage(`{"publicKey":{"challenge":"opaque"}}`), nil
}
func (identityWebAuthnStub) FinishAuthentication(IdentityWebAuthnUser, json.RawMessage, json.RawMessage) (*webauthn.Credential, error) {
	return nil, errors.New("not configured")
}

func TestIdentityLifecycleGeneratesProtectedInvitationCredential(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store := &identityServiceStoreStub{}
	service := newIdentityServiceForTest(t, store, now)
	result, err := service.IssueInvitation(context.Background(), identityTestOrg, identityTestUser, identityTestUser,
		models.IdentityInvitationIssueInput{ExpiresInHours: 2, Reason: "Invite account owner"}, models.IdentityRequestMetadata{IPAddress: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.PlainToken == "" || store.invitation.TokenHash == "" || store.invitation.TokenHash == result.PlainToken ||
		store.invitation.DeliveryCipher == "" || result.ExpiresAt != now.Add(2*time.Hour) {
		t.Fatalf("invitation=%#v stored=%#v", result, store.invitation)
	}
	plaintext, err := service.protector.Open(identityTestOrg, "invitation:"+result.ID, store.invitation.DeliveryCipher)
	if err != nil || string(plaintext) != result.PlainToken {
		t.Fatalf("Open() plaintext=%q err=%v", plaintext, err)
	}
	parsedOrg, parsedHash, err := parseOpaqueIdentityCredential(result.PlainToken)
	if err != nil || parsedOrg != identityTestOrg || parsedHash != store.invitation.TokenHash {
		t.Fatalf("parsed org=%q hash=%q err=%v", parsedOrg, parsedHash, err)
	}
}

func TestIdentityLifecycleTOTPEnrollmentAndOneTimeRecoveryMaterial(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	verifiedAt := now.Add(-time.Hour)
	store := &identityServiceStoreStub{account: &models.IdentityAccountState{EmailVerifiedAt: &verifiedAt, User: models.User{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: identityTestUser, CreatedAt: now}, OrganizationID: identityTestOrg},
		Email:       "owner@example.test", Status: models.UserStatusActive,
	}}}
	service := newIdentityServiceForTest(t, store, now)
	enrollment, err := service.BeginTOTPEnrollment(context.Background(), identityTestOrg, identityTestUser, identityTestUser,
		models.IdentityTOTPEnrollmentInput{DisplayName: "Primary authenticator", Reason: "Enable account MFA"}, models.IdentityRequestMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	code := identityTOTPCode(secret, now.Unix()/identityTOTPStepSeconds)
	verified, err := service.VerifyTOTPEnrollment(context.Background(), identityTestOrg, identityTestUser, identityTestUser,
		models.IdentityTOTPVerifyInput{FactorID: enrollment.FactorID, ExpectedVersion: enrollment.ExpectedVersion,
			Code: code, Reason: "Verify account authenticator"}, models.IdentityRequestMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Factor.Verified || len(verified.RecoveryCodes) != 10 || len(store.recoveryHashes) != 10 || store.verifiedStep != now.Unix()/30 {
		t.Fatalf("verified=%#v hashes=%d step=%d", verified, len(store.recoveryHashes), store.verifiedStep)
	}
	seen := map[string]bool{}
	for index, code := range verified.RecoveryCodes {
		normalized := normalizeIdentityCode(code)
		if seen[normalized] || bcrypt.CompareHashAndPassword([]byte(store.recoveryHashes[index]), []byte(normalized)) != nil {
			t.Fatalf("recovery code %d was duplicate or did not match its hash", index)
		}
		seen[normalized] = true
	}
	if strings.Contains(string(store.factor.SecretCipher), enrollment.Secret) {
		t.Fatal("TOTP secret was persisted without encryption")
	}
}

func TestIdentityLifecyclePasswordResetRequestIsEnumerationSafe(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store := &identityServiceStoreStub{}
	service := newIdentityServiceForTest(t, store, now)
	if err := service.RequestPasswordReset(context.Background(), models.IdentityEmailRequest{
		OrganizationID: "invalid", Email: "not-an-email"}, models.IdentityRequestMetadata{}); err != nil {
		t.Fatalf("malformed public request error=%v", err)
	}
	if store.passwordIssueCalls != 0 {
		t.Fatal("malformed public request reached storage")
	}
	if err := service.RequestPasswordReset(context.Background(), models.IdentityEmailRequest{
		OrganizationID: identityTestOrg, Email: "Owner@Example.Test"}, models.IdentityRequestMetadata{IPAddress: "192.0.2.2"}); err != nil {
		t.Fatal(err)
	}
	if store.passwordIssueCalls != 1 || store.passwordToken.Email != "owner@example.test" ||
		store.passwordToken.TokenHash == "" || store.passwordToken.DeliveryCipher == "" {
		t.Fatalf("password token=%#v calls=%d", store.passwordToken, store.passwordIssueCalls)
	}
	denied, err := NewIdentityLifecycleService(store, service.protector, identityWebAuthnStub{},
		identityLimiterStub{allowed: false}, zerolog.Nop(), WithIdentityClock(func() time.Time { return now }),
		WithIdentityRandom(strings.NewReader(strings.Repeat("x", 1024))), WithIdentityBcryptCost(bcrypt.MinCost))
	if err != nil {
		t.Fatal(err)
	}
	if err := denied.RequestPasswordReset(context.Background(), models.IdentityEmailRequest{
		OrganizationID: identityTestOrg, Email: "owner@example.test"}, models.IdentityRequestMetadata{}); !errors.Is(err, ErrIdentityRateLimited) {
		t.Fatalf("rate limit error=%v", err)
	}
}

func TestIdentityLifecycleRejectsUnknownPolicyMethod(t *testing.T) {
	store := &identityServiceStoreStub{}
	service := newIdentityServiceForTest(t, store, time.Now().UTC())
	_, err := service.UpdatePolicy(context.Background(), identityTestOrg, identityTestUser,
		models.IdentityPolicyPatch{ExpectedVersion: 0, AllowedMethods: []string{"totp", "sms"}, Reason: "Update authentication policy"})
	if !errors.Is(err, ErrIdentityInvalid) {
		t.Fatalf("UpdatePolicy() error=%v", err)
	}
}

func newIdentityServiceForTest(t *testing.T, store IdentityLifecycleStore, now time.Time) *IdentityLifecycleService {
	t.Helper()
	protector, err := secretbox.NewHex(strings.Repeat("ef", 32))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewIdentityLifecycleService(store, protector, identityWebAuthnStub{}, identityLimiterStub{allowed: true},
		zerolog.Nop(), WithIdentityClock(func() time.Time { return now }),
		WithIdentityRandom(&identityCounterReader{}), WithIdentityBcryptCost(bcrypt.MinCost))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type identityCounterReader struct{ value byte }

func (r *identityCounterReader) Read(destination []byte) (int, error) {
	for index := range destination {
		r.value++
		destination[index] = r.value
	}
	return len(destination), nil
}
