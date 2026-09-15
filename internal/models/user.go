package models

import "time"

// UserRole is the stable role slug stored in roles.slug and embedded in JWTs.
type UserRole string

const (
	UserRoleSuperAdmin        UserRole = "super_admin"
	UserRoleAdmin             UserRole = "org_admin"
	UserRoleComplianceOfficer UserRole = "compliance_manager"
	UserRoleRiskManager       UserRole = "risk_manager"
	UserRoleAuditor           UserRole = "auditor"
	UserRolePolicyOwner       UserRole = "policy_owner"
	UserRoleDPO               UserRole = "dpo"
	UserRoleCISO              UserRole = "ciso"
	UserRoleViewer            UserRole = "viewer"
	UserRoleExternalAuditor   UserRole = "external_auditor"
)

// UserStatus mirrors the user_status PostgreSQL enum.
type UserStatus string

const (
	UserStatusActive              UserStatus = "active"
	UserStatusInactive            UserStatus = "inactive"
	UserStatusLocked              UserStatus = "locked"
	UserStatusPendingVerification UserStatus = "pending_verification"
)

// User represents an authenticated user belonging to an organization. Role
// and MFAEnabled are read-model fields derived from user_roles and user_mfa.
type User struct {
	TenantModel
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	FirstName    string     `json:"first_name"`
	LastName     string     `json:"last_name"`
	JobTitle     string     `json:"job_title,omitempty"`
	Department   string     `json:"department,omitempty"`
	Phone        string     `json:"phone,omitempty"`
	AvatarURL    string     `json:"avatar_url,omitempty"`
	Status       UserStatus `json:"status"`
	IsSuperAdmin bool       `json:"is_super_admin"`
	Timezone     string     `json:"timezone,omitempty"`
	Language     string     `json:"language"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	Role         UserRole   `json:"role"`
	MFAEnabled   bool       `json:"mfa_enabled"`
}

// IsActive reports whether the account is permitted to authenticate.
func (u *User) IsActive() bool {
	return u != nil && u.Status == UserStatusActive
}

// UserSession is the persisted, revocable relationship between an access and
// refresh token pair. Only SHA-256 token hashes are stored.
type UserSession struct {
	ID                   string
	UserID               string
	OrganizationID       string
	TokenHash            string
	RefreshTokenHash     string
	IPAddress            string
	UserAgent            string
	DeviceName           string
	AuthenticationMethod IdentityMethod
	MFAVerifiedAt        *time.Time
	ExpiresAt            time.Time
	RevokedAt            *time.Time
	CreatedAt            time.Time
}
