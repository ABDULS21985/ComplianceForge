package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const (
	authTestOrgID  = "30000000-0000-0000-0000-000000000001"
	authTestUserID = "40000000-0000-0000-0000-000000000001"
	authTestSecret = "test-secret-with-more-than-thirty-two-bytes"
)

type fakeAuthRepository struct {
	users          map[string]*models.User
	sessions       map[string]*models.UserSession
	lastLogin      *time.Time
	createSessions int
}

type fakeAuthenticationIdentityLifecycle struct {
	challenge           *models.IdentityMFAChallengeResponse
	verificationResult  *models.IdentityAuthenticationResult
	passkeyResult       *models.IdentityAuthenticationResult
	verificationRequest models.IdentityEmailRequest
	verificationCalls   int
	beginMetadata       models.IdentityRequestMetadata
	proofMetadata       models.IdentityRequestMetadata
}

func (f *fakeAuthenticationIdentityLifecycle) BeginLoginMFA(_ context.Context, _, _ string, metadata models.IdentityRequestMetadata) (*models.IdentityMFAChallengeResponse, error) {
	f.beginMetadata = metadata
	return f.challenge, nil
}

func (f *fakeAuthenticationIdentityLifecycle) VerifyLoginMFA(_ context.Context, _ models.IdentityMFAProofInput, metadata models.IdentityRequestMetadata) (*models.IdentityAuthenticationResult, error) {
	f.proofMetadata = metadata
	return f.verificationResult, nil
}

func (f *fakeAuthenticationIdentityLifecycle) FinishPasskeyAuthentication(context.Context, models.IdentityPasskeyAuthenticationFinishInput, models.IdentityRequestMetadata) (*models.IdentityAuthenticationResult, error) {
	return f.passkeyResult, nil
}

func (f *fakeAuthenticationIdentityLifecycle) RequestEmailVerification(_ context.Context, request models.IdentityEmailRequest, _ models.IdentityRequestMetadata) error {
	f.verificationRequest = request
	f.verificationCalls++
	return nil
}

func newFakeAuthRepository() *fakeAuthRepository {
	return &fakeAuthRepository{
		users:    make(map[string]*models.User),
		sessions: make(map[string]*models.UserSession),
	}
}

func (r *fakeAuthRepository) Create(_ context.Context, user *models.User) error {
	r.users[userKey(user.OrganizationID, user.Email)] = user
	return nil
}

func (r *fakeAuthRepository) GetByID(_ context.Context, orgID, id string) (*models.User, error) {
	for _, user := range r.users {
		if user.OrganizationID == orgID && user.ID == id {
			return user, nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (r *fakeAuthRepository) GetByEmail(_ context.Context, orgID, email string) (*models.User, error) {
	if orgID != "" {
		user, ok := r.users[userKey(orgID, email)]
		if !ok {
			return nil, pgx.ErrNoRows
		}
		return user, nil
	}
	var found *models.User
	for _, user := range r.users {
		if user.Email != email {
			continue
		}
		if found != nil {
			return nil, errors.New("ambiguous email")
		}
		found = user
	}
	if found == nil {
		return nil, pgx.ErrNoRows
	}
	return found, nil
}

func (r *fakeAuthRepository) UpdateLastLogin(_ context.Context, _, _ string, loginTime time.Time) error {
	r.lastLogin = &loginTime
	return nil
}

func (r *fakeAuthRepository) CreateSession(_ context.Context, session *models.UserSession) error {
	copyOfSession := *session
	r.sessions[session.TokenHash] = &copyOfSession
	r.createSessions++
	return nil
}

func (r *fakeAuthRepository) RotateSession(
	_ context.Context,
	orgID string,
	userID string,
	currentRefreshHash string,
	next *models.UserSession,
) (bool, error) {
	for accessHash, session := range r.sessions {
		if session.OrganizationID != orgID || session.UserID != userID ||
			session.RefreshTokenHash != currentRefreshHash || session.RevokedAt != nil ||
			time.Now().After(session.ExpiresAt) {
			continue
		}
		delete(r.sessions, accessHash)
		copyOfSession := *next
		r.sessions[next.TokenHash] = &copyOfSession
		return true, nil
	}
	return false, nil
}

func (r *fakeAuthRepository) RevokeSession(_ context.Context, orgID, userID, accessHash string) error {
	if session, ok := r.sessions[accessHash]; ok && session.OrganizationID == orgID && session.UserID == userID {
		now := time.Now()
		session.RevokedAt = &now
	}
	return nil
}

func (r *fakeAuthRepository) IsSessionActive(_ context.Context, orgID, userID, accessHash string) (bool, error) {
	session, ok := r.sessions[accessHash]
	return ok && session.OrganizationID == orgID && session.UserID == userID &&
		session.RevokedAt == nil && time.Now().Before(session.ExpiresAt), nil
}

func TestAuthServiceRegistrationRefreshAndLogoutLifecycle(t *testing.T) {
	repository := newFakeAuthRepository()
	svc := newAuthService(repository)
	ctx := context.Background()

	registered, err := svc.Register(ctx, authdomain.RegisterRequest{
		Email:          "USER@Example.com ",
		Password:       "correct-password",
		FirstName:      "A",
		LastName:       "User",
		OrganizationID: authTestOrgID,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registered.User == nil || registered.User.Role != models.UserRoleViewer {
		t.Fatalf("Register() user role = %v, want %q", registered.User, models.UserRoleViewer)
	}
	if registered.User.Email != "user@example.com" {
		t.Fatalf("Register() email = %q, want normalized email", registered.User.Email)
	}
	if registered.AccessToken == "" || registered.RefreshToken == "" || registered.AccessToken == registered.RefreshToken {
		t.Fatal("Register() did not return a distinct access/refresh pair")
	}
	if repository.createSessions != 1 {
		t.Fatalf("created sessions = %d, want 1", repository.createSessions)
	}

	claims, err := svc.ValidateAccessToken(ctx, registered.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if claims.TokenType != authdomain.TokenTypeAccess || claims.OrganizationID != authTestOrgID {
		t.Fatalf("access claims = %#v", claims)
	}
	if _, err := svc.ValidateAccessToken(ctx, registered.RefreshToken); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("refresh token as access token error = %v, want ErrInvalidToken", err)
	}
	currentUser, err := svc.CurrentUser(ctx, authTestOrgID, registered.User.ID)
	if err != nil || currentUser.ID != registered.User.ID {
		t.Fatalf("CurrentUser() user = %#v, error = %v", currentUser, err)
	}
	if _, err := svc.RefreshToken(ctx, registered.AccessToken); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("RefreshToken(access token) error = %v, want ErrInvalidToken", err)
	}

	refreshed, err := svc.RefreshToken(ctx, registered.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if refreshed.AccessToken == registered.AccessToken || refreshed.RefreshToken == registered.RefreshToken {
		t.Fatal("RefreshToken() did not rotate both tokens")
	}
	if _, err := svc.ValidateAccessToken(ctx, registered.AccessToken); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("old access token error = %v, want ErrInvalidToken", err)
	}
	if _, err := svc.ValidateAccessToken(ctx, refreshed.AccessToken); err != nil {
		t.Fatalf("new access token validation error = %v", err)
	}

	if err := svc.Logout(ctx, authTestOrgID, registered.User.ID, refreshed.AccessToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := svc.ValidateAccessToken(ctx, refreshed.AccessToken); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("logged-out access token error = %v, want ErrInvalidToken", err)
	}
}

func TestAuthServiceLoginReturnsUserAndPersistsSession(t *testing.T) {
	repository := newFakeAuthRepository()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	user := &models.User{
		TenantModel: models.TenantModel{
			BaseModel:      models.BaseModel{ID: authTestUserID},
			OrganizationID: authTestOrgID,
		},
		Email:        "user@example.com",
		PasswordHash: string(passwordHash),
		FirstName:    "A",
		LastName:     "User",
		Status:       models.UserStatusActive,
		Role:         models.UserRoleAdmin,
		Language:     "en",
	}
	repository.users[userKey(user.OrganizationID, user.Email)] = user
	svc := newAuthService(repository)

	result, err := svc.Login(context.Background(), authdomain.LoginRequest{
		Email:          user.Email,
		Password:       "correct-password",
		OrganizationID: authTestOrgID,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.User != user {
		t.Fatal("Login() did not include the authenticated user")
	}
	if repository.createSessions != 1 || repository.lastLogin == nil {
		t.Fatalf("Login() session count = %d, last login = %v", repository.createSessions, repository.lastLogin)
	}

	_, err = svc.Login(context.Background(), authdomain.LoginRequest{
		Email:          user.Email,
		Password:       "incorrect-password",
		OrganizationID: authTestOrgID,
	})
	if !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("Login() bad password error = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthServiceDefersSessionUntilMFAProofAndPreservesDeviceContext(t *testing.T) {
	repository := newFakeAuthRepository()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := &models.User{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: authTestUserID}, OrganizationID: authTestOrgID},
		Email: "mfa@example.com", PasswordHash: string(passwordHash), Status: models.UserStatusActive,
		Role: models.UserRoleAdmin, Language: "en"}
	repository.users[userKey(user.OrganizationID, user.Email)] = user
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	identity := &fakeAuthenticationIdentityLifecycle{
		challenge: &models.IdentityMFAChallengeResponse{ChallengeToken: "opaque", Methods: []string{"totp"}, ExpiresAt: now.Add(5 * time.Minute)},
		verificationResult: &models.IdentityAuthenticationResult{OrganizationID: authTestOrgID, UserID: authTestUserID,
			AuthenticationMethod: models.IdentityMethodTOTP, AuthenticatedAt: now},
	}
	svc := service.NewAuthService(repository, testAuthJWTConfig(), zerolog.Nop(), identity)
	metadata := models.IdentityRequestMetadata{IPAddress: "192.0.2.10", UserAgent: "test-agent", DeviceName: "Work laptop"}
	challenge, err := svc.Login(context.Background(), authdomain.LoginRequest{Email: user.Email, Password: "correct-password",
		OrganizationID: authTestOrgID, Metadata: metadata})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !challenge.MFARequired || challenge.MFAChallenge == nil || challenge.AccessToken != "" || repository.createSessions != 0 {
		t.Fatalf("Login() challenge = %#v, sessions=%d", challenge, repository.createSessions)
	}
	tokens, err := svc.CompleteLoginMFA(context.Background(), models.IdentityMFAProofInput{
		ChallengeToken: "opaque", Method: "totp", Code: "123456",
	}, metadata)
	if err != nil {
		t.Fatalf("CompleteLoginMFA() error = %v", err)
	}
	if tokens.AccessToken == "" || repository.createSessions != 1 {
		t.Fatalf("tokens=%#v sessions=%d", tokens, repository.createSessions)
	}
	var persisted *models.UserSession
	for _, item := range repository.sessions {
		persisted = item
	}
	if persisted == nil || persisted.AuthenticationMethod != models.IdentityMethodTOTP || persisted.MFAVerifiedAt == nil ||
		!persisted.MFAVerifiedAt.Equal(now) || persisted.IPAddress != metadata.IPAddress || persisted.UserAgent != metadata.UserAgent ||
		persisted.DeviceName != metadata.DeviceName {
		t.Fatalf("persisted session = %#v", persisted)
	}
}

func TestAuthServiceRegistrationRequiresEmailVerificationWhenIdentityIsComposed(t *testing.T) {
	repository := newFakeAuthRepository()
	identity := &fakeAuthenticationIdentityLifecycle{}
	svc := service.NewAuthService(repository, testAuthJWTConfig(), zerolog.Nop(), identity)
	result, err := svc.Register(context.Background(), authdomain.RegisterRequest{Email: " New@Example.com ", Password: "correct-password",
		FirstName: "New", LastName: "User", OrganizationID: authTestOrgID})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !result.EmailVerificationRequired || result.AccessToken != "" || result.RefreshToken != "" ||
		result.User == nil || result.User.Status != models.UserStatusPendingVerification || repository.createSessions != 0 {
		t.Fatalf("Register() result=%#v sessions=%d", result, repository.createSessions)
	}
	if identity.verificationCalls != 1 || identity.verificationRequest.OrganizationID != authTestOrgID ||
		identity.verificationRequest.Email != "new@example.com" {
		t.Fatalf("verification request=%#v calls=%d", identity.verificationRequest, identity.verificationCalls)
	}
}

func TestAuthServiceRejectsDuplicateRegistration(t *testing.T) {
	repository := newFakeAuthRepository()
	repository.users[userKey(authTestOrgID, "user@example.com")] = &models.User{
		TenantModel: models.TenantModel{OrganizationID: authTestOrgID},
		Email:       "user@example.com",
	}
	svc := newAuthService(repository)

	_, err := svc.Register(context.Background(), authdomain.RegisterRequest{
		Email:          "user@example.com",
		Password:       "correct-password",
		FirstName:      "A",
		LastName:       "User",
		OrganizationID: authTestOrgID,
	})
	if !errors.Is(err, service.ErrUserAlreadyExists) {
		t.Fatalf("Register() error = %v, want ErrUserAlreadyExists", err)
	}
}

func TestAuthServiceRequiresHS256AndConfiguredIssuer(t *testing.T) {
	svc := newAuthService(newFakeAuthRepository())
	now := time.Now()
	baseClaims := authdomain.Claims{
		UserID:         authTestUserID,
		OrganizationID: authTestOrgID,
		Role:           string(models.UserRoleViewer),
		Email:          "user@example.com",
		TokenType:      authdomain.TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{authdomain.TokenAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			ID:        "50000000-0000-0000-0000-000000000001",
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "complianceforge-test",
			Subject:   authTestUserID,
		},
	}

	wrongAlgorithm, err := jwt.NewWithClaims(jwt.SigningMethodHS384, baseClaims).SignedString([]byte(authTestSecret))
	if err != nil {
		t.Fatalf("signing wrong-algorithm token: %v", err)
	}
	if _, err := svc.ValidateToken(wrongAlgorithm); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("ValidateToken(HS384) error = %v, want ErrInvalidToken", err)
	}

	baseClaims.Issuer = "untrusted-issuer"
	wrongIssuer, err := jwt.NewWithClaims(jwt.SigningMethodHS256, baseClaims).SignedString([]byte(authTestSecret))
	if err != nil {
		t.Fatalf("signing wrong-issuer token: %v", err)
	}
	if _, err := svc.ValidateToken(wrongIssuer); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("ValidateToken(wrong issuer) error = %v, want ErrInvalidToken", err)
	}
}

func newAuthService(repository service.UserRepository) *service.AuthService {
	return service.NewAuthService(repository, testAuthJWTConfig(), zerolog.Nop())
}

func testAuthJWTConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:      authTestSecret,
		Issuer:      "complianceforge-test",
		ExpiryHours: 1,
	}
}

func userKey(orgID, email string) string {
	return orgID + ":" + email
}
