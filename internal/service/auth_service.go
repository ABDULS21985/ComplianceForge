package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserAlreadyExists  = errors.New("user with this email already exists in the organization")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrUserNotActive      = errors.New("user account is not active")
)

// Backwards-compatible aliases keep the service's public type names stable
// while sharing one contract with handlers and middleware.
type TokenPair = authdomain.TokenPair
type Claims = authdomain.Claims
type LoginRequest = authdomain.LoginRequest
type RegisterRequest = authdomain.RegisterRequest

// UserRepository is the minimum persistence contract required by AuthService.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, orgID, id string) (*models.User, error)
	GetByEmail(ctx context.Context, orgID, email string) (*models.User, error)
	UpdateLastLogin(ctx context.Context, orgID, id string, loginTime time.Time) error
	CreateSession(ctx context.Context, session *models.UserSession) error
	RotateSession(ctx context.Context, orgID, userID, currentRefreshHash string, session *models.UserSession) (bool, error)
	RevokeSession(ctx context.Context, orgID, userID, accessTokenHash string) error
	IsSessionActive(ctx context.Context, orgID, userID, accessTokenHash string) (bool, error)
}

// AuthenticationIdentityLifecycle gates primary authentication with the
// tenant's identity policy and completes one-time MFA/passkey ceremonies. It
// is optional only for backwards-compatible unit construction; production
// composition supplies it whenever identity lifecycle routes are enabled.
type AuthenticationIdentityLifecycle interface {
	BeginLoginMFA(context.Context, string, string, models.IdentityRequestMetadata) (*models.IdentityMFAChallengeResponse, error)
	VerifyLoginMFA(context.Context, models.IdentityMFAProofInput, models.IdentityRequestMetadata) (*models.IdentityAuthenticationResult, error)
	FinishPasskeyAuthentication(context.Context, models.IdentityPasskeyAuthenticationFinishInput, models.IdentityRequestMetadata) (*models.IdentityAuthenticationResult, error)
	RequestEmailVerification(context.Context, models.IdentityEmailRequest, models.IdentityRequestMetadata) error
}

// AuthService handles authentication, registration, and revocable sessions.
type AuthService struct {
	userRepo          UserRepository
	identityLifecycle AuthenticationIdentityLifecycle
	jwtConfig         config.JWTConfig
	logger            zerolog.Logger
}

// NewAuthService constructs a new AuthService.
func NewAuthService(userRepo UserRepository, jwtCfg config.JWTConfig, logger zerolog.Logger, identityLifecycle ...AuthenticationIdentityLifecycle) *AuthService {
	service := &AuthService{
		userRepo:  userRepo,
		jwtConfig: jwtCfg,
		logger:    logger.With().Str("service", "auth").Logger(),
	}
	if len(identityLifecycle) > 0 {
		service.identityLifecycle = identityLifecycle[0]
	}
	return service
}

// Login authenticates a user and persists a revocable token session.
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*TokenPair, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, err := s.userRepo.GetByEmail(ctx, req.OrganizationID, email)
	if err != nil {
		s.logger.Warn().Str("email", email).Msg("login attempt for unknown or ambiguous email")
		return nil, ErrInvalidCredentials
	}

	if !user.IsActive() {
		s.logger.Warn().Str("user_id", user.ID).Msg("login attempt for inactive user")
		return nil, ErrInvalidCredentials
	}
	if user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		s.logger.Warn().Str("email", email).Msg("invalid password attempt")
		return nil, ErrInvalidCredentials
	}
	if s.identityLifecycle != nil {
		challenge, challengeErr := s.identityLifecycle.BeginLoginMFA(ctx, user.OrganizationID, user.ID, req.Metadata)
		if challengeErr != nil {
			return nil, challengeErr
		}
		if challenge != nil {
			return &TokenPair{User: user, MFARequired: true, MFAChallenge: challenge}, nil
		}
	}

	tokens, err := s.issueAuthenticatedSession(ctx, user, models.IdentityMethodPassword, nil, req.Metadata)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to establish login session")
		return nil, err
	}

	s.logger.Info().Str("user_id", user.ID).Str("email", email).Msg("user logged in successfully")
	return tokens, nil
}

// Register creates a viewer account and signs it in. Role is deliberately not
// accepted from this public request.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*TokenPair, error) {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	existing, err := s.userRepo.GetByEmail(ctx, req.OrganizationID, req.Email)
	if err == nil && existing != nil {
		return nil, ErrUserAlreadyExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("checking existing user: %w", err)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to hash password")
		return nil, errors.New("failed to process registration")
	}

	user := &models.User{
		TenantModel: models.TenantModel{
			BaseModel:      models.BaseModel{ID: uuid.NewString()},
			OrganizationID: req.OrganizationID,
		},
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FirstName:    strings.TrimSpace(req.FirstName),
		LastName:     strings.TrimSpace(req.LastName),
		Department:   strings.TrimSpace(req.Department),
		Status:       models.UserStatusActive,
		Role:         models.UserRoleViewer,
		Language:     "en",
	}
	if s.identityLifecycle != nil {
		user.Status = models.UserStatusPendingVerification
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("email", req.Email).Msg("failed to create user")
		return nil, err
	}

	if s.identityLifecycle != nil {
		if err := s.identityLifecycle.RequestEmailVerification(ctx, models.IdentityEmailRequest{
			OrganizationID: user.OrganizationID,
			Email:          user.Email,
		}, req.Metadata); err != nil {
			s.logger.Error().Err(err).Str("user_id", user.ID).Msg("registration created but verification delivery could not be queued")
			return nil, fmt.Errorf("queueing registration verification: %w", err)
		}
		s.logger.Info().Str("user_id", user.ID).Msg("user registered pending email verification")
		return &TokenPair{User: user, EmailVerificationRequired: true}, nil
	}

	tokens, err := s.issueAuthenticatedSession(ctx, user, models.IdentityMethodPassword, nil, req.Metadata)
	if err != nil {
		return nil, err
	}

	s.logger.Info().Str("user_id", user.ID).Str("email", req.Email).Msg("user registered successfully")
	return tokens, nil
}

// CompleteLoginMFA exchanges a consumed, tenant-bound MFA challenge for a
// normal revocable session. No JWT is issued until the lifecycle service has
// atomically accepted the one-time proof.
func (s *AuthService) CompleteLoginMFA(ctx context.Context, input models.IdentityMFAProofInput, metadata models.IdentityRequestMetadata) (*TokenPair, error) {
	if s.identityLifecycle == nil {
		return nil, ErrInvalidCredentials
	}
	result, err := s.identityLifecycle.VerifyLoginMFA(ctx, input, metadata)
	if err != nil {
		return nil, err
	}
	return s.issueIdentityAuthentication(ctx, result, metadata)
}

// CompletePasskeyAuthentication completes WebAuthn verification before
// creating the same revocable JWT session used by password/MFA login.
func (s *AuthService) CompletePasskeyAuthentication(ctx context.Context, input models.IdentityPasskeyAuthenticationFinishInput, metadata models.IdentityRequestMetadata) (*TokenPair, error) {
	if s.identityLifecycle == nil {
		return nil, ErrInvalidCredentials
	}
	result, err := s.identityLifecycle.FinishPasskeyAuthentication(ctx, input, metadata)
	if err != nil {
		return nil, err
	}
	return s.issueIdentityAuthentication(ctx, result, metadata)
}

func (s *AuthService) issueIdentityAuthentication(ctx context.Context, result *models.IdentityAuthenticationResult, metadata models.IdentityRequestMetadata) (*TokenPair, error) {
	if result == nil || result.OrganizationID == "" || result.UserID == "" {
		return nil, ErrInvalidCredentials
	}
	user, err := s.userRepo.GetByID(ctx, result.OrganizationID, result.UserID)
	if err != nil || !user.IsActive() {
		return nil, ErrInvalidCredentials
	}
	authenticatedAt := result.AuthenticatedAt.UTC()
	return s.issueAuthenticatedSession(ctx, user, result.AuthenticationMethod, &authenticatedAt, metadata)
}

// RefreshToken validates and atomically rotates a persisted refresh token.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.parseToken(refreshToken, authdomain.TokenTypeRefresh)
	if err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.userRepo.GetByID(ctx, claims.OrganizationID, claims.UserID)
	if err != nil || !user.IsActive() {
		s.logger.Warn().Str("user_id", claims.UserID).Msg("refresh token for unavailable user")
		return nil, ErrInvalidToken
	}

	tokens, err := s.generateTokenPair(user, claims.MFAVerified)
	if err != nil {
		return nil, err
	}
	nextSession := sessionFromTokens(user, tokens, models.IdentityMethodPassword, nil, models.IdentityRequestMetadata{})
	rotated, err := s.userRepo.RotateSession(
		ctx,
		user.OrganizationID,
		user.ID,
		hashAuthToken(refreshToken),
		nextSession,
	)
	if err != nil {
		return nil, fmt.Errorf("rotating refresh token: %w", err)
	}
	if !rotated {
		return nil, ErrInvalidToken
	}

	s.logger.Info().Str("user_id", user.ID).Msg("token refreshed successfully")
	return tokens, nil
}

// CurrentUser returns the authenticated user in the asserted tenant.
func (s *AuthService) CurrentUser(ctx context.Context, orgID, userID string) (*models.User, error) {
	user, err := s.userRepo.GetByID(ctx, orgID, userID)
	if err != nil || !user.IsActive() {
		return nil, ErrInvalidToken
	}
	return user, nil
}

// Logout revokes the current persisted access/refresh token pair.
func (s *AuthService) Logout(ctx context.Context, orgID, userID, accessToken string) error {
	claims, err := s.parseToken(accessToken, authdomain.TokenTypeAccess)
	if err != nil || claims.OrganizationID != orgID || claims.UserID != userID {
		return ErrInvalidToken
	}
	if err := s.userRepo.RevokeSession(ctx, orgID, userID, hashAuthToken(accessToken)); err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

// ValidateAccessToken validates both JWT integrity and the corresponding
// persisted session. It satisfies middleware.AccessTokenValidator.
func (s *AuthService) ValidateAccessToken(ctx context.Context, token string) (*Claims, error) {
	claims, err := s.parseToken(token, authdomain.TokenTypeAccess)
	if err != nil {
		return nil, ErrInvalidToken
	}
	active, err := s.userRepo.IsSessionActive(ctx, claims.OrganizationID, claims.UserID, hashAuthToken(token))
	if err != nil || !active {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ValidateToken validates a token without checking its persisted session. It
// remains available for non-HTTP callers; request authentication must use
// ValidateAccessToken.
func (s *AuthService) ValidateToken(token string) (*Claims, error) {
	return s.parseToken(token, "")
}

func (s *AuthService) parseToken(tokenString, expectedType string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(s.jwtConfig.Secret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.jwtConfig.Issuer),
		jwt.WithAudience(authdomain.TokenAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.ID == "" || claims.Subject == "" || claims.Subject != claims.UserID ||
		claims.UserID == "" || claims.OrganizationID == "" || claims.Email == "" {
		return nil, ErrInvalidToken
	}
	if expectedType != "" && claims.TokenType != expectedType {
		return nil, ErrInvalidToken
	}
	if claims.TokenType != authdomain.TokenTypeAccess && claims.TokenType != authdomain.TokenTypeRefresh {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (s *AuthService) generateTokenPair(user *models.User, sessionMFAVerified ...bool) (*TokenPair, error) {
	now := time.Now().UTC()
	accessExpiry := now.Add(time.Duration(s.jwtConfig.ExpiryHours) * time.Hour)
	refreshExpiry := now.Add(7 * 24 * time.Hour)
	mfaVerified := len(sessionMFAVerified) > 0 && sessionMFAVerified[0]

	accessToken, err := s.signToken(user, authdomain.TokenTypeAccess, now, accessExpiry, mfaVerified)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.signToken(user, authdomain.TokenTypeRefresh, now, refreshExpiry, mfaVerified)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresAt:        accessExpiry,
		RefreshExpiresAt: refreshExpiry,
		User:             user,
	}, nil
}

func (s *AuthService) signToken(user *models.User, tokenType string, issuedAt, expiresAt time.Time, mfaVerified bool) (string, error) {
	claims := &Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Role:           string(user.Role),
		Email:          user.Email,
		TokenType:      tokenType,
		MFAVerified:    mfaVerified,
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{authdomain.TokenAudience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			Issuer:    s.jwtConfig.Issuer,
			Subject:   user.ID,
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.jwtConfig.Secret))
}

func (s *AuthService) persistSession(ctx context.Context, user *models.User, tokens *TokenPair) error {
	return s.userRepo.CreateSession(ctx, sessionFromTokens(user, tokens, models.IdentityMethodPassword, nil, models.IdentityRequestMetadata{}))
}

func sessionFromTokens(user *models.User, tokens *TokenPair, method models.IdentityMethod, mfaVerifiedAt *time.Time, metadata models.IdentityRequestMetadata) *models.UserSession {
	if method == "" {
		method = models.IdentityMethodPassword
	}
	metadata = normalizeAuthenticationMetadata(metadata)
	return &models.UserSession{
		ID:                   uuid.NewString(),
		UserID:               user.ID,
		OrganizationID:       user.OrganizationID,
		TokenHash:            hashAuthToken(tokens.AccessToken),
		RefreshTokenHash:     hashAuthToken(tokens.RefreshToken),
		IPAddress:            strings.TrimSpace(metadata.IPAddress),
		UserAgent:            strings.TrimSpace(metadata.UserAgent),
		DeviceName:           strings.TrimSpace(metadata.DeviceName),
		AuthenticationMethod: method,
		MFAVerifiedAt:        mfaVerifiedAt,
		ExpiresAt:            tokens.RefreshExpiresAt,
	}
}

func normalizeAuthenticationMetadata(metadata models.IdentityRequestMetadata) models.IdentityRequestMetadata {
	metadata.IPAddress = strings.TrimSpace(metadata.IPAddress)
	if metadata.IPAddress != "" && net.ParseIP(metadata.IPAddress) == nil {
		metadata.IPAddress = ""
	}
	metadata.UserAgent = truncateAuthenticationRunes(strings.TrimSpace(strings.ToValidUTF8(metadata.UserAgent, "")), 500)
	metadata.DeviceName = strings.TrimSpace(strings.ToValidUTF8(metadata.DeviceName, ""))
	deviceRunes := []rune(metadata.DeviceName)
	if len(deviceRunes) < 2 {
		metadata.DeviceName = ""
	} else if len(deviceRunes) > 120 {
		metadata.DeviceName = string(deviceRunes[:120])
	}
	return metadata
}

func truncateAuthenticationRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}

func (s *AuthService) issueAuthenticatedSession(ctx context.Context, user *models.User, method models.IdentityMethod, mfaVerifiedAt *time.Time, metadata models.IdentityRequestMetadata) (*TokenPair, error) {
	tokens, err := s.generateTokenPair(user, mfaVerifiedAt != nil)
	if err != nil {
		return nil, err
	}
	if err := s.userRepo.CreateSession(ctx, sessionFromTokens(user, tokens, method, mfaVerifiedAt, metadata)); err != nil {
		return nil, fmt.Errorf("creating authenticated session: %w", err)
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	if err := s.userRepo.UpdateLastLogin(ctx, user.OrganizationID, user.ID, now); err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to update last login")
	}
	return tokens, nil
}

func hashAuthToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
