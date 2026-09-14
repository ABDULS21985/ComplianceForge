// Package auth contains the transport-independent authentication contracts
// shared by HTTP middleware, handlers, and the authentication service.
package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/complianceforge/platform/internal/models"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
	TokenAudience    = "complianceforge-api"
)

// Claims are the signed identity attributes carried by access and refresh
// tokens. TokenType prevents an access token from being accepted by the
// refresh endpoint (and vice versa).
type Claims struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
	Email          string `json:"email"`
	TokenType      string `json:"token_type"`
	jwt.RegisteredClaims
}

// LoginRequest is accepted by the login service. OrganizationID is optional
// for backwards compatibility, but clients should provide it because email is
// unique per organization rather than globally.
type LoginRequest struct {
	Email          string `json:"email" validate:"required,email"`
	Password       string `json:"password" validate:"required,min=8"`
	OrganizationID string `json:"organization_id,omitempty" validate:"omitempty,uuid"`
}

// RegisterRequest contains the fields permitted during public registration.
// A caller cannot choose its own privileged role; new users start as viewers.
type RegisterRequest struct {
	Email          string `json:"email" validate:"required,email"`
	Password       string `json:"password" validate:"required,min=8"`
	FirstName      string `json:"first_name" validate:"required,max=100"`
	LastName       string `json:"last_name" validate:"required,max=100"`
	OrganizationID string `json:"organization_id" validate:"required,uuid"`
	Department     string `json:"department,omitempty" validate:"max=200"`
}

// RefreshRequest is the payload for a refresh-token exchange.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// TokenPair is returned by login, registration, and token refresh. Returning
// the user makes the response usable by clients without a second round trip;
// RefreshExpiresAt is intentionally internal and is used to persist sessions.
type TokenPair struct {
	AccessToken      string       `json:"access_token"`
	RefreshToken     string       `json:"refresh_token"`
	ExpiresAt        time.Time    `json:"expires_at"`
	User             *models.User `json:"user"`
	RefreshExpiresAt time.Time    `json:"-"`
}
