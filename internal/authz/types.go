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
	Allowed            bool         `json:"allowed"`
	Reason             string       `json:"reason"`
	ReasonCode         string       `json:"reason_code"`
	PolicyID           string       `json:"policy_id,omitempty"`
	PolicyName         string       `json:"policy_name,omitempty"`
	DecisionID         string       `json:"decision_id,omitempty"`
	ConstraintsApplied bool         `json:"constraints_applied"`
	Obligations        []Obligation `json:"obligations"`
}

// Obligation describes a downstream control that must be applied after an
// allow decision, such as watermarking an auditor export. An obligation is not
// itself an authorization grant.
type Obligation struct {
	Kind       string            `json:"kind"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// Authorizer is implemented by RBAC, ABAC, or a composed policy engine.
// Implementations must default to deny when no policy grants access.
type Authorizer interface {
	Authorize(context.Context, Request) (Decision, error)
}
