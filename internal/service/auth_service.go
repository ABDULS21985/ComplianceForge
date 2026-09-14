package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// AuthService handles authentication, registration, and revocable sessions.
type AuthService struct {
	userRepo  UserRepository
	jwtConfig config.JWTConfig
	logger    zerolog.Logger
}

// NewAuthService constructs a new AuthService.
func NewAuthService(userRepo UserRepository, jwtCfg config.JWTConfig, logger zerolog.Logger) *AuthService {
	return &AuthService{
		userRepo:  userRepo,
		jwtConfig: jwtCfg,
		logger:    logger.With().Str("service", "auth").Logger(),
	}
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

	tokens, err := s.generateTokenPair(user)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to generate token pair")
		return nil, err
	}
	if err := s.persistSession(ctx, user, tokens); err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to persist login session")
		return nil, fmt.Errorf("creating login session: %w", err)
	}

	now := time.Now().UTC()
	user.LastLoginAt = &now
	if err := s.userRepo.UpdateLastLogin(ctx, user.OrganizationID, user.ID, now); err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to update last login")
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

	if err := s.userRepo.Create(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("email", req.Email).Msg("failed to create user")
		return nil, err
	}

	tokens, err := s.generateTokenPair(user)
	if err != nil {
		return nil, err
	}
	if err := s.persistSession(ctx, user, tokens); err != nil {
		return nil, fmt.Errorf("creating registration session: %w", err)
	}

	s.logger.Info().Str("user_id", user.ID).Str("email", req.Email).Msg("user registered successfully")
	return tokens, nil
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

	tokens, err := s.generateTokenPair(user)
	if err != nil {
		return nil, err
	}
	nextSession := sessionFromTokens(user, tokens)
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

func (s *AuthService) generateTokenPair(user *models.User) (*TokenPair, error) {
	now := time.Now().UTC()
	accessExpiry := now.Add(time.Duration(s.jwtConfig.ExpiryHours) * time.Hour)
	refreshExpiry := now.Add(7 * 24 * time.Hour)

	accessToken, err := s.signToken(user, authdomain.TokenTypeAccess, now, accessExpiry)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.signToken(user, authdomain.TokenTypeRefresh, now, refreshExpiry)
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

func (s *AuthService) signToken(user *models.User, tokenType string, issuedAt, expiresAt time.Time) (string, error) {
	claims := &Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Role:           string(user.Role),
		Email:          user.Email,
		TokenType:      tokenType,
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
	return s.userRepo.CreateSession(ctx, sessionFromTokens(user, tokens))
}

func sessionFromTokens(user *models.User, tokens *TokenPair) *models.UserSession {
	return &models.UserSession{
		ID:               uuid.NewString(),
		UserID:           user.ID,
		OrganizationID:   user.OrganizationID,
		TokenHash:        hashAuthToken(tokens.AccessToken),
		RefreshTokenHash: hashAuthToken(tokens.RefreshToken),
		ExpiresAt:        tokens.RefreshExpiresAt,
	}
}

func hashAuthToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
