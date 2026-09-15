package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

const (
	identityHandlerOrg  = "91000000-0000-0000-0000-000000000001"
	identityHandlerUser = "92000000-0000-0000-0000-000000000001"
)

type identityHandlerServiceStub struct {
	IdentityLifecycleService
	deliveryRequests []models.IdentityEmailRequest
	invitation       *models.IdentityInvitation
}

func (s *identityHandlerServiceStub) RequestPasswordReset(_ context.Context, request models.IdentityEmailRequest, _ models.IdentityRequestMetadata) error {
	s.deliveryRequests = append(s.deliveryRequests, request)
	return nil
}

func (s *identityHandlerServiceStub) IssueInvitation(context.Context, string, string, string, models.IdentityInvitationIssueInput, models.IdentityRequestMetadata) (*models.IdentityInvitation, error) {
	return s.invitation, nil
}

type identityAuthHandlerStub struct {
	AuthService
	proof    models.IdentityMFAProofInput
	metadata models.IdentityRequestMetadata
}

func (s *identityAuthHandlerStub) CompleteLoginMFA(_ context.Context, proof models.IdentityMFAProofInput, metadata models.IdentityRequestMetadata) (*authdomain.TokenPair, error) {
	s.proof = proof
	s.metadata = metadata
	return &authdomain.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (s *identityAuthHandlerStub) CompletePasskeyAuthentication(context.Context, models.IdentityPasskeyAuthenticationFinishInput, models.IdentityRequestMetadata) (*authdomain.TokenPair, error) {
	return &authdomain.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func TestIdentityPasswordResetRequestHasGenericEnumerationSafeResponse(t *testing.T) {
	service := &identityHandlerServiceStub{}
	handler := NewIdentityLifecycleHandler(service)
	bodies := []string{
		`{"organization_id":"` + identityHandlerOrg + `","email":"known@example.com"}`,
		`{"organization_id":"not-a-uuid","email":"not-an-email"}`,
	}
	var responseBody string
	for _, body := range bodies {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/forgot", strings.NewReader(body))
		response := httptest.NewRecorder()
		handler.RequestPasswordReset(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if responseBody == "" {
			responseBody = response.Body.String()
		} else if response.Body.String() != responseBody {
			t.Fatalf("enumeration responses differ: %q != %q", response.Body.String(), responseBody)
		}
	}
}

func TestIdentityInvitationResponseNeverSerializesCredentialMaterial(t *testing.T) {
	service := &identityHandlerServiceStub{invitation: &models.IdentityInvitation{ID: "93000000-0000-0000-0000-000000000001",
		OrganizationID: identityHandlerOrg, UserID: identityHandlerUser, PlainToken: "plaintext-secret",
		TokenHash: "secret-hash", DeliveryCipher: "encrypted-secret", ExpiresAt: time.Now().Add(time.Hour)}}
	handler := NewIdentityLifecycleHandler(service)
	request := identityHandlerRequest(http.MethodPost, "/api/v1/directory/users/"+identityHandlerUser+"/invitation",
		`{"expires_in_hours":24,"reason":"Invite new colleague"}`, "id", identityHandlerUser)
	response := httptest.NewRecorder()
	handler.IssueInvitation(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	for _, secret := range []string{"plaintext-secret", "secret-hash", "encrypted-secret"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("response leaked %q: %s", secret, response.Body.String())
		}
	}
}

func TestIdentityHandlerRejectsUnknownJSONFields(t *testing.T) {
	handler := NewIdentityLifecycleHandler(&identityHandlerServiceStub{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/forgot", strings.NewReader(
		`{"organization_id":"`+identityHandlerOrg+`","email":"user@example.com","unexpected":true}`))
	response := httptest.NewRecorder()
	handler.RequestPasswordReset(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAuthHandlerMFACompletionIssuesTokensAndCapturesDeviceMetadata(t *testing.T) {
	service := &identityAuthHandlerStub{}
	handler := NewAuthHandler(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", bytes.NewBufferString(
		`{"challenge_token":"opaque","method":"totp","code":"123456"}`))
	request.Header.Set("X-Device-Name", "Company laptop")
	request.Header.Set("User-Agent", "identity-test-agent")
	request.RemoteAddr = "192.0.2.22:1234"
	response := httptest.NewRecorder()
	handler.CompleteMFA(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.proof.Method != "totp" || service.metadata.DeviceName != "Company laptop" ||
		service.metadata.UserAgent != "identity-test-agent" || service.metadata.IPAddress != "192.0.2.22" {
		t.Fatalf("proof=%#v metadata=%#v", service.proof, service.metadata)
	}
	var output map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil || output["access_token"] != "access" {
		t.Fatalf("response=%s error=%v", response.Body.String(), err)
	}
}

func identityHandlerRequest(method, path, body, routeParam, routeValue string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, identityHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, identityHandlerUser)
	route := chi.NewRouteContext()
	route.URLParams.Add(routeParam, routeValue)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	return request.WithContext(ctx)
}
