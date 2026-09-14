package models

import (
	"encoding/json"
	"time"
)

type DirectoryInvitationStatus string

const (
	DirectoryInvitationNotRequired DirectoryInvitationStatus = "not_required"
	DirectoryInvitationReady       DirectoryInvitationStatus = "ready"
	DirectoryInvitationSent        DirectoryInvitationStatus = "sent"
	DirectoryInvitationAccepted    DirectoryInvitationStatus = "accepted"
	DirectoryInvitationExpired     DirectoryInvitationStatus = "expired"
	DirectoryInvitationRevoked     DirectoryInvitationStatus = "revoked"
)

type DirectoryUser struct {
	TenantModel
	Email               string                    `json:"email"`
	FirstName           string                    `json:"first_name"`
	LastName            string                    `json:"last_name"`
	JobTitle            string                    `json:"job_title,omitempty"`
	Department          string                    `json:"department,omitempty"`
	Phone               string                    `json:"phone,omitempty"`
	AvatarURL           string                    `json:"avatar_url,omitempty"`
	Status              UserStatus                `json:"status"`
	IsSuperAdmin        bool                      `json:"is_super_admin"`
	Timezone            string                    `json:"timezone,omitempty"`
	Language            string                    `json:"language"`
	LastLoginAt         *time.Time                `json:"last_login_at,omitempty"`
	MFAEnabled          bool                      `json:"mfa_enabled"`
	ManagerUserID       *string                   `json:"manager_user_id,omitempty"`
	Manager             *DirectoryUserReference   `json:"manager,omitempty"`
	EmployeeID          string                    `json:"employee_id,omitempty"`
	Location            string                    `json:"location,omitempty"`
	InvitationStatus    DirectoryInvitationStatus `json:"invitation_status"`
	InvitedAt           *time.Time                `json:"invited_at,omitempty"`
	InvitationExpiresAt *time.Time                `json:"invitation_expires_at,omitempty"`
	InvitedBy           *string                   `json:"invited_by,omitempty"`
	SuspendedAt         *time.Time                `json:"suspended_at,omitempty"`
	SuspendedBy         *string                   `json:"suspended_by,omitempty"`
	SuspensionReason    string                    `json:"suspension_reason,omitempty"`
	ReactivatedAt       *time.Time                `json:"reactivated_at,omitempty"`
	DeprovisionedAt     *time.Time                `json:"deprovisioned_at,omitempty"`
	DeprovisionedBy     *string                   `json:"deprovisioned_by,omitempty"`
	DeprovisionReason   string                    `json:"deprovision_reason,omitempty"`
	UpdatedBy           *string                   `json:"updated_by,omitempty"`
	Version             int64                     `json:"version"`
	RoleSlugs           []string                  `json:"role_slugs"`
	GroupCount          int                       `json:"group_count"`
}

type DirectoryUserReference struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type DirectoryUserCreateInput struct {
	Email               string     `json:"email"`
	FirstName           string     `json:"first_name"`
	LastName            string     `json:"last_name"`
	JobTitle            string     `json:"job_title,omitempty"`
	Department          string     `json:"department,omitempty"`
	Phone               string     `json:"phone,omitempty"`
	AvatarURL           string     `json:"avatar_url,omitempty"`
	Timezone            string     `json:"timezone,omitempty"`
	Language            string     `json:"language,omitempty"`
	ManagerUserID       *string    `json:"manager_user_id,omitempty"`
	EmployeeID          string     `json:"employee_id,omitempty"`
	Location            string     `json:"location,omitempty"`
	InitialRoleSlug     string     `json:"initial_role_slug,omitempty"`
	InitialStatus       UserStatus `json:"initial_status,omitempty"`
	InvitationExpiresAt *time.Time `json:"invitation_expires_at,omitempty"`
	Reason              string     `json:"reason"`
}

type DirectoryUserPatch struct {
	ExpectedVersion int64   `json:"expected_version"`
	Email           *string `json:"email,omitempty"`
	FirstName       *string `json:"first_name,omitempty"`
	LastName        *string `json:"last_name,omitempty"`
	JobTitle        *string `json:"job_title,omitempty"`
	ClearJobTitle   bool    `json:"clear_job_title,omitempty"`
	Department      *string `json:"department,omitempty"`
	ClearDepartment bool    `json:"clear_department,omitempty"`
	Phone           *string `json:"phone,omitempty"`
	ClearPhone      bool    `json:"clear_phone,omitempty"`
	AvatarURL       *string `json:"avatar_url,omitempty"`
	ClearAvatarURL  bool    `json:"clear_avatar_url,omitempty"`
	Timezone        *string `json:"timezone,omitempty"`
	ClearTimezone   bool    `json:"clear_timezone,omitempty"`
	Language        *string `json:"language,omitempty"`
	ManagerUserID   *string `json:"manager_user_id,omitempty"`
	ClearManager    bool    `json:"clear_manager,omitempty"`
	EmployeeID      *string `json:"employee_id,omitempty"`
	ClearEmployeeID bool    `json:"clear_employee_id,omitempty"`
	Location        *string `json:"location,omitempty"`
	ClearLocation   bool    `json:"clear_location,omitempty"`
	Reason          string  `json:"reason"`
}

type DirectoryUserStateInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type DirectoryUserDeprovisionInput struct {
	ExpectedVersion   int64   `json:"expected_version"`
	Reason            string  `json:"reason"`
	ReplacementUserID *string `json:"replacement_user_id,omitempty"`
}

type DirectoryOwnershipTransferInput struct {
	ExpectedVersion   int64  `json:"expected_version"`
	ReplacementUserID string `json:"replacement_user_id"`
	Reason            string `json:"reason"`
}

type DirectoryUserListFilter struct {
	PaginationRequest
	Search         string
	Status         string
	Department     string
	Location       string
	ManagerID      string
	RoleSlug       string
	GroupID        string
	IncludeDeleted bool
	SortBy         string
	SortDirection  string
}

type DirectoryOwnershipResource struct {
	Resource string `json:"resource"`
	Count    int    `json:"count"`
}

type DirectoryOwnershipImpact struct {
	UserID                    string                       `json:"user_id"`
	Resources                 []DirectoryOwnershipResource `json:"resources"`
	Total                     int                          `json:"total"`
	DirectReports             int                          `json:"direct_reports"`
	StaticGroupMemberships    int                          `json:"static_group_memberships"`
	PersistedRoleAssignments  int                          `json:"persisted_role_assignments"`
	HasAdministrativeGrant    bool                         `json:"has_administrative_grant"`
	IsLastActiveAdministrator bool                         `json:"is_last_active_administrator"`
	RequiresReplacement       bool                         `json:"requires_replacement"`
}

type DirectoryGroupType string

const (
	DirectoryGroupStatic  DirectoryGroupType = "static"
	DirectoryGroupDynamic DirectoryGroupType = "dynamic"
)

type DirectoryDynamicGroupRule struct {
	Departments []string     `json:"departments,omitempty"`
	Locations   []string     `json:"locations,omitempty"`
	Statuses    []UserStatus `json:"statuses,omitempty"`
	RoleSlugs   []string     `json:"role_slugs,omitempty"`
}

type DirectoryGroup struct {
	TenantModel
	Name           string             `json:"name"`
	Slug           string             `json:"slug"`
	Description    string             `json:"description,omitempty"`
	GroupType      DirectoryGroupType `json:"group_type"`
	MembershipRule json.RawMessage    `json:"membership_rule"`
	Version        int64              `json:"version"`
	CreatedBy      string             `json:"created_by"`
	UpdatedBy      string             `json:"updated_by"`
	MemberCount    int                `json:"member_count"`
}

type DirectoryGroupCreateInput struct {
	Name           string             `json:"name"`
	Slug           string             `json:"slug,omitempty"`
	Description    string             `json:"description,omitempty"`
	GroupType      DirectoryGroupType `json:"group_type,omitempty"`
	MembershipRule json.RawMessage    `json:"membership_rule,omitempty"`
	Reason         string             `json:"reason"`
}

type DirectoryGroupPatch struct {
	ExpectedVersion  int64           `json:"expected_version"`
	Name             *string         `json:"name,omitempty"`
	Slug             *string         `json:"slug,omitempty"`
	Description      *string         `json:"description,omitempty"`
	ClearDescription bool            `json:"clear_description,omitempty"`
	MembershipRule   json.RawMessage `json:"membership_rule,omitempty"`
	Reason           string          `json:"reason"`
}

type DirectoryGroupListFilter struct {
	PaginationRequest
	Search        string
	GroupType     string
	SortBy        string
	SortDirection string
}

type DirectoryGroupMemberInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	UserID          string `json:"user_id"`
	Reason          string `json:"reason"`
}

type DirectoryGroupMemberRemoveInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type DirectoryGroupBulkMembersInput struct {
	ExpectedVersion int64    `json:"expected_version"`
	AddUserIDs      []string `json:"add_user_ids,omitempty"`
	RemoveUserIDs   []string `json:"remove_user_ids,omitempty"`
	Reason          string   `json:"reason"`
}

type DirectoryChangeEvent struct {
	ID            string          `json:"id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	TargetUserID  *string         `json:"target_user_id,omitempty"`
	EventType     string          `json:"event_type"`
	ActorUserID   string          `json:"actor_user_id"`
	EntityVersion int64           `json:"entity_version"`
	Reason        string          `json:"reason"`
	BeforeState   json.RawMessage `json:"before_state,omitempty"`
	AfterState    json.RawMessage `json:"after_state,omitempty"`
	RequestID     string          `json:"request_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type DirectoryImportRow struct {
	RowNumber    int    `json:"row_number"`
	Email        string `json:"email"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	JobTitle     string `json:"job_title,omitempty"`
	Department   string `json:"department,omitempty"`
	Location     string `json:"location,omitempty"`
	EmployeeID   string `json:"employee_id,omitempty"`
	ManagerEmail string `json:"manager_email,omitempty"`
	RoleSlug     string `json:"role_slug,omitempty"`
}

type DirectoryImportRowResult struct {
	RowNumber int      `json:"row_number"`
	Email     string   `json:"email"`
	Action    string   `json:"action,omitempty"`
	Errors    []string `json:"errors,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

type DirectoryImportPreview struct {
	ContentSHA256 string                     `json:"content_sha256"`
	RowCount      int                        `json:"row_count"`
	ValidCount    int                        `json:"valid_count"`
	InvalidCount  int                        `json:"invalid_count"`
	CreateCount   int                        `json:"create_count"`
	UpdateCount   int                        `json:"update_count"`
	Rows          []DirectoryImportRowResult `json:"rows"`
}

type DirectoryImportResult struct {
	ID             string                     `json:"id"`
	IdempotencyKey string                     `json:"idempotency_key"`
	ContentSHA256  string                     `json:"content_sha256"`
	RowCount       int                        `json:"row_count"`
	CreatedCount   int                        `json:"created_count"`
	UpdatedCount   int                        `json:"updated_count"`
	SkippedCount   int                        `json:"skipped_count"`
	Rows           []DirectoryImportRowResult `json:"rows"`
	AppliedAt      time.Time                  `json:"applied_at"`
	Replayed       bool                       `json:"replayed"`
}
