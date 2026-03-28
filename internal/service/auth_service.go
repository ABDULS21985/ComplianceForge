package service

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrUserNotActive      = errors.New("user account is not active")
)

// TokenPair holds the access and refresh tokens returned after authentication.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Claims represents JWT claims used throughout the platform.
type Claims struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
	Email          string `json:"email"`
	jwt.RegisteredClaims
}

// RegisterRequest holds the data needed to register a new user.
type RegisterRequest struct {
	Email          string          `json:"email" validate:"required,email"`
	Password       string          `json:"password" validate:"required,min=8"`
	FirstName      string          `json:"first_name" validate:"required"`
	LastName       string          `json:"last_name" validate:"required"`
	OrganizationID string          `json:"organization_id" validate:"required,uuid"`
	Role           models.UserRole `json:"role"`
	Department     string          `json:"department"`
}

// UserRepository defines the data access interface needed by AuthService.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.User, int, error)
}

// AuthService handles authentication, registration, and token management.
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

// Login authenticates a user by email and password and returns a token pair.
func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		s.logger.Warn().Str("email", email).Msg("login attempt for unknown email")
		return nil, ErrInvalidCredentials
	}

	if !user.IsActive {
		s.logger.Warn().Str("user_id", user.ID).Msg("login attempt for inactive user")
		return nil, ErrUserNotActive
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		s.logger.Warn().Str("email", email).Msg("invalid password attempt")
		return nil, ErrInvalidCredentials
	}

	tokenPair, err := s.generateTokenPair(user)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to generate token pair")
		return nil, err
	}

	// Update last login timestamp.
	now := time.Now()
	user.LastLoginAt = &now
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to update last login")
		// Non-fatal: we still return the tokens.
	}

	s.logger.Info().Str("user_id", user.ID).Str("email", email).Msg("user logged in successfully")
	return tokenPair, nil
}

// Register creates a new user account with a hashed password.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*models.User, error) {
	existing, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		return nil, ErrUserAlreadyExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to hash password")
		return nil, errors.New("failed to process registration")
	}

	role := req.Role
	if role == "" {
		role = models.UserRoleViewer
	}

	user := &models.User{
		TenantModel: models.TenantModel{
			OrganizationID: req.OrganizationID,
		},
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Role:         role,
		Department:   req.Department,
		IsActive:     true,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("email", req.Email).Msg("failed to create user")
		return nil, err
	}

	s.logger.Info().Str("user_id", user.ID).Str("email", req.Email).Msg("user registered successfully")
	return user, nil
}

// RefreshToken validates a refresh token and issues a new token pair.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.ValidateToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		s.logger.Warn().Str("user_id", claims.UserID).Msg("refresh token for unknown user")
		return nil, ErrInvalidToken
	}

	if !user.IsActive {
		return nil, ErrUserNotActive
	}

	tokenPair, err := s.generateTokenPair(user)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", user.ID).Msg("failed to generate refreshed token pair")
		return nil, err
	}

	s.logger.Info().Str("user_id", user.ID).Msg("token refreshed successfully")
	return tokenPair, nil
}

// ValidateToken parses and validates a JWT token string, returning the claims.
func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.jwtConfig.Secret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// generateTokenPair creates a new access token and refresh token for the given user.
func (s *AuthService) generateTokenPair(user *models.User) (*TokenPair, error) {
	accessExpiry := time.Now().Add(time.Duration(s.jwtConfig.ExpiryHours) * time.Hour)
	refreshExpiry := time.Now().Add(time.Duration(s.jwtConfig.ExpiryHours) * time.Hour * 7) // 7x access token lifetime

	accessClaims := &Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Role:           string(user.Role),
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.jwtConfig.Issuer,
			Subject:   user.ID,
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return nil, err
	}

	refreshClaims := &Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Role:           string(user.Role),
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.jwtConfig.Issuer,
			Subject:   user.ID,
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshTokenObj.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    accessExpiry,
	}, nil
}
