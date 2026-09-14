package models

import (
	"encoding/json"
	"time"
)

type PermissionGrant struct {
	ID          string `json:"id,omitempty"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
}

type ManagedRole struct {
	ID             string            `json:"id"`
	OrganizationID *string           `json:"organization_id,omitempty"`
	Name           string            `json:"name"`
	Slug           string            `json:"slug"`
	Description    string            `json:"description,omitempty"`
	IsSystemRole   bool              `json:"is_system_role"`
	IsCustom       bool              `json:"is_custom"`
	Version        int64             `json:"version"`
	CreatedBy      *string           `json:"created_by,omitempty"`
	UpdatedBy      *string           `json:"updated_by,omitempty"`
	Permissions    []PermissionGrant `json:"permissions"`
	AssignedUsers  int               `json:"assigned_users"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	DeletedAt      *time.Time        `json:"deleted_at,omitempty"`
}

type ManagedRoleCreateInput struct {
	Name        string            `json:"name"`
	Slug        string            `json:"slug,omitempty"`
	Description string            `json:"description,omitempty"`
	Permissions []PermissionGrant `json:"permissions"`
}

type ManagedRolePatch struct {
	Name             *string            `json:"name,omitempty"`
	Slug             *string            `json:"slug,omitempty"`
	Description      *string            `json:"description,omitempty"`
	ClearDescription bool               `json:"clear_description,omitempty"`
	Permissions      *[]PermissionGrant `json:"permissions,omitempty"`
	ExpectedVersion  int64              `json:"expected_version"`
}

type ManagedRoleCloneInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
}

type ManagedRoleListFilter struct {
	PaginationRequest
	Search        string
	IncludeSystem bool
}

type ManagedRoleImpact struct {
	RoleID              string            `json:"role_id"`
	AssignedUsers       int               `json:"assigned_users"`
	CurrentPermissions  int               `json:"current_permissions"`
	ProposedPermissions int               `json:"proposed_permissions"`
	Added               []PermissionGrant `json:"added"`
	Removed             []PermissionGrant `json:"removed"`
}

type ManagedRoleAssignment struct {
	RoleID     string    `json:"role_id"`
	UserID     string    `json:"user_id"`
	Email      string    `json:"email"`
	FirstName  string    `json:"first_name"`
	LastName   string    `json:"last_name"`
	AssignedBy *string   `json:"assigned_by,omitempty"`
	AssignedAt time.Time `json:"assigned_at"`
}

type ManagedRoleAssignmentInput struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason"`
}

type ManagedRoleUnassignmentInput struct {
	Reason string `json:"reason"`
}

type RoleChangeEvent struct {
	ID           string          `json:"id"`
	RoleID       string          `json:"role_id"`
	TargetUserID *string         `json:"target_user_id,omitempty"`
	EventType    string          `json:"event_type"`
	ActorUserID  string          `json:"actor_user_id"`
	RoleVersion  int64           `json:"role_version"`
	Reason       string          `json:"reason,omitempty"`
	BeforeState  json.RawMessage `json:"before_state,omitempty"`
	AfterState   json.RawMessage `json:"after_state,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}
