package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	scimTestOrg   = "91000000-0000-0000-0000-000000000001"
	scimTestActor = "91000000-0000-0000-0000-000000000002"
	scimTestToken = "91000000-0000-0000-0000-000000000003"
	scimTestUser  = "91000000-0000-0000-0000-000000000004"
)

type scimStoreStub struct {
	SCIMStore
	createdToken *models.SCIMToken
	authToken    *models.SCIMToken
	currentUser  *models.SCIMUser
	recordErr    error
	replaced     *models.SCIMUser
	eventType    string
}

func (s *scimStoreStub) CreateSCIMToken(_ context.Context, _, _ string, token *models.SCIMToken, _ string) (*models.SCIMToken, error) {
	copy := *token
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	copy.CreatedAt, copy.UpdatedAt = now, now
	s.createdToken = &copy
	return &copy, nil
}

func (s *scimStoreStub) ResolveSCIMTokenTenant(context.Context, string) (string, string, error) {
	return scimTestToken, scimTestOrg, nil
}

func (s *scimStoreStub) GetSCIMTokenForAuthentication(context.Context, string, string, string) (*models.SCIMToken, error) {
	return s.authToken, nil
}

func (s *scimStoreStub) RecordSCIMTokenUse(_ context.Context, _, _, _ string, _ int64, _ string, _ time.Time) error {
	return s.recordErr
}

func (s *scimStoreStub) GetSCIMUser(context.Context, string, string) (*models.SCIMUser, error) {
	copy := *s.currentUser
	return &copy, nil
}

func (s *scimStoreStub) ReplaceSCIMUser(_ context.Context, _, _, _, _ string, _ int64, eventType string, user *models.SCIMUser) (*models.SCIMUser, error) {
	copy := *user
	copy.ID, copy.Version = scimTestUser, 2
	copy.Meta.Created = time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	copy.Meta.LastModified = time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	s.replaced, s.eventType = &copy, eventType
	return &copy, nil
}

func TestSCIMTokenIssueStoresOnlyDigestAndRejectsDuplicateScopes(t *testing.T) {
	store := &scimStoreStub{}
	service, err := NewSCIMService(store, zerolog.Nop(), WithSCIMRandom(strings.NewReader(strings.Repeat("r", 32))))
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.CreateToken(context.Background(), scimTestOrg, scimTestActor, models.SCIMTokenCreateInput{
		Name: "Workforce IdP", Scopes: []string{models.SCIMTokenScopeUsersRead, models.SCIMTokenScopeUsersWrite},
		Reason: "Enable governed workforce provisioning",
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(result.Credential))
	if !strings.HasPrefix(result.Credential, "cfs_") || store.createdToken.TokenHash != hex.EncodeToString(digest[:]) ||
		store.createdToken.TokenHash == result.Credential || result.Token.PlainToken != "" {
		t.Fatalf("unsafe issued credential result=%#v stored=%#v", result, store.createdToken)
	}
	_, err = service.CreateToken(context.Background(), scimTestOrg, scimTestActor, models.SCIMTokenCreateInput{
		Name: "Duplicate scopes", Scopes: []string{models.SCIMTokenScopeUsersRead, models.SCIMTokenScopeUsersRead},
		Reason: "This invalid request must be rejected",
	})
	if !errors.Is(err, ErrSCIMTokenInvalid) {
		t.Fatalf("duplicate scope error=%v", err)
	}
}

func TestSCIMAuthenticationFailsClosedWhenCredentialRotatesAfterVerification(t *testing.T) {
	raw := "cfs_" + strings.Repeat("x", 43)
	digest := sha256.Sum256([]byte(raw))
	store := &scimStoreStub{recordErr: repository.ErrSCIMNotFound, authToken: &models.SCIMToken{
		ID: scimTestToken, OrganizationID: scimTestOrg, TokenHash: hex.EncodeToString(digest[:]), Active: true,
		CreatedBy: scimTestActor, Version: 4, RateLimitPerMinute: 120,
	}}
	service, err := NewSCIMService(store, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSCIMToken(context.Background(), raw, "192.0.2.5"); !errors.Is(err, authdomain.ErrInvalidSCIMToken) {
		t.Fatalf("authentication race error=%v", err)
	}
}

func TestSCIMPathlessPatchIsAppliedAndRecordedAsPatch(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	store := &scimStoreStub{currentUser: &models.SCIMUser{
		Schemas: []string{models.SCIMUserSchema}, ID: scimTestUser, UserName: "old@example.test", Active: true,
		Version: 1, Meta: models.SCIMMeta{Created: now, LastModified: now},
	}}
	service, err := NewSCIMService(store, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.PatchUser(context.Background(), scimTestOrg, scimTestUser, scimTestToken,
		scimTestActor, 1, models.SCIMPatchRequest{Schemas: []string{models.SCIMPatchOperationSchema},
			Operations: []models.SCIMPatchOperation{{Operation: "replace", Value: []byte(`{"active":false,"displayName":"Offboarded User"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Active || updated.DisplayName != "Offboarded User" || store.eventType != "patched" || store.replaced == nil {
		t.Fatalf("updated=%#v event=%q", updated, store.eventType)
	}
}
