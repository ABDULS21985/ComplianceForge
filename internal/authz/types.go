// Package authz defines the application-wide authorization decision contract.
// Keeping this contract outside HTTP and persistence packages lets every entry
// point enforce the same fail-closed policy semantics.
package authz

import "context"

// Request describes one attempted action by an authenticated principal.
type Request struct {
	SubjectID      string
	OrganizationID string
	Role           string
	Resource       string
	ResourceID     string
	Action         string
	IPAddress      string
	MFAVerified    bool
	Attributes     map[string]any
}

// Decision is the auditable result returned by a policy decision point.
type Decision struct {
	Allowed    bool
	Reason     string
	PolicyID   string
	PolicyName string
}

// Authorizer is implemented by RBAC, ABAC, or a composed policy engine.
// Implementations must default to deny when no policy grants access.
type Authorizer interface {
	Authorize(context.Context, Request) (Decision, error)
}
