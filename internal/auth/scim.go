package auth

import (
	"context"
	"errors"
)

var ErrInvalidSCIMToken = errors.New("invalid SCIM bearer token")

// SCIMPrincipal is the tenant and exact scope set established by a dedicated
// SCIM credential. It deliberately carries neither the raw credential nor a
// browser role/JWT identity.
type SCIMPrincipal struct {
	TokenID            string
	OrganizationID     string
	CreatedByUserID    string
	Scopes             []string
	RateLimitPerMinute int
}

type SCIMTokenAuthenticator interface {
	AuthenticateSCIMToken(ctx context.Context, rawToken, clientIP string) (*SCIMPrincipal, error)
}
